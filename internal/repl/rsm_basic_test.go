package repl

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"council/internal/conversation"
)

// rsmNewREPL 造一个跑在内存假实现上的 REPL。rn 与 in 都是 nil：
// 本文件的用例走 Submit，不读输入，也不调 provider。
// projectDir 非空时经 SetProjectDir 收成规范化绝对路径；空串保持零值。
func rsmNewREPL(t *testing.T, projectDir string) (*REPL, *fakeStore, *bytes.Buffer) {
	t.Helper()
	svc, f := newTestService(t)
	var out bytes.Buffer
	r := New(newTestREPLConfig(), svc, nil, nil, &out, RulesContext{})
	if projectDir != "" {
		r.SetProjectDir(projectDir)
	}
	return r, f, &out
}

// rsmRun 提交一行命令并返回这一行产生的输出。先清空 buffer，
// 连续提交时后一次不会带上前一次的文本。Submit 与行式 dispatch 共用命令表。
func rsmRun(t *testing.T, r *REPL, out *bytes.Buffer, line string) string {
	t.Helper()
	out.Reset()
	if err := r.Submit(context.Background(), line); err != nil {
		t.Fatalf("Submit(%q) 返回错误: %v", line, err)
	}
	return out.String()
}

// rsmSeed 塞一个指定 id 的会话，再改掉 Cwd 与 CreatedAt。
// putConversation 固定用 replTestNow，/resume 的排序用例要自己指定时间。
func rsmSeed(t *testing.T, f *fakeStore, id, title, cwd string, created time.Time) conversation.Conversation {
	t.Helper()
	f.seedConversation(id, title)
	f.mu.Lock()
	defer f.mu.Unlock()
	i, ok := f.convIdx[id]
	if !ok {
		t.Fatalf("seedConversation 之后 convIdx 里没有 %q", id)
	}
	f.convs[i].Cwd = cwd
	f.convs[i].CreatedAt = created
	return f.convs[i]
}

// rsmSetMsgTime 改一条已塞入消息的 CreatedAt。
// AppendMessage 固定用 replTestNow，最后活动时间要靠这个改。
func rsmSetMsgTime(f *fakeStore, convID string, seq int64, at time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.msgs[convID] {
		if f.msgs[convID][i].Seq == seq {
			f.msgs[convID][i].CreatedAt = at
			return
		}
	}
	panic(fmt.Sprintf("rsmSetMsgTime: 会话 %s 里没有 seq=%d 的消息", convID, seq))
}

// rsmTempDir 在 t.TempDir() 下建一个真实子目录，返回规范化后的绝对路径。
// /new 与 /resume 的项目目录都用它，避免相对路径和盘符大小写各写各的。
func rsmTempDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "rsm-proj")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("建临时项目目录失败: %v", err)
	}
	got := NormalizeProjectDir(dir)
	if got == "" {
		t.Fatalf("NormalizeProjectDir(%q) 返回空串，期望一个绝对路径", dir)
	}
	return got
}

func TestRsmNormalizeProjectDir(t *testing.T) {
	t.Run("空串", func(t *testing.T) {
		if got := NormalizeProjectDir(""); got != "" {
			t.Errorf("NormalizeProjectDir(\"\") = %q，期望空串", got)
		}
	})

	d := rsmTempDir(t)
	t.Run("存在的目录三种写法相同且幂等", func(t *testing.T) {
		withSep := NormalizeProjectDir(d + string(os.PathSeparator))
		viaDotDot := NormalizeProjectDir(filepath.Join(d, "sub", ".."))
		plain := NormalizeProjectDir(d)
		if !filepath.IsAbs(plain) {
			t.Errorf("结果 %q 不是绝对路径", plain)
		}
		if withSep != plain {
			t.Errorf("末尾带分隔符 = %q，对目录本身 = %q，期望相同", withSep, plain)
		}
		if viaDotDot != plain {
			t.Errorf("经 sub/.. = %q，对目录本身 = %q，期望相同", viaDotDot, plain)
		}
		again := NormalizeProjectDir(plain)
		if again != plain {
			t.Errorf("对结果再规范化 = %q，期望仍是 %q", again, plain)
		}
	})

	t.Run("相对路径", func(t *testing.T) {
		t.Chdir(d)
		got := NormalizeProjectDir(".")
		want := NormalizeProjectDir(d)
		if got != want {
			t.Errorf("在 %q 里 NormalizeProjectDir(\".\") = %q，期望 %q", d, got, want)
		}
	})

	t.Run("不存在的子目录", func(t *testing.T) {
		missing := filepath.Join(d, "不存在的子目录")
		got := NormalizeProjectDir(missing)
		if !filepath.IsAbs(got) {
			t.Errorf("结果 %q 不是绝对路径", got)
		}
		base := filepath.Base(got)
		if base != "不存在的子目录" {
			t.Errorf("结果 %q 的末段 = %q，期望 %q（EvalSymlinks 失败应退回 Abs）", got, base, "不存在的子目录")
		}
	})

	t.Run("符号链接", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "rsm-link")
		if err := os.Symlink(d, link); err != nil {
			t.Skipf("建符号链接失败，跳过: %v", err)
		}
		got := NormalizeProjectDir(link)
		want := NormalizeProjectDir(d)
		if got != want {
			t.Errorf("NormalizeProjectDir(链接) = %q，期望与目录本身相同 %q", got, want)
		}
	})

	t.Run("Windows 盘符大写且去掉末尾分隔符", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("盘符与根目录的写法只在 Windows 上验")
		}
		got := NormalizeProjectDir(`d:\rsm_case_probe`)
		if got != `D:\rsm_case_probe` {
			t.Errorf("NormalizeProjectDir(`d:\\rsm_case_probe`) = %q，期望 %q", got, `D:\rsm_case_probe`)
		}
		withSep := NormalizeProjectDir(`D:\rsm_case_probe\`)
		plain := NormalizeProjectDir(`D:\rsm_case_probe`)
		if withSep != plain {
			t.Errorf("末尾带分隔符 = %q，不带 = %q，期望相同", withSep, plain)
		}
		root := NormalizeProjectDir(`C:\`)
		if root != `C:\` {
			t.Errorf("NormalizeProjectDir(`C:\\`) = %q，期望 %q（根目录保留末尾分隔符）", root, `C:\`)
		}
	})

	// SameProjectDir 在 Windows 下忽略大小写，拿它比规范化结果恒为真，
	// 验的是比较函数而不是 NormalizeProjectDir 有没有收成磁盘上的写法。
	t.Run("Windows 已存在目录大小写归一", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("目录大小写归一只在 Windows 上验")
		}
		for _, in := range []string{d, strings.ToLower(d), strings.ToUpper(d)} {
			got := NormalizeProjectDir(in)
			if len(got) < 2 || got[0] < 'A' || got[0] > 'Z' || got[1] != ':' {
				t.Errorf("NormalizeProjectDir(%q) = %q，期望以大写盘符和冒号开头", in, got)
			}
		}
		plain := NormalizeProjectDir(d)
		if rsmSameDir(strings.ToLower(d), d) {
			lower := NormalizeProjectDir(strings.ToLower(d))
			if lower != plain {
				t.Errorf("小写变体规范化 = %q，期望与目录本身逐字相同 %q", lower, plain)
			}
		}
		if rsmSameDir(strings.ToUpper(d), d) {
			upper := NormalizeProjectDir(strings.ToUpper(d))
			if upper != plain {
				t.Errorf("大写变体规范化 = %q，期望与目录本身逐字相同 %q", upper, plain)
			}
		}
	})
}

// rsmSameDir 看两个路径是不是同一个目录。Stat 失败或不是同一项都返回 false：
// 目录名大小写敏感时，大小写变体根本不是这个目录，不能算规范化失败。
func rsmSameDir(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

func TestRsmSameProjectDir(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
		skip string
	}{
		{name: "空对空", a: "", b: "", want: false},
		{name: "左空", a: "", b: `D:\x`, want: false},
		{name: "右空", a: `D:\x`, b: "", want: false},
		{name: "相同", a: `D:\proj\a`, b: `D:\proj\a`, want: true},
		{name: "不同", a: `D:\proj\a`, b: `D:\proj\b`, want: false},
		{name: "Windows 大小写", a: `d:\x`, b: `D:\X`, want: true, skip: "non-windows"},
		{name: "非 Windows 区分大小写", a: "/a/B", b: "/a/b", want: false, skip: "windows"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			switch tc.skip {
			case "non-windows":
				if runtime.GOOS != "windows" {
					t.Skip("盘符大小写折叠只在 Windows 上验")
				}
			case "windows":
				if runtime.GOOS == "windows" {
					t.Skip("大小写敏感的比较只在非 Windows 上验")
				}
			}
			got := sameProjectDir(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("sameProjectDir(%q, %q) = %v，期望 %v", tc.a, tc.b, got, tc.want)
			}
			exported := SameProjectDir(tc.a, tc.b)
			if exported != got {
				t.Errorf("SameProjectDir(%q, %q) = %v，sameProjectDir = %v，期望两者一致", tc.a, tc.b, exported, got)
			}
		})
	}
}

func TestRsmSetProjectDir(t *testing.T) {
	d := rsmTempDir(t)
	r, _, _ := rsmNewREPL(t, "")
	r.SetProjectDir(d)
	want := NormalizeProjectDir(d)
	if r.projectDir != want {
		t.Errorf("SetProjectDir(%q) 之后 projectDir = %q，期望 %q", d, r.projectDir, want)
	}
	r.SetProjectDir("")
	if r.projectDir != "" {
		t.Errorf("SetProjectDir(\"\") 之后 projectDir = %q，期望空串", r.projectDir)
	}
	r.SetProjectDir(d + string(os.PathSeparator))
	fromPlain := NormalizeProjectDir(d)
	if r.projectDir != fromPlain {
		t.Errorf("对带末尾分隔符设置后 projectDir = %q，对目录本身规范化 = %q，期望相同", r.projectDir, fromPlain)
	}
}

func TestRsmTruncateRunes(t *testing.T) {
	han100 := strings.Repeat("字", 100)
	han101 := strings.Repeat("字", 101)
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{name: "空串", in: "", max: 10, want: ""},
		{name: "短于 max", in: "abc", max: 10, want: "abc"},
		{name: "恰好 max", in: "abcd", max: 4, want: "abcd"},
		{name: "比 max 多 1", in: "abcde", max: 4, want: "abcd..."},
		{name: "纯 ASCII 超长", in: "0123456789abcdef", max: 8, want: "01234567..."},
		{name: "中文恰好 100", in: han100, max: 100, want: han100},
		{name: "中文 101", in: han101, max: 100, want: han100 + "..."},
		{name: "max 为 0", in: "非空", max: 0, want: "..."},
		{name: "混合中英文", in: "ab汉字cd", max: 4, want: "ab汉字..."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateRunes(tc.in, tc.max)
			if got != tc.want {
				t.Errorf("truncateRunes(%q, %d) = %q，期望 %q", tc.in, tc.max, got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("结果 %q 不是合法 UTF-8", got)
			}
			if tc.name == "中文 101" {
				if n := utf8.RuneCountInString(got); n != 103 {
					t.Errorf("101 个汉字截到 100 再加三点，rune 数 = %d，期望 103", n)
				}
			}
		})
	}
}

func TestRsmParseResumeArgs(t *testing.T) {
	cases := []struct {
		name   string
		fields []string
		all    bool
		token  string
		ok     bool
	}{
		{name: "空", fields: nil, all: false, token: "", ok: true},
		{name: "只有 --all", fields: []string{"--all"}, all: true, token: "", ok: true},
		{name: "--all 在前", fields: []string{"--all", "2"}, all: true, token: "2", ok: true},
		{name: "--all 在后", fields: []string{"2", "--all"}, all: true, token: "2", ok: true},
		{name: "--all 重复", fields: []string{"--all", "--all"}, all: true, token: "", ok: true},
		{name: "未知 flag", fields: []string{"--bogus"}, ok: false},
		{name: "--all 加未知 flag", fields: []string{"--all", "--bogus"}, ok: false},
		{name: "两个 token", fields: []string{"a", "b"}, ok: false},
		{name: "--all 加两个 token", fields: []string{"--all", "a", "b"}, ok: false},
		{name: "单横线不是 flag", fields: []string{"-x"}, all: false, token: "-x", ok: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			all, token, ok := parseResumeArgs(tc.fields)
			if ok != tc.ok {
				t.Errorf("parseResumeArgs(%v) ok = %v，期望 %v（all=%v token=%q）", tc.fields, ok, tc.ok, all, token)
			}
			if !tc.ok {
				return
			}
			if all != tc.all || token != tc.token {
				t.Errorf("parseResumeArgs(%v) = (%v, %q, %v)，期望 (%v, %q, %v)",
					tc.fields, all, token, ok, tc.all, tc.token, tc.ok)
			}
		})
	}
}

func TestRsmIsResumeIndex(t *testing.T) {
	cases := []struct {
		token string
		want  bool
	}{
		{"1", true},
		{"12", true},
		{"123", true},
		{"0", true},
		{"007", true},
		{"1234", false},
		{"", false},
		{"a1", false},
		{"1a", false},
		{"-1", false},
		{"1.5", false},
		{"\uFF11", false},
	}
	for _, tc := range cases {
		name := tc.token
		if name == "" {
			name = "空串"
		}
		t.Run(name, func(t *testing.T) {
			got := isResumeIndex(tc.token)
			if got != tc.want {
				t.Errorf("isResumeIndex(%q) = %v，期望 %v", tc.token, got, tc.want)
			}
		})
	}
}

func TestRsmNewStampsProjectDir(t *testing.T) {
	d := rsmTempDir(t)

	t.Run("有项目目录", func(t *testing.T) {
		r, f, out := rsmNewREPL(t, d)
		got := rsmRun(t, r, out, "/new 带目录的会话")
		if r.cur == nil {
			t.Fatal("/new 之后 r.cur 是 nil，期望已切到新建的会话")
		}
		want := "已建会话 " + r.cur.ID + "，标题: 带目录的会话\n"
		if got != want {
			t.Errorf("输出 = %q，期望恰好 %q", got, want)
		}
		if r.projectDir == "" {
			t.Fatal("r.projectDir 是空串，期望规范化后的项目目录")
		}
		if r.cur.Cwd != r.projectDir {
			t.Errorf("r.cur.Cwd = %q，期望 r.projectDir %q", r.cur.Cwd, r.projectDir)
		}
		stored, err := f.Conversation(context.Background(), r.cur.ID)
		if err != nil {
			t.Fatalf("读回新建会话失败: %v", err)
		}
		if stored.Cwd != r.projectDir {
			t.Errorf("库里的 Cwd = %q，期望 r.projectDir %q", stored.Cwd, r.projectDir)
		}
	})

	t.Run("没有项目目录", func(t *testing.T) {
		r, f, out := rsmNewREPL(t, "")
		got := rsmRun(t, r, out, "/new 不带目录")
		if r.cur == nil {
			t.Fatal("/new 之后 r.cur 是 nil")
		}
		want := "已建会话 " + r.cur.ID + "，标题: 不带目录\n"
		if got != want {
			t.Errorf("输出 = %q，期望恰好 %q", got, want)
		}
		if r.cur.Cwd != "" {
			t.Errorf("r.cur.Cwd = %q，期望空串", r.cur.Cwd)
		}
		stored, err := f.Conversation(context.Background(), r.cur.ID)
		if err != nil {
			t.Fatalf("读回新建会话失败: %v", err)
		}
		if stored.Cwd != "" {
			t.Errorf("库里的 Cwd = %q，期望空串", stored.Cwd)
		}
	})

	t.Run("use 与 resume 不改旧会话归属", func(t *testing.T) {
		r, f, out := rsmNewREPL(t, d)
		const other = `X:\别的项目`
		old := rsmSeed(t, f, "old-other", "旧会话", other, replTestNow)
		rsmRun(t, r, out, "/use "+old.ID)
		if r.cur == nil || r.cur.ID != old.ID {
			t.Fatalf("/use 之后 r.cur = %+v，期望 id %s", r.cur, old.ID)
		}
		if r.cur.Cwd != other {
			t.Errorf("/use 之后 r.cur.Cwd = %q，期望 %q", r.cur.Cwd, other)
		}
		stored, err := f.Conversation(context.Background(), old.ID)
		if err != nil {
			t.Fatalf("读回旧会话失败: %v", err)
		}
		if stored.Cwd != other {
			t.Errorf("/use 之后库里的 Cwd = %q，期望 %q", stored.Cwd, other)
		}
		rsmRun(t, r, out, "/resume "+old.ID)
		if r.cur == nil || r.cur.ID != old.ID {
			t.Fatalf("/resume 之后 r.cur = %+v，期望 id %s", r.cur, old.ID)
		}
		if r.cur.Cwd != other {
			t.Errorf("/resume 之后 r.cur.Cwd = %q，期望 %q", r.cur.Cwd, other)
		}
		stored, err = f.Conversation(context.Background(), old.ID)
		if err != nil {
			t.Fatalf("/resume 后再读旧会话失败: %v", err)
		}
		if stored.Cwd != other {
			t.Errorf("/resume 之后库里的 Cwd = %q，期望 %q", stored.Cwd, other)
		}
	})

	t.Run("没有标题", func(t *testing.T) {
		r, f, out := rsmNewREPL(t, d)
		before := len(f.convs)
		got := rsmRun(t, r, out, "/new")
		if got != "用法: /new <标题>\n" {
			t.Errorf("输出 = %q，期望 %q", got, "用法: /new <标题>\n")
		}
		if len(f.convs) != before {
			t.Errorf("会话数从 %d 变成 %d，期望不变（没给标题不该建会话）", before, len(f.convs))
		}
	})
}

func TestRsmCommandRegistered(t *testing.T) {
	cmd, ok := newCommandTable()["resume"]
	if !ok {
		t.Fatal("命令表里没有 resume")
	}
	if !cmd.needsSvc {
		t.Error("resume.needsSvc = false，期望 true")
	}
	if cmd.needsConv {
		t.Error("resume.needsConv = true，期望 false（没有当前会话时也要能列和接）")
	}
	if cmd.needsRunner {
		t.Error("resume.needsRunner = true，期望 false")
	}
	const wantSummary = "列出本机会话并接上: /resume 列出当前项目的会话；/resume --all 列出全部；/resume [--all] <编号> 按列表编号接上；/resume <id 前缀> 按 id 接上（同 /use，并回显概况）"
	if cmd.summary != wantSummary {
		t.Errorf("summary = %q，期望 %q", cmd.summary, wantSummary)
	}
	if cmd.run == nil {
		t.Error("resume.run 是 nil，期望指向 cmdResume")
	}

	r, _, out := rsmNewREPL(t, "")
	help := rsmRun(t, r, out, "/help")
	n := 0
	for _, line := range strings.Split(help, "\n") {
		if strings.HasPrefix(line, "/resume") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("/help 里以 /resume 开头的行有 %d 行，期望恰好 1 行；/help:\n%s", n, help)
	}
}
