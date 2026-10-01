package app

// 本文件是 Step 10 A 之五（契约 s10a5 v1.1 §2.4）的 app 包验收：全栈、真库地证明
// 「启动对账补账失败则该成员 /ask 被拒」与「对账成功即解开」。只用 HEAD 已有的标识符。
// 共用 smoke_test.go 的 TestMain（COUNCIL_FAKE_CLI=1 时本二进制扮演 claude CLI）。

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/store"

	_ "modernc.org/sqlite"
)

// s10aRefusalSentence 逐字复制契约 §3.1。
const s10aRefusalSentence = "这个成员有一轮没记上账的在飞记录还留在库里，对账成功之前不再起进程、不花钱；重启 council 会在启动时重新对账，对账失败的原因在启动输出里以「启动在飞对账」开头的那一行"

// s10aSettleFailPrefix 是 app.go 里补账失败那一行的前缀（契约 §3.5，只作前提守卫）。
const s10aSettleFailPrefix = "启动在飞对账补账失败: "

// s10aSeedDB 种库：会话、成员 c1、一条 user 消息；settleFails 时先记一行同 turn_id 的
// turn_usage（让启动对账补账撞主键、整笔回滚），然后写入在飞记录。顺序冻结：先 RecordTurn
// 后 BeginTurn，反过来 RecordTurn 会把在飞记录清掉。
func s10aSeedDB(t *testing.T, dataDir string, settleFails bool) string {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("store.Open 失败: %v", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Fatalf("关闭种库 store 失败: %v", err)
		}
	}()
	svc := conversation.NewService(st)
	conv, err := svc.Create(ctx, "s10a")
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if err := svc.Join(ctx, conv.ID, conversation.Participant{AgentID: "c1", Provider: "claude", JoinedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("Join 失败: %v", err)
	}
	if _, err := svc.Post(ctx, conversation.Message{ConvID: conv.ID, SenderID: conversation.ReservedSenderUser, Kind: conversation.KindUser, Text: "hi"}); err != nil {
		t.Fatalf("Post 失败: %v", err)
	}
	if settleFails {
		if _, err := svc.RecordTurn(ctx, nil, conversation.TurnUsage{TurnID: "s10a-stale-turn", ConvID: conv.ID, AgentID: "c1", Outcome: 7, CreatedAt: time.Now().UTC()}, nil, false); err != nil {
			t.Fatalf("前提不成立：种 turn_usage 失败: %v", err)
		}
	}
	if err := svc.BeginTurn(ctx, conversation.InflightTurn{TurnID: "s10a-stale-turn", ConvID: conv.ID, AgentID: "c1", Provider: "claude", StartedAt: time.Now().UTC(), Phase: conversation.PhaseSpawned, Tier: "read"}); err != nil {
		t.Fatalf("前提不成立：写入在飞记录失败: %v", err)
	}
	return conv.ID
}

// s10aCount 在库文件上跑一条 COUNT 查询。
func s10aCount(t *testing.T, db *sql.DB, q string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("查询 %q 失败: %v", q, err)
	}
	return n
}

// s10aLines 按 "\n" 切行。
func s10aLines(s string) []string {
	return strings.Split(s, "\n")
}

// s10aHasLine 判断是否有整行等于 want。
func s10aHasLine(lines []string, want string) bool {
	for _, l := range lines {
		if l == want {
			return true
		}
	}
	return false
}

// s10aHasPrefixLine 判断是否有一行以 prefix 开头（只用于前提守卫）。
func s10aHasPrefixLine(lines []string, prefix string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}

// TestS10aSettleFailureBlocksAskUntilSettled 是 E1。
//
// 【判别力】两个子测试只差「启动对账成不成功」一处。settle_fails 同时担保「settle 失败则
// 被拒」（13 §3.15 第 10 条）与那句话（askCause 新分支失效，§4 #9，屏幕上缺整行 §3.1
// 而红；HEAD 上同样红）；settle_ok 担保「对账成功就解开」（唯一解开路径），恒拒的实现
// 下它红。E1 对 §4 #1 杀不到：真库下落到唯一索引的第二道，照样同一句话。
func TestS10aSettleFailureBlocksAskUntilSettled(t *testing.T) {
	for _, settleFails := range []bool{true, false} {
		name := "settle_ok"
		if settleFails {
			name = "settle_fails"
		}
		settleFails := settleFails
		t.Run(name, func(t *testing.T) {
			isolateAppEnv(t)
			t.Setenv("COUNCIL_FAKE_CLI", "1")
			dir := t.TempDir()
			dataDir := filepath.Join(dir, "data")
			cfgBody := fmt.Sprintf(`data_dir = %s

[providers.claude]
command = %s

[agents.c1]
display_name = "C1"
provider = "claude"
model = "opus"
effort = "high"
role = "s10a"
permissions = "read"
mode = "discussion"
`, tomlLiteral(dataDir), tomlLiteral(os.Args[0]))
			cfgPath := writeTestConfig(t, dir, cfgBody)

			convID := s10aSeedDB(t, dataDir, settleFails)

			ctx := context.Background()
			out, errOut, err := runAppForTest(t, ctx, cfgPath, strings.NewReader("/use "+convID+"\n/ask c1\n/quit\n"))
			if err != nil {
				t.Fatalf("app.Run 返回 %v；\nOut:\n%s\nErr:\n%s", err, out, errOut)
			}

			db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dataDir, "council.db")))
			if err != nil {
				t.Fatalf("打开库失败: %v", err)
			}
			defer db.Close()

			outLines := s10aLines(out)
			errLines := s10aLines(errOut)
			if settleFails {
				if !s10aHasPrefixLine(errLines, s10aSettleFailPrefix) {
					t.Fatalf("前提不成立：Err 里没有以 %q 开头的行（启动对账没有失败）；Err:\n%s", s10aSettleFailPrefix, errOut)
				}
				if !s10aHasLine(outLines, "outcome refused  agent c1") {
					t.Errorf("Out 里没有整行 %q；Out:\n%s", "outcome refused  agent c1", out)
				}
				if !s10aHasLine(outLines, s10aRefusalSentence) {
					t.Errorf("Out 里没有整行 §3.1 那句话；Out:\n%s", out)
				}
				if s10aHasLine(outLines, "outcome spoke  agent c1") {
					t.Errorf("Out 里不该有整行 %q；Out:\n%s", "outcome spoke  agent c1", out)
				}
				if n := s10aCount(t, db, `SELECT COUNT(*) FROM inflight_turns`); n != 1 {
					t.Errorf("inflight_turns 行数 = %d，期望 1", n)
				}
				if n := s10aCount(t, db, `SELECT COUNT(*) FROM inflight_turns WHERE turn_id = 's10a-stale-turn' AND phase = 'spawned'`); n != 1 {
					t.Errorf("inflight_turns 里 s10a-stale-turn / spawned 行数 = %d，期望 1", n)
				}
				if n := s10aCount(t, db, `SELECT COUNT(*) FROM turn_usage WHERE outcome = 1`); n != 0 {
					t.Errorf("turn_usage outcome=1 行数 = %d，期望 0", n)
				}
				if n := s10aCount(t, db, `SELECT COUNT(*) FROM turn_usage WHERE outcome = 7`); n != 2 {
					t.Errorf("turn_usage outcome=7 行数 = %d，期望 2（种下的一行 + 本次被拒的一行）", n)
				}
			} else {
				if s10aHasPrefixLine(errLines, "启动在飞对账补账失败") {
					t.Errorf("Err 里不该有补账失败的行；Err:\n%s", errOut)
				}
				if !s10aHasLine(outLines, "outcome spoke  agent c1") {
					t.Errorf("Out 里没有整行 %q；Out:\n%s", "outcome spoke  agent c1", out)
				}
				if s10aHasLine(outLines, s10aRefusalSentence) {
					t.Errorf("Out 里不该有整行 §3.1 那句话；Out:\n%s", out)
				}
				if n := s10aCount(t, db, `SELECT COUNT(*) FROM inflight_turns`); n != 0 {
					t.Errorf("inflight_turns 行数 = %d，期望 0", n)
				}
				if n := s10aCount(t, db, `SELECT COUNT(*) FROM turn_usage WHERE outcome = 9`); n != 1 {
					t.Errorf("turn_usage outcome=9 行数 = %d，期望 1", n)
				}
				if n := s10aCount(t, db, `SELECT COUNT(*) FROM turn_usage WHERE outcome = 1`); n != 1 {
					t.Errorf("turn_usage outcome=1 行数 = %d，期望 1", n)
				}
			}
		})
	}
}
