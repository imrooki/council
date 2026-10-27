// Package mcp 实现 `council mcp`：在 stdio 上跑一个最小可用的 MCP 服务器
// （JSON-RPC 2.0，一行一条消息），让 Claude Code 之类的宿主把议会当工具调用。
//
// 这里刻意不引第三方依赖：协议面只有 initialize / ping / tools/list /
// tools/call 与 notifications/initialized、notifications/cancelled，用
// encoding/json 手写比引一套 SDK 更小、也更看得清每一行写出去的字节。
//
// 三条硬约束，改代码前先看：
//  1. stdout 只能有协议消息。任何调试、警告、错误一律走 stderr；横幅、进度
//     在 internal/app 的 MCP 分支里也是整段跳过的。
//  2. 读 stdin 的循环不能被 tools/call 挡住：议会可能跑几十秒，期间必须还能
//     收 notifications/cancelled。
//  3. 工具失败是 isError + 文本，不是 JSON-RPC error。JSON-RPC error 只留给
//     协议错误（解析失败、未知方法、非法 params）。
package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// protocolVersion 是 initialize 回报的 MCP 协议版本。取当前稳定版；
// 宿主按它决定能用哪些字段，写错会让宿主直接断开。
const protocolVersion = "2024-11-05"

const jsonrpcVersion = "2.0"

// JSON-RPC 2.0 的标准错误码。除了 -32700 / -32601 / -32602，另外两个是
// 协议层兜底：请求本身不合法（不是对象、缺 method）与内部 panic。
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// request 是 stdin 上的一行。ID 用 RawMessage 原样保存：JSON-RPC 允许
// number / string / null，回包必须逐字节回同一个 id，不能自己重新编号。
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// response 是 stdout 上的一行。Result 与 Error 二者恰有其一。
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("JSON-RPC %d: %s", e.Code, e.Message)
}

// initializeResult 是 initialize 的 result。capabilities.tools 用空对象
// 表示「支持 tools/list 与 tools/call，但不发 list_changed」。
// instructions 是给宿主模型看的一段话，说明花钱与三个工具的取舍。
func initializeResult(version string) map[string]any {
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "council",
			"version": version,
		},
		"instructions": "Council runs your locally configured AI CLIs (claude / codex / grok / agy) as a panel. " +
			"Call council_models first to see which members exist. " +
			"council_ask asks every named member once and returns their replies; " +
			"council_round runs 1-5 rounds and a summary. " +
			"Every call spends real quota: pick members explicitly when you can, and keep budget_usd (minimum 0.5 USD) in mind.",
	}
}

// idKey 把 JSON-RPC id 折成取消表用的键。
//
// 不直接用原始字节：同一个 id 在两条消息里可能一个带空格、一个不带，
// 数字也可能写成 1 与 1.0。这里按解码后的类型与值归一，字符串与数字分表
// （"1" 与 1 是两个不同的 id）。
//
// 返回 false 表示这不是一个合法的请求 id（缺省或 null）：那样的消息是
// notification，不回包，也不进取消表。
func idKey(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var value any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return "", false
	}
	switch v := value.(type) {
	case string:
		return "s:" + v, true
	case json.Number:
		return "n:" + v.String(), true
	default:
		return "", false
	}
}

// normalizeID 给错误回包兜一个 id: null。request.ID 为 nil 时
// json.Marshal 会把 nil 的 RawMessage 写成 null，这里显式化只是让意图
// 一眼可见，不改变字节。
func normalizeID(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("null")
	}
	return raw
}

// isJSONObject 判断一段原始 JSON 是不是对象。params / arguments 都要求
// 对象；数组、字符串、null 一律按非法参数处理。
func isJSONObject(raw json.RawMessage) bool {
	for _, b := range raw {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		case '{':
			return true
		default:
			return false
		}
	}
	return false
}
