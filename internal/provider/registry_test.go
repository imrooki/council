package provider_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"council/internal/provider"
)

// TestRegistry_IDsAreExactlyTheFourAdapters: IDs() 必须按字典序返回，且不多不少
// 正好是这四个。09_STEP6_SPEC.md B 节「不写 codex.go」、K 节「codex 不在已登记
// 列表里」是 Step 6 的范围裁决；CX1 契约（2026-09-28，00_DECISIONS #45 末尾
// 维护者 裁「接入 codex」）登记了 codex，本条随之改成四家。整表相等比逐个
// 检查更强：它同时钉住了「没有第五家被意外注册」这件事。
func TestRegistry_IDsAreExactlyTheFourAdapters(t *testing.T) {
	want := []string{"agy", "claude", "codex", "grok"}
	got := provider.IDs()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs() = %v, want %v (字典序)", got, want)
	}
}

// TestRegistry_IDsOrderIsStableAcrossCalls: "顺序必须稳定"（09a IDs 注释）。
// map 迭代顺序在 Go 里是随机的，IDs() 内部必须自己排序，不能依赖调用者走运。
func TestRegistry_IDsOrderIsStableAcrossCalls(t *testing.T) {
	first := provider.IDs()
	for i := 0; i < 5; i++ {
		if !reflect.DeepEqual(provider.IDs(), first) {
			t.Fatalf("IDs() order changed across calls: %v vs %v", first, provider.IDs())
		}
	}
}

// TestRegistry_LookupUnknownProvider: 找不到时返回包了 ErrUnknownProvider 的
// 错误，且信息里列出候选（09a Lookup 注释、Register 实现都点名了这一条）。
func TestRegistry_LookupUnknownProvider(t *testing.T) {
	// 这里原来查的是 codex（那时它没有适配器）；CX1 登记了 codex 之后换成一个
	// 永远不会被登记的 id，候选清单随之多一个 codex。
	_, err := provider.Lookup("no-such-provider")
	if !errors.Is(err, provider.ErrUnknownProvider) {
		t.Fatalf("err = %v, want ErrUnknownProvider", err)
	}
	msg := err.Error()
	for _, id := range []string{"agy", "claude", "codex", "grok"} {
		if !strings.Contains(msg, id) {
			t.Errorf("error message %q does not list candidate %q", msg, id)
		}
	}
}

// TestRegistry_IDMatchesRegistrationKey: 四家的 ID() 必须与它注册时用的 key
// 一致——Lookup(id) 拿到的 Provider，它自己报的 ID() 应该原样等于查询用的 id，
// 否则 Lookup 表面查得到、实际货不对板。
func TestRegistry_IDMatchesRegistrationKey(t *testing.T) {
	for _, id := range provider.IDs() {
		t.Run(id, func(t *testing.T) {
			p, err := provider.Lookup(id)
			if err != nil {
				t.Fatalf("Lookup(%s): %v", id, err)
			}
			if p.ID() != id {
				t.Fatalf("p.ID() = %q, want %q", p.ID(), id)
			}
		})
	}
}

// TestRegistry_RegisterDuplicatePanics: 重复登记直接 panic（09a Register 注释：
// "那是编译期就该发现的编码错误，不该留到运行时"）。复用一个已注册的 Provider
// 值（从 Lookup 拿到）去触发，不去定义一个新的桩类型、也不注册任何新 id——
// Register 的实现是"先查是否存在、存在就 panic，不写 map"，所以这次调用
// 在到达任何状态修改之前就已经 panic，不会污染全局 registry，后面的用例仍然
// 看到干净的四家。
func TestRegistry_RegisterDuplicatePanics(t *testing.T) {
	p, err := provider.Lookup("claude")
	if err != nil {
		t.Fatalf("Lookup(claude): %v", err)
	}
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("Register did not panic on duplicate id %q", p.ID())
		}
	}()
	provider.Register(p)
}

// TestRegistry_CapabilitiesDeclareEveryField 逐字段检查 Capabilities()，钉住
// "漏填一项等于未实测，不是支持"这条硬约束（09a capabilities.go 注释、
// 09_STEP6_SPEC.md C 节）。CleanInstructionEnv 与 CacheAcrossTurns 是两个
// 例外——四家在这两位上分别给出了各自的实测结论（不是清一色 Unknown），
// 所以下面对它们做的是按 provider 钉住具体取值的正面断言，不是"非 Unknown
// 就算过"那种泛化检查，且都要求新增 provider 时显式声明，漏声明直接 t.Fatalf。
func TestRegistry_CapabilitiesDeclareEveryField(t *testing.T) {
	for _, id := range provider.IDs() {
		t.Run(id, func(t *testing.T) {
			p, _ := provider.Lookup(id)
			caps := p.Capabilities()

			if caps.SessionID == provider.SessionIDUnknown {
				t.Errorf("SessionID is Unknown")
			}
			if caps.Effort == provider.EffortUnknown {
				t.Errorf("Effort is Unknown")
			}
			if caps.Identity == provider.IdentityUnknown {
				t.Errorf("Identity is Unknown")
			}
			if caps.Prompt == provider.PromptTransportUnknown {
				t.Errorf("Prompt is Unknown")
			}
			if caps.Prompt == provider.PromptTransportArgv && caps.MaxPromptBytes <= 0 {
				t.Errorf("MaxPromptBytes = %d with Argv transport, want >0 (09a: Argv 传输不许填 0 假装没有上限)", caps.MaxPromptBytes)
			}
			if len(caps.Permissions) == 0 {
				t.Errorf("Permissions is empty, want at least one declared tier")
			}
			if caps.UsageFormula == "" {
				t.Errorf("UsageFormula is empty")
			}
			if caps.OutputSchema == provider.SupportUnknown {
				t.Errorf("OutputSchema is SupportUnknown")
			}
			if caps.MaxTurnsFlag == provider.SupportUnknown {
				t.Errorf("MaxTurnsFlag is SupportUnknown")
			}
			if caps.ReportsCost == provider.SupportUnknown {
				t.Errorf("ReportsCost is SupportUnknown")
			}
			if caps.ReportsCacheCreate == provider.SupportUnknown {
				t.Errorf("ReportsCacheCreate is SupportUnknown")
			}
			if caps.ReportsObservedModel == provider.SupportUnknown {
				t.Errorf("ReportsObservedModel is SupportUnknown")
			}
			// CleanInstructionEnv 在 2026-09-21 之前三家都是 Unknown，本条原本正面断言
			// 「必须是 Unknown」。隔离实测做完之后三家分出了三种取值
			//（claude=Yes via --safe-mode，grok=No 实测做不到，agy 仍未测），
			// 所以改成按 provider 钉住各自的**实测结论**：钉一个具体取值才有意义，
			// 钉「全都 Unknown」只是把一句过期的文档抄进了断言。
			wantClean := map[string]provider.Support{
				"claude": provider.SupportYes,
				"grok":   provider.SupportNo,
				"agy":    provider.SupportUnknown,
				"codex":  provider.SupportNo,
			}
			want, ok := wantClean[id]
			if !ok {
				t.Fatalf("新增了 provider %q 却没有为它声明 CleanInstructionEnv 的预期取值；"+
					"加适配器时必须同时给出这一位的依据（实测、或明确记为未测）", id)
			}
			if caps.CleanInstructionEnv != want {
				t.Errorf("CleanInstructionEnv = %v，期望 %v（依据见该适配器 Capabilities 里的注释）",
					caps.CleanInstructionEnv, want)
			}

			// CacheAcrossTurns 同样按 provider 钉住实测结论，形状与上面
			// CleanInstructionEnv 那一段完全对称：claude 与 grok 都在真机上
			// 见过同一条会话 resume 之后 cache_read 命中，agy 见过 input_tokens
			// 按轮数涨而 cache_read 恒为 0，codex 见过同一 thread 第二轮
			// cached_input_tokens 从 0 到 16000（依据逐条写在各适配器 Capabilities()
			// 的注释里）。再新增 provider 而没有为这一位给出实测结论时，
			// 必须在这里当场 t.Fatalf，不能让它默默留在零值 SupportUnknown。
			wantCache := map[string]provider.Support{
				"claude": provider.SupportYes,
				"grok":   provider.SupportYes,
				"agy":    provider.SupportNo,
				"codex":  provider.SupportYes,
			}
			wantC, okC := wantCache[id]
			if !okC {
				t.Fatalf("新增了 provider %q 却没有为它声明 CacheAcrossTurns 的预期取值；"+
					"加适配器时必须同时给出这一位的依据（实测、或明确记为未测）", id)
			}
			if caps.CacheAcrossTurns != wantC {
				t.Errorf("CacheAcrossTurns = %v，期望 %v（依据见该适配器 Capabilities 里的注释）",
					caps.CacheAcrossTurns, wantC)
			}

			// WriteBoundary 同样按 provider 钉住实测结论（11c 契约第 8.1 节
			// 验收第 1 条）：claude / agy 实测围得住，grok 实测围不住，codex 未测、
			// 明确记为 Unknown（CX1）。再新增 provider 而没有为这一位给出实测结论时，必须在这里
			// 当场 t.Fatalf，不能让它默默留在零值 SupportUnknown——那会让
			// validateRequest 的写入边界闸拒掉这一家的每一个 workspace 请求
			// 而不解释原因。
			wantWriteBoundary := map[string]provider.Support{
				"claude": provider.SupportYes,
				"grok":   provider.SupportNo,
				"agy":    provider.SupportYes,
				"codex":  provider.SupportUnknown,
			}
			wantWB, okWB := wantWriteBoundary[id]
			if !okWB {
				t.Fatalf("新增了 provider %q 却没有为它声明 WriteBoundary 的预期取值；"+
					"加适配器时必须同时给出这一位的依据（实测、或明确记为未测）", id)
			}
			if caps.WriteBoundary != wantWB {
				t.Errorf("WriteBoundary = %v，期望 %v（依据见该适配器 Capabilities 里的注释）",
					caps.WriteBoundary, wantWB)
			}
		})
	}
}

// TestRegistry_OnlyClaudeReportsCost: 05 §3 与 09a 都点名"只有 claude 报金额"。
// 这条不是"每项都非 Unknown"那种泛化检查能钉住的，单独写一条。
func TestRegistry_OnlyClaudeReportsCost(t *testing.T) {
	for _, id := range provider.IDs() {
		p, _ := provider.Lookup(id)
		got := p.Capabilities().ReportsCost
		want := provider.SupportNo
		if id == "claude" {
			want = provider.SupportYes
		}
		if got != want {
			t.Errorf("%s: ReportsCost = %v, want %v", id, got, want)
		}
	}
}
