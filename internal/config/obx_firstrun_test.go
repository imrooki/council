package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// obx_firstrun_test.go 是 OB1（首次运行自动配置，契约 §2.1）在 config 包的验收：
// 检测本机工具（C1）、渲染 [providers.*] 与首次运行的全文（C2、C3）、中性模板（C4）、
// 不覆盖地建文件（C5-C7）。检测只看 PATH 与几处已知位置，不起进程；本文件的夹具
// 都是空文件，检测不执行它们。

// obxDetect 是 obxDetectEnv 建好的一组目录。
type obxDetect struct {
	root, p1, p2, home, appdata, local string
}

// obxDetectEnv 在一个临时目录下建 p1、p2、home、appdata、local，PATH = p1;p2，
// USERPROFILE、HOME、APPDATA、LOCALAPPDATA 指到对应目录。
func obxDetectEnv(t *testing.T) *obxDetect {
	t.Helper()
	root := t.TempDir()
	e := &obxDetect{
		root:    root,
		p1:      filepath.Join(root, "p1"),
		p2:      filepath.Join(root, "p2"),
		home:    filepath.Join(root, "home"),
		appdata: filepath.Join(root, "appdata"),
		local:   filepath.Join(root, "local"),
	}
	for _, d := range []string{e.p1, e.p2, e.home, e.appdata, e.local} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("建目录 %s: %v", d, err)
		}
	}
	t.Setenv("PATH", e.p1+string(os.PathListSeparator)+e.p2)
	t.Setenv("USERPROFILE", e.home)
	t.Setenv("HOME", e.home)
	t.Setenv("APPDATA", e.appdata)
	t.Setenv("LOCALAPPDATA", e.local)
	return e
}

// obxTouch 建父目录、写一个空文件，返回它的路径。
func obxTouch(t *testing.T, parts ...string) string {
	t.Helper()
	p := filepath.Join(parts...)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("建目录 %s: %v", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatalf("写 %s: %v", p, err)
	}
	return p
}

// obxOnlyEntries 断言 dir 里的条目名恰好是 names（临时文件都删掉了）。
func obxOnlyEntries(t *testing.T, dir string, names ...string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	var got []string
	for _, e := range ents {
		got = append(got, e.Name())
	}
	if !reflect.DeepEqual(got, names) {
		t.Errorf("%s 里的条目应当恰好是 %q，实际 %q", dir, names, got)
	}
}

// obxWantDetect 按固定顺序拼期望的四项。
func obxWantDetect(claude, grok, agy, codex string) []DetectedProvider {
	return []DetectedProvider{
		{ID: "claude", Path: claude},
		{ID: "grok", Path: grok},
		{ID: "agy", Path: agy},
		{ID: "codex", Path: codex},
	}
}

// obxLoadText 把 text 写进一个临时文件再 LoadAndValidate。
func obxLoadText(t *testing.T, text string) *Config {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatalf("写配置: %v", err)
	}
	cfg, err := LoadAndValidate(p)
	if err != nil {
		t.Fatalf("LoadAndValidate 应当成功: %v\n文本:\n%s", err, text)
	}
	return cfg
}

// TestObxDetectProviders 是 C1。
// 【判别力】path_and_known、npm_and_agy_known、path_wins、known_is_dir、empty_env_no_relative
// 对 M1-M6、M9、M10 红；cwd_planted 对 X1 红，它的前提断言是正对照（宿主注入的
// NoDefaultCurrentDirectoryInExePath 不拿掉时 LookPath 不看当前目录，这一条会成为空用例）。
func TestObxDetectProviders(t *testing.T) {
	t.Run("path_and_known", func(t *testing.T) {
		e := obxDetectEnv(t)
		claude := obxTouch(t, e.p2, "claude.cmd")
		codex := obxTouch(t, e.p1, "codex.cmd")
		grok := obxTouch(t, e.home, ".grok", "bin", "grok.exe")
		got := DetectProviders()
		want := obxWantDetect(claude, grok, "", codex)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("DetectProviders:\n got  %+v\n want %+v", got, want)
		}
	})
	t.Run("npm_and_agy_known", func(t *testing.T) {
		e := obxDetectEnv(t)
		claude := obxTouch(t, e.appdata, "npm", "claude.cmd")
		codex := obxTouch(t, e.appdata, "npm", "codex.cmd")
		agy := obxTouch(t, e.local, "agy", "bin", "agy.exe")
		got := DetectProviders()
		want := obxWantDetect(claude, "", agy, codex)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("DetectProviders:\n got  %+v\n want %+v", got, want)
		}
	})
	t.Run("path_wins", func(t *testing.T) {
		e := obxDetectEnv(t)
		onPath := obxTouch(t, e.p1, "claude.cmd")
		obxTouch(t, e.appdata, "npm", "claude.cmd")
		got := DetectProviders()
		if len(got) != 4 {
			t.Fatalf("应当恰好四项，实际 %+v", got)
		}
		if got[0].ID != "claude" || got[0].Path != onPath {
			t.Errorf("第 0 项应当是 {claude %s}，实际 %+v", onPath, got[0])
		}
	})
	t.Run("known_is_dir", func(t *testing.T) {
		e := obxDetectEnv(t)
		d := filepath.Join(e.home, ".grok", "bin", "grok.exe")
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("建目录: %v", err)
		}
		got := DetectProviders()
		if len(got) != 4 {
			t.Fatalf("应当恰好四项，实际 %+v", got)
		}
		if got[1].ID != "grok" || got[1].Path != "" {
			t.Errorf("grok.exe 是目录时第 1 项应当是 {grok \"\"}，实际 %+v", got[1])
		}
	})
	t.Run("empty_env_no_relative", func(t *testing.T) {
		e := obxDetectEnv(t)
		cwd := filepath.Join(e.root, "cwd")
		obxTouch(t, cwd, "npm", "claude.cmd")
		t.Chdir(cwd)
		t.Setenv("APPDATA", "")
		got := DetectProviders()
		if len(got) != 4 {
			t.Fatalf("应当恰好四项，实际 %+v", got)
		}
		if got[0].ID != "claude" || got[0].Path != "" {
			t.Errorf("APPDATA 为空时第 0 项应当是 {claude \"\"}（不拼相对路径 npm\\claude.cmd），实际 %+v", got[0])
		}
	})
	t.Run("cwd_planted", func(t *testing.T) {
		const key = "NoDefaultCurrentDirectoryInExePath"
		old, had := os.LookupEnv(key)
		t.Cleanup(func() {
			if had {
				os.Setenv(key, old)
			} else {
				os.Unsetenv(key)
			}
		})
		os.Unsetenv(key)
		e := obxDetectEnv(t)
		cwd := filepath.Join(e.root, "cwd")
		obxTouch(t, cwd, "claude.cmd")
		t.Chdir(cwd)
		if _, err := exec.LookPath("claude"); !errors.Is(err, exec.ErrDot) {
			t.Fatalf("前提不成立：当前目录里有 claude.cmd 时 exec.LookPath 应当返回 exec.ErrDot，实际 %v", err)
		}
		got := DetectProviders()
		if len(got) != 4 {
			t.Fatalf("应当恰好四项，实际 %+v", got)
		}
		if got[0].ID != "claude" || got[0].Path != "" {
			t.Errorf("当前目录里的 claude.cmd 不算找到，第 0 项应当是 {claude \"\"}，实际 %+v", got[0])
		}
	})
}

// TestObxProviderBlockTOML 是 C2：字面量字符串与含单引号时的基本字符串，各自往返加载。
// 【判别力】M11、M12。
func TestObxProviderBlockTOML(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`C:\Users\a\AppData\Roaming\npm\claude.cmd`, "[providers.claude]\ncommand = 'C:\\Users\\a\\AppData\\Roaming\\npm\\claude.cmd'\n"},
		{`C:\Users\O'Brien\npm\claude.cmd`, "[providers.claude]\ncommand = \"C:\\\\Users\\\\O'Brien\\\\npm\\\\claude.cmd\"\n"},
	} {
		got := ProviderBlockTOML("claude", c.in)
		if got != c.want {
			t.Errorf("ProviderBlockTOML(claude, %q):\n got  %q\n want %q", c.in, got, c.want)
			continue
		}
		cfg := obxLoadText(t, got)
		if cmd := cfg.Providers["claude"].Command; cmd != c.in {
			t.Errorf("往返：加载回来的 command 应当是 %q，实际 %q", c.in, cmd)
		}
	}
}

// TestObxFirstRunTOML 是 C3（v1.2 拆成两个子测试）。
// 【判别力】literal 对 M12、M14-M17、M21、M101、M106 红；quote_data_dir 对 M21、M101、M102 红。
func TestObxFirstRunTOML(t *testing.T) {
	t.Run("literal", func(t *testing.T) {
		got := FirstRunTOML(`C:\h\.council\data`, []DetectedProvider{
			{ID: "claude", Path: `C:\x\claude.cmd`},
			{ID: "grok", Path: ""},
			{ID: "agy", Path: `C:\y\agy.exe`},
			{ID: "codex", Path: ""},
		})
		want := DefaultTOML() + "\n# 数据目录（本地库与成员的工作目录），council 第一次运行时定在本文件旁边。可以改成别的绝对路径，重启 council 生效（已有的库不会跟着搬）；写了这一行，环境变量 COUNCIL_DATA_DIR 就不再起作用。\ndata_dir = 'C:\\h\\.council\\data'\n\n# 下面是 council 第一次运行时在本机找到的工具。\n[providers.claude]\ncommand = 'C:\\x\\claude.cmd'\n\n[providers.agy]\ncommand = 'C:\\y\\agy.exe'\n"
		if got != want {
			t.Fatalf("FirstRunTOML 不等:\n got  %q\n want %q", got, want)
		}
		cfg := obxLoadText(t, got)
		if len(cfg.Agents) != 0 || len(cfg.Teams) != 0 || cfg.Summary.Agent != "" || len(cfg.Providers) != 2 {
			t.Errorf("加载结果应当是 0 agent、0 team、没有 summary、2 个 provider，实际 agents=%d teams=%d summary=%q providers=%d",
				len(cfg.Agents), len(cfg.Teams), cfg.Summary.Agent, len(cfg.Providers))
		}
		if cfg.DataDir != `C:\h\.council\data` {
			t.Errorf("DataDir 应当是 %q，实际 %q", `C:\h\.council\data`, cfg.DataDir)
		}
	})
	t.Run("quote_data_dir", func(t *testing.T) {
		in := `C:\Users\O'Brien\.council\data`
		got := FirstRunTOML(in, nil)
		line := "\ndata_dir = \"C:\\\\Users\\\\O'Brien\\\\.council\\\\data\"\n"
		if !strings.Contains(got, line) {
			t.Fatalf("FirstRunTOML 应当含基本字符串的 data_dir 行 %q，实际:\n%s", line, got)
		}
		cfg := obxLoadText(t, got)
		if cfg.DataDir != in {
			t.Errorf("往返：DataDir 应当是 %q，实际 %q", in, cfg.DataDir)
		}
	})
}

// TestObxDefaultTOMLNeutral 是 C4：模板全是注释、不含私人示例、含必须出现的几件事，
// 单独加载是零 provider、零 agent、零 team。
// 【判别力】M21、M22。
func TestObxDefaultTOMLNeutral(t *testing.T) {
	s := DefaultTOML()
	for i, l := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
		if !strings.HasPrefix(l, "#") {
			t.Errorf("第 %d 行不是注释: %q", i+1, l)
		}
	}
	for _, bad := range []string{"theory", "critic", "project", "<project>", "剪切"} {
		if strings.Contains(s, bad) {
			t.Errorf("模板不该含 %q", bad)
		}
	}
	for _, need := range []string{DefaultDataDir, "rules_file", "full 档能写进配置并通过校验，不等于编排器可以自动使用它", "[summary]", "/add +编号"} {
		if !strings.Contains(s, need) {
			t.Errorf("模板应当含 %q", need)
		}
	}
	cfg := obxLoadText(t, s)
	if len(cfg.Providers) != 0 || len(cfg.Agents) != 0 || len(cfg.Teams) != 0 {
		t.Errorf("模板单独加载应当是零 provider、零 agent、零 team，实际 %d、%d、%d", len(cfg.Providers), len(cfg.Agents), len(cfg.Teams))
	}
}

// TestObxCreateConfigFile 是 C5：建两层目录、写入、绝不覆盖、父路径是文件时报非 ErrExist 的错、不留临时文件。
// 【判别力】M19、M24。
func TestObxCreateConfigFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "a", "b")
	p := filepath.Join(dir, "config.toml")
	if err := CreateConfigFile(p, []byte("x = 1\n")); err != nil {
		t.Fatalf("第一次 CreateConfigFile 应当成功: %v", err)
	}
	if b, err := os.ReadFile(p); err != nil || string(b) != "x = 1\n" {
		t.Fatalf("内容应当是 %q，实际 %q（%v）", "x = 1\n", b, err)
	}
	err := CreateConfigFile(p, []byte("y = 2\n"))
	if !errors.Is(err, fs.ErrExist) {
		t.Errorf("第二次应当返回满足 fs.ErrExist 的错误，实际 %v", err)
	}
	if b, rerr := os.ReadFile(p); rerr != nil || string(b) != "x = 1\n" {
		t.Errorf("第二次之后内容应当仍是 %q，实际 %q（%v）", "x = 1\n", b, rerr)
	}

	blk := filepath.Join(root, "blk")
	if err := os.WriteFile(blk, []byte("x"), 0o644); err != nil {
		t.Fatalf("写 blk: %v", err)
	}
	bp := filepath.Join(blk, "config.toml")
	if _, serr := os.Stat(bp); !errors.Is(serr, fs.ErrNotExist) {
		t.Fatalf("前提：Stat(%s) 应当满足 ErrNotExist，实际 %v", bp, serr)
	}
	err = CreateConfigFile(bp, []byte("x = 1\n"))
	if err == nil || errors.Is(err, fs.ErrExist) {
		t.Errorf("父路径是文件时应当返回非 nil 且不满足 ErrExist 的错误，实际 %v", err)
	}
	obxOnlyEntries(t, dir, "config.toml")
}

// TestObxCreateConfigFileConcurrent 是 C6：16 路同时建同一个文件，每轮恰好一个成功、其余 ErrExist、内容是赢家那一份。
// 【判别力】X4（概率杀；确定杀手是 C7 via_link）、M24、M19。
func TestObxCreateConfigFileConcurrent(t *testing.T) {
	const rounds, workers = 20, 16
	root := t.TempDir()
	payload := func(i int) []byte { return bytes.Repeat([]byte{byte('a' + i)}, 256*1024) }
	for r := 0; r < rounds; r++ {
		dir := filepath.Join(root, fmt.Sprintf("r%02d", r))
		p := filepath.Join(dir, "config.toml")
		start := make(chan struct{})
		errs := make([]error, workers)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				errs[i] = CreateConfigFile(p, payload(i))
			}(i)
		}
		close(start)
		wg.Wait()
		winner := -1
		for i, err := range errs {
			switch {
			case err == nil:
				if winner >= 0 {
					t.Fatalf("第 %d 轮：%d 与 %d 都返回 nil，覆盖了别人写的文件", r, winner, i)
				}
				winner = i
			case !errors.Is(err, fs.ErrExist):
				t.Errorf("第 %d 轮：第 %d 路应当返回 nil 或满足 ErrExist 的错误，实际 %v", r, i, err)
			}
		}
		if winner < 0 {
			t.Fatalf("第 %d 轮：没有一路返回 nil", r)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("第 %d 轮：读文件: %v", r, err)
		}
		if !bytes.Equal(b, payload(winner)) {
			t.Errorf("第 %d 轮：文件内容不是赢家（第 %d 路）那一份（长度 %d）", r, winner, len(b))
		}
		obxOnlyEntries(t, dir, "config.toml")
	}
}

// obxErrNoLink 是 C7 换掉 linkFile 时返回的错误：不满足 ErrExist。
var obxErrNoLink = errors.New("obx: 这个卷不支持硬链接")

// obxSwapLink 换掉 linkFile，t.Cleanup 还原。
func obxSwapLink(t *testing.T, f func(oldname, newname string) error) {
	t.Helper()
	orig := linkFile
	linkFile = f
	t.Cleanup(func() { linkFile = orig })
}

// TestObxCreateConfigFileLink 是 C7：经硬链接一次到位；硬链接失败时退回 O_EXCL，仍然不覆盖。
// 【判别力】via_link 对 X4、M23 红；fallback 对 M25 红；fallback_exists 对 M18、M25 红；三个都对 M24 红；
// via_link、fallback 对 M19 红。
func TestObxCreateConfigFileLink(t *testing.T) {
	// via_link 与 fallback 的目录事先不存在，靠 CreateConfigFile 自己建（M19 的杀手之一）；
	// fallback_exists 要先写好目标，目录只能预建。
	t.Run("via_link", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "d")
		p := filepath.Join(dir, "config.toml")
		calls := 0
		obxSwapLink(t, func(oldname, newname string) error {
			calls++
			if newname != p {
				t.Errorf("linkFile 的 newname 应当是 %q，实际 %q", p, newname)
			}
			if filepath.Dir(oldname) != dir {
				t.Errorf("临时文件应当在 %q 里，实际 %q", dir, oldname)
			}
			if oldname == p {
				t.Errorf("oldname 不该就是目标 %q", p)
			}
			if _, err := os.Stat(p); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("硬链接之前目标不该存在（不是先建出来再写的），Stat: %v", err)
			}
			if b, err := os.ReadFile(oldname); err != nil || string(b) != "x = 1\n" {
				t.Errorf("硬链接之前临时文件应当已经是完整内容 %q，实际 %q（%v）", "x = 1\n", b, err)
			}
			return os.Link(oldname, newname)
		})
		if err := CreateConfigFile(p, []byte("x = 1\n")); err != nil {
			t.Fatalf("CreateConfigFile 应当成功: %v", err)
		}
		if calls != 1 {
			t.Errorf("linkFile 应当恰好调 1 次，实际 %d", calls)
		}
		if b, err := os.ReadFile(p); err != nil || string(b) != "x = 1\n" {
			t.Errorf("内容应当是 %q，实际 %q（%v）", "x = 1\n", b, err)
		}
		obxOnlyEntries(t, dir, "config.toml")
	})
	t.Run("fallback", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "d")
		p := filepath.Join(dir, "config.toml")
		obxSwapLink(t, func(string, string) error { return obxErrNoLink })
		if err := CreateConfigFile(p, []byte("x = 1\n")); err != nil {
			t.Fatalf("硬链接失败时应当退回 O_EXCL 并成功，实际 %v", err)
		}
		if b, err := os.ReadFile(p); err != nil || string(b) != "x = 1\n" {
			t.Errorf("内容应当是 %q，实际 %q（%v）", "x = 1\n", b, err)
		}
		obxOnlyEntries(t, dir, "config.toml")
	})
	t.Run("fallback_exists", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "config.toml")
		if err := os.WriteFile(p, []byte("old\n"), 0o644); err != nil {
			t.Fatalf("写 old: %v", err)
		}
		obxSwapLink(t, func(string, string) error { return obxErrNoLink })
		if err := CreateConfigFile(p, []byte("x = 1\n")); !errors.Is(err, fs.ErrExist) {
			t.Errorf("目标已存在时退路也应当返回满足 ErrExist 的错误，实际 %v", err)
		}
		if b, err := os.ReadFile(p); err != nil || string(b) != "old\n" {
			t.Errorf("内容应当仍是 %q，实际 %q（%v）", "old\n", b, err)
		}
		obxOnlyEntries(t, dir, "config.toml")
	})
}
