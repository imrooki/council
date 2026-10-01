package config

// 验收 35-39：TOML 点号表名的引号提示（契约第 4.7、6.5 节）。

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// ux2rHintFmt 是契约 6.5 冻结的提示，逐字。三个 %s 依次是推断出的 id、表名、推断出的 id。
const ux2rHintFmt = `如果 %s 本来就是一个含点号的 id：TOML 把不加引号的点号当成层级，表名要写成 [%s."%s"]`

func ux2rHint(table, id string) string { return fmt.Sprintf(ux2rHintFmt, id, table, id) }

// ux2rLoadErr 写一份临时配置，LoadAndValidate，要求失败，返回路径与错误文本。
func ux2rLoadErr(t *testing.T, lines ...string) (string, string) {
	t.Helper()
	path := tstWriteTOML(t, lines...)
	_, err := LoadAndValidate(path)
	if err == nil {
		t.Fatalf("期望加载失败，实际成功；配置:\n%s", strings.Join(lines, "\n"))
	}
	return path, err.Error()
}

// ux2rHintLines 返回错误文本里与某条提示逐字相等的行。
func ux2rHintLines(msg string) []string {
	var out []string
	for _, l := range strings.Split(msg, "\n") {
		if strings.HasPrefix(l, "如果 ") && strings.Contains(l, "TOML 把不加引号的点号当成层级") {
			out = append(out, l)
		}
	}
	return out
}

var ux2rProviderLines = []string{`[providers.pv_alpha]`, `command = "exe-ux2r"`, ``}

func ux2rWith(lines ...string) []string {
	return append(append([]string{}, ux2rProviderLines...), lines...)
}

// 验收 35：[agents.grok-4.7] 带 provider 与 model -> 加载失败，原报错一个字不改，
// 提示逐字、独占一行、恰好一次（长 4 的键有两条，按键逐条提示的实现会出两次）。
func TestUx2rTOMLHintDottedAgentID(t *testing.T) {
	path, msg := ux2rLoadErr(t, ux2rWith(
		`[agents.grok-4.7]`,
		`provider = "pv_alpha"`,
		`model = "mod-ux2r"`,
	)...)

	lines := strings.Split(msg, "\n")
	wantFirst := "unknown config keys in " + path + ": agents.grok-4.7, agents.grok-4.7.model, agents.grok-4.7.provider"
	if lines[0] != wantFirst {
		t.Errorf("原报错那一句必须一个字不改地在最前面\n期望: %q\n实际: %q", wantFirst, lines[0])
	}
	if n := strings.Count(msg, `[agents."grok-4.7"]`); n != 1 {
		t.Errorf("%s 应当恰好出现一次，实际 %d 次；错误:\n%s", `[agents."grok-4.7"]`, n, msg)
	}
	hints := ux2rHintLines(msg)
	if len(hints) != 1 || hints[0] != ux2rHint("agents", "grok-4.7") {
		t.Errorf("应当恰好一条逐字的提示行 %q，实际 %q；错误:\n%s", ux2rHint("agents", "grok-4.7"), hints, msg)
	}
}

// 验收 36：[agents.a.b.c] 带字段。表名键本身也长 4（[agents a b c]），
// 不筛末段的实现会推出 a.b，这条必红。
func TestUx2rTOMLHintMultiDotPicksLastSegmentField(t *testing.T) {
	_, msg := ux2rLoadErr(t, ux2rWith(
		`[agents.a.b.c]`,
		`provider = "pv_alpha"`,
		`model = "mod-ux2r"`,
	)...)

	if !strings.Contains(msg, `[agents."a.b.c"]`) {
		t.Errorf("错误信息应当含 %s；错误:\n%s", `[agents."a.b.c"]`, msg)
	}
	if strings.Contains(msg, `[agents."a.b"]`) {
		t.Errorf("错误信息不许含 %s（表名键被当成了字段键）；错误:\n%s", `[agents."a.b"]`, msg)
	}
	hints := ux2rHintLines(msg)
	if len(hints) != 1 || hints[0] != ux2rHint("agents", "a.b.c") {
		t.Errorf("应当恰好一条提示 %q，实际 %q", ux2rHint("agents", "a.b.c"), hints)
	}
}

// 验收 37：合法 agent 下拼错一个字段（长 3 的键）-> 加载失败，没有提示。用 >= 3 判据的实现会红。
func TestUx2rTOMLNoHintForMisspelledField(t *testing.T) {
	_, msg := ux2rLoadErr(t, ux2rWith(
		`[agents.foo]`,
		`provider = "pv_alpha"`,
		`model = "mod-ux2r"`,
		`bogus = 1`,
	)...)
	if !strings.Contains(msg, "agents.foo.bogus") {
		t.Errorf("原报错应当点名 agents.foo.bogus；错误:\n%s", msg)
	}
	if strings.Contains(msg, `[agents."`) {
		t.Errorf("拼错字段不许出引号提示；错误:\n%s", msg)
	}
}

// 验收 38：合法 agent 下写点号键 bogus.x（长 4、末段不是字段名）-> 没有提示。
// 只判长度 >= 4、不筛末段的实现会推出 foo.bogus，这条必红。
func TestUx2rTOMLNoHintForDottedKeyWithUnknownLastSegment(t *testing.T) {
	_, msg := ux2rLoadErr(t, ux2rWith(
		`[agents.foo]`,
		`provider = "pv_alpha"`,
		`model = "mod-ux2r"`,
		`bogus.x = 1`,
	)...)
	if strings.Contains(msg, `[agents."`) {
		t.Errorf("末段不是已知字段名的点号键不许出引号提示；错误:\n%s", msg)
	}
}

// 验收 39：加了引号的点号表名加载并校验成功，id 逐字节是 grok-4.7。
func TestUx2rTOMLQuotedDottedIDLoads(t *testing.T) {
	path := tstWriteTOML(t, ux2rWith(
		`[agents."grok-4.7"]`,
		`provider = "pv_alpha"`,
		`model = "mod-ux2r"`,
	)...)
	cfg, err := LoadAndValidate(path)
	if err != nil {
		t.Fatalf("加引号的点号表名应当加载并校验成功，实际: %v", err)
	}
	a, ok := cfg.Agents["grok-4.7"]
	if !ok || len(cfg.Agents) != 1 {
		t.Fatalf("应当恰好有一个 id 为 grok-4.7 的 agent，实际 %v", cfg.Agents)
	}
	if a.Provider != "pv_alpha" || a.Model != "mod-ux2r" {
		t.Errorf("字段没读对: %+v", a)
	}
}

// 提示对三张表都生效，表名取 key[0]；多个 id 按字典序、每个 id 只提示一次。
func TestUx2rTOMLHintAllTablesSortedDedup(t *testing.T) {
	_, msg := ux2rLoadErr(t,
		`[providers.pv.x]`,
		`command = "exe-ux2r"`,
		``,
		`[agents.zz-1.5]`,
		`provider = "pv_alpha"`,
		`model = "mod-ux2r"`,
		``,
		`[agents.aa-2.0]`,
		`provider = "pv_alpha"`,
		`model = "mod-ux2r"`,
		``,
		`[teams.t.1]`,
		`members = ["zz"]`,
		`display_name = "T"`,
	)
	want := []string{
		ux2rHint("agents", "aa-2.0"),
		ux2rHint("agents", "zz-1.5"),
		ux2rHint("providers", "pv.x"),
		ux2rHint("teams", "t.1"),
	}
	got := ux2rHintLines(msg)
	gotSet := map[string]int{}
	for _, h := range got {
		gotSet[h]++
	}
	for _, w := range want {
		if gotSet[w] != 1 {
			t.Errorf("提示 %q 应当恰好出现一次，实际 %d 次；错误:\n%s", w, gotSet[w], msg)
		}
	}
	if len(got) != len(want) {
		t.Errorf("提示行应当恰好 %d 条，实际 %d 条: %q", len(want), len(got), got)
	}
	// 同一张表内按 id 的字典序排列。
	ia, iz := strings.Index(msg, ux2rHint("agents", "aa-2.0")), strings.Index(msg, ux2rHint("agents", "zz-1.5"))
	if ia < 0 || iz < 0 || ia > iz {
		t.Errorf("多个 id 的提示应当按字典序：aa-2.0 在 zz-1.5 前面；错误:\n%s", msg)
	}
}

// 已知字段名要从 struct 的 toml tag 取：这里对三个值类型的每个 tag 各造一份
// 「点号 id 下只写这一个字段」的配置，每一份都必须出提示。手写字段清单漏掉任何一个都会红。
func TestUx2rTOMLHintEveryKnownField(t *testing.T) {
	tables := []struct {
		table string
		typ   reflect.Type
	}{
		{"agents", reflect.TypeOf(AgentConfig{})},
		{"teams", reflect.TypeOf(TeamConfig{})},
		{"providers", reflect.TypeOf(ProviderConfig{})},
	}
	for _, tb := range tables {
		for i := 0; i < tb.typ.NumField(); i++ {
			f := tb.typ.Field(i)
			tag := strings.Split(f.Tag.Get("toml"), ",")[0]
			if tag == "" || tag == "-" {
				continue
			}
			value := `"x"`
			if f.Type.Kind() == reflect.Slice {
				value = `["judge"]`
			}
			t.Run(tb.table+"."+tag, func(t *testing.T) {
				_, msg := ux2rLoadErr(t,
					`[`+tb.table+`.ab-1.2]`,
					tag+` = `+value,
				)
				if n := strings.Count(msg, ux2rHint(tb.table, "ab-1.2")); n != 1 {
					t.Errorf("字段 %s 作末段时应当恰好一条提示，实际 %d 条；错误:\n%s", tag, n, msg)
				}
			})
		}
	}
}
