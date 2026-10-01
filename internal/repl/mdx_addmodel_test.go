package repl

// mdx_addmodel_test.go 是 MD1（模型识别 + /add +编号 追加写 config.toml）在 repl 包的
// 验收用例，对应冻结契约 md1_contract.md v1.1 §2.4 的 R1-R16。装置、夹具与期望块
// 在 mdx_addmodel_fixture_test.go。
//
// 全部用例走真 SQLite（store.Open 到 t.TempDir()）、真配置文件（config.LoadAndValidate）、
// 真 turn.Runner；识别用 §2.0 的假执行器，不起任何真 provider CLI。R2 第 5 步经 /ask
// 让真 Runner 起一次本包 TestMain 扮演的假 claude（COUNCIL_FAKE_CLI=1）。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/provider"
	"council/internal/turn"
)

// TestMdxNewWiresDiscovery 是 R10：New 装上生产执行器与 20 s 超时，快照为空。
// 【判别力】New 里 discover 写成 nil（M68）或 discoverTimeout 换成别的值（M69）时各红一处；
// 别的 repl 用例都在第一次 dispatch 之前手动覆盖了这两个字段，只有这一条看得见 New 的装配。
func TestMdxNewWiresDiscovery(t *testing.T) {
	r := New(nil, nil, nil, nil, nil, RulesContext{})
	if r.discover == nil {
		t.Errorf("r.discover 是 nil，应当是 turn.RunDiscovery")
	} else if reflect.ValueOf(r.discover).Pointer() != reflect.ValueOf(turn.RunDiscovery).Pointer() {
		t.Errorf("r.discover 不是 turn.RunDiscovery")
	}
	if r.discoverTimeout != 20*time.Second {
		t.Errorf("r.discoverTimeout = %v，want 20s", r.discoverTimeout)
	}
	if r.modelSnap != nil {
		t.Errorf("r.modelSnap 应当是 nil")
	}
}

// TestMdxAddListsDiscovered 是 R1：不带参数的 /add 先打配置候选，再打识别出、配置里还没有的模型。
// 【判别力】整段 27 行逐字比：配置里 opus 写的是 opus[1m]，经 Alias 藏起 claude-opus-5-5[1m]
// （不比 Alias，M47，会多出一行、编号后移）；+11 agy agy-sonnet-4.6-thinking 在表里，它的 Alias
// 等于 claude 成员 sonnet-4.6 的 model，不比 provider（M48）会把它藏起来；编号从 +1 连续到 +23，
// 拍快照时把已配置的也编号（M72）会从 +2 起。解析层（M1-M12、M22）的变异也都让这一段变。
// 对照是 R12（只差 opus 的 model 一处）。
func TestMdxAddListsDiscovered(t *testing.T) {
	g := mdxNewRig(t, mdxConfigTemplate)
	lines := g.run("/add")
	if len(lines) < 7 {
		t.Fatalf("输出不足 7 行：%q", lines)
	}
	if lines[0] != mdxCandHeader {
		t.Errorf("第 1 行应当是既有候选表头，got %q", lines[0])
	}
	if lines[6] != mdxCandHint {
		t.Errorf("第 7 行应当是既有候选提示行，got %q", lines[6])
	}
	// 这 27 行之后没有别的行（整组相等同时担保「没有别的非空行」）。
	mdxWantLines(t, "R1 识别部分", lines[7:], mdxSplit(mdxR1Block))

	calls := g.exec.callsCopy()
	if len(calls) != 3 {
		t.Fatalf("假执行器应当恰好被调 3 次，实际 %d 次", len(calls))
	}
	wantOrder := []struct{ pid, path string }{
		{"agy", mdxAgyPath},
		{"claude", os.Args[0]},
		{"grok", mdxGrokPath},
	}
	for i, w := range wantOrder {
		c := calls[i]
		if c.Path != w.path {
			t.Errorf("第 %d 次调用 Path = %q，want %q", i+1, c.Path, w.path)
		}
		want, err := provider.DiscoveryCommandFor(w.pid, w.path)
		if err != nil {
			t.Fatalf("DiscoveryCommandFor(%q): %v", w.pid, err)
		}
		if !reflect.DeepEqual(c.Args, want.Args) {
			t.Errorf("第 %d 次调用 Args = %q，want %q", i+1, c.Args, want.Args)
		}
		if !reflect.DeepEqual(c.Stdin, want.Stdin) {
			t.Errorf("第 %d 次调用 Stdin = %q，want %q", i+1, c.Stdin, want.Stdin)
		}
	}
	if g.r.modelSnap == nil {
		t.Errorf("识别之后 r.modelSnap 不应当是 nil")
	}
}

// TestMdxAddPlusWritesConfigAndJoins 是 R2：/add +14 把 sonnet-5-high 追加进配置、换指针、加入会话。
// 【判别力】只换 r.cfg 不换 Runner（M55）时第 5 步是 outcome refused；只换 Runner（M56）时
// 第 3 步 +14 行还在、第 4 步没有新 agent 那一行；不 Join（M57）时第 2 步库里没有成员；
// 显示时重新编号（M49）时第 3 步 +15 变 +14；显示时不按当前配置过滤（M71）时 +14 行还在；
// 每次都重新识别（M50、M67）时第 3 步有「正在识别模型」行、调用次数变 6。追加段逐字比，
// 表名加不加引号（M29）、分隔空行（M34）、role 行（M73）、权限（M74）、display_name（M75）都在里面。
func TestMdxAddPlusWritesConfigAndJoins(t *testing.T) {
	g := mdxNewRig(t, mdxConfigTemplate)
	orig := g.readFile()
	cfg0 := g.r.cfg

	// 第 1 步。
	g.run("/add")

	// 第 2 步。
	baks := g.bakNames()
	lines := g.run("/add +14")
	bak := g.newBak(baks)
	mdxWantLines(t, "R2 第 2 步输出", lines, g.joinedLines("sonnet-5-high", bak))

	idx := strings.Index(mdxAppendAll, "mode = \"discussion\"\n")
	if idx <= 0 {
		t.Fatalf("前提：§3.5 里找不到第一段的结尾")
	}
	seg1 := mdxAppendAll[:idx+len("mode = \"discussion\"\n")]
	if got, want := string(g.readFile()), string(orig)+seg1; got != want {
		t.Errorf("配置文件字节不等\ngot:\n%q\nwant:\n%q", got, want)
	}
	if b, err := os.ReadFile(bak); err != nil || !reflect.DeepEqual(b, orig) {
		t.Errorf("备份文件应当等于原文，err=%v", err)
	}
	if got, want := g.dirNames(), []string{"config.toml", "data", filepath.Base(bak)}; !reflect.DeepEqual(got, mdxSorted(want)) {
		t.Errorf("dir 里的条目 = %q，want %q", got, mdxSorted(want))
	}
	if g.r.cfg == cfg0 {
		t.Errorf("r.cfg 指针应当换了")
	}
	reloaded, err := config.LoadAndValidate(g.cfgPath)
	if err != nil {
		t.Fatalf("写后的配置应当能加载: %v", err)
	}
	if !reflect.DeepEqual(g.r.cfg, reloaded) {
		t.Errorf("r.cfg 与重新加载的配置不等")
	}
	wantAgent := config.AgentConfig{
		DisplayName: "Sonnet 5",
		Provider:    "claude",
		Model:       "claude-sonnet-5",
		Effort:      "high",
		Role:        "",
		Permissions: config.PermissionRead,
		Mode:        config.ModeDiscussion,
	}
	if got := g.r.cfg.Agents["sonnet-5-high"]; !reflect.DeepEqual(got, wantAgent) {
		t.Errorf("新 agent = %+v，want %+v", got, wantAgent)
	}
	if len(g.r.cfg.Agents) != len(cfg0.Agents)+1 {
		t.Errorf("agent 数 %d，want %d", len(g.r.cfg.Agents), len(cfg0.Agents)+1)
	}
	for id, a := range cfg0.Agents {
		if !reflect.DeepEqual(g.r.cfg.Agents[id], a) {
			t.Errorf("已有 agent %s 被改了", id)
		}
	}
	ps := g.participants()
	if len(ps) != 1 {
		t.Fatalf("库里应当恰好一个成员，实际 %d", len(ps))
	}
	if p := ps[0]; p.AgentID != "sonnet-5-high" || p.DisplayName != "Sonnet 5" || p.Provider != "claude" || !p.Active {
		t.Errorf("成员行 = %+v", p)
	}

	// 第 3 步。
	lines = g.run("/add")
	for _, l := range lines {
		if strings.HasPrefix(l, "正在识别模型") {
			t.Errorf("有快照时不应当再识别：%q", l)
		}
	}
	mdxWantLines(t, "R2 第 3 步识别部分", g.afterCandidates(lines), mdxSplit(mdxR2Step3Block))
	if n := g.exec.count(); n != 3 {
		t.Errorf("假执行器调用次数 %d，want 3", n)
	}

	// 第 4 步。
	lines = g.run("/agents")
	if want := "sonnet-5-high   claude    claude-sonnet-5          high    read         discussion"; !mdxHasLine(lines, want) {
		t.Errorf("/agents 里没有整行 %q：\n%s", want, strings.Join(lines, "\n"))
	}

	// 第 5 步。
	g.run("hello")
	lines = g.run("/ask sonnet-5-high")
	if want := "outcome spoke  agent sonnet-5-high"; !mdxHasLine(lines, want) {
		t.Errorf("/ask 输出里没有整行 %q：\n%s", want, strings.Join(lines, "\n"))
	}
}

// mdxSorted 返回排好序的副本。
func mdxSorted(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

// TestMdxAddEffortAndDuplicate 是 R3：同一模型换强度是新 id；同一模型同一强度拒绝；
// 默认强度、没有强度、--id、--effort 与 --id 并用。
// 【判别力】第 3 步与第 2 步只差 --effort：同脑子判据不看强度（M60）时第 2 步就被拒；
// 去掉同脑子判据（M61）时第 3 步变成「配置里已经有 agent sonnet-5-high」那一句。
// 第 4 步钉「不支持 high 时取第一档」（M7），第 5 步钉「没有强度不写 effort 行」（M31）与日期段（M8）；
// 带点号的 id 不加引号（M30）会让第 4 步写出嵌套表、加载失败被恢复。
func TestMdxAddEffortAndDuplicate(t *testing.T) {
	g := mdxNewRig(t, mdxConfigTemplate)
	orig := g.readFile()
	g.run("/add")

	type step struct {
		cmd string
		id  string // 空串表示这一步应当被拒
		msg string // 被拒时的那一行
	}
	steps := []step{
		{cmd: "/add +14", id: "sonnet-5-high"},
		{cmd: "/add +14 --effort max", id: "sonnet-5-max"},
		{cmd: "/add +14", msg: "配置里已经有同一个模型、同一强度的 agent sonnet-5-high，直接 /add sonnet-5-high"},
		{cmd: "/add +17", id: "mdx-lowmed-2.0-low"},
		{cmd: "/add +16", id: "haiku-4.5"},
		{cmd: "/add +19 --id g2", id: "g2"},
		{cmd: "/add +18 --effort max --id mdx-plain-max", id: "mdx-plain-max"},
	}
	var bakPaths []string
	var bakWant [][]byte
	for i, s := range steps {
		before := g.readFile()
		baks := g.bakNames()
		lines := g.run(s.cmd)
		if s.id == "" {
			mdxWantLines(t, "R3 第 "+strconv.Itoa(i+1)+" 步输出", lines, []string{s.msg})
			if got := g.bakNames(); !reflect.DeepEqual(got, baks) {
				t.Errorf("第 %d 步被拒却多了备份：%q", i+1, got)
			}
			if !reflect.DeepEqual(g.readFile(), before) {
				t.Errorf("第 %d 步被拒却改了配置文件", i+1)
			}
			continue
		}
		bak := g.newBak(baks)
		mdxWantLines(t, "R3 第 "+strconv.Itoa(i+1)+" 步输出", lines, g.joinedLines(s.id, bak))
		bakPaths = append(bakPaths, bak)
		bakWant = append(bakWant, before)
	}

	if got, want := string(g.readFile()), string(orig)+mdxAppendAll; got != want {
		t.Errorf("配置文件字节不等\ngot:\n%q\nwant:\n%q", got, want)
	}
	if n := len(g.bakNames()); n != 6 {
		t.Errorf("备份文件 %d 个，want 6", n)
	}
	for k, p := range bakPaths {
		b, err := os.ReadFile(p)
		if err != nil || !reflect.DeepEqual(b, bakWant[k]) {
			t.Errorf("第 %d 个备份 %s 的字节不等于第 %d 次追加之前的文件（err=%v）", k+1, p, k+1, err)
		}
	}
	want := []string{"g2", "haiku-4.5", "mdx-lowmed-2.0-low", "mdx-plain-max", "sonnet-5-high", "sonnet-5-max"}
	if got := g.activeIDs(); !reflect.DeepEqual(got, want) {
		t.Errorf("在场成员 = %q，want %q", got, want)
	}
}

// TestMdxAddRefusals 是 R4：各种拒绝各打恰好一行，全部拒绝之后文件、备份、库、指针都不动。
// 【判别力】第一步（有能识别的 provider、只是还没识别）与 R11 (b)（一家能识别的都没有）是一对：
// 那一支无条件成立（M101）时第一步变成「没有能识别模型的 provider」。表里每一行各钉一道检查：
// --refresh 带多余参数（M70）、--effort 不在支持档（M59）、设备名（M39、M40）、折叠撞名（M41、M42）、
// 已存在（M43，文本变成折叠那一句）、保留名（M44，写进去后被 Validate 拒、走恢复，打的不是这一行）。
func TestMdxAddRefusals(t *testing.T) {
	g := mdxNewRig(t, mdxConfigTemplate)
	orig := g.readFile()
	cfg0 := g.r.cfg

	mdxWantLines(t, "R4 /add +1（识别之前）", g.run("/add +1"),
		[]string{"还没有识别结果：先敲 /add 列出识别出的模型，再用 /add +编号 加入"})
	g.run("/add")

	pairs := strings.Split(mdxR4Pairs, "\n\n")
	if len(pairs) != 22 {
		t.Fatalf("前提：R4 的表应当有 22 条，解析出 %d 条", len(pairs))
	}
	for _, p := range pairs {
		kv := strings.Split(p, "\n")
		if len(kv) != 2 {
			t.Fatalf("前提：R4 表里每条是两行，got %q", p)
		}
		mdxWantLines(t, "R4 "+kv[0], g.run(kv[0]), []string{kv[1]})
	}

	if !reflect.DeepEqual(g.readFile(), orig) {
		t.Errorf("配置文件被改了")
	}
	if b := g.bakNames(); len(b) != 0 {
		t.Errorf("不应当有备份：%q", b)
	}
	if ps := g.participants(); len(ps) != 0 {
		t.Errorf("库里不应当有成员：%+v", ps)
	}
	if g.r.cfg != cfg0 {
		t.Errorf("r.cfg 指针不应当变")
	}
	if n := g.exec.count(); n != 3 {
		t.Errorf("假执行器调用次数 %d，want 3", n)
	}
}

// TestMdxAddSnapshotStable 是 R5：两次 /add 之间不重新识别，--refresh 才重拍。
// 【判别力】每次都重新识别（M50）时第二次调用次数是 6；有快照也打「正在识别模型」（M67）时
// 第三次的识别部分多出那一行；--refresh 不清快照（M51）时最后一步没有「正在识别模型」、调用次数停在 3。
func TestMdxAddSnapshotStable(t *testing.T) {
	g := mdxNewRig(t, mdxConfigTemplate)
	g.run("/add")
	if n := g.exec.count(); n != 3 {
		t.Errorf("第一次 /add 之后调用次数 %d，want 3", n)
	}
	g.run("/add")
	if n := g.exec.count(); n != 3 {
		t.Errorf("第二次 /add 之后调用次数 %d，want 3", n)
	}
	g.exec.set(mdxGrokPath, mdxResp{out: []byte("Available models:\n  * grok-4.7 (default)\n  - grok-9.9\n")})
	lines := g.run("/add")
	if n := g.exec.count(); n != 3 {
		t.Errorf("第三次 /add 之后调用次数 %d，want 3", n)
	}
	r1 := mdxSplit(mdxR1Block)
	mdxWantLines(t, "R5 第三次识别部分", g.afterCandidates(lines), r1[1:])

	lines = g.run("/add --refresh")
	if n := g.exec.count(); n != 6 {
		t.Errorf("--refresh 之后调用次数 %d，want 6", n)
	}
	if !mdxHasLine(lines, "正在识别模型: agy, claude, grok") {
		t.Errorf("--refresh 应当重新识别：\n%s", strings.Join(lines, "\n"))
	}
	row19 := "+19  grok      grok-9.9                 grok-9.9                      -                          本地配置"
	if !mdxHasLine(lines, row19) {
		t.Errorf("--refresh 之后没有整行 %q：\n%s", row19, strings.Join(lines, "\n"))
	}
	for _, l := range lines {
		if strings.Contains(l, "grok-4.6") {
			t.Errorf("--refresh 之后不应当还有 grok-4.6：%q", l)
		}
	}
	// +1 到 +18 那 18 行与 R1 相同：R1 块的第 4-21 行。
	part := g.afterCandidates(lines)
	if len(part) < 21 {
		t.Fatalf("--refresh 之后识别部分不足 21 行：%q", part)
	}
	mdxWantLines(t, "R5 --refresh 的 +1..+18", part[3:21], r1[3:21])
}

// TestMdxAddDiscoveryFailures 是 R6：四种失败组合，失败行与 --refresh 提示随快照一起重印。
//
// 注意：each_kind 里超时的那一家（claude）不能排在最后，它后面必须还有一家（grok）。
// provider 按字典序识别，改夹具时若让超时的那一家落到最后，「三家共用一个 deadline」（M77）
// 就没有可观测的后果，这条对照会悄悄失效。
//
// 【判别力】one_down 是「一家失败不拦别家」与「失败行在表后照样打」（M66）的对照；each_kind 的
// claude 行防超时不带进 ctx（M64，假执行器 3 s 后报 mdx: ctx never cancelled）与超时文案（M65）；
// grok 行防三家共用一个 deadline（M77：grok 那一行会变成超时）。四段末尾的 --refresh 提示行
// 防失败被缓存却不提示（M86），对照是 R1（没有失败行，也没有提示行）。第二次 /add 让 M50、M67 也红。
// unparseable 的 grok 行钉「有 Available models: 算查不到」（M15）；映射成超时（M18）三段都红。
func TestMdxAddDiscoveryFailures(t *testing.T) {
	cases := []struct {
		name  string
		setup func(e *mdxExec)
		want  string
	}{
		{"each_kind", func(e *mdxExec) {
			e.set(mdxAgyPath, mdxResp{out: []byte(`{"status":"SUCCESS","command":{"name":"models","data":{"models":[]}}}`)})
			e.set(os.Args[0], mdxResp{timeout: true})
			e.set(mdxGrokPath, mdxResp{err: errors.New("mdx boom")})
		}, mdxR6EachKind},
		{"unparseable", func(e *mdxExec) {
			e.set(mdxAgyPath, mdxResp{out: []byte("not json")})
			e.set(os.Args[0], mdxResp{out: []byte("{\"type\":\"result\"}\n")})
			e.set(mdxGrokPath, mdxResp{out: []byte("Available models:\n")})
		}, mdxR6Unparseable},
		{"all_down", func(e *mdxExec) {
			e.set(mdxAgyPath, mdxResp{err: errors.New("mdx agy down")})
			e.set(os.Args[0], mdxResp{err: errors.New("mdx claude down")})
			e.set(mdxGrokPath, mdxResp{err: errors.New("mdx grok down")})
		}, mdxR6AllDown},
		{"one_down", func(e *mdxExec) {
			e.set(mdxGrokPath, mdxResp{err: errors.New("mdx grok down")})
		}, mdxR6OneDown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := mdxNewRig(t, mdxConfigTemplate)
			c.setup(g.exec)
			want := mdxSplit(c.want)
			mdxWantLines(t, "第一次 /add", g.afterCandidates(g.run("/add")), want)
			if n := g.exec.count(); n != 3 {
				t.Errorf("第一次 /add 之后调用次数 %d，want 3", n)
			}
			mdxWantLines(t, "第二次 /add", g.afterCandidates(g.run("/add")), want[1:])
			if n := g.exec.count(); n != 3 {
				t.Errorf("第二次 /add 之后调用次数 %d，want 3", n)
			}
		})
	}
}

// TestMdxAddGuard 是 R7：写之前按 config.Load 重读、与 r.cfg 比；六个子测试各只动 Config 的一个字段。
// 【判别力】前缀里「只加注释」那一步是「按加载结果比、不按字节或 mtime 比」的对照（它必须照常加入）。
// 去掉守卫（M52）：agent、store_thought、teams、providers 红；只比 Agents（M78）：store_thought、teams、
// providers 红；只比 Agents 与 Providers（M79）：store_thought、teams 红；抹平 Teams（M103）只红 teams，
// 抹平 Providers（M104）只红 providers；只比 Agents 与 StoreThought（M109）：teams、providers 红；
// 去掉 data_dir / rules_file 那一支（M80）或两支对调（M81）：data_dir、rules_file 红；
// 那一支只判 data_dir（M82）：只红 rules_file。
func TestMdxAddGuard(t *testing.T) {
	const reloadLine = "配置文件在上次加载之后被改过，先 /config reload --apply 让它生效，再 /add +15"
	const restartLine = "配置文件里改了 data_dir 或 rules_file，这两个键只能重启生效：重启 council 之后先敲 /add 看编号，再用 /add +编号 加入"
	cases := []struct {
		name string
		edit func(t *testing.T, base, dataLine, dir string) string
		want string
	}{
		{"agent", func(t *testing.T, base, dataLine, dir string) string {
			return base + "\n[agents.zzz]\nprovider = 'claude'\nmodel = 'm'\n"
		}, reloadLine},
		{"store_thought", func(t *testing.T, base, dataLine, dir string) string {
			return mdxReplaceOnce(t, base, dataLine, dataLine+"store_thought = false\n")
		}, reloadLine},
		{"data_dir", func(t *testing.T, base, dataLine, dir string) string {
			return mdxReplaceOnce(t, base, dataLine, "data_dir = '"+filepath.Join(dir, "data2")+"'\n")
		}, restartLine},
		{"rules_file", func(t *testing.T, base, dataLine, dir string) string {
			return mdxReplaceOnce(t, base, dataLine, dataLine+"rules_file = 'mdx-rules.md'\n")
		}, restartLine},
		{"teams", func(t *testing.T, base, dataLine, dir string) string {
			return mdxReplaceOnce(t, base, "members = ['opus-5.0', 'grok-4.7']", "members = ['opus-5.0']")
		}, reloadLine},
		{"providers", func(t *testing.T, base, dataLine, dir string) string {
			return mdxReplaceOnce(t, base, "command = 'mdx-grok.exe'", "command = 'mdx-grok2.exe'")
		}, reloadLine},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := mdxNewRig(t, mdxConfigTemplate)
			g.run("/add")
			g.writeFile(append(g.readFile(), []byte("\n# edited\n")...))
			baks := g.bakNames()
			lines := g.run("/add +14")
			bak := g.newBak(baks)
			mdxWantLines(t, "前缀 /add +14（只改过注释）", lines, g.joinedLines("sonnet-5-high", bak))

			base := string(g.readFile())
			cfg1 := g.r.cfg
			dataLine := "data_dir = '" + filepath.Join(g.dir, "data") + "'\n"
			if n := strings.Count(base, dataLine); n != 1 {
				t.Fatalf("前提：dataLine 应当在 base 里恰好一处，实际 %d", n)
			}
			edited := c.edit(t, base, dataLine, g.dir)
			if edited == base {
				t.Fatalf("前提：改完之后的文本应当不等于 base")
			}
			g.writeFile([]byte(edited))

			mdxWantLines(t, "/add +15", g.run("/add +15"), []string{c.want})
			if got := string(g.readFile()); got != edited {
				t.Errorf("被拒之后配置文件变了")
			}
			if b := g.bakNames(); len(b) != 1 {
				t.Errorf("备份文件应当恰好 1 个（前缀那一次），实际 %q", b)
			}
			if g.r.cfg != cfg1 {
				t.Errorf("r.cfg 指针不应当变")
			}
			if got := g.activeIDs(); !reflect.DeepEqual(got, []string{"sonnet-5-high"}) {
				t.Errorf("在场成员 = %q，want [sonnet-5-high]", got)
			}
		})
	}
}

// mdxGrok47Section 是 §2.0 配置里 grok-4.7 那一段连同它后面的空行（R8 的变体要删掉它）。
const mdxGrok47Section = "[agents.\"grok-4.7\"]\ndisplay_name = 'Grok 4.7'\nprovider     = 'grok'\nmodel        = 'grok-4.7'\nrole         = 'r'\npermissions  = 'read'\nmode         = 'discussion'\n\n"

// TestMdxAddRestoresOnInvalidConfig 是 R8：写后加载校验不过，写回原字节，屏幕带备份路径。
// 【判别力】不恢复（M54）时文件多出 grok-4.6 那一段。这条也是 grok-4.7 行在 repl 屏幕上可见的
// 唯一用例：` (default)` 不剥（M13）或不认 `  * `（M14）时 grok-4.7 消失，+19 / +20 整体前移。
func TestMdxAddRestoresOnInvalidConfig(t *testing.T) {
	variant := mdxReplaceOnce(t, mdxConfigTemplate, mdxGrok47Section, "")
	variant = mdxReplaceOnce(t, variant, "members = ['opus-5.0', 'grok-4.7']", "members = ['opus-5.0']")
	g := mdxNewRig(t, variant)

	part := g.afterCandidates(g.run("/add"))
	rows := map[string][]string{}
	for _, l := range part {
		f := strings.Fields(l)
		if len(f) > 0 && strings.HasPrefix(f[0], "+") {
			rows[f[0]] = f
		}
	}
	if got, want := rows["+19"], []string{"+19", "grok", "grok-4.7", "grok-4.7", "-", "本地配置"}; !reflect.DeepEqual(got, want) {
		t.Errorf("+19 行的各列 = %q，want %q", got, want)
	}
	if got, want := rows["+20"], []string{"+20", "grok", "grok-4.6", "grok-4.6", "-", "本地配置"}; !reflect.DeepEqual(got, want) {
		t.Errorf("+20 行的各列 = %q，want %q", got, want)
	}

	noGrok := mdxReplaceOnce(t, string(g.readFile()), "[providers.grok]\ncommand = 'mdx-grok.exe'\n\n", "")
	g.writeFile([]byte(noGrok))
	if lines := g.run("/config reload --apply"); !mdxHasLine(lines, mdxAppliedLine) {
		t.Fatalf("前提：reload 应当应用新配置：\n%s", strings.Join(lines, "\n"))
	}
	cfg1 := g.r.cfg

	baks := g.bakNames()
	lines := g.run("/add +20")
	bak := g.newBak(baks)
	mdxWantLines(t, "R8 /add +20", lines, []string{
		"写入后的配置没通过加载或校验，已恢复原文件（备份在 " + bak + "）: agents.grok-4.6.provider: agent \"grok-4.6\" references undefined provider \"grok\"",
	})
	if got := string(g.readFile()); got != noGrok {
		t.Errorf("配置文件应当被恢复成 noGrok\ngot:\n%q", got)
	}
	if b, err := os.ReadFile(bak); err != nil || string(b) != noGrok {
		t.Errorf("备份应当等于 noGrok（err=%v）", err)
	}
	if g.r.cfg != cfg1 {
		t.Errorf("r.cfg 指针不应当变")
	}
	if ps := g.participants(); len(ps) != 0 {
		t.Errorf("库里不应当有成员：%+v", ps)
	}
}

// TestMdxAddNoRunnerNoPath 是 R9：没有 Runner、不知道配置文件路径时各打一行，什么都不动。
// 【判别力】去掉 Runner 判据（M62）时 (a) 在 r.rn.SetConfig 上 panic（runCtx 接住判红，只挂 (a)）；
// 去掉路径判据（M63）时 (b) 变成「读配置文件失败: …」。
func TestMdxAddNoRunnerNoPath(t *testing.T) {
	cases := []struct {
		name string
		opts mdxRigOpts
		want string
	}{
		{"no_runner", mdxRigOpts{noRunner: true}, "provider 调用未就绪，本命令不可用"},
		{"no_path", mdxRigOpts{noPath: true}, "不知道配置文件在哪，没法写入"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := mdxNewRigOpts(t, mdxConfigTemplate, c.opts)
			orig := g.readFile()
			g.run("/add")
			mdxWantLines(t, "/add +14", g.run("/add +14"), []string{c.want})
			if !reflect.DeepEqual(g.readFile(), orig) {
				t.Errorf("配置文件被改了")
			}
			if b := g.bakNames(); len(b) != 0 {
				t.Errorf("不应当有备份：%q", b)
			}
			if ps := g.participants(); len(ps) != 0 {
				t.Errorf("库里不应当有成员：%+v", ps)
			}
		})
	}
}

// R11 (a)、(b) 的配置（契约只写了「两个 provider、没有 agent」与「只有 pv_mdx」，文本由测试腿按
// §2.0 的体例写）。
const (
	mdxClaudePvTemplate = "data_dir = '<dataDir>'\n\n[providers.claude]\ncommand = '<exe>'\n\n[providers.pv_mdx]\ncommand = 'mdx-pv.exe'\n"
	mdxPvOnlyTemplate   = "data_dir = '<dataDir>'\n\n[providers.pv_mdx]\ncommand = 'mdx-pv.exe'\n"
)

// TestMdxAddSkipsUndiscoverableProviders 是 R11。
// 【判别力】(a) 不跳过没有适配器的 provider（M46）时首行是「正在识别模型: claude, pv_mdx」。
// (b) 的 /add +1 与 R4 第一步是一对：去掉「没有能识别的 provider」那一支（M100）时回到「还没有识别结果」。
// (c) 与 R6 all_down 是一对：(c) 有一家识别成功（okCount > 0），打「都已经在配置里了」；
// all_down 三家全失败，不打这一行。去掉这一支（M90）时 (c) 只剩第一行。
func TestMdxAddSkipsUndiscoverableProviders(t *testing.T) {
	t.Run("a", func(t *testing.T) {
		g := mdxNewRig(t, mdxClaudePvTemplate)
		mdxWantLines(t, "(a) /add", g.run("/add"), mdxSplit(mdxR11aBlock))
		calls := g.exec.callsCopy()
		if len(calls) != 1 || calls[0].Path != os.Args[0] {
			t.Errorf("执行器应当恰好被调 1 次、Path 是测试二进制，实际 %d 次 %+v", len(calls), calls)
		}
	})
	t.Run("b", func(t *testing.T) {
		g := mdxNewRig(t, mdxPvOnlyTemplate)
		mdxWantLines(t, "(b) /add", g.run("/add"), []string{"配置里还没有可以 /add 的成员"})
		if n := g.exec.count(); n != 0 {
			t.Errorf("执行器调用次数 %d，want 0", n)
		}
		mdxWantLines(t, "(b) /add +1", g.run("/add +1"), []string{"配置里没有能识别模型的 provider，/add +编号 用不了"})
	})
	t.Run("c", func(t *testing.T) {
		g := mdxNewRig(t, mdxGrokOnlyTemplate)
		mdxWantLines(t, "(c) 识别部分", g.afterCandidates(g.run("/add")), mdxSplit(mdxR11cBlock))
		calls := g.exec.callsCopy()
		if len(calls) != 1 || calls[0].Path != mdxGrokPath {
			t.Errorf("执行器应当恰好被调 1 次、Path 是 %s，实际 %d 次 %+v", mdxGrokPath, len(calls), calls)
		}
	})
}

// TestMdxAddAliasControl 是 R12：配置里 opus 写成别名 opus 时，claude-opus-5-5[1m] 那一行出现。
// 【判别力】与 R1 只差 opus 的 model 一处：opus 既不等于 value（opus[1m]）也不等于 resolvedModel，
// 于是 +14 claude opus-5.5-1m-high 出现、其后编号整体后移一位。它证明 R1 的「藏起来」靠的是 Alias。
func TestMdxAddAliasControl(t *testing.T) {
	variant := mdxReplaceOnce(t, mdxConfigTemplate, "model        = 'opus[1m]'", "model        = 'opus'")
	g := mdxNewRig(t, variant)
	mdxWantLines(t, "R12 识别部分", g.afterCandidates(g.run("/add")), mdxSplit(mdxR12Block))
}

// mdxFable51Section 是 §2.0 配置里 fable-5.1 那一段连同它后面的空行（R13 deleted 要删掉它）。
const mdxFable51Section = "[agents.\"fable-5.1\"]\ndisplay_name = 'Fable 5.1'\nprovider     = 'claude'\nmodel        = 'claude-fable-5-1'\neffort       = 'high'\nrole         = 'r'\npermissions  = 'read'\nmode         = 'discussion'\n\n"

// TestMdxAddSnapshotAcrossReload 是 R13：快照拍完之后配置被 /config reload --apply 换掉的三条分支。
// 【判别力】alias：sameBrainAgent 去掉 Alias 判据（M87）时 /add +14 加进一个同脑子的 sonnet-5-high；
// deleted：整块比对（不写缺席断言，reload 若悄悄失败缺席断言照样过），显示时不看 row.no > 0（M88）
// 时多出一行 +0 fable；provider：去掉 provider 判据（M89）时 agy 的 +11 被拒成同脑子 sonnet-4.6。
func TestMdxAddSnapshotAcrossReload(t *testing.T) {
	reload := func(g *mdxRig) {
		g.t.Helper()
		if lines := g.run("/config reload --apply"); !mdxHasLine(lines, mdxAppliedLine) {
			g.t.Fatalf("前提：reload 应当应用新配置：\n%s", strings.Join(lines, "\n"))
		}
	}
	t.Run("alias", func(t *testing.T) {
		g := mdxNewRig(t, mdxConfigTemplate)
		g.run("/add")
		g.writeFile(append(g.readFile(), []byte("\n[agents.snt]\nprovider = 'claude'\nmodel = 'sonnet'\neffort = 'high'\n")...))
		reload(g)
		after := g.readFile()
		cfg1 := g.r.cfg
		mdxWantLines(t, "alias /add +14", g.run("/add +14"),
			[]string{"配置里已经有同一个模型、同一强度的 agent snt，直接 /add snt"})
		if !reflect.DeepEqual(g.readFile(), after) {
			t.Errorf("配置文件被改了")
		}
		if b := g.bakNames(); len(b) != 0 {
			t.Errorf("不应当有备份：%q", b)
		}
		if g.r.cfg != cfg1 {
			t.Errorf("r.cfg 指针不应当变")
		}
		if ps := g.participants(); len(ps) != 0 {
			t.Errorf("库里不应当有成员：%+v", ps)
		}
	})
	t.Run("deleted", func(t *testing.T) {
		g := mdxNewRig(t, mdxConfigTemplate)
		g.run("/add")
		g.writeFile([]byte(mdxReplaceOnce(t, string(g.readFile()), mdxFable51Section, "")))
		reload(g)
		mdxWantLines(t, "deleted 识别部分", g.afterCandidates(g.run("/add")), mdxSplit(mdxR1Block)[1:])
		if n := g.exec.count(); n != 3 {
			t.Errorf("调用次数 %d，want 3", n)
		}
	})
	t.Run("provider", func(t *testing.T) {
		variant := mdxReplaceOnce(t, mdxConfigTemplate,
			"model        = 'claude-sonnet-4-6'\neffort       = 'high'\n",
			"model        = 'claude-sonnet-4-6'\n")
		g := mdxNewRig(t, variant)
		g.run("/add")
		baks := g.bakNames()
		lines := g.run("/add +11")
		bak := g.newBak(baks)
		mdxWantLines(t, "provider /add +11", lines, g.joinedLines("agy-sonnet-4.6-thinking", bak))
	})
}

// TestMdxAddCancelNotCached 是 R14：识别中途被取消，不存快照，下次 /add 重新识别。
//
// 注意：late 的取消必须落在最后一家（grok）。挪到中间一家（claude）时，循环会在 grok 之前
// break，「只在循环提前结束时才算取消」（M107）就没有可观测的后果了。这与 R6 each_kind
// 「超时的那一家不能排在最后」是同一类坑，方向相反。
//
// 【判别力】取消之后照样缓存（M83）：两个子测试都红（失败行被存下，第二次不再识别）。去掉循环
// 开头的 break（M84）：只有 first 红（调用次数 3 而不是 1）。只在「一行都没拿到」（M105、M106）
// 或「循环提前结束」（M107）时才算取消：只有 late 红（部分快照被存下）。
func TestMdxAddCancelNotCached(t *testing.T) {
	cases := []struct {
		name      string
		n         int
		path      string
		wantFirst int
		wantAfter int
	}{
		{"first", 1, mdxAgyPath, 1, 4},
		{"late", 3, mdxGrokPath, 3, 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := mdxNewRig(t, mdxConfigTemplate)
			ctx1, cancel1 := context.WithCancel(context.Background())
			defer cancel1()
			g.exec.hook = func(n int, dc provider.DiscoveryCommand) {
				if n == c.n && dc.Path == c.path {
					cancel1()
				}
			}
			mdxWantLines(t, "取消的那次 /add", g.afterCandidates(g.runCtx(ctx1, "/add")), mdxSplit(mdxR14CancelBlock))
			if n := g.exec.count(); n != c.wantFirst {
				t.Errorf("取消之后调用次数 %d，want %d", n, c.wantFirst)
			}
			if g.r.modelSnap != nil {
				t.Errorf("取消之后不应当存快照")
			}
			mdxWantLines(t, "再一次 /add", g.afterCandidates(g.runCtx(context.Background(), "/add")), mdxSplit(mdxR1Block))
			if n := g.exec.count(); n != c.wantAfter {
				t.Errorf("再识别之后调用次数 %d，want %d", n, c.wantAfter)
			}
			if g.r.modelSnap == nil {
				t.Errorf("再识别之后应当有快照")
			}
		})
	}
}

// TestMdxAddWriteFailureShowsBackup 是 R15：写入阶段失败（Windows 上 Rename 覆盖只读目标失败），
// 这时备份已经建好，屏幕那一行带上备份路径。
//
// 前提：Windows 上 os.Rename 覆盖只读的目标会失败（MoveFileEx 的语义，Go 1.27.0 本机实测）；
// 系统错误文本按 en-US 取到 Access is denied.。哪天这两条不成立，是前提变了，不是被测代码错了。
//
// 【判别力】整行只有 CreateTemp 的随机后缀一处 \d+ 通配，其余逐字；正对照是 R2 第 2 步。
// 备份路径不上屏（M102）时这一行变成「写配置文件失败: 写入: rename …」。R16 是另一半（backup 为空）。
func TestMdxAddWriteFailureShowsBackup(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("只在 Windows 上成立：Rename 覆盖只读目标失败")
	}
	g := mdxNewRig(t, mdxConfigTemplate)
	g.run("/add")
	orig := g.readFile()
	cfg0 := g.r.cfg
	baks := g.bakNames()
	if err := os.Chmod(g.cfgPath, 0o444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(g.cfgPath, 0o644) })
	lines := g.run("/add +14")
	if err := os.Chmod(g.cfgPath, 0o644); err != nil {
		t.Fatalf("chmod 回去: %v", err)
	}
	bak := g.newBak(baks)
	re := regexp.MustCompile("^" + regexp.QuoteMeta("写配置文件失败（原文件备份在 "+bak+"）: 写入: rename "+filepath.Join(g.dir, "config.toml.tmp-add-")) + `\d+` + regexp.QuoteMeta(" "+g.cfgPath+": Access is denied.") + "$")
	if len(lines) != 1 || !re.MatchString(lines[0]) {
		t.Errorf("输出应当恰好一行、匹配 %s，got %q", re, lines)
	}
	if !reflect.DeepEqual(g.readFile(), orig) {
		t.Errorf("配置文件应当等于原文")
	}
	if b, err := os.ReadFile(bak); err != nil || !reflect.DeepEqual(b, orig) {
		t.Errorf("备份应当等于原文（err=%v）", err)
	}
	if got, want := g.dirNames(), mdxSorted([]string{"config.toml", "data", filepath.Base(bak)}); !reflect.DeepEqual(got, want) {
		t.Errorf("dir 里的条目 = %q，want %q", got, want)
	}
	if g.r.cfg != cfg0 {
		t.Errorf("r.cfg 指针不应当变")
	}
	if ps := g.participants(); len(ps) != 0 {
		t.Errorf("库里不应当有成员：%+v", ps)
	}
}

// TestMdxAddBackupStageFailure 是 R16：备份阶段失败（99 个名字全被占），backup 为空，不带备份路径。
//
// 前提：writeBackup 的名字只到秒，所以预建 [now-2 s, now+8 s] 每一秒的 99 个名字（1089 个文件）。
// 机器慢到 /add +14 在 now 之后 8 秒还没走到 writeBackup，它会拿到空闲的名字、写入成功，这条用例
// 红在「输出恰好一行」上：那是前提坏了，不是被测代码错了。往前多留 2 秒防墙上时钟被往回拨。
//
// 【判别力】整行只通配时间戳，且两处必须相等。if backup != "" 改成 if true（M108）时这一行变成
// 「写配置文件失败（原文件备份在 ）: 备份: …」；改成 if false（M102）时本条不变，红的是 R15。
func TestMdxAddBackupStageFailure(t *testing.T) {
	g := mdxNewRig(t, mdxConfigTemplate)
	g.run("/add")
	orig := g.readFile()
	cfg0 := g.r.cfg
	now := time.Now()
	pre := map[string]bool{}
	for s := -2; s <= 8; s++ {
		base := g.cfgPath + ".bak-add-" + now.Add(time.Duration(s)*time.Second).Format("20060102-150405")
		for i := 1; i <= 99; i++ {
			name := base
			if i > 1 {
				name = base + "-" + strconv.Itoa(i)
			}
			if err := os.WriteFile(name, []byte("old"), 0o644); err != nil {
				t.Fatalf("预建备份名: %v", err)
			}
			pre[filepath.Base(name)] = true
		}
	}
	if len(pre) != 1089 {
		t.Fatalf("前提：应当预建 1089 个互不相同的名字，实际 %d", len(pre))
	}

	lines := g.run("/add +14")
	re := regexp.MustCompile("^" + regexp.QuoteMeta("写配置文件失败: 备份: backup names "+g.cfgPath+".bak-add-") + `(\d{8}-\d{6})` + regexp.QuoteMeta(" and "+g.cfgPath+".bak-add-") + `(\d{8}-\d{6})` + regexp.QuoteMeta("-2..99 all taken") + "$")
	if len(lines) != 1 {
		t.Fatalf("输出应当恰好一行，got %q", lines)
	}
	m := re.FindStringSubmatch(lines[0])
	if m == nil {
		t.Fatalf("输出不匹配 %s：%q", re, lines[0])
	}
	if m[1] != m[2] {
		t.Errorf("两处时间戳应当相等：%q vs %q", m[1], m[2])
	}
	if !pre["config.toml.bak-add-"+m[1]] {
		t.Errorf("时间戳 %s 不在预建的时间窗里", m[1])
	}
	if !reflect.DeepEqual(g.readFile(), orig) {
		t.Errorf("配置文件应当等于原文")
	}
	if g.r.cfg != cfg0 {
		t.Errorf("r.cfg 指针不应当变")
	}
	if ps := g.participants(); len(ps) != 0 {
		t.Errorf("库里不应当有成员：%+v", ps)
	}
	names := g.dirNames()
	var extra []string
	for _, n := range names {
		if !pre[n] {
			extra = append(extra, n)
		}
	}
	if !reflect.DeepEqual(extra, []string{"config.toml", "data"}) || len(names) != 1089+2 {
		t.Errorf("dir 里除了预建的文件应当恰好 config.toml、data，实际多出 %q（共 %d 项）", extra, len(names))
	}
}
