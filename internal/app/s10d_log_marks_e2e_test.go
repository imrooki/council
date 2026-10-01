package app

// 本文件覆盖冻结契约 E:\multi_model_cowork\s10d2_contract.md（v1.1）§2.1：
// /log 的 TEXT 格末尾三种标记，在真 app.Run、真库、真 Runner、假 CLI 上端到端验收。
//
// 假 CLI 是测试二进制自己（smoke_test.go 的 TestMain + COUNCIL_FAKE_CLI=1）；
// 本文件经 S10D_FAKE_STDOUT_FILE（§2.0 那处授权改动）让它改吐一份 grok / agy 的原件，
// 因为 thought 只有 grok 给，denied_actions 只有 agy 给，claude 的原件两样都没有。
//
// 背对背纪律：本文件不引用任何新标识符，在 HEAD 上编译得过；E1b、E2b 在 HEAD 上绿（回归钉），
// 其余三条在 HEAD 上红在「期望有标记的那一行」上。

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// s10dGrokT1JSON 逐字节复制自 internal/turn/fixtures_test.go 的 grokT1JSON
// （那份注释说明它来自 E:\Isolating_test\council\step0-probe\grok-t1.json，grok 冷启动首轮的
// 真实抓包，thought 非空，text 是 OK）。又多一份拷贝：此前 internal/provider/fixtures_test.go
// 与 internal/turn/fixtures_test.go 各有一份，本文件是第三份；重新抓包时三处要一起改。
// 不手写的理由同 smoke_test.go 的 fakeClaudeSuccessJSON：grok 的 ParseTurn 有自己的
// 成功判据，手写的 JSON 容易漏字段而不自知。
const s10dGrokT1JSON = `{
  "text": "OK",
  "stopReason": "end_turn",
  "sessionId": "c0117e5a-4b21-4d3f-9f8e-2a6b1d0e7a31",
  "requestId": "9582f1ad-3946-401e-8d0c-9315b94c86fe",
  "thought": "The user wants me to remember the number 8635 and reply with exactly \"OK\". There's also a user rule that says the first sentence of my reply must be \"收到，Griffin。\"\n\n\n",
  "usage": {
    "input_tokens": 20625,
    "cache_read_input_tokens": 0,
    "cache_creation_input_tokens": 0,
    "output_tokens": 723,
    "reasoning_tokens": 722,
    "total_tokens": 21348
  },
  "num_turns": 1,
  "modelUsage": {
    "grok-4.6-build": {
      "inputTokens": 20625,
      "outputTokens": 723,
      "cacheReadInputTokens": 0,
      "cacheCreationInputTokens": 0,
      "modelCalls": 1
    }
  }
}`

// s10dAgyDeniedJSON 逐字节复制自 internal/turn/fixtures_test.go 的 agyDeniedActionJSON
// （那份注释说明它来自 E:\Isolating_test\council\oq-agy-ws\rerun\raw.json：agy 在 WORKSPACE 档
// 被要求越界写文件时的真实输出，response 被整段清空，denied_actions 一个元素）。
// 又多一份拷贝：此前 internal/provider/fixtures_test.go 与 internal/turn/fixtures_test.go
// 各有一份，本文件是第三份；重新抓包时三处要一起改。
const s10dAgyDeniedJSON = `{"conversation_id":"835f4ebf-727b-4695-a1ee-f0a60ac26240","status":"SUCCESS","response":"","duration_seconds":9.8952514,"num_turns":1,"usage":{"input_tokens":16449,"output_tokens":1150,"thinking_tokens":980,"cache_read_tokens":0,"total_tokens":17599},"denied_actions":[{"action":"write_file","display_name":"WriteToFile"}]}`

// 契约 §3.3 的期望行，逐字节照抄（行尾没有空格）。
const (
	s10dGHdr     = "council> SEQ  SENDER  KIND    TO  BRAIN               TEXT"
	s10dG1       = "1    user    user    -   -                   hello-s10d"
	s10dG2       = "2    system  system  -   -                   正在为 agent g1 开启一条新的 provider 会话 [event=session_opening]"
	s10dG3Off    = "3    g1      agent   -   grok/grok-4.6/high  OK [thought_dropped]"
	s10dG3On     = "3    g1      agent   -   grok/grok-4.6/high  OK"
	s10dG3EOff   = "3    g1      agent   -   grok/grok-4.6/high  - [thought_dropped]"
	s10dG3EPlain = "3    g1      agent   -   grok/grok-4.6/high  -"

	s10dAHdr     = "council> SEQ  SENDER  KIND    TO  BRAIN                        TEXT"
	s10dA1       = "1    user    user    -   -                            hello-s10d"
	s10dA2       = "2    system  system  -   -                            正在为 agent g1 开启一条新的 provider 会话 [event=session_opening]"
	s10dA3Denied = "3    g1      agent   -   agy/gemini-3.8-flash-high/-  - [denied_actions]"
	s10dA3Plain  = "3    g1      agent   -   agy/gemini-3.8-flash-high/-  -"
)

// s10dScript 是契约 §3.3 的六行脚本，每行以 \n 结尾。
const s10dScript = "/new t\n/join g1\nhello-s10d\n/ask g1\n/log\n/quit\n"

// s10dE2EResult 是一次端到端运行的产物。
type s10dE2EResult struct {
	lines  []string // Out 按 "\n" 切开的行
	out    string   // Out 原文，失败时整段贴出
	errOut string   // Err 原文，失败时整段贴出
	dbPath string   // 库文件路径
}

// s10dRunE2E 是 §2.1 的共用装置：provider 是 "grok" 或 "agy"；storeThoughtLine 是
// store_thought 那一行（空串表示整行不写）；fakeStdout 是假 CLI 要吐的字节。
func s10dRunE2E(t *testing.T, providerName, storeThoughtLine, fakeStdout string) s10dE2EResult {
	t.Helper()

	// 第 1 步
	isolateAppEnv(t)
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")

	// 第 2 步
	fakePath := filepath.Join(dir, "fake_stdout.json")
	if err := os.WriteFile(fakePath, []byte(fakeStdout), 0o644); err != nil {
		t.Fatalf("写假 CLI 输出文件失败：%v", err)
	}
	t.Setenv("S10D_FAKE_STDOUT_FILE", fakePath)
	t.Setenv("COUNCIL_GLOBAL_RULES", filepath.Join(dir, "no-such-COUNCIL.md"))

	// 第 3 步：配置正文照 §3.3 逐字拼。
	var b strings.Builder
	b.WriteString("data_dir = " + tomlLiteral(dataDir) + "\n")
	if storeThoughtLine != "" {
		b.WriteString(storeThoughtLine + "\n")
	}
	b.WriteString("\n")
	switch providerName {
	case "grok":
		b.WriteString("[providers.grok]\n")
		b.WriteString("command = " + tomlLiteral(os.Args[0]) + "\n")
		b.WriteString("\n")
		b.WriteString("[agents.g1]\n")
		b.WriteString("display_name = \"G1\"\n")
		b.WriteString("provider = \"grok\"\n")
		b.WriteString("model = \"grok-4.6\"\n")
		b.WriteString("effort = \"high\"\n")
		b.WriteString("role = \"r\"\n")
		b.WriteString("permissions = \"read\"\n")
		b.WriteString("mode = \"discussion\"\n")
	case "agy":
		b.WriteString("[providers.agy]\n")
		b.WriteString("command = " + tomlLiteral(os.Args[0]) + "\n")
		b.WriteString("\n")
		b.WriteString("[agents.g1]\n")
		b.WriteString("display_name = \"G1\"\n")
		b.WriteString("provider = \"agy\"\n")
		b.WriteString("model = \"gemini-3.8-flash-high\"\n")
		b.WriteString("role = \"r\"\n")
		b.WriteString("permissions = \"read\"\n")
		b.WriteString("mode = \"discussion\"\n")
	default:
		t.Fatalf("装置只认 grok / agy，收到 %q", providerName)
	}
	cfgPath := writeTestConfig(t, dir, b.String())

	// 第 4 步
	out, errOut, err := runAppForTest(t, context.Background(), cfgPath, strings.NewReader(s10dScript))
	if err != nil {
		t.Fatalf("前提：app.Run 应当返回 nil，实际 %v；\nOut:\n%s\nErr:\n%s", err, out, errOut)
	}

	// 第 5 步
	return s10dE2EResult{
		lines:  strings.Split(out, "\n"),
		out:    out,
		errOut: errOut,
		dbPath: filepath.Join(dataDir, "council.db"),
	}
}

// s10dAgentRow 读库里唯一那条 agent 消息的 thought 与 meta（前提守卫用）。
// 不是恰好一行就 Fatalf。
func s10dAgentRow(t *testing.T, r s10dE2EResult) (string, map[string]any) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(r.dbPath))
	if err != nil {
		t.Fatalf("前提：打开 %s 失败：%v", r.dbPath, err)
	}
	defer db.Close()

	rows, err := db.QueryContext(context.Background(), `SELECT thought, meta FROM messages WHERE kind = 'agent'`)
	if err != nil {
		t.Fatalf("前提：查询 agent 消息失败：%v", err)
	}
	defer rows.Close()
	type row struct {
		thought string
		meta    string
	}
	var got []row
	for rows.Next() {
		var x row
		if scanErr := rows.Scan(&x.thought, &x.meta); scanErr != nil {
			t.Fatalf("前提：扫 agent 消息失败：%v", scanErr)
		}
		got = append(got, x)
	}
	if iterErr := rows.Err(); iterErr != nil {
		t.Fatalf("前提：遍历 agent 消息失败：%v", iterErr)
	}
	if len(got) != 1 {
		t.Fatalf("前提：agent 消息应恰好一行，实际 %d 行：%+v\nOut:\n%s\nErr:\n%s", len(got), got, r.out, r.errOut)
	}
	meta := map[string]any{}
	if err := json.Unmarshal([]byte(got[0].meta), &meta); err != nil {
		t.Fatalf("前提：agent 消息的 meta 不是 JSON 对象：%v；原文 %q", err, got[0].meta)
	}
	return got[0].thought, meta
}

// s10dCountRun 数 lines 里有几处「从某一行起连续 len(want) 行逐行整行等于 want」。
func s10dCountRun(lines, want []string) int {
	n := 0
	for i := 0; i+len(want) <= len(lines); i++ {
		match := true
		for k := range want {
			if lines[i+k] != want[k] {
				match = false
				break
			}
		}
		if match {
			n++
		}
	}
	return n
}

// s10dCountEq 数 lines 里有几行整行等于 s。
func s10dCountEq(lines []string, s string) int {
	n := 0
	for _, l := range lines {
		if l == s {
			n++
		}
	}
	return n
}

// s10dCountContains 数 lines 里有几行含子串 sub（只用于反向全局扫描，只会让用例更严）。
func s10dCountContains(lines []string, sub string) int {
	n := 0
	for _, l := range lines {
		if strings.Contains(l, sub) {
			n++
		}
	}
	return n
}

// TestS10dE2EThoughtDroppedMarked 是 E1a：grok，store_thought = false，吐 grok 原件。
//
// 【判别力】HEAD 上 G2 与 G3_OFF 都不成立（没有标记），四行连续块数出 0 而不是 1。
// 能为假：「cmdLog 不调新函数」「标记拼在正文前」「标记取代正文」「分隔符两个空格」
// 「加一列」「标记字面量错一个字节」都会让这四行里至少一行不等。
// 只差一处的对照是 E1b（同一装置、同一份原件，只差 store_thought 那一行）：
// 那边 seq 3 是 G3_ON，与 G3_OFF 只差末尾的 " [thought_dropped]"。
// G2 同时担保 event 标记在真实生产面包屑（session_opening）上生效。
// 前提守卫担保 app.go 里 svc.SetThoughtPolicy(cfg.StoreThought) 那行接线：删掉它或改成
// SetThoughtPolicy(true)，thought 照存，守卫 Fatalf（契约 §4 #24、#25）。
func TestS10dE2EThoughtDroppedMarked(t *testing.T) {
	r := s10dRunE2E(t, "grok", "store_thought = false", s10dGrokT1JSON)

	thought, meta := s10dAgentRow(t, r)
	if thought != "" {
		t.Fatalf("前提：store_thought = false 时 agent 那一行的 thought 应为空串，实际 %q", thought)
	}
	if v, ok := meta["thought_dropped"].(bool); !ok || !v {
		t.Fatalf("前提：meta[\"thought_dropped\"] 应是 bool 的 true，实际 %#v（meta = %#v）", meta["thought_dropped"], meta)
	}

	want := []string{s10dGHdr, s10dG1, s10dG2, s10dG3Off}
	if n := s10dCountRun(r.lines, want); n != 1 {
		t.Errorf("四行 G_HDR/G1/G2/G3_OFF 应恰好连续出现一处，实际 %d 处；期望：\n%s\nOut:\n%s",
			n, strings.Join(want, "\n"), r.out)
	}
	if n := s10dCountEq(r.lines, s10dG3On); n != 0 {
		t.Errorf("不应有任何一行等于 G3_ON %q，实际 %d 行；Out:\n%s", s10dG3On, n, r.out)
	}
}

// TestS10dE2EThoughtKeptNotMarked 是 E1b：grok，store_thought = true，吐同一份 grok 原件。
//
// 【判别力】「标记总是打」的实现在这里红（G3_ON 找不到）。对照是 E1a：同一位置（seq 3）
// 在那边是 G3_OFF，两行只差末尾的 " [thought_dropped]"。thought != "" 这道前提守卫
// 排除「这一轮本来就没 thought」的巧合。本条在 HEAD 上绿，是回归钉，单独不构成本刀的证据。
func TestS10dE2EThoughtKeptNotMarked(t *testing.T) {
	r := s10dRunE2E(t, "grok", "store_thought = true", s10dGrokT1JSON)

	thought, meta := s10dAgentRow(t, r)
	if thought == "" {
		t.Fatalf("前提：store_thought = true 时 agent 那一行的 thought 应非空，实际为空串（meta = %#v）", meta)
	}
	if _, ok := meta["thought_dropped"]; ok {
		t.Fatalf("前提：store_thought = true 时 meta 不应有 thought_dropped 键，实际 meta = %#v", meta)
	}

	for _, want := range []string{s10dGHdr, s10dG1, s10dG3On} {
		if n := s10dCountEq(r.lines, want); n != 1 {
			t.Errorf("应恰好一行等于 %q，实际 %d 行；Out:\n%s", want, n, r.out)
		}
	}
	if n := s10dCountEq(r.lines, s10dG3Off); n != 0 {
		t.Errorf("不应有任何一行等于 G3_OFF %q，实际 %d 行；Out:\n%s", s10dG3Off, n, r.out)
	}
	if n := s10dCountContains(r.lines, "[thought_dropped]"); n != 0 {
		t.Errorf("Out 里不应有任何一行含 [thought_dropped]，实际 %d 行；Out:\n%s", n, r.out)
	}
}

// TestS10dE2EEmptyTextThoughtDroppedMarked 是 E1c（v1.1 新增）：grok，store_thought = false，
// 吐的是把原件里 "text": "OK", 换成 "text": "", 的同一份字节（正文清空，thought 不变）。
//
// 【判别力】钉「正文为空时只打 D、丢掉 E / T」（契约 §4 #29）：那种写法在这里打出 G3E_PLAIN。
// HEAD 上打出的也是 G3E_PLAIN（红），且四道前提守卫在 HEAD 上都成立。
// 对照一：E1a（同一份原件、同一个开关，只差 text 的字节；G3_OFF 与 G3E_OFF 只差 OK / -）。
// 对照二：E2a（同是占位符 - 加一个标记，换的是 T 与 D）。
// completion == "empty" 这道守卫担保 TEXT 格的 - 真是因为正文为空，不是标记吃掉了正文。
func TestS10dE2EEmptyTextThoughtDroppedMarked(t *testing.T) {
	fake := strings.Replace(s10dGrokT1JSON, "\"text\": \"OK\",", "\"text\": \"\",", 1)
	if fake == s10dGrokT1JSON {
		t.Fatalf("前提：替换没有发生，原件里找不到 \"text\": \"OK\",")
	}

	r := s10dRunE2E(t, "grok", "store_thought = false", fake)

	thought, meta := s10dAgentRow(t, r)
	if thought != "" {
		t.Fatalf("前提：store_thought = false 时 agent 那一行的 thought 应为空串，实际 %q", thought)
	}
	if v, ok := meta["thought_dropped"].(bool); !ok || !v {
		t.Fatalf("前提：meta[\"thought_dropped\"] 应是 bool 的 true，实际 %#v（meta = %#v）", meta["thought_dropped"], meta)
	}
	if meta["completion"] != "empty" {
		t.Fatalf("前提：meta[\"completion\"] 应是 \"empty\"，实际 %#v（meta = %#v）", meta["completion"], meta)
	}

	want := []string{s10dGHdr, s10dG1, s10dG2, s10dG3EOff}
	if n := s10dCountRun(r.lines, want); n != 1 {
		t.Errorf("四行 G_HDR/G1/G2/G3E_OFF 应恰好连续出现一处，实际 %d 处；期望：\n%s\nOut:\n%s",
			n, strings.Join(want, "\n"), r.out)
	}
	if n := s10dCountEq(r.lines, s10dG3EPlain); n != 0 {
		t.Errorf("不应有任何一行等于 G3E_PLAIN %q，实际 %d 行；Out:\n%s", s10dG3EPlain, n, r.out)
	}
}

// TestS10dE2EDeniedActionsMarked 是 E2a：agy，不写 store_thought，吐 agy 的越界原件。
//
// 【判别力】正文被清空的这一行是最有分辨力的形状：TEXT 格是占位符 - 加标记。
// 标记吃掉占位符、占位符挪到拼接之后（orPlaceholder(oneLine(Text)+标记)）、正文为空时不打标记、
// D 只认 []provider.DeniedAction（真库读回的是 []any）、D 同时印条数，都会改变 A3 这一行。
// HEAD 上 A2 与 A3_DENIED 都不成立（红）。对照是 E2b（只差 denied_actions 这个键）。
func TestS10dE2EDeniedActionsMarked(t *testing.T) {
	r := s10dRunE2E(t, "agy", "", s10dAgyDeniedJSON)

	_, meta := s10dAgentRow(t, r)
	da, ok := meta["denied_actions"].([]any)
	if !ok || len(da) != 1 {
		t.Fatalf("前提：meta[\"denied_actions\"] 应是长度 1 的 []any，实际 %#v（meta = %#v）", meta["denied_actions"], meta)
	}

	want := []string{s10dAHdr, s10dA1, s10dA2, s10dA3Denied}
	if n := s10dCountRun(r.lines, want); n != 1 {
		t.Errorf("四行 A_HDR/A1/A2/A3_DENIED 应恰好连续出现一处，实际 %d 处；期望：\n%s\nOut:\n%s",
			n, strings.Join(want, "\n"), r.out)
	}
	if n := s10dCountEq(r.lines, s10dA3Plain); n != 0 {
		t.Errorf("不应有任何一行等于 A3_PLAIN %q，实际 %d 行；Out:\n%s", s10dA3Plain, n, r.out)
	}
}

// TestS10dE2ENoDeniedActionsNotMarked 是 E2b：agy，不写 store_thought，吐的是 agy 越界原件
// 去掉 denied_actions 这个键之后的同一份字节。
//
// 【判别力】与 E2a 只差 denied_actions 这个键，A3_PLAIN 与 A3_DENIED 只差末尾的
// " [denied_actions]"。「标记总是打」「D 不看长度 / 键在就打」的实现在这里红。
// 本条在 HEAD 上绿，是回归钉，单独不构成本刀的证据。
func TestS10dE2ENoDeniedActionsNotMarked(t *testing.T) {
	fake := strings.Replace(s10dAgyDeniedJSON, `,"denied_actions":[{"action":"write_file","display_name":"WriteToFile"}]`, "", 1)
	if fake == s10dAgyDeniedJSON {
		t.Fatalf("前提：替换没有发生，原件里找不到 denied_actions 那一段")
	}

	r := s10dRunE2E(t, "agy", "", fake)

	_, meta := s10dAgentRow(t, r)
	if _, ok := meta["denied_actions"]; ok {
		t.Fatalf("前提：meta 不应有 denied_actions 键，实际 meta = %#v", meta)
	}

	for _, want := range []string{s10dAHdr, s10dA1, s10dA3Plain} {
		if n := s10dCountEq(r.lines, want); n != 1 {
			t.Errorf("应恰好一行等于 %q，实际 %d 行；Out:\n%s", want, n, r.out)
		}
	}
	if n := s10dCountEq(r.lines, s10dA3Denied); n != 0 {
		t.Errorf("不应有任何一行等于 A3_DENIED %q，实际 %d 行；Out:\n%s", s10dA3Denied, n, r.out)
	}
	if n := s10dCountContains(r.lines, "[denied_actions]"); n != 0 {
		t.Errorf("Out 里不应有任何一行含 [denied_actions]，实际 %d 行；Out:\n%s", n, r.out)
	}
}
