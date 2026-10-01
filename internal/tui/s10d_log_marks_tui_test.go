package tui

// 本文件覆盖冻结契约 E:\multi_model_cowork\s10d2_contract.md（v1.1）§2.3：
// TUI 走 LineWriter 出口时，/log 的标记与行式 REPL 逐字节相同。
//
// 不起真 Program：printLine（model.go）对非空行是恒等的，LineWriter 只切行不改字节，
// 所以直接把 LineWriter 当 REPL 的 io.Writer 就能看到 TUI 这一侧会收到的每一行。
//
// 背对背纪律：本文件不引用任何新标识符，在 HEAD 上编译得过；断言 1 在 HEAD 上红。

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/provider"
	"council/internal/repl"
	"council/internal/store"
)

// s10dTUIHdr 是契约 §3.2 的 R_HDR（前面带提示符）。
const s10dTUIHdr = "council> SEQ  SENDER  KIND    TO  BRAIN  TEXT"

// s10dTUIWant 是契约 §3.6 的期望 4 行，逐字节照抄。
var s10dTUIWant = []string{
	"1    system  system  -   -      中文正文 [event=turn_lost]",
	"2    system  system  -   -      中文正文 [thought_dropped]",
	"3    system  system  -   -      中文正文 [denied_actions]",
	"4    system  system  -   -      中文正文",
}

// TestS10dLogMarksTUILineWriter 是 U1：同一个 svc、同一份输入，行式 REPL 与 LineWriter 出口各跑一遍。
//
// 【判别力】断言 1 在 HEAD 上红（没有标记）；「T 只对 agent 打」「D 只认 []provider.DeniedAction」
// 之外的「D 只认别的形状」、分隔符、字面量、加一列等写法也会让某一行不等。
// 断言 2 是回归钉：LineWriter 只切行不改字节，今天恒真（结构上恒真，单独不构成本刀的证据），
// 写下来是为了哪天有人在 TUI 这一侧对 /log 的行做了加工时它会红。
func TestS10dLogMarksTUILineWriter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "council-data"))
	if err != nil {
		t.Fatalf("前提：store.Open 失败：%v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("关闭测试 store 失败：%v", err)
		}
	})
	svc := conversation.NewService(st)
	conv, err := svc.Create(ctx, "s10d")
	if err != nil {
		t.Fatalf("前提：Create 失败：%v", err)
	}

	da := []provider.DeniedAction{{Action: "write_file", DisplayName: "WriteToFile"}}
	metas := []map[string]any{
		{"event": "turn_lost"},
		{"thought_dropped": true},
		{"denied_actions": da},
		nil,
	}
	for i, meta := range metas {
		m := conversation.Message{
			ConvID:   conv.ID,
			SenderID: conversation.ReservedSenderSystem,
			Kind:     conversation.KindSystem,
			Text:     "中文正文",
			Meta:     meta,
		}
		if _, err := svc.Post(ctx, m); err != nil {
			t.Fatalf("前提：第 %d 条种子 Post 失败：%v", i+1, err)
		}
	}
	input := "/use " + conv.ID + "\n/log\n"

	// 行式
	var buf bytes.Buffer
	if err := repl.New(nil, svc, nil, strings.NewReader(input), &buf, repl.RulesContext{}).Run(ctx); err != nil {
		t.Fatalf("前提：行式 REPL.Run 应返回 nil，实际 %v；输出：\n%s", err, buf.String())
	}

	// TUI 出口
	var mu sync.Mutex
	var got []string
	lw := NewLineWriter(func(line string) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, line)
	})
	if err := repl.New(nil, svc, nil, strings.NewReader(input), lw, repl.RulesContext{}).Run(ctx); err != nil {
		t.Fatalf("前提：LineWriter 出口的 REPL.Run 应返回 nil，实际 %v", err)
	}
	lw.Flush()

	mu.Lock()
	lines := append([]string(nil), got...)
	mu.Unlock()

	// 断言 1：R_HDR 恰好一行，紧跟其后的 4 行逐行整行相等。
	idx := -1
	count := 0
	for i, l := range lines {
		if l == s10dTUIHdr {
			if idx < 0 {
				idx = i
			}
			count++
		}
	}
	if count != 1 {
		t.Fatalf("LineWriter 吐出的行里表头 %q 应恰好一行，实际 %d 行；全部行：\n%s", s10dTUIHdr, count, strings.Join(lines, "\n"))
	}
	if idx+1+len(s10dTUIWant) > len(lines) {
		t.Fatalf("表头之后应至少还有 %d 行，实际只有 %d 行；全部行：\n%s", len(s10dTUIWant), len(lines)-idx-1, strings.Join(lines, "\n"))
	}
	for k, w := range s10dTUIWant {
		if g := lines[idx+1+k]; g != w {
			t.Errorf("断言 1：表头后第 %d 行不符：\n实际 %q\n期望 %q", k+1, g, w)
		}
	}

	// 断言 2：LineWriter 吐出的行与行式输出按 "\n" 切开的行逐行相等，长度也相等。
	plain := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != len(plain) {
		t.Errorf("断言 2：LineWriter 吐出 %d 行，行式输出切出 %d 行；\nLineWriter：\n%s\n行式：\n%s",
			len(lines), len(plain), strings.Join(lines, "\n"), buf.String())
	} else {
		for i := range lines {
			if lines[i] != plain[i] {
				t.Errorf("断言 2：第 %d 行不同：\nLineWriter %q\n行式       %q", i+1, lines[i], plain[i])
			}
		}
	}
}
