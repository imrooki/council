package repl

// CXH 契约 §2.3：/add 列出并加入 codex 识别出的模型，/help 的 /add 一行。
// 装置复用 MD1 的 mdxNewRig（真 SQLite、真配置文件、真 Runner、假执行器），
// 由 cxhNewRig 再给假执行器登记 codex 的输出；夹具与期望块在 cxh_fixture_test.go。
// 屏幕输出一律整行相等；外部命令一律经假执行器，不起真 codex。

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"council/internal/config"
)

// cxhListHead 是识别部分的首行（R3、R6 的期望由它接 cxhGrokOnlyTable）。
const cxhListHead = "正在识别模型: codex, grok"

// cxhRefreshHint 是有失败时最后打的那一行（MD1 既有字面量）。
const cxhRefreshHint = "失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh"

// cxhGrokOnlyWant 是 codex 一行都没显示时的识别部分开头：首行接 cxhGrokOnlyTable。
func cxhGrokOnlyWant() []string {
	return append([]string{cxhListHead}, mdxSplit(cxhGrokOnlyTable)...)
}

// cxhListBlockWithoutNote 是 cxhListBlock 去掉最后一行（custom 说明行）。
func cxhListBlockWithoutNote(t *testing.T) []string {
	t.Helper()
	lines := mdxSplit(cxhListBlock)
	if lines[len(lines)-1] != cxhNote {
		t.Fatalf("前提：cxhListBlock 的最后一行应是 cxhNote，实际 %q", lines[len(lines)-1])
	}
	return append([]string(nil), lines[:len(lines)-1]...)
}

// TestCxhFixtureIntact 覆盖 R0：repl 包那一份夹具没抄坏。
// 【判别力】对测试自身的结构性核对，不对应任何被测变异。
func TestCxhFixtureIntact(t *testing.T) {
	if n := len(cxhCodexFixture()); n != 24916 {
		t.Errorf("cxhCodexFixture() %d 字节，契约写的是 24916", n)
	}
	if n := len(cxhBigModelMessages()); n != 22000 {
		t.Errorf("cxhBigModelMessages() %d 字节，契约写的是 22000", n)
	}
	for _, c := range []struct {
		name string
		got  int
		want int
	}{
		{"cxhListBlock", len(mdxSplit(cxhListBlock)), 17},
		{"cxhGrokDownBlock", len(mdxSplit(cxhGrokDownBlock)), 14},
		{"cxhAllConfiguredBlock", len(mdxSplit(cxhAllConfiguredBlock)), 3},
	} {
		if c.got != c.want {
			t.Errorf("%s %d 行，契约写的是 %d", c.name, c.got, c.want)
		}
	}
}

// TestCxhAddListsCodex 覆盖 R1：codex 与 grok 两家都识别成功，codex 七行可选、
// 两条 custom/ 只计数。
//
// 【判别力】首行钉住 codex 进了识别（C1、C4、C5、C6 时只剩 grok 或多一行失败）；
// gpt-5.5 已配成 agent 而没有编号、+1 到 +7 连续；来源列（C18、C19）；EFFORTS
// 列按模型收窄（C11）；提示行是「claude 与 codex」那一句（C35）；最后一行说明
// 「还有 2 个」（C29、C30、C42）；说明行在表格与提示行之后（C49）；建议 id 列
// （C20、C23、C28）。执行器第 1 次收到的命令钉住 codex 的 Path、Args、Stdin
// （C2、C3、C4）。与 R3 是一对：R3 的可见行里没有 codex，提示行回到原句。
func TestCxhAddListsCodex(t *testing.T) {
	g := cxhNewRig(t, cxhConfigTemplate, cxhCodexFixture())
	mdxWantLines(t, "R1 识别部分", g.afterCandidates(g.run("/add")), mdxSplit(cxhListBlock))
	calls := g.exec.callsCopy()
	if len(calls) != 2 {
		t.Fatalf("假执行器应被调 2 次，实际 %d 次：%#v", len(calls), calls)
	}
	if calls[0].Path != cxhCodexPath {
		t.Errorf("第 1 次调用 Path = %q，期望 %q", calls[0].Path, cxhCodexPath)
	}
	if want := []string{"debug", "models"}; !reflect.DeepEqual(calls[0].Args, want) {
		t.Errorf("第 1 次调用 Args = %q，期望 %q", calls[0].Args, want)
	}
	if calls[0].Stdin != nil {
		t.Errorf("第 1 次调用 Stdin = %q，期望 nil", calls[0].Stdin)
	}
	if calls[1].Path != mdxGrokPath {
		t.Errorf("第 2 次调用 Path = %q，期望 %q", calls[1].Path, mdxGrokPath)
	}
}

// TestCxhAddListNoCustom 覆盖 R2：只差 custom/ 记录的条数（1 / 0），表格逐字同 R1。
//
// 【判别力】if snap.custom > 0 写成 > 1（C31）时只有 one 红；写成恒真（C32）时
// none 多出「还有 0 个」；数目写死成 2 的写法 one 红；说明行块与前面的 if / else
// 整段对调（C49）时 one 红（none 没有说明行）。hide 的 custom/qwen3.8-27b 一直在
// 夹具里，从不计入。
func TestCxhAddListNoCustom(t *testing.T) {
	oneOut := mdxReplaceOnce(t, cxhCodexFixture(), cxhRecGrokCustom, "")
	noneOut := mdxReplaceOnce(t, oneOut, cxhRecGeminiCustom, "")

	t.Run("one", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate, oneOut)
		want := append(cxhListBlockWithoutNote(t), cxhNoteOne)
		mdxWantLines(t, "R2 one 识别部分", g.afterCandidates(g.run("/add")), want)
	})
	t.Run("none", func(t *testing.T) {
		g := cxhNewRig(t, cxhConfigTemplate, noneOut)
		mdxWantLines(t, "R2 none 识别部分", g.afterCandidates(g.run("/add")), cxhListBlockWithoutNote(t))
	})
}

// TestCxhAddHintWhenCodexHidden 覆盖 R3：codex 只识别出 gpt-5.5，且它已配成 agent；
// 识别表里只有 grok 的行，提示行回到 MD1 原句。
//
// 【判别力】codex 那一行在快照里（编号 0）但不可见：提示行按快照全部行判（C33）、
// 恒写「claude 与 codex」（C34）、判据写反（C36）都红。与 R1 是一对。
func TestCxhAddHintWhenCodexHidden(t *testing.T) {
	g := cxhNewRig(t, cxhConfigTemplate, cxhOneGpt55)
	mdxWantLines(t, "R3 识别部分", g.afterCandidates(g.run("/add")), cxhGrokOnlyWant())
}

// TestCxhAddCodexEffortRefusals 覆盖 R4：6 条拒绝，每条恰好打一行，配置文件不变。
//
// 【判别力】minimal 在 CX1 的并集里、不在 gpt-6-sol 的档里；ultra 不在 gpt-6-luna
// 的档里；turbo 只有在不按 codexEffortLevels 收（C11）时才被当成支持的档；HIGH
// 钉住大小写敏感；cxh-bell 没有强度；+8 是 grok 行，拒绝句与 MD1 逐字相同（把
// codex 与 grok 一起放行的写法在这里红）。强度 switch 退回 != "claude"（C37）时
// 前 5 对都变成 grok / agy 那一句。正对照是 R5（同一装置，+1 默认与
// +1 --effort ultra 成功）。
func TestCxhAddCodexEffortRefusals(t *testing.T) {
	g := cxhNewRig(t, cxhConfigTemplate, cxhCodexFixture())
	g.run("/add")
	before := g.readFile()

	pairs := strings.Split(cxhRefusalPairs, "\n\n")
	if len(pairs) != 6 {
		t.Fatalf("前提：cxhRefusalPairs 应切成 6 对，实际 %d 对", len(pairs))
	}
	for _, p := range pairs {
		cl := strings.Split(p, "\n")
		if len(cl) != 2 {
			t.Fatalf("前提：每对应是 2 行，实际 %q", cl)
		}
		mdxWantLines(t, cl[0], g.run(cl[0]), []string{cl[1]})
	}
	if after := g.readFile(); string(after) != string(before) {
		t.Errorf("拒绝之后配置文件变了\n之前: %q\n之后: %q", before, after)
	}
}

// TestCxhAddCodexWritesConfig 覆盖 R5：依次加 6 个 codex 成员，追加的字节整串相等。
//
// 【判别力】默认强度 high、--effort ultra 成功（C37 时 +1 写出没有 effort 行的
// gpt-6-sol，--effort ultra 被拒）；显示名（C12 的 BEL、C25、C26、C27）；带点号
// id 的表名加引号；cxh-bell 没有 effort 行；cxh-min 默认 minimal；追加注释里的
// 来源「（codex 目录）」（C18、C19）；6 步之后 +2、+7 仍在（编号不漂），说明行
// 照样重印且在表格之后（C49）。执行器总共 2 次钉住快照沿用。
func TestCxhAddCodexWritesConfig(t *testing.T) {
	g := cxhNewRig(t, cxhConfigTemplate, cxhCodexFixture())
	orig := g.readFile()
	g.run("/add")
	for _, s := range cxhAddSteps {
		before := g.bakNames()
		lines := g.run(s.cmd)
		bak := g.newBak(before)
		mdxWantLines(t, s.cmd, lines, g.joinedLines(s.id, bak))
	}
	if got, want := string(g.readFile()), string(orig)+cxhAppendAll; got != want {
		t.Errorf("6 步之后配置文件不符\n实际: %q\n期望: %q", got, want)
	}
	wantAgent := config.AgentConfig{
		DisplayName: "GPT-6-Sol",
		Provider:    "codex",
		Model:       "gpt-6-sol",
		Effort:      "high",
		Role:        "",
		Permissions: config.PermissionRead,
		Mode:        config.ModeDiscussion,
	}
	if got := g.r.cfg.Agents["gpt-6-sol-high"]; !reflect.DeepEqual(got, wantAgent) {
		t.Errorf("r.cfg.Agents[gpt-6-sol-high] = %#v，期望 %#v", got, wantAgent)
	}
	mdxWantLines(t, "6 步之后的 /add 识别部分", g.afterCandidates(g.run("/add")), mdxSplit(cxhAfterBlock))
	if n := g.exec.count(); n != 2 {
		t.Errorf("假执行器总共被调 %d 次，期望 2 次（快照沿用）", n)
	}
}

// TestCxhAddCodexFailures 覆盖 R6：codex 识别失败的两种（Empty、Unparseable）。
//
// 【判别力】empty 里有一条 list 的 custom/ 行：把 custom 算成可选（C15、C16）时
// 不再失败、改打说明行；也钉住「Empty 时不打 custom 说明」。unparseable 喂的
// {"data":[]} 没有 models 键：去掉 nil 检查（C17）时解引用 nil 指针 panic，装置
// （runCtx）接住判红。两个子测试只差 codex 的输出。
func TestCxhAddCodexFailures(t *testing.T) {
	for _, c := range []struct {
		name string
		out  string
		fail string
	}{
		{"empty", cxhOnlyHiddenAndCustom, "识别 codex 的模型失败: 一个模型都没列出来（没登录？）"},
		{"unparseable", `{"data":[]}` + "\n", "识别 codex 的模型失败: 输出认不出来（CLI 版本变了？）"},
	} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			g := cxhNewRig(t, cxhConfigTemplate, c.out)
			want := append(cxhGrokOnlyWant(), c.fail, cxhRefreshHint)
			mdxWantLines(t, "R6 "+c.name+" 识别部分", g.afterCandidates(g.run("/add")), want)
		})
	}
}

// TestCxhHelpAddLine 覆盖 R7：/help 渲染出的 /add 与 /join 两整行。
//
// 【判别力】summary 少了 --refresh（C39）时 /add 那一行不等；/join 被跟着改的写法
// 让 /join 那一行不等。TestUx2rJoinIsAdd 钉 summary 字段本身，这里钉经 cmdHelp
// 渲染出的整行（契约 §2 通用纪律允许用 mdxHasLine 找整行）。
func TestCxhHelpAddLine(t *testing.T) {
	g := cxhNewRig(t, cxhConfigTemplate, cxhCodexFixture())
	lines := g.run("/help")
	if !mdxHasLine(lines, cxhHelpAddLine) {
		t.Errorf("/help 输出里没有与 cxhHelpAddLine 整行相等的一行：\n%s", strings.Join(lines, "\n"))
	}
	if !mdxHasLine(lines, cxhHelpJoinLine) {
		t.Errorf("/help 输出里没有与 cxhHelpJoinLine 整行相等的一行：\n%s", strings.Join(lines, "\n"))
	}
}

// TestCxhAddNoteWhenAllConfigured 覆盖 R8：codex 可选行都已配成 agent、没有表格时，
// custom 说明行照样打在「都已经在配置里了」之后。
//
// 【判别力】说明行只在可见行非空时打（C44）或只在成功识别的家数大于 1 时打
// （C47）都少了第三行；> 0 写成 > 1（C31）同样少（这里只剩 1 条 custom）；说明
// 行块与前面的 if / else 整段对调（C49）时第二、三行次序反了。执行器恰好 1 次。
func TestCxhAddNoteWhenAllConfigured(t *testing.T) {
	g := cxhNewRig(t, cxhCodexOnlyTemplate, cxhGpt55AndGrokCustom)
	mdxWantLines(t, "R8 识别部分", g.afterCandidates(g.run("/add")), mdxSplit(cxhAllConfiguredBlock))
	if n := g.exec.count(); n != 1 {
		t.Errorf("假执行器被调 %d 次，期望 1 次", n)
	}
}

// TestCxhAddNoteBeforeFailures 覆盖 R9：grok 识别失败时，说明行在失败行之前。
//
// 【判别力】说明行块整段挪到失败行循环之后（C45）时最后三行次序变；说明行只在
// 成功识别的家数大于 1 时打（C47）时少了说明行；C49 时说明行跑到表头之前。
// C44 只有 R8 红，C45 只有 R9 红。
func TestCxhAddNoteBeforeFailures(t *testing.T) {
	g := cxhNewRig(t, cxhConfigTemplate, cxhCodexFixture())
	g.exec.set(mdxGrokPath, mdxResp{err: errors.New("cxh grok down")})
	mdxWantLines(t, "R9 识别部分", g.afterCandidates(g.run("/add")), mdxSplit(cxhGrokDownBlock))
}
