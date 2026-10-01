package repl

// obz_addlogin_test.go 是 OB3（00_DECISIONS.md #48：/add 的识别结果之后显示各家的登录状态）在 repl 包的验收：
// Z1 说法与登录办法、Z2 零值、Z3 登录段的位置与三种状态、Z4 codex 的登录状态命令、Z5 共用超时、
// Z6 识别失败的那一家、Z7 快照语义、Z8 取消。
// 装置只读复用 mdx_addmodel_fixture_test.go（mdxNewRig、mdxRig、mdxResp 与 g.exec 的 set、count，mdxWantLines、
// mdxHasLine、mdxReplaceOnce、mdxSplit、mdxConfigText、mdxConfigTemplate、mdxGrokOnlyTemplate、mdxClaudeFixture、
// mdxGrokFixture、mdxGrokPath、mdxAppliedLine）、cxh_fixture_test.go（cxhNewRig、cxhConfigTemplate、
// cxhCodexOnlyTemplate、cxhCodexPath、cxhCodexFixture、cxhListBlock、cxhGrokDownBlock、cxhOneGpt55）与
// obx_cliadvice_test.go（obxDetector、obxList、obxHintLines）。登录状态命令一律由 obzStatusExec 假执行器回应，
// 不起任何进程。

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/provider"
)

// obzHead 是登录段的段头（契约 §3）。
const obzHead = "各家的登录状态（本会话的快照，/add --refresh 重新检查）:"

// 登录段里各家的行（契约 §3 的格式套上各档的说法）。
const (
	obzAgyUnknown   = "  agy     登录状态查不出"
	obzClaudeSub    = "  claude  已登录（订阅：Claude Obz）"
	obzGrokNo       = "  grok    没登录；在终端里运行 grok login，登录之后 /add --refresh"
	obzGrokKey      = "  grok    已登录（API key）"
	obzCodexYes     = "  codex   已登录"
	obzCodexKey     = "  codex   已登录（API key）"
	obzCodexNo      = "  codex   没登录；在终端里运行 codex login，登录之后 /add --refresh"
	obzCodexUnknown = "  codex   登录状态查不出"
)

// obzCanceledLine 是识别被取消时那一行（既有字面量）。
const obzCanceledLine = "识别被取消；下次 /add 会重新识别"

// obzCodexOnlyBlock 是只有 codex 一家、识别出 cxhOneGpt55 那一条（已配成成员）时的识别部分（既有字面量）。
var obzCodexOnlyBlock = []string{"正在识别模型: codex", "识别出的模型都已经在配置里了"}

// obzRefreshHint 是有失败时失败行之后那一行（既有字面量）。
const obzRefreshHint = "失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh"

// obzStatusResp 是假登录状态执行器的回应。wait 为真时先等 ctx 结束（返回 ctx.Err()），
// 3 s 还没结束就照其余几项返回：没被这一家的超时约束时，结果看得见。
type obzStatusResp struct {
	code           int
	stdout, stderr string
	err            error
	wait           bool
}

// obzStatusExec 是 provider.LoginStatusExec 的假实现。每次调用的五步顺序冻结：
// 记录调用、ctx 有没有 deadline 与 deadline 的值（加锁）-> 钩子 -> 按次记下此刻 ctx 是否已结束（加锁）
// -> ctx 已结束就返回 ctx.Err() -> 按 resp 返回。
type obzStatusExec struct {
	mu         sync.Mutex
	calls      []provider.DiscoveryCommand
	deadlines  []bool
	deadlineAt []time.Time
	canceled   []bool
	resp       obzStatusResp
	hook       func()
}

func (e *obzStatusExec) run(ctx context.Context, c provider.DiscoveryCommand) (int, []byte, []byte, error) {
	e.mu.Lock()
	e.calls = append(e.calls, c)
	at, has := ctx.Deadline()
	e.deadlines = append(e.deadlines, has)
	e.deadlineAt = append(e.deadlineAt, at)
	resp, hook := e.resp, e.hook
	e.mu.Unlock()
	if hook != nil {
		hook()
	}
	e.mu.Lock()
	e.canceled = append(e.canceled, ctx.Err() != nil)
	e.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return 0, nil, nil, err
	}
	if resp.wait {
		select {
		case <-ctx.Done():
			return 0, nil, nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return resp.code, []byte(resp.stdout), []byte(resp.stderr), resp.err
}

func (e *obzStatusExec) set(r obzStatusResp) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.resp = r
}

func (e *obzStatusExec) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.calls)
}

// obzCanceledCopy 是按次记下的「钩子之后 ctx 是否已结束」的副本。
func obzCanceledCopy(e *obzStatusExec) []bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]bool(nil), e.canceled...)
}

// obzInstall 给装置装上假登录状态执行器（经 SetLoginStatusExec）。
func obzInstall(g *mdxRig, r obzStatusResp) *obzStatusExec {
	e := &obzStatusExec{resp: r}
	g.r.SetLoginStatusExec(e.run)
	return e
}

// obzCodexCmd 是 codex 登录状态命令应有的样子：配置里的 command，参数 login status，stdin 为空。
var obzCodexCmd = provider.DiscoveryCommand{Path: cxhCodexPath, Args: []string{"login", "status"}}

// obzWantCodexCalls 断言假执行器恰好被调 n 次、每次都是 obzCodexCmd、ctx 都带 deadline。
// 它只查有没有 deadline；与识别的 deadline 是不是同一个，由 Z5 的 shared_deadline 查。
func obzWantCodexCalls(t *testing.T, e *obzStatusExec, n int) {
	t.Helper()
	e.mu.Lock()
	calls := append([]provider.DiscoveryCommand(nil), e.calls...)
	deadlines := append([]bool(nil), e.deadlines...)
	e.mu.Unlock()
	if len(calls) != n {
		t.Fatalf("登录状态命令应当恰好跑 %d 次，实际 %d 次：%+v", n, len(calls), calls)
	}
	for i, c := range calls {
		if !reflect.DeepEqual(c, obzCodexCmd) {
			t.Errorf("第 %d 次登录状态命令是 %+v，期望 %+v", i+1, c, obzCodexCmd)
		}
		if !deadlines[i] {
			t.Errorf("第 %d 次登录状态命令的 ctx 没有 deadline", i+1)
		}
	}
}

// obzClaudeSubFixture 是 mdxClaudeFixture 在 council-models 回应里加上 account（订阅 Claude Obz）。
func obzClaudeSubFixture(t *testing.T) string {
	t.Helper()
	return mdxReplaceOnce(t, mdxClaudeFixture, `]},"pending_permission_requests"`, `],"account":{"subscriptionType":"Claude Obz"}},"pending_permission_requests"`)
}

// obzGrokNoFixture 是 mdxGrokFixture 把首行换成 You are not authenticated.（模型行不变）。
func obzGrokNoFixture(t *testing.T) string {
	t.Helper()
	return mdxReplaceOnce(t, mdxGrokFixture, "Model 'grok-4.7' is using its own API key.", "You are not authenticated.")
}

// obzSection 拼出登录段：段头加各行。
func obzSection(rows ...string) []string {
	return append([]string{obzHead}, rows...)
}

// TestObzLoginText 是 Z1：LoginText 的四档六种说法与 LoginCommand 的四家（不认识的 id 是空串）。
// 【判别力】Q20-Q23（说法）、Q24（登录办法）。
func TestObzLoginText(t *testing.T) {
	for _, c := range []struct {
		name string
		in   provider.LoginInfo
		want string
	}{
		{"yes_sub", provider.LoginInfo{State: provider.LoginYes, Subscription: "Claude Obz"}, "已登录（订阅：Claude Obz）"},
		{"yes", provider.LoginInfo{State: provider.LoginYes}, "已登录"},
		{"key_source", provider.LoginInfo{State: provider.LoginAPIKey, KeySource: "ANTHROPIC_API_KEY"}, "已登录（API key：ANTHROPIC_API_KEY）"},
		{"key", provider.LoginInfo{State: provider.LoginAPIKey}, "已登录（API key）"},
		{"no", provider.LoginInfo{State: provider.LoginNo}, "没登录"},
		{"unknown", provider.LoginInfo{}, "登录状态查不出"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := LoginText(c.in); got != c.want {
				t.Errorf("LoginText(%+v) = %q，期望 %q", c.in, got, c.want)
			}
		})
	}
	for id, want := range map[string]string{
		"claude": "claude auth login",
		"grok":   "grok login",
		"agy":    "agy（按提示登录）",
		"codex":  "codex login",
		"obz":    "",
	} {
		if got := LoginCommand(id); got != want {
			t.Errorf("LoginCommand(%q) = %q，期望 %q", id, got, want)
		}
	}
}

// TestObzSetLoginStatusExecZeroValue 是 Z2：直接 New 出来的 REPL 不做登录检测；SetLoginStatusExec 装得上。
// 【判别力】Q12（setter 不赋值）。
func TestObzSetLoginStatusExecZeroValue(t *testing.T) {
	r := New(nil, nil, nil, strings.NewReader(""), nil, RulesContext{})
	if r.loginStatus != nil {
		t.Errorf("New 出来的 REPL 的 loginStatus 应当是 nil")
	}
	e := &obzStatusExec{}
	r.SetLoginStatusExec(e.run)
	if r.loginStatus == nil {
		t.Errorf("SetLoginStatusExec 之后 loginStatus 不该是 nil")
	}
}

// TestObzAddLoginSection 是 Z3：MD1 的配置（agy、claude、grok），claude 已登录（订阅）、grok 没登录、agy 查不出。
// g0 不装执行器、g 装；两者都装 OB1 的检测器桩（codex 装了、配置里没有，末尾三行提示）。
// g 的输出等于 g0 的输出在最后三行提示之前插入登录段；g0 没有登录段（g 是它的正对照）。
// 三家都不跑登录状态命令（只有 codex 跑）。
// 【判别力】Q4、Q5、Q12、Q13、Q14、Q15、Q16、Q22、Q23、Q25。
func TestObzAddLoginSection(t *testing.T) {
	setup := func(withExec bool) (*mdxRig, *obzStatusExec) {
		g := mdxNewRig(t, mdxConfigTemplate)
		g.exec.set(g.r.cfg.Providers["claude"].Command, mdxResp{out: []byte(obzClaudeSubFixture(t))})
		g.exec.set(mdxGrokPath, mdxResp{out: []byte(obzGrokNoFixture(t))})
		calls := 0
		g.r.SetCLIDetector(obxDetector(obxList, &calls))
		var e *obzStatusExec
		if withExec {
			e = obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"})
		}
		return g, e
	}
	g0, _ := setup(false)
	base := g0.run("/add")
	if mdxHasLine(base, obzHead) {
		t.Errorf("不装执行器时不该有登录段：%q", base)
	}
	if len(base) < 3 {
		t.Fatalf("前提不成立：g0 的 /add 输出不足 3 行：%q", base)
	}
	mdxWantLines(t, "前提：g0 的最后三行是 codex 的提示", base[len(base)-3:], obxHintLines(g0.cfgPath)[3:])

	g, e := setup(true)
	got := g.run("/add")
	want := append([]string(nil), base[:len(base)-3]...)
	want = append(want, obzSection(obzAgyUnknown, obzClaudeSub, obzGrokNo)...)
	want = append(want, obxHintLines(g.cfgPath)[3:]...)
	mdxWantLines(t, "装了执行器的 /add", got, want)
	if n := e.count(); n != 0 {
		t.Errorf("agy、claude、grok 都不跑登录状态命令，实际跑了 %d 次", n)
	}
}

// TestObzAddLoginCodex 是 Z4：CXH 的配置（codex、grok），codex 的登录状态命令五种结果；grok 是 MD1 的原夹具
// （经 CPA 的首行），已登录（API key）。识别部分整段等于 CXH 的 R1 块加登录段；命令恰好跑一次、形状对、ctx 带 deadline。
// exec_error 的回应同时带了 ChatGPT 那一句：执行器报错时不看输出（chatgpt 是只差 err 的正对照）。
// 【判别力】五个子测试都对 Q4、Q6、Q7、Q11、Q12、Q14、Q16、Q20、Q25；chatgpt、api_key、not_logged_in 另对 Q10；
// exec_error 另对 Q9；not_logged_in 另对 Q15、Q23、Q24。
func TestObzAddLoginCodex(t *testing.T) {
	for _, c := range []struct {
		name string
		resp obzStatusResp
		row  string
	}{
		{"chatgpt", obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"}, obzCodexYes},
		{"api_key", obzStatusResp{code: 0, stderr: "Logged in using an API key - sk-obz***\n"}, obzCodexKey},
		{"not_logged_in", obzStatusResp{code: 1, stderr: "Not logged in\n"}, obzCodexNo},
		{"exec_error", obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n", err: errors.New("obz exec failed")}, obzCodexUnknown},
		{"unparseable", obzStatusResp{code: 0, stdout: "obz\n"}, obzCodexUnknown},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := cxhNewRig(t, cxhConfigTemplate, cxhCodexFixture())
			e := obzInstall(g, c.resp)
			got := g.afterCandidates(g.run("/add"))
			want := append(mdxSplit(cxhListBlock), obzSection(c.row, obzGrokKey)...)
			mdxWantLines(t, "识别部分", got, want)
			obzWantCodexCalls(t, e, 1)
		})
	}
}

// TestObzAddLoginSharedTimeout 是 Z5：登录状态命令与识别共用这一家的 discoverTimeout（装置是 200 ms）。
// hang：命令挂住，这一家的 ctx 到期时被打断，codex 是查不出，/add 远在假执行器 3 s 兜底之前返回；
// 没被约束时 3 s 后返回 ChatGPT 那一句，那一行会变成已登录。
// discovery_timeout：codex 的识别本身超时，ctx 已到期，登录状态命令一次都不跑。
// shared_deadline：识别拖 50 ms 再返回，登录状态命令拿到的 deadline 与识别的 deadline 相等（Equal），
// 不是另起的一段。
// 【判别力】hang 对 Q6、Q7、Q11、Q12、Q14、Q16、Q25；discovery_timeout 对 Q6、Q8、Q12、Q14、Q16、Q17、Q25；
// shared_deadline 对 Q6、Q7、Q10、Q11、Q12、Q16、Q25、Q32、Q34（Q34 只有它杀得到；去掉那 50 ms 时，
// 原型上它连跑 5 次都杀不到 Q32、Q34）。
func TestObzAddLoginSharedTimeout(t *testing.T) {
	t.Run("hang", func(t *testing.T) {
		g := cxhNewRig(t, cxhCodexOnlyTemplate, cxhOneGpt55)
		e := obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n", wait: true})
		start := time.Now()
		got := g.afterCandidates(g.run("/add"))
		el := time.Since(start)
		want := append([]string(nil), obzCodexOnlyBlock...)
		want = append(want, obzSection(obzCodexUnknown)...)
		mdxWantLines(t, "识别部分", got, want)
		if el >= 2*time.Second {
			t.Errorf("/add 用了 %v，应当小于 2s（登录状态命令受这一家的 200ms 约束）", el)
		}
		obzWantCodexCalls(t, e, 1)
	})
	t.Run("discovery_timeout", func(t *testing.T) {
		g := cxhNewRig(t, cxhCodexOnlyTemplate, cxhOneGpt55)
		g.exec.set(cxhCodexPath, mdxResp{timeout: true})
		e := obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"})
		got := g.afterCandidates(g.run("/add"))
		want := []string{
			"正在识别模型: codex",
			"识别 codex 的模型失败: 超时（200ms 内没有回来）",
			obzRefreshHint,
		}
		want = append(want, obzSection(obzCodexUnknown)...)
		mdxWantLines(t, "识别部分", got, want)
		if n := e.count(); n != 0 {
			t.Errorf("识别超时之后这一家的 ctx 已到期，登录状态命令不该跑，实际跑了 %d 次", n)
		}
	})
	t.Run("shared_deadline", func(t *testing.T) {
		g := cxhNewRig(t, cxhCodexOnlyTemplate, cxhOneGpt55)
		e := obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"})
		var mu sync.Mutex
		var disc []time.Time
		inner := g.r.discover
		g.r.discover = func(c context.Context, cmd provider.DiscoveryCommand) ([]byte, error) {
			d, ok := c.Deadline()
			mu.Lock()
			if ok {
				disc = append(disc, d)
			}
			mu.Unlock()
			out, err := inner(c, cmd)
			// 识别拖 50 ms 再返回：登录状态命令若另起一段新的超时，它的 deadline 会比识别的晚约 50 ms。
			// 不拖时两个 deadline 在 Windows 的时钟精度下可能相等，比不出来。
			time.Sleep(50 * time.Millisecond)
			return out, err
		}
		got := g.afterCandidates(g.run("/add"))
		want := append([]string(nil), obzCodexOnlyBlock...)
		want = append(want, obzSection(obzCodexYes)...)
		mdxWantLines(t, "识别部分", got, want)
		obzWantCodexCalls(t, e, 1)
		mu.Lock()
		discAt := append([]time.Time(nil), disc...)
		mu.Unlock()
		if len(discAt) != 1 {
			t.Fatalf("前提不成立：识别应当恰好一次、ctx 带 deadline，实际记下 %d 个 deadline", len(discAt))
		}
		e.mu.Lock()
		loginAt := e.deadlineAt[0]
		e.mu.Unlock()
		if !loginAt.Equal(discAt[0]) {
			t.Errorf("登录状态命令的 deadline 应当就是识别的 deadline：识别 %v，登录状态命令 %v", discAt[0], loginAt)
		}
	})
}

// TestObzAddLoginDiscoveryFailed 是 Z6：识别失败的那一家照样有一行。
// grok_empty：grok 只打了没登录那一句、一个模型都没有，识别失败，登录状态从截下的 stdout 判出没登录。
// grok_exit_error：grok 的识别报错（exit code 1）但 stdout 有没登录那一句与一个模型，登录状态照样从 stdout
// 判出没登录（与首次运行同一个判据，app 包的 Z13 是首次运行那一侧）。
// codex_failed：codex 的识别报错，登录状态命令照样跑，已登录。
// 【判别力】grok_empty 对 Q4、Q12、Q15、Q16、Q17、Q23；grok_exit_error 对 Q4、Q12、Q15、Q16、Q17、Q23、Q35
// （Q35 只有它杀得到）；codex_failed 对 Q6、Q7、Q10、Q11、Q12、Q16、Q17、Q25。
func TestObzAddLoginDiscoveryFailed(t *testing.T) {
	t.Run("grok_empty", func(t *testing.T) {
		g := mdxNewRig(t, mdxGrokOnlyTemplate)
		g.exec.set(mdxGrokPath, mdxResp{out: []byte("You are not authenticated.\n\nAvailable models:\n")})
		e := obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"})
		got := g.afterCandidates(g.run("/add"))
		want := []string{
			"正在识别模型: grok",
			"识别 grok 的模型失败: 一个模型都没列出来（没登录？）",
			obzRefreshHint,
		}
		want = append(want, obzSection(obzGrokNo)...)
		mdxWantLines(t, "识别部分", got, want)
		if n := e.count(); n != 0 {
			t.Errorf("grok 不跑登录状态命令，实际跑了 %d 次", n)
		}
	})
	t.Run("grok_exit_error", func(t *testing.T) {
		g := mdxNewRig(t, mdxGrokOnlyTemplate)
		g.r.discover = func(context.Context, provider.DiscoveryCommand) ([]byte, error) {
			return []byte("You are not authenticated.\n\nAvailable models:\n  * grok-obz (default)\n"), errors.New("exit code 1")
		}
		e := obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"})
		got := g.afterCandidates(g.run("/add"))
		want := []string{
			"正在识别模型: grok",
			"识别 grok 的模型失败: model discovery command failed: exit code 1",
			obzRefreshHint,
		}
		want = append(want, obzSection(obzGrokNo)...)
		mdxWantLines(t, "识别部分", got, want)
		if n := e.count(); n != 0 {
			t.Errorf("grok 不跑登录状态命令，实际跑了 %d 次", n)
		}
	})
	t.Run("codex_failed", func(t *testing.T) {
		g := cxhNewRig(t, cxhCodexOnlyTemplate, cxhOneGpt55)
		g.exec.set(cxhCodexPath, mdxResp{err: errors.New("obz codex down")})
		e := obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"})
		got := g.afterCandidates(g.run("/add"))
		want := []string{
			"正在识别模型: codex",
			"识别 codex 的模型失败: model discovery command failed: obz codex down",
			obzRefreshHint,
		}
		want = append(want, obzSection(obzCodexYes)...)
		mdxWantLines(t, "识别部分", got, want)
		obzWantCodexCalls(t, e, 1)
	})
}

// TestObzAddLoginSnapshot 是 Z7：登录状态随快照留到本会话结束。
// 第 2 步：执行器的回应换成没登录，再 /add 不重新识别、不重跑命令，登录段逐字不变。
// 第 3 步：配置里删掉 grok（换成只有 codex 的配置）并 /config reload --apply，快照不作废，登录段仍有 grok 那一行
// （只照快照打，不按当前配置滤）。
// 第 4 步：/add --refresh 重拍，只剩 codex，而且是没登录。
// 【判别力】Q18（登录段按当前配置滤）、Q19（reload 作废快照）；第 4 步钉 --refresh 重跑命令。另有 Q4、Q6、Q7、
// Q10、Q11、Q12、Q14、Q15、Q16、Q20、Q23、Q24、Q25。
func TestObzAddLoginSnapshot(t *testing.T) {
	g := cxhNewRig(t, cxhConfigTemplate, cxhCodexFixture())
	e := obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"})
	first := obzSection(obzCodexYes, obzGrokKey)

	got := g.afterCandidates(g.run("/add"))
	mdxWantLines(t, "第 1 步", got, append(mdxSplit(cxhListBlock), first...))
	obzWantCodexCalls(t, e, 1)
	discovered := g.exec.count()

	e.set(obzStatusResp{code: 1, stderr: "Not logged in\n"})
	got = g.afterCandidates(g.run("/add"))
	mdxWantLines(t, "第 2 步", got, append(mdxSplit(cxhListBlock)[1:], first...))
	obzWantCodexCalls(t, e, 1)

	g.writeFile([]byte(mdxConfigText(cxhCodexOnlyTemplate, g.dir)))
	reload := g.run("/config reload --apply")
	if !mdxHasLine(reload, mdxAppliedLine) {
		t.Fatalf("前提不成立：/config reload --apply 没有应用：%q", reload)
	}
	got = g.run("/add")
	if mdxHasLine(got, "正在识别模型: codex") || mdxHasLine(got, "正在识别模型: codex, grok") {
		t.Errorf("/config reload --apply 不该作废快照：%q", got)
	}
	if len(got) < 3 {
		t.Fatalf("第 3 步的输出不足 3 行：%q", got)
	}
	mdxWantLines(t, "第 3 步的最后三行", got[len(got)-3:], first)
	obzWantCodexCalls(t, e, 1)
	if n := g.exec.count(); n != discovered {
		t.Errorf("第 3 步不该重新识别：识别次数从 %d 变成 %d", discovered, n)
	}

	// 只有 codex 一家时的识别部分：CXH R9 的块（grok 失败、表里只有 codex 的行）去掉头一行与末尾两行 grok 的失败。
	down := mdxSplit(cxhGrokDownBlock)
	if down[0] != "正在识别模型: codex, grok" || down[len(down)-2] != "识别 grok 的模型失败: model discovery command failed: cxh grok down" {
		t.Fatalf("前提不成立：cxhGrokDownBlock 的头尾不是预期的样子：%q", down)
	}
	got = g.afterCandidates(g.run("/add --refresh"))
	want := append([]string{"正在识别模型: codex"}, down[1:len(down)-2]...)
	want = append(want, obzSection(obzCodexNo)...)
	mdxWantLines(t, "第 4 步", got, want)
	obzWantCodexCalls(t, e, 2)
}

// TestObzAddLoginCanceled 是 Z8：codex 的登录状态命令期间父 ctx 被取消，整份快照丢掉，只打「识别被取消」。
// not_last：codex、grok 两家，取消落在 codex 那一家，由循环头接住，grok 不再识别。
// last：只有 codex 一家，循环不再回到开头，由拍完之后那一处接住。
// 两种都不存快照：下一次 /add 重新识别，登录段照常出现。
// 钩子取消父 ctx 之后，登录状态命令的 ctx 已经结束：假执行器按次记下，钩子那一次是 true；没有钩子的
// 下一次是 false（正对照）。
// 【判别力】not_last 对 Q26（识别次数）；两个子测试都对 Q27、Q32、Q33（Q33 只有 Z8 杀得到），另有 Q6、Q7、
// Q10、Q11、Q12、Q16、Q25。
func TestObzAddLoginCanceled(t *testing.T) {
	for _, c := range []struct {
		name, tmpl, out, head string
		row                   []string
	}{
		{"not_last", cxhConfigTemplate, cxhCodexFixture(), "正在识别模型: codex, grok", obzSection(obzCodexYes, obzGrokKey)},
		{"last", cxhCodexOnlyTemplate, cxhOneGpt55, "正在识别模型: codex", obzSection(obzCodexYes)},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := cxhNewRig(t, c.tmpl, c.out)
			e := obzInstall(g, obzStatusResp{code: 0, stderr: "Logged in using ChatGPT\n"})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e.hook = cancel
			got := g.afterCandidates(g.runCtx(ctx, "/add"))
			mdxWantLines(t, "被取消的那一次", got, []string{c.head, obzCanceledLine})
			if g.r.modelSnap != nil {
				t.Errorf("被取消的那一次不该存快照")
			}
			if n := g.exec.count(); n != 1 {
				t.Errorf("取消落在 codex 那一家，之后不该再识别别家：识别次数 %d，期望 1", n)
			}
			obzWantCodexCalls(t, e, 1)
			if cc := obzCanceledCopy(e); len(cc) != 1 || !cc[0] {
				t.Errorf("钩子取消父 ctx 之后，登录状态命令的 ctx 应当已经结束（按次记下的是 %v，期望 [true]）", cc)
			}

			e.mu.Lock()
			e.hook = nil
			e.mu.Unlock()
			again := g.afterCandidates(g.run("/add"))
			if len(again) == 0 || again[0] != c.head {
				t.Fatalf("取消之后的下一次 /add 应当重新识别，第一行是 %q，实际 %q", c.head, again)
			}
			if len(again) < len(c.row) {
				t.Fatalf("下一次 /add 的识别部分不足 %d 行：%q", len(c.row), again)
			}
			mdxWantLines(t, "下一次 /add 的登录段", again[len(again)-len(c.row):], c.row)
			// 正对照：没有钩子的那一次，登录状态命令的 ctx 没结束，记下的是 false。
			if cc := obzCanceledCopy(e); !reflect.DeepEqual(cc, []bool{true, false}) {
				t.Errorf("两次登录状态命令按次记下的 ctx 是否已结束是 %v，期望 [true false]", cc)
			}
		})
	}
}
