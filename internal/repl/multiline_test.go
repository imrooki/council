package repl

import (
	"context"
	"strings"
	"testing"
)

// bom / zwsp 用码位构造，源码里不出现字面的不可见字符。
// 理由同 commands.go 里那组 rune 常量：带反斜杠 u 的转义写法会在某些编辑与传输路径上
// 被还原成真字符，而 Go 不允许文件中间出现 BOM，落进源码就编译不过。
var (
	bom  = string(rune(0xFEFF))
	zwsp = string(rune(0x200B))
)

// TestMultilineDraftPostsOneMessage 锁住多行输入模式要解决的那件事：
// 一次粘贴成为**一条**消息，不是 N 条。
//
// 这是本功能存在的全部理由。读循环按行切，而 Canonical Conversation 只追加不回收，
// 所以粘贴一段三行的需求本来会留下三条各自独立、永远删不掉的记录。
func TestMultilineDraftPostsOneMessage(t *testing.T) {
	svc, f := newTestService(t)
	f.seedConversation("conv_ml", "多行用例")

	if _, err := runREPLSvc(t, svc,
		"/use conv_ml\n/say\n请审查下面这段需求:\n1. 第一条\n2. 第二条\n/end\n"); err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v", err)
	}

	msgs := f.msgs["conv_ml"]
	if len(msgs) != 1 {
		t.Fatalf("会话里有 %d 条消息，期望 1 条（多行应当合成一条）", len(msgs))
	}
	const want = "请审查下面这段需求:\n1. 第一条\n2. 第二条"
	if msgs[0].Text != want {
		t.Errorf("正文是 %q，期望 %q", msgs[0].Text, want)
	}
}

// TestMultilineDraftKeepsLinesVerbatim 锁住草稿里的行一律原样保存：
// 缩进不被吃掉，以 / 开头的行是正文而不是命令。
//
// 后半条尤其要紧：多行模式的意义就是「粘贴进来的东西不再被解释一遍」，
// 若草稿里的 /join 仍被当命令执行，这个模式就等于没有。
func TestMultilineDraftKeepsLinesVerbatim(t *testing.T) {
	svc, f := newTestService(t)
	f.seedConversation("conv_verb", "原样用例")

	if _, err := runREPLSvc(t, svc,
		"/use conv_verb\n/say\n/join ag_alpha\n    缩进的一行\n/help\n/end\n"); err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v", err)
	}

	msgs := f.msgs["conv_verb"]
	if len(msgs) != 1 {
		t.Fatalf("会话里有 %d 条消息，期望 1 条", len(msgs))
	}
	const want = "/join ag_alpha\n    缩进的一行\n/help"
	if msgs[0].Text != want {
		t.Errorf("正文是 %q，期望 %q", msgs[0].Text, want)
	}
	// /join 若被当成命令执行过，这里就会多出一个成员。
	if ps, err := svc.Participants(context.Background(), "conv_verb", true); err != nil {
		t.Fatalf("列成员失败: %v", err)
	} else if len(ps) != 0 {
		t.Errorf("草稿里的 /join 被当成命令执行了：会话里有 %d 个成员", len(ps))
	}
}

// TestMultilineDraftNotSent 锁住三条「不发送」的路径。
// 草稿没被显式发送就一条都不该落库 —— 事实源里多一条假记录，比丢一份草稿糟得多。
func TestMultilineDraftNotSent(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"主动放弃", "/use conv_x\n/say\n这段不要\n/cancel\n"},
		{"输入流结束时还攒着草稿", "/use conv_x\n/say\n这段没来得及发\n"},
		{"空草稿", "/use conv_x\n/say\n/end\n"},
		{"只有空白的草稿", "/use conv_x\n/say\n   \n\t\n/end\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, f := newTestService(t)
			f.seedConversation("conv_x", "不发送用例")

			if _, err := runREPLSvc(t, svc, tc.input); err != nil {
				t.Fatalf("Run 应当返回 nil，实际: %v", err)
			}
			if n := len(f.msgs["conv_x"]); n != 0 {
				t.Errorf("不该发送却落了 %d 条消息", n)
			}
		})
	}
}

// TestMultilineDraftNeedsConversation 锁住进入多行模式与 /say 走同一套前置条件：
// 没选会话时 /say 被拦下，不该把用户留在一个发不出去的模式里。
func TestMultilineDraftNeedsConversation(t *testing.T) {
	svc, f := newTestService(t)
	f.seedConversation("conv_unused", "没被选中")

	// 没有 /use。若 /say 仍然进了多行模式，后面那两行会被当草稿吞掉；
	// 若被前置条件拦下，它们就是两条普通的裸文本发言 —— 而那同样会被前置条件拦下。
	if _, err := runREPLSvc(t, svc, "/say\n第一行\n/end\n"); err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v", err)
	}
	if n := len(f.msgs["conv_unused"]); n != 0 {
		t.Errorf("没选会话却往库里写了 %d 条消息", n)
	}
}

// TestBareTextLinesStillPostSeparately 把「已知限制」钉成用例：
// 不进多行模式时，三行输入仍然是三条消息。
//
// 存在的理由是防止误解：多行模式没有把裸文本也改成缓冲，
// 直接粘贴（不先敲 /say）依旧会切成多条。这条限制写在 docs/08_STEP5_SPEC.md H 节。
func TestBareTextLinesStillPostSeparately(t *testing.T) {
	svc, f := newTestService(t)
	f.seedConversation("conv_sep", "已知限制用例")

	if _, err := runREPLSvc(t, svc, "/use conv_sep\n第一行\n第二行\n第三行\n"); err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v", err)
	}
	if n := len(f.msgs["conv_sep"]); n != 3 {
		t.Errorf("落了 %d 条消息，期望 3 条（这是已知限制，不是 bug）", n)
	}
}

// TestInvisiblePrefixStillDispatchesCommand 锁住不可见前导字符不再吃掉斜杠命令。
//
// TrimSpace 认不出 U+FEFF 与零宽字符，于是带 BOM 的 /help 会落进裸文本分支
// 变成一条不可回收的发言，而屏幕上打的是「已记下 seq N」这种正常成功行。
// 本机 PowerShell 5.1 用管道喂 stdin 时就会在流首插 BOM，是已实证的高发场景。
//
// 断言方式是「带前缀与不带前缀的输出逐字节相同」，不碰任何提示的措辞（08a 的要求）。
func TestInvisiblePrefixStillDispatchesCommand(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prefix string
	}{
		{"BOM", bom},
		{"零宽空格", zwsp},
		{"BOM 后面还跟着空格", bom + "  "},
		{"空格在前 BOM 在后", "  " + bom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, f := newTestService(t)
			f.seedConversation("conv_inv", "不可见前缀用例")

			dirty, err := runREPLSvc(t, svc, "/use conv_inv\n"+tc.prefix+"/help\n")
			if err != nil {
				t.Fatalf("Run 应当返回 nil，实际: %v", err)
			}
			if n := len(f.msgs["conv_inv"]); n != 0 {
				t.Fatalf("带前缀的命令被当成发言写了 %d 条消息", n)
			}

			svc2, f2 := newTestService(t)
			f2.seedConversation("conv_inv", "不可见前缀用例")
			clean, err := runREPLSvc(t, svc2, "/use conv_inv\n/help\n")
			if err != nil {
				t.Fatalf("对照组 Run 应当返回 nil，实际: %v", err)
			}
			if dirty != clean {
				t.Errorf("带前缀与不带前缀的输出不一致。\n带前缀:\n%s\n不带:\n%s", dirty, clean)
			}
		})
	}
}

// TestInvalidUTF8IsRejected 锁住非法 UTF-8 不入库。
//
// SQLite 的 TEXT 不校验编码，而 Step 6 把历史拼成 JSON 送给 provider 时
// json.Marshal 会把坏字节换成 U+FFFD 且不报错 —— 于是库里存的、屏幕上显示的、
// agent 实际看到的三者永久不一致。宁可让用户重敲一行。
func TestInvalidUTF8IsRejected(t *testing.T) {
	// GBK 的「你好」，在 UTF-8 下不是合法序列。
	bad := string([]byte{0xC4, 0xE3, 0xBA, 0xC3})

	t.Run("单行模式", func(t *testing.T) {
		svc, f := newTestService(t)
		f.seedConversation("conv_enc", "编码用例")

		if _, err := runREPLSvc(t, svc, "/use conv_enc\n"+bad+"\n"); err != nil {
			t.Fatalf("Run 应当返回 nil，实际: %v", err)
		}
		if n := len(f.msgs["conv_enc"]); n != 0 {
			t.Errorf("非法 UTF-8 落库了 %d 条", n)
		}
	})

	t.Run("多行模式整份草稿作废", func(t *testing.T) {
		svc, f := newTestService(t)
		f.seedConversation("conv_enc", "编码用例")

		// 坏字节夹在中间。只丢这一行会把粘贴悄悄截去一截，用户看不出少了什么，
		// 所以整份草稿必须作废 —— 后面的 /end 落在单行模式里，是一条未知命令。
		if _, err := runREPLSvc(t, svc,
			"/use conv_enc\n/say\n好的第一行\n"+bad+"\n好的第三行\n/end\n"); err != nil {
			t.Fatalf("Run 应当返回 nil，实际: %v", err)
		}
		if n := len(f.msgs["conv_enc"]); n != 0 {
			t.Errorf("坏草稿落库了 %d 条", n)
		}
	})
}

// TestOneLineNeutralizesTabwriterSeparators 锁住 oneLine 把 tabwriter 的五种分隔符
// 全部中和掉。漏掉任意一种，/log 的表格都会被一条消息撑散。
//
// \v 与 \f 是本轮补上的：text/tabwriter 的 case 是 '\t', '\v', '\n', '\f'，
// 四个行为等价，而 oneLine 原先只挡了其中两个加 \r。
func TestOneLineNeutralizesTabwriterSeparators(t *testing.T) {
	for _, sep := range []string{"\t", "\n", "\r", "\r\n", "\v", "\f"} {
		got := oneLine("前" + sep + "后")
		if got != "前 后" {
			t.Errorf("分隔符 %q 没被中和：oneLine 得到 %q，期望 %q", sep, got, "前 后")
		}
		if strings.ContainsAny(got, "\t\n\r\v\f") {
			t.Errorf("分隔符 %q 处理后仍残留控制字符: %q", sep, got)
		}
	}
}
