package provider_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"council/internal/provider"
)

var allProviderIDs = []string{"agy", "claude", "grok"}

// baseRequest 是一份"能被至少一家 provider 接受"的最小合法请求，各测试按需要
// 覆盖个别字段。Permission 用 caps.Permissions[0] 而不是硬编码 PermissionRead——
// 09a 没有冻结"Step 6 只放行 read"这件事发生在 provider 包内（那是
// internal/turn.TranslatePermission 的事），provider 包自己的契约只保证
// "Capabilities.Permissions 里声明了什么就该被接受"，测试应该跟着这份声明走，
// 不是跟着"目前恰好只有 read"这个巧合走。
func baseRequest(caps provider.Capabilities, workDir string) provider.TurnRequest {
	return provider.TurnRequest{
		AgentID:     "mechanics-expert",
		DisplayName: "Mechanics Expert",
		Model:       "some-model",
		Effort:      "",
		Role:        "你是力学专家。",
		Permission:  caps.Permissions[0],
		Prompt:      "user: 继续",
		WorkDir:     workDir,
	}
}

func lookup(t *testing.T, id string) provider.Provider {
	t.Helper()
	p, err := provider.Lookup(id)
	if err != nil {
		t.Fatalf("Lookup(%s): %v", id, err)
	}
	return p
}

// pickUnsupportedTier 从三档里找一个不在 have 里的，用来测 ErrUnsupportedTier
// 而不硬编码"workspace 一定不支持"——09a 只冻结了"没列出的一律拒绝"这条行为，
// 没有冻结每家 Permissions 列表的具体内容，测试应该照着 Capabilities() 实际
// 声明的内容走。
func pickUnsupportedTier(have []provider.PermissionTier) (provider.PermissionTier, bool) {
	for _, want := range []provider.PermissionTier{provider.PermissionRead, provider.PermissionWorkspace, provider.PermissionFull} {
		found := false
		for _, h := range have {
			if h == want {
				found = true
				break
			}
		}
		if !found {
			return want, true
		}
	}
	return "", false
}

// TestBuildTurn_WorkDirRequired: req.WorkDir 为空 -> ErrNoWorkDir，对三家都成立。
// 09a 原文："继承 council 的启动目录会让三家 CLI 读到 cwd 下的项目级指令文件，
// 入场费与行为随启动目录漂移、不可复现，而且拿到写盘权限的成员会把文件写进
// 那个仓库"。
func TestBuildTurn_WorkDirRequired(t *testing.T) {
	for _, id := range allProviderIDs {
		t.Run(id, func(t *testing.T) {
			p := lookup(t, id)
			req := baseRequest(p.Capabilities(), "")
			_, err := p.BuildTurn(req)
			if !errors.Is(err, provider.ErrNoWorkDir) {
				t.Fatalf("err = %v, want ErrNoWorkDir", err)
			}
		})
	}
}

// TestBuildTurn_UnsupportedPermissionTier: 权限档不在 Capabilities().Permissions
// 里 -> ErrUnsupportedTier，绝不静默降档（09a BuildTurn 注释原文）。
func TestBuildTurn_UnsupportedPermissionTier(t *testing.T) {
	for _, id := range allProviderIDs {
		t.Run(id, func(t *testing.T) {
			p := lookup(t, id)
			caps := p.Capabilities()
			tier, ok := pickUnsupportedTier(caps.Permissions)
			if !ok {
				t.Skipf("%s declares all three permission tiers supported, nothing to test here", id)
			}
			req := baseRequest(caps, `C:\fake\workdir`)
			req.Permission = tier
			_, err := p.BuildTurn(req)
			if !errors.Is(err, provider.ErrUnsupportedTier) {
				t.Fatalf("err = %v, want ErrUnsupportedTier (tier=%q)", err, tier)
			}
		})
	}
}

// TestBuildTurn_PromptTooLong: Transport 是 Argv 且 prompt 超 MaxPromptBytes ->
// ErrPromptTooLong。长度取自 Capabilities() 本身而不是硬编码 32767——09a 把
// windowsArgvPromptLimit 这个具体数字放在实现里当理由，不是冻结在契约面的
// 数字；契约冻结的是"超过 MaxPromptBytes 就拒绝"这条行为，测试应该用
// Capabilities() 报出来的门槛去触发它，这样即使门槛将来改了这条用例也不会
// 因为一个过时的硬编码常量而失真。
func TestBuildTurn_PromptTooLong(t *testing.T) {
	for _, id := range allProviderIDs {
		t.Run(id, func(t *testing.T) {
			p := lookup(t, id)
			caps := p.Capabilities()
			if caps.Prompt != provider.PromptTransportArgv {
				t.Skipf("%s transport is %v, not Argv, ErrPromptTooLong does not apply", id, caps.Prompt)
			}
			if caps.MaxPromptBytes <= 0 {
				t.Skipf("%s declares MaxPromptBytes<=0 (unlimited), nothing to test here", id)
			}
			req := baseRequest(caps, `C:\fake\workdir`)
			req.Prompt = strings.Repeat("x", caps.MaxPromptBytes+1)
			_, err := p.BuildTurn(req)
			if !errors.Is(err, provider.ErrPromptTooLong) {
				t.Fatalf("err = %v, want ErrPromptTooLong", err)
			}
		})
	}
}

// TestBuildTurn_CommandEnvAndDir: Env 必须是 nil（09a turn.go 原文："nil 表示
// 原样继承 —— Step 6 一律 nil，不改 HOME"），Dir 必须等于 WorkDir。
// 不测 Path：TurnRequest 里没有可执行文件路径这个输入，Path 由上层 Runner
// 从配置补，本层填不了（任务书明确交代不要为它写断言）。
func TestBuildTurn_CommandEnvAndDir(t *testing.T) {
	for _, id := range allProviderIDs {
		t.Run(id, func(t *testing.T) {
			p := lookup(t, id)
			caps := p.Capabilities()
			workDir := `C:\fake\workdir\` + id
			req := baseRequest(caps, workDir)
			cmd, err := p.BuildTurn(req)
			if err != nil {
				t.Fatalf("BuildTurn: %v", err)
			}
			if cmd.Env != nil {
				t.Errorf("Env = %v, want nil", cmd.Env)
			}
			if cmd.Dir != workDir {
				t.Errorf("Dir = %q, want %q", cmd.Dir, workDir)
			}
		})
	}
}

// TestBuildTurn_Determinism_SameRequestTwice: 同一个 TurnRequest 连调两次
// BuildTurn，Args 必须逐元素相等——除了 grok 在 ProviderSession=="" 时会现铸
// 一个新 UUID（D 节裁决：铸币不向上冒泡，但也不是缓存前缀的一部分，那个 uuid
// 本来就是这一次调用现造的，两次调用当然造出两个不同的 uuid）。
func TestBuildTurn_Determinism_SameRequestTwice(t *testing.T) {
	for _, id := range allProviderIDs {
		for _, sessionEmpty := range []bool{true, false} {
			name := id + "/session_empty=" + boolStr(sessionEmpty)
			t.Run(name, func(t *testing.T) {
				p := lookup(t, id)
				caps := p.Capabilities()
				req := baseRequest(caps, `C:\fake\workdir`)
				if !sessionEmpty {
					req.ProviderSession = "prior-session-existing-id"
				}
				cmd1, err1 := p.BuildTurn(req)
				cmd2, err2 := p.BuildTurn(req)
				if err1 != nil || err2 != nil {
					t.Fatalf("BuildTurn errors: %v / %v", err1, err2)
				}

				if id == "grok" && sessionEmpty {
					// grok 首轮：唯一允许不同的是 --session-id 后面那个刚铸的
					// uuid 本身，其余每一个元素必须逐字节相同。
					if len(cmd1.Args) != len(cmd2.Args) {
						t.Fatalf("Args length differs: %v vs %v", cmd1.Args, cmd2.Args)
					}
					i := indexOf(cmd1.Args, "--session-id")
					if i < 0 || i+1 >= len(cmd1.Args) {
						t.Fatalf("--session-id not found with a following value in %v", cmd1.Args)
					}
					for k := range cmd1.Args {
						if k == i+1 {
							continue
						}
						if cmd1.Args[k] != cmd2.Args[k] {
							t.Fatalf("Args[%d] differs outside the minted uuid slot: %q vs %q", k, cmd1.Args[k], cmd2.Args[k])
						}
					}
					if cmd1.Args[i+1] == cmd2.Args[i+1] {
						t.Fatalf("minted session id repeated across two BuildTurn calls: %q (D 节: 从不复用、从不派生)", cmd1.Args[i+1])
					}
					if !isV4UUID(cmd1.Args[i+1]) || !isV4UUID(cmd2.Args[i+1]) {
						t.Fatalf("minted session ids are not valid v4 UUIDs: %q / %q", cmd1.Args[i+1], cmd2.Args[i+1])
					}
					// grok 首轮这一支已经单独比较过（除铸出的 uuid 外逐元素相等），
					// 不落到下面 reflect.DeepEqual 那个通用分支：这里的 t.Run 闭包
					// 不是 for 循环体本身，continue 在这里不合法，用 return 提前退出。
					return
				}

				if !reflect.DeepEqual(cmd1.Args, cmd2.Args) {
					t.Fatalf("Args not stable across identical calls:\n1: %v\n2: %v", cmd1.Args, cmd2.Args)
				}
			})
		}
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// TestBuildTurn_OnlyDifferenceIsSessionFlag 是 D-4 缓存前缀稳定性里最核心的一条：
// 构造两个只有 ProviderSession 不同的请求（空 vs 非空），断言 Args 的差异
// 严格地只落在会话 flag 那一段（D 节表格冻结的 flag 名：claude/grok 是
// --resume、grok 空会话是 --session-id、agy 是 --conversation），除此之外
// 每一个元素原样，不多不少、不挪位置。
//
// agy 是这里唯一的例外，必须单独处理：D 节表格把"不带会话 flag"与"把身份块
// 拼进首轮正文最前面"焊在同一个判定点上（agy 那一行："不带会话 flag，并且把
// 身份块拼进首轮正文最前面"），所以当 Role 非空时，空会话与非空会话两条请求
// 的 prompt 元素本来就不该相等——那不是缺陷，是规格。为了不让这条测试的
// "只差会话 flag"结论被这第二个真实存在的差异污染，agy 这一支把 Role 设成
// 空串，从而让身份拼接这条分支本身不生效，才能干净地单独测会话 flag 这一条。
func TestBuildTurn_OnlyDifferenceIsSessionFlag(t *testing.T) {
	resumeFlag := map[string]string{"claude": "--resume", "grok": "--resume", "agy": "--conversation"}
	freshFlag := map[string]string{"claude": "", "grok": "--session-id", "agy": ""}

	for _, id := range allProviderIDs {
		t.Run(id, func(t *testing.T) {
			p := lookup(t, id)
			caps := p.Capabilities()

			reqFresh := baseRequest(caps, `C:\fake\workdir`)
			reqResume := baseRequest(caps, `C:\fake\workdir`)
			reqResume.ProviderSession = "prior-session-existing-id"
			if id == "agy" {
				// 见函数注释：隔离掉身份拼接这条独立的分支。
				reqFresh.Role = ""
				reqResume.Role = ""
			}

			cmdFresh, err := p.BuildTurn(reqFresh)
			if err != nil {
				t.Fatalf("BuildTurn(fresh): %v", err)
			}
			cmdResume, err := p.BuildTurn(reqResume)
			if err != nil {
				t.Fatalf("BuildTurn(resume): %v", err)
			}

			strippedResume := removeFlagPair(t, cmdResume.Args, resumeFlag[id])
			strippedFresh := cmdFresh.Args
			if ff := freshFlag[id]; ff != "" {
				// grok：空会话那一支也带了一个 2 格的会话 flag（--session-id
				// <刚铸的uuid>），同样要摘掉才能跟 resume 那一支比"其余部分"。
				strippedFresh = removeFlagPair(t, cmdFresh.Args, ff)
			}

			if !reflect.DeepEqual(strippedFresh, strippedResume) {
				t.Fatalf("differences beyond the session flag segment:\nfresh (stripped):  %v\nresume (stripped): %v", strippedFresh, strippedResume)
			}
		})
	}
}

// TestBuildTurn_GrokSessionIDSpecifics 覆盖 D 节表格 grok 那一行的全部细节：
// 空会话铸 v4 UUID、写进 Args 也写进 PlannedSessionID、且两者是同一个值；
// 非空会话发 --resume、PlannedSessionID 必须是空串（K 节明写"claude / agy
// 空会话 -> ... 且 PlannedSessionID 为空"，同一条规则对 grok 的 resume 支也
// 成立——PlannedSessionID 只在"这一轮刚铸了一个还没被确认的 id"这个场合有
// 意义，resume 那一支根本没有铸新 id）。
func TestBuildTurn_GrokSessionIDSpecifics(t *testing.T) {
	p := lookup(t, "grok")
	caps := p.Capabilities()

	t.Run("empty_session_mints_and_plants_uuid", func(t *testing.T) {
		req := baseRequest(caps, `C:\fake\workdir`)
		cmd, err := p.BuildTurn(req)
		if err != nil {
			t.Fatalf("BuildTurn: %v", err)
		}
		i := indexOf(cmd.Args, "--session-id")
		if i < 0 || i+1 >= len(cmd.Args) {
			t.Fatalf("--session-id not found with a following value in %v", cmd.Args)
		}
		if cmd.PlannedSessionID == "" {
			t.Fatalf("PlannedSessionID is empty, want the minted uuid")
		}
		if !isV4UUID(cmd.PlannedSessionID) {
			t.Fatalf("PlannedSessionID = %q is not a valid v4 UUID", cmd.PlannedSessionID)
		}
		if cmd.Args[i+1] != cmd.PlannedSessionID {
			t.Fatalf("Args[--session-id+1] = %q != PlannedSessionID %q, they must be the same minted id", cmd.Args[i+1], cmd.PlannedSessionID)
		}
		if indexOf(cmd.Args, "--resume") >= 0 {
			t.Fatalf("--resume present on a fresh session: %v", cmd.Args)
		}
	})

	t.Run("non_empty_session_resumes_and_clears_planned_id", func(t *testing.T) {
		req := baseRequest(caps, `C:\fake\workdir`)
		req.ProviderSession = "prior-session-existing-id"
		cmd, err := p.BuildTurn(req)
		if err != nil {
			t.Fatalf("BuildTurn: %v", err)
		}
		i := indexOf(cmd.Args, "--resume")
		if i < 0 || i+1 >= len(cmd.Args) {
			t.Fatalf("--resume not found with a following value in %v", cmd.Args)
		}
		if cmd.Args[i+1] != req.ProviderSession {
			t.Fatalf("Args[--resume+1] = %q, want %q", cmd.Args[i+1], req.ProviderSession)
		}
		if cmd.PlannedSessionID != "" {
			t.Fatalf("PlannedSessionID = %q, want empty on a resume turn (nothing was minted)", cmd.PlannedSessionID)
		}
		if indexOf(cmd.Args, "--session-id") >= 0 {
			t.Fatalf("--session-id present on a resume turn: %v", cmd.Args)
		}
	})

	t.Run("minted_id_never_reused_across_two_fresh_turns", func(t *testing.T) {
		req := baseRequest(caps, `C:\fake\workdir`)
		cmd1, err := p.BuildTurn(req)
		if err != nil {
			t.Fatalf("BuildTurn 1: %v", err)
		}
		cmd2, err := p.BuildTurn(req)
		if err != nil {
			t.Fatalf("BuildTurn 2: %v", err)
		}
		if cmd1.PlannedSessionID == cmd2.PlannedSessionID {
			t.Fatalf("two independent fresh turns minted the same session id: %q", cmd1.PlannedSessionID)
		}
	})
}

// TestBuildTurn_GrokNeverEmitsEffortFlag: E 节裁决——grok 经 本地转发服务 会把命令行
// effort 静默丢弃，退出码仍是 0，所以适配器干脆不发这个 flag（发了也是在给
// 自己伪造一份没生效的证据），并且要把这件事记进 Dropped，Option 字段的取值
// "effort" 是 09a turn.go 里 DroppedOption.Option 字段注释逐字冻结的三个字面量
// 之一。
func TestBuildTurn_GrokNeverEmitsEffortFlag(t *testing.T) {
	p := lookup(t, "grok")
	req := baseRequest(p.Capabilities(), `C:\fake\workdir`)
	req.Effort = "high"
	cmd, err := p.BuildTurn(req)
	if err != nil {
		t.Fatalf("BuildTurn: %v", err)
	}
	for _, flag := range []string{"--effort", "--reasoning-effort"} {
		if containsExact(cmd.Args, flag) {
			t.Errorf("Args contains %q, grok must never emit an effort flag (E 节)", flag)
		}
	}
	found := false
	for _, d := range cmd.Dropped {
		if d.Option == "effort" {
			found = true
		}
	}
	if !found {
		t.Errorf("Dropped does not contain an entry with Option==\"effort\": %+v", cmd.Dropped)
	}
}

// TestBuildTurn_ClaudeAppendSystemPromptFileAlwaysPresent: 2026-09-21 claude
// 的身份载体从 --append-system-prompt <文本> 改成了 --append-system-prompt-file
// <path>（claude.go 头部注释：Windows 上 claude 是 npm 装的 .cmd 垫片，Go 起
// .cmd 时命令行由 cmd.exe 再解析一遍，argv 元素里的换行被当成命令分隔符，
// 而 renderIdentity 产出的身份块天然多行，原来那条 argv 路根本走不通——
// 2026-09-20 实测 claude 报 "Input must be provided either through stdin or
// as a prompt argument"）。这条用例保留原意（身份 flag 每轮都发，Role 为空串
// 时也要发，flag 在不在是缓存前缀的一部分），只是换成断言新的 flag 名和它
// 指向的路径。
func TestBuildTurn_ClaudeAppendSystemPromptFileAlwaysPresent(t *testing.T) {
	p := lookup(t, "claude")
	caps := p.Capabilities()
	workDir := `C:\fake\workdir`
	req := baseRequest(caps, workDir)
	req.Role = ""
	cmd, err := p.BuildTurn(req)
	if err != nil {
		t.Fatalf("BuildTurn: %v", err)
	}
	i := indexOf(cmd.Args, "--append-system-prompt-file")
	if i < 0 {
		t.Fatalf("--append-system-prompt-file missing even though it must be sent every turn regardless of Role: %v", cmd.Args)
	}
	wantPath := filepath.Join(workDir, "council-role.txt")
	if i+1 >= len(cmd.Args) || cmd.Args[i+1] != wantPath {
		t.Fatalf("--append-system-prompt-file should be followed by %q, got Args=%v", wantPath, cmd.Args)
	}
}

// TestBuildTurn_ClaudeRoleGoesThroughFilesNotArgv 覆盖新增的 Command.Files：
// 身份块的字节内容不再出现在 Args 里的任何地方，而是挂在 Files[rolePath] 上，
// 值必须逐字节等于 req.Role（不 Trim、不加换行）。Role 为空串时这个键仍然要
// 在——它对应的 --append-system-prompt-file 这个 flag 本身就照发不误
// （见上一条用例），键消失会让 flag 和它指向的文件内容对不上，Files 这张表
// 就不再是"起进程前必须先落盘的东西"的完整清单了。
func TestBuildTurn_ClaudeRoleGoesThroughFilesNotArgv(t *testing.T) {
	p := lookup(t, "claude")
	caps := p.Capabilities()
	workDir := `C:\fake\workdir`
	rolePath := filepath.Join(workDir, "council-role.txt")

	t.Run("non_empty_role", func(t *testing.T) {
		req := baseRequest(caps, workDir)
		req.Role = "You are X (id=mechanics-expert).\nProvider: claude\n力学专家"
		cmd, err := p.BuildTurn(req)
		if err != nil {
			t.Fatalf("BuildTurn: %v", err)
		}
		content, ok := cmd.Files[rolePath]
		if !ok {
			t.Fatalf("Files does not contain key %q: %v", rolePath, cmd.Files)
		}
		if string(content) != req.Role {
			t.Fatalf("Files[%q] = %q, want %q verbatim", rolePath, content, req.Role)
		}
		for _, a := range cmd.Args {
			if strings.Contains(a, req.Role) {
				t.Fatalf("Role bytes leaked into an Args element %q; Role must only travel via Files", a)
			}
		}
	})

	t.Run("empty_role_key_still_present", func(t *testing.T) {
		req := baseRequest(caps, workDir)
		req.Role = ""
		cmd, err := p.BuildTurn(req)
		if err != nil {
			t.Fatalf("BuildTurn: %v", err)
		}
		content, ok := cmd.Files[rolePath]
		if !ok {
			t.Fatalf("Files does not contain key %q even for an empty Role (the flag is still sent, so its file must still be planned): %v", rolePath, cmd.Files)
		}
		if len(content) != 0 {
			t.Fatalf("Files[%q] = %q, want empty", rolePath, content)
		}
	})
}

// TestBuildTurn_ClaudePromptGoesThroughStdinNotArgv 覆盖新增的 Command.Stdin：
// 正文不再是末尾位置参数，而是整段进 Stdin，逐字节等于 req.Prompt；Args 里
// 不应该出现这段正文（不管在哪个位置）——同样是为了不让含换行的正文落进
// .cmd 垫片会被截断的那条 argv 路。
func TestBuildTurn_ClaudePromptGoesThroughStdinNotArgv(t *testing.T) {
	p := lookup(t, "claude")
	caps := p.Capabilities()
	req := baseRequest(caps, `C:\fake\workdir`)
	req.Prompt = "user: 继续，这是正文"
	cmd, err := p.BuildTurn(req)
	if err != nil {
		t.Fatalf("BuildTurn: %v", err)
	}
	if string(cmd.Stdin) != req.Prompt {
		t.Fatalf("Stdin = %q, want %q verbatim", cmd.Stdin, req.Prompt)
	}
	if containsExact(cmd.Args, req.Prompt) {
		t.Fatalf("Args contains the Prompt as an element; Prompt must travel only via Stdin: %v", cmd.Args)
	}
	if cmd.Transport != provider.PromptTransportStdin {
		t.Fatalf("Transport = %v, want PromptTransportStdin", cmd.Transport)
	}
}

// TestBuildTurn_ClaudeArgsNeverContainNewlines 是本次改动真正要守住的不变式：
// Windows 上 claude 是 .cmd 垫片，Go 起 .cmd 时命令行由 cmd.exe 再解析一遍，
// argv 元素里的换行在 cmd.exe 眼里是命令分隔符，整条命令行会在第一个换行处
// 截断——截断后若剩下的部分仍然合法，claude 会带着被截短的系统提示跑完、
// 退出码 0、JSON 合法、没有任何一层报错（claude.go 头部注释）。这条用一个
// 多行 Role 和一个多行 Prompt 做输入，断言 Args 里任何一个元素都不含 \n 或
// \r——两者现在分别走 Files 和 Stdin，理应如此，但这条不变式值得单独钉住，
// 不依赖"我恰好没往 Args 里塞多行内容"这种巧合。
func TestBuildTurn_ClaudeArgsNeverContainNewlines(t *testing.T) {
	p := lookup(t, "claude")
	caps := p.Capabilities()
	req := baseRequest(caps, `C:\fake\workdir`)
	req.Role = "You are X (id=mechanics-expert).\nProvider: claude\r\n力学专家"
	req.Prompt = "第一行\n第二行\r\n第三行"
	cmd, err := p.BuildTurn(req)
	if err != nil {
		t.Fatalf("BuildTurn: %v", err)
	}
	for i, a := range cmd.Args {
		if strings.ContainsAny(a, "\n\r") {
			t.Fatalf("Args[%d] = %q contains a newline; cmd.exe would treat it as a command separator and truncate the command line", i, a)
		}
	}
}

// TestBuildTurn_ClaudeCapabilitiesPromptIsStdin 断言 2026-09-21 的能力声明：
// Prompt 从 PromptTransportArgv 改成了 PromptTransportStdin，MaxPromptBytes
// 改成 0（stdin 没有 argv 那条硬上限）。
func TestBuildTurn_ClaudeCapabilitiesPromptIsStdin(t *testing.T) {
	caps := lookup(t, "claude").Capabilities()
	if caps.Prompt != provider.PromptTransportStdin {
		t.Fatalf("Capabilities().Prompt = %v, want PromptTransportStdin", caps.Prompt)
	}
	if caps.MaxPromptBytes != 0 {
		t.Fatalf("Capabilities().MaxPromptBytes = %d, want 0 (stdin 没有 argv 上限)", caps.MaxPromptBytes)
	}
}

// TestBuildTurn_AgyIdentityPreamble 覆盖 agy 特有的判定点二：它没有可用的
// 身份文本 flag（--agent 收的是预设名），所以身份块只能拼进首轮正文最前面
// （09_STEP6_SPEC.md B 节 agy.go 那一行 + D 节表格）。空会话且 Role 非空时
// 正文元素必须等于 Role+"\n"+Prompt；非空会话时完全不拼，正文逐字节等于
// req.Prompt。用 containsExact 而不是断言下标，因为 09 B 节没有像 claude 那样
// 显式写出 agy 命令行末尾的 <prompt> 占位符，prompt 在 Args 里的具体位置不
// 冻结。
func TestBuildTurn_AgyIdentityPreamble(t *testing.T) {
	p := lookup(t, "agy")
	caps := p.Capabilities()

	t.Run("empty_session_prepends_identity", func(t *testing.T) {
		req := baseRequest(caps, `C:\fake\workdir`)
		req.Role = "你是力学专家。"
		req.Prompt = "user: 继续"
		cmd, err := p.BuildTurn(req)
		if err != nil {
			t.Fatalf("BuildTurn: %v", err)
		}
		want := req.Role + "\n" + req.Prompt
		if !containsExact(cmd.Args, want) {
			t.Fatalf("Args does not contain the identity-prefixed prompt %q: %v", want, cmd.Args)
		}
		if containsExact(cmd.Args, req.Prompt) {
			t.Fatalf("Args contains the bare Prompt %q without the identity prefix on a fresh session", req.Prompt)
		}
	})

	t.Run("resumed_session_does_not_prepend", func(t *testing.T) {
		req := baseRequest(caps, `C:\fake\workdir`)
		req.Role = "你是力学专家。"
		req.Prompt = "user: 继续"
		req.ProviderSession = "prior-session-existing-id"
		cmd, err := p.BuildTurn(req)
		if err != nil {
			t.Fatalf("BuildTurn: %v", err)
		}
		if !containsExact(cmd.Args, req.Prompt) {
			t.Fatalf("Args does not contain the bare Prompt %q byte-for-byte on a resumed session: %v", req.Prompt, cmd.Args)
		}
		unwanted := req.Role + "\n" + req.Prompt
		if containsExact(cmd.Args, unwanted) {
			t.Fatalf("Args contains the identity-prefixed prompt on a resumed session (identity must only be prepended once, on the fresh turn): %v", cmd.Args)
		}
	})
}

// TestBuildTurn_AgyModelIsSentVerbatimAsOneArg 覆盖 2026-09-21 对 agy.go 的一处
// 修正：09_STEP6_SPEC.md 第 165 行列 agy 命令行时漏了 --model，这是规格本身的
// 缺陷（grok 是照规格实现的，两者对照能看出 agy 这边补 --model 不是走样），补上
// 之后每轮都无条件发。req.Model 用一个带空格和括号的真实标签（agy 自己的模型
// 目录长这样，见 E:\<tmp-root>\council\step6-probe\agy-badmodel.err 里列出
// 的 "Gemini 3.8 Flash (High)" 这类名字——agy 把推理强度编进了模型名，不是独立
// 参数），断言它作为 Args 的单一一个元素、逐字节原样出现：Go 的 exec 逐元素
// 传参、不经 shell，这条钉住"没有人把它拼进一个字符串又用空格切开"。
func TestBuildTurn_AgyModelIsSentVerbatimAsOneArg(t *testing.T) {
	p := lookup(t, "agy")
	req := baseRequest(p.Capabilities(), `C:\fake\workdir`)
	req.Model = "Gemini 3.8 Flash (High)"
	cmd, err := p.BuildTurn(req)
	if err != nil {
		t.Fatalf("BuildTurn: %v", err)
	}
	i := indexOf(cmd.Args, "--model")
	if i < 0 || i+1 >= len(cmd.Args) {
		t.Fatalf("--model not found with a following value in %v", cmd.Args)
	}
	if cmd.Args[i+1] != req.Model {
		t.Fatalf("Args[--model+1] = %q, want %q verbatim as a single element (不能被拼接后再切开)", cmd.Args[i+1], req.Model)
	}
}

// TestBuildTurn_AgyNeverEmitsEffortFlag_DropsInstead 覆盖同一次修正的另一半：
// Capabilities().Effort 从 EffortPerTurnFlag 改成了 EffortBakedIntoModelLabel
// （agy 把推理强度编进模型标签，不是独立参数），--effort 这个 flag 不再发；
// req.Effort 非空时改记进 Dropped（Option 字面量 "effort" 冻结在 09a turn.go
// 的 DroppedOption.Option 注释里，与 grok 那条同款），为空时不该产生这一项——
// 空 Effort 没什么好丢的，硬塞一条空记录只是噪声。
func TestBuildTurn_AgyNeverEmitsEffortFlag_DropsInstead(t *testing.T) {
	p := lookup(t, "agy")
	caps := p.Capabilities()
	if caps.Effort != provider.EffortBakedIntoModelLabel {
		t.Fatalf("Capabilities().Effort = %v, want EffortBakedIntoModelLabel", caps.Effort)
	}

	t.Run("effort_non_empty_is_dropped", func(t *testing.T) {
		req := baseRequest(caps, `C:\fake\workdir`)
		req.Effort = "high"
		cmd, err := p.BuildTurn(req)
		if err != nil {
			t.Fatalf("BuildTurn: %v", err)
		}
		if containsExact(cmd.Args, "--effort") {
			t.Errorf("Args contains --effort: %v (agy 的推理强度由 model 标签决定，不是独立参数)", cmd.Args)
		}
		found := false
		for _, d := range cmd.Dropped {
			if d.Option == "effort" {
				found = true
				if d.Value != req.Effort {
					t.Errorf("Dropped effort entry Value = %q, want %q", d.Value, req.Effort)
				}
			}
		}
		if !found {
			t.Errorf("Dropped does not contain an entry with Option==\"effort\": %+v", cmd.Dropped)
		}
	})

	t.Run("effort_empty_produces_no_dropped_entry", func(t *testing.T) {
		req := baseRequest(caps, `C:\fake\workdir`)
		req.Effort = ""
		cmd, err := p.BuildTurn(req)
		if err != nil {
			t.Fatalf("BuildTurn: %v", err)
		}
		for _, d := range cmd.Dropped {
			if d.Option == "effort" {
				t.Errorf("Dropped contains an \"effort\" entry for empty req.Effort: %+v", cmd.Dropped)
			}
		}
	})
}

// TestBuildTurn_ClaudeNoSessionFlagWhenFresh: claude 空会话 -> 不带会话 flag，
// PlannedSessionID 为空（D 节表格：claude 是 SessionIDReturnedOnFirstTurn，
// 只能从输出里取回，没有可预铸的 id）。
func TestBuildTurn_ClaudeNoSessionFlagWhenFresh(t *testing.T) {
	p := lookup(t, "claude")
	req := baseRequest(p.Capabilities(), `C:\fake\workdir`)
	cmd, err := p.BuildTurn(req)
	if err != nil {
		t.Fatalf("BuildTurn: %v", err)
	}
	if containsExact(cmd.Args, "--resume") {
		t.Errorf("Args contains --resume on a fresh session: %v", cmd.Args)
	}
	if cmd.PlannedSessionID != "" {
		t.Errorf("PlannedSessionID = %q, want empty (claude has nothing to pre-mint)", cmd.PlannedSessionID)
	}
}

// TestBuildTurn_GrokWorkspaceRejectedForUnverifiedWriteBoundary 覆盖 11c 契约
// 第 8.1 节验收第 3 条：grok 请求 workspace 必须被拒绝，且错误文本说的是
// 「证明不了边界」（"no verified write boundary"），不能是「本家没这个档」
// 那种泛泛的措辞——两种「不行」不许混（3.2 节原话）。
func TestBuildTurn_GrokWorkspaceRejectedForUnverifiedWriteBoundary(t *testing.T) {
	p := lookup(t, "grok")
	req := baseRequest(p.Capabilities(), `C:\fake\workdir`)
	req.Permission = provider.PermissionWorkspace
	_, err := p.BuildTurn(req)
	if !errors.Is(err, provider.ErrUnsupportedTier) {
		t.Fatalf("err = %v, want ErrUnsupportedTier", err)
	}
	if !strings.Contains(err.Error(), "no verified write boundary") {
		t.Fatalf("err = %v，期望文本点名「证明不了边界」（no verified write boundary），不是泛泛的「没有这个档」", err)
	}
}

// TestBuildTurn_ClaudeAndAgyAcceptWorkspace 是上一条的反例：claude 与 agy
// 的 WriteBoundary 都是 SupportYes 且 Permissions 都列了 workspace
// （11c 契约第 3.1 节），请求 workspace 必须成功，不报错。
func TestBuildTurn_ClaudeAndAgyAcceptWorkspace(t *testing.T) {
	for _, id := range []string{"claude", "agy"} {
		t.Run(id, func(t *testing.T) {
			p := lookup(t, id)
			req := baseRequest(p.Capabilities(), `C:\fake\workdir`)
			req.Permission = provider.PermissionWorkspace
			if _, err := p.BuildTurn(req); err != nil {
				t.Fatalf("BuildTurn(workspace) 应当成功，实际: %v", err)
			}
		})
	}
}

// TestBuildTurn_FullRejectedForAllThree 覆盖 11c 契约第 8.1 节验收第 4 条：
// FULL 对三家一律拒绝（06 第 2 节：MVP 里不给编排器自动使用）。这条与
// TestBuildTurn_UnsupportedPermissionTier 的覆盖有重叠但不完全相同——
// pickUnsupportedTier 对 grok 会先选中 workspace（grok 的 Permissions 只有
// read 一档），FULL 这一档对 grok 从未被那条用例单独断言过，这里补上，
// 三家都显式验一遍。
func TestBuildTurn_FullRejectedForAllThree(t *testing.T) {
	for _, id := range allProviderIDs {
		t.Run(id, func(t *testing.T) {
			p := lookup(t, id)
			req := baseRequest(p.Capabilities(), `C:\fake\workdir`)
			req.Permission = provider.PermissionFull
			_, err := p.BuildTurn(req)
			if !errors.Is(err, provider.ErrUnsupportedTier) {
				t.Fatalf("err = %v, want ErrUnsupportedTier（FULL 档三家都不提供）", err)
			}
		})
	}
}

// TestBuildTurn_AgyNoSessionFlagWhenFresh: 同上，agy 的会话 flag 是 --conversation。
func TestBuildTurn_AgyNoSessionFlagWhenFresh(t *testing.T) {
	p := lookup(t, "agy")
	req := baseRequest(p.Capabilities(), `C:\fake\workdir`)
	cmd, err := p.BuildTurn(req)
	if err != nil {
		t.Fatalf("BuildTurn: %v", err)
	}
	if containsExact(cmd.Args, "--conversation") {
		t.Errorf("Args contains --conversation on a fresh session: %v", cmd.Args)
	}
	if cmd.PlannedSessionID != "" {
		t.Errorf("PlannedSessionID = %q, want empty (agy has nothing to pre-mint)", cmd.PlannedSessionID)
	}
}
