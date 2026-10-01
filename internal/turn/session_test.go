package turn

// session.go 的测试面：IdentityFingerprint 的编解码往返、coldExpectedAt 的
// 判据、以及 identityDriftError 点名哪一项变了。这些都是纯函数，不需要
// Runner、不需要 store，直接调用同包内的未导出符号（这也是白盒测试要放在
// package turn 里的第二个理由：Runner 级别的漂移检测另在 runner_outcomes_test.go
// 与 cost_test.go 里用真实探针字节走整条 Ask 流程）。

import (
	"strings"
	"testing"
	"time"

	"council/internal/provider"
)

// TestPackUnpackFingerprintRoundTrip 钉住 09 D 节配套规则 4 的实现基础：
// packFingerprint 用「十进制长度 + 冒号 + 正文 + 分号」编码，选这个格式就是
// 为了不怕正文里出现分隔符本身。role 是自由文本，用户完全可能在里面写分号、
// 冒号，这里故意塞两个进去，证明长度前缀让解码不依赖内容字节。
func TestPackUnpackFingerprintRoundTrip(t *testing.T) {
	role := `只做审阅；不做决策: 见 04 第 2 节`
	model := "claude-fable-5-1"
	effort := "high"
	tier := string(provider.PermissionRead)

	fp := packFingerprint(role, model, effort, tier)
	parts, ok := unpackFingerprint(fp)
	if !ok {
		t.Fatalf("unpackFingerprint(%q) 返回 ok=false，期望能正确拆回", fp)
	}
	if len(parts) != 4 {
		t.Fatalf("拆出 %d 段，期望 4 段: %+v", len(parts), parts)
	}
	want := []string{role, model, effort, tier}
	for i, w := range want {
		if parts[i] != w {
			t.Errorf("第 %d 段 = %q，期望 %q", i, parts[i], w)
		}
	}
}

// TestUnpackFingerprintBadEncodingFailsClosed 覆盖 unpackFingerprint 自己的
// 注释里写的规矩：格式不对时 ok 为 false，调用方不得猜字段，只能退回
// 「指纹整体不符」——不把坏编码拆成四个碰巧能用的空串，那会让一次身份漂移
// 被显示成「什么都没变」。
//
// 空串不在这张负例表里：它按这套「十进制长度前缀 + 冒号 + 正文 + 分号」的
// 语法本身是良构的（零段），unpackFingerprint 对它返回 ok=true、parts=nil
// 完全正确——「格式合法但段数不对」是 fingerprintFields 的职责（它要求
// 恰好 4 段），不是 unpackFingerprint 的职责，下面单独一个用例钉住这条分工。
func TestUnpackFingerprintBadEncodingFailsClosed(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{name: "缺冒号", in: "4role;"},
		{name: "长度数字之后紧跟的不是分号", in: "4:role#"},
		{name: "声明的长度比实际正文长", in: "99:role;"},
		{name: "长度前缀是负数形状（减号不是数字）", in: "-1:x;"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parts, ok := unpackFingerprint(tc.in)
			if ok {
				t.Fatalf("unpackFingerprint(%q) 应当返回 ok=false，实际 ok=true parts=%+v", tc.in, parts)
			}
		})
	}

	// parseFingerprint（原 fingerprintFields）是 identityDriftError 报错时
	// 唯一的取值入口：坏编码（格式不对，或格式对但段数不对）必须让它整体判
	// !ok，而不是返回一个填了几个字段的零值结构体外加 ok=true。
	//
	// 第四个取值是**四段的旧格式**（规则文件那一步把段数从 4 加到 6）；
	// 前缀摘要那一步又把段数从 6 加到 7（fpFieldCount），库里现存的六段
	// 指纹同样要落进这张负例表——这一次没有配纯追加迁移，见 session.go
	// IdentityFingerprint 的注释：新算出来的第七段是真实摘要，不是空段，
	// 追加不出来。不许为了「兼容」让它判 ok=true：那会把段数校验从严格
	// 变成看情况。
	for _, bad := range []string{
		"这不是合法编码",
		"",
		"4:role;",
		packFingerprint("role-a", "model-a", "high", "read"),                                        // 四段：规则文件之前的格式
		packFingerprint("role-a", "model-a", "high", "read", "", "", "prefix-args", "one-too-many"), // 八段：多算了一段
		packFingerprint("role-a", "model-a", "high", "read", "", ""),                                // 六段：前缀摘要之前的格式
	} {
		parts, ok := parseFingerprint(bad)
		if ok {
			t.Errorf("parseFingerprint(%q) 应当返回 ok=false，实际 ok=true 且 %+v", bad, parts)
		}
	}
}

// TestIdentityDriftErrorNamesTheChangedField 覆盖 identityDriftError：
// 错误信息必须点得出具体哪一项变了（09 D 节配套规则 4：「不一致就
// ErrIdentityDrift 并点名哪一项变了」），而不是一句笼统的「变了」。
func TestIdentityDriftErrorNamesTheChangedField(t *testing.T) {
	// 夹具必须是**七段**的。规则文件那一步把段数从 4 加到 6，前缀摘要那一步
	// 又从 6 加到 7；段数不对时 parseFingerprint 整体判 !ok，于是
	// identityDriftError 只会回退成一句笼统的 fingerprint mismatch，本用例
	// 想验的「点名」根本走不到。直接用 IdentityFingerprint 造，不手写段数：
	// 段数再变时这里自动跟上。
	//
	// 第六个参数（命令行摘要）在本组用例里固定成同一个占位符：这一组只关心
	// role/model/effort/tier/规则五类字段各自能否被点名，不关心命令行本身，
	// 固定住它就不会被误算成一次意外的「命令行」漂移。命令行那一段单独有
	// TestIdentityDriftErrorNamesCommandLineChange 覆盖。
	const fixedDigest = "0/000000000000"
	fp := func(role, model, effort string, tier provider.PermissionTier, rules RulesBlocks) string {
		return IdentityFingerprint(role, model, effort, tier, rules, fixedDigest)
	}
	oldFP := fp("role-a", "model-a", "high", provider.PermissionRead, RulesBlocks{})

	cases := []struct {
		name       string
		newFP      string
		wantSubstr string
		notSubstr  string
	}{
		{
			name:       "只改 model",
			newFP:      fp("role-a", "model-b", "high", provider.PermissionRead, RulesBlocks{}),
			wantSubstr: `model "model-a" -> "model-b"`,
			notSubstr:  "role",
		},
		{
			name:       "只改 role",
			newFP:      fp("role-b", "model-a", "high", provider.PermissionRead, RulesBlocks{}),
			wantSubstr: `role "role-a" -> "role-b"`,
			notSubstr:  "model",
		},
		{
			name:       "只改 effort",
			newFP:      fp("role-a", "model-a", "low", provider.PermissionRead, RulesBlocks{}),
			wantSubstr: `effort "high" -> "low"`,
		},
		{
			name:       "只改权限档",
			newFP:      fp("role-a", "model-a", "high", provider.PermissionWorkspace, RulesBlocks{}),
			wantSubstr: `permission "read" -> "workspace"`,
		},
		{
			// 规格 11 第 5 节验收第 5 条：改的是一个 md 文件，
			// 报错里出现的必须是用户认得的「全局规则」，不是 role。
			// notSubstr 钉住 role 尤其重要：身份块与规则曾经是同一段文本，
			// 若哪天有人把规则塞回 role 段，这条会红。
			name:       "只改全局规则",
			newFP:      fp("role-a", "model-a", "high", provider.PermissionRead, RulesBlocks{Global: "新的全局规则"}),
			wantSubstr: `全局规则 "" -> "`,
			notSubstr:  "role",
		},
		{
			name:       "只改局部规则",
			newFP:      fp("role-a", "model-a", "high", provider.PermissionRead, RulesBlocks{Local: "新的局部规则"}),
			wantSubstr: `局部规则 "" -> "`,
			notSubstr:  "role",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := identityDriftError(oldFP, tc.newFP)
			if err == nil {
				t.Fatal("identityDriftError 返回 nil，期望非 nil")
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("错误信息 %q 里没有点名 %q", err.Error(), tc.wantSubstr)
			}
			if tc.notSubstr != "" && strings.Contains(err.Error(), tc.notSubstr) {
				t.Errorf("错误信息 %q 不该提到 %q（那一项没变）", err.Error(), tc.notSubstr)
			}
		})
	}
}

// TestIdentityDriftErrorNamesCommandLineChange 覆盖未决 #18 加的第七段：
// 只改「命令行」摘要（其余六段不变）时，identityDriftError 必须点名
// 「命令行」，不点名 role——这正是 6530839 那次该红而没红的情形：权限档
// 名字没变，适配器实际发出去的 flag 变了。
//
// 直接用 packFingerprint 手工造七段，不经 IdentityFingerprint：这里要的是
// 「第七段单独变化」这个前提本身可控，而不是通过某个真实 provider 的
// BuildTurn 间接凑出来。
func TestIdentityDriftErrorNamesCommandLineChange(t *testing.T) {
	oldFP := packFingerprint("role-a", "model-a", "high", "read", "", "", "12/3f2a1b9c4d5e")
	newFP := packFingerprint("role-a", "model-a", "high", "read", "", "", "15/9c1b7e0a2f44")

	err := identityDriftError(oldFP, newFP)
	if err == nil {
		t.Fatal("identityDriftError 返回 nil，期望非 nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, `命令行 "12/3f2a1b9c4d5e" -> "15/9c1b7e0a2f44"`) {
		t.Errorf("错误信息 %q 里没有点名命令行段的变化", msg)
	}
	if strings.Contains(msg, "role") {
		t.Errorf("错误信息 %q 不该提到 role（role 没有变）", msg)
	}
}

// TestIdentityDriftErrorBadFingerprintSaysMismatchNotEmptyFields 覆盖坏编码时
// identityDriftError 的降级路径：必须说「指纹整体不符」，而不是把两个坏指纹
// 拆成四个空串然后报告「role "" -> ""」——那种报告是错的：它意味着「什么都
// 没变」，而实际情况是我们连旧指纹长什么样都读不出来了。
func TestIdentityDriftErrorBadFingerprintSaysMismatchNotEmptyFields(t *testing.T) {
	err := identityDriftError("坏编码-旧", "坏编码-新")
	if err == nil {
		t.Fatal("identityDriftError 返回 nil，期望非 nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "fingerprint mismatch") {
		t.Errorf("坏编码时错误信息应当含 \"fingerprint mismatch\"，实际: %q", msg)
	}
	if strings.Contains(msg, `role "" -> ""`) {
		t.Errorf("坏编码不该被拆成四个空串再报告成\"什么都没变\": %q", msg)
	}
}

// TestIdentityFingerprintExcludesObservedModel 覆盖 09 D 节配套规则 4 的
// 反例：指纹只由 role/model/effort/tier 四项决定，ObservedModel（provider
// 实测回报的型号别名，例如 opus -> claude-fable-5-1）完全不参与——这四个
// 参数本身就是 IdentityFingerprint 的全部输入，函数签名里根本没有
// ObservedModel 这个位置，这里直接断言签名调用两次给同样四个参数得到同样的
// 指纹，防止将来有人往函数体里塞进第五个隐藏输入。
func TestIdentityFingerprintExcludesObservedModel(t *testing.T) {
	a := IdentityFingerprint("role", "opus", "high", provider.PermissionRead, RulesBlocks{}, "")
	b := IdentityFingerprint("role", "opus", "high", provider.PermissionRead, RulesBlocks{}, "")
	if a != b {
		t.Fatalf("同样的输入算出的指纹不同: %q vs %q", a, b)
	}
	// 规则也是输入的一部分，同样必须是确定性的：同一份规则两次调用相同，
	// 不同规则必须不同。后半条防的是「摘要算错了但恒定」这种假通过。
	c := IdentityFingerprint("role", "opus", "high", provider.PermissionRead, RulesBlocks{Global: "g", Local: "l"}, "")
	d := IdentityFingerprint("role", "opus", "high", provider.PermissionRead, RulesBlocks{Global: "g", Local: "l"}, "")
	if c != d {
		t.Fatalf("同一份规则两次算出的指纹不同: %q vs %q", c, d)
	}
	if c == a {
		t.Fatalf("规则从空变成非空，指纹却没变: %q", c)
	}
	// 两段不许互串：Global 与 Local 对调必须得到不同的指纹，
	// 否则 RulesBlocks 的两个字段被传反时没有任何一层会发现。
	if x := IdentityFingerprint("role", "opus", "high", provider.PermissionRead,
		RulesBlocks{Global: "l", Local: "g"}, ""); x == c {
		t.Fatalf("Global 与 Local 对调后指纹相同，两段互串了: %q", x)
	}
	// 第七段（前缀摘要）同样是真实输入，不是摆设：只改它，指纹必须跟着变，
	// 否则未决 #18 想防的那类走失（flag 变了、指纹没动）在这一层就已经堵不住。
	e := IdentityFingerprint("role", "opus", "high", provider.PermissionRead, RulesBlocks{}, "14/9c1b7e0a2f44")
	if e == a {
		t.Fatalf("只改命令行摘要，指纹却没变: %q", e)
	}
}

// TestColdExpectedAt 覆盖 09 G 节「TTL 过期不是失败，也不是重建」与
// session.go 的 CacheTTL 常量：新会话恒为 true；已有会话时按距
// lastTurnAt 是否超过 1 小时判断；lastTurnAt 是零值（状态表自己坏了）时
// 按「预报冷启动」处理，而不是假装还暖着。
func TestColdExpectedAt(t *testing.T) {
	base := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		now        time.Time
		lastTurnAt time.Time
		fresh      bool
		want       bool
	}{
		{name: "新会话恒为真，即使 lastTurnAt 是刚刚", now: base, lastTurnAt: base, fresh: true, want: true},
		{name: "59 分钟前，未过 TTL", now: base, lastTurnAt: base.Add(-59 * time.Minute), fresh: false, want: false},
		{name: "恰好 1 小时不算过期（严格大于）", now: base, lastTurnAt: base.Add(-CacheTTL), fresh: false, want: false},
		{name: "61 分钟前，已过 TTL", now: base, lastTurnAt: base.Add(-61 * time.Minute), fresh: false, want: true},
		{name: "lastTurnAt 是零值：状态表自己坏了，预报冷启动", now: base, lastTurnAt: time.Time{}, fresh: false, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := coldExpectedAt(tc.now, tc.lastTurnAt, tc.fresh)
			if got != tc.want {
				t.Errorf("coldExpectedAt(%v, %v, fresh=%v) = %v，期望 %v", tc.now, tc.lastTurnAt, tc.fresh, got, tc.want)
			}
		})
	}
}

// TestCacheTTLIsOneHour 钉住这个值本身：session.go 的注释说它来自 D-4 实测
// （claude 三轮的 cache_creation 全部落在 ephemeral_1h_input_tokens），
// 09 G 节的验收标尺、以及本包其它测试里"61 分钟"这类构造都依赖这个具体数字，
// 数值本身漂移了要在这里第一时间红。
func TestCacheTTLIsOneHour(t *testing.T) {
	if CacheTTL != time.Hour {
		t.Errorf("CacheTTL = %v，期望 1 小时（09 G 节：TTL 绝不进入重建判据，只作为价格预报）", CacheTTL)
	}
}
