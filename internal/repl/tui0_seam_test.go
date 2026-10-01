package repl

// 本文件覆盖冻结契约「TUI 第 0 刀：冻缝」
// （E:\multi_model_cowork\tui0_seam_contract.md）第 5 节的 12 条验收。
//
// 背对背纪律：本文件由测试腿写，与实现腿同时进行，互相看不见。断言只依据
// 契约文本写，不依据对 seam.go / console_windows.go / console_other.go
// 新增内容的任何观察——这三个文件本轮没有读过，一旦读到会在报告里如实
// 记下时间点与内容，不会继续读或据此改动。
//
// 只新建本文件，不改任何既有 _test.go；本包已有 TestMain（见
// session_inflight_test.go），这里不重复定义。跑测试只跑 ./internal/repl/。
//
// 实现腿落地之前，Submit、IsTerminal、msgDraftNotInTUI 三个符号都还不存在，
// 本文件预期编译不过——这是预期状态。

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
)

// newSeamREPL 造一个已经建好并选中一个新会话的 REPL，专供本文件直接调用
// Submit / dispatch —— 两者都不读 r.in，助手传 nil。svc / fakeStore 复用
// conversation_test.go 的 newTestService / newTestREPLConfig，不重新发明。
func newSeamREPL(t *testing.T) (*REPL, *bytes.Buffer, *fakeStore) {
	t.Helper()
	svc, f := newTestService(t)
	var buf bytes.Buffer
	r := New(newTestREPLConfig(), svc, nil, nil, &buf, RulesContext{})
	conv, err := svc.Create(context.Background(), "seam 用例")
	if err != nil {
		t.Fatalf("建会话失败: %v", err)
	}
	r.cur = &conv
	return r, &buf, f
}

// assertWantOrWantNewline 断言 got 恰好等于 want，或者恰好等于 want 后面
// 补一个换行。契约对这两处只写了"打某常量"，没有像验收 3 的四行 dispatch
// 复用那样明说带不带换行；仓库现有代码清一色用 r.printf("%s\n", ...)，
// 但这条不是契约文本本身锁死的，所以断言容忍这一个字节的差异，同时仍然
// 要求内容精确相等——不是子串判定，不会把"输出里恰好含有这几个字，但混着
// 别的东西"错判成通过。
func assertWantOrWantNewline(t *testing.T, got, want string) {
	t.Helper()
	if got != want && got != want+"\n" {
		t.Errorf("输出是 %q，期望恰好是 %q 或 %q（是否补一个尾随换行，契约没有明说）", got, want, want+"\n")
	}
}

// ============================================================
// 验收 1: Submit(ctx, "hello") 与 dispatch(ctx, "hello") 产生相同输出
// ============================================================
//
// 判别力说明：dispatch 与 Submit 各自调用都会落一条消息，第二次的 seq 会变；
// 若在同一个会话上先后调用两者，输出里的 seq 数字必然不同，逐字节比较必
// 然失败——那不是在测两条路径的行为差异，只是在测"巧合地对不上"，是一条
// 废用例。
//
// 处理办法：仍然用同一个 *REPL（同一份 cmds 表、同一份 svc），但让两次
// 调用各自落在一个全新创建的空会话上。fakeStore 按会话独立记 seq
// （AppendMessage: m.Seq = len(f.msgs[m.ConvID]) + 1），两个全新会话各自
// 的第一条消息都是 seq 1；这样"落库效果分处两个会话、但屏幕输出相同"就不
// 再是巧合，而是因为两条路径的处理逻辑本身相同。若 Submit 在裁剪、命令表
// 查找、或 postUserText 调用方式上与 dispatch 有任何不同，这条断言会抓到。
func TestSubmitMatchesDispatchForBareText(t *testing.T) {
	r, buf, _ := newSeamREPL(t)
	ctx := context.Background()

	buf.Reset()
	if err := r.dispatch(ctx, "hello"); err != nil {
		t.Fatalf("dispatch 返回了非 nil 错误: %v", err)
	}
	outDispatch := buf.String()

	convB, err := r.svc.Create(ctx, "第二个会话")
	if err != nil {
		t.Fatalf("建第二个会话失败: %v", err)
	}
	r.cur = &convB
	buf.Reset()
	if err := r.Submit(ctx, "hello"); err != nil {
		t.Fatalf("Submit 返回了非 nil 错误: %v", err)
	}
	outSubmit := buf.String()

	// 这条只是测试基础设施的自检（证明 dispatch 确实产生了输出，不是环境
	// 搭错了导致两边都是空字符串、白白通过），不是对契约行为本身的断言。
	if outDispatch == "" {
		t.Fatalf("dispatch 的输出是空的，这条用例的基础设施搭建有问题，测不出任何东西")
	}
	if outDispatch != outSubmit {
		t.Errorf("dispatch 与 Submit 对同样的裸文本输出不同。\ndispatch: %q\nSubmit:   %q", outDispatch, outSubmit)
	}
}

// ============================================================
// 验收 2: Submit(ctx, "/help") 与 dispatch(ctx, "/help") 产生相同输出
// ============================================================
//
// /help 走 cmdHelp，只遍历命令表打印，不碰会话、不落库，没有随调用次数
// 变化的状态，所以可以直接在同一个 REPL、同一个会话上先后调用两次、比较
// 输出是否逐字节相等——不需要像验收 1 那样借两个空会话把 seq 抵消掉。
func TestSubmitMatchesDispatchForHelp(t *testing.T) {
	r, buf, _ := newSeamREPL(t)
	ctx := context.Background()

	buf.Reset()
	if err := r.dispatch(ctx, "/help"); err != nil {
		t.Fatalf("dispatch 返回了非 nil 错误: %v", err)
	}
	outDispatch := buf.String()

	buf.Reset()
	if err := r.Submit(ctx, "/help"); err != nil {
		t.Fatalf("Submit 返回了非 nil 错误: %v", err)
	}
	outSubmit := buf.String()

	if outDispatch == "" {
		t.Fatalf("dispatch 的输出是空的，这条用例的基础设施搭建有问题，测不出任何东西")
	}
	if outDispatch != outSubmit {
		t.Errorf("dispatch 与 Submit 对 /help 输出不同。\ndispatch: %q\nSubmit:   %q", outDispatch, outSubmit)
	}
}

// ============================================================
// 验收 3: Submit(ctx, "/nosuchcmd") 打 msgUnknownCommand
// ============================================================
//
// 契约 4.1 第 5 条明写：以 / 开头的输入走"与 dispatch 逐字相同的四行"，
// dispatch 对查不到的命令名恰好是 r.printf("%s\n", msgUnknownCommand)
// 这一句（commands.go 已读过的既有代码），所以这里按逐字相等断言，
// 不是宽松的 Contains——如果 Submit 打印时漏了换行或多打了别的东西，
// 这条要能抓到。
func TestSubmitUnknownCommand(t *testing.T) {
	r, buf, _ := newSeamREPL(t)
	ctx := context.Background()

	buf.Reset()
	if err := r.Submit(ctx, "/nosuchcmd"); err != nil {
		t.Fatalf("Submit 返回了非 nil 错误: %v", err)
	}
	want := msgUnknownCommand + "\n"
	if got := buf.String(); got != want {
		t.Errorf("Submit(\"/nosuchcmd\") 的输出是 %q，期望恰好是 %q", got, want)
	}
}

// ============================================================
// 验收 4: Submit(ctx, "") 与 Submit(ctx, "   ") 都返回 nil 且零字节输出
// ============================================================
func TestSubmitBlankInputsAreNoop(t *testing.T) {
	r, buf, f := newSeamREPL(t)
	ctx := context.Background()
	convID := r.cur.ID

	for _, in := range []string{"", "   "} {
		buf.Reset()
		if err := r.Submit(ctx, in); err != nil {
			t.Errorf("Submit(%q) 返回了非 nil 错误: %v", in, err)
		}
		if buf.Len() != 0 {
			t.Errorf("Submit(%q) 输出了 %d 字节，期望 0 字节；输出: %q", in, buf.Len(), buf.String())
		}
	}
	if n := len(f.msgs[convID]); n != 0 {
		t.Errorf("空白输入不该落库，实际落了 %d 条", n)
	}
}

// ============================================================
// 验收 5: Submit(ctx, "a\nb\nc") 只产生一条消息，正文逐字节等于 "a\nb\nc"
// ============================================================
//
// 对照（行式路径喂三行会产生三条消息）已经由 multiline_test.go /
// bare_text_test.go 的既有用例（TestBareTextLinesStillPostSeparately 等）
// 覆盖，本文件不重复搭建；这里只钉住 Submit 自己这一侧的行为。
func TestSubmitMultilineBodyIsOneMessage(t *testing.T) {
	r, _, f := newSeamREPL(t)
	ctx := context.Background()
	convID := r.cur.ID

	const body = "a\nb\nc"
	if err := r.Submit(ctx, body); err != nil {
		t.Fatalf("Submit 返回了非 nil 错误: %v", err)
	}

	msgs := f.msgs[convID]
	if len(msgs) != 1 {
		t.Fatalf("应当恰好落 1 条消息，实际 %d 条", len(msgs))
	}
	if msgs[0].Text != body {
		t.Errorf("正文是 %q，期望逐字节等于 %q", msgs[0].Text, body)
	}
}

// ============================================================
// 验收 6: Submit(ctx, "/say x\ny") 产生一条消息，正文逐字节等于 "x\ny"
// ============================================================
func TestSubmitSayWithBodyPostsOneMessage(t *testing.T) {
	r, _, f := newSeamREPL(t)
	ctx := context.Background()
	convID := r.cur.ID

	if err := r.Submit(ctx, "/say x\ny"); err != nil {
		t.Fatalf("Submit 返回了非 nil 错误: %v", err)
	}

	msgs := f.msgs[convID]
	if len(msgs) != 1 {
		t.Fatalf("应当恰好落 1 条消息，实际 %d 条", len(msgs))
	}
	const want = "x\ny"
	if msgs[0].Text != want {
		t.Errorf("正文是 %q，期望逐字节等于 %q", msgs[0].Text, want)
	}
}

// ============================================================
// 验收 7: Submit(ctx, "/say") 打 msgDraftNotInTUI，调用后 r.inDraft 仍为 false
// ============================================================
func TestSubmitBareSayDoesNotEnterDraftMode(t *testing.T) {
	r, buf, f := newSeamREPL(t)
	ctx := context.Background()
	convID := r.cur.ID

	if r.inDraft {
		t.Fatalf("测试前提错误: r.inDraft 在调用前就已经是 true")
	}

	buf.Reset()
	if err := r.Submit(ctx, "/say"); err != nil {
		t.Fatalf("Submit 返回了非 nil 错误: %v", err)
	}

	assertWantOrWantNewline(t, buf.String(), msgDraftNotInTUI)

	if r.inDraft {
		t.Errorf("Submit(\"/say\") 之后 r.inDraft 变成了 true，TUI 下不该进多行模式")
	}
	if n := len(f.msgs[convID]); n != 0 {
		t.Errorf("Submit(\"/say\") 不该落库，实际落了 %d 条", n)
	}
}

// ============================================================
// 验收 8: Submit(ctx, "/quit") 返回的 error 满足 errors.Is(err, errQuit)
// ============================================================
func TestSubmitQuitReturnsErrQuit(t *testing.T) {
	r, _, _ := newSeamREPL(t)
	ctx := context.Background()

	err := r.Submit(ctx, "/quit")
	if !errors.Is(err, errQuit) {
		t.Errorf("Submit(\"/quit\") 返回的错误是 %v，应当满足 errors.Is(err, errQuit)", err)
	}
}

// ============================================================
// 验收 9: 非法 UTF-8 打 msgNotUTF8，且不产生任何消息
// ============================================================
//
// 用契约 5 节验收 9 原文给的字面量 "\xff\xfe"，不用别的编码构造的坏字节：
// 更贴合契约文本本身，且 0xFF、0xFE 在 UTF-8 里永远不是合法的首字节，
// 不依赖任何特定源编码的巧合。
func TestSubmitInvalidUTF8IsRejected(t *testing.T) {
	r, buf, f := newSeamREPL(t)
	ctx := context.Background()
	convID := r.cur.ID

	const bad = "\xff\xfe"

	buf.Reset()
	if err := r.Submit(ctx, bad); err != nil {
		t.Fatalf("Submit 返回了非 nil 错误: %v", err)
	}
	assertWantOrWantNewline(t, buf.String(), msgNotUTF8)

	if n := len(f.msgs[convID]); n != 0 {
		t.Errorf("非法 UTF-8 不该落库，实际落了 %d 条", n)
	}
}

// ============================================================
// 验收 10: 一次 Submit 前后，r.inPaste / r.inDraft / r.draftPoisoned 三个
// 字段都不变；带括号粘贴标记的输入被当成普通正文，不进粘贴层
// ============================================================

// TestSubmitDoesNotTouchPasteOrDraftLayers 覆盖验收 10 的前半句。
//
// 只比较三个布尔字段"调用前后是否相等"不够：如果实现误判 r.inDraft 为真
// 就把这一行错误地转给 collectDraft（该函数对一条普通行只会把它追加进
// r.draft、返回 nil，三个布尔字段确实一个都不碰），或者误判 r.inPaste 为
// 真转给 handlePaste（同样只追加 r.paste、不碰三个布尔字段），上面的字段
// 比较会全部通过，完全抓不到"被错误路由"这件事。
//
// 所以本用例两件事都做：(1) 起始状态分别取"全假"（TUI 最常见的初始状态）
// 与"全真"（人为构造，用来证明字段确实没被 Submit 读，而不只是碰巧本来
// 就是假）两组，调用后比对三个字段与起始值逐一相等；(2) 额外核对确实落了
// 一条消息、且正文就是喂进去的原文——这是"没有被路由去 collectDraft /
// handlePaste"的直接证据，字段不变只是"没有被写"的证据，两者合起来才
// 覆盖住"不读也不写"这句话的全部含义。
func TestSubmitDoesNotTouchPasteOrDraftLayers(t *testing.T) {
	cases := []struct {
		name          string
		inPaste       bool
		inDraft       bool
		draftPoisoned bool
	}{
		{"三个字段起始都是假（TUI 最常见的初始状态）", false, false, false},
		{"三个字段起始都是真（人为构造，验证 Submit 真的不碰，而不是巧合地仍是假）", true, true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, _, f := newSeamREPL(t)
			ctx := context.Background()
			convID := r.cur.ID

			r.inPaste = tc.inPaste
			r.inDraft = tc.inDraft
			r.draftPoisoned = tc.draftPoisoned

			const line = "普通一行"
			if err := r.Submit(ctx, line); err != nil {
				t.Fatalf("Submit 返回了非 nil 错误: %v", err)
			}

			if r.inPaste != tc.inPaste {
				t.Errorf("r.inPaste 从 %v 变成了 %v", tc.inPaste, r.inPaste)
			}
			if r.inDraft != tc.inDraft {
				t.Errorf("r.inDraft 从 %v 变成了 %v", tc.inDraft, r.inDraft)
			}
			if r.draftPoisoned != tc.draftPoisoned {
				t.Errorf("r.draftPoisoned 从 %v 变成了 %v", tc.draftPoisoned, r.draftPoisoned)
			}

			msgs := f.msgs[convID]
			if len(msgs) != 1 {
				t.Fatalf("Submit 应当把这一行当普通正文直接发出去，落 1 条消息，实际 %d 条"+
					"（如果实现按 inDraft/inPaste 分支路由到了 collectDraft/handlePaste，这里会是 0 条）",
					len(msgs))
			}
			if msgs[0].Text != line {
				t.Errorf("落库正文是 %q，期望 %q", msgs[0].Text, line)
			}
		})
	}
}

// TestSubmitTreatsBracketedPasteMarkersAsLiteralText 覆盖验收 10 的后半句：
// 带括号粘贴标记的输入在 Submit 下应当被当成普通正文处理，不进粘贴层，
// 且不调用 splitPasteMarkers。
//
// 判别力来自落库正文：如果 Submit 偷偷调用了 splitPasteMarkers（paste.go
// 里的既有函数，剥掉 ESC[200~ / ESC[201~ 两个标记），落库正文会变成剥完
// 之后的 "abc"，而不是带标记的原文 "ESC[200~abcESC[201~"——这里用逐字节
// 比较原文来抓这个坑，不是只看 r.inPaste 有没有变。
func TestSubmitTreatsBracketedPasteMarkersAsLiteralText(t *testing.T) {
	r, _, f := newSeamREPL(t)
	ctx := context.Background()
	convID := r.cur.ID

	// pasteStartSeq / pasteEndSeq 是 paste.go 里已有的包级常量
	// （"\x1b[200~" / "\x1b[201~"），与契约验收 10 给出的字面量
	// "\x1b[200~abc\x1b[201~" 是同一组字节，直接复用而不是另抄一遍。
	const in = pasteStartSeq + "abc" + pasteEndSeq
	if err := r.Submit(ctx, in); err != nil {
		t.Fatalf("Submit 返回了非 nil 错误: %v", err)
	}
	if r.inPaste {
		t.Errorf("Submit 之后 r.inPaste 变成了 true，说明这次调用进了粘贴层")
	}

	msgs := f.msgs[convID]
	if len(msgs) != 1 {
		t.Fatalf("应当恰好落 1 条消息，实际 %d 条", len(msgs))
	}
	if msgs[0].Text != in {
		t.Errorf("落库正文是 %q，期望原样保留标记的 %q（说明 Submit 没有调用 splitPasteMarkers）",
			msgs[0].Text, in)
	}
}

// ============================================================
// 验收 11: IsTerminal(nil) 返回 false
// ============================================================
//
// 不是结构上恒真的用例：一个没有先判 f == nil 就去调 f.Fd() 的实现会在
// 这里对 nil *os.File 解引用 panic，而不是老老实实返回 false——那同样是
// 一次可观测的失败，只是失败的形式是进程崩溃而不是断言不等。
func TestIsTerminalNilReturnsFalse(t *testing.T) {
	if IsTerminal(nil) {
		t.Errorf("IsTerminal(nil) 返回了 true，期望 false")
	}
}

// ============================================================
// 验收 12: IsTerminal 对一个普通文件（os.CreateTemp 出来的）返回 false
// ============================================================
func TestIsTerminalPlainFileReturnsFalse(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "council-seam-*.txt")
	if err != nil {
		t.Fatalf("建临时文件失败: %v", err)
	}
	t.Cleanup(func() {
		if cerr := f.Close(); cerr != nil {
			t.Logf("关闭临时文件失败（不影响用例结果）: %v", cerr)
		}
	})

	if IsTerminal(f) {
		t.Errorf("IsTerminal(普通文件) 返回了 true，期望 false")
	}
}
