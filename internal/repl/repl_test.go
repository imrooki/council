package repl

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"council/internal/config"
)

// testREPLTimeout 是单个 Run 允许的最长阻塞时间。超过即判定循环没有按契约结束。
const testREPLTimeout = 3 * time.Second

// testMaxPromptBytes 是一次提示符允许的字节上限，只用于空行用例的次要 sanity check。
const testMaxPromptBytes = 64

// testBlockingReader 在测试结束前既不产出数据也不返回 EOF，
// 用来制造「只有 ctx 取消或 /quit 才能让 Run 返回」的局面。
type testBlockingReader struct {
	release <-chan struct{}
}

func (r testBlockingReader) Read(p []byte) (int, error) {
	<-r.release
	return 0, io.EOF
}

// newTestBlockingReader 返回一个永久阻塞的 reader，测试结束时自动解除阻塞，避免 goroutine 泄漏。
func newTestBlockingReader(t *testing.T) io.Reader {
	t.Helper()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	return testBlockingReader{release: release}
}

// newTestREPLConfig 造一份含多条目的配置。
// 三张表的 id 用互不相同、互不为子串的前缀（pv_ / ag_ / tm_），
// command / model 之类的值也不含 id 文本，这样用 strings.Index 判定顺序时不会被别的列抢先命中。
// 三张表都按 key 降序写入，用来暴露「没有排序就直接遍历 map」的实现。
func newTestREPLConfig() *config.Config {
	cfg := &config.Config{
		Providers: make(map[string]config.ProviderConfig),
		Agents:    make(map[string]config.AgentConfig),
		Teams:     make(map[string]config.TeamConfig),
	}

	cfg.Providers["pv_zeta"] = config.ProviderConfig{Command: "exe-three.exe"}
	cfg.Providers["pv_mid"] = config.ProviderConfig{Command: "exe-two.exe"}
	cfg.Providers["pv_alpha"] = config.ProviderConfig{Command: "exe-one.exe"}

	cfg.Agents["ag_zeta"] = config.AgentConfig{
		DisplayName: "Zeta",
		Provider:    "pv_zeta",
		Model:       "mod-three",
		Effort:      "",
		Role:        "role three",
		Permissions: config.PermissionFull,
		Mode:        config.ModeJudge,
	}
	cfg.Agents["ag_mid"] = config.AgentConfig{
		DisplayName: "Mid",
		Provider:    "pv_mid",
		Model:       "mod-two",
		Effort:      "eff-two",
		Role:        "role two",
		Permissions: config.PermissionWorkspace,
		Mode:        config.ModeWorker,
	}
	cfg.Agents["ag_alpha"] = config.AgentConfig{
		DisplayName: "Alpha",
		Provider:    "pv_alpha",
		Model:       "mod-one",
		Effort:      "eff-one",
		Role:        "role one",
		Permissions: config.PermissionRead,
		Mode:        config.ModeDiscussion,
	}

	cfg.Teams["tm_zeta"] = config.TeamConfig{DisplayName: "Team Zeta", Members: []string{"ag_zeta"}}
	cfg.Teams["tm_mid"] = config.TeamConfig{DisplayName: "Team Mid", Members: []string{"ag_mid", "ag_zeta"}}
	cfg.Teams["tm_alpha"] = config.TeamConfig{DisplayName: "Team Alpha", Members: []string{"ag_alpha", "ag_mid"}}

	return cfg
}

// runREPLReader 起一个 REPL 跑到结束，带看门狗：Run 不返回就直接判失败，不会把测试挂死。
// svc 传 nil：本文件锁的是 Step 4 的只读命令，它们不碰持久化。
// 带 Service 的跑法见 conversation_test.go 的 runREPLSvc。
func runREPLReader(t *testing.T, ctx context.Context, cfg *config.Config, in io.Reader) (string, error) {
	t.Helper()

	var out bytes.Buffer
	r := New(cfg, nil, nil, in, &out, RulesContext{})

	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()

	select {
	case err := <-done:
		return out.String(), err
	case <-time.After(testREPLTimeout):
		t.Fatalf("REPL.Run 在 %v 内没有返回", testREPLTimeout)
		return "", nil
	}
}

// runREPLInput 是 runREPLReader 的便捷形式：喂一段字面输入，输入耗尽即 EOF。
func runREPLInput(t *testing.T, cfg *config.Config, input string) (string, error) {
	t.Helper()
	return runREPLReader(t, context.Background(), cfg, strings.NewReader(input))
}

// TestREPLCommandOutput 覆盖五个命令与未知命令的输出内容。
// 断言只用关键片段，不锁死排版，排版由实现自己决定。
//
// 非斜杠输入曾经也在这张表里，断言它指向 Step 4 的规格文档。
// Step 5 起非斜杠输入不再是错误，而是直接发言，那条用例随 msgNotChat 一起移走了；
// 新行为由 bare_text_test.go 覆盖，那里比的是两条路径的行为，不是某句提示的措辞。
func TestREPLCommandOutput(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		wantContains []string
	}{
		{
			name:         "help 列出全部命令",
			input:        "/help\n",
			wantContains: []string{"/help", "/quit", "/providers", "/agents", "/teams"},
		},
		{
			name:  "providers 列出 id 与 command",
			input: "/providers\n",
			wantContains: []string{
				"pv_alpha", "pv_mid", "pv_zeta",
				"exe-one.exe", "exe-two.exe", "exe-three.exe",
			},
		},
		{
			name:  "agents 列出 id 与关键字段",
			input: "/agents\n",
			wantContains: []string{
				"ag_alpha", "ag_mid", "ag_zeta",
				"pv_alpha", "pv_mid", "pv_zeta",
				"mod-one", "mod-two", "mod-three",
				"eff-one", "eff-two",
				"read", "workspace", "full",
				"discussion", "worker", "judge",
			},
		},
		{
			name:  "teams 列出 id 与成员",
			input: "/teams\n",
			wantContains: []string{
				"tm_alpha", "tm_mid", "tm_zeta",
				"ag_alpha", "ag_mid", "ag_zeta",
			},
		},
		{
			name:         "未知命令提示 help",
			input:        "/no_such_command\n",
			wantContains: []string{"/help"},
		},
		{
			name:         "quit 正常退出",
			input:        "/quit\n",
			wantContains: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestREPLConfig()
			out, err := runREPLInput(t, cfg, tc.input)
			if err != nil {
				t.Fatalf("Run 应当返回 nil，实际返回 %v", err)
			}
			for _, want := range tc.wantContains {
				if !strings.Contains(out, want) {
					t.Errorf("输出里缺少 %q；完整输出：\n%s", want, out)
				}
			}
		})
	}
}

// TestREPLOutputSorted 断言三张表的输出按 id 升序。
// 配置是按降序写入 map 的，实现若直接遍历 map 就会随机失败，
// 所以每个用例重复多轮，压掉「单轮碰巧有序」的可能。
func TestREPLOutputSorted(t *testing.T) {
	const rounds = 5

	cases := []struct {
		name      string
		input     string
		wantOrder []string
	}{
		{"providers 升序", "/providers\n", []string{"pv_alpha", "pv_mid", "pv_zeta"}},
		{"agents 升序", "/agents\n", []string{"ag_alpha", "ag_mid", "ag_zeta"}},
		{"teams 升序", "/teams\n", []string{"tm_alpha", "tm_mid", "tm_zeta"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for round := 0; round < rounds; round++ {
				cfg := newTestREPLConfig()
				out, err := runREPLInput(t, cfg, tc.input)
				if err != nil {
					t.Fatalf("第 %d 轮 Run 返回 %v", round, err)
				}
				prev := -1
				for _, id := range tc.wantOrder {
					at := strings.Index(out, id)
					if at < 0 {
						t.Fatalf("第 %d 轮输出里找不到 id %q；完整输出：\n%s", round, id, out)
					}
					if at <= prev {
						t.Fatalf("第 %d 轮 id %q 出现在位置 %d，没有排在前一个 id 之后，说明没有排序；完整输出：\n%s",
							round, id, at, out)
					}
					prev = at
				}
			}
		})
	}
}

// TestREPLTermination 覆盖循环结束的两条路径：/quit 与输入流结束。
// /quit 用例在命令之后接一个永不结束的 reader：实现若不认 /quit，Run 会挂住并被看门狗判失败。
func TestREPLTermination(t *testing.T) {
	cases := []struct {
		name    string
		makeIn  func(t *testing.T) io.Reader
		wantNil bool
	}{
		{
			name: "quit 让循环结束",
			makeIn: func(t *testing.T) io.Reader {
				return io.MultiReader(strings.NewReader("/quit\n"), newTestBlockingReader(t))
			},
			wantNil: true,
		},
		{
			name: "输入流直接结束等同 quit",
			makeIn: func(t *testing.T) io.Reader {
				return strings.NewReader("")
			},
			wantNil: true,
		},
		{
			name: "命令之后输入流结束",
			makeIn: func(t *testing.T) io.Reader {
				return strings.NewReader("/help\n")
			},
			wantNil: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestREPLConfig()
			_, err := runREPLReader(t, context.Background(), cfg, tc.makeIn(t))
			if tc.wantNil && err != nil {
				t.Fatalf("Run 应当返回 nil，实际返回 %v", err)
			}
		})
	}
}

// TestREPLEmptyLineIsNoop 断言空行不触发任何命令、提示或表格。
// 提示符长什么样契约没规定，所以主断言是「输出里不出现任何已知的命令或提示文本」，
// 字节增量只作为次要 sanity check。
func TestREPLEmptyLineIsNoop(t *testing.T) {
	cfg := newTestREPLConfig()

	forbidden := []string{
		"/help", "/quit", "/providers", "/agents", "/teams",
		"07_STEP4_SPEC",
		"pv_alpha", "ag_alpha", "tm_alpha",
		"exe-one.exe", "mod-one",
	}

	// 基线是「不喂任何行、直接 EOF」的输出。实现若打印启动提示或告别语，两次都会打印，
	// 所以比的是出现次数而不是有没有出现，这样既不会误伤启动提示，也照样能抓住空行触发的提示。
	baseline, err := runREPLInput(t, cfg, "")
	if err != nil {
		t.Fatalf("空输入下 Run 应当返回 nil，实际返回 %v", err)
	}

	oneLine, err := runREPLInput(t, cfg, "\n")
	if err != nil {
		t.Fatalf("空行之后 Run 应当返回 nil，实际返回 %v", err)
	}
	for _, bad := range forbidden {
		if got, want := strings.Count(oneLine, bad), strings.Count(baseline, bad); got != want {
			t.Errorf("空行不应触发任何输出，但 %q 的出现次数从基线的 %d 变成了 %d；完整输出：\n%s",
				bad, want, got, oneLine)
		}
	}

	threeLines, err := runREPLInput(t, cfg, "\n\n\n")
	if err != nil {
		t.Fatalf("三个空行之后 Run 应当返回 nil，实际返回 %v", err)
	}
	if delta := len(threeLines) - len(oneLine); delta > 2*testMaxPromptBytes {
		t.Errorf("多两个空行让输出多了 %d 字节，超过两段提示符的上限 %d，说明空行产生了多余输出",
			delta, 2*testMaxPromptBytes)
	}
}

// TestREPLRunReturnsOnContextCancel 断言 ctx 取消能打断阻塞中的读。
// 输入是一个永不产出也永不结束的 reader，因此 Run 只可能因 ctx 取消而返回。
// 取消后的返回值契约没有规定（nil 还是 ctx.Err()），这里只断言有限时间内返回。
func TestREPLRunReturnsOnContextCancel(t *testing.T) {
	cfg := newTestREPLConfig()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var out bytes.Buffer
	r := New(cfg, nil, nil, newTestBlockingReader(t), &out, RulesContext{})

	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()

	cancel()

	select {
	case err := <-done:
		t.Logf("ctx 取消后 Run 返回：%v", err)
	case <-time.After(testREPLTimeout):
		t.Fatalf("ctx 取消后 Run 在 %v 内没有返回", testREPLTimeout)
	}
}
