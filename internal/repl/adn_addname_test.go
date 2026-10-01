package repl

// adn_addname_test.go 是 AD1（00_DECISIONS.md #49：/add 按名字加识别出的模型）在 repl 包的验收，契约 §2.1。
// 装置只读复用 mdx_addmodel_fixture_test.go 与 cxh_fixture_test.go（mdxNewRig、mdxNewRigOpts、mdxRigOpts、
// mdxRig、mdxResp、mdxWantLines、mdxHasLine、mdxReplaceOnce、mdxConfigTemplate、mdxGrokFixture、mdxGrokPath、cxhNewRig、
// cxhConfigTemplate、cxhCodexPath、cxhCodexFixture）。repl 的用例不起任何进程：识别用 mdxExec（r.discover），登录状态用
// 本文件的 adnLoginExec。开关用 SetAddByName 打开；不调它的装置就是开关关着（N1 的对照）。v1.1（契约变更记录）
// 加了 N16、N17 与若干子用例；v1.2 加了 N22-N24 与若干子用例，改了 N9、N14 的一条与 N15 alias_same_brain；
// v1.3 加了 N25、N14 的一条、N22 的三个子用例与 one_absent_config 的成员记录断言（夹具多一行 display_name）；
// 【判别力】里的变异编号与杀手是 v1.3 原型上的实测。

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/provider"
)

// adnCodexOut 是合成的 codex debug models 输出（不是真输出）：gpt-6.1-sol 照 Griffin 这次敲的名字取；
// gpt-5.5 在 cxhConfigTemplate 里已以 high 配成 gpt-5.5-high（拍快照时已在配置里、没有编号）；
// adn-x（只有 high）与 adn-x-high（没有强度）的 ID 都是 adn-x-high，是同一家里的歧义；
// custom/ 那一条不进快照。
const adnCodexOut = `{"models":[` +
	`{"slug":"gpt-6.1-sol","display_name":"GPT-6.1-Sol","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"},` +
	`{"slug":"gpt-5.5","display_name":"GPT-5.5","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"},` +
	`{"slug":"adn-x","display_name":"Adn X","supported_reasoning_levels":[{"effort":"high"}],"visibility":"list"},` +
	`{"slug":"adn-x-high","display_name":"Adn X High","visibility":"list"},` +
	`{"slug":"custom/adn-cpa","display_name":"Adn CPA","visibility":"list"}` +
	`]}` + "\n"

// adnCodexIDFirst 只给 N6 union_second_row 用（不动 adnCodexOut，免得 +N 的编号跟着变）：adn-x-high（没有强度）
// 排在 adn-x（只有 high）之前，两行的 ID 都是 adn-x-high；两个词写法里强度取几行的并集，要看到第二行才认得 high。
const adnCodexIDFirst = `{"models":[` +
	`{"slug":"adn-x-high","display_name":"Adn X High","visibility":"list"},` +
	`{"slug":"adn-x","display_name":"Adn X","supported_reasoning_levels":[{"effort":"high"}],"visibility":"list"}` +
	`]}` + "\n"

// adnCodexSol61 只给 N22 prefix_other_model 用（不动 adnCodexOut）：gpt-6.1 与 gpt-6.1-sol 是两个模型，前者的名字
// 是后者 ID 的前缀。按 id 前缀判「同一个模型已有成员」的实现会把 /add gpt-6.1 挡在在场的 gpt-6.1-sol-xhigh 上。
const adnCodexSol61 = `{"models":[` +
	`{"slug":"gpt-6.1-sol","display_name":"GPT-6.1-Sol","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"},` +
	`{"slug":"gpt-6.1","display_name":"GPT-6.1","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"},{"effort":"xhigh"}],"visibility":"list"}` +
	`]}` + "\n"

// adnIdeoSpace 是全角空格 U+3000 的 UTF-8 字节，写成十六进制转义（编辑链路会把 \u 转义还原成真字符）。
const adnIdeoSpace = "\xe3\x80\x80"

// adnGrokExtra 是 MD1 的 grok 夹具末尾多两个模型：gpt-6.1-sol（与 codex 的模型名相同）、gpt-5.5（与 codex 里
// 已配成 agent、没有编号的那一行相同），跨 provider 的歧义用。
const adnGrokExtra = mdxGrokFixture + "  - gpt-6.1-sol\n  - gpt-5.5\n"

// 按名字加的几种说法（契约 §3，逐字）。
const (
	adnDiscovering = "正在识别模型: codex, grok"
	adnCanceled    = "识别被取消；下次 /add 会重新识别"
)

// adnNotFound 是配置里与识别快照里都对不上时的那一行。
func adnNotFound(name string) string {
	return "配置里和识别结果里都没有 " + name + "；不带参数的 /add 列出配置里的成员与识别出的模型，识别表里的模型可以直接 /add 它的 ID"
}

// adnOldNotFound 是开关关着时（AD1 之前）的那一行。
func adnOldNotFound(name string) string {
	return "配置里没有可加入的 " + name + "，不带参数的 /add 能列出可加的"
}

// adnOneAtATime 是多段里有一段要按识别结果加时，那一段的说法。
func adnOneAtATime(name string) string {
	return name + " 是识别出的模型，识别出的模型一次加一个：单独 /add " + name
}

const adnUsage = "用法: /add <识别表里的 ID 或模型名> [--id <新 id>] [--effort <强度>]；带 --id 或 --effort 时只能加一个"

// 写进配置的几段（契约 §2.1 冻结；与 /add +编号 写出的字节相同，N2 的 plus 子测试钉这一条）。
const (
	adnBlockSolXhigh = "\n# council /add 追加：codex 识别出的 gpt-6.1-sol（codex 目录）\n[agents.\"gpt-6.1-sol-xhigh\"]\ndisplay_name = \"GPT-6.1-Sol xhigh\"\nprovider = \"codex\"\nmodel = \"gpt-6.1-sol\"\neffort = \"xhigh\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	adnBlockSolHigh  = "\n# council /add 追加：codex 识别出的 gpt-6.1-sol（codex 目录）\n[agents.\"gpt-6.1-sol-high\"]\ndisplay_name = \"GPT-6.1-Sol\"\nprovider = \"codex\"\nmodel = \"gpt-6.1-sol\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	adnBlockSolLow   = "\n# council /add 追加：codex 识别出的 gpt-6.1-sol（codex 目录）\n[agents.\"gpt-6.1-sol-low\"]\ndisplay_name = \"GPT-6.1-Sol low\"\nprovider = \"codex\"\nmodel = \"gpt-6.1-sol\"\neffort = \"low\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	adnBlockSolX     = "\n# council /add 追加：codex 识别出的 gpt-6.1-sol（codex 目录）\n[agents.sol-x]\ndisplay_name = \"GPT-6.1-Sol xhigh\"\nprovider = \"codex\"\nmodel = \"gpt-6.1-sol\"\neffort = \"xhigh\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	adnBlock55Xhigh  = "\n# council /add 追加：codex 识别出的 gpt-5.5（codex 目录）\n[agents.\"gpt-5.5-xhigh\"]\ndisplay_name = \"GPT-5.5 xhigh\"\nprovider = \"codex\"\nmodel = \"gpt-5.5\"\neffort = \"xhigh\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	adnBlockGrok46   = "\n# council /add 追加：grok 识别出的 grok-4.6（本地配置）\n[agents.\"grok-4.6\"]\ndisplay_name = \"Grok 4.6\"\nprovider = \"grok\"\nmodel = \"grok-4.6\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	adnBlockGrok46A  = "\n# council /add 追加：grok 识别出的 grok-4.6（本地配置）\n[agents.\"grok-4.6-a\"]\ndisplay_name = \"Grok 4.6\"\nprovider = \"grok\"\nmodel = \"grok-4.6\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	adnBlockSol      = "\n# council /add 追加：codex 识别出的 gpt-6.1-sol（codex 目录）\n[agents.sol]\ndisplay_name = \"GPT-6.1-Sol\"\nprovider = \"codex\"\nmodel = \"gpt-6.1-sol\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
	adnBlock61High   = "\n# council /add 追加：codex 识别出的 gpt-6.1（codex 目录）\n[agents.\"gpt-6.1-high\"]\ndisplay_name = \"GPT-6.1\"\nprovider = \"codex\"\nmodel = \"gpt-6.1\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n"
)

// adnPresent 是没给强度的名字对上的模型已有成员在场时的那一行；hint 为真时带「要另加一档强度」的后半句。
func adnPresent(name, ids string, hint bool) string {
	s := name + " 对上的模型已经有成员在当前会话里：" + ids
	if hint {
		s += "；要另加一档强度，用 /add " + name + " <强度> 或 --effort <强度>"
	}
	return s
}

// adnConfigured 是没给强度的名字对上的模型在配置里有不止一个成员、都不在场时的那一行。
func adnConfigured(name, ids string, hint bool) string {
	s := name + " 对上的模型在配置里有这几个成员：" + ids + "；用 /add <id> 加入其中一个"
	if hint {
		s += "，或用 /add " + name + " <强度> 带强度新加一档"
	}
	return s
}

// adnFlagsConfig 是带 --id 或 --effort、名字在配置里对上一个 agent 时的那一行。
func adnFlagsConfig(name, id string) string {
	return name + " 对上的是配置里的 agent " + id + "，--id 与 --effort 只用于加识别出的模型；直接 /add " + id
}

// adnJoinedNoWrite 敲 cmd，断言屏幕整行等于 want（最后一行是 id 的加入那一行），而且没写配置：文件字节、备份个数
// 不变，id 在场。
func adnJoinedNoWrite(t *testing.T, g *mdxRig, cmd, id string, want []string) {
	t.Helper()
	orig := g.readFile()
	baks := g.bakNames()
	mdxWantLines(t, cmd, g.run(cmd), append(want, id+" 已加入会话 "+g.conv.ID))
	if got := g.readFile(); string(got) != string(orig) {
		t.Errorf("%s：配置文件不该变\n实际 %q", cmd, got)
	}
	if got := g.bakNames(); !reflect.DeepEqual(got, baks) {
		t.Errorf("%s：不该多出备份，之前 %q 之后 %q", cmd, baks, got)
	}
	if !mdxHasLine(g.activeIDs(), id) {
		t.Errorf("%s：%s 应当在场，在场的是 %q", cmd, id, g.activeIDs())
	}
}

// adnNewRig 是 CXH 的配置（codex、grok 两家；grok-4.7、gpt-5.5-high 已配成 agent）加 adnCodexOut；
// on 为真时打开开关。
func adnNewRig(t *testing.T, on bool) *mdxRig {
	t.Helper()
	g := cxhNewRig(t, cxhConfigTemplate, adnCodexOut)
	if on {
		g.r.SetAddByName(true)
	}
	return g
}

// adnAdded 敲 cmd，断言这一次写进了一段、加入了 id：屏幕整行相等（discover 为真时第一行是
// 「正在识别模型」），文件等于原文加 block，新出现恰好一个备份、字节等于原文，id 在场。
func adnAdded(t *testing.T, g *mdxRig, cmd, id, block string, discover bool) {
	t.Helper()
	orig := g.readFile()
	baks := g.bakNames()
	lines := g.run(cmd)
	bak := g.newBak(baks)
	var want []string
	if discover {
		want = append(want, adnDiscovering)
	}
	mdxWantLines(t, cmd, lines, append(want, g.joinedLines(id, bak)...))
	if got := string(g.readFile()); got != string(orig)+block {
		t.Errorf("%s：配置文件不等\n实际 %q\n期望 %q", cmd, got, string(orig)+block)
	}
	if b, err := os.ReadFile(bak); err != nil || string(b) != string(orig) {
		t.Errorf("%s：备份 %s 应当是原文（err %v）", cmd, bak, err)
	}
	if !mdxHasLine(g.activeIDs(), id) {
		t.Errorf("%s：%s 应当在场，在场的是 %q", cmd, id, g.activeIDs())
	}
}

// adnRefused 敲 cmd，断言屏幕整行等于 want，而且什么都没变：配置文件字节、备份个数、在场成员。
func adnRefused(t *testing.T, g *mdxRig, cmd string, want []string) {
	t.Helper()
	orig := g.readFile()
	baks := g.bakNames()
	active := g.activeIDs()
	mdxWantLines(t, cmd, g.run(cmd), want)
	if got := g.readFile(); string(got) != string(orig) {
		t.Errorf("%s：配置文件不该变\n实际 %q", cmd, got)
	}
	if got := g.bakNames(); !reflect.DeepEqual(got, baks) {
		t.Errorf("%s：不该多出备份，之前 %q 之后 %q", cmd, baks, got)
	}
	if got := g.activeIDs(); !reflect.DeepEqual(got, active) {
		t.Errorf("%s：在场成员不该变，之前 %q 之后 %q", cmd, active, got)
	}
}

// adnLoginExec 是登录状态执行器：只数次数，一律答 codex 已登录（ChatGPT）。
type adnLoginExec struct{ n int }

func (e *adnLoginExec) run(ctx context.Context, c provider.DiscoveryCommand) (int, []byte, []byte, error) {
	e.n++
	return 0, nil, []byte("Logged in using ChatGPT\n"), nil
}

// adnSeedMember 往当前会话塞一个在场、却不在配置里的成员，搭 TestUx2rAddB3GoneFromConfigMember 那种「从配置
// 删掉、仍是成员」的状态。ux2r 的 seedActive 挂在 ux2rFix 上，mdxRig 用不了；这里照 chxAddRig.join 的写法，
// 经 conversation.Service.Join 直接塞。不经 /join：/join 与 /add 是同一个 handler，只加配置里的 agent。
func adnSeedMember(t *testing.T, g *mdxRig, id, prov string) {
	t.Helper()
	p := conversation.Participant{ConvID: g.conv.ID, AgentID: id, Provider: prov, Active: true, JoinedAt: time.Now().UTC()}
	if err := g.svc.Join(g.ctx, g.conv.ID, p); err != nil {
		t.Fatalf("装置：塞入成员 %s 失败: %v", id, err)
	}
}

// adnNoDiscovery 断言执行器一次都没被调过（配置里命中时不识别）。
func adnNoDiscovery(t *testing.T, g *mdxRig, what string) {
	t.Helper()
	if n := g.exec.count(); n != 0 {
		t.Errorf("%s：配置里命中时不该识别，执行器被调了 %d 次", what, n)
	}
}

// TestAdnSwitchOffUnchanged 是 N1：开关零值是关；关着时 /add 的说法与 AD1 之前逐字相同、不识别、不写文件。
// 同一个配置、同一条命令，打开开关的装置是正对照：它识别了两家、写进了配置。
// 【判别力】V40（关着时也换成新说法）、V1（SetAddByName 不记下参数）；正对照 on 对 V38（cmdAdd 不接 cmdAddByName）、V32（从不自动识别）红。
func TestAdnSwitchOffUnchanged(t *testing.T) {
	if r := New(nil, nil, nil, nil, nil, RulesContext{}); r.addByName {
		t.Fatalf("New 出来的 REPL 开关应当是关")
	}
	r := New(nil, nil, nil, nil, nil, RulesContext{})
	r.SetAddByName(true)
	if !r.addByName {
		t.Errorf("SetAddByName(true) 之后开关应当是开")
	}
	r.SetAddByName(false)
	if r.addByName {
		t.Errorf("SetAddByName(false) 之后开关应当是关")
	}

	g := adnNewRig(t, false)
	adnRefused(t, g, "/add gpt-6.1-sol-xhigh", []string{adnOldNotFound("gpt-6.1-sol-xhigh")})
	adnRefused(t, g, "/add gpt-6.1-sol xhigh", []string{adnOldNotFound("gpt-6.1-sol"), adnOldNotFound("xhigh")})
	adnRefused(t, g, "/add gpt-6.1-sol --effort xhigh", []string{adnOldNotFound("gpt-6.1-sol"), adnOldNotFound("--effort"), adnOldNotFound("xhigh")})
	if n := g.exec.count(); n != 0 {
		t.Errorf("开关关着时不该识别，执行器被调了 %d 次", n)
	}
	if g.r.modelSnap != nil {
		t.Errorf("开关关着时不该有快照")
	}

	on := adnNewRig(t, true)
	adnAdded(t, on, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	if n := on.exec.count(); n != 2 {
		t.Errorf("正对照：开关打开时应当识别 codex、grok 两家，执行器被调了 %d 次", n)
	}
}

// TestAdnAddByID 是 N2：Griffin 这次的第二条命令。新会话里第一次就敲 ID 形式，先识别再写配置、加入；
// 与 /add +编号 --effort xhigh 写出的字节、打出的两行相同（plus）；/join 是同一个 handler（join）；
// 名字不分大小写，写进去的 id 是建议的小写（fold）。
// 【判别力】fold 对 V3（ID 形式按字节比）；id、join、fold 对 V17（名字里的强度不用）、V37（只认默认档的 ID）、V30、V32；plus 是对照：它与 id 是同一份期望，V47（+编号 丢掉 --effort）只有它与 MD1、CXH 的用例杀得到。
func TestAdnAddByID(t *testing.T) {
	t.Run("id", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
		if n := g.exec.count(); n != 2 {
			t.Errorf("应当识别 codex、grok 各一次，执行器被调了 %d 次", n)
		}
	})
	t.Run("plus", func(t *testing.T) {
		g := adnNewRig(t, true)
		g.run("/add")
		adnAdded(t, g, "/add +1 --effort xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, false)
	})
	t.Run("join", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/join gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	})
	t.Run("fold", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add GPT-6.1-Sol-XHIGH", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	})
}

// TestAdnTwoWords 是 N3：Griffin 这次的第一条命令的两种敲法。/add gpt-6.1-sol xhigh 与 ID 形式写出相同的字节；
// 他真机上敲的 xigh 是敲错的强度（裁决 (a)）：只打 +编号 那条路现成的「支持的强度是……，没有 xigh」，不写（typo）；
// 第二个词区分大小写，XHIGH 同样是敲错的强度（upper）；ID 形式已带强度再跟第二个词，一档强度相同也拒、敲错的也拒
// （id_then_effort、id_then_other、id_then_typo）；第一段对上多行时列候选（ambiguous_typo）；第二个词在快照里也是一个
// 模型时照多段说「一次加一个」（second_is_model）；第二个词是编号时是配置里的一段，不当强度（digit）；没有强度可选的
// grok 行后面跟 high 照多段（no_efforts：强度那一句只给有强度的行）；第二个词在配置里命中（是某个候选的前缀）时不当强度
// （effort_is_config_prefix）；第一个词在配置里按全名命中时不是两个词的写法（config_first）。
// 【判别力】model_effort 对 V18（第二个词不当强度）、V10（不认两个词）；typo、upper、id_then_typo、ambiguous_typo 对 V58（敲错的强度那一支去掉）；upper 另对 V48；second_is_model 对 V59；no_efforts 对 V60、V39；digit、effort_is_config_prefix、no_efforts、second_is_model 对 V27（不说「一次加一个」）；digit、effort_is_config_prefix 对 V12；id_then_* 对 V14；config_first 对 V13、V63、V39；ambiguous_typo 对 V9。
func TestAdnTwoWords(t *testing.T) {
	t.Run("model_effort", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add gpt-6.1-sol xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	})
	for _, c := range []struct {
		name, cmd string
		want      []string
	}{
		{"upper", "/add gpt-6.1-sol XHIGH", []string{adnDiscovering, "gpt-6.1-sol 支持的强度是 low|medium|high|xhigh，没有 XHIGH"}},
		{"digit", "/add gpt-6.1-sol 2", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol")}},
		{"typo", "/add gpt-6.1-sol xigh", []string{adnDiscovering, "gpt-6.1-sol 支持的强度是 low|medium|high|xhigh，没有 xigh"}},
		{"id_then_effort", "/add gpt-6.1-sol-xhigh xhigh", []string{adnDiscovering, "gpt-6.1-sol-xhigh 已经带着强度 xhigh，不能再给强度 xhigh"}},
		{"id_then_other", "/add gpt-6.1-sol-xhigh low", []string{adnDiscovering, "gpt-6.1-sol-xhigh 已经带着强度 xhigh，不能再给强度 low"}},
		{"id_then_typo", "/add gpt-6.1-sol-xhigh xigh", []string{adnDiscovering, "gpt-6.1-sol-xhigh 已经带着强度 xhigh，不能再给强度 xigh"}},
		{"ambiguous_typo", "/add adn-x-high xigh", []string{adnDiscovering, "adn-x-high 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:", "  codex   adn-x  +2", "  codex   adn-x-high  +3"}},
		{"second_is_model", "/add gpt-6.1-sol grok-4.6", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol"), adnOneAtATime("grok-4.6")}},
		{"no_efforts", "/add grok-4.6 high", []string{adnDiscovering, adnOneAtATime("grok-4.6"), adnNotFound("high")}},
		{"config_first", "/add gpt-5.5-high xhigh", []string{adnDiscovering, adnNotFound("xhigh")}},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := adnNewRig(t, true)
			adnRefused(t, g, c.cmd, c.want)
		})
	}
	t.Run("effort_is_config_prefix", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[agents.high-adn]\nprovider = 'grok'\nmodel = 'grok-4.5'\n", adnCodexOut)
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add gpt-6.1-sol high", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol")})
	})
}

// TestAdnModelForm 是 N4：模型名形式与 --effort、--id；Griffin 这次的第三条命令（id_high，识别表 ID 列原样）；ID 形式与 --effort 的对账；强度区分大小写；
// grok 不能带 --effort；对上多行时带 --effort 也列候选（ambiguous_with_effort）；codex 里没有强度、也没有歧义的一行
// 带 --effort 打「没有可选的强度」（no_efforts_codex）。
// 【判别力】effort_flag、flag_first、unsupported、effort_case、grok_effort、ambiguous_with_effort、no_efforts_codex、id_flag、id_same_effort、id_other_effort 对 V21（不认参数）、V26（带参数时不识别）；id_flag 另对 V17；id_same_effort 对 V15；id_other_effort 对 V16；default_effort、grok、effort_flag、flag_first、unsupported、effort_case、grok_effort、ambiguous_with_effort、no_efforts_codex 对 V35（ID 对上之后不 continue）；effort_case 对 V48（强度不分大小写）；ambiguous_with_effort 对 V9；id_high 是 Griffin 的原样命令，只有通用的几条杀它（high 是默认档，V17、V37 下结果相同）。codex 的模型名现在按 SuggestedID("") 对上，模型名形式的 Model 那一支由 N15 的 claude_model 担保（V8、V2）。
func TestAdnModelForm(t *testing.T) {
	for _, c := range []struct {
		name, cmd, id, block string
	}{
		{"default_effort", "/add gpt-6.1-sol", "gpt-6.1-sol-high", adnBlockSolHigh},
		{"id_high", "/add gpt-6.1-sol-high", "gpt-6.1-sol-high", adnBlockSolHigh},
		{"effort_flag", "/add GPT-6.1-SOL --effort low", "gpt-6.1-sol-low", adnBlockSolLow},
		{"flag_first", "/add --effort low gpt-6.1-sol", "gpt-6.1-sol-low", adnBlockSolLow},
		{"id_flag", "/add gpt-6.1-sol-xhigh --id sol-x", "sol-x", adnBlockSolX},
		{"id_same_effort", "/add gpt-6.1-sol-xhigh --effort xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh},
		{"grok", "/add grok-4.6", "grok-4.6", adnBlockGrok46},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := adnNewRig(t, true)
			adnAdded(t, g, c.cmd, c.id, c.block, true)
		})
	}
	for _, c := range []struct {
		name, cmd string
		want      []string
	}{
		{"id_other_effort", "/add gpt-6.1-sol-xhigh --effort high", []string{adnDiscovering, "gpt-6.1-sol-xhigh 已经带着强度 xhigh，不能再给强度 high"}},
		{"unsupported", "/add gpt-6.1-sol --effort ultra", []string{adnDiscovering, "gpt-6.1-sol 支持的强度是 low|medium|high|xhigh，没有 ultra"}},
		{"effort_case", "/add gpt-6.1-sol --effort HIGH", []string{adnDiscovering, "gpt-6.1-sol 支持的强度是 low|medium|high|xhigh，没有 HIGH"}},
		{"grok_effort", "/add grok-4.6 --effort high", []string{adnDiscovering, "grok 的强度不在 council 里选（grok 写在它自己的配置里，agy 写在模型名里），不能带 --effort"}},
		{"ambiguous_with_effort", "/add adn-x-high --effort high", []string{adnDiscovering, "adn-x-high 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:", "  codex   adn-x  +2", "  codex   adn-x-high  +3"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := adnNewRig(t, true)
			adnRefused(t, g, c.cmd, c.want)
		})
	}
	t.Run("no_efforts_codex", func(t *testing.T) {
		// CXH 的整份夹具：cxh-bell 是 codex 里没有强度、也没有歧义的一行（只按 SuggestedID("") 对上一行）。
		g := cxhNewRig(t, cxhConfigTemplate, cxhCodexFixture())
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add cxh-bell --effort high", []string{adnDiscovering, "cxh-bell 没有可选的强度，不能带 --effort"})
	})
}

// TestAdnConfiguredRow 是 N5（16a open item (c)）：gpt-5.5 拍快照时已在配置里（gpt-5.5-high），没有编号，
// 照样能按 ID 形式加另一档（xhigh）；/add gpt-5.5 是配置里 gpt-5.5-high 的前缀，照配置加入、不识别（prefix）；
// 大写的 GPT-5.5-HIGH 配置里对不上（配置按字节比），到快照里是同一个模型同一档，照 +编号 的同脑子那一句（same_brain）。
// refusal_*（W1 的 major）：开关打开时配置里的拒绝型命中照旧只打配置的那一句、不识别。
// 【判别力】other_effort、same_brain 对 V5（只找有编号的行）；same_brain 另对 V3；prefix 对 V29（每段都命中也插手）；refusal_* 对 V49（anyMiss 只看 id）、V51（共用判据只看 id）、V29、V41，正对照那一次加入另对 V17、V37。
func TestAdnConfiguredRow(t *testing.T) {
	t.Run("other_effort", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add gpt-5.5-xhigh", "gpt-5.5-xhigh", adnBlock55Xhigh, true)
	})
	t.Run("prefix", func(t *testing.T) {
		g := adnNewRig(t, true)
		mdxWantLines(t, "/add gpt-5.5", g.run("/add gpt-5.5"), []string{"gpt-5.5-high 已加入会话 " + g.conv.ID})
		if n := g.exec.count(); n != 0 {
			t.Errorf("配置里命中时不该识别，执行器被调了 %d 次", n)
		}
	})
	t.Run("same_brain", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add GPT-5.5-HIGH", []string{adnDiscovering, "配置里已经有同一个模型、同一强度的 agent gpt-5.5-high，直接 /add gpt-5.5-high"})
	})
	// 开关打开时，配置里的拒绝型命中（超出范围的编号、前缀歧义、总结成员、只是成员不在配置里）照旧只打配置的那一句、
	// 不识别。每条一个新装置：执行器 0 次与「没有正在识别模型那一行」只在还没拍快照时有判别力。同一装置接着按名字加
	// 一个配置里没有的模型，是正对照：它识别了、写了。
	refusals := []struct {
		name, cmd, want string
		rig             func(t *testing.T) *mdxRig
	}{
		{"refusal_out_of_range", "/add 99", "编号 99 超出范围（1-2）", func(t *testing.T) *mdxRig { return adnNewRig(t, true) }},
		{"refusal_ambiguous", "/add g", "g 有歧义，可能是：gpt-5.5-high, grok-4.7", func(t *testing.T) *mdxRig { return adnNewRig(t, true) }},
		{"refusal_summary", "/add grok-4.7", "grok-4.7 是总结成员（config.toml 的 [summary]），不用 /add：多个成员回复之后它会自己加入做总结", func(t *testing.T) *mdxRig {
			// cxhNewRig 的配置末尾加 [summary]：cxh 与 mdx 的配置里都没有总结成员。
			g := cxhNewRig(t, cxhConfigTemplate+"\n[summary]\nagent = 'grok-4.7'\n", adnCodexOut)
			g.r.SetAddByName(true)
			return g
		}},
		{"refusal_member_only", "/add gpt-6.1-sol", "配置里没有 agent gpt-6.1-sol，用 /agents 看看有哪些", func(t *testing.T) *mdxRig {
			// 成员 id 恰好等于识别表里的模型名：把拒绝当成没命中的实现会到快照里对上它、写进配置。
			g := adnNewRig(t, true)
			adnSeedMember(t, g, "gpt-6.1-sol", "codex")
			return g
		}},
	}
	for _, c := range refusals {
		t.Run(c.name, func(t *testing.T) {
			g := c.rig(t)
			adnRefused(t, g, c.cmd, []string{c.want})
			adnNoDiscovery(t, g, c.cmd)
			adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
		})
	}
}

// TestAdnAmbiguous 是 N6：一个名字对上快照里的多行时整条拒绝、逐行列出候选，不猜。同一家（adn-x 的 high 档与
// adn-x-high 的 ID 都是 adn-x-high）、跨两家（codex 的模型名 gpt-6.1-sol 与 grok 的 gpt-6.1-sol）；两个词的写法里
// 第一段有歧义时照样列候选（two_words）；第二个词的强度取几行的并集（union_second_row）；第二个词是敲错的强度、有强度的那一行
// 排在第二时同样列候选（typo_second_row，v1.2）。
// 【判别力】same_provider、two_words、cross_provider、no_number、union_second_row、typo_second_row 对 V9（取第一行）、V35；two_words、union_second_row、typo_second_row 另对 V10；union_second_row 对 V57（只看第一行的强度）、V62（不认配置前缀命中的第一段）；typo_second_row 对 V67（只看第一行有没有强度可选）、V58；no_number 对 V36（没有编号也带 +0）、V5；cross_provider 另对 V31。
func TestAdnAmbiguous(t *testing.T) {
	t.Run("same_provider", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add adn-x-high", []string{adnDiscovering, "adn-x-high 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:", "  codex   adn-x  +2", "  codex   adn-x-high  +3"})
	})
	t.Run("two_words", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add adn-x-high high", []string{adnDiscovering, "adn-x-high 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:", "  codex   adn-x  +2", "  codex   adn-x-high  +3"})
	})
	t.Run("cross_provider", func(t *testing.T) {
		g := adnNewRig(t, true)
		g.exec.set(mdxGrokPath, mdxResp{out: []byte(adnGrokExtra)})
		adnRefused(t, g, "/add gpt-6.1-sol", []string{adnDiscovering, "gpt-6.1-sol 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:", "  codex   gpt-6.1-sol  +1", "  grok    gpt-6.1-sol  +9"})
		// 带强度的 ID 只对上 codex 那一行，照常加入（正对照：同一装置、同一份快照）。
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, false)
	})
	t.Run("no_number", func(t *testing.T) {
		// codex 的 gpt-5.5 拍快照时已在配置里，没有编号，那一行不带 +N。GPT-5.5 大写：配置按字节比对不上。
		g := adnNewRig(t, true)
		g.exec.set(mdxGrokPath, mdxResp{out: []byte(adnGrokExtra)})
		adnRefused(t, g, "/add GPT-5.5", []string{adnDiscovering, "GPT-5.5 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:", "  codex   gpt-5.5", "  grok    gpt-5.5  +10"})
	})
	t.Run("union_second_row", func(t *testing.T) {
		// 两个词的写法、第一段在配置里只是前缀命中（adn-x-high-old），快照里对上两行，没有强度的那一行排在前面：
		// high 是第二行（adn-x）的一档，按一个名字处理、列候选。只看第一行的实现认不出 high，落到多段的说法。
		g := cxhNewRig(t, cxhConfigTemplate+"\n[agents.adn-x-high-old]\nprovider = 'grok'\nmodel = 'grok-4.5'\n", adnCodexIDFirst)
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add adn-x-high high", []string{adnDiscovering, "adn-x-high 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:", "  codex   adn-x-high  +1", "  codex   adn-x  +2"})
	})
	t.Run("typo_second_row", func(t *testing.T) {
		// v1.2（W1b minor 4）：同一份夹具、第一段没命中配置，第二段是敲错的强度。有强度的那一行排在第二：只看第一行
		// 有没有强度的实现认不出这是敲错的强度，落到多段的说法。
		g := cxhNewRig(t, cxhConfigTemplate, adnCodexIDFirst)
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add adn-x-high xigh", []string{adnDiscovering, "adn-x-high 对上了识别出的 2 个模型，不猜；用 +编号 或带强度的 ID 指明:", "  codex   adn-x-high  +1", "  codex   adn-x  +2"})
	})
}

// TestAdnMultiSegment 是 N7：多段里只要有一段要按识别结果加，就整条拒绝、一个都不加（grok-4.7 在配置里、
// 本来能加也不加）；每段按出现顺序各打一行。一段对上多行也是「一次加一个」（ambiguous_seg）；每段都命中配置、其中一段
// 是拒绝句时不识别（all_config_refusal）。
// 【判别力】with_config、three、ambiguous_seg 对 V27；three 另对 V28（配置里的拒绝句不打）、V39、V50（多段那一处只看 id）、V51；ambiguous_seg 对 V52（对上多行不算）；all_config 对 V29；all_config_refusal 对 V49、V51、V29、V41。
func TestAdnMultiSegment(t *testing.T) {
	t.Run("with_config", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add grok-4.7,gpt-6.1-sol-xhigh", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol-xhigh")})
	})
	t.Run("three", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add gpt-6.1-sol-xhigh nosuch 99", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol-xhigh"), adnNotFound("nosuch"), "编号 99 超出范围（1-2）"})
	})
	t.Run("all_config", func(t *testing.T) {
		// 每一段都在配置里命中：照 AD1 之前加入，不识别。
		g := adnNewRig(t, true)
		mdxWantLines(t, "/add grok-4.7,gpt-5.5-high", g.run("/add grok-4.7,gpt-5.5-high"), []string{"grok-4.7 已加入会话 " + g.conv.ID, "gpt-5.5-high 已加入会话 " + g.conv.ID})
		if n := g.exec.count(); n != 0 {
			t.Errorf("每段都在配置里命中时不该识别，执行器被调了 %d 次", n)
		}
	})
	t.Run("all_config_refusal", func(t *testing.T) {
		// 每一段都在配置里命中、其中一段是拒绝句：照 AD1 之前全或无，只打那一句，不识别。新装置（理由同 N5 的 refusal_*），
		// 同一装置接着按名字加是正对照。
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add grok-4.7,99", []string{"编号 99 超出范围（1-2）"})
		adnNoDiscovery(t, g, "/add grok-4.7,99")
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	})
	t.Run("ambiguous_seg", func(t *testing.T) {
		// 多段里一段在快照里对上多行（codex 与 grok 都有 gpt-6.1-sol），同样是「一次加一个」，不是「都没有」。
		g := adnNewRig(t, true)
		g.exec.set(mdxGrokPath, mdxResp{out: []byte(adnGrokExtra)})
		adnRefused(t, g, "/add gpt-6.1-sol grok-4.7", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol")})
	})
}

// TestAdnNotFound 是 N8：配置里与快照里都对不上时的新说法；已有快照不重拍（第二次不再识别）；custom/ 行加不了。
// codex_failed：codex 识别失败时，它的模型对不上、说法不变，失败行不在这里打；grok 的照常能加。
// no_providers：配置里一家能识别的都没有时不识别。
// 【判别力】snapshot_kept、codex_failed 对 V31（每次重拍）、V39（最后一句仍是旧说法）；codex_failed 另对 V35；no_providers 对 V54（照样调识别）、V39。
func TestAdnNotFound(t *testing.T) {
	t.Run("snapshot_kept", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add nosuch", []string{adnDiscovering, adnNotFound("nosuch")})
		adnRefused(t, g, "/add custom/adn-cpa", []string{adnNotFound("custom/adn-cpa")})
		if n := g.exec.count(); n != 2 {
			t.Errorf("已有快照不该重拍，执行器共被调了 %d 次（应当是 2）", n)
		}
	})
	t.Run("codex_failed", func(t *testing.T) {
		g := adnNewRig(t, true)
		g.exec.set(cxhCodexPath, mdxResp{err: errors.New("adn codex down")})
		adnRefused(t, g, "/add gpt-6.1-sol-xhigh", []string{adnDiscovering, adnNotFound("gpt-6.1-sol-xhigh")})
		adnAdded(t, g, "/add grok-4.6", "grok-4.6", adnBlockGrok46, false)
	})
	t.Run("no_providers", func(t *testing.T) {
		// 配置里一家能识别的 provider 都没有：不识别（没有「正在识别模型」那一行），快照留 nil，只打都没有那一句。
		// 有 provider 时同一条命令先识别，见 snapshot_kept。
		g := mdxNewRig(t, "data_dir = '<dataDir>'\n")
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add nosuch", []string{adnNotFound("nosuch")})
		if n := g.exec.count(); n != 0 {
			t.Errorf("没有能识别的 provider 时不该识别，执行器被调了 %d 次", n)
		}
		if g.r.modelSnap != nil {
			t.Errorf("没有能识别的 provider 时快照应当留 nil")
		}
	})
}

// TestAdnPriority 是 N9：识别快照里的精确命中排在「X 开头的都已经在当前会话里了」之前。先按名字加一个 grok-4.6-a
// （在场），再 /add grok-4.6：配置里对不上，快照里是同一个模型，这个模型已有在场的成员，打 v1.2 的拒绝句（D17；
// grok 没有强度可选，不带「另加一档」的后半句）；同一装置把开关关上是对照，打的是「开头的都已经在」。快照里也对不上的
// grok-4 照旧落到「开头的都已经在」。summary_prefix（v1.2，W1b minor 4）：开关打开时总结成员前缀那一句同样排在快照
// 之后、照旧打。
// 【判别力】V34（「开头的都已经在」排到快照之前）只有它杀得到；另对 V21、V26、V29、V30、V31、V35；v1.2 另对 V71（去掉
// 「有在场的」那一支）、V79（在场的那句一律带「另加一档」）；summary_prefix 对 V69（开关打开时跳过总结成员前缀那一句）。
func TestAdnPriority(t *testing.T) {
	g := adnNewRig(t, true)
	mdxWantLines(t, "/add grok-4.7", g.run("/add grok-4.7"), []string{"grok-4.7 已加入会话 " + g.conv.ID})
	adnAdded(t, g, "/add grok-4.6 --id grok-4.6-a", "grok-4.6-a", adnBlockGrok46A, true)
	adnRefused(t, g, "/add grok-4.6", []string{adnPresent("grok-4.6", "grok-4.6-a", false)})
	adnRefused(t, g, "/add grok-4", []string{"grok-4 开头的都已经在当前会话里了"})
	g.r.SetAddByName(false)
	adnRefused(t, g, "/add grok-4.6", []string{"grok-4.6 开头的都已经在当前会话里了"})

	t.Run("summary_prefix", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[summary]\nagent = 'grok-4.7'\n", adnCodexOut)
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add gro", []string{adnDiscovering, "gro 开头的除总结成员 grok-4.7 外没有可加入的；总结成员不用 /add：多个成员回复之后它会自己加入做总结"})
	})
}

// TestAdnCanceled 是 N10：自动识别期间取消，只打两行，什么都不加、不存快照；之后再敲重新识别、照常加入。
// not_last 取消落在 codex（grok 不再识别），last 落在 grok；flag_not_last 是带 --effort 那条路，取消落在 codex。
// 【判别力】V42（取消之后照样存快照）；flag_not_last 对 V53（带参数那条路被取消不停），另对 V21、V26、V35；not_last、last 另对 V17、V37（取消之后那一次加入的字节）。
func TestAdnCanceled(t *testing.T) {
	for _, c := range []struct {
		name   string
		at     int
		before int
		cmd    string
	}{
		{"not_last", 1, 1, "/add gpt-6.1-sol-xhigh"},
		{"last", 2, 2, "/add gpt-6.1-sol-xhigh"},
		{"flag_not_last", 1, 1, "/add gpt-6.1-sol --effort xhigh"},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := adnNewRig(t, true)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			g.exec.hook = func(n int, _ provider.DiscoveryCommand) {
				if n == c.at {
					cancel()
				}
			}
			orig := g.readFile()
			mdxWantLines(t, "取消的那次", g.runCtx(ctx, c.cmd), []string{adnDiscovering, adnCanceled})
			if g.r.modelSnap != nil {
				t.Fatalf("取消之后不该存快照")
			}
			if n := g.exec.count(); n != c.before {
				t.Errorf("取消之后执行器应当共被调 %d 次，实际 %d", c.before, n)
			}
			if string(g.readFile()) != string(orig) || len(g.bakNames()) != 0 || len(g.activeIDs()) != 0 {
				t.Fatalf("取消之后不该写配置、不该有备份、不该有人加入")
			}
			g.exec.hook = nil
			adnAdded(t, g, c.cmd, "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
		})
	}
}

// TestAdnSharesSnapshot 是 N11：按名字加与不带参数的 /add 是同一份快照。by_name_first：按名字加时自动识别
// （登录状态一起拍），之后不带参数的 /add 不再识别、照这份快照打表与登录段（登录状态执行器只跑过一次）；
// list_first：先不带参数的 /add，再按名字加，不再识别。
// 【判别力】by_name_first 对 V33（按名字加打登录段）；list_first 对 V31（每次重拍）。
func TestAdnSharesSnapshot(t *testing.T) {
	t.Run("by_name_first", func(t *testing.T) {
		g := adnNewRig(t, true)
		le := &adnLoginExec{}
		g.r.SetLoginStatusExec(le.run)
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
		mdxWantLines(t, "之后的 /add", g.afterCandidates(g.run("/add")), []string{
			"识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:",
			"NO  PROVIDER  ID                      MODEL                   EFFORTS  SOURCE",
			"+2  codex     adn-x-high              adn-x                   high     codex 目录",
			"+3  codex     adn-x-high              adn-x-high              -        codex 目录",
			"+4  grok      grok-4.6                grok-4.6                -        本地配置",
			"+5  grok      grok-4.5                grok-4.5                -        本地配置",
			"+6  grok      grok-4.3                grok-4.3                -        本地配置",
			"+7  grok      grok-composer-2.5-fast  grok-composer-2.5-fast  -        本地配置",
			"+8  grok      grok-build              grok-build              -        本地配置",
			"用 /add +编号 加入；--id <新 id> 改名；claude 与 codex 可加 --effort <强度> 换档，EFFORTS 列是它支持的档",
			"codex 目录里还有 1 个 custom/ 开头的模型没有列出：它们是你在 codex 配置里自定义转发的模型，多半是别家的（例如 grok、gemini），要用就加那一家的成员",
			"各家的登录状态（本会话的快照，/add --refresh 重新检查）:",
			"  codex   已登录",
			"  grok    已登录（API key）",
		})
		if n := g.exec.count(); n != 2 {
			t.Errorf("之后的 /add 不该重新识别，执行器共被调了 %d 次", n)
		}
		if le.n != 1 {
			t.Errorf("登录状态执行器应当只在按名字加那一次跑过 1 次，实际 %d 次", le.n)
		}
	})
	t.Run("list_first", func(t *testing.T) {
		g := adnNewRig(t, true)
		g.run("/add")
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, false)
		if n := g.exec.count(); n != 2 {
			t.Errorf("已有快照不该重拍，执行器共被调了 %d 次", n)
		}
	})
}

// TestAdnGuard 是 N12：文件在上次加载之后被改过时，守卫那一句里「再 /add」后面是敲的名字，不带 --effort 与第二个词。
// 改的是 grok-4.7 的 display_name（不是注释），整份比对不上。
// 【判别力】two_words 对 V20（再 /add 后面带上第二个词）；flag 对 V21、V26。
func TestAdnGuard(t *testing.T) {
	for _, c := range []struct {
		name, cmd, retry string
	}{
		{"id", "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh"},
		{"two_words", "/add gpt-6.1-sol xhigh", "gpt-6.1-sol"},
		{"flag", "/add gpt-6.1-sol --effort xhigh", "gpt-6.1-sol"},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := adnNewRig(t, true)
			g.writeFile([]byte(adnReplaceOnce(t, g.readFile(), "display_name = 'Grok 4.7'", "display_name = 'Grok 4.7 adn'")))
			adnRefused(t, g, c.cmd, []string{adnDiscovering, "配置文件在上次加载之后被改过，先 /config reload --apply 让它生效，再 /add " + c.retry})
		})
	}
}

// adnReplaceOnce 把 b 里恰好一处 old 换成 repl（经 mdxReplaceOnce，不是恰好一处就 Fatal）。
func adnReplaceOnce(t *testing.T, b []byte, old, repl string) string {
	t.Helper()
	return mdxReplaceOnce(t, string(b), old, repl)
}

// TestAdnNoRunner 是 N13：Runner 不在时，按名字对上之后打 Runner 那一句，不写配置。
// 【判别力】V19（不判 Runner：写完配置之后在 SetConfig 处 panic，装置接住判红）。
func TestAdnNoRunner(t *testing.T) {
	g := mdxNewRigOpts(t, cxhConfigTemplate, mdxRigOpts{noRunner: true})
	g.exec.set(cxhCodexPath, mdxResp{out: []byte(adnCodexOut)})
	g.r.SetAddByName(true)
	adnRefused(t, g, "/add gpt-6.1-sol-xhigh", []string{adnDiscovering, "provider 调用未就绪，本命令不可用"})
}

// TestAdnFlags 是 N14：带 --id / --effort 时的参数形状；名字在配置里按全名命中、或命中一句拒绝时不识别。
// /add --id x +1：+编号 只在第一个词时才认，这里打 +编号 的用法，不说「都没有 +1」。v1.2：/add grok --id x 的 grok 在配置里
// 只是前缀命中，先识别（D9），挪到 N23 prefix_no_match；plus_letters（W1b minor 4）：/add --id x +abc 不是 + 加数字，照常
// 识别、落到「都没有 +abc」（§7 (u)），新装置。v1.3（W1c 的 W7）：/add 1 --effort high 按编号命中 gpt-5.5-high，
// 编号不算前缀命中，不识别、直接打「对上的是配置里的 agent」。
// 【判别力】V22（多个名字取第一个）、V23（名字不按逗号切）、V24（命中 agent 不说明）、V25（命中的拒绝句不打）、V65（+ 加数字不打 +编号 的用法）、V21；/add 1 --effort high 对 V92（configPrefixOnly 把编号命中也算前缀命中）；plus_letters 对 V68（+ 开头一律打 +编号 的用法）。
func TestAdnFlags(t *testing.T) {
	g := adnNewRig(t, true)
	for _, c := range []struct {
		cmd  string
		want string
	}{
		{"/add grok-4.7 --effort high", "grok-4.7 对上的是配置里的 agent grok-4.7，--id 与 --effort 只用于加识别出的模型；直接 /add grok-4.7"},
		{"/add 99 --id z", "编号 99 超出范围（1-2）"},
		{"/add --effort high", adnUsage},
		{"/add a b --effort high", adnUsage},
		{"/add a,b --effort high", adnUsage},
		{"/add gpt-6.1-sol --effort", adnUsage},
		{"/add gpt-6.1-sol --id a --id b", adnUsage},
		{"/add gpt-6.1-sol --effort low --effort high", adnUsage},
		{"/add gpt-6.1-sol xhigh --id x", adnUsage},
		{"/add --id x +1", "用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别"},
		{"/add 1 --effort high", adnFlagsConfig("1", "gpt-5.5-high")},
	} {
		adnRefused(t, g, c.cmd, []string{c.want})
	}
	if n := g.exec.count(); n != 0 {
		t.Errorf("这几条都不该识别，执行器被调了 %d 次", n)
	}

	t.Run("plus_letters", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add --id x +abc", []string{adnDiscovering, adnNotFound("+abc")})
	})
}

// TestAdnOtherProviders 是 N15：MD1 的配置（agy、claude、grok）。claude 的别名（sonnet[1m]）是模型名形式、默认 high，没有 high 的（mdx-lowmed）取第一档；
// claude 的 Model（claude-sonnet-5）同样是模型名形式；claude 的 ID 形式带强度；agy 的 id（Alias）；claude 没有强度的行不能带
// --effort；别名 opus[1m] 对上已配成 opus-5.0（model 写的就是这个别名）的那一行：带 --effort high 是同脑子（alias_same_brain），
// 不带强度时加入 opus-5.0、不写配置（alias_configured，v1.2 的 D17）；不带强度的 claude ID 段（sonnet-5）
// 按 DefaultEffort 加，跟第二个词时按那一档加（裁决 (d)）；别名与没有强度的 ID 不分大小写（claude_alias_fold、haiku_fold）。
// 【判别力】claude_alias、claude_alias_fold、claude_first_effort、alias_same_brain、alias_configured 对 V6（不认 Alias）；claude_alias_fold 对 V55（别名按字节比）、V2；claude_model、claude_model_fold 对 V8（不认 Model）；claude_model_fold 另对 V2；alias_same_brain、alias_configured 另对 V5；alias_configured 另对 V73（同一个模型不认别名写法）、V75（恰好一个不在场时不加入）；claude_base、claude_base_second 对 V64（有强度的行不认不带强度的 ID）、V4；claude_base_second 另对 V10、V18；haiku_effort、haiku_fold 对 V4；haiku_fold 对 V56（不带强度的 ID 按字节比）；agy_alias 对 V35；claude_id 对 V17、V37。
func TestAdnOtherProviders(t *testing.T) {
	const disc = "正在识别模型: agy, claude, grok"
	for _, c := range []struct {
		name, cmd, id string
	}{
		{"claude_alias", "/add sonnet[1m]", "sonnet-5-1m-high"},
		{"claude_alias_fold", "/add SONNET[1m]", "sonnet-5-1m-high"},
		{"claude_model", "/add claude-sonnet-5", "sonnet-5-high"},
		{"claude_model_fold", "/add Claude-Sonnet-5", "sonnet-5-high"},
		{"claude_first_effort", "/add mdx-lowmed", "mdx-lowmed-2.0-low"},
		{"claude_id", "/add sonnet-5-xhigh", "sonnet-5-xhigh"},
		{"claude_base", "/add sonnet-5", "sonnet-5-high"},
		{"claude_base_second", "/add sonnet-5 xhigh", "sonnet-5-xhigh"},
		{"haiku_fold", "/add HAIKU-4.5", "haiku-4.5"},
		{"agy_alias", "/add gemini-3.7-flash-high", "gemini-3.7-flash-high"},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := mdxNewRig(t, mdxConfigTemplate)
			g.r.SetAddByName(true)
			orig := g.readFile()
			baks := g.bakNames()
			lines := g.run(c.cmd)
			bak := g.newBak(baks)
			mdxWantLines(t, c.cmd, lines, append([]string{disc}, g.joinedLines(c.id, bak)...))
			if len(g.readFile()) <= len(orig) {
				t.Errorf("%s 之后配置文件应当变长", c.cmd)
			}
		})
	}
	for _, c := range []struct {
		name, cmd, want string
	}{
		{"haiku_effort", "/add haiku-4.5 --effort high", "claude-haiku-4-5-20251001 没有可选的强度，不能带 --effort"},
		{"alias_same_brain", "/add opus[1m] --effort high", "配置里已经有同一个模型、同一强度的 agent opus-5.0，直接 /add opus-5.0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := mdxNewRig(t, mdxConfigTemplate)
			g.r.SetAddByName(true)
			adnRefused(t, g, c.cmd, []string{disc, c.want})
		})
	}
	t.Run("alias_configured", func(t *testing.T) {
		// v1.2（D17 第 4 步）：没给强度、对上的模型在配置里恰好有一个不在场的成员（opus-5.0，model 写的是别名），
		// 加入它、不写配置。v1.1 这里打同脑子那一句；同脑子那一句现在只在给了强度时打（alias_same_brain）。
		g := mdxNewRig(t, mdxConfigTemplate)
		g.r.SetAddByName(true)
		adnJoinedNoWrite(t, g, "/add opus[1m]", "opus-5.0", []string{disc})
	})
}

// TestAdnTwoWordsConfigPrefix 是 N16（裁决 (b)）：模型加进配置、又不在场之后，第一段在配置里只是前缀命中，
// 两个词的写法照样成立。先按名字加 gpt-6.1-sol-xhigh 再 /leave；/add gpt-6.1-sol xhigh 是同一模型同一强度，打同脑子
// 那一句、不写；/add gpt-6.1-sol xigh 不是一档强度，第一段又在配置里命中，照多段只说 xigh 都没有（敲错的强度那一支只给
// 配置里没命中的第一段）；/add gpt-6.1-sol high 新建 gpt-6.1-sol-high。单段 /add gpt-6.1-sol 仍按配置前缀加入已有的
// gpt-6.1-sol-xhigh（对照：单段不变）。之后都不再识别。XHIGH 不是一档强度（区分大小写），同 xigh。
// 【判别力】V61（敲错的强度那一支也给前缀命中）、V11（第二个词当强度时不分大小写）只有它杀得到；V62（不认配置前缀命中的
// 第一段）另有 union_second_row；另对 V10、V17、V18、V29、V30、V31、V35、V37、V39。
func TestAdnTwoWordsConfigPrefix(t *testing.T) {
	g := adnNewRig(t, true)
	adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	mdxWantLines(t, "/leave", g.run("/leave gpt-6.1-sol-xhigh"), []string{"gpt-6.1-sol-xhigh 已退出会话 " + g.conv.ID})
	adnRefused(t, g, "/add gpt-6.1-sol xhigh", []string{"配置里已经有同一个模型、同一强度的 agent gpt-6.1-sol-xhigh，直接 /add gpt-6.1-sol-xhigh"})
	adnRefused(t, g, "/add gpt-6.1-sol xigh", []string{adnNotFound("xigh")})
	adnRefused(t, g, "/add gpt-6.1-sol XHIGH", []string{adnNotFound("XHIGH")})
	adnAdded(t, g, "/add gpt-6.1-sol high", "gpt-6.1-sol-high", adnBlockSolHigh, false)
	mdxWantLines(t, "/add gpt-6.1-sol", g.run("/add gpt-6.1-sol"), []string{"gpt-6.1-sol-xhigh 已加入会话 " + g.conv.ID})
	if n := g.exec.count(); n != 2 {
		t.Errorf("只在第一次识别，执行器共被调了 %d 次（应当是 2）", n)
	}
}

// TestAdnIdeographicSpace 是 N17：全角空格 U+3000 与半角空格一样切段，不看开关。开着：两个词的写法用全角空格照样加入
// （on_two_words）；fields 本来就在全角空格上切，带 --effort 的写法同样加入（on_flag）。关着：两个配置里的全名用全角空格
// 隔开，两个都加入、不识别（off_config）。
// 【判别力】on_two_words、off_config 对 V66（splitAddArgs 不按全角空格切）；on_flag 不经 splitAddArgs 切段（fields 已经切开），
// V66 下照样绿，它实测的是「fields 在全角空格上切」这一条前提，另对 V21、V26。
func TestAdnIdeographicSpace(t *testing.T) {
	t.Run("on_two_words", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add gpt-6.1-sol"+adnIdeoSpace+"xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	})
	t.Run("on_flag", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add gpt-6.1-sol"+adnIdeoSpace+"--effort"+adnIdeoSpace+"xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	})
	t.Run("off_config", func(t *testing.T) {
		g := adnNewRig(t, false)
		cmd := "/add grok-4.7" + adnIdeoSpace + "gpt-5.5-high"
		mdxWantLines(t, cmd, g.run(cmd), []string{"grok-4.7 已加入会话 " + g.conv.ID, "gpt-5.5-high 已加入会话 " + g.conv.ID})
		adnNoDiscovery(t, g, cmd)
	})
}

// adnTwoSolAgents 是 cxhConfigTemplate 末尾加两个 gpt-6.1-sol 的成员（id 不以模型名开头，所以 /add gpt-6.1-sol 在配置里
// 按前缀对不上）。
const adnTwoSolAgents = "\n[agents.sol-a]\nprovider = 'codex'\nmodel = 'gpt-6.1-sol'\neffort = 'high'\n\n[agents.sol-b]\nprovider = 'codex'\nmodel = 'gpt-6.1-sol'\neffort = 'xhigh'\n"

// TestAdnExistingMembers 是 N22（v1.2，D17）：没给强度也没给 --id 的单个名字（模型名形式或不带强度的 ID 形式）在配置里
// 没命中、在快照里对上一行时，先看配置里这个模型已有的成员（provider 与 model 相同，不看 id 前缀；不含总结成员）：有在场的
// 就拒绝、点名；全不在场、恰好一个就加入它；不止一个就列出来。三种都不写配置。
// repeat：同一条 /add gpt-6.1-sol 敲两次，第一次（成员不在场，配置前缀命中）加入 gpt-6.1-sol-xhigh，第二次打拒绝句，不写、
// 不多出成员（W1b 的 major）。prefix_other_model：gpt-6.1 与 gpt-6.1-sol 是两个模型，在场的 gpt-6.1-sol-xhigh 不挡
// /add gpt-6.1。alias_present、alias_config_present：claude 的别名形式，同一个模型的成员在场时拒绝（前者是按名字加进来的，
// 配置里 model 写的是 Model；后者是 model 写别名的 opus-5.0）。several_absent：两个不在场的同模型成员，列出来；同一装置
// 带强度照常新建（正对照）。one_absent_config、one_absent_renamed：恰好一个不在场、id 又不以名字开头（配置里起的名字、
// --id 起的名字），加入它；同一装置带强度照常新建，或再敲一次打拒绝句。several_absent_grok：没有强度可选的行，列出来时
// 不提强度。summary_excluded：总结成员不算（大写绕过配置第 0 步的字节比），落到新建、再被同脑子那一句拦下，不加入总结
// 成员；同一装置接着加别的是正对照。
// v1.3（W1c 的 W1-W5）：present_among_absent、present_among_absent_grok：S 里一个在场、一个不在场，拒绝句只点名在场的
// （有强度可选的行与 grok 各一条）；other_provider_same_model：grok 的 gx 的 model 字符串恰好是 gpt-6.1-sol，不进 codex 那一行的
// S，照常新建 gpt-6.1-sol-high、写配置；one_absent_config 另断言会话成员记录里的显示名与 provider 取配置里的值。
// 【判别力】（v1.3）present_among_absent 对 V87（在场那一句列出整个 S）、present_among_absent_grok 对 V88（同上，没有强度的行）；
// one_absent_config 对 V89、V90（成员记录的显示名、provider 写成空串）；other_provider_same_model 对 V91（S 不比 provider）；
// present_among_absent、present_among_absent_grok 另对 V5、V29、V71、V72，present_among_absent_grok 另对 V79；新的三条都对
// V1、V30、V32、V35、V38 红。
// 【判别力】repeat、one_absent_renamed、alias_present、alias_config_present 对 V71（去掉「有在场的」那一支）；prefix_other_model、
// alias_present、alias_config_present、one_absent_config、one_absent_renamed、several_absent、several_absent_grok 对 V72（S 按 id
// 前缀判；repeat 杀不到它：在场的 gpt-6.1-sol-xhigh 恰好以名字开头）；alias_config_present 对 V73（S 不认别名写法）；
// several_absent、several_absent_grok 对 V74（不止一个时加入第一个）；one_absent_config、one_absent_renamed 对 V75（恰好一个时
// 不加入）；summary_excluded 对 V78（总结成员算进 S）；several_absent 对 V80（列出时一律不提强度）；正对照那几次新建另对
// V81（one_absent_config）、V82（several_absent）；另对 V30、V32。
func TestAdnExistingMembers(t *testing.T) {
	const mdxDisc = "正在识别模型: agy, claude, grok"
	t.Run("repeat", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
		mdxWantLines(t, "/leave", g.run("/leave gpt-6.1-sol-xhigh"), []string{"gpt-6.1-sol-xhigh 已退出会话 " + g.conv.ID})
		adnJoinedNoWrite(t, g, "/add gpt-6.1-sol", "gpt-6.1-sol-xhigh", nil)
		adnRefused(t, g, "/add gpt-6.1-sol", []string{adnPresent("gpt-6.1-sol", "gpt-6.1-sol-xhigh", true)})
		if got := g.activeIDs(); !reflect.DeepEqual(got, []string{"gpt-6.1-sol-xhigh"}) {
			t.Errorf("在场成员应当只有 gpt-6.1-sol-xhigh，实际 %q", got)
		}
		if n := g.exec.count(); n != 2 {
			t.Errorf("只在第一次识别，执行器共被调了 %d 次（应当是 2）", n)
		}
	})
	t.Run("prefix_other_model", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate, adnCodexSol61)
		g.r.SetAddByName(true)
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
		adnAdded(t, g, "/add gpt-6.1", "gpt-6.1-high", adnBlock61High, false)
	})
	t.Run("alias_present", func(t *testing.T) {
		g := mdxNewRig(t, mdxConfigTemplate)
		g.r.SetAddByName(true)
		baks := g.bakNames()
		lines := g.run("/add sonnet[1m] xhigh")
		mdxWantLines(t, "/add sonnet[1m] xhigh", lines, append([]string{mdxDisc}, g.joinedLines("sonnet-5-1m-xhigh", g.newBak(baks))...))
		adnRefused(t, g, "/add sonnet[1m]", []string{adnPresent("sonnet[1m]", "sonnet-5-1m-xhigh", true)})
	})
	t.Run("alias_config_present", func(t *testing.T) {
		g := mdxNewRig(t, mdxConfigTemplate)
		g.r.SetAddByName(true)
		mdxWantLines(t, "/add opus-5.0", g.run("/add opus-5.0"), []string{"opus-5.0 已加入会话 " + g.conv.ID})
		adnRefused(t, g, "/add opus[1m]", []string{mdxDisc, adnPresent("opus[1m]", "opus-5.0", true)})
	})
	t.Run("several_absent", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+adnTwoSolAgents, adnCodexOut)
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add gpt-6.1-sol", []string{adnDiscovering, adnConfigured("gpt-6.1-sol", "sol-a, sol-b", true)})
		adnAdded(t, g, "/add gpt-6.1-sol low", "gpt-6.1-sol-low", adnBlockSolLow, false)
	})
	t.Run("one_absent_config", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[agents.sol-a]\ndisplay_name = 'Sol A'\nprovider = 'codex'\nmodel = 'gpt-6.1-sol'\neffort = 'xhigh'\n", adnCodexOut)
		g.r.SetAddByName(true)
		adnJoinedNoWrite(t, g, "/add gpt-6.1-sol", "sol-a", []string{adnDiscovering})
		// v1.3（W1c 的 W3、W4）：会话成员记录里的显示名与 provider 取配置里的值。夹具的 display_name 非空，
		// 换成空串才看得出来。
		found := false
		for _, p := range g.participants() {
			if p.AgentID != "sol-a" {
				continue
			}
			found = true
			if p.DisplayName != "Sol A" || p.Provider != "codex" {
				t.Errorf("sol-a 的成员记录应当是显示名 Sol A、provider codex，实际 %q、%q", p.DisplayName, p.Provider)
			}
		}
		if !found {
			t.Errorf("会话成员记录里没有 sol-a")
		}
		adnAdded(t, g, "/add gpt-6.1-sol --effort low", "gpt-6.1-sol-low", adnBlockSolLow, false)
	})
	t.Run("one_absent_renamed", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh --id sol-x", "sol-x", adnBlockSolX, true)
		mdxWantLines(t, "/leave", g.run("/leave sol-x"), []string{"sol-x 已退出会话 " + g.conv.ID})
		adnJoinedNoWrite(t, g, "/add gpt-6.1-sol", "sol-x", nil)
		adnRefused(t, g, "/add gpt-6.1-sol", []string{adnPresent("gpt-6.1-sol", "sol-x", true)})
	})
	t.Run("several_absent_grok", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[agents.g46-a]\nprovider = 'grok'\nmodel = 'grok-4.6'\n\n[agents.g46-b]\nprovider = 'grok'\nmodel = 'grok-4.6'\n", adnCodexOut)
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add grok-4.6", []string{adnDiscovering, adnConfigured("grok-4.6", "g46-a, g46-b", false)})
	})
	t.Run("summary_excluded", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[summary]\nagent = 'grok-4.7'\n", adnCodexOut)
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add GROK-4.7", []string{adnDiscovering, "配置里已经有同一个模型、同一强度的 agent grok-4.7，直接 /add grok-4.7"})
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, false)
	})
	t.Run("present_among_absent", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+adnTwoSolAgents, adnCodexOut)
		g.r.SetAddByName(true)
		mdxWantLines(t, "/add sol-a", g.run("/add sol-a"), []string{"sol-a 已加入会话 " + g.conv.ID})
		adnRefused(t, g, "/add gpt-6.1-sol", []string{adnDiscovering, adnPresent("gpt-6.1-sol", "sol-a", true)})
	})
	t.Run("present_among_absent_grok", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[agents.g46-a]\nprovider = 'grok'\nmodel = 'grok-4.6'\n\n[agents.g46-b]\nprovider = 'grok'\nmodel = 'grok-4.6'\n", adnCodexOut)
		g.r.SetAddByName(true)
		mdxWantLines(t, "/add g46-a", g.run("/add g46-a"), []string{"g46-a 已加入会话 " + g.conv.ID})
		adnRefused(t, g, "/add grok-4.6", []string{adnDiscovering, adnPresent("grok-4.6", "g46-a", false)})
	})
	t.Run("other_provider_same_model", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[agents.gx]\nprovider = 'grok'\nmodel = 'gpt-6.1-sol'\n", adnCodexOut)
		g.r.SetAddByName(true)
		adnAdded(t, g, "/add gpt-6.1-sol", "gpt-6.1-sol-high", adnBlockSolHigh, true)
		if mdxHasLine(g.activeIDs(), "gx") {
			t.Errorf("grok 的 gx 不该被当成 codex 的 gpt-6.1-sol 加入，在场的是 %q", g.activeIDs())
		}
	})
}

// TestAdnFlagsOverConfigPrefix 是 N23（v1.2，裁决书规则一、规则三，原 §7 (r)）：带 --effort 或 --id 时，名字在配置里只是
// 前缀命中、在识别快照里又对得上，按识别结果加（D9）。effort_over_prefix：配置里有不在场的 gpt-6.1-sol-xhigh（同一个模型），
// /add gpt-6.1-sol --effort high 新建 gpt-6.1-sol-high；effort_over_other_model：配置里的 gpt-6.1-sol-mini 是另一个模型，
// 同一条命令同样新建；id_over_prefix：只有 --id 时同样按识别结果、按默认档新建。id_only_present：只有 --id、这个模型已有
// 在场成员时照样新建（--id 是明确要新建，不过 D17 那道门）。prefix_no_match：名字在快照里对不上时照旧打「对上的是配置里的
// agent」，只是先识别了一次。
// 【判别力】effort_over_prefix、effort_over_other_model、id_over_prefix、prefix_no_match 对 V76（前缀命中照旧打「对上的是配置里的
// agent」、不识别）；id_over_prefix、id_only_present 对 V77（只有 --id 也过 D17 那道门）；effort_over_prefix 对 V81（--effort 也过
// 那道门）；prefix_no_match 对 V84（前缀命中、快照里对不上时落到「都没有」），另对 V24、V26。
func TestAdnFlagsOverConfigPrefix(t *testing.T) {
	addThenLeave := func(t *testing.T, g *mdxRig) {
		t.Helper()
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
		mdxWantLines(t, "/leave", g.run("/leave gpt-6.1-sol-xhigh"), []string{"gpt-6.1-sol-xhigh 已退出会话 " + g.conv.ID})
	}
	t.Run("effort_over_prefix", func(t *testing.T) {
		g := adnNewRig(t, true)
		addThenLeave(t, g)
		adnAdded(t, g, "/add gpt-6.1-sol --effort high", "gpt-6.1-sol-high", adnBlockSolHigh, false)
	})
	t.Run("effort_over_other_model", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[agents.\"gpt-6.1-sol-mini\"]\nprovider = 'codex'\nmodel = 'gpt-6.1-sol-mini'\n", adnCodexOut)
		g.r.SetAddByName(true)
		adnAdded(t, g, "/add gpt-6.1-sol --effort high", "gpt-6.1-sol-high", adnBlockSolHigh, true)
	})
	t.Run("id_over_prefix", func(t *testing.T) {
		g := adnNewRig(t, true)
		addThenLeave(t, g)
		adnAdded(t, g, "/add gpt-6.1-sol --id sol", "sol", adnBlockSol, false)
	})
	t.Run("id_only_present", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
		adnAdded(t, g, "/add gpt-6.1-sol --id sol", "sol", adnBlockSol, false)
		if got := g.activeIDs(); !reflect.DeepEqual(got, []string{"gpt-6.1-sol-xhigh", "sol"}) {
			t.Errorf("在场成员应当是 gpt-6.1-sol-xhigh 与 sol，实际 %q", got)
		}
	})
	t.Run("prefix_no_match", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add grok --id x", []string{adnDiscovering, adnFlagsConfig("grok", "grok-4.7")})
		adnRefused(t, g, "/add grok --effort high", []string{adnFlagsConfig("grok", "grok-4.7")})
		if n := g.exec.count(); n != 2 {
			t.Errorf("只在第一次识别，执行器共被调了 %d 次（应当是 2）", n)
		}
	})
}

// TestAdnTypoIsReference 是 N24（v1.2，W1b minor 1）：两个词的第二段是在场成员的前缀、总结成员的前缀或以 + 开头时，它指的
// 是配置里的人或 +编号，不当敲错的强度，照多段说（每段一行）。present_prefix、id_present_prefix：grok-4.7 在场，第二段
// grok；summary_prefix：总结成员 grok-4.7 没加入，第二段 gro；plus：第二段 +1。四条都不写配置。敲错的强度那一支仍然成立
// 的正对照是 N3 的 typo、id_then_typo、ambiguous_typo。
// 【判别力】present_prefix、id_present_prefix、summary_prefix 对 V85（去掉「addSegmentNotFound 打的是都没有」那一条）；plus 对
// V86（去掉「不以 + 开头」那一条）；summary_prefix 另对 V69；四条都对 V27。
func TestAdnTypoIsReference(t *testing.T) {
	joinGrok := func(t *testing.T, g *mdxRig) {
		t.Helper()
		mdxWantLines(t, "/add grok-4.7", g.run("/add grok-4.7"), []string{"grok-4.7 已加入会话 " + g.conv.ID})
	}
	t.Run("present_prefix", func(t *testing.T) {
		g := adnNewRig(t, true)
		joinGrok(t, g)
		adnRefused(t, g, "/add gpt-6.1-sol grok", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol"), "grok 开头的都已经在当前会话里了"})
	})
	t.Run("id_present_prefix", func(t *testing.T) {
		g := adnNewRig(t, true)
		joinGrok(t, g)
		adnRefused(t, g, "/add gpt-6.1-sol-xhigh grok", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol-xhigh"), "grok 开头的都已经在当前会话里了"})
	})
	t.Run("summary_prefix", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate+"\n[summary]\nagent = 'grok-4.7'\n", adnCodexOut)
		g.r.SetAddByName(true)
		adnRefused(t, g, "/add gpt-6.1-sol gro", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol"), "gro 开头的除总结成员 grok-4.7 外没有可加入的；总结成员不用 /add：多个成员回复之后它会自己加入做总结"})
	})
	t.Run("plus", func(t *testing.T) {
		g := adnNewRig(t, true)
		adnRefused(t, g, "/add gpt-6.1-sol +1", []string{adnDiscovering, adnOneAtATime("gpt-6.1-sol"), adnNotFound("+1")})
	})
}

// TestAdnSingleIDConfigPrefix 是 N25（v1.3，§7 (w)，W1c 的 w2 装置）：单段、不带参数、带强度的 ID 在配置里只是前缀命中时，
// 照 HEAD 的前缀语义加入配置里的那个 agent、不识别、不写，即使它是另一个模型（gpt-6.1-sol-xhigh-old 的 model 是 gpt-5.5）；
// 屏幕回显的是实际加入的 id。有意保留，钉住现状，以后要改就是有意改。同一装置再敲一次是正对照：-old 在场之后不是候选，
// 配置里没命中，识别之后按 ID 形式新建 gpt-6.1-sol-xhigh、写配置（同一条命令敲两次结果不同，§7 (w)）。
// 【判别力】V29（每一段都在配置里命中时也插手，也就是单段前缀命中也先识别：第一条就识别、新建）；正对照那一次另对 V1、
// V17、V30、V32、V34、V37、V38 红。
func TestAdnSingleIDConfigPrefix(t *testing.T) {
	g := cxhNewRig(t, cxhConfigTemplate+"\n[agents.\"gpt-6.1-sol-xhigh-old\"]\nprovider = 'codex'\nmodel = 'gpt-5.5'\neffort = 'xhigh'\n", adnCodexOut)
	g.r.SetAddByName(true)
	adnJoinedNoWrite(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh-old", nil)
	adnNoDiscovery(t, g, "/add gpt-6.1-sol-xhigh")
	adnAdded(t, g, "/add gpt-6.1-sol-xhigh", "gpt-6.1-sol-xhigh", adnBlockSolXhigh, true)
	if n := g.exec.count(); n != 2 {
		t.Errorf("正对照：第二次应当识别 codex、grok 两家，执行器共被调了 %d 次", n)
	}
}
