package tui

// i18n_tui_test.go 钉住本片的两处中英分叉：状态栏短句与轮次看板/段终态块
// （repl.OutcomeWordLang 的 TUI 出口）。中文一侧不走目录兜底以外的任何分支，
// 所以断言的是「中文有汉字、英文一个汉字都没有、两边确实不同」。

import (
	"strings"
	"testing"
	"time"
	"unicode"

	"council/internal/i18n"
	"council/internal/repl"
	"council/internal/turn"
)

func tuiHasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// TestTUIStatusLineFromOptionsLanguage 钉住状态栏的初始短句来自 Options.Lang：
// 零值 / zh 是「空闲」，en 是 idle，且英文里没有汉字。
func TestTUIStatusLineFromOptionsLanguage(t *testing.T) {
	zh := New(Options{Lang: i18n.LangZh})
	if zh.status != "空闲" {
		t.Errorf("zh 状态栏 = %q，期望 空闲", zh.status)
	}
	en := New(Options{Lang: i18n.LangEn})
	if en.status != "idle" {
		t.Errorf("en 状态栏 = %q，期望 idle", en.status)
	}
	if tuiHasHan(en.status) {
		t.Errorf("en 状态栏里出现汉字: %q", en.status)
	}
}

// TestTUIStatusCopyHintBothLanguages 覆盖一条运行期状态栏提示：它经 m.t 取，
// 中英必须分叉，英文不得含汉字。
func TestTUIStatusCopyHintBothLanguages(t *testing.T) {
	zh := New(Options{Lang: i18n.LangZh})
	zh.status = zh.t("tui.status.copyBusy", "一轮正在跑，等它回来再复制")
	en := New(Options{Lang: i18n.LangEn})
	en.status = en.t("tui.status.copyBusy", "一轮正在跑，等它回来再复制")
	if zh.status == en.status {
		t.Fatalf("中英状态栏相同：%q", zh.status)
	}
	if !tuiHasHan(zh.status) {
		t.Errorf("中文状态栏里没有汉字：%q", zh.status)
	}
	if tuiHasHan(en.status) {
		t.Errorf("英文状态栏里出现汉字：%q", en.status)
	}
}

// TestTUIBoardWordsChineseAndEnglish 覆盖看板的全部结局词与三个标记词，
// 以及「在跑」「子进程」「回来」三处格式词。
func TestTUIBoardWordsChineseAndEnglish(t *testing.T) {
	outcomes := []turn.Outcome{
		turn.OutcomeUnknown, turn.OutcomeSpoke, turn.OutcomePostRejected,
		turn.OutcomeProviderFailed, turn.OutcomeUnparsable, turn.OutcomeTimeout,
		turn.OutcomeSpawnFailed, turn.OutcomeRefused, turn.OutcomeCanceled,
		turn.OutcomeLost,
	}
	for _, o := range outcomes {
		zh := outcomeWord(i18n.LangZh, o)
		en := outcomeWord(i18n.LangEn, o)
		if zh == en {
			t.Errorf("%v 的中英结局词相同：%q", o, zh)
		}
		if !tuiHasHan(zh) {
			t.Errorf("%v 的中文结局词没有汉字：%q", o, zh)
		}
		if tuiHasHan(en) {
			t.Errorf("%v 的英文结局词出现汉字：%q", o, en)
		}
	}

	zhRun := memberRunningLine(i18n.LangZh, "ag_a", 4, time.Second)
	enRun := memberRunningLine(i18n.LangEn, "ag_a", 4, time.Second)
	if !strings.Contains(zhRun, "在跑") || strings.Contains(enRun, "在跑") {
		t.Errorf("在跑行的中英分叉不对：zh=%q en=%q", zhRun, enRun)
	}
	if tuiHasHan(enRun) {
		t.Errorf("英文在跑行出现汉字：%q", enRun)
	}

	zhWall := wallText(i18n.LangZh, 1200)
	enWall := wallText(i18n.LangEn, 1200)
	if !strings.Contains(zhWall, "子进程") || !strings.Contains(enWall, "subprocess") {
		t.Errorf("子进程格式词中英不对：zh=%q en=%q", zhWall, enWall)
	}

	zhSeg := segmentCell(i18n.LangZh, roundRun{has: true, label: "发言轮", k: 1, n: 2, members: 2, back: 1})
	enSeg := segmentCell(i18n.LangEn, roundRun{has: true, label: "speaking round", k: 1, n: 2, members: 2, back: 1})
	if !strings.Contains(zhSeg, "回来 1/2") || !strings.Contains(enSeg, "back 1/2") {
		t.Errorf("回来计数中英不对：zh=%q en=%q", zhSeg, enSeg)
	}
}

// TestTUISegmentBlockEnglishAndChinese 钉住段终态块的整段分叉：同一份事件
// 换 Lang 之后，结局词、标记词、回来计数全变，英文整段没有汉字。
func TestTUISegmentBlockEnglishAndChinese(t *testing.T) {
	mk := func(lang i18n.Lang, label string) repl.RoundEndEvent {
		return repl.RoundEndEvent{
			Label: label,
			K:     1,
			N:     2,
			Lang:  lang,
			Results: []repl.MemberDoneEvent{
				{AgentID: "ag_a", Outcome: turn.OutcomeSpoke, WallMS: 1200, Lang: lang, MaybeAdvanced: true},
				{AgentID: "ag_b", Outcome: turn.OutcomeTimeout, WallMS: 0, Lang: lang, Dropped: true, Drifted: true},
			},
		}
	}
	zh := strings.Join(endBlockLines(mk(i18n.LangZh, "发言轮")), "\n")
	en := strings.Join(endBlockLines(mk(i18n.LangEn, "speaking round")), "\n")

	if !tuiHasHan(zh) {
		t.Fatalf("中文段终态块没有汉字：\n%s", zh)
	}
	if tuiHasHan(en) {
		t.Fatalf("英文段终态块出现汉字：\n%s", en)
	}
	for _, want := range []string{"done", "timeout", " subprocess", "back 2/2", "may have consumed", "text shown above", "(prefix broken)"} {
		if !strings.Contains(en, want) {
			t.Errorf("英文段终态块缺少 %q：\n%s", want, en)
		}
	}
	for _, want := range []string{"完成", "超时", "子进程", "回来 2/2", "可能已吃下", "正文见上方", "(前缀已断)"} {
		if !strings.Contains(zh, want) {
			t.Errorf("中文段终态块缺少 %q：\n%s", want, zh)
		}
	}
}
