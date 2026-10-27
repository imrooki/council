package localagent

// rules_test.go 用编造的进程列表驱动规则表。这里不枚举真实进程：测试的
// 判别力来自「同一份人造快照 → 固定的识别结果」，真机上有几个 claude
// 与这些断言无关。
//
// 路径用例是这组测试的重点：Claude Desktop 也装了 claude.exe，IDE 里也可能
// 带 codex.exe。只有落在已知 CLI 安装目录里的同名程序才允许 exact。

import (
	"reflect"
	"testing"
	"time"
)

func tstProc(pid, ppid int, exe string) Process {
	return Process{PID: pid, PPID: ppid, ExeName: exe}
}

func tstProcPath(pid, ppid int, exe, path string) Process {
	return Process{PID: pid, PPID: ppid, ExeName: exe, ExePath: path}
}

func TestIdentify(t *testing.T) {
	started := time.Date(2026, 10, 27, 9, 30, 0, 0, time.UTC)

	tests := []struct {
		name  string
		procs []Process
		want  []Agent
	}{
		{
			name: "node.exe 作为父进程的 claude：只有进程关系证据，probable",
			procs: []Process{
				tstProc(200, 100, "node.exe"),
				tstProc(300, 200, "claude.exe"),
			},
			want: []Agent{{
				Kind: KindClaude, PID: 300, PPID: 200, ExeName: "claude.exe",
				Source: SourceObserved, Certainty: CertaintyProbable,
			}},
		},
		{
			name: "npm 目录里的 claude.exe：exact",
			procs: []Process{
				tstProcPath(301, 1, "claude.exe", `C:\Users\me\AppData\Roaming\npm\claude.exe`),
			},
			want: []Agent{{
				Kind: KindClaude, PID: 301, PPID: 1, ExeName: "claude.exe",
				ExePath: `C:\Users\me\AppData\Roaming\npm\claude.exe`,
				Source:  SourceObserved, Certainty: CertaintyExact,
			}},
		},
		{
			name: "原生安装器 .local\\bin 下的 claude.exe：exact",
			procs: []Process{
				tstProcPath(302, 1, "claude.exe", `C:\Users\me\.local\bin\claude.exe`),
			},
			want: []Agent{{
				Kind: KindClaude, PID: 302, PPID: 1, ExeName: "claude.exe",
				ExePath: `C:\Users\me\.local\bin\claude.exe`,
				Source:  SourceObserved, Certainty: CertaintyExact,
			}},
		},
		{
			name: "Claude Desktop 的 claude.exe 明确不报（路径消歧）",
			procs: []Process{
				tstProcPath(303, 1, "claude.exe", `C:\Users\me\AppData\Local\AnthropicClaude\app-2.1.311\claude.exe`),
				tstProcPath(304, 1, "claude.exe", `C:\Program Files\Claude\claude.exe`),
				// 实测的 Store 安装路径：C:\Program Files\WindowsApps\Claude_<版本>_x64__<发布者>\app\claude.exe
				tstProcPath(306, 1, "claude.exe", `C:\Program Files\WindowsApps\Claude_3.6556.2.0_x64__pzs8sxrjxfjjc\app\claude.exe`),
				tstProcPath(308, 1, "claude.exe", `C:\Users\me\AppData\Local\Programs\Claude\claude.exe`),
			},
			want: []Agent{},
		},
		{
			name: "npm 全局目录里带版本后缀的 claude.exe 也是 exact",
			procs: []Process{
				tstProcPath(307, 1, "claude.exe", `C:\Users\me\AppData\Roaming\npm\node_modules\@anthropic-ai\claude-code\bin\claude.exe.old.1793052902872`),
			},
			want: []Agent{{
				Kind: KindClaude, PID: 307, PPID: 1, ExeName: "claude.exe",
				ExePath: `C:\Users\me\AppData\Roaming\npm\node_modules\@anthropic-ai\claude-code\bin\claude.exe.old.1793052902872`,
				Source:  SourceObserved, Certainty: CertaintyExact,
			}},
		},
		{
			name: "拿不到路径的 claude.exe 只标 probable，不当确定",
			procs: []Process{
				tstProcPath(305, 1, "claude.exe", ""),
			},
			want: []Agent{{
				Kind: KindClaude, PID: 305, PPID: 1, ExeName: "claude.exe",
				Source: SourceObserved, Certainty: CertaintyProbable,
			}},
		},
		{
			name: "两个 codex、三个 claude：实例分离且按类型与 PID 排序",
			procs: []Process{
				tstProc(10, 1, "claude.exe"),
				tstProc(30, 1, "codex.exe"),
				tstProc(12, 1, "claude.exe"),
				tstProc(31, 1, "codex.exe"),
				tstProc(11, 1, "claude.exe"),
			},
			want: []Agent{
				{Kind: KindClaude, PID: 10, PPID: 1, ExeName: "claude.exe", Source: SourceObserved, Certainty: CertaintyProbable},
				{Kind: KindClaude, PID: 11, PPID: 1, ExeName: "claude.exe", Source: SourceObserved, Certainty: CertaintyProbable},
				{Kind: KindClaude, PID: 12, PPID: 1, ExeName: "claude.exe", Source: SourceObserved, Certainty: CertaintyProbable},
				{Kind: KindCodex, PID: 30, PPID: 1, ExeName: "codex.exe", Source: SourceObserved, Certainty: CertaintyProbable},
				{Kind: KindCodex, PID: 31, PPID: 1, ExeName: "codex.exe", Source: SourceObserved, Certainty: CertaintyProbable},
			},
		},
		{
			name: "grok 与 agy 各一个",
			procs: []Process{
				tstProc(40, 1, "grok.exe"),
				tstProc(41, 2, "AGY.EXE"),
			},
			want: []Agent{
				{Kind: KindAgy, PID: 41, PPID: 2, ExeName: "AGY.EXE", Source: SourceObserved, Certainty: CertaintyProbable},
				{Kind: KindGrok, PID: 40, PPID: 1, ExeName: "grok.exe", Source: SourceObserved, Certainty: CertaintyProbable},
			},
		},
		{
			name: "codex 的 .cargo\\bin 路径：exact",
			procs: []Process{
				tstProcPath(50, 1, "codex.exe", `C:\Users\me\.cargo\bin\codex.exe`),
			},
			want: []Agent{{
				Kind: KindCodex, PID: 50, PPID: 1, ExeName: "codex.exe",
				ExePath: `C:\Users\me\.cargo\bin\codex.exe`,
				Source:  SourceObserved, Certainty: CertaintyExact,
			}},
		},
		{
			name: "IDE 里带出来的 codex.exe：只有 probable（node_modules 不算安装位置）",
			procs: []Process{
				tstProcPath(51, 1, "codex.exe", `C:\Users\me\AppData\Local\Programs\Cursor\resources\app\node_modules\codex\codex.exe`),
			},
			want: []Agent{{
				Kind: KindCodex, PID: 51, PPID: 1, ExeName: "codex.exe",
				ExePath: `C:\Users\me\AppData\Local\Programs\Cursor\resources\app\node_modules\codex\codex.exe`,
				Source:  SourceObserved, Certainty: CertaintyProbable,
			}},
		},
		{
			name: "OpenAI Codex 安装目录也只标 probable（与 app-server 无法区分）",
			procs: []Process{
				tstProcPath(52, 1, "codex.exe", `C:\Users\me\AppData\Local\OpenAI\Codex\bin\2993767bd1dfb009\codex.exe`),
			},
			want: []Agent{{
				Kind: KindCodex, PID: 52, PPID: 1, ExeName: "codex.exe",
				ExePath: `C:\Users\me\AppData\Local\OpenAI\Codex\bin\2993767bd1dfb009\codex.exe`,
				Source:  SourceObserved, Certainty: CertaintyProbable,
			}},
		},
		{
			name: "启动时间原样带出",
			procs: []Process{{
				PID: 60, PPID: 1, ExeName: "claude.exe", StartedAt: started,
			}},
			want: []Agent{{
				Kind: KindClaude, PID: 60, PPID: 1, ExeName: "claude.exe",
				StartedAt: started, Source: SourceObserved, Certainty: CertaintyProbable,
			}},
		},
		{
			name: "通用宿主一律不认：node / python / pwsh / cmd",
			procs: []Process{
				tstProc(60, 1, "node.exe"),
				tstProc(61, 1, "python.exe"),
				tstProc(62, 1, "python3.exe"),
				tstProc(63, 1, "pwsh.exe"),
				tstProc(64, 1, "powershell.exe"),
				tstProc(65, 1, "cmd.exe"),
			},
			want: []Agent{},
		},
		{
			name: "通用宿主即使有 agent 子进程，自身也不报",
			procs: []Process{
				tstProc(70, 1, "node.exe"),
				tstProc(71, 70, "claude.exe"),
				tstProc(72, 71, "node.exe"), // claude 起的内置 node，不是另一个 agent
			},
			want: []Agent{{
				Kind: KindClaude, PID: 71, PPID: 70, ExeName: "claude.exe",
				Source: SourceObserved, Certainty: CertaintyProbable,
			}},
		},
		{
			name: "名字相近但不是精确命中：一律不认（误判风险用例）",
			procs: []Process{
				tstProc(80, 1, "myclaude.exe"),
				tstProc(81, 1, "claude-helper.exe"),
				tstProc(82, 1, "claude-codex.exe"),
				tstProc(83, 1, "codex.cmd"), // .cmd 不是进程；就算有人塞进来也不猜
				tstProc(84, 1, "claude"),
				tstProc(85, 1, "Cursor.exe"),
				tstProc(86, 1, "Code.exe"),
				tstProc(87, 1, "ollama.exe"),
			},
			want: []Agent{},
		},
		{
			name: "父进程不在快照里时仍按名字判定",
			procs: []Process{
				tstProc(90, 9999, "claude.exe"),
			},
			want: []Agent{{
				Kind: KindClaude, PID: 90, PPID: 9999, ExeName: "claude.exe",
				Source: SourceObserved, Certainty: CertaintyProbable,
			}},
		},
		{
			name: "路径与 exe 名大小写不敏感：npm 目录下的 CLAUDE.EXE 仍是 exact",
			procs: []Process{
				tstProcPath(100, 1, `CLAUDE.EXE`, `C:\Users\Me\AppData\Roaming\NPM\CLAUDE.EXE`),
			},
			want: []Agent{{
				Kind: KindClaude, PID: 100, PPID: 1,
				ExeName: `CLAUDE.EXE`,
				ExePath: `C:\Users\Me\AppData\Roaming\NPM\CLAUDE.EXE`,
				Source:  SourceObserved, Certainty: CertaintyExact,
			}},
		},
		{
			name: "ExeName 里混入完整路径时仍按基名判定（但 ExePath 为空只到 probable）",
			procs: []Process{
				tstProcPath(101, 1, `C:\Users\Me\AppData\Roaming\NPM\CLAUDE.EXE`, ``),
			},
			// 这是调用方拼错快照的情形：基名仍在 ExeName 里，能认出来；
			// 但规则判定所依据的 ExePath 是空的，只能 probable。
			want: []Agent{{
				Kind: KindClaude, PID: 101, PPID: 1,
				ExeName: `C:\Users\Me\AppData\Roaming\NPM\CLAUDE.EXE`,
				Source:  SourceObserved, Certainty: CertaintyProbable,
			}},
		},
		{
			name: "同一个 PID 重复出现只报一条",
			procs: []Process{
				tstProc(110, 1, "codex.exe"),
				tstProc(110, 1, "codex.exe"),
			},
			want: []Agent{{
				Kind: KindCodex, PID: 110, PPID: 1, ExeName: "codex.exe",
				Source: SourceObserved, Certainty: CertaintyProbable,
			}},
		},
		{
			name:  "空快照",
			procs: nil,
			want:  []Agent{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Identify(tc.procs)
			if got == nil {
				t.Fatal("Identify 返回 nil，期望空切片（调用方不该为 nil 单独写一条路）")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("识别结果不符\n实际: %+v\n期望: %+v", got, tc.want)
			}
		})
	}
}

// TestUnknownRulesAreDeliberatelyListed 钉住「通用宿主不是漏写规则，而是
// 被显式判成认不出」：以后有人把这六条删掉、或改成 KindClaude，这里会红。
func TestUnknownRulesAreDeliberatelyListed(t *testing.T) {
	for _, exe := range []string{"node.exe", "python.exe", "python3.exe", "pwsh.exe", "powershell.exe", "cmd.exe"} {
		kind, _, matched := identifyOne(Process{PID: 1, PPID: 2, ExeName: exe}, map[int]Process{
			2: {PID: 2, ExeName: "explorer.exe"},
		})
		if !matched {
			t.Errorf("%s 在规则表里没有对应条目；通用宿主必须显式判成 %q", exe, KindUnknown)
			continue
		}
		if kind != KindUnknown {
			t.Errorf("%s 的规则结论是 %q，期望 %q", exe, kind, KindUnknown)
		}
	}
}

// TestRulesHaveCertainty 钉住规则表的完整性：任何一条会把进程报成四家
// CLI 之一的规则，都必须写清楚 exact 还是 probable。空 certainty 在
// ruleRank 里是 0 分、永远不会命中；没有这条用例，那种漏写只会表现为
// 「某个 agent 偶尔看不见」。
func TestRulesHaveCertainty(t *testing.T) {
	for i, rule := range Rules {
		if rule.Kind == KindUnknown {
			if rule.Certainty != "" {
				t.Errorf("第 %d 条是 KindUnknown 规则（%s），不该带 certainty=%q", i, rule.ExeName, rule.Certainty)
			}
			continue
		}
		switch rule.Certainty {
		case CertaintyExact, CertaintyProbable:
		default:
			t.Errorf("第 %d 条识别规则（%s → %s）的 certainty = %q，期望 exact 或 probable",
				i, rule.ExeName, rule.Kind, rule.Certainty)
		}
	}
}

// TestDesktopExclusionBeatsProbable 钉住证据强度：Claude Desktop 路径同时
// 命中「排除规则」与「按名字的 probable 规则」时，排除规则必须赢；而
// exact 规则又要赢过排除规则。这三条判决由 ruleRank 一处决定。
func TestDesktopExclusionBeatsProbable(t *testing.T) {
	parent := map[int]Process{1: {PID: 1, ExeName: "explorer.exe"}}

	desktop := Process{
		PID: 1, PPID: 1, ExeName: "claude.exe",
		ExePath: `C:\Users\me\AppData\Local\AnthropicClaude\claude.exe`,
	}
	kind, certainty, matched := identifyOne(desktop, parent)
	if !matched || kind != KindUnknown {
		t.Errorf("桌面端路径的判定 = (%q, %q, %v)，期望命中并判成 %q", kind, certainty, matched, KindUnknown)
	}

	npm := Process{
		PID: 1, PPID: 1, ExeName: "claude.exe",
		ExePath: `C:\Users\me\AppData\Roaming\npm\claude.exe`,
	}
	kind, certainty, matched = identifyOne(npm, parent)
	if !matched || kind != KindClaude || certainty != CertaintyExact {
		t.Errorf("npm 路径的判定 = (%q, %q, %v)，期望 claude/exact", kind, certainty, matched)
	}
}
