package serve

// write.go 是 SRV4 的写模式：`council serve -rw` 的库装配、任务表、
// POST /api/ask 与 SSE。
//
// 这里刻意不碰轮次、预算与汇总：那些全部交给 internal/headless.Run，
// 本文件只做三件事——把 HTTP 请求翻译成 headless.Options、把 OnStart /
// OnRecord 回调搬进内存事件表、按 SSE 协议写给浏览器。任何一处开始自己
// 计算「该问谁」「花多少钱」「要不要汇总」，都意味着 serve 与 `council ask`
// 走上了两条会各自漂移的路。

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/headless"
	"council/internal/repl"
	"council/internal/store"
	"council/internal/turn"
)

// defaultJobTTL 是任务结束后事件表保留的时长。任务书 §3 定的是 60 秒：
// 浏览器连上得晚一点、或者断线重连一次，仍然拿得到完整历史；更久则没有
// 意义——job id 只活在一个页面会话里，60 秒足够覆盖 EventSource 的重连退避。
const defaultJobTTL = 60 * time.Second

// 关闭服务时留给在跑议会的两段时间。先等 10 秒自然收尾；到点还没完就
// 显式取消任务（jobCtx），再给 2 秒让它把取消写进库。两段都超时仍然返回，
// 但这时是「明确取消」而不是「关库之后悄悄丢掉」。
//
// 用变量而不是常量，是为了让测试能把它压到毫秒级验证同一条路径；
// 生产值就是任务书复审要求的 10s / 2s。
var (
	jobDrainTimeout = 10 * time.Second
	jobDrainGrace   = 2 * time.Second
)

// maxAskBody 是 POST /api/ask 请求体的上限。问题正文是普通文本，1 MiB
// 已经远超任何正常提问；设上限是为了不让人把一个流式请求挂在这里。
const maxAskBody = 1 << 20

// WriteDeps 是写模式需要的全部装配结果。它由 serve.Run 在拿到排他锁、
// 加载配置与规则之后填，与 internal/app/app.go 的装配逐项对应
// （docs/21d_SRV4_M2_TASK.md §2）。
type WriteDeps struct {
	Cfg     *config.Config
	Svc     *conversation.Service
	Rn      *turn.Runner
	DataDir string
	// ProjectDir 与 app.Run 里那个同义：从服务进程的启动目录规范化而来，
	// 新会话据此归属项目，CLI 的 /resume 才能按同一个项目把它们列出来。
	ProjectDir string
}

// EnableWrite 把 Server 切到写模式。调用方必须先打开可写库、装配好
// Service 与 Runner；本方法只保存引用并建任务表，不做任何装配工作，
// 因为「怎么装配」属于 serve.Run 的职责，不属于 HTTP handler。
func (s *Server) EnableWrite(deps WriteDeps) {
	s.write = &deps
	s.jobs = newJobTable()
	// 后台议会的父 ctx 跟着 Server 的生命周期，不跟任何一条 HTTP 请求。
	s.jobCtx, s.cancelJobs = context.WithCancel(context.Background())
}

// drainJobs 等所有后台议会结束，供 listenAndServe 在 HTTP Shutdown 之后
// 调用。ctx 的期限就是「自然等待」的上限（生产 10 秒）：
//   - 期限前全部结束 → 取消 jobCtx 释放资源，返回 nil；
//   - 到期还没完 → 显式 cancelJobs()，再给 jobDrainGrace（生产 2 秒）让
//     取消写进库；这段宽限也过了才返回错误。
//
// 这样关闭时最坏的结果是「明确取消一轮」，而不是「关库之后那一轮静默
// 消失在数据库错误里」。
func (s *Server) drainJobs(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.jobsWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		if s.cancelJobs != nil {
			s.cancelJobs()
		}
		return nil
	case <-ctx.Done():
		if s.cancelJobs != nil {
			s.cancelJobs()
		}
		grace := time.NewTimer(jobDrainGrace)
		defer grace.Stop()
		select {
		case <-done:
			return nil
		case <-grace.C:
			return fmt.Errorf("等待运行中的议会结束超时（已取消）")
		}
	}
}

// runWritable 是 `council serve -rw` 的进程前半段：加载完整配置与规则 ->
// store.Open（拿 council.lock、跑迁移）-> 照 app.Run 装配 Service/Runner ->
// 交给与只读模式共用的 HTTP 生命周期。
func runWritable(ctx context.Context, opts Options, version string, out, errOut io.Writer) error {
	cfg, rules, dataDir, err := loadWriteConfig(opts.ConfigPath)
	if err != nil {
		return err
	}

	st, err := store.Open(ctx, dataDir)
	if err != nil {
		// 任务书 §2 要求错误里出现这两句话：一句说事实（谁占着目录），
		// 一句说下一步（关掉再试）。store.ErrDataDirLocked 自己的英文
		// 文本也保留在 %w 里，排查时还能看到锁文件路径与系统错误。
		if errors.Is(err, store.ErrDataDirLocked) {
			return fmt.Errorf("另一个 council 正在用这个 data_dir（%s）；关掉它再试: %w", dataDir, err)
		}
		return fmt.Errorf("打开本地库失败 (%s): %w", dataDir, err)
	}
	// 关闭会同时释放 council.lock。失败必须说出来：锁没放干净，下一次
	// 启动会被自己上一次的进程挡住，而错误现场已经离开了。
	defer func() {
		if cerr := st.Close(); cerr != nil {
			fmt.Fprintf(errOut, "council serve: 关闭本地库失败: %v\n", cerr)
		}
	}()

	// 以下装配与 internal/app/app.go 的 write 路径同源。thought 策略必须
	// 接上，否则 store_thought=false 的配置在网页里跑议会时会静默把
	// thought 存进库——同一个配置在 CLI 与网页下产生不同的事实源。
	svc := conversation.NewService(st)
	svc.SetThoughtPolicy(cfg.StoreThought)
	rn := turn.NewRunner(cfg, svc, filepath.Join(dataDir, "workdir"), turn.RulesBlocks{
		Global: rules.GlobalText,
		Local:  rules.LocalText,
	})

	// 启动对账不能省：上一次进程崩在一轮中间时，inflight_turns 里会留着
	// 一行，Runner 的在飞闸会把此后所有同 (会话, 成员) 的 ask 都拒掉
	// （conversation.ErrInflightConflict）。TUI 靠 app.Run 的启动对账清掉
	// 这些行，serve -rw 既然接管了同一个库，就必须做同一件事。
	// 失败只提示、不阻止启动：对账本身读不了库时，看板仍值得开着。
	reconcileLostTurns(ctx, rn, errOut)

	// 启动目录决定新会话的项目归属，与 CLI 用同一个 NormalizeProjectDir，
	// 网页里建的会话才能被同一个项目下的 /resume 看到。
	projectDir := ""
	if wd, wdErr := os.Getwd(); wdErr == nil {
		projectDir = repl.NormalizeProjectDir(wd)
	}

	srv := NewServer(st, version, modelsFromConfig(cfg))
	srv.Verbose = opts.Verbose
	srv.EnableWrite(WriteDeps{
		Cfg:        cfg,
		Svc:        svc,
		Rn:         rn,
		DataDir:    dataDir,
		ProjectDir: projectDir,
	})
	// drainJobs 必须在 st.Close() 之前跑完：runWritable 的 defer 在
	// listenAndServe 返回后立刻关库放锁，而这时后台议会可能还在写。
	return listenAndServe(ctx, opts.Addr, srv.Handler(), opts.Open, out, errOut, srv.drainJobs)
}

// reconcileLostTurns 照 internal/app/app.go 的启动对账做一遍，只保留
// AuditInflight / SettleLostTurns 这一对：它们清掉的是会挡住后续 ask 的
// 陈旧在飞行，属于「写模式能不能正常跑」的一部分。
// 漂移审计只是给人看的提醒，不在这里重复一遍。
func reconcileLostTurns(ctx context.Context, rn *turn.Runner, errOut io.Writer) {
	lost, err := rn.AuditInflight(ctx)
	if err != nil {
		fmt.Fprintf(errOut, "council serve: 启动在飞对账失败: %v\n", err)
		return
	}
	if len(lost) == 0 {
		return
	}
	marked, errs := rn.SettleLostTurns(ctx, lost)
	for _, settleErr := range errs {
		fmt.Fprintf(errOut, "council serve: 启动在飞对账补账失败: %v\n", settleErr)
	}
	fmt.Fprintf(errOut, "council serve: 有 %d 轮在上次退出时没记上账，其中 %d 轮已把相关成员的成本基线标为可疑\n",
		len(lost), marked)
}

// loadWriteConfig 是写模式与只读模式的 resolveDataDir 分开的一份：写模式
// 需要完整配置（agents / providers / summary）与规则文件，缺一个都跑不了
// 议会，所以配置缺失必须直接报错，不能像只读模式那样回落到一个空
// models 表。错误文本沿用 app.Run 的措辞，用户在两处看到的是同一句话。
func loadWriteConfig(configPath string) (*config.Config, config.Rules, string, error) {
	path, err := config.ResolveConfigPath(configPath)
	if err != nil {
		return nil, config.Rules{}, "", fmt.Errorf("无法确定配置文件位置: %w", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return nil, config.Rules{}, "", fmt.Errorf(
				"配置文件不存在: %s（council serve -rw 需要完整配置：data_dir、agent 与 provider 命令）", path)
		}
		return nil, config.Rules{}, "", fmt.Errorf("查看配置文件 %s: %w", path, statErr)
	}
	cfg, err := config.LoadAndValidate(path)
	if err != nil {
		return nil, config.Rules{}, "", fmt.Errorf("加载配置失败 (%s): %w", path, err)
	}
	rules, err := config.LoadRules(path, cfg.RulesFile)
	if err != nil {
		return nil, config.Rules{}, "", fmt.Errorf("加载规则文件失败: %w", err)
	}
	dataDir, err := config.ResolveDataDir(cfg.DataDir)
	if err != nil {
		return nil, config.Rules{}, "", fmt.Errorf("解析数据目录失败: %w", err)
	}
	return cfg, rules, dataDir, nil
}

// modelsFromConfig 给只读端点用的 agent id -> model 映射；写模式与只读
// 模式共用 NewServer，这个映射的口径必须一致。
func modelsFromConfig(cfg *config.Config) map[string]string {
	if cfg == nil {
		return nil
	}
	models := make(map[string]string, len(cfg.Agents))
	for id, ac := range cfg.Agents {
		models[id] = ac.Model
	}
	return models
}

// ---- 任务表 ----

// jobEvent 是 SSE 上的一帧。Name 就是 event: 后面的名字，Data 会被编码成
// 一行的 JSON。
type jobEvent struct {
	Name string
	Data any
}

// job 是一次 /api/ask 的内存状态。
//
// 为什么保留完整历史而不是只开一个 channel：POST 返回 202 与浏览器建立
// EventSource 之间有真实的时间差（React/V8 都要先跑完当前任务），
// started 完全可能先于订阅发生；断线重连时浏览器的 Last-Event-ID 也要求
// 能补发。channel 里没人收的事件会永远丢掉，所以这里用切片做历史，
// changed 通道只承担「有新事件了」的广播。这样做仍然不会让回调阻塞在
// 网络上：emit 只拿一小会儿锁、追加一个值就返回。
type job struct {
	id string

	mu       sync.Mutex
	events   []jobEvent
	changed  chan struct{}
	finished bool
	// result / err 只给测试与清理逻辑读；生产的 done 数据直接来自
	// headless 写出的 JSON，不从这里再拼一遍，避免两处口径。
	result headless.Result
	err    error
	// done 在任务结束（无论成功失败）时关闭，测试用它观察「客户端断开
	// 之后议会仍然跑完」，不必轮询库或硬等。
	done chan struct{}
	// finishOnce 让 finish 幂等：正常收尾与 panic 兜底可能都想结束同一个
	// job，重复 close(done) 会 panic，而兜底逻辑自己不能再把进程带崩。
	finishOnce sync.Once
}

func newJob(id string) *job {
	return &job{
		id:      id,
		changed: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// emit 追加一帧并唤醒等待者。关闭旧 changed、换一个新的，是 Go 里
// 一对多广播的惯用写法：等待者拿到的通道一定会被关闭，不会漏掉唤醒。
func (j *job) emit(name string, data any) {
	j.mu.Lock()
	j.events = append(j.events, jobEvent{Name: name, Data: data})
	close(j.changed)
	j.changed = make(chan struct{})
	j.mu.Unlock()
}

// finish 标记任务结束。done 的关闭放在锁外：等待者被唤醒后要能立刻看到
// result / err，锁内关闭与「先写字段再 close」的顺序由互斥锁保证。
func (j *job) finish(result headless.Result, err error) {
	j.finishOnce.Do(func() {
		j.mu.Lock()
		j.result = result
		j.err = err
		j.finished = true
		close(j.changed)
		j.changed = make(chan struct{})
		j.mu.Unlock()
		close(j.done)
	})
}

// isFinished 给 panic 兜底用：如果 panic 发生在正常 finish 之后，兜底
// 不能再往同一个 job 里补一对 error/done，否则前端会收到互相矛盾的两份结局。
func (j *job) isFinished() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.finished
}

// next 原子地取出第 i 帧：没有新帧时返回的 wait 与 finished 是同一把锁
// 下的快照，SSE handler 不会在「查完没事件、还没开始等」的缝隙里丢掉一帧。
func (j *job) next(i int) (ev jobEvent, ok, finished bool, wait <-chan struct{}) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if i < len(j.events) {
		return j.events[i], true, j.finished, nil
	}
	return jobEvent{}, false, j.finished, j.changed
}

func (j *job) outcome() (headless.Result, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.result, j.err
}

// jobTable 是进程内的任务表。job id 只活在这一个进程里，没有持久化：
// 服务重启后旧 id 一律 404，这是正确的——那些议会进程也已经不在了。
type jobTable struct {
	mu   sync.Mutex
	jobs map[string]*job
}

func newJobTable() *jobTable {
	return &jobTable{jobs: make(map[string]*job)}
}

func (t *jobTable) add(j *job) {
	t.mu.Lock()
	t.jobs[j.id] = j
	t.mu.Unlock()
}

func (t *jobTable) get(id string) (*job, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	j, ok := t.jobs[id]
	return j, ok
}

func (t *jobTable) remove(id string) {
	t.mu.Lock()
	delete(t.jobs, id)
	t.mu.Unlock()
}

// newJobID 铸一个 64 位十六进制 id。用 crypto/rand 而不是时间/自增：
// 端口对本机任何进程开放，可猜的 id 等于把别人的议会进度与正文送给
// 同一台机器上的任何程序。它不做鉴权，但也不主动送路径。
func newJobID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("生成任务 id: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

// ---- POST /api/ask ----

// askRequest 是 POST /api/ask 的请求体。指针字段用来区分「没给」与
// 「给了零值」：rounds 缺席按 1 轮，给了 0 是纯用法错误，不能静默当成 1。
type askRequest struct {
	Prompt         string   `json:"prompt"`
	Members        []string `json:"members"`
	Rounds         *int     `json:"rounds"`
	Summary        string   `json:"summary"`
	BudgetUSD      *float64 `json:"budget_usd"`
	TimeoutSeconds *float64 `json:"timeout_seconds"`
}

type askResponse struct {
	JobID string `json:"job_id"`
}

type startedEvent struct {
	ConversationID string `json:"conversation_id"`
}

type turnEvent struct {
	AgentID string  `json:"agent_id"`
	Outcome string  `json:"outcome"`
	CostUSD float64 `json:"cost_usd"`
	Text    string  `json:"text"`
}

type doneEvent struct {
	Status  string  `json:"status"`
	Summary string  `json:"summary"`
	CostUSD float64 `json:"cost_usd"`
	Failed  int     `json:"failed"`
	Total   int     `json:"total"`
}

type errorEvent struct {
	Message string `json:"message"`
}

// writeGuardError 让 checkWriteRequest 能同时带回 HTTP 状态码与中文说明。
// 415 与 403 是两类完全不同的拒绝：前者是请求格式不对，后者是浏览器
// 跨站页面在借用户的端口花钱；状态码不能都写成 400。
type writeGuardError struct {
	status  int
	message string
}

func (e *writeGuardError) Error() string { return e.message }

// checkWriteRequest 是写端点专用的两道闸，读端点一条都不加：
//
//  1. Content-Type 必须解析成 application/json。浏览器跨站「简单请求」
//     只能发 text/plain、application/x-www-form-urlencoded 或 multipart，
//     先把它们挡在门外，恶意网页就无法在不触发 CORS 预检的情况下提交。
//  2. Origin 存在时，host 必须与本次请求的 Host 一致；Sec-Fetch-Site 存在
//     时只放行 same-origin / none。没有这两个头的请求（curl、脚本、dsh
//     工具调用）放行——它们不是浏览器跨站，挡了反而误伤本机自动化。
//
// 两道同时上而不是只挑一道：Content-Type 挡的是浏览器不触发预检就能
// 发出的「简单请求」；Origin / Sec-Fetch-Site 是在请求真的到达时再做一次
// 同源确认。漏掉任何一道，都会留下一类浏览器能发起的写请求。
func checkWriteRequest(r *http.Request) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return &writeGuardError{
			status:  http.StatusUnsupportedMediaType,
			message: "写接口要求 Content-Type: application/json",
		}
	}

	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		u, parseErr := url.Parse(origin)
		if parseErr != nil || u.Host == "" || !strings.EqualFold(u.Host, r.Host) {
			return &writeGuardError{
				status:  http.StatusForbidden,
				message: "拒绝跨站写请求：Origin 与本次请求的 Host 不一致",
			}
		}
	}

	switch strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))) {
	case "", "same-origin", "none":
	default:
		return &writeGuardError{
			status:  http.StatusForbidden,
			message: "拒绝跨站写请求：Sec-Fetch-Site 不是 same-origin",
		}
	}
	return nil
}

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	if s.write == nil {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed,
			"只读模式没有写接口：用 council serve -rw 启动才能跑议会")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "只支持 POST")
		return
	}
	if err := checkWriteRequest(r); err != nil {
		status := http.StatusForbidden
		var guardErr *writeGuardError
		if errors.As(err, &guardErr) {
			status = guardErr.status
		}
		writeError(w, status, err.Error())
		return
	}

	var req askRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAskBody))
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法 JSON: "+err.Error())
		return
	}
	// 只认一个对象：Json 后面再跟一段东西时 Decode 会停在第一个值上，
	// 静默忽略后半段会让调用方以为自己的第二段参数生效了。
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "请求体只能有一个 JSON 对象")
		return
	}

	opts, err := optionsFromAsk(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// 先校验再建任务：未知成员、非法预算、round 模式缺汇总，这些在
	// headless.Run 里也是同一套判据，但在这里挡住可以给调用方一个
	// 干净的 400，不必开一个必然失败的后台任务再让 SSE 去报 error。
	if err := headless.Validate(opts, s.write.Cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := newJobID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	j := newJob(id)
	s.jobs.add(j)
	// 议会挂 s.jobCtx，不挂请求的 ctx：浏览器关掉页面时 r.Context() 会
	// 取消，但已经花出去的钱与正在跑的成员不该跟着作废。s.jobCtx 只在
	// 进程关闭且等满 10 秒仍未收尾时被 cancelJobs 显式取消。
	//
	// Add 必须在 go 之前：关闭流程的 Wait 看到 0 时就会去关库，
	// 那一瞬间才 Add 的任务会立刻撞上已经关掉的 Service/Runner。
	s.jobsWG.Add(1)
	go s.runAskJob(j, opts)
	writeJSON(w, http.StatusAccepted, askResponse{JobID: id})
}

// optionsFromAsk 把 HTTP 请求体翻译成 headless.Options。默认值与
// `council ask` 的 flag 默认值一致（rounds=1、budget=5、timeout=10m），
// 这样网页与命令行对同一句问题的收费上限与超时口径相同。
func optionsFromAsk(req askRequest) (headless.Options, error) {
	rounds := 1
	if req.Rounds != nil {
		rounds = *req.Rounds
	}
	mode := "ask"
	if rounds > 1 {
		// rounds > 1 与 CLI 的 -mode round 是同义词：那边的多轮必须有人
		// 收口，所以这里也走 round，让 headless 的汇总规则原样生效。
		mode = "round"
	}

	budget := 5.0
	if req.BudgetUSD != nil {
		budget = *req.BudgetUSD
	}
	timeout := 10 * time.Minute
	if req.TimeoutSeconds != nil {
		// time.Duration 的上限远大于任何合理会话；这里不做额外上限，
		// 只挡住「非正数」与「乘出来溢出成非正数」两种非法值。
		timeout = time.Duration(*req.TimeoutSeconds * float64(time.Second))
		if timeout <= 0 {
			return headless.Options{}, fmt.Errorf("timeout_seconds 必须大于 0")
		}
	}

	members := make([]string, 0, len(req.Members))
	for _, raw := range req.Members {
		if id := strings.TrimSpace(raw); id != "" {
			members = append(members, id)
		}
	}

	return headless.Options{
		Prompt:    strings.TrimSpace(req.Prompt),
		Members:   members,
		Mode:      mode,
		Rounds:    rounds,
		Summary:   strings.TrimSpace(req.Summary),
		BudgetUSD: budget,
		Timeout:   timeout,
		JSON:      true,
	}, nil
}

// runAskJob 在后台跑一次议会。轮次、预算、汇总全部由 headless.Run 决定；
// 本函数只把它的三个观察点（开始、每条记录、最终 JSON）搬成 SSE 事件。
func (s *Server) runAskJob(j *job, opts headless.Options) {
	// defer 顺序（LIFO）：scheduleJobCleanup 最后注册、最先执行；
	// recover 其次；jobsWG.Done 最先注册、最后执行。清理排在 recover
	// 之前，是为了让 recover 也能接住清理自身可能出现的 panic；而关闭
	// 流程以 jobsWG 归零作为「可以关库」的信号，所以它必须最后发生。
	defer s.jobsWG.Done()
	defer func() {
		if rec := recover(); rec != nil {
			// panic 不能让整个 serve 进程退出：别的会话、只读端点、
			// 已经排队的任务都还活着。把这一轮明确结束掉，前端拿到
			// error + done 就能解锁，库与锁由正常关闭流程负责。
			if j.isFinished() {
				return
			}
			msg := fmt.Sprintf("议会执行时发生 panic: %v", rec)
			j.emit("error", errorEvent{Message: msg})
			j.emit("done", doneEvent{Status: "error"})
			j.finish(headless.Result{Status: "error"}, errors.New(msg))
		}
	}()
	defer s.scheduleJobCleanup(j.id)

	opts.JSON = true
	var buf bytes.Buffer

	deps := headless.Deps{
		Cfg:        s.write.Cfg,
		Svc:        s.write.Svc,
		Rn:         s.write.Rn,
		DataDir:    s.write.DataDir,
		ProjectDir: s.write.ProjectDir,
		// headless 的 stdout 只能是那一个 JSON 结果：这里的 buffer 就是
		// 捕获它的地方，解析出来的 Result 直接变成 done 的字段。
		Out: &buf,
		// 给人看的 stderr 在网页里没有去处；真正的失败会经 error 事件
		// 交给浏览器，不再往服务进程的终端重复一份。
		Err: io.Discard,
		OnStart: func(convID string) {
			j.emit("started", startedEvent{ConversationID: convID})
		},
		OnRecord: func(rec turn.Record) {
			j.emit("turn", turnEvent{
				AgentID: rec.AgentID,
				Outcome: eventOutcomeName(rec.Outcome),
				CostUSD: eventCost(rec),
				Text:    eventText(rec),
			})
		},
	}

	runCtx := s.jobCtx
	if runCtx == nil {
		// EnableWrite 一定会建 jobCtx；这里是给直接构造 Server 的测试
		// 留的兜底，免得一个装配疏漏变成 nil ctx 的 panic。
		runCtx = context.Background()
	}
	runErr := headless.Run(runCtx, deps, opts)

	res, ok := decodeResult(buf.Bytes())
	if !ok {
		// headless 在写 Result 之前就失败（配置为空、装配不完整——本路径
		// 上不会发生，但错误处理不能假设）。仍然发一条 done，让前端一定
		// 能解锁按钮；否则一次早期失败会把输入框永久禁在「运行中」。
		msg := "议会执行失败"
		if runErr != nil {
			msg = runErr.Error()
		}
		j.emit("error", errorEvent{Message: msg})
		j.emit("done", doneEvent{Status: "error"})
		j.finish(headless.Result{Status: "error"}, runErr)
		return
	}

	// status=error 是执行失败，除了 done 里的状态之外再给一条可读的
	// 原因；budget_exceeded / partial 是业务结果，不额外报 error。
	if runErr != nil && res.Status == "error" {
		j.emit("error", errorEvent{Message: runErr.Error()})
	}
	j.emit("done", doneEvent{
		Status:  res.Status,
		Summary: res.Summary,
		CostUSD: res.CostUSD,
		Failed:  res.Failed,
		Total:   res.Total,
	})
	j.finish(res, runErr)
}

// decodeResult 解 headless 写出的那个 JSON 对象。空 buffer 不是错误，
// 只表示 Run 在写结果之前就返回了；调用方据此走 error 分支。
func decodeResult(raw []byte) (headless.Result, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return headless.Result{}, false
	}
	var res headless.Result
	if err := json.Unmarshal(raw, &res); err != nil {
		return headless.Result{}, false
	}
	return res, true
}

// scheduleJobCleanup 在任务结束后按 jobTTL 从内存表删掉它。用
// time.AfterFunc 而不是让 SSE handler 自己延时：没有浏览器连着的任务
// 同样要被清掉，清理不能依赖读者存在。
func (s *Server) scheduleJobCleanup(id string) {
	ttl := s.jobTTL
	if ttl <= 0 {
		ttl = defaultJobTTL
	}
	time.AfterFunc(ttl, func() {
		if s.jobs != nil {
			s.jobs.remove(id)
		}
	})
}

// eventOutcomeName 与 headless 的 outcomeName 逐字一致。两份都从
// turn.Outcome 的同一组常量派生；差别只在包边界——headless 不导出这个
// 纯映射，而 SSE 需要实时给，不能等整趟结束从 Result 里回头取。
func eventOutcomeName(o turn.Outcome) string {
	switch o {
	case turn.OutcomeSpoke:
		return "ok"
	case turn.OutcomePostRejected:
		return "post_rejected"
	case turn.OutcomeProviderFailed:
		return "provider_failed"
	case turn.OutcomeUnparsable:
		return "unparsable"
	case turn.OutcomeTimeout:
		return "timeout"
	case turn.OutcomeSpawnFailed:
		return "spawn_failed"
	case turn.OutcomeRefused:
		return "refused"
	case turn.OutcomeCanceled:
		return "canceled"
	case turn.OutcomeLost:
		return "lost"
	default:
		return "unknown"
	}
}

// eventCost 与 headless.costOf 同口径：没报金额的一轮是 0，不是负数、
// 也不是「免费」的断言——总额可不可信由 done.status 与 CLI 共用的
// Result.cost_complete 决定，SSE 不另造一套。
func eventCost(rec turn.Record) float64 {
	if rec.CostUSDDelta == nil {
		return 0
	}
	return *rec.CostUSDDelta
}

// eventText 取这一轮真正付过钱拿到的那段正文：正常发言在 Message 里，
// post_rejected 的正文在 DroppedText 里，不能让它在网页上消失。
func eventText(rec turn.Record) string {
	if rec.Message != nil {
		return rec.Message.Text
	}
	return rec.DroppedText
}

// ---- GET /api/jobs/{id}/events ----

func (s *Server) handleJobPath(w http.ResponseWriter, r *http.Request) {
	if s.write == nil {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed,
			"只读模式没有任务接口：用 council serve -rw 启动才能跑议会")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "只支持 GET")
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	segments := strings.Split(rest, "/")
	if len(segments) != 2 || segments[0] == "" || segments[1] != "events" {
		writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
		return
	}
	id := segments[0]
	j, ok := s.jobs.get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "任务不存在: "+id)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "当前 HTTP 连接不支持流式响应")
		return
	}

	// Last-Event-ID 由浏览器在自动重连时带回来。从它开始续传，断线重连
	// 就不会把已经显示的 turn 再打一遍；首次连接没有这个头，从头放。
	start := 0
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			start = n
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		ev, hasEvent, finished, wait := j.next(start)
		if hasEvent {
			start++
			if err := writeSSEEvent(w, start, ev); err != nil {
				return
			}
			flusher.Flush()
			continue
		}
		if finished {
			return
		}
		select {
		case <-wait:
		case <-r.Context().Done():
			// 只取消订阅。任务在 s.runAskJob 的 goroutine 里继续跑，
			// 已经花掉的钱必须换回结果（任务书 §3）。
			return
		}
	}
}

// writeSSEEvent 按 SSE 线格式写一帧：id 用于断线续传，event 是事件名，
// data 是一行 JSON（json.Marshal 会把正文里的换行转义掉，不会破坏分帧）。
func writeSSEEvent(w io.Writer, id int, ev jobEvent) error {
	data, err := json.Marshal(ev.Data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", id, ev.Name, data)
	return err
}
