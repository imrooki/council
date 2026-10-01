package repl

// 本文件是 UX-2 修订版（冻结契约第三版，E:\multi_model_cowork\ux2r_contract.md）
// 测试腿的公共夹具。背对背：断言只来自契约文字，不读实现腿的产出。
//
// 为什么不用 runREPLRunner / newTestRunner：它们写死了 newTestREPLConfig()，
// 而本刀的用例需要 opus-a、grok-4.7、opus-5-0 这类自定义 id（契约「夹具约定」）。
// 自造配置时 REPL 与 Runner 必须拿**同一份** cfg：两份配置对「这个 id 在不在」
// 答得不一样时，turn_usage 计数会测到 Runner 的配置查找而不是 REPL 的闸。
//
// provider id 一律用 pv_alpha：claude / grok / agy 在 provider registry 里登记过，
// 走到 Ask 会真去起进程。pv_alpha 没登记，Ask 会在 provider.Lookup 那一步被拒绝，
// 但仍然经 refuse -> persistTurn 写下一行 turn_usage（x1_ask_membership_test.go
// 的自检证明了这条通道）。本包的「前缀可用」用例（验收 7）在自定义配置下断言
// turn_usage +1，顺带证明这条通道在本文件的夹具形状下同样成立。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/turn"
)

// ux2rProvider 是本刀全部自定义配置用的 provider id，没有在 registry 里登记。
const ux2rProvider = "pv_alpha"

// ux2rConvID 不含任何一个用例里用到的 agent id 的文本：/use 会回显会话 id，
// 若会话 id 里含 agent 名，下面按行或按子串的断言会被它抢先命中。
const ux2rConvID = "conv_ux2r_session"

// 以下格式串全部照抄契约第 4.3 节与第 6.2、6.3 节冻结的措辞。
const (
	ux2rFmtAmbiguous      = "%s 有歧义，可能是：%s"
	ux2rFmtAskUnknown     = "配置里没有 %s 这个 agent，用 /agents 看看有哪些"
	ux2rFmtAskLeft        = "%s 已退出当前会话，用 /join %s 重新加入"
	ux2rFmtAskNotJoined   = "%s 在配置里但还没加入当前会话，用 /join %s 加入"
	ux2rFmtAsking         = "正在问 %s ..."
	ux2rFmtJoined         = "%s 已加入会话 %s"
	ux2rFmtJoinNoConfig   = "配置里没有 agent %s，用 /agents 看看有哪些"
	ux2rFmtOutOfRange     = "编号 %s 超出范围（1-%d）"
	ux2rFmtOutOfRangeNone = "编号 %s 超出范围（没有可加入的）"
	ux2rFmtAllPresentPfx  = "%s 开头的都已经在当前会话里了"
	ux2rFmtAddNotFound    = "配置里没有可加入的 %s，不带参数的 /add 能列出可加的"
	ux2rMsgAllPresent     = "配置里的 agent 都已经在当前会话里了"
	ux2rMsgAddUsage       = "用法: /add 列出可加入的；/add <全名|编号|前缀>[,...] 一次加多个"
	ux2rTableHeader       = "NO\tAGENT\tPROVIDER\tDISPLAY_NAME"
)

// ux2rFullComma 是全角逗号。写成码位数字，不写反斜杠 u 转义：本仓库踩过编辑链路
// 把那种转义还原成真字符的坑（契约第 2 节）。
const ux2rFullComma rune = 0xFF0C

// ux2rConfig 造一份只含给定 agent id 的配置。display_name 按出现顺序取 DN1、DN2……，
// 故意不含 agent id 的文本：/add 的表格会打出 display_name，名字里若含 id，
// 「输出不含 opus-5-0-max」这类断言会被它误伤。
func ux2rConfig(ids ...string) *config.Config {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{ux2rProvider: {Command: "exe-ux2r.exe"}},
		Agents:    make(map[string]config.AgentConfig, len(ids)),
		Teams:     map[string]config.TeamConfig{},
	}
	for i, id := range ids {
		cfg.Agents[id] = config.AgentConfig{
			DisplayName: fmt.Sprintf("DN%d", i+1),
			Provider:    ux2rProvider,
			Model:       "mod-ux2r",
			Effort:      "eff-ux2r",
			Role:        "role ux2r",
			Permissions: config.PermissionRead,
			Mode:        config.ModeDiscussion,
		}
	}
	return cfg
}

// ux2rStore 在 fakeStore 外面包一层，记下每一次 AddParticipant，并能让第 N 次失败。
//
// WriteTx 必须覆写：fakeStore.WriteTx 把内嵌的 *fakeStore 自己当 Tx 传给 fn，
// 不覆写的话 Service.Join 调的是内层的 AddParticipant，记账形同虚设
// （team_use_test.go 的 joinTrackingStore 同一个理由）。
// seedParticipant 是 *fakeStore 的方法，直接调内层的 AddParticipant，不计数 ——
// 这正是要的：种数据不算「命令调了 Join」。
type ux2rStore struct {
	*fakeStore
	addCalls   []string
	failOnCall int // 第几次 AddParticipant 返回错误；0 表示从不失败
}

var (
	_ conversation.Store = (*ux2rStore)(nil)
	_ conversation.Tx    = (*ux2rStore)(nil)
)

var errUx2rAddInjected = errors.New("ux2r 用例注入的 AddParticipant 失败")

func (s *ux2rStore) WriteTx(ctx context.Context, fn func(context.Context, conversation.Tx) error) error {
	return fn(ctx, s)
}

func (s *ux2rStore) AddParticipant(ctx context.Context, p conversation.Participant) error {
	s.addCalls = append(s.addCalls, p.AgentID)
	if s.failOnCall > 0 && len(s.addCalls) == s.failOnCall {
		return errUx2rAddInjected
	}
	return s.fakeStore.AddParticipant(ctx, p)
}

// ux2rFix 是一套自定义配置的夹具：同一份 cfg 同时交给 REPL 与 Runner。
type ux2rFix struct {
	cfg *config.Config
	st  *ux2rStore
	svc *conversation.Service
	rn  *turn.Runner
}

func ux2rNewFix(t *testing.T, ids ...string) *ux2rFix {
	t.Helper()
	cfg := ux2rConfig(ids...)
	st := &ux2rStore{fakeStore: newFakeStore()}
	svc := conversation.NewService(st)
	rn := turn.NewRunner(cfg, svc, t.TempDir(), turn.RulesBlocks{})
	st.seedConversation(ux2rConvID, "ux2r")
	return &ux2rFix{cfg: cfg, st: st, svc: svc, rn: rn}
}

// seedActive 直接往库里塞一个在场成员，provider 取给定值（空串时取配置里的值）。
// 不经 /join：/join 的回显也含 agent id，会污染对输出的断言。
func (fx *ux2rFix) seedActive(t *testing.T, id, provider string) {
	t.Helper()
	if provider == "" {
		provider = fx.cfg.Agents[id].Provider
	}
	err := fx.st.fakeStore.AddParticipant(context.Background(), conversation.Participant{
		ConvID:      ux2rConvID,
		AgentID:     id,
		DisplayName: fx.cfg.Agents[id].DisplayName,
		Provider:    provider,
		Active:      true,
		JoinedAt:    replTestNow,
	})
	if err != nil {
		t.Fatalf("塞入在场成员 %s 失败: %v", id, err)
	}
}

// seedLeft 塞一个已退出的成员：先加入再退出。
func (fx *ux2rFix) seedLeft(t *testing.T, id, provider string) {
	t.Helper()
	fx.seedActive(t, id, provider)
	if err := fx.st.DeactivateParticipant(context.Background(), ux2rConvID, id, replTestNow.Add(time.Minute)); err != nil {
		t.Fatalf("塞入退出状态 %s 失败: %v", id, err)
	}
}

// usage 读回 id 名下的 turn_usage 行数。这是判「Ask 有没有被调」的观测通道。
func (fx *ux2rFix) usage(t *testing.T, id string) int {
	t.Helper()
	rows, err := fx.svc.TurnUsage(context.Background(), ux2rConvID, id, -1)
	if err != nil {
		t.Fatalf("读 TurnUsage(%s) 失败: %v", id, err)
	}
	return len(rows)
}

// usages 一次取多个 id 的 turn_usage 行数。
func (fx *ux2rFix) usages(t *testing.T, ids ...string) map[string]int {
	t.Helper()
	m := make(map[string]int, len(ids))
	for _, id := range ids {
		m[id] = fx.usage(t, id)
	}
	return m
}

// requireUsageUnchanged 断言每个 id 的 turn_usage 行数与 before 相同。
func (fx *ux2rFix) requireUsageUnchanged(t *testing.T, before map[string]int, out string) {
	t.Helper()
	for id, n := range before {
		if got := fx.usage(t, id); got != n {
			t.Errorf("%s 的 turn_usage 从 %d 变成 %d：Ask 被调过（以这个 id 起了一轮）；完整输出:\n%s", id, n, got, out)
		}
	}
}

// participant 取库里 id 这一行。
func (fx *ux2rFix) participant(t *testing.T, id string) (conversation.Participant, bool) {
	t.Helper()
	ps, err := fx.st.fakeStore.Participants(context.Background(), ux2rConvID, true)
	if err != nil {
		t.Fatalf("读 participants 失败: %v", err)
	}
	for _, p := range ps {
		if p.AgentID == id {
			return p, true
		}
	}
	return conversation.Participant{}, false
}

// activeIDs 返回当前在场成员 id 的字典序列表。
func (fx *ux2rFix) activeIDs(t *testing.T) []string {
	t.Helper()
	ps, err := fx.st.fakeStore.Participants(context.Background(), ux2rConvID, false)
	if err != nil {
		t.Fatalf("读 participants 失败: %v", err)
	}
	ids := make([]string, 0, len(ps))
	for _, p := range ps {
		ids = append(ids, p.AgentID)
	}
	sort.Strings(ids)
	return ids
}

// run 先 /use 本夹具的会话，再喂 input，返回完整输出。Run 返回非 nil（含 panic
// 被收成的错误）直接判失败：本刀所有路径都不该让 REPL 退出或 panic。
func (fx *ux2rFix) run(t *testing.T, input string) string {
	t.Helper()
	var out bytes.Buffer
	r := New(fx.cfg, fx.svc, fx.rn, strings.NewReader("/use "+ux2rConvID+"\n"+input), &out, RulesContext{})

	done := make(chan error, 1)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				done <- fmt.Errorf("REPL.Run panic: %v", rec)
			}
		}()
		done <- r.Run(context.Background())
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run 应当返回 nil，实际 %v；输出:\n%s", err, out.String())
		}
		return out.String()
	case <-time.After(testREPLTimeout):
		t.Fatalf("REPL.Run 在 %v 内没有返回", testREPLTimeout)
		return ""
	}
}

// ux2rLines 把输出切成行，并剥掉行首的提示符。提示符打在读输入之前、命令输出
// 紧接在它后面，于是每条命令的第一行输出与提示符挤在同一物理行上。
func ux2rLines(out string) []string {
	raw := strings.Split(out, "\n")
	lines := make([]string, 0, len(raw))
	for _, l := range raw {
		for strings.HasPrefix(l, prompt) {
			l = strings.TrimPrefix(l, prompt)
		}
		lines = append(lines, l)
	}
	return lines
}

// ux2rCountLine 数输出里与 want 逐字节相同的行有几条。整行相等天然带词边界：
// 「opus-5-0 已加入会话 X」不会与「opus-5-0-max 已加入会话 X」相等。
func ux2rCountLine(out, want string) int {
	n := 0
	for _, l := range ux2rLines(out) {
		if l == want {
			n++
		}
	}
	return n
}

// ux2rLineIndex 返回第一条与 want 相同的行的下标，没有时返回 -1。
func ux2rLineIndex(out, want string) int {
	for i, l := range ux2rLines(out) {
		if l == want {
			return i
		}
	}
	return -1
}

func ux2rRequireLine(t *testing.T, out, want string) {
	t.Helper()
	if ux2rCountLine(out, want) == 0 {
		t.Errorf("输出里缺少这一整行: %q；完整输出:\n%s", want, out)
	}
}

// ux2rWordRe 按词边界匹配一个 agent id：两侧都不能是 id 字符。
// 契约「夹具约定」：断言前缀对里短的那个名字时不许用 strings.Contains。
func ux2rWordRe(id string) *regexp.Regexp {
	return regexp.MustCompile(`(^|[^a-z0-9_.-])` + regexp.QuoteMeta(id) + `([^a-z0-9_.-]|$)`)
}

func ux2rJoined(id string) string { return fmt.Sprintf(ux2rFmtJoined, id, ux2rConvID) }

func ux2rSameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
