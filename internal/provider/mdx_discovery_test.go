package provider_test

// MD1 契约 §2.1：provider 包的模型识别（能力位、识别命令、三家解析器、建议 id 与
// 执行器错误映射）。只用导出标识符；夹具字节逐字取自契约 §2.0，不许改动。

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"council/internal/provider"
)

// mdxClaudeFixture 是契约 §2.0 的 claude 夹具（1586 字节，LF，末尾一个换行）：
// 第 1 行 system 帧，第 2 行 request_id 不匹配的诱饵回应，第 3 行真回应。
const mdxClaudeFixture = `{"type":"system","subtype":"commands_changed","commands":[]}
{"type":"control_response","response":{"subtype":"success","request_id":"mdx-other","response":{"models":[{"value":"decoy","resolvedModel":"claude-decoy-9","displayName":"Decoy"}]}}}
{"type":"control_response","response":{"subtype":"success","request_id":"council-models","response":{"models":[{"value":"default","resolvedModel":"claude-opus-5-5[1m]","displayName":"Default (recommended)","supportedEffortLevels":["low","medium","high","xhigh","max"]},{"value":"opus[1m]","resolvedModel":"claude-opus-5-5[1m]","displayName":"Opus (1M context)","supportedEffortLevels":["low","medium","high","xhigh","max"]},{"value":"claude-fable-5-1","resolvedModel":"claude-fable-5-1","displayName":"Fable","supportedEffortLevels":["low","medium","high","xhigh","max"]},{"value":"sonnet","resolvedModel":"claude-sonnet-5","displayName":"Sonnet","supportedEffortLevels":["low","medium","high","xhigh","max"]},{"value":"sonnet[1m]","resolvedModel":"claude-sonnet-5[1m]","displayName":"Sonnet 5 (1M context)","supportedEffortLevels":["low","medium","high","xhigh","max"]},{"value":"haiku","resolvedModel":"claude-haiku-4-5-20251001","displayName":"Haiku"},{"value":"opus-legacy","resolvedModel":"claude-opus-4-1","displayName":"Opus 4.1","supportedEffortLevels":["high"],"disabled":true},{"value":"mdx-lowmed","resolvedModel":"claude-mdx-lowmed-2-0","displayName":"Mdx","supportedEffortLevels":["low","medium"]},{"value":"claude-mdx-plain-1-2","displayName":"Plain","supportedEffortLevels":["high","max"]}]},"pending_permission_requests":[]}}
`

// mdxAgyFixture 是契约 §2.0 的 agy 夹具（1823 字节，一行，末尾一个换行）。
// response 字段里的 \t \n 是 JSON 转义的两个字符，原始字符串字面量里照样是两个字符。
const mdxAgyFixture = `{"conversation_id":"","status":"SUCCESS","response":"gemini-3.8-flash-high\tGemini 3.8 Flash (High)\ngemini-3.8-flash-medium\tGemini 3.8 Flash (Medium)\ngemini-3.8-flash-low\tGemini 3.8 Flash (Low)\ngemini-3.7-flash-high\tGemini 3.7 Flash (High)\ngemini-3.7-flash-medium\tGemini 3.7 Flash (Medium)\ngemini-3.7-flash-low\tGemini 3.7 Flash (Low)\ngemini-3.6-flash-high\tGemini 3.6 Flash (High)\ngemini-3.6-flash-medium\tGemini 3.6 Flash (Medium)\ngemini-3.6-flash-low\tGemini 3.6 Flash (Low)\ngemini-3.1-pro-high\tGemini 3.1 Pro (High)\ngemini-3.1-pro-low\tGemini 3.1 Pro (Low)\nclaude-sonnet-4-6\tClaude Sonnet 4.6 (Thinking)\nclaude-opus-4-6-thinking\tClaude Opus 4.6 (Thinking)\ngpt-oss-120b-medium\tGPT-OSS 120B (Medium)\n","duration_seconds":0,"num_turns":0,"usage":{"input_tokens":0,"output_tokens":0,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":0},"command":{"name":"models","data":{"models":[{"id":"gemini-3.8-flash-high","label":"Gemini 3.8 Flash (High)"},{"id":"gemini-3.8-flash-medium","label":"Gemini 3.8 Flash (Medium)"},{"id":"gemini-3.8-flash-low","label":"Gemini 3.8 Flash (Low)"},{"id":"gemini-3.7-flash-high","label":"Gemini 3.7 Flash (High)"},{"id":"gemini-3.7-flash-medium","label":"Gemini 3.7 Flash (Medium)"},{"id":"gemini-3.7-flash-low","label":"Gemini 3.7 Flash (Low)"},{"id":"gemini-3.6-flash-high","label":"Gemini 3.6 Flash (High)"},{"id":"gemini-3.6-flash-medium","label":"Gemini 3.6 Flash (Medium)"},{"id":"gemini-3.6-flash-low","label":"Gemini 3.6 Flash (Low)"},{"id":"gemini-3.1-pro-high","label":"Gemini 3.1 Pro (High)"},{"id":"gemini-3.1-pro-low","label":"Gemini 3.1 Pro (Low)"},{"id":"claude-sonnet-4-6","label":"Claude Sonnet 4.6 (Thinking)"},{"id":"claude-opus-4-6-thinking","label":"Claude Opus 4.6 (Thinking)"},{"id":"gpt-oss-120b-medium","label":"GPT-OSS 120B (Medium)"}]}}}
`

// mdxGrokFixture 是契约 §2.0 的 grok 夹具（191 字节，LF，末尾一个换行）。
const mdxGrokFixture = `Model 'grok-4.7' is using its own API key.

Default model: grok-4.7

Available models:
  * grok-4.7 (default)
  - grok-4.6
  - grok-4.5
  - grok-4.3
  - grok-composer-2.5-fast
  - grok-build
`

// mdxInitializeLine 是契约 §3.4 的 initialize 请求行（92 字节，含末尾换行）。
const mdxInitializeLine = `{"type":"control_request","request_id":"council-models","request":{"subtype":"initialize"}}` + "\n"

// mdxSuggestedGolden 是契约 §2.1 P7 的期望块（原型实测，逐字）。
const mdxSuggestedGolden = `claude-opus-5-5[1m]
    def="high" id="opus-5.5-1m-high" idmax="opus-5.5-1m-max" dn="Opus 5.5 1m" dnmax="Opus 5.5 1m max"
claude-fable-5-1
    def="high" id="fable-5.1-high" idmax="fable-5.1-max" dn="Fable 5.1" dnmax="Fable 5.1 max"
claude-sonnet-5
    def="high" id="sonnet-5-high" idmax="sonnet-5-max" dn="Sonnet 5" dnmax="Sonnet 5 max"
claude-sonnet-5[1m]
    def="high" id="sonnet-5-1m-high" idmax="sonnet-5-1m-max" dn="Sonnet 5 1m" dnmax="Sonnet 5 1m max"
claude-haiku-4-5-20251001
    def="" id="haiku-4.5" idmax="haiku-4.5-max" dn="Haiku 4.5" dnmax="Haiku 4.5 max"
claude-mdx-lowmed-2-0
    def="low" id="mdx-lowmed-2.0-low" idmax="mdx-lowmed-2.0-max" dn="Mdx Lowmed 2.0 low" dnmax="Mdx Lowmed 2.0 max"
claude-mdx-plain-1-2
    def="high" id="mdx-plain-1.2-high" idmax="mdx-plain-1.2-max" dn="Mdx Plain 1.2" dnmax="Mdx Plain 1.2 max"
Gemini 3.8 Flash (High)
    def="" id="gemini-3.8-flash-high" idmax="gemini-3.8-flash-high" dn="Agy Gemini 3.8 Flash (High)" dnmax="Agy Gemini 3.8 Flash (High)"
Gemini 3.8 Flash (Medium)
    def="" id="gemini-3.8-flash-medium" idmax="gemini-3.8-flash-medium" dn="Agy Gemini 3.8 Flash (Medium)" dnmax="Agy Gemini 3.8 Flash (Medium)"
Gemini 3.8 Flash (Low)
    def="" id="gemini-3.8-flash-low" idmax="gemini-3.8-flash-low" dn="Agy Gemini 3.8 Flash (Low)" dnmax="Agy Gemini 3.8 Flash (Low)"
Gemini 3.7 Flash (High)
    def="" id="gemini-3.7-flash-high" idmax="gemini-3.7-flash-high" dn="Agy Gemini 3.7 Flash (High)" dnmax="Agy Gemini 3.7 Flash (High)"
Gemini 3.7 Flash (Medium)
    def="" id="gemini-3.7-flash-medium" idmax="gemini-3.7-flash-medium" dn="Agy Gemini 3.7 Flash (Medium)" dnmax="Agy Gemini 3.7 Flash (Medium)"
Gemini 3.7 Flash (Low)
    def="" id="gemini-3.7-flash-low" idmax="gemini-3.7-flash-low" dn="Agy Gemini 3.7 Flash (Low)" dnmax="Agy Gemini 3.7 Flash (Low)"
Gemini 3.6 Flash (High)
    def="" id="gemini-3.6-flash-high" idmax="gemini-3.6-flash-high" dn="Agy Gemini 3.6 Flash (High)" dnmax="Agy Gemini 3.6 Flash (High)"
Gemini 3.6 Flash (Medium)
    def="" id="gemini-3.6-flash-medium" idmax="gemini-3.6-flash-medium" dn="Agy Gemini 3.6 Flash (Medium)" dnmax="Agy Gemini 3.6 Flash (Medium)"
Gemini 3.6 Flash (Low)
    def="" id="gemini-3.6-flash-low" idmax="gemini-3.6-flash-low" dn="Agy Gemini 3.6 Flash (Low)" dnmax="Agy Gemini 3.6 Flash (Low)"
Gemini 3.1 Pro (High)
    def="" id="gemini-3.1-pro-high" idmax="gemini-3.1-pro-high" dn="Agy Gemini 3.1 Pro (High)" dnmax="Agy Gemini 3.1 Pro (High)"
Gemini 3.1 Pro (Low)
    def="" id="gemini-3.1-pro-low" idmax="gemini-3.1-pro-low" dn="Agy Gemini 3.1 Pro (Low)" dnmax="Agy Gemini 3.1 Pro (Low)"
Claude Sonnet 4.6 (Thinking)
    def="" id="agy-sonnet-4.6-thinking" idmax="agy-sonnet-4.6-thinking" dn="Agy Claude Sonnet 4.6 (Thinking)" dnmax="Agy Claude Sonnet 4.6 (Thinking)"
Claude Opus 4.6 (Thinking)
    def="" id="agy-opus-4.6-thinking" idmax="agy-opus-4.6-thinking" dn="Agy Claude Opus 4.6 (Thinking)" dnmax="Agy Claude Opus 4.6 (Thinking)"
GPT-OSS 120B (Medium)
    def="" id="gpt-oss-120b-medium" idmax="gpt-oss-120b-medium" dn="Agy GPT-OSS 120B (Medium)" dnmax="Agy GPT-OSS 120B (Medium)"
grok-4.7
    def="" id="grok-4.7" idmax="grok-4.7" dn="Grok 4.7" dnmax="Grok 4.7"
grok-4.6
    def="" id="grok-4.6" idmax="grok-4.6" dn="Grok 4.6" dnmax="Grok 4.6"
grok-4.5
    def="" id="grok-4.5" idmax="grok-4.5" dn="Grok 4.5" dnmax="Grok 4.5"
grok-4.3
    def="" id="grok-4.3" idmax="grok-4.3" dn="Grok 4.3" dnmax="Grok 4.3"
grok-composer-2.5-fast
    def="" id="grok-composer-2.5-fast" idmax="grok-composer-2.5-fast" dn="Grok Composer 2.5 Fast" dnmax="Grok Composer 2.5 Fast"
grok-build
    def="" id="grok-build" idmax="grok-build" dn="Grok Build" dnmax="Grok Build"
`

// mdxCheckFixtures 是前提守卫：夹具的字节数与契约写的对不上，说明抄错了，
// 后面的断言全都没有意义。
func mdxCheckFixtures(t *testing.T) {
	t.Helper()
	if len(mdxClaudeFixture) != 1586 {
		t.Fatalf("claude 夹具应为 1586 字节，实际 %d", len(mdxClaudeFixture))
	}
	if len(mdxAgyFixture) != 1823 {
		t.Fatalf("agy 夹具应为 1823 字节，实际 %d", len(mdxAgyFixture))
	}
	if len(mdxGrokFixture) != 191 {
		t.Fatalf("grok 夹具应为 191 字节，实际 %d", len(mdxGrokFixture))
	}
	if len(mdxInitializeLine) != 92 {
		t.Fatalf("initialize 请求行应为 92 字节，实际 %d", len(mdxInitializeLine))
	}
}

var mdxAllEfforts = []string{"low", "medium", "high", "xhigh", "max"}

// mdxWantClaude 是 P4 的 7 行期望。
func mdxWantClaude() []provider.DiscoveredModel {
	row := func(model, alias, name string, efforts []string) provider.DiscoveredModel {
		return provider.DiscoveredModel{
			Provider: "claude",
			Model:    model,
			Alias:    alias,
			Name:     name,
			Efforts:  efforts,
			Source:   provider.ModelSourceSubscription,
		}
	}
	return []provider.DiscoveredModel{
		row("claude-opus-5-5[1m]", "opus[1m]", "Opus (1M context)", mdxAllEfforts),
		row("claude-fable-5-1", "claude-fable-5-1", "Fable", mdxAllEfforts),
		row("claude-sonnet-5", "sonnet", "Sonnet", mdxAllEfforts),
		row("claude-sonnet-5[1m]", "sonnet[1m]", "Sonnet 5 (1M context)", mdxAllEfforts),
		row("claude-haiku-4-5-20251001", "haiku", "Haiku", nil),
		row("claude-mdx-lowmed-2-0", "mdx-lowmed", "Mdx", []string{"low", "medium"}),
		row("claude-mdx-plain-1-2", "claude-mdx-plain-1-2", "Plain", []string{"high", "max"}),
	}
}

// mdxAgyRows 是 agy 夹具 command.data.models 的 id 与 label，顺序同夹具。
var mdxAgyRows = [][2]string{
	{"gemini-3.8-flash-high", "Gemini 3.8 Flash (High)"},
	{"gemini-3.8-flash-medium", "Gemini 3.8 Flash (Medium)"},
	{"gemini-3.8-flash-low", "Gemini 3.8 Flash (Low)"},
	{"gemini-3.7-flash-high", "Gemini 3.7 Flash (High)"},
	{"gemini-3.7-flash-medium", "Gemini 3.7 Flash (Medium)"},
	{"gemini-3.7-flash-low", "Gemini 3.7 Flash (Low)"},
	{"gemini-3.6-flash-high", "Gemini 3.6 Flash (High)"},
	{"gemini-3.6-flash-medium", "Gemini 3.6 Flash (Medium)"},
	{"gemini-3.6-flash-low", "Gemini 3.6 Flash (Low)"},
	{"gemini-3.1-pro-high", "Gemini 3.1 Pro (High)"},
	{"gemini-3.1-pro-low", "Gemini 3.1 Pro (Low)"},
	{"claude-sonnet-4-6", "Claude Sonnet 4.6 (Thinking)"},
	{"claude-opus-4-6-thinking", "Claude Opus 4.6 (Thinking)"},
	{"gpt-oss-120b-medium", "GPT-OSS 120B (Medium)"},
}

// mdxWantAgy 是 P5 的 14 行期望：前 11 行是订阅，后 3 行走 agy 额度。
func mdxWantAgy() []provider.DiscoveredModel {
	var out []provider.DiscoveredModel
	for i, r := range mdxAgyRows {
		src := provider.ModelSourceSubscription
		if i >= 11 {
			src = provider.ModelSourceAgyQuota
		}
		out = append(out, provider.DiscoveredModel{
			Provider: "agy",
			Model:    r[1],
			Alias:    r[0],
			Name:     r[1],
			Source:   src,
		})
	}
	return out
}

// mdxWantGrok 按名字给出 grok 行的期望（Alias 空、Efforts nil、来源本地配置）。
func mdxWantGrok(names ...string) []provider.DiscoveredModel {
	var out []provider.DiscoveredModel
	for _, n := range names {
		out = append(out, provider.DiscoveredModel{
			Provider: "grok",
			Model:    n,
			Name:     n,
			Source:   provider.ModelSourceLocalConfig,
		})
	}
	return out
}

var mdxGrokNames = []string{"grok-4.7", "grok-4.6", "grok-4.5", "grok-4.3", "grok-composer-2.5-fast", "grok-build"}

// TestMdxCapabilitiesDeclareModelDiscovery 覆盖 P1。
//
// 【判别力】claude 的能力位改成 Unknown（M22）时 claude 那一项不等；
// 再新增 provider 而不在这里声明它的识别方式时 Fatalf。
// CX1 登记了 codex；CXH 把它接进识别框架，声明 Subcommand（codex debug models）；
// codex.go 退回 Unknown 时 codex 那一条不等。
// Available 四个取值两真两假，写成恒真或恒假都会有一项不等。
func TestMdxCapabilitiesDeclareModelDiscovery(t *testing.T) {
	want := map[string]provider.ModelDiscovery{
		"claude": provider.ModelDiscoveryControlHandshake,
		"agy":    provider.ModelDiscoverySubcommand,
		"grok":   provider.ModelDiscoverySubcommand,
		"codex":  provider.ModelDiscoverySubcommand,
	}
	for _, id := range provider.IDs() {
		if _, ok := want[id]; !ok {
			t.Fatalf("登记了 provider %q 却没有在这里声明它的 ModelDiscovery 预期取值；再新增 provider 必须同时声明这一位", id)
		}
	}
	for _, id := range []string{"claude", "agy", "grok"} {
		p, err := provider.Lookup(id)
		if err != nil {
			t.Fatalf("Lookup(%q): %v", id, err)
		}
		if got := p.Capabilities().ModelDiscovery; got != want[id] {
			t.Errorf("%s 的 ModelDiscovery = %d，期望 %d", id, got, want[id])
		}
	}

	// CXH：codex 声明 Subcommand。want 表里的 codex 一项只让上面的 Fatalf 放行，
	// 取值由下面这一条钉住（与上面三家的循环分开写，那三条一字不动）。
	pc, err := provider.Lookup("codex")
	if err != nil {
		t.Fatalf("Lookup(%q): %v", "codex", err)
	}
	if got := pc.Capabilities().ModelDiscovery; got != provider.ModelDiscoverySubcommand {
		t.Errorf("codex 的 ModelDiscovery = %d，期望 %d", got, provider.ModelDiscoverySubcommand)
	}

	// §3.1 冻结的取值
	if provider.ModelDiscoveryUnknown != 0 || provider.ModelDiscoverySubcommand != 1 ||
		provider.ModelDiscoveryControlHandshake != 2 || provider.ModelDiscoveryUnsupported != 3 {
		t.Errorf("ModelDiscovery 的取值与契约 §3.1 不符: %d %d %d %d",
			provider.ModelDiscoveryUnknown, provider.ModelDiscoverySubcommand,
			provider.ModelDiscoveryControlHandshake, provider.ModelDiscoveryUnsupported)
	}

	avail := []struct {
		d    provider.ModelDiscovery
		want bool
	}{
		{provider.ModelDiscoveryUnknown, false},
		{provider.ModelDiscoverySubcommand, true},
		{provider.ModelDiscoveryControlHandshake, true},
		{provider.ModelDiscoveryUnsupported, false},
	}
	for _, a := range avail {
		if got := a.d.Available(); got != a.want {
			t.Errorf("ModelDiscovery(%d).Available() = %v，期望 %v", a.d, got, a.want)
		}
	}
}

// TestMdxDiscoveryCommandFor 覆盖 P2。
//
// 【判别力】claude 的参数少了 --safe-mode（M19）时 Args 不等；claude 的
// CloseStdinAfter 为 nil（M20）时最后一条断言红。agy / grok 的 Stdin 与
// CloseStdinAfter 必须是 nil：给了非空 stdin 或谓词的写法在这里红。
func TestMdxDiscoveryCommandFor(t *testing.T) {
	mdxCheckFixtures(t)

	c, err := provider.DiscoveryCommandFor("claude", "P")
	if err != nil {
		t.Fatalf("claude: %v", err)
	}
	if c.Path != "P" {
		t.Errorf("claude Path = %q，期望 %q", c.Path, "P")
	}
	wantClaudeArgs := []string{"-p", "--safe-mode", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--strict-mcp-config"}
	if !reflect.DeepEqual(c.Args, wantClaudeArgs) {
		t.Errorf("claude Args = %q，期望 %q", c.Args, wantClaudeArgs)
	}
	if string(c.Stdin) != mdxInitializeLine {
		t.Errorf("claude Stdin = %q，期望 %q", c.Stdin, mdxInitializeLine)
	}
	if c.CloseStdinAfter == nil {
		t.Errorf("claude CloseStdinAfter 是 nil，期望非 nil（收到匹配的回应才关 stdin）")
	}

	for _, tc := range []struct {
		id   string
		args []string
	}{
		{"agy", []string{"--output-format", "json", "models"}},
		{"grok", []string{"models"}},
	} {
		c, err := provider.DiscoveryCommandFor(tc.id, "P")
		if err != nil {
			t.Fatalf("%s: %v", tc.id, err)
		}
		if c.Path != "P" {
			t.Errorf("%s Path = %q，期望 %q", tc.id, c.Path, "P")
		}
		if !reflect.DeepEqual(c.Args, tc.args) {
			t.Errorf("%s Args = %q，期望 %q", tc.id, c.Args, tc.args)
		}
		if c.Stdin != nil {
			t.Errorf("%s Stdin = %q，期望 nil", tc.id, c.Stdin)
		}
		if c.CloseStdinAfter != nil {
			t.Errorf("%s CloseStdinAfter 非 nil，期望 nil（写完立刻关 stdin）", tc.id)
		}
	}

	if _, err := provider.DiscoveryCommandFor("nosuch", "P"); !errors.Is(err, provider.ErrDiscoveryUnsupported) {
		t.Errorf("nosuch: err = %v，期望 ErrDiscoveryUnsupported", err)
	}
}

// TestMdxClaudeCloseStdinPredicate 覆盖 P3。
//
// 【判别力】第 2 段是诱饵（request_id 不匹配）：不认 request_id（M1）或只看
// 有没有 control_response 字样（M21）的谓词在它上面为真。第 1 段 system 帧、
// 第 4 段空串、control_request 与非 JSON 都必须为假，恒真的谓词在这些上面红。
func TestMdxClaudeCloseStdinPredicate(t *testing.T) {
	mdxCheckFixtures(t)
	c, err := provider.DiscoveryCommandFor("claude", "P")
	if err != nil || c.CloseStdinAfter == nil {
		t.Fatalf("claude 的 CloseStdinAfter 取不到: err=%v", err)
	}
	segs := strings.Split(mdxClaudeFixture, "\n")
	if len(segs) != 4 {
		t.Fatalf("claude 夹具按换行切应得 4 段，实际 %d", len(segs))
	}
	want := []bool{false, false, true, false}
	for i, s := range segs {
		if got := c.CloseStdinAfter([]byte(s)); got != want[i] {
			t.Errorf("第 %d 段: CloseStdinAfter = %v，期望 %v", i+1, got, want[i])
		}
	}
	if c.CloseStdinAfter([]byte(`{"type":"control_request","request_id":"council-models"}`)) {
		t.Errorf("control_request 那一行: CloseStdinAfter = true，期望 false")
	}
	if c.CloseStdinAfter([]byte("not json")) {
		t.Errorf("not json: CloseStdinAfter = true，期望 false")
	}
}

// TestMdxParseClaudeFixture 覆盖 P4。
//
// 【判别力】不跳 default（M2）多出一行、不跳 disabled（M3）多出 opus-legacy、
// 不认 request_id（M1）读到诱饵、Model 取 value（M4）、不退回 value（M5）
// 各让这张表的某一行变；整表 DeepEqual，haiku 行的 Efforts 必须是 nil。
func TestMdxParseClaudeFixture(t *testing.T) {
	mdxCheckFixtures(t)
	got, err := provider.ParseDiscovery("claude", []byte(mdxClaudeFixture))
	if err != nil {
		t.Fatalf("ParseDiscovery(claude): %v", err)
	}
	if want := mdxWantClaude(); !reflect.DeepEqual(got, want) {
		t.Errorf("claude 解析结果不符\n实际: %#v\n期望: %#v", got, want)
	}
}

// TestMdxParseAgyFixture 覆盖 P5。
//
// 【判别力】agy 的 Claude / GPT-OSS 行不标「走 agy 额度」（M12）时后 3 行的
// Source 不等；Label 三个取值与未知值逐字比。
func TestMdxParseAgyFixture(t *testing.T) {
	mdxCheckFixtures(t)
	got, err := provider.ParseDiscovery("agy", []byte(mdxAgyFixture))
	if err != nil {
		t.Fatalf("ParseDiscovery(agy): %v", err)
	}
	if want := mdxWantAgy(); !reflect.DeepEqual(got, want) {
		t.Errorf("agy 解析结果不符\n实际: %#v\n期望: %#v", got, want)
	}

	labels := []struct {
		s    provider.ModelSource
		want string
	}{
		{provider.ModelSourceSubscription, "订阅"},
		{provider.ModelSourceLocalConfig, "本地配置"},
		{provider.ModelSourceAgyQuota, "走 agy 额度"},
		{provider.ModelSourceUnknown, "-"},
	}
	for _, l := range labels {
		if got := l.s.Label(); got != l.want {
			t.Errorf("ModelSource(%d).Label() = %q，期望 %q", l.s, got, l.want)
		}
	}
	if provider.ModelSourceUnknown != 0 || provider.ModelSourceSubscription != 1 ||
		provider.ModelSourceLocalConfig != 2 || provider.ModelSourceAgyQuota != 3 {
		t.Errorf("ModelSource 的取值与契约 §3.1 不符")
	}
}

// TestMdxParseGrokFixture 覆盖 P6。
//
// 【判别力】不剥 " (default)"（M13）时 grok-4.7 那一行含空格被跳过、或只认
// "  - "（M14）时第 1 行 grok-4.7 消失；CRLF 变体里行尾 \r 不剥时两行都带 \r。
func TestMdxParseGrokFixture(t *testing.T) {
	mdxCheckFixtures(t)
	got, err := provider.ParseDiscovery("grok", []byte(mdxGrokFixture))
	if err != nil {
		t.Fatalf("ParseDiscovery(grok): %v", err)
	}
	if want := mdxWantGrok(mdxGrokNames...); !reflect.DeepEqual(got, want) {
		t.Errorf("grok 解析结果不符\n实际: %#v\n期望: %#v", got, want)
	}

	i := strings.Index(mdxGrokFixture, "Available models:")
	if i < 0 {
		t.Fatalf("grok 夹具里没有 Available models:")
	}
	crlf := mdxGrokFixture[:i] + "Available models:\r\n  * grok-4.7 (default)\r\n  - grok-4.6\r\n"
	got, err = provider.ParseDiscovery("grok", []byte(crlf))
	if err != nil {
		t.Fatalf("ParseDiscovery(grok, CRLF): %v", err)
	}
	if want := mdxWantGrok("grok-4.7", "grok-4.6"); !reflect.DeepEqual(got, want) {
		t.Errorf("grok CRLF 解析结果不符\n实际: %#v\n期望: %#v", got, want)
	}
}

// TestMdxSuggestedNames 覆盖 P7：对 P4 / P5 / P6 解析出的每一行打五个值，
// 与契约的期望块整段比。
//
// 【判别力】默认强度不优先 high（M6）、没有 high 时回 high（M7）、不去日期段
// （M8）、不把相邻数字段连成版本号（M9）、不接方括号尾巴（M10）、agy 不换
// 同家前缀（M11）各让某几行变。
func TestMdxSuggestedNames(t *testing.T) {
	mdxCheckFixtures(t)
	var rows []provider.DiscoveredModel
	for _, pc := range []struct {
		id  string
		fix string
	}{
		{"claude", mdxClaudeFixture},
		{"agy", mdxAgyFixture},
		{"grok", mdxGrokFixture},
	} {
		got, err := provider.ParseDiscovery(pc.id, []byte(pc.fix))
		if err != nil {
			t.Fatalf("ParseDiscovery(%s): %v", pc.id, err)
		}
		rows = append(rows, got...)
	}
	var b strings.Builder
	for _, m := range rows {
		def := m.DefaultEffort()
		fmt.Fprintf(&b, "%s\n    def=%q id=%q idmax=%q dn=%q dnmax=%q\n",
			m.Model, def, m.SuggestedID(def), m.SuggestedID("max"),
			m.SuggestedDisplayName(def), m.SuggestedDisplayName("max"))
	}
	if got := b.String(); got != mdxSuggestedGolden {
		t.Errorf("建议名与契约 P7 不符\n实际:\n%s\n期望:\n%s", got, mdxSuggestedGolden)
	}
}

// TestMdxParseRejects 覆盖 P8：每一行都得不到结果，只差报哪个哨兵。
//
// 【判别力】第 7 行的 models 合法，不看 status（M16）时解析出 1 行；第 12、13
// 行只差有没有 "Available models:"（M15 对调时两行一起红）；第 15-18、20、21
// 行各让一个防御过滤单独负责（M91-M96）；第 19 行 subtype 是 error 却带合法
// models（M97）。Unparseable 与 Empty 两个方向都断言，报错哨兵写反会红。
func TestMdxParseRejects(t *testing.T) {
	rows := []struct {
		no       int
		provider string
		in       string
		want     string
	}{
		{1, "claude", "{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"mdx-other\",\"response\":{\"models\":[{\"value\":\"x\",\"resolvedModel\":\"claude-x-1\"}]}}}\n", "Unparseable"},
		{2, "claude", "{\"type\":\"control_response\",\"response\":{\"subtype\":\"error\",\"request_id\":\"council-models\",\"error\":\"boom\"}}\n", "Unparseable"},
		{3, "claude", "{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"council-models\",\"response\":{\"models\":[]}}}\n", "Empty"},
		{4, "claude", "{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"council-models\",\"response\":{\"models\":[{\"value\":\"default\",\"resolvedModel\":\"claude-a-1\"},{\"value\":\"b\",\"resolvedModel\":\"claude-b-1\",\"disabled\":true}]}}}\n", "Empty"},
		{5, "claude", "", "Unparseable"},
		{6, "agy", "not json", "Unparseable"},
		{7, "agy", "{\"status\":\"ERROR\",\"command\":{\"name\":\"models\",\"data\":{\"models\":[{\"id\":\"a\",\"label\":\"A\"}]}}}", "Unparseable"},
		{8, "agy", "{\"status\":\"SUCCESS\"}", "Unparseable"},
		{9, "agy", "{\"status\":\"SUCCESS\",\"command\":{\"name\":\"agents\",\"data\":{\"models\":[{\"id\":\"a\",\"label\":\"A\"}]}}}", "Unparseable"},
		{10, "agy", "{\"status\":\"SUCCESS\",\"command\":{\"name\":\"models\",\"data\":{\"models\":[]}}}", "Empty"},
		{11, "agy", "{\"status\":\"SUCCESS\",\"command\":{\"name\":\"models\",\"data\":{\"models\":[{\"id\":\"a\",\"label\":\"\"}]}}}", "Empty"},
		{12, "grok", "", "Unparseable"},
		{13, "grok", "Available models:\n", "Empty"},
		{14, "grok", "Available models:\n  - bad name\n", "Empty"},
		{15, "grok", "Available models:\n  - a\x1bb\n", "Empty"},
		{16, "agy", "{\"status\":\"SUCCESS\",\"command\":{\"name\":\"models\",\"data\":{\"models\":[{\"id\":\"a\",\"label\":\"A\\u0007\"}]}}}", "Empty"},
		{17, "claude", "{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"council-models\",\"response\":{\"models\":[{\"value\":\"x\",\"resolvedModel\":\"claude-x\\u0007\"}]}}}\n", "Empty"},
		{18, "claude", "{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"council-models\",\"response\":{\"models\":[{\"value\":\"\",\"resolvedModel\":\"\",\"displayName\":\"E\"}]}}}\n", "Empty"},
		{19, "claude", "{\"type\":\"control_response\",\"response\":{\"subtype\":\"error\",\"request_id\":\"council-models\",\"response\":{\"models\":[{\"value\":\"x\",\"resolvedModel\":\"claude-x-1\"}]}}}\n", "Unparseable"},
		{20, "grok", "Available models:\n  - \n", "Empty"},
		{21, "grok", "Available models:\n  - a\x7fb\n", "Empty"},
	}
	for _, r := range rows {
		r := r
		t.Run(fmt.Sprintf("row%02d", r.no), func(t *testing.T) {
			got, err := provider.ParseDiscovery(r.provider, []byte(r.in))
			if len(got) != 0 {
				t.Errorf("结果应为空，实际 %d 行: %#v", len(got), got)
			}
			isU := errors.Is(err, provider.ErrDiscoveryUnparseable)
			isE := errors.Is(err, provider.ErrDiscoveryEmpty)
			switch r.want {
			case "Unparseable":
				if !isU || isE {
					t.Errorf("err = %v，期望 ErrDiscoveryUnparseable 且不是 ErrDiscoveryEmpty", err)
				}
			case "Empty":
				if !isE || isU {
					t.Errorf("err = %v，期望 ErrDiscoveryEmpty 且不是 ErrDiscoveryUnparseable", err)
				}
			default:
				t.Fatalf("表里的期望写错了: %q", r.want)
			}
		})
	}
	t.Run("nosuch", func(t *testing.T) {
		got, err := provider.ParseDiscovery("nosuch", []byte("x"))
		if len(got) != 0 || !errors.Is(err, provider.ErrDiscoveryUnsupported) {
			t.Errorf("nosuch: got=%#v err=%v，期望空结果与 ErrDiscoveryUnsupported", got, err)
		}
	})
}

// TestMdxDiscoverModelsMapsExecErrors 覆盖 P9。
//
// 【判别力】不认 DeadlineExceeded（M17）时第一种变成 Failed；把其它错误也
// 映射成 Timeout（M18）时第二、三种不等。取消（Canceled）不是超时。
// 没有登记的 provider（nosuch）没有识别方式，执行器一次都不许被调。
func TestMdxDiscoverModelsMapsExecErrors(t *testing.T) {
	mdxCheckFixtures(t)
	ctx := context.Background()
	errExec := func(e error) provider.DiscoveryExec {
		return func(context.Context, provider.DiscoveryCommand) ([]byte, error) { return nil, e }
	}

	_, err := provider.DiscoverModels(ctx, "grok", "P", errExec(fmt.Errorf("killed: %w", context.DeadlineExceeded)))
	if err == nil || err.Error() != "model discovery timed out: killed: context deadline exceeded" {
		t.Errorf("超时: err = %v，期望文本 %q", err, "model discovery timed out: killed: context deadline exceeded")
	}
	if !errors.Is(err, provider.ErrDiscoveryTimeout) || errors.Is(err, provider.ErrDiscoveryFailed) {
		t.Errorf("超时: err = %v，期望是 ErrDiscoveryTimeout 且不是 ErrDiscoveryFailed", err)
	}

	_, err = provider.DiscoverModels(ctx, "grok", "P", errExec(errors.New("mdx boom")))
	if err == nil || err.Error() != "model discovery command failed: mdx boom" {
		t.Errorf("失败: err = %v，期望文本 %q", err, "model discovery command failed: mdx boom")
	}
	if !errors.Is(err, provider.ErrDiscoveryFailed) || errors.Is(err, provider.ErrDiscoveryTimeout) {
		t.Errorf("失败: err = %v，期望是 ErrDiscoveryFailed 且不是 ErrDiscoveryTimeout", err)
	}

	_, err = provider.DiscoverModels(ctx, "grok", "P", errExec(fmt.Errorf("x: %w", context.Canceled)))
	if !errors.Is(err, provider.ErrDiscoveryFailed) || errors.Is(err, provider.ErrDiscoveryTimeout) {
		t.Errorf("取消: err = %v，期望是 ErrDiscoveryFailed 且不是 ErrDiscoveryTimeout", err)
	}

	var seen []provider.DiscoveryCommand
	okExec := func(_ context.Context, c provider.DiscoveryCommand) ([]byte, error) {
		seen = append(seen, c)
		return []byte(mdxGrokFixture), nil
	}
	got, err := provider.DiscoverModels(ctx, "grok", "P", okExec)
	if err != nil {
		t.Fatalf("grok 夹具: %v", err)
	}
	if want := mdxWantGrok(mdxGrokNames...); !reflect.DeepEqual(got, want) {
		t.Errorf("grok 夹具解析结果不符\n实际: %#v\n期望: %#v", got, want)
	}
	want, _ := provider.DiscoveryCommandFor("grok", "P")
	if len(seen) != 1 {
		t.Fatalf("执行器应被调 1 次，实际 %d 次", len(seen))
	}
	if seen[0].Path != want.Path || !reflect.DeepEqual(seen[0].Args, want.Args) ||
		!reflect.DeepEqual(seen[0].Stdin, want.Stdin) || (seen[0].CloseStdinAfter == nil) != (want.CloseStdinAfter == nil) {
		t.Errorf("执行器收到的命令 %#v 与 DiscoveryCommandFor(grok, P) 的 %#v 不符", seen[0], want)
	}

	calls := 0
	countExec := func(context.Context, provider.DiscoveryCommand) ([]byte, error) {
		calls++
		return []byte(mdxGrokFixture), nil
	}
	if _, err := provider.DiscoverModels(ctx, "nosuch", "P", countExec); !errors.Is(err, provider.ErrDiscoveryUnsupported) {
		t.Errorf("nosuch: err = %v，期望 ErrDiscoveryUnsupported", err)
	}
	if calls != 0 {
		t.Errorf("nosuch: 执行器被调了 %d 次，期望 0 次", calls)
	}
}
