package repl

// clx_cl1_test.go 是 CL1（一批小修补，契约 §2.1）在 repl 包的验收：
//   - C1：/add <名字> --id 缺值时打用法（AD1 的 M9，19g 文件头），不识别、不写配置；
//   - C2：/team use 不把总结成员（[summary] agent）当普通成员加入（#47 在 /team use 漏了）；
//   - C9：/help 里 team 那一行说明总结成员除外（v1.1）。
// 只读复用 adn_addname_test.go（adnNewRig、adnRefused、adnAdded、adnNoDiscovery、adnUsage、adnBlockSol、
// adnDiscovering）、mdx_addmodel_fixture_test.go（mdxLines、mdxWantLines、mdxHasLine）、team_use_test.go（newTeamTrioConfig、
// newJoinTrackingStore、joinTrackingStore）与 conversation_test.go 的 fakeStore；不改它们。
// repl 的用例不起任何进程：C1 的识别用 mdxExec（adnNewRig 装好的假执行器），C2、C9 不识别。

import (
	"bytes"
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/conversation"
)

// TestClxAddIDMissingValue 是 C1（19g 文件头的 M9）：--id 是最后一个词、后面没有值时打按名字加的用法那一句，
// 执行器 0 次、配置文件字节与备份个数都不变。同一装置接着敲 --id 带值的同一条命令是正对照：识别两家、新建 sol、
// 写配置并加入（证明这个装置下按名字加是通的，「不识别、不写」来自守卫）。
// 【判别力】V2（--id 那一半去掉 i+1 >= len(fields)）：第一条命令在 parseAddNameArgs 里越界 panic，mdxRig.runCtx
// 接住 panic、Fatalf 判红（「dispatch("/add gpt-6.1-sol --id") panic: runtime error: index out of range ...」），
// 后面几条不再跑。--effort 那一半（V3）的杀手是 TestAdnFlags 的 /add gpt-6.1-sol --effort。
func TestClxAddIDMissingValue(t *testing.T) {
	g := adnNewRig(t, true)
	for _, cmd := range []string{"/add gpt-6.1-sol --id", "/add --id"} {
		adnRefused(t, g, cmd, []string{adnUsage})
	}
	adnNoDiscovery(t, g, "--id 缺值的两条")
	adnAdded(t, g, "/add gpt-6.1-sol --id sol", "sol", adnBlockSol, true)
}

// clxSumLine 是 /team use 跳过总结成员时打的那一行（契约 §3，逐字）。
func clxSumLine(id string) string {
	return id + " 是总结成员（config.toml 的 [summary]），不随 team 加入：多个成员回复之后它会自己加入做总结"
}

// clxTeamConv 是 C2 各子测试的会话 id（直接塞进 fakeStore，再 /use 它）。
const clxTeamConv = "convclx01"

// clxTeamRig 是 C2 的装置：newTeamTrioConfig（tm_trio = ag_alpha、ag_mid、ag_zeta）加上 summary 指定的总结成员
// （空串是没配），joinTrackingStore 记每一次 Join；seed 里的成员先以在场塞进会话，left 里的再标成已退出。
// 不经 Run：直接调 dispatch，按行取每条命令的输出增量（与 mdxRig.run 同一种取法）。
type clxTeamRig struct {
	t     *testing.T
	ctx   context.Context
	store *joinTrackingStore
	r     *REPL
	out   *bytes.Buffer
}

func clxNewTeamRig(t *testing.T, summary string, seed, left []string) *clxTeamRig {
	t.Helper()
	cfg := newTeamTrioConfig()
	cfg.Summary = config.SummaryConfig{Agent: summary}
	store := newJoinTrackingStore()
	ctx := context.Background()
	store.fakeStore.seedConversation(clxTeamConv, "CL1 team 与总结成员")
	for _, id := range seed {
		if err := store.fakeStore.AddParticipant(ctx, conversation.Participant{
			ConvID: clxTeamConv, AgentID: id, Provider: "fake", JoinedAt: replTestNow, Active: true,
		}); err != nil {
			t.Fatalf("装置：塞入 %s 失败: %v", id, err)
		}
	}
	for _, id := range left {
		if err := store.fakeStore.DeactivateParticipant(ctx, clxTeamConv, id, replTestNow.Add(1)); err != nil {
			t.Fatalf("装置：让 %s 退出失败: %v", id, err)
		}
	}
	out := &bytes.Buffer{}
	r := New(cfg, conversation.NewService(store), nil, strings.NewReader(""), out, RulesContext{})
	g := &clxTeamRig{t: t, ctx: ctx, store: store, r: r, out: out}
	g.run("/use " + clxTeamConv)
	return g
}

// run 喂一行，返回这一行的输出增量（按行）；dispatch 的 panic 与错误都判红。
func (g *clxTeamRig) run(line string) []string {
	g.t.Helper()
	before := g.out.Len()
	var panicked any
	var err error
	func() {
		defer func() { panicked = recover() }()
		err = g.r.dispatch(g.ctx, line)
	}()
	if panicked != nil {
		g.t.Fatalf("dispatch(%q) panic: %v", line, panicked)
	}
	if err != nil {
		g.t.Fatalf("dispatch(%q) 返回错误: %v", line, err)
	}
	return mdxLines(g.out.String()[before:])
}

// joins 是 /team use 期间每个成员各被 Join 了几次（只列次数非 0 的）。
func (g *clxTeamRig) joins() map[string]int {
	got := map[string]int{}
	for _, c := range g.store.joinCalls {
		if c.convID == clxTeamConv {
			got[c.agentID]++
		}
	}
	return got
}

// active 是会话里在场成员的 id（排序）。
func (g *clxTeamRig) active() []string {
	g.t.Helper()
	ps, err := g.store.Participants(g.ctx, clxTeamConv, true)
	if err != nil {
		g.t.Fatalf("Participants: %v", err)
	}
	var out []string
	for _, p := range ps {
		if p.Active {
			out = append(out, p.AgentID)
		}
	}
	sort.Strings(out)
	return out
}

// TestClxTeamUseSummaryMember 是 C2：tm_trio 的 members 里有总结成员 ag_alpha（PermissionRead，真配置也过得了校验）。
//   - not_member：ag_alpha 还不是本会话的成员。打 clxSumLine、不 Join 它；ag_mid、ag_zeta 照旧加入。
//   - no_summary：同一装置只差 [summary] 没配（正对照）：ag_alpha 照旧加入。证明「不 Join」来自总结成员这一条。
//   - present：ag_alpha 已在场（总结成员加入过）：输出与没配 [summary] 的同一装置逐行相同，不打 clxSumLine。
//   - left：ag_alpha 已退出：照旧打「已退出，未自动拉回」，不打 clxSumLine，不拉回。
//
// 【判别力】not_member 对 V4（去掉总结成员这一条：Join 了 ag_alpha）、V6（打了说明却不 continue：说明之后照样 Join）
// 红；present、left 对 V5（这一条挪到「已是成员」那一支之前：在场、已退出的总结成员也打 clxSumLine）红；V7（判据换成
// 「配了总结成员就跳过所有缺的成员」：ag_mid、ag_zeta 也打 clxSumLine、没加入）下 not_member、present、left 三条都红。
// no_summary 是 not_member 的对照，在这几条变异下都绿（它没配 [summary]）。
func TestClxTeamUseSummaryMember(t *testing.T) {
	const cmd = "/team use tm_trio"
	bound := "已绑定 team tm_trio"
	joined := func(id string) string { return id + " 已加入会话 " + clxTeamConv }

	t.Run("not_member", func(t *testing.T) {
		g := clxNewTeamRig(t, "ag_alpha", nil, nil)
		mdxWantLines(t, cmd, g.run(cmd), []string{bound, clxSumLine("ag_alpha"), joined("ag_mid"), joined("ag_zeta")})
		if got, want := g.joins(), map[string]int{"ag_mid": 1, "ag_zeta": 1}; !reflect.DeepEqual(got, want) {
			t.Errorf("Join 次数应当是 %v（总结成员 ag_alpha 一次都不 Join），实际 %v", want, got)
		}
		if got, want := g.active(), []string{"ag_mid", "ag_zeta"}; !reflect.DeepEqual(got, want) {
			t.Errorf("在场成员应当是 %q，实际 %q", want, got)
		}
	})

	t.Run("no_summary", func(t *testing.T) {
		g := clxNewTeamRig(t, "", nil, nil)
		mdxWantLines(t, cmd, g.run(cmd), []string{bound, joined("ag_alpha"), joined("ag_mid"), joined("ag_zeta")})
		if got, want := g.joins(), map[string]int{"ag_alpha": 1, "ag_mid": 1, "ag_zeta": 1}; !reflect.DeepEqual(got, want) {
			t.Errorf("没配 [summary] 时 Join 次数应当是 %v，实际 %v", want, got)
		}
	})

	t.Run("present", func(t *testing.T) {
		g := clxNewTeamRig(t, "ag_alpha", []string{"ag_alpha"}, nil)
		got := g.run(cmd)
		c := clxNewTeamRig(t, "", []string{"ag_alpha"}, nil)
		want := c.run(cmd)
		mdxWantLines(t, cmd+"（总结成员已在场）", got, []string{bound, joined("ag_mid"), joined("ag_zeta")})
		mdxWantLines(t, cmd+"（与没配 [summary] 的同一装置比）", got, want)
		if n := g.joins()["ag_alpha"]; n != 0 {
			t.Errorf("已在场的 ag_alpha 不该再 Join，实际 %d 次", n)
		}
	})

	t.Run("left", func(t *testing.T) {
		g := clxNewTeamRig(t, "ag_alpha", []string{"ag_alpha"}, []string{"ag_alpha"})
		mdxWantLines(t, cmd, g.run(cmd), []string{bound, "ag_alpha 已退出，未自动拉回，要加回用 /join", joined("ag_mid"), joined("ag_zeta")})
		if n := g.joins()["ag_alpha"]; n != 0 {
			t.Errorf("已退出的 ag_alpha 不该被拉回，实际 Join %d 次", n)
		}
		if got, want := g.active(), []string{"ag_mid", "ag_zeta"}; !reflect.DeepEqual(got, want) {
			t.Errorf("在场成员应当是 %q，实际 %q", want, got)
		}
	})
}

// clxHelpTeamLine 是 /help 输出里 team 那一整行（COMMAND 列按 tabwriter 补到 12 个字符；契约 §3 第 3 句）。
const clxHelpTeamLine = "/team       绑定当前会话到一个 team，并把尚未加入的成员（总结成员除外）一次性加进来: /team use <team-id>"

// TestClxHelpTeamLine 是 C9（v1.1）：/help 里 team 那一行说明总结成员不随 team 加入（C2 的行为在帮助里的说法）。
// 用 C2 的装置（没配 [summary]）敲 /help，按整行找。
// 【判别力】V17（team 的 summary 换回 HEAD 的旧句，不提总结成员）：没有与 clxHelpTeamLine 整行相等的一行，红。
func TestClxHelpTeamLine(t *testing.T) {
	g := clxNewTeamRig(t, "", nil, nil)
	lines := g.run("/help")
	if !mdxHasLine(lines, clxHelpTeamLine) {
		t.Errorf("/help 输出里没有与 clxHelpTeamLine 整行相等的一行：\n%s", strings.Join(lines, "\n"))
	}
}
