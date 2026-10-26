package repl

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

// msgDraftNotInTUI 是 TUI 下敲光秃秃 /say 的答复。
const msgDraftNotInTUI = "TUI 下直接在输入框里换行即可，不需要多行模式"

// IsQuit 报告这个 error 是不是 Submit 在用户敲 /quit 时返回的哨兵。
//
// 不导出哨兵本身（也不写 var ErrQuit = errQuit）：两个名字指同一个值时，
// 下一个人看到未导出的那个会以为它是内部的、可以随便改，一改导出的那个
// 跟着变，而包外的 errors.Is 会在没有任何编译错误的情况下开始返回 false。
// TUI 只需要问「是不是要退」，不需要拿到哨兵。
//
// 用 errors.Is 而不是 ==：将来若有人把它包一层（fmt.Errorf 带 %w），
// 这里仍认得出来。
func IsQuit(err error) bool {
	return errors.Is(err, errQuit)
}

// Submit 是 TUI 前端的唯一入口：吃一段完整的提交文本（可含换行），
// 走与行式路径同一套命令表与同一套前置条件。
//
// 它与 dispatch 共用 r.cmds 与 sayCmd，发言与斜杠命令都经 runCommand 出去。
// needsSvc、needsConv、needsRunner 只在 runCommand 里判一次，所以「持久化没起来」
// 「还没选会话」「provider 调用未就绪」这些答复在两条路上是同一句，不会各写一份。
//
// 它不读、不写 inPaste、inDraft、draft、draftPoisoned，也不调用 splitPasteMarkers。
// 这两层属于行式路径：读循环按行切，一次三行的粘贴会变成三条独立的、不可回收的
// 永久消息，多行草稿（/say ... /end）是把它们拼回一条的办法。X-39b 之后行式路径
// 不再请求括号粘贴，粘贴状态机只剥终端自己发来的标记。TUI 的输入框本身就是
// 多行编辑器，一次提交已经是完整的一段。
// 带括号粘贴标记的提交在这里就是普通正文。
//
// dispatch 在斜杠判断之前有一道控制字符闸（X-39b），这里**不接**：
// 那道闸把 0x0a 算作控制字符，TUI 的多行提交正文里本来就有换行，
// 接过来会挡掉每一条多行消息。以后若把 Submit 改成调 dispatch，这一点要单独处理。
//
// 由此带来一处与 dispatch 的**有意分叉**，不是疏漏：dispatch 遇到非法 UTF-8 时，
// 若当时在粘贴层里或这一行带起标记，走的是 handlePasteInvalid，打的是
// msgPasteBadEncoding 并改 inPaste / pastePoisoned。Submit 没有粘贴层，
// 一律打 msgNotUTF8 并返回 nil。两条路对「坏字节绝不落库」这件事是一致的，
// 分叉只在措辞与状态机上，而那个状态机在 TUI 下不存在。
//
// /say 不带参数在两条路上行为不同。行式路径需要多行模式，是因为读循环按行切，
// 一次三行的粘贴会变成三条独立的、不可回收的永久消息；/say 空参因此进入 inDraft，
// 靠 /end 与 /cancel 两个行退出。TUI 的输入框天然多行，这个问题不存在。
// 若照搬，用户会掉进一个没有退出提示的模式里：TUI 下 /end 与 /cancel 这两行会被
// 当成普通正文提交。所以这里命令名是 say 且 restOf 为空时，只打 msgDraftNotInTUI
// 并返回 nil，不调 runCommand，不置 inDraft。
func (r *REPL) Submit(ctx context.Context, text string) error {
	// 非法 UTF-8 一旦落进 messages.text 就是永久的：SQLite 的 TEXT 不校验编码，
	// 而 json.Marshal 会把坏字节换成 U+FFFD 且不报错，于是库里存的、屏幕上显示的、
	// agent 实际看到的三者永久不一致。闸放在最前面，坏字节不进任何后续分支。
	if !utf8.ValidString(text) {
		return r.printf("%s\n", r.t("tui.repl.notUTF8", msgNotUTF8))
	}

	text = strings.TrimFunc(text, isTrimmable)
	if text == "" {
		return nil
	}
	if !strings.HasPrefix(text, "/") {
		// 交给 sayCmd 这个表项，而不是直接调 cmdSay：这样连前置条件一起复用，
		// 「持久化没起来」「还没选会话」两种情况下，裸文本与 /say 的答复完全一致。
		// rest 传整段原文；fields 按空白切分，与斜杠命令那条路的构造方式保持一致
		// （cmdSay 只读 rest，fields 在这里用不上，但两条路的 cmdArgs 形状应当一样）。
		// bare 置上：这是裸文本，自动回复打开时落库之后问一轮（#47）。
		return r.runCommand(ctx, sayCmd, cmdArgs{fields: strings.Fields(text), rest: text, bare: true})
	}

	fields := strings.Fields(text)
	name := strings.TrimPrefix(fields[0], "/")
	// 这一拦放在查表之前。say 一定在表里，所以先拦与先查表再拦对输出没有区别；
	// 放在前面能保证这条路根本不碰 runCommand，inDraft 不可能被 cmdSay 置上。
	// 仅含空白的参数由 restOf 里的 TrimSpace 收成空串，同样算空参。
	if name == "say" && restOf(text) == "" {
		return r.printf("%s\n", r.t("tui.say.notInTUI", msgDraftNotInTUI))
	}
	// 查表包含别名（与 dispatch 同一条路）：/exit 与 /quit 解析到同一条命令。
	_, cmd, ok := r.cmds.Lookup(name)
	if !ok {
		return r.printf("%s\n", r.t("tui.repl.unknownCommand", msgUnknownCommand))
	}
	return r.runCommand(ctx, cmd, cmdArgs{fields: fields[1:], rest: restOf(text)})
}
