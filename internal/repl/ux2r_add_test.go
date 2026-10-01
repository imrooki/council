package repl

// 验收 15-30：/add 与 /join，一个 handler 两个名字（契约第 6.3 节）。

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// ux2rTableRow 是 /add 表格里的一行，按 tabwriter 展开后的列切出来。
type ux2rTableRow struct {
	no, agent, provider, display string
}

// ux2rParseAddTable 在输出里找逐字的表头行，再按表头各列的起始偏移切出后续各行。
// 按偏移切而不是按空白切：这样同时断言了「是 tabwriter 对齐出来的列」与列序。
func ux2rParseAddTable(t *testing.T, out string) []ux2rTableRow {
	t.Helper()
	lines := ux2rLines(out)
	hi := -1
	for i, l := range lines {
		if strings.Join(strings.Fields(l), "\t") == ux2rTableHeader {
			hi = i
			break
		}
	}
	if hi < 0 {
		t.Fatalf("输出里找不到表头 %q（tabwriter 展开后按列比较）；完整输出:\n%s", ux2rTableHeader, out)
	}
	h := lines[hi]
	if !strings.HasPrefix(h, "NO") {
		t.Fatalf("表头应当以 NO 开头，实际 %q", h)
	}
	cAgent := strings.Index(h, "AGENT")
	cProv := strings.Index(h, "PROVIDER")
	cDisp := strings.Index(h, "DISPLAY_NAME")
	if !(0 < cAgent && cAgent < cProv && cProv < cDisp) {
		t.Fatalf("表头列序不对：%q", h)
	}
	var rows []ux2rTableRow
	for _, l := range lines[hi+1:] {
		if len(l) <= cDisp || l[0] < '0' || l[0] > '9' {
			break
		}
		cell := func(a, b int) string { return strings.TrimSpace(l[a:b]) }
		// 每一格的起点必须恰好落在表头对应列的起点上，且前一格与它之间有空白。
		for _, c := range []int{cAgent, cProv, cDisp} {
			if l[c-1] != ' ' || l[c] == ' ' {
				t.Fatalf("表格行 %q 在偏移 %d 上没有对齐到表头的列起点（表头 %q）", l, c, h)
			}
		}
		rows = append(rows, ux2rTableRow{
			no:       cell(0, cAgent),
			agent:    cell(cAgent, cProv),
			provider: cell(cProv, cDisp),
			display:  strings.TrimSpace(l[cDisp:]),
		})
	}
	return rows
}

// 验收 15：不带参数。配置 4 个、在场 1 个、已退出 1 个 -> 表里 3 行，
// 已退出的在里面，在场的不在，1 起编号、按字典序。
func TestUx2rAddListCandidates(t *testing.T) {
	fx := ux2rNewFix(t, "k4", "k2", "k3", "k1")
	fx.seedActive(t, "k2", "")
	fx.seedLeft(t, "k3", "")

	out := fx.run(t, "/add\n")
	rows := ux2rParseAddTable(t, out)

	want := []string{"k1", "k3", "k4"}
	if len(rows) != len(want) {
		t.Fatalf("表里应当恰好 %d 行，实际 %d 行 %+v；完整输出:\n%s", len(want), len(rows), rows, out)
	}
	for i, row := range rows {
		if row.no != fmt.Sprint(i+1) {
			t.Errorf("第 %d 行编号应当是 %d，实际 %q", i+1, i+1, row.no)
		}
		if row.agent != want[i] {
			t.Errorf("第 %d 行 AGENT 应当是 %s（配置 id 去掉在场成员、按字典序），实际 %q", i+1, want[i], row.agent)
		}
		ac := fx.cfg.Agents[want[i]]
		if row.provider != ac.Provider {
			t.Errorf("第 %d 行 PROVIDER 应当是 %s，实际 %q", i+1, ac.Provider, row.provider)
		}
		if row.display != ac.DisplayName {
			t.Errorf("第 %d 行 DISPLAY_NAME 应当是 %s，实际 %q", i+1, ac.DisplayName, row.display)
		}
	}
	if len(fx.st.addCalls) != 0 {
		t.Errorf("不带参数只列候选，不该调 Join，实际调了 %v", fx.st.addCalls)
	}
}

// 验收 16：全在场。不打表头，打那一句。
// 「不含 AGENT」按大小写敏感判：那一句里的 agent 是小写，不会误撞。
func TestUx2rAddListAllPresent(t *testing.T) {
	fx := ux2rNewFix(t, "k1", "k2")
	fx.seedActive(t, "k1", "")
	fx.seedActive(t, "k2", "")

	out := fx.run(t, "/add\n")

	if strings.Contains(out, "AGENT") {
		t.Errorf("候选为空时不许打表头；完整输出:\n%s", out)
	}
	ux2rRequireLine(t, out, ux2rMsgAllPresent)
}

// 验收 17：编号、全名、前缀混用，ASCII 空白与逗号混切；按首次出现顺序加入并回显全名。
func TestUx2rAddMixedRefs(t *testing.T) {
	fx := ux2rNewFix(t, "alpha", "grok-4.7", "opus-a")

	out := fx.run(t, "/add 1,grok-4.7 op\n")

	if got := fx.activeIDs(t); !ux2rSameStrings(got, []string{"alpha", "grok-4.7", "opus-a"}) {
		t.Fatalf("三个都该加入，实际在场 %v；完整输出:\n%s", got, out)
	}
	idx := make([]int, 0, 3)
	for _, id := range []string{"alpha", "grok-4.7", "opus-a"} {
		if n := ux2rCountLine(out, ux2rJoined(id)); n != 1 {
			t.Errorf("%q 应当恰好出现 1 次，实际 %d 次；完整输出:\n%s", ux2rJoined(id), n, out)
		}
		idx = append(idx, ux2rLineIndex(out, ux2rJoined(id)))
	}
	if !(idx[0] < idx[1] && idx[1] < idx[2]) {
		t.Errorf("回显顺序应当是段的首次出现顺序 alpha、grok-4.7、opus-a，实际行号 %v；完整输出:\n%s", idx, out)
	}
	if !ux2rSameStrings(fx.st.addCalls, []string{"alpha", "grok-4.7", "opus-a"}) {
		t.Errorf("Join 的调用顺序应当是 [alpha grok-4.7 opus-a]，实际 %v", fx.st.addCalls)
	}
}

// 验收 18：多段解析到同一个 id 只 Join 一次、只回显一次。
func TestUx2rAddDedup(t *testing.T) {
	fx := ux2rNewFix(t, "alpha", "beta")

	out := fx.run(t, "/add 1,alpha\n")

	if !ux2rSameStrings(fx.st.addCalls, []string{"alpha"}) {
		t.Errorf("alpha 只该 Join 一次，实际调用 %v；完整输出:\n%s", fx.st.addCalls, out)
	}
	if n := ux2rCountLine(out, ux2rJoined("alpha")); n != 1 {
		t.Errorf("alpha 的加入回显应当恰好 1 次，实际 %d 次；完整输出:\n%s", n, out)
	}
	if _, ok := fx.participant(t, "beta"); ok {
		t.Errorf("beta 不该被加入；完整输出:\n%s", out)
	}
}

// 验收 19：/leave 之后 /add 能把同一行重新激活；全名与前缀两条路都要行（已退出的在候选里）。
func TestUx2rAddRejoinAfterLeave(t *testing.T) {
	for _, ref := range []string{"alpha", "al"} {
		t.Run(ref, func(t *testing.T) {
			fx := ux2rNewFix(t, "alpha", "beta")
			fx.seedActive(t, "alpha", "")

			out := fx.run(t, "/leave alpha\n/add "+ref+"\n")

			p, ok := fx.participant(t, "alpha")
			if !ok || !p.Active {
				t.Fatalf("/leave 之后 /add %s 应当让 alpha 重新在场，实际 %+v（存在=%v）；完整输出:\n%s", ref, p, ok, out)
			}
			ux2rRequireLine(t, out, ux2rJoined("alpha"))
		})
	}
}

// 验收 20a：全或无。合法段写在前面、歧义段在后：合法那个也不许加入。
// 「解析一段就 Join 一段」的实现会先把 alpha 加进去，这条必红。
func TestUx2rAddAllOrNothingValidFirst(t *testing.T) {
	fx := ux2rNewFix(t, "alpha", "opus-a", "opus-b")

	out := fx.run(t, "/add alpha opus\n")

	if len(fx.st.addCalls) != 0 {
		t.Errorf("任何一段失败就一个都不加，实际调了 Join %v；完整输出:\n%s", fx.st.addCalls, out)
	}
	if got := fx.activeIDs(t); len(got) != 0 {
		t.Errorf("不该有任何人在场，实际 %v", got)
	}
	ux2rRequireLine(t, out, fmt.Sprintf(ux2rFmtAmbiguous, "opus", "opus-a, opus-b"))
	if ux2rCountLine(out, ux2rJoined("alpha")) != 0 {
		t.Errorf("alpha 不该有加入回显；完整输出:\n%s", out)
	}
}

// 验收 20b：每段都报错。歧义段在前、越界段在后：两段的错误都要按段序打出来。
// 「遇到第一个错误就停」的实现会漏掉后面那段，这条必红。
func TestUx2rAddReportsEverySegmentError(t *testing.T) {
	fx := ux2rNewFix(t, "opus-a", "opus-b")

	out := fx.run(t, "/add opus,99\n")

	amb := fmt.Sprintf(ux2rFmtAmbiguous, "opus", "opus-a, opus-b")
	oor := fmt.Sprintf(ux2rFmtOutOfRange, "99", 2)
	ia, io := ux2rLineIndex(out, amb), ux2rLineIndex(out, oor)
	if ia < 0 {
		t.Errorf("缺歧义那一行 %q；完整输出:\n%s", amb, out)
	}
	if io < 0 {
		t.Errorf("缺越界那一行 %q；完整输出:\n%s", oor, out)
	}
	if ia >= 0 && io >= 0 && ia > io {
		t.Errorf("错误应当按段出现的顺序打：歧义在前、越界在后；完整输出:\n%s", out)
	}
	if len(fx.st.addCalls) != 0 {
		t.Errorf("不该调 Join，实际 %v", fx.st.addCalls)
	}
}

// 验收 21（B2，W1 blocker）：opus-5-0 在场，/add opus-5-0 与 /join opus-5-0 都是
// 「照今天 /join 的行为」刷新 opus-5-0 自己，opus-5-0-max 绝不被加入。
// 先在候选里做前缀的实现会静默加入 max。
func TestUx2rAddB2ExactActiveMemberNotStolenByPrefix(t *testing.T) {
	for _, cmd := range []string{"/add", "/join"} {
		t.Run(strings.TrimPrefix(cmd, "/"), func(t *testing.T) {
			fx := ux2rNewFix(t, "opus-5-0", "opus-5-0-max")
			fx.seedActive(t, "opus-5-0", "")

			out := fx.run(t, cmd+" opus-5-0\n")

			if _, ok := fx.participant(t, "opus-5-0-max"); ok {
				t.Fatalf("opus-5-0-max 被加入了：全名被另一个以它为前缀的候选偷走；完整输出:\n%s", out)
			}
			if strings.Contains(out, "opus-5-0-max") {
				t.Errorf("输出里不该出现 opus-5-0-max；完整输出:\n%s", out)
			}
			if n := ux2rCountLine(out, ux2rJoined("opus-5-0")); n != 1 {
				t.Errorf("应当恰好一行 %q，实际 %d 行；完整输出:\n%s", ux2rJoined("opus-5-0"), n, out)
			}
			if !ux2rWordRe("opus-5-0").MatchString(out) {
				t.Errorf("opus-5-0 没有作为完整的词出现；完整输出:\n%s", out)
			}
			if !ux2rSameStrings(fx.st.addCalls, []string{"opus-5-0"}) {
				t.Errorf("Join 应当恰好对 opus-5-0 调一次（刷新快照），实际 %v", fx.st.addCalls)
			}
		})
	}
}

// 验收 22（B3，W1b major）：配置只有 opus-5-0-max，库里有一个不在配置里的成员 opus-5-0
// （在场与已退出两种）。/add opus-5-0 必须按今天 cmdJoin 那句失败，绝不进前缀解析。
// 精确集合只看配置 id 的实现会加入 max，这条必红。
func TestUx2rAddB3GoneFromConfigMember(t *testing.T) {
	for _, state := range []string{"active", "left"} {
		for _, cmd := range []string{"/add", "/join"} {
			t.Run(state+"_"+strings.TrimPrefix(cmd, "/"), func(t *testing.T) {
				fx := ux2rNewFix(t, "opus-5-0-max")
				if state == "active" {
					fx.seedActive(t, "opus-5-0", "fake")
				} else {
					fx.seedLeft(t, "opus-5-0", "fake")
				}

				out := fx.run(t, cmd+" opus-5-0\n")

				if _, ok := fx.participant(t, "opus-5-0-max"); ok {
					t.Fatalf("opus-5-0-max 被加入了：已从配置删掉的成员敲全名时被前缀解析偷走；完整输出:\n%s", out)
				}
				ux2rRequireLine(t, out, fmt.Sprintf(ux2rFmtJoinNoConfig, "opus-5-0"))
				if len(fx.st.addCalls) != 0 {
					t.Errorf("这一段失败，不该调 Join，实际 %v", fx.st.addCalls)
				}
			})
		}
	}
}

// 验收 23：刷新快照保留。X 在场、参与者行的 provider 是旧值，/join X 回显加入，
// provider 变成配置里的值。「在场就拒绝」的实现会红。
func TestUx2rJoinActiveRefreshesSnapshot(t *testing.T) {
	for _, cmd := range []string{"/join", "/add"} {
		t.Run(strings.TrimPrefix(cmd, "/"), func(t *testing.T) {
			fx := ux2rNewFix(t, "alpha", "beta")
			fx.seedActive(t, "alpha", "pv_old_snapshot")

			out := fx.run(t, cmd+" alpha\n")

			ux2rRequireLine(t, out, ux2rJoined("alpha"))
			p, ok := fx.participant(t, "alpha")
			if !ok || !p.Active {
				t.Fatalf("alpha 应当仍在场，实际 %+v（存在=%v）", p, ok)
			}
			if p.Provider != ux2rProvider {
				t.Errorf("参与者行的 provider 应当刷新成配置里的 %s，实际 %q；完整输出:\n%s", ux2rProvider, p.Provider, out)
			}
			if _, ok := fx.participant(t, "beta"); ok {
				t.Errorf("beta 不该被加入")
			}
		})
	}
}

// 验收 24：前缀只命中在场成员 -> 打「开头的都已经在当前会话里了」，什么都不加。
func TestUx2rAddPrefixOnlyActiveMembers(t *testing.T) {
	fx := ux2rNewFix(t, "opus-a", "grok-4.7")
	fx.seedActive(t, "opus-a", "")

	out := fx.run(t, "/add op\n")

	ux2rRequireLine(t, out, fmt.Sprintf(ux2rFmtAllPresentPfx, "op"))
	if len(fx.st.addCalls) != 0 {
		t.Errorf("不该调 Join，实际 %v；完整输出:\n%s", fx.st.addCalls, out)
	}
}

// 没找到、也不是任何在场成员的前缀 -> 打「配置里没有可加入的」那一句。
func TestUx2rAddNotFound(t *testing.T) {
	fx := ux2rNewFix(t, "opus-a", "grok-4.7")
	fx.seedActive(t, "opus-a", "")

	out := fx.run(t, "/add zz\n")

	ux2rRequireLine(t, out, fmt.Sprintf(ux2rFmtAddNotFound, "zz"))
	if len(fx.st.addCalls) != 0 {
		t.Errorf("不该调 Join，实际 %v；完整输出:\n%s", fx.st.addCalls, out)
	}
}

// 验收 25：全数字 id。配置里有一个叫 7 的 agent，且候选列表至少 7 个，
// 这样「编号优先于精确匹配」的实现会加入候选列表的第 7 个（a6），能被区分出来。
func TestUx2rAddAllDigitIDExactBeatsIndex(t *testing.T) {
	fx := ux2rNewFix(t, "7", "a1", "a2", "a3", "a4", "a5", "a6", "a7")

	out := fx.run(t, "/add 7\n")

	if got := fx.activeIDs(t); !ux2rSameStrings(got, []string{"7"}) {
		t.Fatalf("/add 7 应当按名字加入 7，实际在场 %v（候选第 7 个是 a6）；完整输出:\n%s", got, out)
	}
	ux2rRequireLine(t, out, ux2rJoined("7"))
}

// 验收 26：Join 中途失败即停。第 2 次 AddParticipant 返回错误，/add a,b,c 只有 a 加入，
// AddParticipant 一共只被调 2 次。拿 reportErr 的返回值当「出错了」的实现会接着加 c。
func TestUx2rAddStopsOnJoinFailure(t *testing.T) {
	fx := ux2rNewFix(t, "a", "b", "c")
	fx.st.failOnCall = 2

	out := fx.run(t, "/add a,b,c\n")

	if len(fx.st.addCalls) != 2 {
		t.Fatalf("AddParticipant 应当恰好被调 2 次（第 2 次失败后必须停下），实际 %d 次 %v；完整输出:\n%s",
			len(fx.st.addCalls), fx.st.addCalls, out)
	}
	if got := fx.activeIDs(t); !ux2rSameStrings(got, []string{"a"}) {
		t.Errorf("只有 a 应当在场，实际 %v", got)
	}
	if !strings.Contains(out, "加入失败") {
		t.Errorf("输出里应当有「加入失败」；完整输出:\n%s", out)
	}
	ux2rRequireLine(t, out, ux2rJoined("a"))
	if ux2rCountLine(out, ux2rJoined("c")) != 0 {
		t.Errorf("c 不该有加入回显；完整输出:\n%s", out)
	}
}

// 验收 27：编号越界与溢出，不 panic，什么都不加；候选为空时是另一句。
func TestUx2rAddIndexOutOfRange(t *testing.T) {
	for _, seg := range []string{"99", "0", "99999999999999999999999"} {
		t.Run(seg, func(t *testing.T) {
			fx := ux2rNewFix(t, "alpha", "beta")
			out := fx.run(t, "/add "+seg+"\n")
			ux2rRequireLine(t, out, fmt.Sprintf(ux2rFmtOutOfRange, seg, 2))
			if len(fx.st.addCalls) != 0 {
				t.Errorf("不该调 Join，实际 %v", fx.st.addCalls)
			}
		})
	}
	t.Run("候选为空", func(t *testing.T) {
		fx := ux2rNewFix(t, "alpha")
		fx.seedActive(t, "alpha", "")
		out := fx.run(t, "/add 1\n")
		ux2rRequireLine(t, out, fmt.Sprintf(ux2rFmtOutOfRangeNone, "1"))
		if len(fx.st.addCalls) != 0 {
			t.Errorf("不该调 Join，实际 %v", fx.st.addCalls)
		}
	})
}

// 验收 28：切分后一段都不剩 -> 打用法那一行，什么都不加。
func TestUx2rAddEmptySegments(t *testing.T) {
	for _, arg := range []string{",", string(ux2rFullComma), ", " + string(ux2rFullComma) + " ,"} {
		t.Run(arg, func(t *testing.T) {
			fx := ux2rNewFix(t, "alpha")
			out := fx.run(t, "/add "+arg+"\n")
			ux2rRequireLine(t, out, ux2rMsgAddUsage)
			if len(fx.st.addCalls) != 0 {
				t.Errorf("不该调 Join，实际 %v；完整输出:\n%s", fx.st.addCalls, out)
			}
		})
	}
}

// 验收 29：全角逗号也是分隔符。
func TestUx2rAddFullWidthComma(t *testing.T) {
	fx := ux2rNewFix(t, "alpha", "grok-4.7")

	out := fx.run(t, "/add alpha"+string(ux2rFullComma)+"grok-4.7\n")

	if got := fx.activeIDs(t); !ux2rSameStrings(got, []string{"alpha", "grok-4.7"}) {
		t.Fatalf("两个都该加入，实际在场 %v；完整输出:\n%s", got, out)
	}
}

// 验收 30：/join 与 /add 同一实现。同样的状态下输出逐字节相同；命令表里两项指向
// 同一个函数 cmdAdd，summary 逐字照契约。
func TestUx2rJoinIsAdd(t *testing.T) {
	type setup func(t *testing.T) *ux2rFix
	states := map[string]setup{
		"唯一前缀": func(t *testing.T) *ux2rFix { return ux2rNewFix(t, "opus-a", "grok-4.7") },
		"歧义": func(t *testing.T) *ux2rFix {
			return ux2rNewFix(t, "opus-a", "opus-b")
		},
		"前缀只命中在场": func(t *testing.T) *ux2rFix {
			fx := ux2rNewFix(t, "opus-a", "grok-4.7")
			fx.seedActive(t, "opus-a", "")
			return fx
		},
	}
	for _, arg := range []string{"op", "", "opus", "op,99"} {
		for name, mk := range states {
			t.Run(name+"_"+arg, func(t *testing.T) {
				outJoin := mk(t).run(t, strings.TrimSpace("/join "+arg)+"\n")
				outAdd := mk(t).run(t, strings.TrimSpace("/add "+arg)+"\n")
				if outJoin != outAdd {
					t.Errorf("/join %s 与 /add %s 在同样状态下输出不同\n/join:\n%s\n/add:\n%s", arg, arg, outJoin, outAdd)
				}
			})
		}
	}

	tbl := newCommandTable()
	add, okA := tbl["add"]
	join, okJ := tbl["join"]
	if !okA || !okJ {
		t.Fatalf("命令表里必须同时有 add 与 join（add=%v join=%v）", okA, okJ)
	}
	pAdd := reflect.ValueOf(add.run).Pointer()
	pJoin := reflect.ValueOf(join.run).Pointer()
	pCmd := reflect.ValueOf(cmdAdd).Pointer()
	if pAdd != pCmd || pJoin != pCmd {
		t.Errorf("add 与 join 的 run 都必须是 cmdAdd，不许两份实现并存")
	}
	const wantAddSummary = "把 agent 加进当前会话: /add 列出配置里可加的与识别出的模型；/add <全名|编号|前缀>[,...] 一次加多个；识别出的模型可以直接 /add 它的名字（识别表里的 ID，或模型名），也可以 /add +编号，都可带 [--id <新 id>] [--effort <强度>]，配置里还没有就先写进配置再加入；/add --refresh 重新识别"
	if add.summary != wantAddSummary {
		t.Errorf("add 的 summary 应当是 %q，实际 %q", wantAddSummary, add.summary)
	}
	if join.summary != "同 /add" {
		t.Errorf("join 的 summary 应当是 %q，实际 %q", "同 /add", join.summary)
	}
	for name, c := range map[string]command{"add": add, "join": join} {
		if !c.needsSvc || !c.needsConv || c.needsRunner {
			t.Errorf("%s 的前置条件应当是 needsSvc、needsConv、不要 runner，实际 %+v", name, c)
		}
	}
}
