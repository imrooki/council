package app

// CX1 测试腿：codex 成员的端到端验收（契约 E:\multi_model_cowork\cx1_contract.md §2.3，A1）。
//
// 只用 HEAD 已有标识符：provider CLI 换成测试二进制自己（COUNCIL_FAKE_CLI=1，
// smoke_test.go 的 TestMain 与 runFakeProviderCLI），它读掉 stdin、把
// S10D_FAKE_STDOUT_FILE 指向的文件原样吐到 stdout。不新写 TestMain，不断言屏幕
// （CO1 会改屏幕），只查库。store 开在 t.TempDir() 之下；COUNCIL_GLOBAL_RULES
// 指向一个不存在的文件，免得读到真全局规则文件。

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
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

const cxxThreadID = "01a0e373-ac6c-7083-87d7-62536800ada0"

// cxxConfigBody 是契约 §2.3 第 2 步的配置正文，只有 effort 一处可变。
func cxxConfigBody(dataDir, effort string) string {
	return "data_dir = " + tomlLiteral(dataDir) + "\n\n" +
		"[providers.codex]\n" +
		"command = " + tomlLiteral(os.Args[0]) + "\n\n" +
		"[agents.sol]\n" +
		"display_name = \"Sol\"\n" +
		"provider = \"codex\"\n" +
		"model = \"gpt-6-sol\"\n" +
		"effort = \"" + effort + "\"\n" +
		"role = \"cxx\"\n" +
		"permissions = \"read\"\n" +
		"mode = \"discussion\"\n"
}

func cxxOpenDB(t *testing.T, dataDir string) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(dataDir, "council.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatalf("打开 %s 失败：%v", dbPath, err)
	}
	return db
}

func cxxCount(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("查询失败：%v（%s）", err, q)
	}
	return n
}

// TestCxxCodexResumesAcrossRestart 覆盖 A1：两次 app.Run 启动，第一次开会话、第二次从库里
// 读回会话续上；effort_changed 子测试在第二次启动前改了强度。
//
// 【判别力】这是唯一一条走真 spawnProcess（stdin 经管道写给真子进程、stdout 的 CRLF 原样
// 回到 ParseTurn）、真 app.Run 两次启动（指纹与会话从库里读回）的用例。same_config 的
// session_rebuilt == 0、session_opening == 1 由只差 effort 的 effort_changed（1、2）作
// 正对照；HEAD 上 codex 没有登记，两个子测试的 agent 消息都为空、provider_sessions 没有行；
// 未报清单少一项（X39、X40）时 token 列非 NULL；Register 去掉（X48）时同 HEAD。
func TestCxxCodexResumesAcrossRestart(t *testing.T) {
	cases := []struct {
		name        string
		effort2     string
		wantOpening int
		wantRebuilt int
	}{
		{"same_config", "low", 1, 0},
		{"effort_changed", "high", 2, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			isolateAppEnv(t)
			t.Setenv("COUNCIL_FAKE_CLI", "1")
			dir := t.TempDir()
			dataDir := filepath.Join(dir, "data")
			fake := filepath.Join(dir, "fake_stdout.jsonl")
			t.Setenv("S10D_FAKE_STDOUT_FILE", fake)
			t.Setenv("COUNCIL_GLOBAL_RULES", filepath.Join(dir, "no-such-COUNCIL.md"))
			ctx := context.Background()

			// 第一次启动：新会话，sol 首轮。
			cfgPath := writeTestConfig(t, dir, cxxConfigBody(dataDir, "low"))
			if err := os.WriteFile(fake, []byte(cxxExec1CRLF), 0o644); err != nil {
				t.Fatalf("写假 stdout 失败：%v", err)
			}
			out, errOut, err := runAppForTest(t, ctx, cfgPath, strings.NewReader("/new t\n/join sol\nhello-cxx\n/ask sol\n/quit\n"))
			if err != nil {
				t.Fatalf("前提：第一次 app.Run 应返回 nil，实际 %v\nOut:\n%s\nErr:\n%s", err, out, errOut)
			}
			db := cxxOpenDB(t, dataDir)
			var ids []string
			rows, err := db.Query(`SELECT id FROM conversations`)
			if err != nil {
				db.Close()
				t.Fatalf("查 conversations 失败：%v", err)
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					db.Close()
					t.Fatalf("扫 conversations 失败：%v", err)
				}
				ids = append(ids, id)
			}
			rows.Close()
			db.Close()
			if len(ids) != 1 {
				t.Fatalf("前提：conversations 应恰好一行，实际 %d", len(ids))
			}
			convID := ids[0]

			// 第二次启动：用子测试的第二个 effort 重写配置，续会话。
			writeTestConfig(t, dir, cxxConfigBody(dataDir, c.effort2))
			if err := os.WriteFile(fake, []byte(cxxExec2CRLF), 0o644); err != nil {
				t.Fatalf("写假 stdout 失败：%v", err)
			}
			out, errOut, err = runAppForTest(t, ctx, cfgPath, strings.NewReader("/use "+convID+"\nsecond-cxx\n/ask sol\n/quit\n"))
			if err != nil {
				t.Fatalf("第二次 app.Run 应返回 nil，实际 %v\nOut:\n%s\nErr:\n%s", err, out, errOut)
			}

			db = cxxOpenDB(t, dataDir)
			defer db.Close()
			var texts []string
			trows, err := db.Query(`SELECT text FROM messages WHERE kind = 'agent' ORDER BY seq`)
			if err != nil {
				t.Fatalf("查 agent 消息失败：%v", err)
			}
			for trows.Next() {
				var s string
				if err := trows.Scan(&s); err != nil {
					trows.Close()
					t.Fatalf("扫 agent 消息失败：%v", err)
				}
				texts = append(texts, s)
			}
			trows.Close()
			if want := []string{"PONG", "KIWI"}; !reflect.DeepEqual(texts, want) {
				t.Errorf("agent 消息 = %q，期望 %q\nOut(第二次):\n%s", texts, want, out)
			}
			if n := cxxCount(t, db, `SELECT COUNT(*) FROM messages WHERE kind = 'system' AND json_extract(meta, '$.event') = 'session_opening'`); n != c.wantOpening {
				t.Errorf("session_opening 条数 = %d，期望 %d", n, c.wantOpening)
			}
			if n := cxxCount(t, db, `SELECT COUNT(*) FROM messages WHERE kind = 'system' AND json_extract(meta, '$.event') = 'session_rebuilt'`); n != c.wantRebuilt {
				t.Errorf("session_rebuilt 条数 = %d，期望 %d", n, c.wantRebuilt)
			}
			var psIDs []string
			prows, err := db.Query(`SELECT provider_session_id FROM provider_sessions WHERE agent_id = 'sol'`)
			if err != nil {
				t.Fatalf("查 provider_sessions 失败：%v", err)
			}
			for prows.Next() {
				var s string
				if err := prows.Scan(&s); err != nil {
					prows.Close()
					t.Fatalf("扫 provider_sessions 失败：%v", err)
				}
				psIDs = append(psIDs, s)
			}
			prows.Close()
			if want := []string{cxxThreadID}; !reflect.DeepEqual(psIDs, want) {
				t.Errorf("provider_sessions(sol) = %q，期望恰好一行 %q", psIDs, want)
			}
			if n := cxxCount(t, db, `SELECT COUNT(*) FROM turn_usage WHERE outcome = 1`); n != 2 {
				t.Errorf("turn_usage 里 outcome = 1 的行数 = %d，期望 2", n)
			}
			if n := cxxCount(t, db, `SELECT COUNT(*) FROM turn_usage WHERE outcome = 1 AND input_tokens IS NULL AND cache_read IS NULL AND output_tokens IS NULL`); n != 2 {
				t.Errorf("outcome = 1 且 input_tokens / cache_read / output_tokens 都为 NULL 的行数 = %d，期望 2", n)
			}
		})
	}
}
