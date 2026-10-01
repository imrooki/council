package repl

// 本文件覆盖 X-39b 冻结契约（E:\multi_model_cowork\x39b_linemode_contract.md v1.1）
// 第 2 节的全部验收：一行模式里含控制字符的单行不记、不执行。
//
// 驱动方式：复用 tui0_seam_test.go 的 newSeamREPL（已选一个会话、out 是可清空的
// bytes.Buffer），每喂一行前清空缓冲区、直接调 r.dispatch，于是每一步的输出
// 是这一行独有的，不混提示符；落库效果查 fakeStore 的 f.msgs / f.convs。
// Run 的读循环只做按换行切行（bufio.ScanLines，行尾 CR 被剥），切出来的行
// 原样交给 dispatch，所以直接调 dispatch 与经 Run 喂一行在 dispatch 这一层等价。
//
// 冻结提示在这里写成字面量，不引用实现方命名的常量：名字由实现方定，
// 引用它会让本文件在 HEAD 上编不过，也就证明不了本文件抓得住今天的故障。
//
// 每一条「新行为」断言都配了一个只差一处的对照组（见各用例注释），
// 防的是断言在结构上恒真。
//
// 只新建本文件，不改任何既有 _test.go。

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

// x39bMsgControl 是契约第 3 节冻结的提示（逐字节）。打印时后面跟一个换行，
// 所以「打出提示」一律按 x39bMsgControl+"\n" 判；「不打提示」只按整句判（更宽，更难误放）。
const x39bMsgControl = "这一行含有控制字符，没有记录，也没有当成命令执行"

// x39bFeed 清空输出缓冲区后把一行交给 dispatch，返回这一行独有的输出。
// dispatch 在这些路径上只会返回打印失败的错误，写 bytes.Buffer 不会失败，
// 所以非 nil 一律视为失败。
func x39bFeed(t *testing.T, r *REPL, buf *bytes.Buffer, line string) string {
	t.Helper()
	buf.Reset()
	if err := r.dispatch(context.Background(), line); err != nil {
		t.Fatalf("dispatch(%q) 返回了非 nil 错误: %v；输出: %q", line, err, buf.String())
	}
	return buf.String()
}

// x39bMsgs 返回当前会话里的消息条数。
func x39bMsgs(r *REPL, f *fakeStore) int {
	return len(f.msgs[r.cur.ID])
}

// x39bLastText 返回当前会话最后一条消息的正文；没有消息时让用例失败。
func x39bLastText(t *testing.T, r *REPL, f *fakeStore) string {
	t.Helper()
	list := f.msgs[r.cur.ID]
	if len(list) == 0 {
		t.Fatalf("当前会话里没有消息")
	}
	return list[len(list)-1].Text
}

// x39bAssertBlocked 断言这一行被闸挡住：输出含冻结整句，消息条数不变。
func x39bAssertBlocked(t *testing.T, r *REPL, f *fakeStore, buf *bytes.Buffer, line string) {
	t.Helper()
	before := x39bMsgs(r, f)
	out := x39bFeed(t, r, buf, line)
	if !strings.Contains(out, x39bMsgControl+"\n") {
		t.Errorf("喂 %q 应当打出冻结提示 %q，实际输出: %q", line, x39bMsgControl, out)
	}
	if got := x39bMsgs(r, f); got != before {
		t.Errorf("喂 %q 不该记下任何消息，条数 %d -> %d；输出: %q", line, before, got, out)
	}
}

// x39bAssertRecorded 断言这一行照常记下恰好一条、正文逐字节等于 want，且不打冻结提示。
func x39bAssertRecorded(t *testing.T, r *REPL, f *fakeStore, buf *bytes.Buffer, line, want string) {
	t.Helper()
	before := x39bMsgs(r, f)
	out := x39bFeed(t, r, buf, line)
	if strings.Contains(out, x39bMsgControl) {
		t.Errorf("喂 %q 不该打出冻结提示，实际输出: %q", line, out)
	}
	if got := x39bMsgs(r, f); got != before+1 {
		t.Fatalf("喂 %q 应当恰好记下一条，条数 %d -> %d；输出: %q", line, before, got, out)
	}
	if got := x39bLastText(t, r, f); got != want {
		t.Errorf("喂 %q 记下的正文 = %q，期望逐字节 %q", line, got, want)
	}
}

// ============================================================
// 验收 1：方向键那一行不记
// ============================================================

// 对照：同一个全新 REPL 上喂 "你好" 条数加 1——证明「条数不变」不是因为
// 这个 REPL 根本记不下东西。
func TestX39b_01_ArrowKeysLineNotRecorded(t *testing.T) {
	t.Run("方向键序列被挡", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertBlocked(t, r, f, buf, "\x1b[A\x1b[A\x1b[A\x1b[B")
	})
	t.Run("对照_普通正文照常记下", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertRecorded(t, r, f, buf, "你好", "你好")
	})
}

// ============================================================
// 验收 2：带控制字符的命令不执行
// ============================================================

// 对照：去掉尾巴上的 ESC 序列，同一行就新建了一个会话、并切了过去。
// 被挡那一支还额外钉住当前会话没被换掉（/new 执行了的话 r.cur 会变）。
func TestX39b_02_CommandWithControlNotExecuted(t *testing.T) {
	t.Run("带ESC的new被挡", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		convsBefore := len(f.convs)
		curBefore := r.cur.ID
		out := x39bFeed(t, r, buf, "/new 标题甲\x1b[D")
		if !strings.Contains(out, x39bMsgControl+"\n") {
			t.Errorf("应当打出冻结提示，实际输出: %q", out)
		}
		if got := len(f.convs); got != convsBefore {
			t.Errorf("不该新建会话，会话总数 %d -> %d；输出: %q", convsBefore, got, out)
		}
		if r.cur == nil || r.cur.ID != curBefore {
			t.Errorf("当前会话不该被换掉，原来是 %q，现在是 %+v", curBefore, r.cur)
		}
	})
	t.Run("对照_干净的new照常执行", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		convsBefore := len(f.convs)
		out := x39bFeed(t, r, buf, "/new 标题甲")
		if strings.Contains(out, x39bMsgControl) {
			t.Errorf("不该打出冻结提示，实际输出: %q", out)
		}
		if got := len(f.convs); got != convsBefore+1 {
			t.Fatalf("应当新建恰好一个会话，会话总数 %d -> %d；输出: %q", convsBefore, got, out)
		}
		if got := f.convs[len(f.convs)-1].Title; got != "标题甲" {
			t.Errorf("新会话标题 = %q，期望 %q", got, "标题甲")
		}
	})
}

// ============================================================
// 验收 3：Tab 放行
// ============================================================

// 对照（TestX39bContrast_03）：只把 0x09 换成相邻的 0x0b（垂直制表），同一行就被挡。
func TestX39b_03_TabAllowed(t *testing.T) {
	t.Run("Tab放行", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertRecorded(t, r, f, buf, "a\tb", "a\tb")
	})
}

// TestX39bContrast_03 是验收 3 的新行为对照组（HEAD 上预期为红）。
func TestX39bContrast_03(t *testing.T) {
	t.Run("对照_垂直制表被挡", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertBlocked(t, r, f, buf, "a\x0bb")
	})
}

// ============================================================
// 验收 4：DEL 挡住
// ============================================================

// 对照：只把 0x7f 换成相邻的 0x7e（波浪号），同一行照常记下。
func TestX39b_04_DELBlocked(t *testing.T) {
	t.Run("DEL被挡", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertBlocked(t, r, f, buf, "abc\x7f")
	})
	t.Run("对照_波浪号照常记下", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertRecorded(t, r, f, buf, "abc\x7e", "abc~")
	})
}

// ============================================================
// 验收 5：行中间的回车挡住
// ============================================================

// 对照：只把 0x0d 换成空格，同一行照常记下。
func TestX39b_05_MidLineCRBlocked(t *testing.T) {
	t.Run("行中CR被挡", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertBlocked(t, r, f, buf, "a\rb")
	})
	t.Run("对照_空格照常记下", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertRecorded(t, r, f, buf, "a b", "a b")
	})
}

// ============================================================
// 验收 6：粘贴标记仍按粘贴处理
// ============================================================

// 对照（TestX39bContrast_06）：把 START 标记的 200 改成 199（不再是粘贴标记），行里的 ESC 就是
// 普通控制字符，同一行被挡——证明放行靠的是「这是一段粘贴」，不是闸漏了 ESC。
func TestX39b_06_PasteMarkersStillPaste(t *testing.T) {
	t.Run("括号粘贴照常记下", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertRecorded(t, r, f, buf, "\x1b[200~hello\x1b[201~", "hello")
		if r.inPaste {
			t.Errorf("一行里同时有 START 与 END，粘贴应当已经结束")
		}
	})
}

// TestX39bContrast_06 是验收 6 的新行为对照组（HEAD 上预期为红）。
func TestX39bContrast_06(t *testing.T) {
	t.Run("对照_不是粘贴标记的ESC序列被挡", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertBlocked(t, r, f, buf, "\x1b[199~hello\x1b[201~")
	})
}

// ============================================================
// 验收 6b：粘贴正文里的控制字符照原样记下
// ============================================================

// 三条断言缺一不可：恰好一条、正文逐字节 "a\x0cb"、不打冻结提示。
// 对照（TestX39bContrast_06b）：同样的正文不带粘贴标记，被挡。
func TestX39b_06b_ControlInsidePasteKept(t *testing.T) {
	t.Run("粘贴里的换页符照原样记下", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertRecorded(t, r, f, buf, "\x1b[200~a\x0cb\x1b[201~", "a\x0cb")
	})
}

// TestX39bContrast_06b 是验收 6b 的新行为对照组（HEAD 上预期为红）。
func TestX39bContrast_06b(t *testing.T) {
	t.Run("对照_不在粘贴里被挡", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertBlocked(t, r, f, buf, "a\x0cb")
	})
}

// ============================================================
// 验收 7：草稿不受影响
// ============================================================

// 与 HEAD 相同：草稿行那一步没有输出；/end 输出「已记下 seq」；恰好多一条，
// 正文逐字节 "a\x1b[Ab"；全程不打冻结提示。
// 对照（TestX39bContrast_07）：同一行不在草稿里被挡。
func TestX39b_07_DraftUnaffected(t *testing.T) {
	t.Run("草稿里的ESC照原样发出", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		before := x39bMsgs(r, f)

		outSay := x39bFeed(t, r, buf, "/say")
		if !r.inDraft {
			t.Fatalf("/say 不带参数应当进入草稿；输出: %q", outSay)
		}
		outLine := x39bFeed(t, r, buf, "a\x1b[Ab")
		if outLine != "" {
			t.Errorf("喂草稿行那一步不该有任何输出，实际: %q", outLine)
		}
		outEnd := x39bFeed(t, r, buf, "/end")
		if !strings.Contains(outEnd, "已记下 seq") {
			t.Errorf("/end 应当输出「已记下 seq」，实际: %q", outEnd)
		}
		for i, o := range []string{outSay, outLine, outEnd} {
			if strings.Contains(o, x39bMsgControl) {
				t.Errorf("第 %d 步不该打出冻结提示，实际: %q", i+1, o)
			}
		}
		if got := x39bMsgs(r, f); got != before+1 {
			t.Fatalf("草稿发送后应当恰好多一条，条数 %d -> %d", before, got)
		}
		if got := x39bLastText(t, r, f); got != "a\x1b[Ab" {
			t.Errorf("草稿正文 = %q，期望逐字节 %q", got, "a\x1b[Ab")
		}
	})
}

// TestX39bContrast_07 是验收 7 的新行为对照组（HEAD 上预期为红）。
func TestX39bContrast_07(t *testing.T) {
	t.Run("对照_不在草稿里被挡", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertBlocked(t, r, f, buf, "a\x1b[Ab")
	})
}

// ============================================================
// 验收 8：不改状态
// ============================================================

// 第 1 条那一行之后立刻喂 "你好"，照常记下；两种模式标记都没被置上，
// 下一次读取的提示符仍是常规提示符。
func TestX39b_08_NoStateLeftBehind(t *testing.T) {
	r, buf, f := newSeamREPL(t)
	curBefore := r.cur.ID
	x39bAssertBlocked(t, r, f, buf, "\x1b[A\x1b[A\x1b[A\x1b[B")
	if r.inDraft || r.inPaste {
		t.Errorf("被挡之后 REPL 不该留在任何模式里：inDraft=%v inPaste=%v", r.inDraft, r.inPaste)
	}
	if p := r.currentPrompt(); p != prompt {
		t.Errorf("被挡之后提示符应当仍是 %q，实际 %q", prompt, p)
	}
	if r.cur == nil || r.cur.ID != curBefore {
		t.Errorf("被挡之后当前会话不该变，原来 %q，现在 %+v", curBefore, r.cur)
	}
	x39bAssertRecorded(t, r, f, buf, "你好", "你好")
}

// ============================================================
// 验收 9：判的是剥过首尾空白之后的 text
// ============================================================

// 对照（TestX39bContrast_09）：同一个控制字符挪到行中间（剥不掉），同一行被挡；
// "\v/new 标题乙" 的对照是 "/new 标\v题乙"，不新建会话。
func TestX39b_09_JudgeTrimmedText(t *testing.T) {
	t.Run("行首换页符剥掉后照常记下", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertRecorded(t, r, f, buf, "\fhello", "hello")
	})
	t.Run("行尾垂直制表剥掉后照常记下", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertRecorded(t, r, f, buf, "hello\v", "hello")
	})
	t.Run("行首垂直制表剥掉后照常执行new", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		convsBefore := len(f.convs)
		out := x39bFeed(t, r, buf, "\v/new 标题乙")
		if strings.Contains(out, x39bMsgControl) {
			t.Errorf("不该打出冻结提示，实际输出: %q", out)
		}
		if got := len(f.convs); got != convsBefore+1 {
			t.Fatalf("应当新建恰好一个会话，会话总数 %d -> %d；输出: %q", convsBefore, got, out)
		}
		if got := f.convs[len(f.convs)-1].Title; got != "标题乙" {
			t.Errorf("新会话标题 = %q，期望 %q", got, "标题乙")
		}
	})
}

// TestX39bContrast_09 是验收 9 的新行为对照组（HEAD 上预期为红）。
func TestX39bContrast_09(t *testing.T) {
	t.Run("对照_行中垂直制表的new被挡", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		convsBefore := len(f.convs)
		out := x39bFeed(t, r, buf, "/new 标\v题乙")
		if !strings.Contains(out, x39bMsgControl+"\n") {
			t.Errorf("应当打出冻结提示，实际输出: %q", out)
		}
		if got := len(f.convs); got != convsBefore {
			t.Errorf("不该新建会话，会话总数 %d -> %d；输出: %q", convsBefore, got, out)
		}
	})
	t.Run("对照_行中换页符被挡", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertBlocked(t, r, f, buf, "he\fllo")
	})
	t.Run("对照_行中垂直制表被挡", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertBlocked(t, r, f, buf, "hel\vlo")
	})
}

// ============================================================
// 验收 10：粘贴外的孤立结束标记
// ============================================================

// 对照（TestX39bContrast_10）：把 201 改成 202（不再是结束标记），行里剩下 ESC，同一行被挡。
func TestX39b_10_StrayEndMarkerOutsidePaste(t *testing.T) {
	t.Run("孤立结束标记剥掉后照常记下", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		if r.inPaste {
			t.Fatalf("前提不成立：新 REPL 不该在粘贴中")
		}
		x39bAssertRecorded(t, r, f, buf, "\x1b[201~hello", "hello")
		if r.inPaste {
			t.Errorf("孤立的结束标记不该让 REPL 进入粘贴")
		}
	})
}

// TestX39bContrast_10 是验收 10 的新行为对照组（HEAD 上预期为红）。
func TestX39bContrast_10(t *testing.T) {
	t.Run("对照_不是标记的ESC序列被挡", func(t *testing.T) {
		r, buf, f := newSeamREPL(t)
		x39bAssertBlocked(t, r, f, buf, "\x1b[202~hello")
	})
}

// ============================================================
// 验收 11：定义的边界（表驱动）
// ============================================================

// 控制字符一律放在行中间，免得被首尾剥掉。码位写法按契约冻结：
// 小于 0x80 写成 "\xNN"，大于等于 0x80 写成 string(rune(0xNN))。
// 两半互为对照：0x1f/0x20、0x7e/0x7f（第 4 条）、0x00/0x01 各在边界两侧。
// U+00A0 在忠实实现与 M12 下都放行，它是边界对照，不是杀手。
func TestX39b_11_DefinitionBoundary(t *testing.T) {
	blocked := []struct {
		cp int
		c  string
	}{
		{0x00, "\x00"},
		{0x01, "\x01"},
		{0x08, "\x08"},
		{0x0b, "\x0b"},
		{0x0c, "\x0c"},
		{0x1f, "\x1f"},
	}
	allowed := []struct {
		cp int
		c  string
	}{
		{0x20, "\x20"},
		{0x7e, "\x7e"},
		{0x80, string(rune(0x80))},
		{0x85, string(rune(0x85))},
		{0x9b, string(rune(0x9b))},
		{0x9f, string(rune(0x9f))},
		{0xa0, string(rune(0xa0))},
	}
	for _, tc := range blocked {
		tc := tc
		t.Run(fmt.Sprintf("挡_U+%04X", tc.cp), func(t *testing.T) {
			r, buf, f := newSeamREPL(t)
			line := "a" + tc.c + "b"
			before := x39bMsgs(r, f)
			out := x39bFeed(t, r, buf, line)
			if !strings.Contains(out, x39bMsgControl+"\n") {
				t.Errorf("码位 U+%04X 应当被挡并打出冻结提示，实际输出: %q", tc.cp, out)
			}
			if got := x39bMsgs(r, f); got != before {
				t.Errorf("码位 U+%04X 不该记下，条数 %d -> %d", tc.cp, before, got)
			}
		})
	}
	for _, tc := range allowed {
		tc := tc
		t.Run(fmt.Sprintf("放_U+%04X", tc.cp), func(t *testing.T) {
			r, buf, f := newSeamREPL(t)
			line := "a" + tc.c + "b"
			before := x39bMsgs(r, f)
			out := x39bFeed(t, r, buf, line)
			if strings.Contains(out, x39bMsgControl) {
				t.Errorf("码位 U+%04X 应当放行，不该打出冻结提示，实际输出: %q", tc.cp, out)
			}
			if got := x39bMsgs(r, f); got != before+1 {
				t.Fatalf("码位 U+%04X 应当恰好记下一条，条数 %d -> %d；输出: %q", tc.cp, before, got, out)
			}
			if got := x39bLastText(t, r, f); got != line {
				t.Errorf("码位 U+%04X 记下的正文 = %q，期望逐字节 %q", tc.cp, got, line)
			}
		})
	}
}
