package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// discovery_parse.go 是 A3 的下半。W1b 修订把它从 discovery.go 拆出来，只为交付时
// 每块在 400 行上下，代码没有改。本文件是 DiscoverModels、ParseDiscovery、各家的
// 解析器与 hasControlChar（codex 的解析器由 CXH 加）。
//
// 它用 discovery.go 的类型、哨兵、DiscoveryCommandFor、claudeDiscoveryResponse
// 与 claudeDiscoveryRequestID（完整清单见 A3 抬头）。discovery.go 不用本文件的
// 任何东西。与 discovery.go 同一条约定：本包不起进程，本文件也不 import os/exec、os。

// DiscoverModels 跑一家的识别命令并解析。
//
// 五个哨兵声明在 discovery.go。映射是：
// ErrDiscoveryTimeout：执行器的错误链里有 context.DeadlineExceeded 时返回它，执行器的错误文本用 %v 接在后面。
// ErrDiscoveryFailed：其它执行器错误返回它。哨兵用 %w 包进错误链，执行器的错误文本用 %v 接在后面，不 %w。
// ErrDiscoveryUnparseable：解析不出已知形状。ParseDiscovery 返回它，本函数原样上传。
// ErrDiscoveryEmpty：形状对，但一行可选的都没有（多半是没登录）。ParseDiscovery 返回它，本函数原样上传。
// ErrDiscoveryUnsupported：这家 provider 没有识别方式。DiscoveryCommandFor 失败时立刻返回，执行器不会被调用。
func DiscoverModels(ctx context.Context, providerID, path string, exec DiscoveryExec) ([]DiscoveredModel, error) {
	cmd, err := DiscoveryCommandFor(providerID, path)
	if err != nil {
		return nil, err
	}
	out, err := exec(ctx, cmd)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %v", ErrDiscoveryTimeout, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrDiscoveryFailed, err)
	}
	return ParseDiscovery(providerID, out)
}

// ParseDiscovery 解析一家识别命令的 stdout。
func ParseDiscovery(providerID string, stdout []byte) ([]DiscoveredModel, error) {
	switch providerID {
	case "claude":
		return parseClaudeDiscovery(stdout)
	case "agy":
		return parseAgyDiscovery(stdout)
	case "grok":
		return parseGrokDiscovery(stdout)
	case "codex":
		return parseCodexDiscovery(stdout)
	default:
		return nil, fmt.Errorf("provider %q: %w", providerID, ErrDiscoveryUnsupported)
	}
}

// parseClaudeDiscovery 取第一份 request_id 为 council-models 的 control_response。
//
// Model 取 resolvedModel，不取 value。value 多是别名（sonnet、opus[1m]），指向哪个
// 模型由 CLI 与服务端决定，会漂。写别名的 agent 在别名漂走之后换了脑子，配置一个字
// 没变，身份指纹（含 model 字段）不动，旧 provider 会话照样续上，sender_model 记的
// 是别名。写 resolvedModel 把模型钉死：换模型只能新建 id（#45「换模型只新建 id」），
// 指纹跟着变。它与输出里 modelUsage 的键是同一种写法（claude-fable-5-1），
// ObservedModel 与配置可以直接对上。resolvedModel 为空时退回 value。
//
// value 为 default 的行不单列，它只是「当前默认是谁」的指针（#45）。disabled 为真的
// 行不给选；字段名 disabled 未实测（§0 第 5 条）。model 为空，或 hasControlChar 为真，
// 就跳过。subtype 不是 success，或内层 response 为空，返回 ErrDiscoveryUnparseable。
// 一行可选的都没有则返回 ErrDiscoveryEmpty；一份匹配的回应都没有同样算认不出。
func parseClaudeDiscovery(stdout []byte) ([]DiscoveredModel, error) {
	for _, line := range bytes.Split(stdout, []byte("\n")) {
		v, ok := claudeDiscoveryResponse(line)
		if !ok {
			continue
		}
		if v.Response.Subtype != "success" || v.Response.Response == nil {
			return nil, fmt.Errorf("claude: control_response subtype %q: %w", v.Response.Subtype, ErrDiscoveryUnparseable)
		}
		var out []DiscoveredModel
		for _, o := range v.Response.Response.Models {
			if o.Value == "default" || o.Disabled {
				continue
			}
			model := o.ResolvedModel
			if model == "" {
				model = o.Value
			}
			if model == "" || hasControlChar(model) {
				continue
			}
			out = append(out, DiscoveredModel{
				Provider: "claude",
				Model:    model,
				Alias:    o.Value,
				Name:     o.DisplayName,
				Efforts:  o.SupportedEffortLevels,
				Source:   ModelSourceSubscription,
			})
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("claude: %w", ErrDiscoveryEmpty)
		}
		return out, nil
	}
	return nil, fmt.Errorf("claude: no control_response for request %s: %w", claudeDiscoveryRequestID, ErrDiscoveryUnparseable)
}

// parseAgyDiscovery 只读 command.data.models 的 id 与 label。status 必须是 SUCCESS，
// command.name 必须是 models，否则返回 ErrDiscoveryUnparseable。label 就是要写进
// 配置 model 字段的值（#45）。不走 ParseTurn：这份 stdout 的 conversation_id 恒为
// 空串，ParseTurn 会报 ErrNoSessionID。label 为空或 hasControlChar 为真就跳过。
// label 以 "Gemini " 开头来源记订阅，其余来源记走 agy 额度。
func parseAgyDiscovery(stdout []byte) ([]DiscoveredModel, error) {
	var v struct {
		Status  string `json:"status"`
		Command *struct {
			Name string `json:"name"`
			Data struct {
				Models []struct {
					ID    string `json:"id"`
					Label string `json:"label"`
				} `json:"models"`
			} `json:"data"`
		} `json:"command"`
	}
	if err := json.Unmarshal(stdout, &v); err != nil {
		return nil, fmt.Errorf("agy: stdout is not JSON: %w", ErrDiscoveryUnparseable)
	}
	if v.Status != "SUCCESS" || v.Command == nil || v.Command.Name != "models" {
		return nil, fmt.Errorf("agy: status %q: %w", v.Status, ErrDiscoveryUnparseable)
	}
	var out []DiscoveredModel
	for _, m := range v.Command.Data.Models {
		if m.Label == "" || hasControlChar(m.Label) {
			continue
		}
		src := ModelSourceAgyQuota
		if strings.HasPrefix(m.Label, "Gemini ") {
			src = ModelSourceSubscription
		}
		out = append(out, DiscoveredModel{
			Provider: "agy",
			Model:    m.Label,
			Alias:    m.ID,
			Name:     m.Label,
			Source:   src,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("agy: %w", ErrDiscoveryEmpty)
	}
	return out, nil
}

// parseGrokDiscovery 只认两个空格开头的 "  * <名字>" 与 "  - <名字>"。行尾的 \r
// 先剥掉。星号那一行末尾的 " (default)" 去掉。名字为空、含空格或制表符、或
// hasControlChar 为真，就跳过。一行都没有时：输出里有 "Available models:" 返回
// ErrDiscoveryEmpty（查不到），否则返回 ErrDiscoveryUnparseable（认不出）。
func parseGrokDiscovery(stdout []byte) ([]DiscoveredModel, error) {
	var out []DiscoveredModel
	for _, line := range strings.Split(string(stdout), "\n") {
		line = strings.TrimRight(line, "\r")
		var rest string
		switch {
		case strings.HasPrefix(line, "  * "):
			rest = line[4:]
		case strings.HasPrefix(line, "  - "):
			rest = line[4:]
		default:
			continue
		}
		name := strings.TrimSuffix(rest, " (default)")
		if name == "" || strings.ContainsAny(name, " \t") || hasControlChar(name) {
			continue
		}
		out = append(out, DiscoveredModel{
			Provider: "grok",
			Model:    name,
			Name:     name,
			Source:   ModelSourceLocalConfig,
		})
	}
	if len(out) == 0 {
		if bytes.Contains(stdout, []byte("Available models:")) {
			return nil, fmt.Errorf("grok: %w", ErrDiscoveryEmpty)
		}
		return nil, fmt.Errorf("grok: no model list: %w", ErrDiscoveryUnparseable)
	}
	return out, nil
}

// codexCustomSlugPrefix 是用户在自己的 codex 配置里自定义转发的模型的 slug 前缀。
const codexCustomSlugPrefix = "custom/"

// codexCatalogModel 是 codex debug models 输出里 models 数组的一行，只读四个键；其余的键（含很大的 model_messages）一律不解。
type codexCatalogModel struct {
	Slug                     string `json:"slug"`
	DisplayName              string `json:"display_name"`
	Visibility               string `json:"visibility"`
	SupportedReasoningLevels []struct {
		Effort string `json:"effort"`
	} `json:"supported_reasoning_levels"`
}

// parseCodexDiscovery 只读顶层 models 数组（codex debug models 的 stdout 是一行 JSON）。
//
// 顶层解不开、或没有 models 数组（缺席或 null），返回 ErrDiscoveryUnparseable。
// visibility 不是 list 的行跳过（[推断] codex 自己的选择器也不列它们；同 claude
// 的 disabled 行）；slug 为空或 hasControlChar 为真的行跳过。强度只收
// codexEffortLevels 里有的档，保持该模型自己的顺序，一档都没有时是 nil；
// display_name 含控制字符时记空串，建议名退回由 slug 生成。slug 以 custom/
// 开头的行照样返回、Custom 为真（repl 只数它们）。一行非 custom 的都没有时返回
// ErrDiscoveryEmpty。Model 取 slug，Alias 留空（codex 配置的 model 写的就是 slug）。
func parseCodexDiscovery(stdout []byte) ([]DiscoveredModel, error) {
	var v struct {
		Models *[]codexCatalogModel `json:"models"`
	}
	if err := json.Unmarshal(stdout, &v); err != nil {
		return nil, fmt.Errorf("codex: stdout is not JSON: %w", ErrDiscoveryUnparseable)
	}
	if v.Models == nil {
		return nil, fmt.Errorf("codex: no models array: %w", ErrDiscoveryUnparseable)
	}
	var out []DiscoveredModel
	selectable := 0
	for _, m := range *v.Models {
		if m.Visibility != "list" || m.Slug == "" || hasControlChar(m.Slug) {
			continue
		}
		var efforts []string
		for _, l := range m.SupportedReasoningLevels {
			if codexEffortAllowed(l.Effort) {
				efforts = append(efforts, l.Effort)
			}
		}
		name := m.DisplayName
		if hasControlChar(name) {
			name = ""
		}
		custom := strings.HasPrefix(m.Slug, codexCustomSlugPrefix)
		if !custom {
			selectable++
		}
		out = append(out, DiscoveredModel{
			Provider: "codex",
			Model:    m.Slug,
			Name:     name,
			Efforts:  efforts,
			Source:   ModelSourceCodexCatalog,
			Custom:   custom,
		})
	}
	if selectable == 0 {
		return nil, fmt.Errorf("codex: %w", ErrDiscoveryEmpty)
	}
	return out, nil
}

// hasControlChar 报告 s 里有没有码位小于 0x20 的字符或 0x7f。四家解析器说的控制字符就是这一处。
func hasControlChar(s string) bool {
	for _, c := range s {
		if c < 0x20 || c == 0x7f {
			return true
		}
	}
	return false
}
