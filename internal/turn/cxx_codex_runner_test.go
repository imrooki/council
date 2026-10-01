package turn

// CX1 测试腿：codex 成员经真 Runner + 真 store 的验收（契约
// E:\multi_model_cowork\cx1_contract.md §2.2，T1-T6）。
//
// 不起任何进程：外部命令一律经 fakeSpawner；不写 TestMain、不依赖任何 TestMain。
// store 一律开在 t.TempDir() 之下（newRealTurnStore）。夹具是两份真机原件
// （codexprobe_exec1.jsonl / codexprobe_exec2.jsonl）逐字节抄成的常量，行尾 CRLF；
// T2 的两个失败形状是合成的，未实测。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"council/internal/config"
	"council/internal/provider"
)

// cxxExec1CRLF 逐字节等于 codexprobe_exec1.jsonl（347 字节）：新会话一轮，正文 PONG。
const cxxExec1CRLF = "{\"type\":\"thread.started\",\"thread_id\":\"01a0e373-ac6c-7083-87d7-62536800ada0\"}\r\n" +
	"{\"type\":\"turn.started\"}\r\n" +
	"{\"type\":\"item.completed\",\"item\":{\"id\":\"item_0\",\"type\":\"agent_message\",\"text\":\"PONG\"}}\r\n" +
	"{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":16184,\"cached_input_tokens\":0,\"cache_write_input_tokens\":0,\"output_tokens\":6,\"reasoning_output_tokens\":0}}\r\n"

// cxxExec2CRLF 逐字节等于 codexprobe_exec2.jsonl（352 字节）：同一 thread 续会话，正文 KIWI。
const cxxExec2CRLF = "{\"type\":\"thread.started\",\"thread_id\":\"01a0e373-ac6c-7083-87d7-62536800ada0\"}\r\n" +
	"{\"type\":\"turn.started\"}\r\n" +
	"{\"type\":\"item.completed\",\"item\":{\"id\":\"item_0\",\"type\":\"agent_message\",\"text\":\"KIWI\"}}\r\n" +
	"{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":32396,\"cached_input_tokens\":16000,\"cache_write_input_tokens\":0,\"output_tokens\":12,\"reasoning_output_tokens\":0}}\r\n"

const (
	cxxUsage1   = `{"input_tokens":16184,"cached_input_tokens":0,"cache_write_input_tokens":0,"output_tokens":6,"reasoning_output_tokens":0}`
	cxxUsage2   = `{"input_tokens":32396,"cached_input_tokens":16000,"cache_write_input_tokens":0,"output_tokens":12,"reasoning_output_tokens":0}`
	cxxThreadID = "01a0e373-ac6c-7083-87d7-62536800ada0"

	// cxxFP 是契约 §3.7 冻结的 sol 身份指纹（原型实测）。第七段是空会话 argv 摘要；
	// codex 的 argv 不含工作目录，所以这个值与机器、临时目录无关。
	cxxFP = "56:You are Sol (id=sol).\nProvider: codex\n只做反例检查;9:gpt-6-sol;3:low;4:read;0:;0:;15:19/c6979ae99b51;"
)

// cxxArgs 按契约 §2 通用纪律生成期望 argv（§3.1 / §3.2），model 固定 gpt-6-sol。
func cxxArgs(session, effort string) []string {
	a := []string{"exec"}
	if session != "" {
		a = append(a, "resume", session)
	}
	a = append(a, "--json", "--skip-git-repo-check", "-c", "sandbox_mode=read-only", "-c", "approval_policy=never", "-m", "gpt-6-sol")
	if effort != "" {
		a = append(a, "-c", "model_reasoning_effort="+effort)
	}
	a = append(a, "--ignore-rules", "--disable", "plugins", "--disable", "apps", "-c", "skills.include_instructions=false", "-")
	return a
}

// cxxCfg 是 newTestConfig 的一份，再加 codex provider 与成员 sol。alice（claude）、
// nova（agy）原样保留，作同一装置下只差 provider 的对照。
func cxxCfg() *config.Config {
	cfg := newTestConfig()
	cfg.Providers["codex"] = config.ProviderConfig{Command: "codex-fake"}
	cfg.Agents["sol"] = config.AgentConfig{
		DisplayName: "Sol",
		Provider:    "codex",
		Model:       "gpt-6-sol",
		Effort:      "low",
		Role:        "只做反例检查",
		Permissions: config.PermissionRead,
		Mode:        config.ModeDiscussion,
	}
	return cfg
}

// cxxUsageRow 是 turn_usage 一行里本文件要看的列。
type cxxUsageRow struct {
	session     string
	outcome     int
	input       sql.NullInt64
	cacheCreate sql.NullInt64
	cacheRead   sql.NullInt64
	output      sql.NullInt64
	thinking    sql.NullInt64
	cost        sql.NullFloat64
	promptBytes int
	identityFP  string
}

func cxxReadUsage(t *testing.T, db *sql.DB, turnID string) cxxUsageRow {
	t.Helper()
	var u cxxUsageRow
	err := db.QueryRow(`SELECT provider_session_id, outcome, input_tokens, cache_create, cache_read,
       output_tokens, thinking_tokens, cost_usd, prompt_bytes, identity_fp
FROM turn_usage WHERE turn_id = ?`, turnID).Scan(&u.session, &u.outcome, &u.input, &u.cacheCreate, &u.cacheRead,
		&u.output, &u.thinking, &u.cost, &u.promptBytes, &u.identityFP)
	if err != nil {
		t.Fatalf("查 turn_usage(turn_id=%s) 失败: %v", turnID, err)
	}
	return u
}

// TestCxxAskTwoTurnsRealStore 覆盖 T1：真 Runner + 真 store，sol 第一轮开会话、第二轮续会话，
// 中间夹一轮 claude 成员 alice 作对照。
//
// 【判别力】token 列全 NULL 的正对照是同一会话、同一 Runner、同一库里 alice 那一行
// （只差 provider，input_tokens = 47751）；stdin 不含身份的正对照是第 1 步（只差是不是
// 首轮）；effort_flag_omitted == false 与 effort_policy 钉住 Capabilities.Effort（X20）；
// argv / stdin 的整串比较杀 X06、X08-X18、X55-X57（X07 只在 Role 为空时可见，sol 有 Role，
// 它的杀手是 provider 包 P2 的 fresh_no_role，本条杀不到）；usage_raw 的原文杀 X41；
// 未报清单杀 X39、X40。
func TestCxxAskTwoTurnsRealStore(t *testing.T) {
	ctx := context.Background()
	rs := newRealTurnStore(t)
	r := newTestRunner(t, cxxCfg(), rs.svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn
	conv, err := rs.svc.Create(ctx, "cxx")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	convID := conv.ID
	joinAgent(t, rs.svc, convID, "sol")
	joinAgent(t, rs.svc, convID, testAgentClaude)
	postUser(t, rs.svc, convID, "hello-cxx")

	// 第 1 步：sol 首轮。
	fs.enqueueSuccess(cxxExec1CRLF)
	rec1, err := r.Ask(ctx, convID, "sol")
	if err != nil || rec1.Outcome != OutcomeSpoke {
		t.Fatalf("前提：sol 首轮应 spoke，Outcome=%v err=%v", rec1.Outcome, err)
	}
	c1 := fs.calls[0].cmd
	if want := cxxArgs("", "low"); !reflect.DeepEqual(c1.Args, want) {
		t.Errorf("首轮 Args = %q\n期望 %q", c1.Args, want)
	}
	if got, want := string(c1.Stdin), "You are Sol (id=sol).\nProvider: codex\n只做反例检查\ncxx\n\nuser: hello-cxx"; got != want {
		t.Errorf("首轮 Stdin = %q\n期望 %q", got, want)
	}
	if c1.Path != "codex-fake" {
		t.Errorf("首轮 Path = %q，期望 %q", c1.Path, "codex-fake")
	}
	if want := filepath.Join(r.workDir, "sol"); c1.Dir != want {
		t.Errorf("首轮 Dir = %q，期望 %q", c1.Dir, want)
	}
	if c1.Transport != provider.PromptTransportStdin {
		t.Errorf("首轮 Transport = %v，期望 Stdin", c1.Transport)
	}
	if c1.Files != nil || c1.Env != nil {
		t.Errorf("首轮 Files / Env 应为 nil，实际 Files=%v Env=%v", c1.Files, c1.Env)
	}
	if !rec1.Fresh || rec1.Drifted {
		t.Errorf("首轮 Fresh=%v Drifted=%v，期望 true / false", rec1.Fresh, rec1.Drifted)
	}
	if rec1.ProviderSession != cxxThreadID {
		t.Errorf("首轮 ProviderSession = %q，期望 %q", rec1.ProviderSession, cxxThreadID)
	}
	if rec1.Message == nil || rec1.Message.Text != "PONG" || rec1.Message.Thought != "" {
		t.Errorf("首轮 Message = %+v，期望 Text=PONG、Thought 为空", rec1.Message)
	}
	if want := len("cxx\n\nuser: hello-cxx"); rec1.PromptBytes != want {
		t.Errorf("首轮 PromptBytes = %d，期望 %d（只量 delta，不含身份块）", rec1.PromptBytes, want)
	}

	// 第 2 步：对照成员 alice（claude）。
	fs.enqueueSuccess(claudeT1JSON)
	recA, err := r.Ask(ctx, convID, testAgentClaude)
	if err != nil || recA.Outcome != OutcomeSpoke {
		t.Fatalf("前提：alice 应 spoke，Outcome=%v err=%v", recA.Outcome, err)
	}

	// 第 3 步：sol 续会话。
	postUser(t, rs.svc, convID, "second-cxx")
	fs.enqueueSuccess(cxxExec2CRLF)
	rec2, err := r.Ask(ctx, convID, "sol")
	if err != nil || rec2.Outcome != OutcomeSpoke {
		t.Fatalf("前提：sol 第二轮应 spoke，Outcome=%v err=%v", rec2.Outcome, err)
	}
	c2 := fs.calls[2].cmd
	if want := cxxArgs(cxxThreadID, "low"); !reflect.DeepEqual(c2.Args, want) {
		t.Errorf("续会话 Args = %q\n期望 %q", c2.Args, want)
	}
	if got, want := string(c2.Stdin), "alice: OK\n\nuser: second-cxx"; got != want {
		t.Errorf("续会话 Stdin = %q\n期望 %q", got, want)
	}
	if rec2.Fresh || rec2.Drifted {
		t.Errorf("续会话 Fresh=%v Drifted=%v，期望 false / false", rec2.Fresh, rec2.Drifted)
	}
	if rec2.ProviderSession != cxxThreadID {
		t.Errorf("续会话 ProviderSession = %q，期望 %q", rec2.ProviderSession, cxxThreadID)
	}
	if rec2.Message == nil || rec2.Message.Text != "KIWI" {
		t.Errorf("续会话 Message = %+v，期望 Text=KIWI", rec2.Message)
	}

	// 第 4 步：库。
	db := rs.rawDB(t)
	var psID, psFP string
	var spoken int
	var cursor int64
	if err := db.QueryRow(`SELECT provider_session_id, identity_fp, spoken, cursor_seq FROM provider_sessions WHERE conv_id = ? AND agent_id = 'sol'`, convID).
		Scan(&psID, &psFP, &spoken, &cursor); err != nil {
		t.Fatalf("查 provider_sessions 失败: %v", err)
	}
	if psID != cxxThreadID || psFP != cxxFP || spoken != 1 || cursor != rec2.ScannedTo {
		t.Errorf("provider_sessions = (%q, %q, %d, %d)\n期望 (%q, %q, 1, %d)", psID, psFP, spoken, cursor, cxxThreadID, cxxFP, rec2.ScannedTo)
	}
	for i, rec := range []Record{rec1, rec2} {
		u := cxxReadUsage(t, db, rec.TurnID)
		if u.session != cxxThreadID || u.outcome != 1 || u.identityFP != cxxFP || u.promptBytes != rec.PromptBytes {
			t.Errorf("sol 第 %d 轮 turn_usage = session %q outcome %d fp %q prompt_bytes %d\n期望 %q 1 %q %d",
				i+1, u.session, u.outcome, u.identityFP, u.promptBytes, cxxThreadID, cxxFP, rec.PromptBytes)
		}
		if u.input.Valid || u.cacheCreate.Valid || u.cacheRead.Valid || u.output.Valid || u.thinking.Valid || u.cost.Valid {
			t.Errorf("sol 第 %d 轮 token / 金额列应全 NULL，实际 %+v", i+1, u)
		}
	}
	ua := cxxReadUsage(t, db, recA.TurnID)
	if !ua.input.Valid || ua.input.Int64 != 47751 {
		t.Errorf("对照 alice 的 input_tokens = %+v，期望非 NULL 且等于 47751", ua.input)
	}

	rows, err := db.Query(`SELECT meta FROM messages WHERE sender_id = 'sol' ORDER BY seq`)
	if err != nil {
		t.Fatalf("查 sol 的消息失败: %v", err)
	}
	var metas []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			rows.Close()
			t.Fatalf("扫 meta 失败: %v", err)
		}
		metas = append(metas, m)
	}
	rows.Close()
	if len(metas) != 2 {
		t.Fatalf("sol 的消息应恰好 2 条，实际 %d", len(metas))
	}
	for i, wantUsage := range []string{cxxUsage1, cxxUsage2} {
		var meta map[string]json.RawMessage
		if err := json.Unmarshal([]byte(metas[i]), &meta); err != nil {
			t.Fatalf("第 %d 条 meta 不是 JSON 对象: %v（%s）", i+1, err, metas[i])
		}
		want := map[string]string{
			"usage_raw":           wantUsage,
			"usage_unreported":    `["input_tokens","cache_create","cache_read","output_tokens","thinking_tokens"]`,
			"effort_policy":       `"per_turn_flag"`,
			"effort_flag_omitted": `false`,
			"stop_raw":            `"turn.completed"`,
			"model_source":        `"requested"`,
		}
		for k, v := range want {
			if got := string(meta[k]); got != v {
				t.Errorf("第 %d 条 meta[%s] = %s，期望 %s", i+1, k, got, v)
			}
		}
	}

	// 第 5 步：冷暖未知。
	if _, known := rec2.MeasuredCold(); known {
		t.Errorf("rec2.MeasuredCold() known = true，期望 false（cache_read 标了未报）")
	}
}

// TestCxxAskFailureOutcomes 覆盖 T2：两个失败形状（合成，未实测）与一个正对照，退出码都是 1。
//
// 【判别力】turn_failed 与 no_completed 只差最后有没有 turn.failed：前者吸收了 thread id，
// 后者没有；把 turn.failed 归成认不出（X38）或把「没有 turn.completed」归成失败（X37）
// 都会让 provider_session_id 一格对调；失败分支丢掉会话 id（X49）红前者。exit1_success
// 与另两个只差 stdout，退出码同为 1：失败的两条不是因为退出码。
func TestCxxAskFailureOutcomes(t *testing.T) {
	cases := []struct {
		name    string
		stdout  string
		outcome Outcome
		sent    error
		session string
	}{
		{"turn_failed", "{\"type\":\"thread.started\",\"thread_id\":\"cxx-failed-thread\"}\n{\"type\":\"turn.started\"}\n{\"type\":\"turn.failed\",\"error\":{\"message\":\"cxx boom\"}}\n",
			OutcomeProviderFailed, provider.ErrTurnFailed, "cxx-failed-thread"},
		{"no_completed", "{\"type\":\"thread.started\",\"thread_id\":\"cxx-cut-thread\"}\n{\"type\":\"turn.started\"}\n",
			OutcomeUnparsable, provider.ErrUnparseableOutput, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			rs := newRealTurnStore(t)
			r := newTestRunner(t, cxxCfg(), rs.svc)
			fs := &fakeSpawner{}
			r.spawn = fs.spawn
			convID := newConv(t, rs.svc)
			joinAgent(t, rs.svc, convID, "sol")
			postUser(t, rs.svc, convID, "hello-cxx")

			fs.enqueue(spawnResult{stdout: []byte(c.stdout), exitCode: 1})
			rec, err := r.Ask(ctx, convID, "sol")
			if rec.Outcome != c.outcome {
				t.Errorf("Outcome = %v，期望 %v", rec.Outcome, c.outcome)
			}
			if !errors.Is(err, c.sent) {
				t.Errorf("err = %v，期望包着 %v", err, c.sent)
			}
			if _, found, perr := rs.svc.ProviderSession(ctx, convID, "sol"); perr != nil || found {
				t.Errorf("失败轮之后 ProviderSession found=%v err=%v，期望 found=false", found, perr)
			}
			u := cxxReadUsage(t, rs.rawDB(t), rec.TurnID)
			if u.outcome != int(c.outcome) || u.session != c.session {
				t.Errorf("turn_usage outcome=%d session=%q，期望 %d %q", u.outcome, u.session, int(c.outcome), c.session)
			}
			if u.input.Valid || u.cacheRead.Valid || u.output.Valid {
				t.Errorf("turn_usage 的 input / cache_read / output 应为 NULL，实际 %+v", u)
			}

			// 失败轮没有留下会话，下一轮仍是新会话。
			fs.enqueueSuccess(cxxExec1CRLF)
			rec2, err := r.Ask(ctx, convID, "sol")
			if err != nil || rec2.Outcome != OutcomeSpoke || !rec2.Fresh {
				t.Errorf("失败之后的下一轮：err=%v Outcome=%v Fresh=%v，期望 nil / spoke / true", err, rec2.Outcome, rec2.Fresh)
			}
			if len(fs.calls) < 2 {
				t.Fatalf("前提：应有 2 次 spawn，实际 %d", len(fs.calls))
			}
			if want := cxxArgs("", "low"); !reflect.DeepEqual(fs.calls[1].cmd.Args, want) {
				t.Errorf("下一轮 Args = %q\n期望 %q", fs.calls[1].cmd.Args, want)
			}
		})
	}
	t.Run("exit1_success", func(t *testing.T) {
		ctx := context.Background()
		rs := newRealTurnStore(t)
		r := newTestRunner(t, cxxCfg(), rs.svc)
		fs := &fakeSpawner{}
		r.spawn = fs.spawn
		convID := newConv(t, rs.svc)
		joinAgent(t, rs.svc, convID, "sol")
		postUser(t, rs.svc, convID, "hello-cxx")

		fs.enqueue(spawnResult{stdout: []byte(cxxExec1CRLF), exitCode: 1})
		rec, err := r.Ask(ctx, convID, "sol")
		if err != nil || rec.Outcome != OutcomeSpoke {
			t.Fatalf("退出码 1 + 成功 stdout：err=%v Outcome=%v，期望 nil / spoke", err, rec.Outcome)
		}
		if rec.Message == nil || rec.Message.Text != "PONG" {
			t.Errorf("Message = %+v，期望 Text=PONG", rec.Message)
		}
		ps, found, perr := rs.svc.ProviderSession(ctx, convID, "sol")
		if perr != nil || !found || ps.ProviderSessionID != cxxThreadID {
			t.Errorf("ProviderSession = %+v found=%v err=%v，期望 found 且 id=%q", ps, found, perr, cxxThreadID)
		}
	})
}

// TestCxxBudgetHeadroomIgnoresPromptLength 覆盖 T3：codex 的命令行余量与 prompt 长度无关。
//
// 【判别力】正文若进了 argv（X09），sol 两次不相等、也不等于 32542；nova（agy，正文在
// argv）是同一装置下只差 provider 的正对照，证明两段 prompt 的长度差确实传到了命令行
// 余量。32542 依赖 exec.LookPath("codex-fake") 找不到（budget.go 用原值）。
func TestCxxBudgetHeadroomIgnoresPromptLength(t *testing.T) {
	ctx := context.Background()
	rs := newRealTurnStore(t)
	r := newTestRunner(t, cxxCfg(), rs.svc)
	var sol, nova []int
	for _, body := range []string{"short", strings.Repeat("长", 3000)} {
		convID := newConv(t, rs.svc)
		joinAgent(t, rs.svc, convID, "sol")
		joinAgent(t, rs.svc, convID, testAgentAgy)
		postUser(t, rs.svc, convID, body)
		rows, err := r.Budget(ctx, convID, Plan{Speakers: []string{"sol", testAgentAgy}})
		if err != nil {
			t.Fatalf("Budget 报错: %v", err)
		}
		if len(rows) != 2 || rows[0].ArgvHeadroom == nil || rows[1].ArgvHeadroom == nil {
			t.Fatalf("前提：两行都应有 ArgvHeadroom，实际 %+v", rows)
		}
		sol = append(sol, *rows[0].ArgvHeadroom)
		nova = append(nova, *rows[1].ArgvHeadroom)
	}
	want := maxCommandLineBytes - CommandLineBytes("codex-fake", cxxArgs("", "low"))
	if want != 32542 {
		t.Errorf("maxCommandLineBytes - CommandLineBytes(codex-fake, 19 个元素) = %d，期望 32542", want)
	}
	if sol[0] != want || sol[1] != want {
		t.Errorf("sol 两次 ArgvHeadroom = %d / %d，期望都等于 %d", sol[0], sol[1], want)
	}
	if d := nova[0] - nova[1]; d != 3000*3-len("short") {
		t.Errorf("nova 两次 ArgvHeadroom 之差 = %d，期望 %d", d, 3000*3-len("short"))
	}
}

// TestCxxPrefixArgsGolden 覆盖 T4：codex 规范化之后的 argv 与摘要。
//
// 【判别力】钉 argv 本身（11b 契约 4.2 的体例）；argv 任何一项变（X09-X18、X55、X57）
// 摘要都变；最后一条是指纹算不出时 Ask 在花钱之前拒绝的根据（T6 在 Runner 上验），
// 去掉强度校验（X01）或换哨兵（X05）时它红。
func TestCxxPrefixArgsGolden(t *testing.T) {
	p, err := provider.Lookup("codex")
	if err != nil {
		t.Fatalf("provider.Lookup(codex) 失败: %v", err)
	}
	req := prefixDigestGoldenReq()
	req.Effort = "high"
	want := []string{
		"exec",
		"--json",
		"--skip-git-repo-check",
		"-c", "sandbox_mode=read-only",
		"-c", "approval_policy=never",
		"-m", "model-golden",
		"-c", "model_reasoning_effort=high",
		"--ignore-rules",
		"--disable", "plugins",
		"--disable", "apps",
		"-c", "skills.include_instructions=false",
		"-",
	}
	if len(want) != 19 {
		t.Fatalf("期望表应有 19 个元素，实际 %d", len(want))
	}
	got, err := normalizedPrefixArgs(p, req)
	if err != nil {
		t.Fatalf("normalizedPrefixArgs 失败: %v", err)
	}
	assertArgsEqual(t, "codex", got, want)

	digest, err := PrefixArgsDigest(p, req)
	if err != nil {
		t.Fatalf("PrefixArgsDigest 失败: %v", err)
	}
	if g := goldenDigestFor(want); digest != g || digest != "19/282ff78dfd33" {
		t.Errorf("PrefixArgsDigest = %q，期望等于由 golden argv 派生的 %q，且等于 %q", digest, g, "19/282ff78dfd33")
	}

	if _, err := PrefixArgsDigest(p, prefixDigestGoldenReq()); !errors.Is(err, provider.ErrUnsupportedEffort) {
		t.Errorf("Effort=effort-golden 时 PrefixArgsDigest err = %v，期望包着 ErrUnsupportedEffort", err)
	}
}

// TestCxxEffortChangeIsDrift 覆盖 T5：同一 Runner 里改 sol 的强度，下一轮以漂移拒绝、不起进程。
//
// 【判别力】EffortPerTurnFlag 才会让 effort 一段进指纹；声明成
// EffortProcessExternalConfig（X20）时文本只剩「命令行」一段。对照是第一轮（同一配置，成功）。
func TestCxxEffortChangeIsDrift(t *testing.T) {
	ctx := context.Background()
	rs := newRealTurnStore(t)
	r := newTestRunner(t, cxxCfg(), rs.svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn
	convID := newConv(t, rs.svc)
	joinAgent(t, rs.svc, convID, "sol")
	postUser(t, rs.svc, convID, "hello-cxx")
	fs.enqueueSuccess(cxxExec1CRLF)
	if rec, err := r.Ask(ctx, convID, "sol"); err != nil || rec.Outcome != OutcomeSpoke {
		t.Fatalf("前提：首轮应 spoke，Outcome=%v err=%v", rec.Outcome, err)
	}

	cfg2 := cxxCfg()
	ac := cfg2.Agents["sol"]
	ac.Effort = "high"
	cfg2.Agents["sol"] = ac
	r.SetConfig(cfg2)
	postUser(t, rs.svc, convID, "second-cxx")
	rec, err := r.Ask(ctx, convID, "sol")
	if rec.Outcome != OutcomeRefused {
		t.Errorf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if !errors.Is(err, ErrIdentityDrift) {
		t.Fatalf("err = %v，期望包着 ErrIdentityDrift", err)
	}
	if len(fs.calls) != 1 {
		t.Errorf("spawn 次数 = %d，期望 1（漂移在起进程之前拒绝）", len(fs.calls))
	}
	const want = `effort "low" -> "high"; 命令行 "19/c6979ae99b51" -> "19/121092233b57": identity or brain changed since this session was opened`
	if err.Error() != want {
		t.Errorf("err.Error() = %q\n期望 %q", err.Error(), want)
	}
}

// TestCxxInvalidEffortRefusedBeforeSpawn 覆盖 T6：配置里写了 codex 不认的强度，Ask 在花钱
// 之前拒绝。
//
// 【判别力】队列里放了一份会成功的 stdout：不拒的话这一轮会真的 spoke。对照是 T1 第 1 步
// （只差 effort 合法：起进程、有 session_opening）。去掉强度校验（X01）时起了进程；
// 哨兵换掉（X05）或新哨兵文本改掉（X52）时 errors.Is 或整串红；合法值表改了（X02、X03）
// 时报错文本里的清单变。
func TestCxxInvalidEffortRefusedBeforeSpawn(t *testing.T) {
	ctx := context.Background()
	rs := newRealTurnStore(t)
	cfg := cxxCfg()
	ac := cfg.Agents["sol"]
	ac.Effort = "turbo"
	cfg.Agents["sol"] = ac
	r := newTestRunner(t, cfg, rs.svc)
	fs := &fakeSpawner{}
	r.spawn = fs.spawn
	convID := newConv(t, rs.svc)
	joinAgent(t, rs.svc, convID, "sol")
	postUser(t, rs.svc, convID, "hello-cxx")
	fs.enqueueSuccess(cxxExec1CRLF)

	rec, err := r.Ask(ctx, convID, "sol")
	if rec.Outcome != OutcomeRefused {
		t.Errorf("Outcome = %v，期望 OutcomeRefused", rec.Outcome)
	}
	if !errors.Is(err, provider.ErrUnsupportedEffort) {
		t.Fatalf("err = %v，期望包着 ErrUnsupportedEffort", err)
	}
	if len(fs.calls) != 0 {
		t.Errorf("spawn 次数 = %d，期望 0", len(fs.calls))
	}
	hist, herr := rs.svc.History(ctx, convID, 0, 100)
	if herr != nil || len(hist) != 1 {
		t.Errorf("History 条数 = %d（err=%v），期望恰好 1 条（没有 session_opening）", len(hist), herr)
	}
	const want = `prefix args digest for sol: effort "turbo" is not a codex reasoning level (want one of minimal, low, medium, high, xhigh, max, ultra): effort value not supported by this provider`
	if err.Error() != want {
		t.Errorf("err.Error() = %q\n期望 %q", err.Error(), want)
	}
	u := cxxReadUsage(t, rs.rawDB(t), rec.TurnID)
	if u.outcome != 7 || u.identityFP != "" {
		t.Errorf("turn_usage outcome=%d identity_fp=%q，期望 7 与空串", u.outcome, u.identityFP)
	}
}
