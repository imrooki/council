package repl

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
)

// obx_cliadvice_test.go 是 OB1（首次运行自动配置，契约 §2.2）在 repl 包的验收：
// /add 之后对本机装了、配置里还没有的工具打提示（R1-R4）、识别失败说法的导出包装（R5）、
// /add 候选为空与前缀没命中时对总结成员的说法（R6）。装置用 HEAD 的 mdxNewRig。

// obxTemplate 是 R2-R4 与 R6 no_members 的配置：只有 claude 一个 provider，没有成员。
const obxTemplate = "data_dir = '<dataDir>'\n\n[providers.claude]\ncommand = '<exe>'\n"

// obxSumTemplate 是 R6 的配置：两个成员，其中 sa-sum 是总结成员。
const obxSumTemplate = "data_dir = '<dataDir>'\n\n[providers.claude]\ncommand = '<exe>'\n\n[agents.sa-alpha]\nprovider = 'claude'\nmodel = 'claude-obx-a'\n\n[agents.sa-sum]\nprovider = 'claude'\nmodel = 'claude-obx-s'\n\n[summary]\nagent = 'sa-sum'\n"

// obxSumOnlyTemplate 是 R6 的配置：只有一个 agent，它就是总结成员。
const obxSumOnlyTemplate = "data_dir = '<dataDir>'\n\n[providers.claude]\ncommand = '<exe>'\n\n[agents.sa-sum]\nprovider = 'claude'\nmodel = 'claude-obx-s'\n\n[summary]\nagent = 'sa-sum'\n"

// obxList 是检测器桩返回的清单：claude 已在配置里、agy 没装，都不该提示；
// codex 的路径含单引号，走基本字符串。
var obxList = []config.DetectedProvider{
	{ID: "claude", Path: `C:\obx\claude.cmd`},
	{ID: "grok", Path: mdxGrokPath},
	{ID: "agy"},
	{ID: "codex", Path: `C:\obx\O'Neil\codex.cmd`},
}

// obxDetector 是检测器桩：每次调用 calls++，返回 list 的副本。
func obxDetector(list []config.DetectedProvider, calls *int) func() []config.DetectedProvider {
	return func() []config.DetectedProvider {
		*calls++
		return append([]config.DetectedProvider(nil), list...)
	}
}

// obxHintLines 是装了 obxList 之后 /add 末尾应当多出的六行（契约 §2.2 逐字）。
func obxHintLines(cfgPath string) []string {
	return []string{
		"本机装了 grok（mdx-grok.exe），配置里还没有：要用它，把下面两行追加到 " + cfgPath + " 末尾，再敲 /config reload --apply 与 /add --refresh",
		"[providers.grok]",
		"command = 'mdx-grok.exe'",
		"本机装了 codex（C:\\obx\\O'Neil\\codex.cmd），配置里还没有：要用它，把下面两行追加到 " + cfgPath + " 末尾，再敲 /config reload --apply 与 /add --refresh",
		"[providers.codex]",
		"command = \"C:\\\\obx\\\\O'Neil\\\\codex.cmd\"",
	}
}

// obxTail 返回 lines 的最后 n 行；不够 n 行时判红。
func obxTail(t *testing.T, lines []string, n int) []string {
	t.Helper()
	if len(lines) < n {
		t.Fatalf("输出只有 %d 行，不够取最后 %d 行: %q", len(lines), n, lines)
	}
	return lines[len(lines)-n:]
}

// TestObxSetCLIDetectorZeroValue 是 R1：直接 New 出来的 REPL 不检测。
// 【判别力】M64。
func TestObxSetCLIDetectorZeroValue(t *testing.T) {
	r := New(nil, nil, nil, strings.NewReader(""), nil, RulesContext{})
	if r.detectCLIs != nil {
		t.Errorf("New 出来的 REPL 的 detectCLIs 应当是 nil")
	}
}

// TestObxAddHint 是 R2：/add 列完识别结果之后对 grok、codex 各打一行提示和两行配置，每次现检测。
// g0 不装检测器是正对照（没有提示）。
// 【判别力】M11、M12、M60、M61、M63、M65。
func TestObxAddHint(t *testing.T) {
	g0 := mdxNewRig(t, obxTemplate)
	base := g0.run("/add")
	if len(base) == 0 {
		t.Fatalf("前提：不装检测器时 /add 应当有输出")
	}

	g := mdxNewRig(t, obxTemplate)
	calls := 0
	g.r.SetCLIDetector(obxDetector(obxList, &calls))
	want := append(append([]string(nil), base...), obxHintLines(g.cfgPath)...)
	mdxWantLines(t, "装了检测器的第一次 /add", g.run("/add"), want)
	if calls != 1 {
		t.Errorf("第一次 /add 之后检测器应当恰好调 1 次，实际 %d", calls)
	}
	second := g.run("/add")
	mdxWantLines(t, "第二次 /add 的最后六行", obxTail(t, second, 6), obxHintLines(g.cfgPath))
	if calls != 2 {
		t.Errorf("第二次 /add 之后检测器应当共调 2 次（不缓存），实际 %d", calls)
	}
}

// TestObxAddHintFollow 是 R3：照提示把 grok 那两行追加进配置、重载之后，grok 的提示不再出现。
// 【判别力】M11、M60、M61、M63。
func TestObxAddHintFollow(t *testing.T) {
	g := mdxNewRig(t, obxTemplate)
	calls := 0
	g.r.SetCLIDetector(obxDetector(obxList, &calls))
	first := g.run("/add")
	tail := obxTail(t, first, 6)
	grokLines := tail[1:3] // 倒数第 5、4 行
	if grokLines[0] != "[providers.grok]" {
		t.Fatalf("前提：倒数第 5 行应当是 [providers.grok]，实际 %q（输出 %q）", grokLines[0], first)
	}
	body := g.readFile()
	g.writeFile(append(body, []byte(grokLines[0]+"\n"+grokLines[1]+"\n")...))
	reload := g.run("/config reload --apply")
	if last := obxTail(t, reload, 1)[0]; last != mdxAppliedLine {
		t.Fatalf("/config reload --apply 的最后一行应当是 %q，实际 %q（输出 %q）", mdxAppliedLine, last, reload)
	}
	refresh := g.run("/add --refresh")
	if !mdxHasLine(refresh, "正在识别模型: claude, grok") {
		t.Errorf("/add --refresh 应当有「正在识别模型: claude, grok」这一行，实际 %q", refresh)
	}
	for _, l := range refresh {
		if strings.HasPrefix(l, "本机装了 grok") {
			t.Errorf("grok 已经写进配置，不该再提示: %q", l)
		}
	}
	mdxWantLines(t, "/add --refresh 的最后三行", obxTail(t, refresh, 3), obxHintLines(g.cfgPath)[3:])
}

// TestObxAddHintCanceled 是 R4：识别被取消的那一次不检测、不提示。
// 【判别力】M62。
func TestObxAddHintCanceled(t *testing.T) {
	g := mdxNewRig(t, obxTemplate)
	calls := 0
	g.r.SetCLIDetector(obxDetector(obxList, &calls))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g.exec.hook = func(int, provider.DiscoveryCommand) { cancel() }
	got := g.runCtx(ctx, "/add")
	if last := obxTail(t, got, 1)[0]; last != "识别被取消；下次 /add 会重新识别" {
		t.Errorf("最后一行应当是「识别被取消；下次 /add 会重新识别」，实际 %q（输出 %q）", last, got)
	}
	if calls != 0 {
		t.Errorf("识别被取消时检测器不该被调，实际调了 %d 次", calls)
	}
}

// TestObxDiscoveryFailureTextExported 是 R5：导出包装与包内那一份说法相同。
// 【判别力】M66。
func TestObxDiscoveryFailureTextExported(t *testing.T) {
	timeoutErr := fmt.Errorf("%w: x", provider.ErrDiscoveryTimeout)
	for _, err := range []error{
		timeoutErr,
		fmt.Errorf("g: %w", provider.ErrDiscoveryEmpty),
		fmt.Errorf("g: %w", provider.ErrDiscoveryUnparseable),
		errors.New("obx 其它"),
	} {
		got := DiscoveryFailureText(err, 7*time.Second)
		want := discoveryFailureText(err, 7*time.Second)
		if got != want {
			t.Errorf("DiscoveryFailureText(%v) = %q，包内那一份是 %q", err, got, want)
		}
	}
	if got := DiscoveryFailureText(timeoutErr, 7*time.Second); got != "超时（7s 内没有回来）" {
		t.Errorf("超时那一种应当是 %q，实际 %q", "超时（7s 内没有回来）", got)
	}
}

// obxJoinDirect 绕过 /add 直接 Join 一个成员（总结成员 /add 加不进去），DisplayName、Provider 抄配置。
func obxJoinDirect(t *testing.T, g *mdxRig, id string) {
	t.Helper()
	ac, ok := g.r.cfg.Agents[id]
	if !ok {
		t.Fatalf("前提：配置里应当有 agent %s", id)
	}
	p := conversation.Participant{
		ConvID:      g.conv.ID,
		AgentID:     id,
		DisplayName: ac.DisplayName,
		Provider:    ac.Provider,
		Active:      true,
		JoinedAt:    time.Now().UTC(),
	}
	if err := g.svc.Join(g.ctx, g.conv.ID, p); err != nil {
		t.Fatalf("Join %s: %v", id, err)
	}
}

// obxLeaveDirect 直接让成员退出，造「加入过又退出」。
func obxLeaveDirect(t *testing.T, g *mdxRig, id string) {
	t.Helper()
	if err := g.svc.Leave(g.ctx, g.conv.ID, id, time.Now().UTC()); err != nil {
		t.Fatalf("Leave %s: %v", id, err)
	}
}

// obxFirstLine 是 g.run("/add") 输出的第 0 行。
func obxFirstLine(t *testing.T, g *mdxRig) string {
	t.Helper()
	lines := g.run("/add")
	if len(lines) == 0 {
		t.Fatalf("/add 没有输出")
	}
	return lines[0]
}

// obxWantFirst 断言 /add 的第一行。
func obxWantFirst(t *testing.T, g *mdxRig, want string) {
	t.Helper()
	if got := obxFirstLine(t, g); got != want {
		t.Errorf("/add 的第一行:\n got  %q\n want %q", got, want)
	}
}

// TestObxAddCandidatesWording 是 R6：候选为空的四种说法、前缀没命中时对总结成员的说法、
// 第 0 步对在场总结成员的说法。
// 【判别力】summary_only_config 对 M67、M68、M96；summary_missing 对 M69、M96；all_present 对 M70
// （summary_missing 与 summary_left 的正对照）；prefix_summary、prefix_summary_only 对 M71、M99；
// prefix_all_present 对 M72（prefix_summary 与 prefix_summary_left 的正对照）；summary_left 对 M95、M100；
// summary_only_left 对 M95、M97、M100（summary_only_config 是它的正对照）；prefix_summary_left 对 M71、M98；
// summary_present 对 M104、M105（前一半是后一半的正对照）。
func TestObxAddCandidatesWording(t *testing.T) {
	const noMembers = "配置里还没有可以 /add 的成员"
	const sumLeft = "总结成员 sa-sum 已退出当前会话，要恢复总结用 /join sa-sum；除它之外配置里没有可以 /add 的成员"
	t.Run("no_members", func(t *testing.T) {
		g := mdxNewRig(t, obxTemplate)
		obxWantFirst(t, g, noMembers)
	})
	t.Run("summary_only_config", func(t *testing.T) {
		g := mdxNewRig(t, obxSumOnlyTemplate)
		obxWantFirst(t, g, noMembers)
	})
	t.Run("summary_missing", func(t *testing.T) {
		g := mdxNewRig(t, obxSumTemplate)
		mdxWantLines(t, "/add sa-alpha", g.run("/add sa-alpha"), []string{"sa-alpha 已加入会话 " + g.conv.ID})
		obxWantFirst(t, g, "除总结成员 sa-sum 外，配置里的 agent 都已经在当前会话里了；总结成员不用 /add：多个成员回复之后它会自己加入做总结")
	})
	t.Run("all_present", func(t *testing.T) {
		g := mdxNewRig(t, obxSumTemplate)
		obxJoinDirect(t, g, "sa-alpha")
		obxJoinDirect(t, g, "sa-sum")
		obxWantFirst(t, g, mdxAllPresent)
	})
	t.Run("prefix_summary", func(t *testing.T) {
		g := mdxNewRig(t, obxSumTemplate)
		obxJoinDirect(t, g, "sa-alpha")
		mdxWantLines(t, "/add sa", g.run("/add sa"), []string{"sa 开头的除总结成员 sa-sum 外没有可加入的；总结成员不用 /add：多个成员回复之后它会自己加入做总结"})
	})
	t.Run("prefix_summary_only", func(t *testing.T) {
		g := mdxNewRig(t, obxSumTemplate)
		obxJoinDirect(t, g, "sa-alpha")
		mdxWantLines(t, "/add sa-s", g.run("/add sa-s"), []string{"sa-s 开头的除总结成员 sa-sum 外没有可加入的；总结成员不用 /add：多个成员回复之后它会自己加入做总结"})
	})
	t.Run("prefix_all_present", func(t *testing.T) {
		g := mdxNewRig(t, obxSumTemplate)
		obxJoinDirect(t, g, "sa-alpha")
		obxJoinDirect(t, g, "sa-sum")
		mdxWantLines(t, "/add sa", g.run("/add sa"), []string{"sa 开头的都已经在当前会话里了"})
	})
	t.Run("prefix_none", func(t *testing.T) {
		g := mdxNewRig(t, obxSumTemplate)
		obxJoinDirect(t, g, "sa-alpha")
		mdxWantLines(t, "/add zz", g.run("/add zz"), []string{"配置里没有可加入的 zz，不带参数的 /add 能列出可加的"})
	})
	t.Run("summary_left", func(t *testing.T) {
		g := mdxNewRig(t, obxSumTemplate)
		obxJoinDirect(t, g, "sa-alpha")
		obxJoinDirect(t, g, "sa-sum")
		obxLeaveDirect(t, g, "sa-sum")
		obxWantFirst(t, g, sumLeft)
	})
	t.Run("summary_only_left", func(t *testing.T) {
		g := mdxNewRig(t, obxSumOnlyTemplate)
		obxJoinDirect(t, g, "sa-sum")
		obxLeaveDirect(t, g, "sa-sum")
		obxWantFirst(t, g, sumLeft)
	})
	t.Run("prefix_summary_left", func(t *testing.T) {
		g := mdxNewRig(t, obxSumTemplate)
		obxJoinDirect(t, g, "sa-alpha")
		obxJoinDirect(t, g, "sa-sum")
		obxLeaveDirect(t, g, "sa-sum")
		mdxWantLines(t, "/add sa", g.run("/add sa"), []string{"sa 开头的除总结成员 sa-sum 外没有可加入的；sa-sum 已退出当前会话，要恢复总结用 /join sa-sum"})
	})
	t.Run("summary_present", func(t *testing.T) {
		g := mdxNewRig(t, obxSumTemplate)
		mdxWantLines(t, "没加入过时 /add sa-sum", g.run("/add sa-sum"), []string{"sa-sum 是总结成员（config.toml 的 [summary]），不用 /add：多个成员回复之后它会自己加入做总结"})
		obxJoinDirect(t, g, "sa-sum")
		mdxWantLines(t, "在场时 /add sa-sum", g.run("/add sa-sum"), []string{"sa-sum 是总结成员（config.toml 的 [summary]），已经在当前会话里了"})
	})
}
