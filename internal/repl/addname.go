package repl

import (
	"context"
	"fmt"
	"strings"

	"council/internal/provider"
)

// addname.go 是 /add 按名字加识别出的模型（AD1，00_DECISIONS.md #49）。
//
// /add <名字> 的名字在配置里对不上（resolveAddConfig 的第 0-3 步都没命中）时，到本会话
// 的识别快照里找；两个词的写法（/add <模型名> <强度>）与带 --id / --effort 的写法里，名字
// 在配置里只是前缀命中（configPrefixOnly）也会去找。找到之后要新建成员的，走与 /add +编号
// 同一个写配置流程（addDiscoveredModel），屏幕输出与写进文件的字节都相同；不是每一种都
// 写配置：没给强度也没给 --id 的单个名字可能只是加入配置里已有的成员，或者拒绝、列出，
// 见下面「单个名字没给强度」那一段。快照里的每一行都能按名字加，包括拍快照时已在配置里、
// 没有编号的行（16a open item (c)）；custom/ 开头的 codex 行不进快照，加不了。本会话
// 还没有快照时先识别一次（ensureModelSnapshot，与不带参数的 /add 同一份快照）。
//
// 两种名字形式（matchDiscoveredName）：
//   - ID 形式：等于某一行在它支持的某一档强度下的 SuggestedID，强度随名字定；或等于
//     不带强度的 SuggestedID("")（没有强度可选的行只有这一种，例如 grok、agy、claude 的
//     haiku；有强度的行例如 claude 的 sonnet-5），强度与模型名形式一样另取。
//   - 模型名形式：等于某一行的 Model，或非空的 Alias；强度取 --effort 或两个词写法的
//     第二个词，都没给取 DefaultEffort。
//
// 单个名字没给强度（名字不带强度、没有第二个词、没有 --effort）也没给 --id 时，先看配置里
// 这个模型已有的成员（addname_existing.go 的 addNameExisting）：有就加入、拒绝或列出，都不写
// 配置；没有才按默认档新建。
//
// 名字按 strings.EqualFold 比，写进配置的 id 是 SuggestedID（或 --id），不是敲的原样。
// 强度一律区分大小写：--effort 与两个词写法的第二个词，都与 /add +编号 的 --effort
// 同一条判据。

// 按名字加识别出的模型的说法（§3 冻结）。都是整行，含换行。
const (
	// msgAddNameNotFound 是配置里与识别快照里都对不上时的那一句（addSegmentNotFound）。
	msgAddNameNotFound = "配置里和识别结果里都没有 %s；不带参数的 /add 列出配置里的成员与识别出的模型，识别表里的模型可以直接 /add 它的 ID\n"
	// msgAddNameUsage 是带 --id 或 --effort 时参数形状不对。
	msgAddNameUsage = "用法: /add <识别表里的 ID 或模型名> [--id <新 id>] [--effort <强度>]；带 --id 或 --effort 时只能加一个\n"
	// msgAddNameOneAtATime 是多段里有一段要按识别结果加。
	msgAddNameOneAtATime = "%s 是识别出的模型，识别出的模型一次加一个：单独 /add %s\n"
	// msgAddNameEffortTwice 是 ID 形式已带强度，又给了不同的 --effort，或后面又跟了第二个词。
	msgAddNameEffortTwice = "%s 已经带着强度 %s，不能再给强度 %s\n"
	// msgAddNameFlagsConfig 是带 --id 或 --effort、名字却在配置里命中了一个 agent（全名或编号，或只是
	// 前缀命中而名字在识别快照里对不上）。
	msgAddNameFlagsConfig = "%s 对上的是配置里的 agent %s，--id 与 --effort 只用于加识别出的模型；直接 /add %s\n"
	// msgAddNameAmbiguous 是一个名字对上快照里的多行；后面逐行列出候选。
	msgAddNameAmbiguous = "%s 对上了识别出的 %d 个模型，不猜；用 +编号 或带强度的 ID 指明:\n"
)

// SetAddByName 打开或关上「/add 按名字加识别出的模型」（AD1）。零值 false 是关：/add
// 不按名字识别、不多起任何进程、说法与 AD1 之前逐字节相同（只有 splitAddArgs 按全角
// 空格切段这一条不看开关），所以直接 New 出来的 REPL（既有测试）行为不变；app 装配时在
// TUI 与行式两处都打开。/join 与 /add 是同一个 handler，同样生效。只许在 Run 或第一次
// Submit 之前调。
func (r *REPL) SetAddByName(on bool) {
	r.addByName = on
}

// nameMatch 是识别快照里按名字对上的一行。byID 为真表示按 ID 形式对上，effort 是名字
// 里带的那一档（不带强度的 SuggestedID("") 对上时是空串）；模型名形式对上时 byID 为假、
// effort 为空串。effort 为空串时，两种形式的强度都由 addByOneName 另取。
type nameMatch struct {
	row    snapRow
	byID   bool
	effort string
}

// matchDiscoveredName 在快照里按名字找行，按快照的顺序返回，每一行至多一次。snap 为
// nil 时一行都没有。一行先按 ID 形式比（idFormEffort），对不上再按模型名形式比 Model
// 与非空的 Alias。codex 的 SuggestedID("") 多半就是 Model，这时它的模型名按 ID 形式
// 对上，强度同样另取，结果相同。
func matchDiscoveredName(snap *modelSnapshot, name string) []nameMatch {
	if snap == nil {
		return nil
	}
	var out []nameMatch
	for _, row := range snap.rows {
		m := row.m
		if e, ok := idFormEffort(m, name); ok {
			out = append(out, nameMatch{row: row, byID: true, effort: e})
			continue
		}
		if strings.EqualFold(name, m.Model) || (m.Alias != "" && strings.EqualFold(name, m.Alias)) {
			out = append(out, nameMatch{row: row})
		}
	}
	return out
}

// idFormEffort 报告 name 是不是 m 的 ID 形式；是的话返回名字里带的那一档。有强度的行
// 先按 Efforts 的顺序逐档比 SuggestedID(档)，取第一个对上的；都对不上再比不带强度的
// SuggestedID("")，没有强度可选的行只比这一个。按 SuggestedID("") 对上时返回空串。
func idFormEffort(m provider.DiscoveredModel, name string) (string, bool) {
	for _, e := range m.Efforts {
		if strings.EqualFold(name, m.SuggestedID(e)) {
			return e, true
		}
	}
	return "", strings.EqualFold(name, m.SuggestedID(""))
}

// addNameArgs 是 --id 与 --effort 两个参数；have 为假时对应的值不看。
type addNameArgs struct {
	newID      string
	haveID     bool
	effort     string
	haveEffort bool
}

// cmdAddByName 是 cmdAdd 在开关打开时先走的一步。done 为假表示这条命令与按名字加无关
// （没有 --id / --effort，而且每一段都在配置里命中），cmdAdd 照 AD1 之前的做法接着解析，
// 行为一个字节不变。done 为真时这条命令已经处理完（加了、拒了、或识别被取消），err
// 只来自写屏幕与存储。
//
// 顺序冻结：
//  1. fields 里有恰好等于 --id 或 --effort 的词：参数形状（每个参数至多一次、后面要有值、
//     恰好一个名字，否则打用法）；名字是 + 加数字时打 +编号 的用法（+编号 只在第一个词
//     时才认）；名字在配置里命中一句拒绝就打那一句；按全名（或编号）命中一个 agent 就说
//     这两个参数只用于识别出的模型；只是前缀命中时先识别（第 3 步），名字在快照里对得上
//     就按识别结果加，对不上照样说那一句；没命中接第 3 步。
//  2. 没有：逐段在配置里解析（resolveAddConfig），全部命中返回 done 为假。命中与否只有
//     一个判据（miss），单段、多段、两个词三处都用它。
//  3. 本会话还没有快照、配置里又有能识别的 provider，就识别一次（ensureModelSnapshot）；
//     被取消只有「识别被取消」那一行，什么都不加。带参数的那一个名字接 addByOneName。
//  4. 两个词的写法：恰好两段，第二段没命中配置，第一段没命中配置或者在配置里只是前缀
//     命中（configPrefixOnly），第一段在快照里对上，而且第二段恰好是对上的某一行支持的
//     一档强度（区分大小写，几行取并集）：第二段当强度，接 addByOneName。第一段没命中
//     配置、对上的行里至少一行有强度可选、第二段在快照里也对不上任何行、不以 + 开头、
//     也不是在场成员或总结成员的前缀（addSegmentNotFound 打的是「都没有」那一句）时，
//     第二段多半是敲错的强度，同样接 addByOneName，由它打现成的说法（addDiscoveredModel
//     那句「支持的强度是」，对上多行时列候选，ID 形式已带强度时是「已经带着强度」），
//     不写配置。
//  5. 只有一段：接 addByOneName。
//  6. 其余多段：全或无，一个都不加，逐段按出现顺序打说法：命中配置的照配置的说法（加入
//     成功的那种不打），在快照里对上的打「识别出的模型一次加一个」，都对不上的打
//     addSegmentNotFound。
func cmdAddByName(ctx context.Context, r *REPL, fields, segs []string, candidates []string, active, member map[string]bool) (bool, error) {
	flagged := false
	for _, f := range fields {
		if f == "--id" || f == "--effort" {
			flagged = true
			break
		}
	}
	if flagged {
		name, a, ok := parseAddNameArgs(fields)
		if !ok {
			return true, r.printf("%s", r.t("tui.addName.usage", msgAddNameUsage))
		}
		if strings.HasPrefix(name, "+") && isASCIIDigits(name[1:]) {
			return true, r.printf("%s", r.t("tui.add.usage", msgAddModelUsage))
		}
		prefixID := ""
		if id, problem := resolveAddConfig(r, name, candidates, active, member); problem != "" {
			return true, r.printf("%s", problem)
		} else if id != "" {
			if !configPrefixOnly(name, id) {
				return true, r.printf("%s", r.t("tui.addName.flagsConfig", msgAddNameFlagsConfig, name, id, id))
			}
			prefixID = id
		}
		if canceled, err := snapshotForName(ctx, r); canceled || err != nil {
			return true, err
		}
		if prefixID != "" && len(matchDiscoveredName(r.modelSnap, name)) == 0 {
			return true, r.printf("%s", r.t("tui.addName.flagsConfig", msgAddNameFlagsConfig, name, prefixID, prefixID))
		}
		return true, addByOneName(ctx, r, name, "", a, active, member)
	}

	ids := make([]string, len(segs))
	problems := make([]string, len(segs))
	miss := func(i int) bool { return ids[i] == "" && problems[i] == "" }
	anyMiss := false
	for i, seg := range segs {
		ids[i], problems[i] = resolveAddConfig(r, seg, candidates, active, member)
		if miss(i) {
			anyMiss = true
		}
	}
	if !anyMiss {
		return false, nil
	}
	if canceled, err := snapshotForName(ctx, r); canceled || err != nil {
		return true, err
	}

	if len(segs) == 2 && miss(1) && (miss(0) || configPrefixOnly(segs[0], ids[0])) {
		ms := matchDiscoveredName(r.modelSnap, segs[0])
		typo := miss(0) && anyRowHasEfforts(ms) && len(matchDiscoveredName(r.modelSnap, segs[1])) == 0
		// 第二段以 + 开头，或是在场成员、总结成员的前缀时，它指的是 +编号 或配置里的人，
		// 不当敲错的强度，照多段说（W1b minor 1）。
		typo = typo && !strings.HasPrefix(segs[1], "+") && addSegmentNotFound(r, segs[1], active, member) == r.t("tui.addName.notFound", msgAddNameNotFound, segs[1])
		if effortOfAny(ms, segs[1]) || typo {
			return true, addByOneName(ctx, r, segs[0], segs[1], addNameArgs{}, active, member)
		}
	}
	if len(segs) == 1 {
		return true, addByOneName(ctx, r, segs[0], "", addNameArgs{}, active, member)
	}
	for i, seg := range segs {
		problem := problems[i]
		if miss(i) {
			if len(matchDiscoveredName(r.modelSnap, seg)) > 0 {
				problem = r.t("tui.addName.oneAtATime", msgAddNameOneAtATime, seg, seg)
			} else {
				problem = addSegmentNotFound(r, seg, active, member)
			}
		}
		if problem == "" {
			continue
		}
		if err := r.printf("%s", problem); err != nil {
			return true, err
		}
	}
	return true, nil
}

// configPrefixOnly 报告 seg 在配置里只是按前缀命中了 id（resolveAddConfig 的第 3 步给出了
// 唯一的候选）：不是全名（全名命中时 id 等于 seg）、不是编号（全是数字的段走第 2 步）。
// 总结成员、只是成员、前缀歧义与超出范围的编号都走 problem，id 是空串，不算。
func configPrefixOnly(seg, id string) bool {
	return id != "" && id != seg && !isASCIIDigits(seg)
}

// effortOfAny 报告 e 是不是 ms 里任意一行支持的一档（区分大小写，几行取并集）。
func effortOfAny(ms []nameMatch, e string) bool {
	for _, mt := range ms {
		if hasEffort(mt.row.m, e) {
			return true
		}
	}
	return false
}

// anyRowHasEfforts 报告 ms 里是否至少一行有强度可选（claude、codex 的多数行）。
func anyRowHasEfforts(ms []nameMatch) bool {
	for _, mt := range ms {
		if len(mt.row.m.Efforts) > 0 {
			return true
		}
	}
	return false
}

// parseAddNameArgs 从 fields 里取出 --id、--effort 与唯一的名字。每个参数至多一次、
// 后面要有值；其余的词都是名字，恰好一个，而且按 /add 的分隔符（splitAddArgs）切出来
// 也恰好一段（a,b --effort x 算两个名字）。
func parseAddNameArgs(fields []string) (string, addNameArgs, bool) {
	var a addNameArgs
	var names []string
	for i := 0; i < len(fields); i++ {
		switch fields[i] {
		case "--id":
			if a.haveID || i+1 >= len(fields) {
				return "", addNameArgs{}, false
			}
			a.haveID = true
			a.newID = fields[i+1]
			i++
		case "--effort":
			if a.haveEffort || i+1 >= len(fields) {
				return "", addNameArgs{}, false
			}
			a.haveEffort = true
			a.effort = fields[i+1]
			i++
		default:
			names = append(names, splitAddArgs(fields[i])...)
		}
	}
	if len(names) != 1 {
		return "", addNameArgs{}, false
	}
	return names[0], a, true
}

// snapshotForName 是按名字加用的「没有快照就识别一次」：配置里一家能识别的都没有时
// 不识别（快照留 nil，按名字一个都对不上）。已有快照直接用，不重拍。
func snapshotForName(ctx context.Context, r *REPL) (bool, error) {
	if r.modelSnap != nil {
		return false, nil
	}
	pids := discoverableProviders(r.cfg)
	if len(pids) == 0 {
		return false, nil
	}
	return ensureModelSnapshot(ctx, r, pids)
}

// hasEffort 报告 e 是不是 m 支持的一档（区分大小写）。
func hasEffort(m provider.DiscoveredModel, e string) bool {
	for _, x := range m.Efforts {
		if x == e {
			return true
		}
	}
	return false
}

// addByOneName 按一个名字加识别出的模型。second 是两个词写法的第二个词（一档强度，或
// cmdAddByName 第 4 步认定的敲错的强度），没有时是空串。顺序冻结：在快照里找（一行都
// 没有打 addSegmentNotFound；多行整条拒绝、逐行列出候选），没给强度也没给 --id 时先看
// 配置里这个模型已有的成员（addNameExisting，有就不写配置），名字里带的强度与 --effort
// 或第二个词对账，Runner 在不在，然后交给 addDiscoveredModel（强度、同脑子、id、配置
// 文件路径、守卫、追加、加载校验、换指针、Join）。第二个词不是那一行的一档强度时，由
// addDiscoveredModel 的强度那一步拒绝，不写配置。
//
// ID 形式带了强度时：给了不同的 --effort 报错；给了相同的照常；后面又跟了第二个词，
// 相同也报错（两个词的写法只给模型名形式与不带强度的 ID 形式用）。
func addByOneName(ctx context.Context, r *REPL, name, second string, a addNameArgs, active, member map[string]bool) error {
	ms := matchDiscoveredName(r.modelSnap, name)
	switch {
	case len(ms) == 0:
		return r.printf("%s", addSegmentNotFound(r, name, active, member))
	case len(ms) > 1:
		if err := r.printf("%s", r.t("tui.addName.ambiguous", msgAddNameAmbiguous, name, len(ms))); err != nil {
			return err
		}
		for _, mt := range ms {
			no := ""
			if mt.row.no > 0 {
				no = fmt.Sprintf("  +%d", mt.row.no)
			}
			if err := r.printf("  %-6s  %s%s\n", mt.row.m.Provider, mt.row.m.Model, no); err != nil {
				return err
			}
		}
		return nil
	}
	mt := ms[0]
	if second == "" && !a.haveEffort && !a.haveID && !(mt.byID && mt.effort != "") {
		if done, err := addNameExisting(ctx, r, name, mt.row.m, active); done {
			return err
		}
	}
	effort, haveEffort := a.effort, a.haveEffort
	if mt.byID && mt.effort != "" {
		if second != "" {
			return r.printf("%s", r.t("tui.addName.effortTwice", msgAddNameEffortTwice, name, mt.effort, second))
		}
		if haveEffort && effort != mt.effort {
			return r.printf("%s", r.t("tui.addName.effortTwice", msgAddNameEffortTwice, name, mt.effort, effort))
		}
		effort, haveEffort = mt.effort, true
	} else if second != "" {
		effort, haveEffort = second, true
	}
	if r.rn == nil {
		return r.printf("%s\n", r.t("tui.repl.noRunner", msgNoRunner))
	}
	return addDiscoveredModel(ctx, r, mt.row.m, effort, haveEffort, a.newID, a.haveID, name)
}
