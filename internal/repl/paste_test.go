package repl

// 本文件覆盖 Step 10 B 片（括号粘贴识别，未决 #12 的便宜分支）：
// docs/13_STEP10_CONTRACT.md 5.2 节的两条验收，外加两条本文件自己另加的、
// 针对「粘贴与草稿共用一个布尔」「半段粘贴被静默永久化」这两类静默失败的用例。
//
// 纯函数 splitPasteMarkers 用表驱动钉死（package repl，不是 repl_test，
// 因为要直接调这个未导出函数）；端到端那层复用 conversation_test.go 里的
// newTestService / runREPLSvc，断言一律查假实现里落下的消息，不锁 REPL
// 屏幕输出的具体措辞（和本包其余测试的断言原则一致）。
//
// 只新建本文件，不改任何既有 _test.go；跑测试只跑 ./internal/repl/。

import (
	"context"
	"strings"
	"testing"
)

// ============================================================
// splitPasteMarkers：纯字符串处理，优先钉死
// ============================================================

// TestSplitPasteMarkers 表驱动覆盖 paste.go 顶部注释里写的六条规则。
// 每一行都额外断言剥完标记之后的正文里不留 CSI 字面（ESC[200~ / ESC[201~
// 两个序列本身），防的是"布尔答对了、但正文里混进了没剥干净的标记字节"
// 这种一半对一半错的静默失败——如果只断言布尔，这种坏法测不出来。
func TestSplitPasteMarkers(t *testing.T) {
	cases := []struct {
		name      string
		line      string
		wantText  string
		wantStart bool
		wantEnd   bool
	}{
		{
			// 防的是：把普通输入误判成粘贴，多出的分支把一句正常发言吞掉或改写。
			name:      "无标记，原样返回，两个布尔都是 false",
			line:      "普通输入不含任何标记",
			wantText:  "普通输入不含任何标记",
			wantStart: false,
			wantEnd:   false,
		},
		{
			// 防的是：只认整行匹配、不认"标记在行首、后面还跟着正文"这种真实终端会发的形状。
			name:      "只有 START，正文是标记之后的部分",
			line:      pasteStartSeq + "第一行",
			wantText:  "第一行",
			wantStart: true,
			wantEnd:   false,
		},
		{
			// 防的是：把 END 之前的正文连着标记一起丢掉。
			name:      "只有 END，正文是标记之前的部分",
			line:      "第三行" + pasteEndSeq,
			wantText:  "第三行",
			wantStart: false,
			wantEnd:   true,
		},
		{
			// 本机没实测过多行粘贴怎么分块，实现被要求对"标记分落两行"与
			// "两个标记同落一行"两种形状都成立。这一条钉的是后一种：
			// 如果实现只会认"一行只能有一个标记"，这里就会漏掉 end=true，
			// 是本文件里最关键的一条（对应任务清单第 4 条）。
			name:      "一行里同时有 START 与 END，两个布尔都是 true",
			line:      pasteStartSeq + "只有一行" + pasteEndSeq,
			wantText:  "只有一行",
			wantStart: true,
			wantEnd:   true,
		},
		{
			// 防的是：把标记外侧、用户手敲的字符当成标记的一部分一起吃掉。
			// 形状是"先打了字再粘贴"：标记落在行中间，两侧都要保留。
			name:      "标记落在行中间，外侧字符原样保留",
			line:      "已经打字" + pasteStartSeq + "又粘贴的内容",
			wantText:  "已经打字又粘贴的内容",
			wantStart: true,
			wantEnd:   false,
		},
		{
			// 防的是用 strings.Replace(s, old, new, 1) 只剥第一次出现——
			// 那样第二个 START 会原样剩在正文里，字面 ESC[200~ 混进
			// 库里存的正文，日后 grep/diff 全是噪声。ReplaceAll 才会
			// 把 "a" + START + "b" 剥成 "ab"；剥一次的话剩下的是
			// "a" + START + "b"，与期望值不相等，测试会红。
			name:      "同一标记重复出现，每一次都被剥掉",
			line:      pasteStartSeq + "a" + pasteStartSeq + "b",
			wantText:  "ab",
			wantStart: true,
			wantEnd:   false,
		},
		{
			name:      "空串，两个布尔都是 false",
			line:      "",
			wantText:  "",
			wantStart: false,
			wantEnd:   false,
		},
		{
			// 防的是：只有标记没有正文时（粘贴内容为空、或整行只是两个标记）
			// 把标记字面当正文剩下来。
			name:      "只有标记，没有正文",
			line:      pasteStartSeq + pasteEndSeq,
			wantText:  "",
			wantStart: true,
			wantEnd:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotText, gotStart, gotEnd := splitPasteMarkers(tc.line)
			if gotText != tc.wantText {
				t.Errorf("text = %q, 期望 %q", gotText, tc.wantText)
			}
			if gotStart != tc.wantStart {
				t.Errorf("start = %v, 期望 %v", gotStart, tc.wantStart)
			}
			if gotEnd != tc.wantEnd {
				t.Errorf("end = %v, 期望 %v", gotEnd, tc.wantEnd)
			}
			if strings.Contains(gotText, pasteStartSeq) || strings.Contains(gotText, pasteEndSeq) {
				t.Errorf("剥完之后的正文里还留着 CSI 字面: %q", gotText)
			}
		})
	}
}

// ============================================================
// 端到端：喂进 dispatch / Run，查假实现里真正落下的消息
// ============================================================
//
// 五个函数各自只钉一件事，方便 -run 单独跑、方便变异报告指到具体哪一条：
//   8  契约 13 5.2 节的原验收（两行粘贴只落一条消息，正文含换行）
//   9  契约 13 5.2 节的第二条验收（粘贴里的 /end 字面量不生效）
//   10 我方另加：/say 草稿中途粘一段，END 不许把草稿提前发出去
//   11 我方另加：粘到一半 EOF，库里不许多出消息，且要打出"已丢弃"
//   12 粘贴期间的斜杠命令（/quit）不被解释

// TestPasteEndToEnd_ContractAcceptance 钉住契约 13 5.2 节的原验收：
// 把 "ESC[200~第一行\n第二行\nESC[201~\n" 喂进去，库里只多一条 user 消息，
// 正文等于两行原文含换行（"第一行\n第二行"，不含结尾那个多余的换行——
// finishPaste 用 TrimFunc 把 END 那一行贡献的空正文与末尾换行一起削掉，
// 这里断言的是削干净之后的确切字节，不是用 Contains 弱断言）。
func TestPasteEndToEnd_ContractAcceptance(t *testing.T) {
	svc, f := newTestService(t)
	const convID = "conv001"

	input := "/new 粘贴验收\n" + pasteStartSeq + "第一行\n第二行\n" + pasteEndSeq + "\n"
	out, err := runREPLSvc(t, svc, input)
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
	}

	msgs, err := f.Messages(context.Background(), convID, 0, 0)
	if err != nil {
		t.Fatalf("读假实现里的消息失败: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("括号粘贴应当只多落一条消息，实际 %d 条；输出:\n%s", len(msgs), out)
	}

	const want = "第一行\n第二行"
	if got := msgs[0].Text; got != want {
		t.Errorf("粘贴正文 = %q，期望 %q（两行原文含换行，不含结尾多余换行）", got, want)
	}
}

// TestPasteEndToEnd_EndTokenLiteralInBody 钉住契约 13 5.2 节的第二条验收：
// 正文中间含一行 "/end" 字面量的粘贴，仍然只落一条消息——括号粘贴期间
// draftEndToken 不生效，这正是这条修法相对"让用户先敲 /say 走草稿模式"
// 多出来的正确性：草稿模式下一行孤立的 "/end" 会真的把消息截断，
// 括号粘贴模式下不会。
func TestPasteEndToEnd_EndTokenLiteralInBody(t *testing.T) {
	svc, f := newTestService(t)
	const convID = "conv001"

	input := "/new 粘贴含 end 字面量\n" +
		pasteStartSeq + "第一行\n" +
		"/end\n" +
		"第三行\n" +
		pasteEndSeq + "\n"
	out, err := runREPLSvc(t, svc, input)
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
	}

	msgs, err := f.Messages(context.Background(), convID, 0, 0)
	if err != nil {
		t.Fatalf("读假实现里的消息失败: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("正文里的 /end 字面量不该提前结束粘贴，应当仍只有一条消息，实际 %d 条；输出:\n%s", len(msgs), out)
	}

	const want = "第一行\n/end\n第三行"
	if got := msgs[0].Text; got != want {
		t.Errorf("粘贴正文 = %q，期望 %q（/end 原样留在正文里，没被当成命令解释）", got, want)
	}
}

// TestPasteEndToEnd_MidDraftDoesNotFlushDraft 防的是"inPaste 与 inDraft
// 共用一个布尔"这一类写法：那会把用户没写完的 /say 草稿，在粘贴的 END
// 到达时就提前变成一条永久消息，而 Canonical Conversation 只追加不回收，
// 覆水难收。
//
// 序列：/say 进草稿 -> 敲一行草稿正文 -> 中途粘一段（START...END）
//
//	-> 断言粘贴的 END 没有提前发出任何消息，粘贴内容原样并进草稿
//	-> 之后敲 /end 才发出一条消息，且正文同时含草稿行与粘贴行。
//
// 这条同时验证了 finishPaste 里 "if r.inDraft { r.draft = append(...) }"
// 这一支：粘贴内容走 strings.Split 按行拆开并进 draft，而不是走
// collectDraft（否则粘贴块里万一也含 /end /cancel 字面量会被当成草稿的
// 控制符，见 paste.go 该函数的注释）。
func TestPasteEndToEnd_MidDraftDoesNotFlushDraft(t *testing.T) {
	svc, f := newTestService(t)
	const convID = "conv001"

	input := "/new 草稿中途粘贴\n" +
		"/say\n" + // 进多行草稿
		"草稿行\n" + // 手敲一行草稿正文
		pasteStartSeq + "粘贴行\n" + // 中途粘一段……
		pasteEndSeq + "\n" + // ……END 到达，不许提前发送
		"/end\n" // 显式结束草稿，这才应该真正发出消息
	out, err := runREPLSvc(t, svc, input)
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
	}

	msgs, err := f.Messages(context.Background(), convID, 0, 0)
	if err != nil {
		t.Fatalf("读假实现里的消息失败: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("粘贴的 END 不许提前把草稿发出去，全程应当只有一条消息，实际 %d 条；输出:\n%s", len(msgs), out)
	}

	got := msgs[0].Text
	if !strings.Contains(got, "草稿行") || !strings.Contains(got, "粘贴行") {
		t.Fatalf("最终发出的消息应当同时含草稿行与粘贴行，实际正文: %q", got)
	}
	const want = "草稿行\n粘贴行"
	if got != want {
		t.Errorf("正文 = %q，期望 %q（粘贴内容按行并进草稿末尾）", got, want)
	}
}

// TestPasteEndToEnd_EOFDiscardsIncompletePaste 防的是"半段粘贴被静默
// 永久化"：粘贴只送到一半（见过 START，还没见到 END）就撞上输入流结束
// （管道读完、Ctrl+Z），此时用户从没确认过这份内容，Canonical Conversation
// 只追加不回收，写进去就再也拿不回来——所以必须丢弃，一条消息都不许多。
//
// 同时断言屏幕上真的打出了 msgPasteDiscarded：farewell 三条退出路径
// （/quit、EOF、ctx 取消）共用同一段收尾逻辑，这里钉住 EOF 这一条确实
// 触发了 farewell、确实检查了 r.inPaste，而不是只在 /quit 路径上生效。
func TestPasteEndToEnd_EOFDiscardsIncompletePaste(t *testing.T) {
	svc, f := newTestService(t)
	const convID = "conv001"

	// 故意不带 pasteEndSeq：整份输入在粘贴收集期间就结束了。
	input := "/new 半途而废的粘贴\n" + pasteStartSeq + "没写完的粘贴"
	out, err := runREPLSvc(t, svc, input)
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
	}

	msgs, err := f.Messages(context.Background(), convID, 0, 0)
	if err != nil {
		t.Fatalf("读假实现里的消息失败: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("没被 END 确认过的半段粘贴不许写成永久记录，实际多了 %d 条消息；输出:\n%s", len(msgs), out)
	}

	if !strings.Contains(out, msgPasteDiscarded) {
		t.Errorf("EOF 撞上未完成的粘贴时，farewell 应当打出 %q，实际输出:\n%s", msgPasteDiscarded, out)
	}
}

// TestPasteEndToEnd_SlashCommandNotInterpretedDuringPaste 钉住任务清单
// 第 12 条：粘贴块里的斜杠命令（这里用 /quit——后果最容易看出来：真被
// 解释的话进程会提前退出、后面的行根本不会被处理）不该被当成命令执行，
// 只是正文的一部分。
//
// 判据是消息数与正文：如果 /quit 被解释成命令，dispatch 会在处理完
// "/quit" 那一行时就返回 errQuit，Run 直接走 farewell 退出，
// 根本不会走到粘贴的 END、也就不会有任何消息落库——那与"一条消息，
// 正文含 /quit 字面量"是两种可以清楚分辨的结果。
func TestPasteEndToEnd_SlashCommandNotInterpretedDuringPaste(t *testing.T) {
	svc, f := newTestService(t)
	const convID = "conv001"

	input := "/new 粘贴里混进斜杠命令\n" +
		pasteStartSeq + "/quit\n" +
		"第二行\n" +
		pasteEndSeq + "\n"
	out, err := runREPLSvc(t, svc, input)
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际: %v；输出:\n%s", err, out)
	}

	msgs, err := f.Messages(context.Background(), convID, 0, 0)
	if err != nil {
		t.Fatalf("读假实现里的消息失败: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("粘贴块里的 /quit 不该被解释成命令，应当仍然只有一条消息，实际 %d 条（若 /quit 被解释成命令，这里会是 0 条）；输出:\n%s", len(msgs), out)
	}

	const want = "/quit\n第二行"
	if got := msgs[0].Text; got != want {
		t.Errorf("正文 = %q，期望 %q（/quit 原样留在正文里）", got, want)
	}
	if !strings.Contains(out, msgFarewell) {
		t.Errorf("粘贴处理完之后输入流结束，应当仍然走到正常的告别行；输出:\n%s", out)
	}
}
