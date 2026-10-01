// 本文件将来落到 internal/turn/session_test.go（与既有的 session_test.go 合并，
// 或作为独立文件并存——由实现方决定；本文件只新增 11a 契约第 3.2 节的用例）。
//
// 覆盖 11a_STEP8A_API_CONTRACT.md 3.2 节：RulesDigest、六段指纹、
// fingerprintFields 新返回值、identityDriftError 新增的两条比对（验收表
// 第 3、5 条）。只照契约写，不看 internal/turn/session.go 的实现——那是本片
// 改动、任务书明令不读的文件。packFingerprint / unpackFingerprint /
// identityDriftError 不在禁读名单内（前两个定义在 session.go 但契约 3.2
// 明写其行为且既有 session_test.go 已经把它们当白盒符号在用；
// identityDriftError 实际定义在 runner.go，任务书允许读的文件）。
package turn

import (
	"regexp"
	"strings"
	"testing"

	"council/internal/provider"
)

// ---------------------------------------------------------------------------
// RulesDigest（契约 3.2，验收表第 3 条）
// ---------------------------------------------------------------------------

// TestRulesDigestEmptyReturnsEmpty 钉住契约最容易被"顺手优化掉"的一条：
// 空串必须返回空串，不是空串的哈希。schema v3 的迁移（契约第 2 节）靠这条
// 对上：迁移追加的是字面的空段 "0:;0:;"，不是 RulesDigest("") 算出来的任何值——
// 只有 RulesDigest("") 恰好也是空串，两边才能相等。
func TestRulesDigestEmptyReturnsEmpty(t *testing.T) {
	if got := RulesDigest(""); got != "" {
		t.Fatalf("RulesDigest(\"\") = %q，期望空串", got)
	}
}

// tstDigestFormat 是 "<字节数>/<12 位小写十六进制>" 的形状。
var tstDigestFormat = regexp.MustCompile(`^[0-9]+/[0-9a-f]{12}$`)

// TestRulesDigestNonEmptyFormat 覆盖非空串的形状，并且长度前缀必须是**字节数**
// 而不是 rune 数——中文字符每个占三字节，用中文输入才分得出两者的差异。
func TestRulesDigestNonEmptyFormat(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "ASCII 短文本", text: "hello"},
		{name: "中文文本（字节数三倍于 rune 数，用来分辨长度前缀量的是哪一种）", text: "规则"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RulesDigest(tc.text)
			if !tstDigestFormat.MatchString(got) {
				t.Fatalf("RulesDigest(%q) = %q，形状不符合 <字节数>/<12位小写十六进制>", tc.text, got)
			}
			parts := strings.SplitN(got, "/", 2)
			wantLen := len(tc.text) // len() 对 string 取字节数，不是 rune 数
			if parts[0] != tstItoa(wantLen) {
				t.Errorf("RulesDigest(%q) 的长度前缀 = %s，期望 %d（字节数）", tc.text, parts[0], wantLen)
			}
		})
	}
}

// tstItoa 是本文件唯一需要的整数转字符串，避免为一个断言单独引入 strconv
// 再管理一次 import 分组。
func tstItoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// TestRulesDigestPureFunction 覆盖"再加一条：同样的输入两次调用结果相同"
// （它必须是纯函数，任务书原话）。
func TestRulesDigestPureFunction(t *testing.T) {
	const text = "一段规则正文，包含中文与 ASCII mixed content。"
	a := RulesDigest(text)
	b := RulesDigest(text)
	if a != b {
		t.Fatalf("同样的输入两次调用不相同: %q vs %q", a, b)
	}
}

// TestRulesDigestDistinctInputsDistinctOutputs 覆盖摘要的基本可用性：
// 不同正文不能压成同一个摘要（sha256 前 12 位十六进制发生碰撞的概率
// 在测试场景下可忽略不计，这条断言是"没有把输入直接丢弃"的下限检查，
// 不是密码学强度证明）。
func TestRulesDigestDistinctInputsDistinctOutputs(t *testing.T) {
	a := RulesDigest("规则版本一")
	b := RulesDigest("规则版本二")
	if a == b {
		t.Fatalf("两段不同的正文算出了相同的摘要: %q", a)
	}
}

// ---------------------------------------------------------------------------
// 指纹：六段变七段（11b 契约 3.3；本文件原覆盖四段变六段，随实现同步订正）
// ---------------------------------------------------------------------------

// TestParseFingerprintSevenSegmentsRoundTrip 覆盖 parseFingerprint（取代旧的
// fingerprintFields，签名从一串位置返回值改成 fingerprintParts 结构体，
// 理由见 11b 契约 3.3：位置传错编译器拦不住，传错的后果是 drift 报错点名
// 了错的字段）：func parseFingerprint(fp string) (fingerprintParts, bool)。
func TestParseFingerprintSevenSegmentsRoundTrip(t *testing.T) {
	role := "只做审阅；含分号: 与冒号"
	model := "claude-fable-5-1"
	effort := "high"
	tier := string(provider.PermissionRead)
	rulesGlobal := RulesDigest("全局规则正文")
	rulesLocal := RulesDigest("局部规则正文")
	prefixArgs := "14/9c1b7e0a2f44"

	fp := packFingerprint(role, model, effort, tier, rulesGlobal, rulesLocal, prefixArgs)

	got, ok := parseFingerprint(fp)
	if !ok {
		t.Fatalf("parseFingerprint(%q) 返回 ok=false，期望能正确拆回七段", fp)
	}
	want := fingerprintParts{
		Role: role, Model: model, Effort: effort, Tier: tier,
		RulesGlobal: rulesGlobal, RulesLocal: rulesLocal, PrefixArgs: prefixArgs,
	}
	if got != want {
		t.Errorf("parseFingerprint(%q) = %+v，期望 %+v", fp, got, want)
	}
}

// TestParseFingerprintRejectsLegacySixSegmentFingerprint 覆盖未决 #18 的
// 不迁移边界：升级前库里的六段指纹（前缀摘要那一步之前）必须被判 ok=false，
// 不能被误读成"前六段有效、第七段是零值"——契约把"段数不对"整体归为
// "指纹整体不符"，不允许调用方部分采信；这一次没有配纯追加迁移
// （11b 契约第 3.4 节：新算出来的第七段是真实摘要，不是空段，追加不出来）。
func TestParseFingerprintRejectsLegacySixSegmentFingerprint(t *testing.T) {
	legacyFP := packFingerprint("role-a", "model-a", "high", "read", "", "") // 只有六段
	if _, ok := parseFingerprint(legacyFP); ok {
		t.Fatalf("parseFingerprint(%q)（六段的旧指纹）应当返回 ok=false，实际 ok=true", legacyFP)
	}
}

// TestFingerprintFieldsRejectsLegacyFourSegmentFingerprint 覆盖再往前一版的
// 边界：四段指纹（规则文件那一步之前）同样必须被判 ok=false。
func TestFingerprintFieldsRejectsLegacyFourSegmentFingerprint(t *testing.T) {
	legacyFP := packFingerprint("role-a", "model-a", "high", "read") // 只有四段
	if _, ok := parseFingerprint(legacyFP); ok {
		t.Fatalf("parseFingerprint(%q)（四段的旧指纹）应当返回 ok=false，实际 ok=true", legacyFP)
	}
}

// TestIdentityFingerprintDeterministic 覆盖新签名
// IdentityFingerprint(role, model, effortFlagEmitted string, tier provider.PermissionTier, rules RulesBlocks, prefixArgsDigest string) string
// 仍然是纯函数：同样的六个参数两次调用必须逐字节相同。
func TestIdentityFingerprintDeterministic(t *testing.T) {
	rules := RulesBlocks{Global: "全局规则", Local: "局部规则"}
	a := IdentityFingerprint("role", "opus", "high", provider.PermissionRead, rules, "14/9c1b7e0a2f44")
	b := IdentityFingerprint("role", "opus", "high", provider.PermissionRead, rules, "14/9c1b7e0a2f44")
	if a != b {
		t.Fatalf("同样六个参数算出的指纹不同: %q vs %q", a, b)
	}
}

// TestIdentityFingerprintEncodesRulesAsDigestNotFullText 是契约 3.2 的核心
// 实现断言："实现即 packFingerprint(role, model, effortFlagEmitted,
// string(tier), RulesDigest(rules.Global), RulesDigest(rules.Local), ...)"。
// 存进指纹第 5、6 段的必须是 RulesDigest 算出来的摘要，不是规则原文——
// 原文进 identity_fp 这一列只会把库撑大而换不到表达力（3.2 节原话）。
func TestIdentityFingerprintEncodesRulesAsDigestNotFullText(t *testing.T) {
	const globalText = "一段足够长的全局规则原文，用来确认它不会被逐字塞进指纹列。"
	const localText = "对应的局部规则原文。"
	rules := RulesBlocks{Global: globalText, Local: localText}

	fp := IdentityFingerprint("role", "opus", "high", provider.PermissionRead, rules, "")

	parts, ok := parseFingerprint(fp)
	if !ok {
		t.Fatalf("parseFingerprint(%q) 返回 ok=false", fp)
	}
	if parts.RulesGlobal != RulesDigest(globalText) {
		t.Errorf("指纹里的全局规则段 = %q，期望等于 RulesDigest(globalText) = %q", parts.RulesGlobal, RulesDigest(globalText))
	}
	if parts.RulesGlobal == globalText {
		t.Error("指纹里的全局规则段竟然是原文本身，不是摘要")
	}
	if parts.RulesLocal != RulesDigest(localText) {
		t.Errorf("指纹里的局部规则段 = %q，期望等于 RulesDigest(localText) = %q", parts.RulesLocal, RulesDigest(localText))
	}
}

// TestIdentityFingerprintEmptyRulesEncodeAsEmptySegments 覆盖"升级不花钱"
// 的另一面：两段规则都为空时，指纹第 5、6 段必须都是空串（RulesDigest("")==""）。
func TestIdentityFingerprintEmptyRulesEncodeAsEmptySegments(t *testing.T) {
	fp := IdentityFingerprint("role", "opus", "high", provider.PermissionRead, RulesBlocks{}, "")
	parts, ok := parseFingerprint(fp)
	if !ok {
		t.Fatalf("parseFingerprint(%q) 返回 ok=false", fp)
	}
	if parts.RulesGlobal != "" || parts.RulesLocal != "" {
		t.Errorf("两段规则都为空时指纹的规则段应当都是空串，实际 RulesGlobal=%q RulesLocal=%q", parts.RulesGlobal, parts.RulesLocal)
	}
}

// ---------------------------------------------------------------------------
// identityDriftError 新增的两条比对（契约 3.2，验收表第 5 条）
// ---------------------------------------------------------------------------

// TestIdentityDriftErrorNamesGlobalRulesOnly 是验收表第 5 条本尊：只改全局
// 规则正文时，identityDriftError 产出的文本里要出现"全局规则"四个字，
// 且不出现"role"。role/model/effort/tier 与局部规则全部保持不变，
// 只有 RulesBlocks.Global 从 "A" 换成 "B"。
func TestIdentityDriftErrorNamesGlobalRulesOnly(t *testing.T) {
	oldFP := IdentityFingerprint("同一个 role", "opus", "high", provider.PermissionRead,
		RulesBlocks{Global: "A", Local: "不变的局部规则"}, "")
	newFP := IdentityFingerprint("同一个 role", "opus", "high", provider.PermissionRead,
		RulesBlocks{Global: "B", Local: "不变的局部规则"}, "")

	err := identityDriftError(oldFP, newFP)
	if err == nil {
		t.Fatal("identityDriftError 返回 nil，期望非 nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "全局规则") {
		t.Errorf("错误信息 %q 里没有出现\"全局规则\"", msg)
	}
	if strings.Contains(msg, "role") {
		t.Errorf("错误信息 %q 不该出现 \"role\"（role 没有变）", msg)
	}
	if strings.Contains(msg, "局部规则") {
		t.Errorf("错误信息 %q 不该出现\"局部规则\"（局部规则没有变）", msg)
	}
}

// TestIdentityDriftErrorNamesLocalRulesOnly 是上一条的镜像：只改局部规则时
// 出现"局部规则"，不出现"role"、不出现"全局规则"。
func TestIdentityDriftErrorNamesLocalRulesOnly(t *testing.T) {
	oldFP := IdentityFingerprint("同一个 role", "opus", "high", provider.PermissionRead,
		RulesBlocks{Global: "不变的全局规则", Local: "A"}, "")
	newFP := IdentityFingerprint("同一个 role", "opus", "high", provider.PermissionRead,
		RulesBlocks{Global: "不变的全局规则", Local: "B"}, "")

	err := identityDriftError(oldFP, newFP)
	if err == nil {
		t.Fatal("identityDriftError 返回 nil，期望非 nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "局部规则") {
		t.Errorf("错误信息 %q 里没有出现\"局部规则\"", msg)
	}
	if strings.Contains(msg, "role") {
		t.Errorf("错误信息 %q 不该出现 \"role\"（role 没有变）", msg)
	}
	if strings.Contains(msg, "全局规则") {
		t.Errorf("错误信息 %q 不该出现\"全局规则\"（全局规则没有变）", msg)
	}
}

// TestIdentityDriftErrorBothRulesChanged 覆盖两段规则同时改变时两条措辞都要出现。
func TestIdentityDriftErrorBothRulesChanged(t *testing.T) {
	oldFP := IdentityFingerprint("role", "opus", "high", provider.PermissionRead,
		RulesBlocks{Global: "A1", Local: "A2"}, "")
	newFP := IdentityFingerprint("role", "opus", "high", provider.PermissionRead,
		RulesBlocks{Global: "B1", Local: "B2"}, "")

	err := identityDriftError(oldFP, newFP)
	if err == nil {
		t.Fatal("identityDriftError 返回 nil，期望非 nil")
	}
	msg := err.Error()
	for _, want := range []string{"全局规则", "局部规则"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误信息 %q 里缺少 %q", msg, want)
		}
	}
}

// TestIdentityDriftErrorLegacyFingerprintSaysMismatch 覆盖：旧库里遗留的
// 历史格式指纹（理论上升级之后不该再出现，但 AuditDrift 的"指纹解不开"
// 分支就是为这种情形准备的）与一个当前七段指纹比较时，必须降级为
// "指纹整体不符"，不能把段数错的旧指纹错位拆成七段里的前几段。
// 两版历史格式各测一遍：规则文件那一步之前的四段、前缀摘要这一步之前的
// 六段——后者正是未决 #18 升级当轮真正会在库里遇到的那种旧指纹。
func TestIdentityDriftErrorLegacyFingerprintSaysMismatch(t *testing.T) {
	newFP := IdentityFingerprint("role-a", "model-a", "high", provider.PermissionRead, RulesBlocks{}, "")

	cases := []struct {
		name  string
		oldFP string
	}{
		{name: "四段（规则文件之前）", oldFP: packFingerprint("role-a", "model-a", "high", "read")},
		{name: "六段（前缀摘要之前，本次升级当轮真会遇到的形状）", oldFP: packFingerprint("role-a", "model-a", "high", "read", "", "")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := identityDriftError(tc.oldFP, newFP)
			if err == nil {
				t.Fatal("identityDriftError 返回 nil，期望非 nil")
			}
			if !strings.Contains(err.Error(), "fingerprint mismatch") {
				t.Errorf("旧指纹参与比较时错误信息应当含 \"fingerprint mismatch\"，实际: %q", err.Error())
			}
		})
	}
}
