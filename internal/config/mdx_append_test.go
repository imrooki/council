package config

// MD1 契约 §2.3：/add +编号 往配置文件末尾追加一段 agent 表用到的 config 包函数：
// 渲染（AgentBlockTOML）、追加（AppendBlock）、备份（writeBackup）、写回
// （RestoreFile）与新 id 的检查（NewAgentIDProblem）。期望值逐字取自契约。

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"testing"
	"time"
)

// mdxListDir 返回 dir 下的文件名（排好序）。
func mdxListDir(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读目录 %s: %v", dir, err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// mdxReadFile 读文件内容，读不到就 Fatalf。
func mdxReadFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("读 %s: %v", p, err)
	}
	return string(b)
}

// mdxWriteFile 写文件，写不了就 Fatalf。
func mdxWriteFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatalf("写 %s: %v", p, err)
	}
}

// TestMdxAgentBlockTOML 覆盖 C1：三例逐字比，再各拼上 provider 表写成文件、
// Load 读回，Agents[id] 必须与传入的 AgentConfig 相等。
//
// 【判别力】表名总加引号（M29）时 (b) 红；从不加引号（M30）时 (a) 红，而且
// (a) 读回失败（点号被 TOML 当成层级）；effort 为空也写（M31）时 (b) 红；
// 双引号不转义（M32）时 (c) 红且读回失败；不写 role（M73）时三例都红。
// (c) 同时钉住注释行里控制字符换空格、值里控制字符写成 \uXXXX。
func TestMdxAgentBlockTOML(t *testing.T) {
	cases := []struct {
		name    string
		id      string
		a       AgentConfig
		comment string
		want    string
	}{
		{
			name:    "a",
			id:      "opus-5.5-1m-high",
			a:       AgentConfig{DisplayName: "Opus 5.5 1m", Provider: "claude", Model: "claude-opus-5-5[1m]", Effort: "high", Permissions: PermissionRead, Mode: ModeDiscussion},
			comment: "council /add 追加：claude 识别出的 claude-opus-5-5[1m]（订阅）",
			want:    "# council /add 追加：claude 识别出的 claude-opus-5-5[1m]（订阅）\n[agents.\"opus-5.5-1m-high\"]\ndisplay_name = \"Opus 5.5 1m\"\nprovider = \"claude\"\nmodel = \"claude-opus-5-5[1m]\"\neffort = \"high\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n",
		},
		{
			name:    "b",
			id:      "g2",
			a:       AgentConfig{DisplayName: "Grok 4.6", Provider: "grok", Model: "grok-4.6", Permissions: PermissionRead, Mode: ModeDiscussion},
			comment: "",
			want:    "[agents.g2]\ndisplay_name = \"Grok 4.6\"\nprovider = \"grok\"\nmodel = \"grok-4.6\"\nrole = \"\"\npermissions = \"read\"\nmode = \"discussion\"\n",
		},
		{
			name:    "c",
			id:      "x_1",
			a:       AgentConfig{DisplayName: "a\"b\\c\td", Provider: "agy", Model: "M\x7f", Role: "r\n2", Permissions: PermissionRead, Mode: ModeDiscussion},
			comment: "line1\nline2\x7f",
			want:    "# line1 line2 \n[agents.x_1]\ndisplay_name = \"a\\\"b\\\\c\\u0009d\"\nprovider = \"agy\"\nmodel = \"M\\u007F\"\nrole = \"r\\u000A2\"\npermissions = \"read\"\nmode = \"discussion\"\n",
		},
	}
	const prefix = "data_dir = ''\n[providers.claude]\ncommand = 'c'\n[providers.grok]\ncommand = 'g'\n[providers.agy]\ncommand = 'a'\n"
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := AgentBlockTOML(tc.id, tc.a, tc.comment)
			if got != tc.want {
				t.Errorf("AgentBlockTOML 不符\n实际: %q\n期望: %q", got, tc.want)
			}
			p := filepath.Join(t.TempDir(), "c.toml")
			mdxWriteFile(t, p, prefix+got)
			cfg, err := Load(p)
			if err != nil {
				t.Fatalf("读回失败: %v", err)
			}
			if back, ok := cfg.Agents[tc.id]; !ok || !reflect.DeepEqual(back, tc.a) {
				t.Errorf("读回的 Agents[%q] = %#v（存在=%v），期望 %#v", tc.id, back, ok, tc.a)
			}
		})
	}
}

// TestMdxAppendBlockSeparators 覆盖 C2：四种原文结尾的分隔规则、备份名与内容、
// 不留临时文件。
//
// 【判别力】不以换行结尾时只补一个换行（M33）第 3 个文件红；以换行结尾时不再
// 补空行（M34）第 2 个文件红；不走 writeBackup（M36）时备份名不匹配。第 4 个
// 文件钉住「不检测 CRLF」：追加段仍是 LF。
func TestMdxAppendBlockSeparators(t *testing.T) {
	dir := t.TempDir()
	origs := []string{"", "a\n", "a", "a\r\n"}
	wants := []string{"[agents.z]\n", "a\n\n[agents.z]\n", "a\n\n[agents.z]\n", "a\r\n\n[agents.z]\n"}
	for i := range origs {
		i := i
		t.Run(fmt.Sprintf("c%d", i), func(t *testing.T) {
			name := fmt.Sprintf("c%d.toml", i)
			p := filepath.Join(dir, name)
			mdxWriteFile(t, p, origs[i])
			backup, orig, err := AppendBlock(p, "[agents.z]\n")
			if err != nil {
				t.Fatalf("AppendBlock: %v", err)
			}
			if string(orig) != origs[i] {
				t.Errorf("返回的 orig = %q，期望 %q", orig, origs[i])
			}
			if got := mdxReadFile(t, p); got != wants[i] {
				t.Errorf("追加后文件 = %q，期望 %q", got, wants[i])
			}
			if filepath.Dir(backup) != dir {
				t.Errorf("备份 %q 不在配置文件的目录 %q 里", backup, dir)
			}
			re := regexp.MustCompile(fmt.Sprintf(`^c%d\.toml\.bak-add-\d{8}-\d{6}$`, i))
			if !re.MatchString(filepath.Base(backup)) {
				t.Errorf("备份文件名 %q 不匹配 %s", filepath.Base(backup), re)
			}
			if got := mdxReadFile(t, backup); got != origs[i] {
				t.Errorf("备份内容 = %q，期望原文 %q", got, origs[i])
			}
		})
	}
	// 恰好 8 个：4 个配置加 4 个备份，多出来的只可能是没删的 *.tmp-add-* 临时文件
	names := mdxListDir(t, dir)
	if len(names) != 8 {
		t.Errorf("目录里应恰好有 8 个文件（4 个配置 + 4 个备份），实际 %d 个: %q", len(names), names)
	}
}

// TestMdxWriteBackupNeverOverwrites 覆盖 C3：同一秒里连建三个备份，依次加
// -2、-3，绝不覆盖。
//
// 【判别力】去掉 O_EXCL（M35）时三次都是同一个名字、只剩 v2。
func TestMdxWriteBackupNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c9.toml")
	now := time.Date(2026, 9, 27, 15, 30, 12, 0, time.Local)
	wantNames := []string{"c9.toml.bak-add-20260927-153012", "c9.toml.bak-add-20260927-153012-2", "c9.toml.bak-add-20260927-153012-3"}
	contents := []string{"v0", "v1", "v2"}
	var got []string
	for i, c := range contents {
		name, err := writeBackup(p, []byte(c), now)
		if err != nil {
			t.Fatalf("第 %d 次 writeBackup: %v", i+1, err)
		}
		got = append(got, name)
		if want := filepath.Join(dir, wantNames[i]); name != want {
			t.Errorf("第 %d 次返回 %q，期望 %q", i+1, name, want)
		}
	}
	for i, name := range got {
		if s := mdxReadFile(t, name); s != contents[i] {
			t.Errorf("%s 的内容 = %q，期望 %q", name, s, contents[i])
		}
	}
}

// TestMdxWriteBackupAllTaken 覆盖 C6：第 99 个名字是边界，之后全被占时报错。
//
// 【判别力】循环写成 i < maxBackupAttempts（M98）时第一次调用就报错；报错
// 文本回到 v1 只写 -2..99（M99）时第二次的文本不等。
func TestMdxWriteBackupAllTaken(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c9.toml")
	now := time.Date(2026, 9, 27, 15, 30, 12, 0, time.Local)
	base := p + ".bak-add-20260927-153012"
	olds := []string{base}
	for i := 2; i <= 98; i++ {
		olds = append(olds, fmt.Sprintf("%s-%d", base, i))
	}
	for _, o := range olds {
		mdxWriteFile(t, o, "old")
	}
	if len(olds) != 98 {
		t.Fatalf("预建的旧备份应为 98 个，实际 %d", len(olds))
	}

	name, err := writeBackup(p, []byte("v99"), now)
	if err != nil || name != base+"-99" {
		t.Fatalf("第 99 个名字: 返回 %q, %v；期望 %q, nil", name, err, base+"-99")
	}
	if s := mdxReadFile(t, name); s != "v99" {
		t.Errorf("%s 的内容 = %q，期望 v99", name, s)
	}

	name, err = writeBackup(p, []byte("v100"), now)
	if name != "" {
		t.Errorf("全被占时返回 %q，期望空串", name)
	}
	wantErr := "backup names " + base + " and " + base + "-2..99 all taken"
	if err == nil || err.Error() != wantErr {
		t.Errorf("全被占时 err = %v，期望 %q", err, wantErr)
	}

	if names := mdxListDir(t, dir); len(names) != 99 {
		t.Errorf("目录里应恰好 99 个文件，实际 %d 个", len(names))
	}
	for _, o := range olds {
		if s := mdxReadFile(t, o); s != "old" {
			t.Errorf("%s 被改成 %q，期望仍是 old", o, s)
		}
	}
}

// TestMdxRestoreFile 覆盖 C4：写回原字节，不留临时文件。
//
// 【判别力】不写或写错内容时文件不是 orig bytes；临时文件没删时目录里多一个。
func TestMdxRestoreFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.toml")
	mdxWriteFile(t, p, "changed")
	if err := RestoreFile(p, []byte("orig bytes")); err != nil {
		t.Fatalf("RestoreFile: %v", err)
	}
	if s := mdxReadFile(t, p); s != "orig bytes" {
		t.Errorf("写回后内容 = %q，期望 %q", s, "orig bytes")
	}
	if names := mdxListDir(t, dir); !reflect.DeepEqual(names, []string{"c.toml"}) {
		t.Errorf("目录里应只有 c.toml（不留 *.tmp-add-* 临时文件），实际 %q", names)
	}
}

// TestMdxNewAgentIDProblem 覆盖 C5：五道检查逐句比（契约原型实测，逐字）。
//
// 【判别力】去掉设备名检查（M39）时 nul 等变空串；只比整个 id（M40）时
// nul.1 / com1.x 变空串；折叠只换点号（M41）时 opus_5.0 变空串；去掉折叠
// 检查（M42）时 opus-5-0 等变空串；去掉「已存在」（M43）时 opus-5.0 变成
// 折叠那一句；去掉保留名（M44）时 user / system 变空串。com0、com10、con-x
// 这些是反例：写宽了的判据在它们上面报错。
func TestMdxNewAgentIDProblem(t *testing.T) {
	cfg := &Config{Agents: map[string]AgentConfig{"opus-5.0": {}, "grok-4.7": {}, "a_b": {}}}
	cases := []struct {
		id   string
		want string
	}{
		{"sonnet-5-high", ""},
		{"opus-5.5-1m-high", ""},
		{"Bad", "id Bad 不合法（要匹配 ^[a-z0-9]([a-z0-9_.-]*[a-z0-9_-])?$）；用 --id <新 id> 换一个"},
		{"-x", "id -x 不合法（要匹配 ^[a-z0-9]([a-z0-9_.-]*[a-z0-9_-])?$）；用 --id <新 id> 换一个"},
		{"x.", "id x. 不合法（要匹配 ^[a-z0-9]([a-z0-9_.-]*[a-z0-9_-])?$）；用 --id <新 id> 换一个"},
		{"user", "id user 是保留名；用 --id <新 id> 换一个"},
		{"system", "id system 是保留名；用 --id <新 id> 换一个"},
		{"nul", "id nul 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"},
		{"con", "id con 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"},
		{"aux", "id aux 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"},
		{"prn", "id prn 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"},
		{"com1", "id com1 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"},
		{"com9", "id com9 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"},
		{"lpt1", "id lpt1 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"},
		{"lpt9", "id lpt9 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"},
		{"com0", ""},
		{"lpt0", ""},
		{"com10", ""},
		{"nul.1", "id nul.1 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"},
		{"com1.x", "id com1.x 是 Windows 保留设备名，不能当工作目录名；用 --id <新 id> 换一个"},
		{"con-x", ""},
		{"nulx", ""},
		{"opus-5.0", "配置里已经有 agent opus-5.0；用 --id <新 id> 换一个"},
		{"grok-4.7", "配置里已经有 agent grok-4.7；用 --id <新 id> 换一个"},
		{"opus-5-0", "id opus-5-0 与已有的 agent opus-5.0 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录；用 --id <新 id> 换一个"},
		{"opus_5.0", "id opus_5.0 与已有的 agent opus-5.0 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录；用 --id <新 id> 换一个"},
		{"opus-5_0", "id opus-5_0 与已有的 agent opus-5.0 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录；用 --id <新 id> 换一个"},
		{"opus-5.00", ""},
		{"a-b", "id a-b 与已有的 agent a_b 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录；用 --id <新 id> 换一个"},
		{"a.b", "id a.b 与已有的 agent a_b 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录；用 --id <新 id> 换一个"},
		{"a_b", "配置里已经有 agent a_b；用 --id <新 id> 换一个"},
	}
	for _, tc := range cases {
		if got := NewAgentIDProblem(cfg, tc.id); got != tc.want {
			t.Errorf("NewAgentIDProblem(%q) = %q，期望 %q", tc.id, got, tc.want)
		}
	}
}
