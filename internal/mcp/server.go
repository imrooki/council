package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/i18n"
	"council/internal/turn"
)

// Options 是 `council mcp` 自己的开关。Version 与 Verbose 不在这里：它们
// 由 app.Options 的同名字段带进来（Version 是构建时注入的版本号，Verbose
// 是全局 -verbose），app 原样填进 Deps，避免两个地方各存一份再比谁生效。
type Options struct {
	// Lang 是没带 lang 参数时的默认输出语言，由 -lang / COUNCIL_LANG /
	// 配置里的 language 求出来。
	Lang i18n.Lang
}

// Deps 是 internal/app 在装配完成之后填进来的东西。MCP 自己不加载配置、
// 不打开库、不构造 Runner：那三件事的顺序（先合并 Claude agents、再
// NewRunner）在 app 里只有一处，不在这里复制第二份。
type Deps struct {
	Cfg                 *config.Config
	Svc                 *conversation.Service
	Rn                  *turn.Runner
	DataDir, ProjectDir string
	In                  io.Reader
	Out, Err            io.Writer
	Version             string
	Verbose             bool
	Lang                i18n.Lang
	Claude              ClaudeImport
}

// shutdownCancelGrace 是收到关闭信号（Ctrl-C）之后等工具收尾的上限。
// stdin 正常结束时不用它：那种情况要让在飞请求跑完，否则管道里喂完就关
// stdin 的脚本永远拿不到 council_ask 的结果。
const shutdownCancelGrace = 5 * time.Second

// lockedWriter 让并发工具调用写 stderr 时不会互相插进对方的一行中间。
// 只用一把粗锁：stderr 是诊断流，不值得为它做更细的同步。
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// inflightCall 是一个还没回包的 tools/call。cancelled 由
// notifications/cancelled 置位；置位之后即使工具自己跑完了也不回包
// （MCP 规范：被取消的请求不再发送响应）。
type inflightCall struct {
	cancel    context.CancelFunc
	cancelled bool
}

type server struct {
	deps    Deps
	in      io.Reader
	out     io.Writer
	err     *lockedWriter
	lang    i18n.Lang
	version string

	rootCtx context.Context

	writeMu   sync.Mutex
	outBroken atomic.Bool

	mu    sync.Mutex
	calls map[string]*inflightCall

	wg sync.WaitGroup
}

// Run 在 stdio 上服务到 stdin 结束（或 ctx 被取消），然后返回。
//
// 它永远返回 nil：协议错误、读写出错、工具失败都已经写成 stderr 或
// isError 响应，MCP 宿主不看退出码；把失败往上抛只会让 app.Run 多打一行
// 重复的错误，退出码仍然是 0。
func Run(ctx context.Context, deps Deps) error {
	in := deps.In
	if in == nil {
		in = strings.NewReader("")
	}
	out := deps.Out
	if out == nil {
		out = io.Discard
	}
	errOut := deps.Err
	if errOut == nil {
		errOut = io.Discard
	}
	lang := deps.Lang
	if lang == "" {
		lang = i18n.Default
	}
	version := deps.Version
	if version == "" {
		// 直接 go build 的二进制保留这个占位值；scripts/build.ps1 才会
		// 注入真版本。MCP 的 initialize 必须有话可说，不能给空串。
		version = "0.0.0-dev"
	}

	s := &server{
		deps:    deps,
		in:      in,
		out:     out,
		err:     &lockedWriter{w: errOut},
		lang:    lang,
		version: version,
		rootCtx: ctx,
		calls:   make(map[string]*inflightCall),
	}
	s.logf(fmt.Sprintf("stdio 服务器就绪（MCP %s，%d 个工具）", protocolVersion, len(toolDefinitions())))
	s.serve()
	s.logf("退出")
	return nil
}

type readEvent struct {
	line string
	err  error
}

func (s *server) serve() {
	events := make(chan readEvent)
	go s.readLoop(events)

	for {
		select {
		case <-s.rootCtx.Done():
			n := s.cancelAll()
			s.logf(fmt.Sprintf("收到关闭信号，中断 %d 个在飞请求", n))
			s.waitFor(shutdownCancelGrace)
			return
		case ev, ok := <-events:
			if !ok {
				// stdin 读到底：先让在飞请求跑完再走。工具自己有超时
				// （council_ask / council_round 都是 10 分钟），不会永远挂着；
				// 这里不主动取消，是因为「把几条消息一次喂完就关 stdin」
				// 的脚本与测试正是靠这一条拿到 tools/call 的结果。
				if n := s.inFlight(); n > 0 {
					s.logf(fmt.Sprintf("stdin 结束，等待 %d 个在飞请求完成", n))
				}
				s.wg.Wait()
				return
			}
			if ev.err != nil {
				s.logf(fmt.Sprintf("读 stdin 失败: %v", ev.err))
				continue
			}
			if ev.line != "" {
				s.handleSafely(ev.line)
			}
		}
	}
}

// readLoop 一次读一行（NDJSON）。用 ReadString 而不是 Scanner：Scanner 的
// 单行上限是 64 KiB，一个带长 prompt 的 tools/call 很容易超；ReadString 按
// 需扩容，也没有「扫到一半报 token too long」的失败模式。
func (s *server) readLoop(events chan<- readEvent) {
	defer close(events)
	reader := bufio.NewReaderSize(s.in, 64*1024)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			events <- readEvent{line: strings.TrimRight(line, "\r\n")}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				events <- readEvent{err: err}
			}
			return
		}
	}
}

func (s *server) waitFor(limit time.Duration) {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(limit):
		s.logf("等待超时，仍有请求未结束；进程退出（provider 子进程随父进程结束）")
	}
}

func (s *server) inFlight() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *server) cancelAll() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, call := range s.calls {
		call.cancelled = true
		call.cancel()
	}
	return len(s.calls)
}

// handleSafely 把 dispatched 的一行包起来。协议处理里出现 panic 不该让
// 进程直接死掉：MCP 宿主只会看到管道断了，而拿不到任何说明。
func (s *server) handleSafely(line string) {
	defer func() {
		if r := recover(); r != nil {
			s.logf(fmt.Sprintf("处理请求时 panic: %v\n%s", r, debug.Stack()))
			s.writeError(nil, codeInternalError, "internal error")
		}
	}()
	s.handleLine(line)
}

func (s *server) handleLine(line string) {
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		s.writeError(nil, codeParseError, "parse error: "+err.Error())
		return
	}
	if !isJSONObject(raw) {
		// MCP 不用 JSON-RPC 批量数组；标量也不是合法请求。
		s.writeError(nil, codeInvalidRequest, "invalid request: expected a JSON-RPC object")
		return
	}

	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		s.writeError(nil, codeInvalidRequest, "invalid request: "+err.Error())
		return
	}
	if req.JSONRPC != "" && req.JSONRPC != jsonrpcVersion {
		s.writeError(req.ID, codeInvalidRequest, `invalid request: jsonrpc must be "2.0"`)
		return
	}
	if req.Method == "" {
		s.writeError(req.ID, codeInvalidRequest, "invalid request: method must be a non-empty string")
		return
	}
	hasID := len(req.ID) > 0 && string(req.ID) != "null"

	switch req.Method {
	case "initialize":
		if len(req.Params) > 0 && string(req.Params) != "null" && !isJSONObject(req.Params) {
			if hasID {
				s.writeError(req.ID, codeInvalidParams, "invalid params: params must be a JSON object")
			}
			return
		}
		if hasID {
			s.writeResult(req.ID, initializeResult(s.version))
		}
	case "ping":
		if hasID {
			s.writeResult(req.ID, map[string]any{})
		}
	case "tools/list":
		if hasID {
			s.writeResult(req.ID, map[string]any{"tools": toolDefinitions()})
		}
	case "tools/call":
		s.handleToolCall(req, hasID)
	case "notifications/cancelled":
		s.handleCancelled(req.Params)
	default:
		if strings.HasPrefix(req.Method, "notifications/") {
			// notifications/initialized 以及别的 notification：没有响应体。
			// 不认识的 notification 也安静吞掉，JSON-RPC 本来就不允许回包。
			return
		}
		if hasID {
			s.writeError(req.ID, codeMethodNotFound, "method not found: "+req.Method)
		}
	}
}

type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *server) handleToolCall(req request, hasID bool) {
	if !isJSONObject(req.Params) {
		if hasID {
			s.writeError(req.ID, codeInvalidParams, "invalid params: params must be a JSON object")
		}
		return
	}
	var params callToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		if hasID {
			s.writeError(req.ID, codeInvalidParams, "invalid params: "+err.Error())
		}
		return
	}
	name := strings.TrimSpace(params.Name)
	if name == "" {
		if hasID {
			s.writeError(req.ID, codeInvalidParams, "invalid params: missing tool name")
		}
		return
	}
	if !knownTool(name) {
		if hasID {
			s.writeError(req.ID, codeInvalidParams, fmt.Sprintf("invalid params: unknown tool %q", name))
		}
		return
	}
	if len(params.Arguments) > 0 && string(params.Arguments) != "null" && !isJSONObject(params.Arguments) {
		if hasID {
			s.writeError(req.ID, codeInvalidParams, "invalid params: arguments must be a JSON object")
		}
		return
	}
	s.startToolCall(req.ID, hasID, name, params.Arguments)
}

// startToolCall 把一次工具调用扔进 goroutine，读循环立刻回去收下一行。
// 这是 notifications/cancelled 能在几十秒的议会期间被处理的前提。
func (s *server) startToolCall(id json.RawMessage, hasID bool, name string, args json.RawMessage) {
	key, keyOK := "", false
	if hasID {
		key, keyOK = idKey(id)
	}
	ctx := s.rootCtx
	cancel := func() {}
	if hasID {
		ctx, cancel = context.WithCancel(s.rootCtx)
	}
	call := &inflightCall{cancel: cancel}
	if keyOK {
		s.mu.Lock()
		s.calls[key] = call
		s.mu.Unlock()
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()

		if s.deps.Verbose {
			s.logf(fmt.Sprintf("tools/call %s 开始（id=%s）", name, key))
		}
		result := s.callToolSafely(ctx, name, args)
		s.logf(fmt.Sprintf("tools/call %s 结束（isError=%v）", name, result.IsError))

		cancelled := false
		if keyOK {
			s.mu.Lock()
			if current, ok := s.calls[key]; ok && current == call {
				delete(s.calls, key)
			}
			cancelled = call.cancelled
			s.mu.Unlock()
		}
		if !hasID {
			return
		}
		if cancelled {
			s.logf(fmt.Sprintf("请求 %s 已被取消，按 MCP 规范不再回响应", key))
			return
		}
		s.writeResult(id, result)
	}()
}

func (s *server) handleCancelled(params json.RawMessage) {
	if len(params) == 0 || string(params) == "null" {
		s.logf("取消通知缺少 params，已忽略")
		return
	}
	if !isJSONObject(params) {
		s.logf("取消通知的 params 不是对象，已忽略")
		return
	}
	var p struct {
		RequestID json.RawMessage `json:"requestId"`
		Reason    string          `json:"reason"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		s.logf(fmt.Sprintf("取消通知的 params 不合法: %v", err))
		return
	}
	key, ok := idKey(p.RequestID)
	if !ok {
		s.logf("取消通知没有合法的 requestId，已忽略")
		return
	}

	s.mu.Lock()
	call, found := s.calls[key]
	if found {
		call.cancelled = true
	}
	s.mu.Unlock()

	reason := ""
	if p.Reason != "" {
		reason = "（原因: " + p.Reason + "）"
	}
	if !found {
		s.logf(fmt.Sprintf("取消请求 %s：不在飞（可能已完成）%s", key, reason))
		return
	}
	call.cancel()
	s.logf(fmt.Sprintf("取消请求 %s：已请求中断%s", key, reason))
}

func (s *server) callToolSafely(ctx context.Context, name string, args json.RawMessage) (result toolResult) {
	defer func() {
		if r := recover(); r != nil {
			s.logf(fmt.Sprintf("tools/call %s panic: %v\n%s", name, r, debug.Stack()))
			result = errorResult(fmt.Sprintf("内部错误: %v", r))
		}
	}()
	return s.callTool(ctx, name, args)
}

func (s *server) writeResult(id json.RawMessage, result any) {
	s.writeJSON(response{JSONRPC: jsonrpcVersion, ID: normalizeID(id), Result: result})
}

func (s *server) writeError(id json.RawMessage, code int, message string) {
	s.writeJSON(response{
		JSONRPC: jsonrpcVersion,
		ID:      normalizeID(id),
		Error:   &rpcError{Code: code, Message: message},
	})
}

// writeJSON 是 stdout 的唯一写入口：一次 Encode 一次 Write，任何两行都不会
// 交错。SetEscapeHTML(false) 只是让人在终端里直接看 transcript 时中文与
// 尖括号保持原样，解出来的结构完全相同。
func (s *server) writeJSON(v any) {
	if s.outBroken.Load() {
		return
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		s.logf(fmt.Sprintf("序列化响应失败: %v", err))
		return
	}

	s.writeMu.Lock()
	_, err := s.out.Write(buf.Bytes())
	s.writeMu.Unlock()
	if err != nil {
		s.outBroken.Store(true)
		s.logf(fmt.Sprintf("写 stdout 失败，后续响应不再尝试: %v", err))
	}
}

// logf 只写 stderr。这是「stdout 洁净」那条约束在代码里的唯一落点：
// 任何诊断信息都必须经过它。
func (s *server) logf(message string) {
	fmt.Fprintln(s.err, "council mcp: "+message)
}
