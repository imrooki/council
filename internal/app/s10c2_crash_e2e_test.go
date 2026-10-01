package app

// s10c2_crash_e2e_test.go 是 Step 10 契约 §6.2（C2，崩溃对账端到端）的探针用例。
//
// 形状：沿用 smoke_test.go 的 helper-process 套路（TestMain 在 COUNCIL_FAKE_CLI=1
// 时扮演 claude CLI，打印逐字节抄来的成功 JSON），store 放在 t.TempDir()，不花钱。
//
// 与 §6.2 原文的一处有意偏离：原文是「第二次 app.Run 里先对账、再 /ask」。本用例拆成
// 三次 Run：
//   - Run 0：正常一轮，建起库、会话与 provider_sessions；再追加一条用户消息 B，
//     作为「崩掉的那一轮送出去的 delta」。
//   - 塞一条 phase='spawned' 的在飞记录（形状照 runner.go 里 BeginTurn 的真实写法：
//     先 begun 再 SetInflightPhase(spawned)，两笔各自的写事务）。
//   - Run A：stdin 直接 EOF。只做启动对账，跑完在库静止时查横幅、inflight_turns、
//     baseline_suspect、turn_lost 面包屑。
//   - Run B：/use 会话、/ask c1、/log。查紧接着那一轮的 Record 在库里的投影。
// 拆开的理由：commitSession 在下一轮成功时把 baselineSuspect 清零并经 RecordTurn
// 写回（runner.go snapshotProviderSession 带着这一列），同一次 Run 里「跑完
// baseline_suspect == 1」与「紧接着的 /ask」两个观察点互相覆盖。拆开之后 Run B
// 的 sessionState 只能从库里读回可疑标记（sessionStateFrom），正好验的是第 2 节
// 那座「跨重启」的桥，比单次 Run 更严。
//
// Record 在 app.Run 里拿不到，用它在库里的投影代替，每一条都现查过来源：
//   - Record.CostUSDDelta -> turn_usage.cost_usd（makeTurnUsage: CostUSD = rec.CostUSDDelta）
//   - Record.BaselineSuspect -> agent 消息 meta.cost_baseline_suspect（agentMeta）
//   - Record.Delivered -> agent 消息 meta.delivered_seqs（agentMeta: MetaDelivered）
//     与 turn_usage.delivered_lo / delivered_hi
//
// A 之五契约 §6 第 7 步 (i)：Run B 在 /ask 前后各敲一次 /session，c1 那一行的 BASELINE
// 格先 suspect（从库里读回的可疑标记）、后 ok（commitSession 清零）；对照用例同一位置
// 两次都是 ok。第 7 步 (ii)「对账失败留行时 /ask 被拒且不起进程」由
// s10a_settle_fail_blocks_ask_test.go 的 E1 覆盖，这里不重复。

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/store"
	"council/internal/turn"

	_ "modernc.org/sqlite"
)

const (
	s10cTurnID      = "5c10c2aa-0000-4000-8000-00000000c2c2"
	s10cMsgA        = "s10c 第一条用户消息"
	s10cMsgB        = "s10c 第二条用户消息，崩掉的那一轮送出去的就是它"
	s10cPlannedGrok = "5c10c2bb-1111-4111-8111-00000000beef"
)

// s10cConfig 写一份配置：c1 是 claude（命令是测试二进制自己），g1 是 grok
// （只在 bootstrap 那条用例里被塞在飞记录，从来不会被 /ask，不起进程）。
func s10cConfig(t *testing.T) (cfgPath, dataDir string) {
	t.Helper()
	dir := t.TempDir()
	dataDir = filepath.Join(dir, "data")
	body := fmt.Sprintf(`data_dir = %s

[providers.claude]
command = %s

[providers.grok]
command = %s

[agents.c1]
display_name = "C1"
provider = "claude"
model = "opus"
effort = "high"
role = "C2 探针用的假成员"
permissions = "read"
mode = "discussion"

[agents.g1]
display_name = "G1"
provider = "grok"
model = "grok-4.7"
effort = "high"
role = "C2 探针用的假成员，只用来塞 bootstrap 形状的在飞记录"
permissions = "read"
mode = "discussion"
`, tomlLiteral(dataDir), tomlLiteral(os.Args[0]), tomlLiteral(os.Args[0]))
	return writeTestConfig(t, dir, body), dataDir
}

func s10cOpenDB(t *testing.T, dataDir string) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(dataDir, "council.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatalf("打开 %s 失败：%v", dbPath, err)
	}
	return db
}

// s10cQuery 每次现开现关，避免与 app.Run 持有的写锁长时间并存（Windows 上尤其）。
func s10cQueryInt(t *testing.T, dataDir, q string, args ...any) int64 {
	t.Helper()
	db := s10cOpenDB(t, dataDir)
	defer db.Close()
	var n int64
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("查询失败 %q：%v", q, err)
	}
	return n
}

type s10cMsg struct {
	seq      int64
	kind     string
	senderID string
	text     string
	meta     map[string]any
}

func s10cMessages(t *testing.T, dataDir string) []s10cMsg {
	t.Helper()
	db := s10cOpenDB(t, dataDir)
	defer db.Close()
	rows, err := db.Query(`SELECT seq, kind, sender_id, text, meta FROM messages ORDER BY seq`)
	if err != nil {
		t.Fatalf("查询 messages 失败：%v", err)
	}
	defer rows.Close()
	var out []s10cMsg
	for rows.Next() {
		var m s10cMsg
		var metaRaw string
		if err := rows.Scan(&m.seq, &m.kind, &m.senderID, &m.text, &metaRaw); err != nil {
			t.Fatalf("扫 messages 失败：%v", err)
		}
		m.meta = map[string]any{}
		if err := json.Unmarshal([]byte(metaRaw), &m.meta); err != nil {
			t.Fatalf("seq %d 的 meta 不是合法 JSON（%q）：%v", m.seq, metaRaw, err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历 messages 失败：%v", err)
	}
	return out
}

type s10cUsage struct {
	turnID     string
	seq        sql.NullInt64
	outcome    int
	sessionID  string
	costUSD    sql.NullFloat64
	deliveredL sql.NullInt64
	deliveredH sql.NullInt64
}

func s10cUsageRows(t *testing.T, dataDir string) []s10cUsage {
	t.Helper()
	db := s10cOpenDB(t, dataDir)
	defer db.Close()
	rows, err := db.Query(`SELECT turn_id, seq, outcome, provider_session_id, cost_usd, delivered_lo, delivered_hi FROM turn_usage ORDER BY created_at, turn_id`)
	if err != nil {
		t.Fatalf("查询 turn_usage 失败：%v", err)
	}
	defer rows.Close()
	var out []s10cUsage
	for rows.Next() {
		var u s10cUsage
		if err := rows.Scan(&u.turnID, &u.seq, &u.outcome, &u.sessionID, &u.costUSD, &u.deliveredL, &u.deliveredH); err != nil {
			t.Fatalf("扫 turn_usage 失败：%v", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历 turn_usage 失败：%v", err)
	}
	return out
}

type s10cSession struct {
	sessionID string
	cursorSeq int64
	suspect   int64
	costTotal sql.NullFloat64
	identity  string
}

func s10cProviderSession(t *testing.T, dataDir, agentID string) (s10cSession, bool) {
	t.Helper()
	db := s10cOpenDB(t, dataDir)
	defer db.Close()
	var s s10cSession
	err := db.QueryRow(`SELECT provider_session_id, cursor_seq, baseline_suspect, cost_total_usd, identity_fp FROM provider_sessions WHERE agent_id = ?`, agentID).
		Scan(&s.sessionID, &s.cursorSeq, &s.suspect, &s.costTotal, &s.identity)
	if err == sql.ErrNoRows {
		return s, false
	}
	if err != nil {
		t.Fatalf("查询 provider_sessions 失败：%v", err)
	}
	return s, true
}

// s10cLastTurnAtCell 读 c1 这类成员 provider_sessions.last_turn_at，按 /session 的
// 渲染（repl formatTime：UTC、RFC3339、截到秒）格式化成那一格应有的文本。
func s10cLastTurnAtCell(t *testing.T, dataDir, agentID string) string {
	t.Helper()
	db := s10cOpenDB(t, dataDir)
	defer db.Close()
	var raw string
	if err := db.QueryRow(`SELECT last_turn_at FROM provider_sessions WHERE agent_id = ?`, agentID).Scan(&raw); err != nil {
		t.Fatalf("查询 provider_sessions.last_turn_at 失败：%v", err)
	}
	ts, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		t.Fatalf("last_turn_at %q 解析失败：%v", raw, err)
	}
	return ts.UTC().Format(time.RFC3339)
}

// s10cSessionHeader 是 /session 表头按空白切开之后的五格。
var s10cSessionHeader = []string{"AGENT", "PROVIDER_SESSION", "LAST_TURN_AT", "BASELINE", "NEXT"}

// s10cSessionRows 在 out 里按出现顺序找每一张 /session 表，返回每张表里 agentID 那一行
// 按空白切开的格（tabwriter 把制表符换成了空格对齐，所以按 strings.Fields 切）。
// 提示符 "council> " 不换行，表头与它同在一行，切格之前先剥掉这个前缀。
// 前提守卫：表头恰好 want 张、每张表里都找得到这一行，否则 Fatalf。
func s10cSessionRows(t *testing.T, out, agentID string, want int) [][]string {
	t.Helper()
	lines := strings.Split(out, "\n")
	cells := func(l string) []string { return strings.Fields(strings.TrimPrefix(l, "council> ")) }
	var rows [][]string
	for i, l := range lines {
		if !reflect.DeepEqual(cells(l), s10cSessionHeader) {
			continue
		}
		var row []string
		for _, l2 := range lines[i+1:] {
			f := cells(l2)
			if reflect.DeepEqual(f, s10cSessionHeader) {
				break
			}
			if len(f) > 0 && f[0] == agentID {
				row = f
				break
			}
		}
		if row == nil {
			t.Fatalf("前提不成立：第 %d 张 /session 表里没有 %s 那一行；Out:\n%s", len(rows)+1, agentID, out)
		}
		rows = append(rows, row)
	}
	if len(rows) != want {
		t.Fatalf("前提不成立：输出里 /session 表头应有 %d 张，实际 %d；Out:\n%s", want, len(rows), out)
	}
	return rows
}

func s10cConvID(t *testing.T, dataDir string) string {
	t.Helper()
	db := s10cOpenDB(t, dataDir)
	defer db.Close()
	var id string
	if err := db.QueryRow(`SELECT id FROM conversations`).Scan(&id); err != nil {
		t.Fatalf("查询 conversations 失败（应恰好一条）：%v", err)
	}
	return id
}

func s10cRun(t *testing.T, cfgPath string, lines ...string) (string, string) {
	t.Helper()
	script := ""
	if len(lines) > 0 {
		script = strings.Join(lines, "\n") + "\n"
	}
	out, errOut, err := runAppForTest(t, context.Background(), cfgPath, strings.NewReader(script))
	if err != nil {
		t.Fatalf("app.Run 应当返回 nil，实际 %v；\nOut:\n%s\nErr:\n%s", err, out, errOut)
	}
	return out, errOut
}

// s10cSeedInflight 照 runner.go 的真实写法塞在飞记录：BeginTurn(begun) 一笔、
// SetInflightPhase(spawned) 再一笔。不直接 BeginTurn(Phase: spawned)——真代码
// 写不出那种形状（契约 3.3a 最后一段：仍然写两次）。
func s10cSeedInflight(t *testing.T, dataDir string, it conversation.InflightTurn) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("打开 store 失败：%v", err)
	}
	defer func() {
		if cerr := st.Close(); cerr != nil {
			t.Errorf("关闭 store 失败：%v", cerr)
		}
	}()
	svc := conversation.NewService(st)
	it.Phase = conversation.PhaseBegun
	if err := svc.BeginTurn(ctx, it); err != nil {
		t.Fatalf("BeginTurn 种子失败：%v", err)
	}
	if err := svc.SetInflightPhase(ctx, it.TurnID, conversation.PhaseSpawned); err != nil {
		t.Fatalf("SetInflightPhase(spawned) 种子失败：%v", err)
	}
}

func s10cMetaBool(m map[string]any, key string) (bool, bool) {
	v, ok := m[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func s10cMetaSeqs(m map[string]any, key string) []int64 {
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]int64, 0, len(raw))
	for _, v := range raw {
		if f, ok := v.(float64); ok {
			out = append(out, int64(f))
		}
	}
	return out
}

func s10cContainsSeq(seqs []int64, want int64) bool {
	for _, s := range seqs {
		if s == want {
			return true
		}
	}
	return false
}

// s10cPhase0 跑 Run 0：一轮正常 /ask，然后追加用户消息 B。返回 convID 与 B 的 seq。
func s10cPhase0(t *testing.T, cfgPath, dataDir string) (convID string, seqB int64, out0 string) {
	t.Helper()
	out0, _ = s10cRun(t, cfgPath, "/new s10c", "/join c1", s10cMsgA, "/ask c1", s10cMsgB, "/quit")
	convID = s10cConvID(t, dataDir)
	for _, m := range s10cMessages(t, dataDir) {
		if m.kind == "user" && m.text == s10cMsgB {
			seqB = m.seq
		}
	}
	if seqB == 0 {
		t.Fatalf("前置：Run 0 之后找不到用户消息 B；Out:\n%s", out0)
	}
	return convID, seqB, out0
}

// TestS10cC2CrashReconcileEndToEnd 是 §6.2 的主用例：resume 轮崩在 spawn 与
// RecordTurn 之间（claude，provider_sessions 那一行存在）。
func TestS10cC2CrashReconcileEndToEnd(t *testing.T) {
	isolateAppEnv(t)
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	cfgPath, dataDir := s10cConfig(t)

	convID, seqB, out0 := s10cPhase0(t, cfgPath, dataDir)
	sess0, ok := s10cProviderSession(t, dataDir, "c1")
	if !ok {
		t.Fatalf("前置：Run 0 之后 provider_sessions 里应有 c1 的一行；Out:\n%s", out0)
	}
	if sess0.suspect != 0 {
		t.Fatalf("前置：崩溃之前 baseline_suspect 应为 0，实际 %d", sess0.suspect)
	}
	if sess0.cursorSeq >= seqB {
		t.Fatalf("前置：Run 0 的游标 %d 应小于 B 的 seq %d（B 是在 /ask 之后才追加的）", sess0.cursorSeq, seqB)
	}
	usage0 := s10cUsageRows(t, dataDir)
	if len(usage0) != 1 || !usage0[0].costUSD.Valid {
		t.Fatalf("前置/对照：Run 0 那一轮 turn_usage 应恰好一行且 cost_usd 非 NULL，实际 %+v", usage0)
	}
	var agent0 s10cMsg
	for _, m := range s10cMessages(t, dataDir) {
		if m.kind == "agent" {
			agent0 = m
		}
	}
	if b, ok := s10cMetaBool(agent0.meta, turn.MetaCostBaselineSuspect); !ok || b {
		t.Fatalf("前置/对照：Run 0 的 agent 消息 meta.%s 应存在且为 false，实际 %v（存在=%v）", turn.MetaCostBaselineSuspect, b, ok)
	}
	if strings.Contains(out0, "没记上账") {
		t.Fatalf("对照：干净库的 Run 0 横幅不该出现「没记上账」；Out:\n%s", out0)
	}

	// 真实崩溃轮的形状（runner.go BeginTurn 那一段）：claude 不预铸，所以
	// PlannedSessionID 是空串；ResumeSessionID 是本轮送出去 resume 的会话；
	// CursorSeq 是进轮时的游标；ScannedTo 是这一轮扫到的最大 seq（B）；
	// read 档的 WorkDir 是 <data>/workdir/<agent>（agentWorkDir）。
	s10cSeedInflight(t, dataDir, conversation.InflightTurn{
		TurnID:           s10cTurnID,
		ConvID:           convID,
		AgentID:          "c1",
		Provider:         "claude",
		PlannedSessionID: "",
		ResumeSessionID:  sess0.sessionID,
		CursorSeq:        sess0.cursorSeq,
		ScannedTo:        seqB,
		StartedAt:        time.Now().UTC(),
		Tier:             "read",
		WorkDir:          filepath.Join(dataDir, "workdir", "c1"),
	})
	if n := s10cQueryInt(t, dataDir, `SELECT COUNT(*) FROM inflight_turns WHERE turn_id = ? AND phase = 'spawned'`, s10cTurnID); n != 1 {
		t.Fatalf("前置：种子在飞记录应恰好一行且 phase=spawned，实际 %d", n)
	}

	// ---- Run A：只做启动对账 ----
	outA, errA := s10cRun(t, cfgPath)

	t.Run("banner_names_conv_agent_turn", func(t *testing.T) {
		const first = "注意: 有 1 轮在上次退出时没记上账，其中 1 轮已把相关成员的成本基线标为可疑"
		if !strings.Contains(outA, first) {
			t.Errorf("横幅缺冻结首行 %q；Out:\n%s\nErr:\n%s", first, outA, errA)
		}
		found := false
		for _, line := range strings.Split(outA, "\n") {
			if strings.Contains(line, convID) && strings.Contains(line, "c1") && strings.Contains(line, s10cTurnID) {
				found = true
			}
		}
		if !found {
			t.Errorf("横幅里应有一行同时点名 conv %s / agent c1 / turn %s；Out:\n%s", convID, s10cTurnID, outA)
		}
	})

	t.Run("inflight_empty_after_settle", func(t *testing.T) {
		if n := s10cQueryInt(t, dataDir, `SELECT COUNT(*) FROM inflight_turns`); n != 0 {
			t.Errorf("对账之后 inflight_turns 应为空，实际 %d 行；Err:\n%s", n, errA)
		}
	})

	t.Run("baseline_suspect_is_1_and_cursor_untouched", func(t *testing.T) {
		s, ok := s10cProviderSession(t, dataDir, "c1")
		if !ok {
			t.Fatalf("对账之后 c1 的 provider_sessions 行不见了")
		}
		if s.suspect != 1 {
			t.Errorf("对账之后 baseline_suspect 应为 1，实际 %d", s.suspect)
		}
		if s.cursorSeq != sess0.cursorSeq {
			t.Errorf("对账不许推游标：cursor_seq 应保持 %d，实际 %d（ScannedTo=%d）", sess0.cursorSeq, s.cursorSeq, seqB)
		}
		if s.sessionID != sess0.sessionID || s.identity != sess0.identity || s.costTotal != sess0.costTotal {
			t.Errorf("整行回写只许改 baseline_suspect：前 %+v 后 %+v", sess0, s)
		}
	})

	var lostSeq int64
	t.Run("turn_lost_breadcrumb_with_orphan_id", func(t *testing.T) {
		var lost []s10cMsg
		for _, m := range s10cMessages(t, dataDir) {
			if ev, _ := m.meta[turn.MetaEvent].(string); ev == "turn_lost" {
				lost = append(lost, m)
			}
		}
		if len(lost) != 1 {
			t.Fatalf("messages 里应恰好一条 event=turn_lost 的面包屑，实际 %d 条", len(lost))
		}
		m := lost[0]
		lostSeq = m.seq
		if m.kind != "system" || m.senderID != conversation.ReservedSenderSystem {
			t.Errorf("面包屑应为 kind=system、sender=system，实际 kind=%q sender=%q", m.kind, m.senderID)
		}
		if about, _ := m.meta[turn.MetaAboutAgent].(string); about != "c1" {
			t.Errorf("面包屑 meta.%s 应为 c1，实际 %q", turn.MetaAboutAgent, about)
		}
		// §6.2 写「Meta 里带得住 planned_session_id」。今天的键是 orphan_session_id，
		// resume 轮取 ResumeSessionID（recover.go orphanSessionID）；claude 不预铸，
		// planned 本来就是空串。断言的是今天代码真正承诺的东西。
		if got, _ := m.meta[turn.MetaOrphanSession].(string); got != sess0.sessionID {
			t.Errorf("面包屑 meta.%s 应为 resume 的会话 id %q，实际 %q；meta=%v", turn.MetaOrphanSession, sess0.sessionID, got, m.meta)
		}
		if _, has := m.meta["planned_session_id"]; has {
			t.Logf("观察：面包屑 meta 里出现了 planned_session_id 键：%v", m.meta)
		}
		if m.seq <= seqB {
			t.Errorf("面包屑应追加在 B（seq %d）之后，实际 seq %d", seqB, m.seq)
		}
	})

	t.Run("lost_turn_usage_row", func(t *testing.T) {
		var got *s10cUsage
		for _, u := range s10cUsageRows(t, dataDir) {
			if u.turnID == s10cTurnID {
				u := u
				got = &u
			}
		}
		if got == nil {
			t.Fatalf("对账之后应补一条 turn_id=%s 的 turn_usage", s10cTurnID)
		}
		if got.outcome != int(turn.OutcomeLost) || got.costUSD.Valid || got.seq.Valid || got.deliveredL.Valid || got.deliveredH.Valid {
			t.Errorf("Lost 行应 outcome=%d、cost/seq/delivered 全 NULL，实际 %+v", int(turn.OutcomeLost), *got)
		}
	})

	// ---- Run B：紧接着的 /ask ----
	lastCellA := s10cLastTurnAtCell(t, dataDir, "c1")
	outB, errB := s10cRun(t, cfgPath, "/use "+convID, "/session", "/ask c1", "/session", "/log", "/quit")
	lastCellB := s10cLastTurnAtCell(t, dataDir, "c1")

	// 【判别力】/session 的 BASELINE 格恒印 ok 时 before 这一行红；SessionInfo 读库那一支
	// 丢了 BaselineSuspect 时同样红（第一次 /session 时本进程还没起过这一轮，闸空闲、
	// 内存为空，走的是读库分支，不经过 sessionStateFrom；sessionStateFrom 丢了这一列
	// 由下面 next_ask_* 两条子用例抓）。commitSession 不清零时 after 这一行红。对照是
	// TestS10cC2ControlNoCrash 同一位置的两行：没有崩溃时两次都是 ok，恒印 suspect
	// 在那边红。NEXT 格恒为 warm：CacheTTL 是一小时，两次 last_turn_at 都在几秒之内。
	t.Run("session_baseline_suspect_then_ok", func(t *testing.T) {
		rows := s10cSessionRows(t, outB, "c1", 2)
		wantBefore := []string{"c1", sess0.sessionID, lastCellA, "suspect", "warm"}
		wantAfter := []string{"c1", sess0.sessionID, lastCellB, "ok", "warm"}
		if !reflect.DeepEqual(rows[0], wantBefore) {
			t.Errorf("/ask 之前 /session 的 c1 那一行 = %q，期望 %q；Out:\n%s", rows[0], wantBefore, outB)
		}
		if !reflect.DeepEqual(rows[1], wantAfter) {
			t.Errorf("/ask 之后 /session 的 c1 那一行 = %q，期望 %q；Out:\n%s", rows[1], wantAfter, outB)
		}
	})

	var agentB s10cMsg
	for _, m := range s10cMessages(t, dataDir) {
		if m.kind == "agent" && m.seq > seqB {
			if agentB.seq != 0 {
				t.Fatalf("Run B 之后 seq > B 的 agent 消息应恰好一条，多出 seq %d 与 %d", agentB.seq, m.seq)
			}
			agentB = m
		}
	}
	if agentB.seq == 0 {
		t.Fatalf("Run B 的 /ask 没有产生 agent 消息（被拒？ErrNoDelta 说明游标被偷推了）；Out:\n%s\nErr:\n%s", outB, errB)
	}
	var usageB *s10cUsage
	for _, u := range s10cUsageRows(t, dataDir) {
		if u.seq.Valid && u.seq.Int64 == agentB.seq {
			u := u
			usageB = &u
		}
	}
	if usageB == nil {
		t.Fatalf("Run B 的 agent 消息（seq %d）没有对应的 turn_usage 行", agentB.seq)
	}

	t.Run("next_ask_cost_delta_nil", func(t *testing.T) {
		if usageB.costUSD.Valid {
			t.Errorf("崩溃之后第一轮 turn_usage.cost_usd（即 Record.CostUSDDelta）应为 NULL，实际 %v", usageB.costUSD.Float64)
		}
	})

	t.Run("next_ask_meta_cost_baseline_suspect", func(t *testing.T) {
		b, ok := s10cMetaBool(agentB.meta, turn.MetaCostBaselineSuspect)
		if !ok || !b {
			t.Errorf("崩溃之后第一轮 agent meta.%s 应为 true，实际 %v（存在=%v）；meta=%v", turn.MetaCostBaselineSuspect, b, ok, agentB.meta)
		}
	})

	t.Run("next_ask_delivered_contains_crashed_seq", func(t *testing.T) {
		seqs := s10cMetaSeqs(agentB.meta, turn.MetaDelivered)
		if !s10cContainsSeq(seqs, seqB) {
			t.Errorf("紧接着那一轮 Delivered 应含崩溃那轮送过的 seq %d，实际 %v", seqB, seqs)
		}
		if !usageB.deliveredL.Valid || !usageB.deliveredH.Valid || usageB.deliveredL.Int64 > seqB || usageB.deliveredH.Int64 < seqB {
			t.Errorf("turn_usage.delivered_lo/hi 应覆盖 %d，实际 lo=%v hi=%v", seqB, usageB.deliveredL, usageB.deliveredH)
		}
	})

	t.Run("next_ask_resumes_not_fresh", func(t *testing.T) {
		opening := 0
		for _, m := range s10cMessages(t, dataDir) {
			if ev, _ := m.meta[turn.MetaEvent].(string); ev == "session_opening" {
				opening++
			}
		}
		if opening != 1 {
			t.Errorf("Run B 应 resume 原会话，session_opening 面包屑应仍只有 1 条，实际 %d", opening)
		}
		if usageB.sessionID != sess0.sessionID {
			t.Errorf("Run B 那一轮 provider_session_id 应为原会话 %q，实际 %q", sess0.sessionID, usageB.sessionID)
		}
	})

	t.Run("log_shows_breadcrumb", func(t *testing.T) {
		if lostSeq == 0 {
			t.Skip("面包屑那条子用例没找到 seq")
		}
		// 证据链：cmdLog -> r.svc.History -> Store.Messages，SQL 是
		// WHERE conv_id = ? AND seq > ? ORDER BY seq ASC，无 kind 谓词。
		// 屏幕格式随便改，这里只认「以面包屑 seq 开头、含 system 的一行」。
		prefix := strconv.FormatInt(lostSeq, 10)
		found := false
		for _, line := range strings.Split(outB, "\n") {
			f := strings.Fields(line)
			if len(f) >= 3 && f[0] == prefix && f[1] == conversation.ReservedSenderSystem && f[2] == "system" {
				found = true
			}
		}
		if !found {
			t.Errorf("/log 输出里应有 seq %d 的 system 面包屑一行；Out:\n%s", lostSeq, outB)
		}
	})

	t.Run("suspect_cleared_after_successful_turn", func(t *testing.T) {
		s, _ := s10cProviderSession(t, dataDir, "c1")
		if s.suspect != 0 {
			t.Errorf("观察（commitSession 清零）：成功一轮之后 baseline_suspect 应回到 0，实际 %d", s.suspect)
		}
	})
}

// TestS10cC2ControlNoCrash 是对照组：同样的 Run 0 / Run A / Run B，只是不塞在飞记录。
// 证明主用例里「cost_usd 为 NULL」「cost_baseline_suspect 为 true」不是假 CLI
// 的常态：没有崩溃时，同一份累计金额算出来的差值是 0（非 NULL）、标记是 false。
func TestS10cC2ControlNoCrash(t *testing.T) {
	isolateAppEnv(t)
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	cfgPath, dataDir := s10cConfig(t)

	convID, seqB, _ := s10cPhase0(t, cfgPath, dataDir)
	outA, _ := s10cRun(t, cfgPath)
	for _, forbidden := range []string{"没记上账", "在飞"} {
		if strings.Contains(outA, forbidden) {
			t.Errorf("没有在飞记录时横幅不该出现 %q；Out:\n%s", forbidden, outA)
		}
	}
	if s, _ := s10cProviderSession(t, dataDir, "c1"); s.suspect != 0 {
		t.Errorf("对照：没有崩溃时 baseline_suspect 应为 0，实际 %d", s.suspect)
	}

	sess0, ok := s10cProviderSession(t, dataDir, "c1")
	if !ok {
		t.Fatalf("前置：对照 Run A 之后 provider_sessions 里应有 c1 的一行")
	}
	lastCellA := s10cLastTurnAtCell(t, dataDir, "c1")
	outB, errB := s10cRun(t, cfgPath, "/use "+convID, "/session", "/ask c1", "/session", "/quit")
	lastCellB := s10cLastTurnAtCell(t, dataDir, "c1")
	// 【判别力】主用例 session_baseline_suspect_then_ok 的对照：只差库里有没有那条在飞记录。
	// /session 的 BASELINE 格恒印 suspect 时这两行红；主用例的 suspect 因此不是恒印出来的。
	rows := s10cSessionRows(t, outB, "c1", 2)
	for i, want := range [][]string{
		{"c1", sess0.sessionID, lastCellA, "ok", "warm"},
		{"c1", sess0.sessionID, lastCellB, "ok", "warm"},
	} {
		if !reflect.DeepEqual(rows[i], want) {
			t.Errorf("对照：第 %d 次 /session 的 c1 那一行 = %q，期望 %q；Out:\n%s", i+1, rows[i], want, outB)
		}
	}
	var agentB s10cMsg
	for _, m := range s10cMessages(t, dataDir) {
		if m.kind == "agent" && m.seq > seqB {
			agentB = m
		}
		if ev, _ := m.meta[turn.MetaEvent].(string); ev == "turn_lost" {
			t.Errorf("对照：不该有 turn_lost 面包屑，实际 seq %d", m.seq)
		}
	}
	if agentB.seq == 0 {
		t.Fatalf("对照 Run B 没有产生 agent 消息；Out:\n%s\nErr:\n%s", outB, errB)
	}
	var usageB *s10cUsage
	for _, u := range s10cUsageRows(t, dataDir) {
		if u.seq.Valid && u.seq.Int64 == agentB.seq {
			u := u
			usageB = &u
		}
	}
	if usageB == nil || !usageB.costUSD.Valid {
		t.Errorf("对照：没有崩溃时第二轮 cost_usd 应非 NULL，实际 %+v", usageB)
	}
	if b, ok := s10cMetaBool(agentB.meta, turn.MetaCostBaselineSuspect); !ok || b {
		t.Errorf("对照：没有崩溃时 meta.%s 应存在且为 false，实际 %v（存在=%v）", turn.MetaCostBaselineSuspect, b, ok)
	}
	if !s10cContainsSeq(s10cMetaSeqs(agentB.meta, turn.MetaDelivered), seqB) {
		t.Errorf("对照：第二轮 Delivered 同样应含 %d，实际 %v", seqB, s10cMetaSeqs(agentB.meta, turn.MetaDelivered))
	}
}

// TestS10cC2GrokBootstrapOrphanCarriesPlannedID 覆盖 §6.2「Meta 里带得住
// planned_session_id」唯一真实存在的形状：grok 的 fresh（bootstrap）轮预铸了
// PlannedSessionID、ResumeSessionID 为空、provider_sessions 没有那一行。
// 这一形状下 baseline_suspect 无行可标（M=0），所以它与主用例的
// 「baseline_suspect == 1」在今天的代码里结构上不能同时成立。
func TestS10cC2GrokBootstrapOrphanCarriesPlannedID(t *testing.T) {
	isolateAppEnv(t)
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	cfgPath, dataDir := s10cConfig(t)

	s10cRun(t, cfgPath, "/new s10c-grok", "/join g1", s10cMsgA, "/quit")
	convID := s10cConvID(t, dataDir)
	var seqA int64
	for _, m := range s10cMessages(t, dataDir) {
		if m.kind == "user" {
			seqA = m.seq
		}
	}
	if _, ok := s10cProviderSession(t, dataDir, "g1"); ok {
		t.Fatalf("前置：g1 从没开过口，不该有 provider_sessions 行")
	}
	const turnID = "5c10c2cc-2222-4222-8222-00000000g1g1"
	s10cSeedInflight(t, dataDir, conversation.InflightTurn{
		TurnID:           turnID,
		ConvID:           convID,
		AgentID:          "g1",
		Provider:         "grok",
		PlannedSessionID: s10cPlannedGrok,
		ResumeSessionID:  "",
		CursorSeq:        0,
		ScannedTo:        seqA,
		StartedAt:        time.Now().UTC(),
		Tier:             "read",
		WorkDir:          filepath.Join(dataDir, "workdir", "g1"),
	})

	outA, errA := s10cRun(t, cfgPath)
	const first = "注意: 有 1 轮在上次退出时没记上账，其中 0 轮已把相关成员的成本基线标为可疑"
	if !strings.Contains(outA, first) {
		t.Errorf("bootstrap 崩溃横幅首行应为 %q；Out:\n%s\nErr:\n%s", first, outA, errA)
	}
	if n := s10cQueryInt(t, dataDir, `SELECT COUNT(*) FROM inflight_turns`); n != 0 {
		t.Errorf("对账之后 inflight_turns 应为空，实际 %d", n)
	}
	var lost []s10cMsg
	for _, m := range s10cMessages(t, dataDir) {
		if ev, _ := m.meta[turn.MetaEvent].(string); ev == "turn_lost" {
			lost = append(lost, m)
		}
	}
	if len(lost) != 1 {
		t.Fatalf("应恰好一条 turn_lost 面包屑，实际 %d", len(lost))
	}
	if got, _ := lost[0].meta[turn.MetaOrphanSession].(string); got != s10cPlannedGrok {
		t.Errorf("bootstrap 崩溃面包屑 meta.%s 应为预铸的 planned id %q，实际 %q", turn.MetaOrphanSession, s10cPlannedGrok, got)
	}
	if _, ok := s10cProviderSession(t, dataDir, "g1"); ok {
		t.Errorf("bootstrap 崩溃的对账不该凭空造出 provider_sessions 行")
	}
	n := s10cQueryInt(t, dataDir, `SELECT COUNT(*) FROM turn_usage WHERE turn_id = ? AND outcome = ? AND cost_usd IS NULL AND provider_session_id = ''`, turnID, int(turn.OutcomeLost))
	if n != 1 {
		t.Errorf("bootstrap 崩溃仍应补一条 OutcomeLost 的 turn_usage（provider_session_id 空串、cost NULL），实际 %d 行", n)
	}
}
