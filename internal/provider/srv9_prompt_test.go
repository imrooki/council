package provider_test

// SRV9：claude 成员的外部系统提示（AgentConfig.Prompt）注入。
//
// 这里钉的是三条最容易在实现里走失的性质：
//  1. 每轮都带，fresh 与 resume 两套形状都不例外；
//  2. 正文不进 argv（.cmd 垫片会把换行当命令分隔符），而是落进
//     --append-system-prompt-file 指向的那个文件，内容逐字正确；
//  3. 其它 provider 一个字节都不许因为多出 SystemPrompt 而变。

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// claudeSystemPromptFixture 是一段带换行、引号、反斜杠与中文的正文：换行钉的是
// 「不能进 argv」，其余字符钉的是「逐字进文件，不做任何转义或改写」。
const claudeSystemPromptFixture = "你是 code reviewer。\n\n只读，不要改文件。\n第三行：C:\\tmp\\x 与 \"引号\"。"

// TestClaudeSystemPromptInjectedEveryTurn 覆盖任务书 §5 第 1 条：
// 有 Prompt 的 claude 成员，两轮（fresh / resume）都要带上，且文件内容逐字正确。
func TestClaudeSystemPromptInjectedEveryTurn(t *testing.T) {
	p := lookup(t, "claude")
	workDir := t.TempDir()

	req := baseRequest(p.Capabilities(), workDir)
	req.SystemPrompt = claudeSystemPromptFixture
	wantBody := req.Role + "\n\n" + claudeSystemPromptFixture

	shapes := []struct {
		name    string
		session string
	}{
		{name: "fresh", session: ""},
		{name: "resume", session: "srv9-session-id"},
	}

	var firstPath string
	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			req.ProviderSession = shape.session
			cmd, err := p.BuildTurn(req)
			if err != nil {
				t.Fatalf("BuildTurn 失败: %v", err)
			}

			i := indexOf(cmd.Args, "--append-system-prompt-file")
			if i < 0 || i+1 >= len(cmd.Args) {
				t.Fatalf("argv 里没有 --append-system-prompt-file <file>: %v", cmd.Args)
			}
			path := cmd.Args[i+1]
			if !filepath.IsAbs(path) {
				t.Errorf("身份文件路径必须是绝对路径，实际 %q", path)
			}
			if dir, _ := filepath.Split(path); filepath.Clean(dir) != filepath.Clean(workDir) {
				t.Errorf("身份文件落在 %q，期望工作目录 %q", dir, workDir)
			}

			body, ok := cmd.Files[path]
			if !ok {
				t.Fatalf("Files 里没有 argv 指向的 %q: %v", path, cmd.Files)
			}
			if string(body) != wantBody {
				t.Errorf("文件内容不是「身份块 + 空行 + 外部提示」逐字拼接：\n实际: %q\n期望: %q", string(body), wantBody)
			}
			if len(cmd.Files) != 1 {
				t.Errorf("带一个外部提示只该落一个文件，实际 %d 个: %v", len(cmd.Files), cmd.Files)
			}

			// 正文一个字节都不许进 argv：这不是洁癖，是 .cmd 垫片的硬约束
			// （spawn.go 的 ErrShimNewlineArg 会在起进程前拒绝）。
			if containsExact(cmd.Args, claudeSystemPromptFixture) {
				t.Error("外部系统提示整段出现在 argv 里")
			}
			for n, arg := range cmd.Args {
				if strings.ContainsAny(arg, "\r\n") {
					t.Errorf("argv[%d] 含换行: %q", n, arg)
				}
			}

			if firstPath == "" {
				firstPath = path
			} else if path != firstPath {
				t.Errorf("两轮的身份文件路径不同：%q vs %q", firstPath, path)
			}
		})
	}

	// 改了 Prompt 就必须换文件名：文件名带内容哈希，指纹靠 argv 里的路径
	// 感知到这次改动（见 claude.go 的 claudeRoleFile）。同名的话，旧会话
	// 会带着 --system-prompt-snapshot 记下的旧提示继续跑。
	req.ProviderSession = ""
	changed := req
	changed.SystemPrompt = claudeSystemPromptFixture + "\n多一行"
	cmd, err := p.BuildTurn(changed)
	if err != nil {
		t.Fatalf("BuildTurn(改后的 prompt) 失败: %v", err)
	}
	changedPath := cmd.Args[indexOf(cmd.Args, "--append-system-prompt-file")+1]
	if changedPath == firstPath {
		t.Errorf("Prompt 改了，身份文件路径却不变: %q", changedPath)
	}
}

// TestClaudeArgvUnchangedWithoutSystemPrompt 覆盖任务书 §5 第 2 条：
// 空 Prompt 时 argv 与改动前逐字节一致。既有的 golden 用例已经钉过一遍，
// 这里再显式断言一次，是因为本片的改动第一次让「空值」有了另一条分支。
func TestClaudeArgvUnchangedWithoutSystemPrompt(t *testing.T) {
	p := lookup(t, "claude")
	workDir := t.TempDir()
	req := baseRequest(p.Capabilities(), workDir)

	cmd, err := p.BuildTurn(req)
	if err != nil {
		t.Fatalf("BuildTurn 失败: %v", err)
	}

	rolePath := filepath.Join(workDir, "council-role.txt")
	want := []string{
		"-p", "--output-format", "json",
		"--model", req.Model,
		"--append-system-prompt-file", rolePath,
		"--safe-mode", "--restricted", "--permission-mode", "default",
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("空 Prompt 的 argv 变了：\n实际: %v\n期望: %v", cmd.Args, want)
	}
	if len(cmd.Files) != 1 {
		t.Fatalf("空 Prompt 只该落身份文件，实际 %d 个: %v", len(cmd.Files), cmd.Files)
	}
	if string(cmd.Files[rolePath]) != req.Role {
		t.Errorf("空 Prompt 的身份文件内容变了: %q", string(cmd.Files[rolePath]))
	}
}

// TestSystemPromptIgnoredByOtherProviders 覆盖任务书 §5 第 5 条：
// codex / grok / agy 的 argv 不因为 SystemPrompt 非空而改变（夹带进来的话，
// 等于偷偷给这三家也做了半套注入；任务书明写这一片只做 claude）。
func TestSystemPromptIgnoredByOtherProviders(t *testing.T) {
	for _, id := range []string{"codex", "grok", "agy"} {
		t.Run(id, func(t *testing.T) {
			p := lookup(t, id)
			req := baseRequest(p.Capabilities(), t.TempDir())
			// 用 resume 形状：grok 的空会话分支每次现铸 UUID，两次调用的 argv
			// 必然不同，会把「字段被忽略」误报成回归。
			req.ProviderSession = "srv9-existing-session"

			without, err := p.BuildTurn(req)
			if err != nil {
				t.Fatalf("BuildTurn(SystemPrompt 空) 失败: %v", err)
			}
			with := req
			with.SystemPrompt = claudeSystemPromptFixture
			got, err := p.BuildTurn(with)
			if err != nil {
				t.Fatalf("BuildTurn(SystemPrompt 非空) 失败: %v", err)
			}

			if !reflect.DeepEqual(got.Args, without.Args) {
				t.Errorf("%s 的 argv 因为 SystemPrompt 变了：\n空:   %v\n非空: %v", id, without.Args, got.Args)
			}
			if !reflect.DeepEqual(got.Files, without.Files) {
				t.Errorf("%s 的 Files 因为 SystemPrompt 变了：空 %v，非空 %v", id, without.Files, got.Files)
			}
			if string(got.Stdin) != string(without.Stdin) {
				t.Errorf("%s 的 Stdin 因为 SystemPrompt 变了", id)
			}
		})
	}
}
