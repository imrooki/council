package repl

import (
	"context"
	"strings"
)

// 括号粘贴的 CSI 标记。ESC 为 0x1b。
const (
	pasteStartSeq = "\x1b[200~"
	pasteEndSeq   = "\x1b[201~"
)

// splitPasteMarkers 从一行里剥掉括号粘贴标记，返回剩余正文以及是否见到起止标记。
//
// 规则：
//   - START 为 ESC[200~，END 为 ESC[201~；按出现位置剥，不要求贴在行首或行尾。
//   - 一行里同时有 START 与 END 时，两个布尔都为 true。全部标记剥掉后的剩余即正文：
//     对形状 ESC[200~中间ESC[201~，剩余就是中间那段；若标记外侧还有字符
//     （先打了字再粘贴），外侧一并保留。
//   - 同一标记在一行里出现多次：对应布尔仍为 true，每一次出现都剥掉。
//     终端协议不应嵌套括号粘贴，重复标记视为噪声，不按第一次或最后一次切片，
//     以免把标记字面留进正文。
//   - 一行里没有标记时原样返回，两个布尔都为 false。
//
// 本函数是纯字符串处理，不碰任何系统调用，所以能被单测完全钉死。
//
// 为什么逐行就够：REPL 用 bufio.Scanner + ScanLines 读输入，无论字节怎么分块
// 到达，到达这一层时一定已经按换行切成行了。多行粘贴怎么分块本机尚未实测
// （2026-09-21 那次粘的是一行，两个标记落在同一段里），但两种形状本函数都处理：
// 标记分落首尾两行时各自回答 start/end，同落一行时两个布尔一起为 true。
func splitPasteMarkers(line string) (text string, start, end bool) {
	start = strings.Contains(line, pasteStartSeq)
	end = strings.Contains(line, pasteEndSeq)
	if !start && !end {
		return line, false, false
	}
	text = line
	if start {
		text = strings.ReplaceAll(text, pasteStartSeq, "")
	}
	if end {
		text = strings.ReplaceAll(text, pasteEndSeq, "")
	}
	return text, start, end
}

// handlePasteInvalid 处理粘贴途中（含本行刚见到 START）的非法 UTF-8。
// 整份粘贴作废，但留在 inPaste 里继续吃后续行，直到 END 才退出；END 仍只结束粘贴层。
//
// 与 inDraft 那一支同一条理由：只丢这一行会把一段粘贴悄悄截去一截、用户看不出
// 少了什么；就此退出模式则会让粘贴剩下的行退回单行模式、一条条变成永久消息。
func (r *REPL) handlePasteInvalid(ctx context.Context, start, end bool) error {
	if start && !r.inPaste {
		r.inPaste = true
		r.paste = nil
		r.pastePoisoned = false
	}
	if !r.inPaste {
		return r.printf("%s\n", msgNotUTF8)
	}
	var err error
	if !r.pastePoisoned {
		r.pastePoisoned = true
		err = r.printf("%s\n", msgPasteBadEncoding)
	}
	if end {
		if ferr := r.finishPaste(ctx); ferr != nil && err == nil {
			err = ferr
		}
	}
	return err
}

// handlePaste 收集括号粘贴的一行。
//
// 粘贴期间不解释任何斜杠命令；/end 与 /cancel 这两个字面量也不生效，
// 出现在正文里就是正文。这正是相对「让用户先敲 /say」多出来的正确性：
// 粘贴块里的斜杠行不会提前结束或取消输入。
//
// 已在 inDraft 里又收到 START 时，仍走本函数自己的缓冲，不另起一条发言。
// END 只把 inPaste 关掉；随后 finishPaste 把收集到的行并进 draft。
// 理由：若让 END 去结束草稿层，会把还没写完的 /say 提前发出，而事实源只追加不回收。
//
// 已在粘贴中再见到 START：不重置缓冲（否则已收行会丢），标记已剥，剩余当正文。
func (r *REPL) handlePaste(ctx context.Context, text string, start, end bool) error {
	if start && !r.inPaste {
		r.inPaste = true
		r.paste = nil
		r.pastePoisoned = false
	}
	if !r.inPaste {
		return nil
	}
	if !r.pastePoisoned {
		r.paste = append(r.paste, text)
	}
	if end {
		return r.finishPaste(ctx)
	}
	return nil
}

// finishPaste 结束 inPaste：毒化则丢弃；否则把行用换行连成一条。
// 若此时仍在草稿里，并进 draft（不走 collectDraft，以免粘贴块里的 /end /cancel
// 被当成控制符）。否则走 /say 表项，连前置条件一起复用。
func (r *REPL) finishPaste(ctx context.Context) error {
	lines := r.paste
	poisoned := r.pastePoisoned
	r.inPaste = false
	r.paste = nil
	r.pastePoisoned = false

	if poisoned {
		return nil
	}
	body := strings.TrimFunc(strings.Join(lines, "\n"), isTrimmable)
	if body == "" {
		return nil
	}
	if r.inDraft {
		r.draft = append(r.draft, strings.Split(body, "\n")...)
		return nil
	}
	// 粘贴进来的一段与敲进来的一行同样是裸文本（#47），bare 置上。
	return r.runCommand(ctx, sayCmd, cmdArgs{
		fields: strings.Fields(body),
		rest:   body,
		bare:   true,
	})
}
