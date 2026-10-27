package provider_test

// CXH 契约 §2.1：codex 接进 MD1 的识别框架（codex debug models）。
// 夹具与期望块在 cxh_fixture_test.go（契约 §2.0 的 F1、G1，逐字照抄）。

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"council/internal/provider"
)

// cxhCheckFixture 是前提守卫：夹具字节数与契约 §2.0 写的一致，防止编辑链路
// 把 JSON 转义还原成真字符或抄坏。它不对应任何被测变异。
func cxhCheckFixture(t *testing.T) {
	t.Helper()
	if n := len(cxhCodexFixture()); n != 24948 {
		t.Fatalf("前提：cxhCodexFixture() 应为 24948 字节，实际 %d", n)
	}
	if n := len(cxhBigModelMessages()); n != 22000 {
		t.Fatalf("前提：cxhBigModelMessages() 应为 22000 字节，实际 %d", n)
	}
}

// TestCxhCodexCapabilityAndCommand 覆盖 P1：codex 声明 Subcommand、识别命令是
// debug models、来源标注是「codex 目录」。
//
// 【判别力】能力位退回 Unknown（C1）时第一条红；命令少了 debug（C2）时 Args
// 不等；给了非 nil 的 Stdin（C3）时 Stdin 那条红；codex 一支从
// DiscoveryCommandFor 删掉（C5）时 err 非 nil；Path 写死（C4）时 Path 不等；
// 标签写成「订阅」（C19）时 Label 不等；在 AgyQuota 与 CodexCatalog 之间多插
// 一个常量（C40）时取值变 5。ModelSource(5) 的「-」单独看是结构性的（HEAD 上
// 也成立），与「== 4」配对才有意义：C40 时 5 就是 CodexCatalog，标签变「codex 目录」。
func TestCxhCodexCapabilityAndCommand(t *testing.T) {
	p, err := provider.Lookup("codex")
	if err != nil {
		t.Fatalf("Lookup(codex): %v", err)
	}
	md := p.Capabilities().ModelDiscovery
	if md != provider.ModelDiscoverySubcommand {
		t.Errorf("codex 的 ModelDiscovery = %d，期望 %d（Subcommand）", md, provider.ModelDiscoverySubcommand)
	}
	if !md.Available() {
		t.Errorf("codex 的 ModelDiscovery.Available() = false，期望 true")
	}

	c, err := provider.DiscoveryCommandFor("codex", "P")
	if err != nil {
		t.Fatalf("DiscoveryCommandFor(codex, P): err = %v，期望 nil", err)
	}
	if c.Path != "P" {
		t.Errorf("codex Path = %q，期望 %q", c.Path, "P")
	}
	if want := []string{"debug", "models"}; !reflect.DeepEqual(c.Args, want) {
		t.Errorf("codex Args = %q，期望 %q", c.Args, want)
	}
	if c.Stdin != nil {
		t.Errorf("codex Stdin = %q，期望 nil", c.Stdin)
	}
	if c.CloseStdinAfter != nil {
		t.Errorf("codex CloseStdinAfter 非 nil，期望 nil（写完空 stdin 就关）")
	}

	if int(provider.ModelSourceCodexCatalog) != 4 {
		t.Errorf("ModelSourceCodexCatalog = %d，期望 4（排在 ModelSourceAgyQuota 之后）", int(provider.ModelSourceCodexCatalog))
	}
	if got := provider.ModelSourceCodexCatalog.Label(); got != "codex 目录" {
		t.Errorf("ModelSourceCodexCatalog.Label() = %q，期望 %q", got, "codex 目录")
	}
	if got := provider.ModelSource(5).Label(); got != "-" {
		t.Errorf("ModelSource(5).Label() = %q，期望 %q", got, "-")
	}
}

// TestCxhParseCodexFixture 覆盖 P2：整份夹具解析成 10 行，整值 DeepEqual。
//
// 【判别力】不滤 visibility（C7：多出 gpt-reserve、cxh-novis 与 hide 的
// custom/qwen3.8-27b）；只滤 hide（C8：多出 cxh-novis）；不滤空 slug（C9）、
// 控制字符 slug（C10）；强度不按 codexEffortLevels 收（C11：claude-cxh-9 多出
// turbo、LOW）；display_name 不滤控制字符（C12）；Custom 恒假（C13）；前缀写成
// custom（C14：customer-cxh 变成 Custom）；来源写错（C18）；Efforts 写成空切片
// （C43：cxh-bell、customer-cxh 的 nil 变 []string{}）；codex 一支从
// ParseDiscovery 删掉（C6，err 非 nil）。第 1 条 model_messages 里的嵌套诱饵
// （slug 为 cxh-decoy）让按键名全文扫的写法多出一行。
func TestCxhParseCodexFixture(t *testing.T) {
	cxhCheckFixture(t)
	got, err := provider.ParseDiscovery("codex", []byte(cxhCodexFixture()))
	if err != nil {
		t.Fatalf("ParseDiscovery(codex, 夹具): err = %v，期望 nil", err)
	}
	if want := cxhWantCodex(); !reflect.DeepEqual(got, want) {
		t.Errorf("codex 解析结果不符\n实际: %#v\n期望: %#v", got, want)
	}
}

// TestCxhSuggestedNames 覆盖 P3：非 custom 行的默认强度、建议 id 与显示名，
// 格式同 MD1 的 P7；子测试 infix 钉住撞名判据是「以……开头」。
//
// 【判别力】撞名前缀规则去掉（C20：claude-cxh-9-high）；强度为空也接 -
// （C23：cxh-bell-）；display_name 为空不回退（C25）；强度 high 也写进显示名
// （C26）；codex 一支从 SuggestedDisplayName / SuggestedID 删掉走 default（C27、
// C28）。cxh-min 一行钉住「没有 high 取第一档」；gpt-6-sol（目录默认 medium）与
// gpt-5.5（目录默认 xhigh）钉住「不取 default_reasoning_level」。撞名判据写成
// strings.Contains（C50）时 golden 部分不红（夹具里会撞名的 slug 以 provider id
// 开头），只有 infix 红：cxh-claude-x 得到 codex-cxh-claude-x-high。
func TestCxhSuggestedNames(t *testing.T) {
	cxhCheckFixture(t)
	models, err := provider.ParseDiscovery("codex", []byte(cxhCodexFixture()))
	if err != nil {
		t.Fatalf("ParseDiscovery(codex, 夹具): err = %v，期望 nil", err)
	}
	var b strings.Builder
	for _, m := range models {
		if m.Custom {
			continue
		}
		def := m.DefaultEffort()
		fmt.Fprintf(&b, "%s\n    def=%q id=%q idmax=%q dn=%q dnmax=%q\n",
			m.Model, def, m.SuggestedID(def), m.SuggestedID("max"),
			m.SuggestedDisplayName(def), m.SuggestedDisplayName("max"))
	}
	if got := b.String(); got != cxhSuggestedGolden {
		t.Errorf("建议名不符\n实际:\n%s\n期望:\n%s", got, cxhSuggestedGolden)
	}

	t.Run("infix", func(t *testing.T) {
		m := provider.DiscoveredModel{Provider: "codex", Model: "cxh-claude-x"}
		if got, want := m.SuggestedID("high"), "cxh-claude-x-high"; got != want {
			t.Errorf("SuggestedID(high) = %q，期望 %q（claude- 在 slug 中间，不是前缀，不换）", got, want)
		}
	})
}

// TestCxhParseCodexRejects 覆盖 P4：先正对照，再 vendor_slash，再 14 行各只报
// 一种哨兵。
//
// 【判别力】第 9-14 行各只比正对照差一处：第 9 行（hide）由 C7；第 10 行（只有
// 一条 custom/）由 C13、C15、C16；第 11 行（空 slug）由 C9；第 12 行（BEL）由
// C10；第 13、14 行（缺 visibility、List）由 C7、C8。第 3-5 行在去掉 nil 检查
// （C17）时解引用 nil 指针 panic，子测试红。两个哨兵两个方向都断言，对调哨兵的
// 写法都红。vendor_slash 钉住 custom 判据是 custom/ 前缀：写成含斜杠（C46）或
// 含 custom/（C48）时这一行被当成 custom、报 Empty。正对照与 vendor_slash 的
// Efforts 一档都没有，C43（空切片）时整值不等；来源写错（C18）时也不等。
func TestCxhParseCodexRejects(t *testing.T) {
	pos, err := provider.ParseDiscovery("codex", []byte(`{"models":[{"slug":"gpt-6-sol","visibility":"list"}]}`))
	wantPos := []provider.DiscoveredModel{{Provider: "codex", Model: "gpt-6-sol", Source: provider.ModelSourceCodexCatalog}}
	if err != nil || !reflect.DeepEqual(pos, wantPos) {
		t.Fatalf("正对照：got=%#v err=%v，期望 %#v 与 nil", pos, err, wantPos)
	}

	t.Run("vendor_slash", func(t *testing.T) {
		got, err := provider.ParseDiscovery("codex", []byte(`{"models":[{"slug":"vendor/custom/cxh-x","visibility":"list"}]}`))
		want := []provider.DiscoveredModel{{Provider: "codex", Model: "vendor/custom/cxh-x", Source: provider.ModelSourceCodexCatalog}}
		if err != nil {
			t.Fatalf("vendor/custom/cxh-x: err = %v，期望 nil（不是 custom/ 开头，是可选行）", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("vendor/custom/cxh-x: got=%#v，期望 %#v", got, want)
		}
	})

	for _, row := range cxhRejectRows {
		row := row
		t.Run(fmt.Sprintf("row%02d", row.no), func(t *testing.T) {
			got, err := provider.ParseDiscovery("codex", []byte(row.in))
			if len(got) != 0 {
				t.Errorf("第 %d 行 %q: 结果 %#v，期望为空", row.no, row.in, got)
			}
			isU := errors.Is(err, provider.ErrDiscoveryUnparseable)
			isE := errors.Is(err, provider.ErrDiscoveryEmpty)
			switch row.want {
			case "Unparseable":
				if !isU || isE {
					t.Errorf("第 %d 行 %q: err = %v，期望 ErrDiscoveryUnparseable 且不是 ErrDiscoveryEmpty", row.no, row.in, err)
				}
			case "Empty":
				if !isE || isU {
					t.Errorf("第 %d 行 %q: err = %v，期望 ErrDiscoveryEmpty 且不是 ErrDiscoveryUnparseable", row.no, row.in, err)
				}
			default:
				t.Fatalf("前提：第 %d 行的 want = %q 不认识", row.no, row.want)
			}
		})
	}
}

// TestCxhDiscoverModelsCodex 覆盖 P5：DiscoverModels 把传进来的 path 交给执行器，
// 命令与 DiscoveryCommandFor 一致，结果与 P2 相同。
//
// 【判别力】Path 写死（C4：cxh-nosuch-codex）时 Path 不等；C2（少 debug）、
// C3（非 nil Stdin）、C5（codex 一支删掉，执行器 0 次、err 非 nil）、C6（解析
// 一支删掉）同 P1 / P2；解析层的变异（C7-C14、C18、C43）同 P2。
func TestCxhDiscoverModelsCodex(t *testing.T) {
	cxhCheckFixture(t)
	var seen []provider.DiscoveryCommand
	exec := func(_ context.Context, c provider.DiscoveryCommand) ([]byte, error) {
		seen = append(seen, c)
		return []byte(cxhCodexFixture()), nil
	}
	got, err := provider.DiscoverModels(context.Background(), "codex", "cxh-codex.exe", exec)
	if err != nil {
		t.Errorf("DiscoverModels(codex): err = %v，期望 nil", err)
	}
	if want := cxhWantCodex(); !reflect.DeepEqual(got, want) {
		t.Errorf("DiscoverModels(codex) 结果不符\n实际: %#v\n期望: %#v", got, want)
	}
	if len(seen) != 1 {
		t.Fatalf("执行器应被调 1 次，实际 %d 次", len(seen))
	}
	c := seen[0]
	if c.Path != "cxh-codex.exe" {
		t.Errorf("执行器收到的 Path = %q，期望 %q", c.Path, "cxh-codex.exe")
	}
	if want := []string{"debug", "models"}; !reflect.DeepEqual(c.Args, want) {
		t.Errorf("执行器收到的 Args = %q，期望 %q", c.Args, want)
	}
	if c.Stdin != nil {
		t.Errorf("执行器收到的 Stdin = %q，期望 nil", c.Stdin)
	}
	if c.CloseStdinAfter != nil {
		t.Errorf("执行器收到的 CloseStdinAfter 非 nil，期望 nil")
	}
}
