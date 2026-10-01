// 本文件将来落到 internal/turn/spawn_test.go（与既有的 spawn_test.go 合并，
// 或作为独立文件并存——由实现方决定；本文件只新增 11a 契约第 3.6 节的用例：
// 命令行总长闸 CommandLineBytes / ErrCommandLineTooLong，以及验收表第 8 条
// 的端到端场景）。
//
// 只照契约写，不看 internal/turn/spawn.go 的实现——那是本片改动、任务书明令
// 不读的文件。spawnProcess 本身是既有符号（既有 spawn_test.go 已经把它当白盒
// 符号在用），这里只是给它加一组新场景。
//
// 端到端场景（验收表第 8 条）直接调三家 provider 真正的 BuildTurn（
// council/internal/provider 不在禁读名单内），不经 internal/config/rules.go
// 或 renderIdentity：CommandLineBytes 这道闸只关心组好的 Command.Args 有多长，
// 不关心那些字节最终是不是规则文件的正文，直接用一个足够大的字符串模拟
// "超大规则文件" 展开之后写进 Role 的效果，是等价的、更短的构造。
package turn

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"council/internal/provider"
)

// ---------------------------------------------------------------------------
// CommandLineBytes 本身的算式（契约 3.6）
// ---------------------------------------------------------------------------

// TestCommandLineBytesFormula 覆盖："可执行文件路径，加上每个参数前面一个
// 分隔空格"，即 len(path) + sum(1+len(arg))。多字节参数单独一个用例，
// 用来确认量的是字节而不是 rune 数——中文字符每个占三字节。
func TestCommandLineBytesFormula(t *testing.T) {
	const path = `C:\tools\grok.exe`

	t.Run("没有参数时等于路径长度本身", func(t *testing.T) {
		got := CommandLineBytes(path, nil)
		if want := len(path); got != want {
			t.Errorf("CommandLineBytes(path, nil) = %d，期望 %d", got, want)
		}
	})

	t.Run("每个参数前一个分隔空格", func(t *testing.T) {
		args := []string{"--flag", "value"}
		got := CommandLineBytes(path, args)
		want := len(path) + (1 + len("--flag")) + (1 + len("value"))
		if got != want {
			t.Errorf("CommandLineBytes = %d，期望 %d", got, want)
		}
	})

	t.Run("空字符串参数仍然占一个分隔空格的位置", func(t *testing.T) {
		args := []string{"--flag", ""}
		got := CommandLineBytes(path, args)
		want := len(path) + (1 + len("--flag")) + (1 + 0)
		if got != want {
			t.Errorf("CommandLineBytes = %d，期望 %d", got, want)
		}
	})

	t.Run("多字节参数按字节数计，不是 rune 数", func(t *testing.T) {
		// 这一条原本手算了一个 24 当期望值，而那个串是 7 个字不是 8 个，
		// 于是用例自己的前提断言先红了。手算常数在这里没有任何价值：
		// 要验的是「按字节不按 rune」这个区分，不是某个具体数字。
		// 改成两条都不依赖手算 —— 一条对着 len 算期望，
		// 另一条直接钉死它不等于按 rune 算出来的那个值。
		const role = "只做审阅与校核"
		if len(role) == utf8.RuneCountInString(role) {
			t.Fatalf("测试前提有误：%q 必须是多字节串，否则本用例区分不出字节与 rune", role)
		}

		args := []string{"--role", role}
		got := CommandLineBytes(path, args)

		want := len(path) + (1 + len("--role")) + (1 + len(role))
		if got != want {
			t.Errorf("CommandLineBytes = %d，期望 %d（按字节）", got, want)
		}

		runeBased := utf8.RuneCountInString(path) +
			(1 + utf8.RuneCountInString("--role")) +
			(1 + utf8.RuneCountInString(role))
		if got == runeBased {
			t.Errorf("CommandLineBytes = %d，正好等于按 rune 数算出来的值，说明它数的是字符不是字节", got)
		}
	})
}

// tstArgsHittingTotal 构造一组 args = [flag, 填充串]，使
// CommandLineBytes(path, args) 恰好等于 target。用来在边界值上做用例，
// 不依赖 t.TempDir() 生成的路径长度不可预测这一事实。
func tstArgsHittingTotal(t *testing.T, path, flag string, target int) []string {
	t.Helper()
	base := CommandLineBytes(path, []string{flag, ""})
	if target < base {
		t.Fatalf("目标总长 %d 小于不带填充内容时的基线 %d，测试参数设置有误", target, base)
	}
	pad := target - base
	return []string{flag, strings.Repeat("x", pad)}
}

// TestCommandLineGateBoundary 覆盖边界：恰好等于 maxCommandLineBytes 不触发
// （契约"总长 > maxCommandLineBytes"，是严格大于），多一个字节就触发。
// 闸的落点是 spawnProcess（"闸的行为，在 Spawner 起进程之前、与换行那道闸
// 同一处"），用一个必然不存在的路径，闸没生效时会得到"文件找不到"一类的
// 另一种错误，用 errors.Is 与既有 spawn_test.go 同样的手法分辨。
func TestCommandLineGateBoundary(t *testing.T) {
	fakePath := filepath.Join(t.TempDir(), "does-not-exist-boundary.exe")
	const flag = "--system-prompt-override"

	t.Run("恰好等于上限不触发这道闸", func(t *testing.T) {
		args := tstArgsHittingTotal(t, fakePath, flag, maxCommandLineBytes)
		if got := CommandLineBytes(fakePath, args); got != maxCommandLineBytes {
			t.Fatalf("测试构造有误：CommandLineBytes = %d，期望恰好 %d", got, maxCommandLineBytes)
		}
		_, _, exitCode, killed, err := spawnProcess(context.Background(), provider.Command{
			Path: fakePath, Args: args, Dir: t.TempDir(),
		})
		if errors.Is(err, ErrCommandLineTooLong) {
			t.Fatalf("恰好等于上限（不是严格大于）不该触发 ErrCommandLineTooLong，实际触发: %v", err)
		}
		if err == nil {
			t.Fatal("路径本来就不存在，spawnProcess 应当报错（原因是路径找不到，不是这道闸），实际返回 nil")
		}
		if exitCode != 0 || killed {
			t.Errorf("这道闸没生效时不该有 exitCode/killed 的痕迹，实际 exitCode=%d killed=%v", exitCode, killed)
		}
	})

	t.Run("上限之上一个字节就触发", func(t *testing.T) {
		args := tstArgsHittingTotal(t, fakePath, flag, maxCommandLineBytes+1)
		_, _, exitCode, killed, err := spawnProcess(context.Background(), provider.Command{
			Path: fakePath, Args: args, Dir: t.TempDir(),
		})
		if !errors.Is(err, ErrCommandLineTooLong) {
			t.Fatalf("总长 %d（上限+1）应当触发 ErrCommandLineTooLong，实际: %v", maxCommandLineBytes+1, err)
		}
		if exitCode != 0 || killed {
			t.Errorf("闸在起进程之前生效，exitCode/killed 应当是零值，实际 exitCode=%d killed=%v", exitCode, killed)
		}
		if !strings.Contains(err.Error(), flag) {
			t.Errorf("错误信息 %q 里没有点名最长参数对应的 flag %q", err.Error(), flag)
		}
	})
}

// TestCommandLineGateNamesFlagWhenPrecededByDoubleDash 覆盖判定分支 (a)：
// 最长的参数若被一个以 "--" 开头的元素紧跟在前，报成"该 flag 的值"。
func TestCommandLineGateNamesFlagWhenPrecededByDoubleDash(t *testing.T) {
	fakePath := filepath.Join(t.TempDir(), "does-not-exist-flag-branch.exe")
	hugeArg := strings.Repeat("z", maxCommandLineBytes+200)
	args := []string{"--system-prompt-override", hugeArg}

	_, _, _, _, err := spawnProcess(context.Background(), provider.Command{
		Path: fakePath, Args: args, Dir: t.TempDir(),
	})
	if !errors.Is(err, ErrCommandLineTooLong) {
		t.Fatalf("期望 ErrCommandLineTooLong，实际: %v", err)
	}
	if !strings.Contains(err.Error(), "--system-prompt-override") {
		t.Errorf("错误信息 %q 里没有点名 --system-prompt-override", err.Error())
	}
}

// TestCommandLineGateIndexBranchWhenNoPrecedingFlag 覆盖判定分支 (b)：
// 最长的参数在下标 0（前面没有任何元素），必然走"直接报下标"这一支。
// 契约没有给出下标分支的具体措辞（只说"直接报下标"），所以这里只断言
// 触发了正确的哨兵，不锁定措辞的具体格式。
func TestCommandLineGateIndexBranchWhenNoPrecedingFlag(t *testing.T) {
	fakePath := filepath.Join(t.TempDir(), "does-not-exist-index-branch.exe")
	hugeArg := strings.Repeat("z", maxCommandLineBytes+200)
	args := []string{hugeArg} // 下标 0，i>0 不成立，不可能走 flag 分支

	_, _, _, _, err := spawnProcess(context.Background(), provider.Command{
		Path: fakePath, Args: args, Dir: t.TempDir(),
	})
	if !errors.Is(err, ErrCommandLineTooLong) {
		t.Fatalf("期望 ErrCommandLineTooLong，实际: %v", err)
	}
}

// TestCommandLineGateDoesNotMisattributeNonFlagPredecessor 覆盖判定分支 (b)
// 的另一种触发方式：最长参数前面确实有一个元素，但那个元素不以 "--" 开头，
// 不能被误报成"某个 flag 的值"。
func TestCommandLineGateDoesNotMisattributeNonFlagPredecessor(t *testing.T) {
	fakePath := filepath.Join(t.TempDir(), "does-not-exist-nonflag-branch.exe")
	hugeArg := strings.Repeat("z", maxCommandLineBytes+200)
	const notAFlag = "positional-value-not-a-flag"
	args := []string{notAFlag, hugeArg}

	_, _, _, _, err := spawnProcess(context.Background(), provider.Command{
		Path: fakePath, Args: args, Dir: t.TempDir(),
	})
	if !errors.Is(err, ErrCommandLineTooLong) {
		t.Fatalf("期望 ErrCommandLineTooLong，实际: %v", err)
	}
	if strings.Contains(err.Error(), notAFlag+" 的值") {
		t.Errorf("错误信息 %q 把一个不以 -- 开头的元素误报成了 flag，这违反契约里 i>0 且以 -- 开头 两个条件都要满足的判定", err.Error())
	}
}

// ---------------------------------------------------------------------------
// 验收表第 8 条：端到端，三家 provider 各一例
// ---------------------------------------------------------------------------

// tstHugeRoleText 是模拟"超大规则文件展开之后的身份块"的正文。长度选
// maxCommandLineBytes 的量级，足以让 grok / claude 的整条命令行明显越界，
// 不依赖任何精确的字节余量假设。
func tstHugeRoleText() string {
	return strings.Repeat("R", maxCommandLineBytes+500)
}

// TestAcceptance8GrokFailsBeforeSpawnWithHugeRole 覆盖验收表第 8 条 grok 的
// 那一半：grok 把 Role 整段放进 --system-prompt-override 的值（provider/grok.go
// BuildTurn 里 `args = append(args, "--system-prompt-override", req.Role)`，
// 这是允许读的 provider 包，不在禁读名单内），所以超大规则文件直接顶大命令行，
// 在起进程之前就必须以 ErrCommandLineTooLong 失败，错误文本点名这个 flag。
func TestAcceptance8GrokFailsBeforeSpawnWithHugeRole(t *testing.T) {
	p, err := provider.Lookup("grok")
	if err != nil {
		t.Fatalf("provider.Lookup(grok) 失败: %v", err)
	}
	req := provider.TurnRequest{
		AgentID:    "g1",
		Model:      "grok-4.6",
		Role:       tstHugeRoleText(),
		Prompt:     "",
		WorkDir:    t.TempDir(),
		Permission: provider.PermissionRead,
	}
	cmd, err := p.BuildTurn(req)
	if err != nil {
		t.Fatalf("grok BuildTurn 不应在这一步报错（长度闸在 spawn 时才生效），实际: %v", err)
	}
	// BuildTurn 不设置可执行文件路径（由 Runner 用 LookPath 填），
	// 这里手动给一个必然不存在的路径，只为了让 spawnProcess 在长度闸之后
	// 走到"文件找不到"而不是意外真的起了进程。
	cmd.Path = filepath.Join(t.TempDir(), "does-not-exist-grok.exe")

	_, _, exitCode, killed, spawnErr := spawnProcess(context.Background(), cmd)
	if !errors.Is(spawnErr, ErrCommandLineTooLong) {
		t.Fatalf("grok 超大 Role 应当以 ErrCommandLineTooLong 失败，实际: %v", spawnErr)
	}
	if exitCode != 0 || killed {
		t.Errorf("闸在起进程之前生效，exitCode/killed 应当是零值，实际 exitCode=%d killed=%v", exitCode, killed)
	}
	if !strings.Contains(spawnErr.Error(), "--system-prompt-override") {
		t.Errorf("错误信息 %q 里没有点名 --system-prompt-override", spawnErr.Error())
	}
}

// TestAcceptance8ClaudeDoesNotTriggerWithHugeRole 覆盖验收表第 8 条 claude
// 的那一半："claude 同样规则下不触发（它走文件与 stdin）"。claude 的
// BuildTurn 把 Role 写进 Command.Files（落盘）、Prompt 走 Stdin，
// Command.Args 里只有一个固定长度的临时文件路径，同样大小的 Role
// 不会让 Args 变长。
func TestAcceptance8ClaudeDoesNotTriggerWithHugeRole(t *testing.T) {
	p, err := provider.Lookup("claude")
	if err != nil {
		t.Fatalf("provider.Lookup(claude) 失败: %v", err)
	}
	req := provider.TurnRequest{
		AgentID:    "c1",
		Model:      "opus",
		Role:       tstHugeRoleText(),
		Prompt:     "",
		WorkDir:    t.TempDir(),
		Permission: provider.PermissionRead,
	}
	cmd, err := p.BuildTurn(req)
	if err != nil {
		t.Fatalf("claude BuildTurn 失败: %v", err)
	}
	cmd.Path = filepath.Join(t.TempDir(), "does-not-exist-claude.exe")

	_, _, _, _, spawnErr := spawnProcess(context.Background(), cmd)
	if errors.Is(spawnErr, ErrCommandLineTooLong) {
		t.Fatalf("claude 走文件与 stdin，同样大小的 Role 不该触发 ErrCommandLineTooLong，实际: %v", spawnErr)
	}
	if spawnErr == nil {
		t.Fatal("路径本来就不存在，spawnProcess 应当报错（原因是路径找不到），实际返回 nil")
	}
}

// TestAcceptance8AgyBoundaryCase 覆盖验收表第 8 条 agy 的那一半，但要点名
// 一处契约本身的窄缝（详见任务报告"发现"一节）：
//
// agy 的 Identity 策略是 IdentityPromptPreamble——BuildTurn 把 Role 拼进
// "prompt"（Role + "\n" + Prompt），而 checkArgvPrompt 仍然原样保留
// （契约 3.6 明写"两条闸并存"），它用 Capabilities.MaxPromptBytes
// （agy 与 grok 一样是 windowsArgvPromptLimit = 32767）单独判 prompt 本身
// 是否越界。一份真正意义上的"超大规则文件"（Role 远超 32767 字节）会先在
// BuildTurn 内部触发 ErrPromptTooLong，根本走不到 spawn 时的
// ErrCommandLineTooLong 这道新闸——只有 Role 恰好卡在 32767 字节以内、
// 但加上其余 flag（-p、--output-format、--model、--add-dir 等）之后的
// 整条命令行超过 32767 字节的这一小段窄缝，才会真正触发新闸。
// 这正是未决 #15 的原话"挡不住刚好卡在边上的那几十字节"所指的窄缝，
// 这里用一个恰好落在窄缝里的 Role 长度构造出来。
func TestAcceptance8AgyBoundaryCase(t *testing.T) {
	p, err := provider.Lookup("agy")
	if err != nil {
		t.Fatalf("provider.Lookup(agy) 失败: %v", err)
	}
	// role 长度选在"prompt 本身不超 32767" 但"加上其余 flag 之后的整条
	// 命令行超过 32767" 这一段窄缝里：role 长度 32759，加上 BuildTurn
	// 内部拼的 "\n" 分隔符，prompt 总长 32760 字节，仍在 32767 以内。
	role := strings.Repeat("x", 32759)
	req := provider.TurnRequest{
		AgentID:    "a1",
		Model:      "gemini-3.8-flash-high",
		Role:       role,
		Prompt:     "",
		WorkDir:    t.TempDir(),
		Permission: provider.PermissionRead,
	}
	cmd, err := p.BuildTurn(req)
	if err != nil {
		t.Fatalf("agy BuildTurn 在窄缝构造下不应报错（prompt 本身仍在 MaxPromptBytes 以内），实际: %v", err)
	}
	cmd.Path = filepath.Join(t.TempDir(), "does-not-exist-agy.exe")

	total := CommandLineBytes(cmd.Path, cmd.Args)
	if total <= maxCommandLineBytes {
		t.Fatalf("测试构造有误：整条命令行总长 %d 未超过上限 %d，窄缝没有被真正踩中，"+
			"需要调大 role 长度或检查 agy 的 flag 组装是否变化", total, maxCommandLineBytes)
	}

	_, _, exitCode, killed, spawnErr := spawnProcess(context.Background(), cmd)
	if !errors.Is(spawnErr, ErrCommandLineTooLong) {
		t.Fatalf("agy 在窄缝构造下应当以 ErrCommandLineTooLong 失败，实际: %v", spawnErr)
	}
	if exitCode != 0 || killed {
		t.Errorf("闸在起进程之前生效，exitCode/killed 应当是零值，实际 exitCode=%d killed=%v", exitCode, killed)
	}
	// 不断言错误文本点名哪个 flag：agy 的正文紧跟在 "-p" 后面，
	// 而 "-p" 只有一个减号，契约判定分支写的是"以 -- 开头"（两个减号），
	// 单减号的短 flag 不满足这个条件，理应落进"直接报下标"分支——
	// 这也是本文件在报告里指出的契约窄缝之一，不在这里替契约做主张。
}
