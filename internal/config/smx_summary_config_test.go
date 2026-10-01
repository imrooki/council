package config

// SM1 §2.1：[summary] 表的读取、默认 role、两条校验、未知键，以及与 MD1 追加 agents 表的相交。
// 只引用本刀新增的 SummaryConfig / Config.Summary / DefaultSummaryRole，不依赖 CO1：
// 在「HEAD + 本刀 §1.1 A-D」上就能编译运行；在没有本刀的 HEAD 上编译不过是预期的红。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// smxBase 是 §2.1 冻结的配置文本；各用例在它后面追加一段（模拟把表追加到文件末尾）。
const smxBase = "[providers.claude]\n" +
	"command = \"claude\"\n" +
	"\n" +
	"[agents.a]\n" +
	"provider = \"claude\"\n" +
	"model = \"m\"\n" +
	"\n" +
	"[agents.sm]\n" +
	"provider = \"claude\"\n" +
	"model = \"m\"\n" +
	"\n" +
	"[agents.w]\n" +
	"provider = \"claude\"\n" +
	"model = \"m\"\n" +
	"permissions = \"workspace\"\n" +
	"\n" +
	"[agents.f]\n" +
	"provider = \"claude\"\n" +
	"model = \"m\"\n" +
	"permissions = \"full\"\n" +
	"\n" +
	"[agents.r]\n" +
	"provider = \"claude\"\n" +
	"model = \"m\"\n" +
	"role = \"自己写的\"\n"

// smxRoleText 是 §3.1 的默认 role，逐字（测试侧独立抄一份，不引用被测常量来比它自己）。
const smxRoleText = "你是这场讨论的总结人，不参与讨论。每次被叫到时，读用户的问题和各成员的发言，做一致性判断：他们在哪些点上一致，在哪些点上有分歧、各自的依据是什么，有没有人答非所问或明显出错。你的话只给用户看，其余成员看不到；不要替任何一方继续论证，不要提出新的方案。用中文回答：先用一句话给出总的判断，再分条列出一致之处与分歧之处，写得简短。"

// smxMD1Block 是 MD1 契约 §2.3 C1(a) 期望串的原样（C5 用）。
const smxMD1Block = "# council /add 追加：claude 识别出的 claude-opus-5-5[1m]（订阅）\n" +
	"[agents.\"opus-5.5-1m-high\"]\n" +
	"display_name = \"Opus 5.5 1m\"\n" +
	"provider = \"claude\"\n" +
	"model = \"claude-opus-5-5[1m]\"\n" +
	"effort = \"high\"\n" +
	"role = \"\"\n" +
	"permissions = \"read\"\n" +
	"mode = \"discussion\"\n"

// smxWriteCfg 把 body 写进 t.TempDir() 下的 config.toml，返回路径。
func smxWriteCfg(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("装置：写配置失败: %v", err)
	}
	return p
}

// smxErrText 把 nil 记作空串。
func smxErrText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// TestSmxSummaryLoadDefaults 是 C1：Load 成功且 Validate 为 nil，默认 role 只补给总结成员、只补空值。
//
// 【判别力】appended 同时钉「补了」（sm 的 role 从空变成 §3.1）与「只补总结成员」（a 的 role 仍为空：
// 给所有 role 为空的 agent 都补时这里红）；own_role 钉「只补空值」（覆盖用户写的 role 时 r 那一格红，
// 且 sm 的 role 必须仍为空：总结成员不是 sm）。能为假：Load 里那段整个不补时 appended 的第二格是空串，红。
// 对照是 absent（不追加，四格都是原值）与 empty_table（空表 = 没配，什么都不补）。
func TestSmxSummaryLoadDefaults(t *testing.T) {
	type want struct{ agent, sm, a, r string }
	cases := []struct {
		name string
		add  string
		want want
	}{
		{"absent", "", want{"", "", "", "自己写的"}},
		{"appended", "\n[summary]\nagent = \"sm\"\n", want{"sm", smxRoleText, "", "自己写的"}},
		{"own_role", "\n[summary]\nagent = \"r\"\n", want{"r", "", "", "自己写的"}},
		{"empty_table", "\n[summary]\n", want{"", "", "", "自己写的"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Load(smxWriteCfg(t, smxBase+tc.add))
			if err != nil {
				t.Fatalf("Load 失败: %v", err)
			}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate 应当为 nil，实际 %v", err)
			}
			got := want{cfg.Summary.Agent, cfg.Agents["sm"].Role, cfg.Agents["a"].Role, cfg.Agents["r"].Role}
			if got != tc.want {
				t.Fatalf("(Summary.Agent, sm.Role, a.Role, r.Role) 不对\n实际: %q\n期望: %q", got, tc.want)
			}
		})
	}
}

// TestSmxSummaryValidateErrors 是 C2：summary.agent 的两条校验。
//
// 【判别力】取值全集：未定义、大小写不同、workspace、full、read 各一。ghost / upper 钉「未定义」那一条
// （删掉它时两者的错误变成空串，红；做大小写折叠时 upper 变成空串或权限错，红）；
// full 杀「写成 == PermissionWorkspace」（那样 full 漏过，错误是空串）；workspace 与 full 一起杀「权限那一条整个不判」；
// ok 是对照（read 的已定义 agent 不报），证明前四条的错误不是装置本身带来的。
func TestSmxSummaryValidateErrors(t *testing.T) {
	cases := []struct {
		name, id, want string
	}{
		{"ghost", "ghost", `summary.agent: undefined agent "ghost"`},
		{"upper", "SM", `summary.agent: undefined agent "SM"`},
		{"workspace", "w", `summary.agent: agent "w" must have permissions = "read": the summary runs without being asked`},
		{"full", "f", `summary.agent: agent "f" must have permissions = "read": the summary runs without being asked`},
		{"ok", "sm", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Load(smxWriteCfg(t, smxBase+"\n[summary]\nagent = \""+tc.id+"\"\n"))
			if err != nil {
				t.Fatalf("Load 失败: %v", err)
			}
			if got := smxErrText(cfg.Validate()); got != tc.want {
				t.Fatalf("Validate 的错误整串不对\n实际: %q\n期望: %q", got, tc.want)
			}
		})
	}
}

// TestSmxSummaryUnknownKey 是 C3：[summary] 唯一的键是 agent，多写一个键被 Load 以未知键拒绝。
//
// 【判别力】SummaryConfig 若多开一个 mode 字段，Load 成功、错误是空串，红；错误整串含文件路径，
// 证明走的是既有未知键拒绝那一条路（不是别处另写一份）。
func TestSmxSummaryUnknownKey(t *testing.T) {
	p := smxWriteCfg(t, smxBase+"\n[summary]\nagent = \"sm\"\nmode = \"x\"\n")
	_, err := Load(p)
	want := "unknown config keys in " + p + ": summary.mode"
	if got := smxErrText(err); got != want {
		t.Fatalf("Load 的错误整串不对\n实际: %q\n期望: %q", got, want)
	}
}

// TestSmxDefaultSummaryRoleBytes 是 C4：DefaultSummaryRole 整串等于 §3.1。
//
// 【判别力】措辞、全角标点、末尾句号任何一处不同都红（例如去掉末尾「。」）；测试侧的字面量独立抄写，不自比。
func TestSmxDefaultSummaryRoleBytes(t *testing.T) {
	if DefaultSummaryRole != smxRoleText {
		t.Fatalf("DefaultSummaryRole 不对\n实际: %q\n期望: %q", DefaultSummaryRole, smxRoleText)
	}
}

// TestSmxSummaryThenAppendedAgent 是 C5：[summary] 已在文件末尾时，再按 MD1 的形状追加一张 agents 表。
//
// 【判别力】table_after_summary 能为假：追加的键若落进 [summary]（没有表头），它们就成了 summary 的未知键，
// Load 拒绝 —— keys_without_header 与它只差表头那一行，证明同一装置下这条路确实会红，所以前一条的绿说明的是
// 「表头把 [summary] 关上了」，不是「什么都会过」。注释行夹在 [summary] 之后、表头之前也不碍事（两个子测试都带着它）。
// 这是给集成的守卫，不对应 §1.1 里哪一处可变异的代码。
func TestSmxSummaryThenAppendedAgent(t *testing.T) {
	const summary = "\n[summary]\nagent = \"sm\"\n"
	t.Run("table_after_summary", func(t *testing.T) {
		cfg, err := Load(smxWriteCfg(t, smxBase+summary+"\n"+smxMD1Block))
		if err != nil {
			t.Fatalf("Load 失败: %v", err)
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate 应当为 nil，实际 %v", err)
		}
		ag, ok := cfg.Agents["opus-5.5-1m-high"]
		if !ok {
			t.Fatalf("追加的 agent opus-5.5-1m-high 没有被读出来；Agents: %v", cfg.Agents)
		}
		if ag.DisplayName != "Opus 5.5 1m" || ag.Model != "claude-opus-5-5[1m]" || ag.Role != "" {
			t.Fatalf("追加的 agent 字段不对: DisplayName=%q Model=%q Role=%q", ag.DisplayName, ag.Model, ag.Role)
		}
		if cfg.Summary.Agent != "sm" {
			t.Fatalf("Summary.Agent 应当仍是 sm，实际 %q", cfg.Summary.Agent)
		}
		if got := cfg.Agents["sm"].Role; got != smxRoleText {
			t.Fatalf("sm 的默认 role 应当照补\n实际: %q\n期望: %q", got, smxRoleText)
		}
	})
	t.Run("keys_without_header", func(t *testing.T) {
		body := smxBase + summary + "\n" + strings.Replace(smxMD1Block, "[agents.\"opus-5.5-1m-high\"]\n", "", 1)
		p := smxWriteCfg(t, body)
		_, err := Load(p)
		want := "unknown config keys in " + p + ": summary.display_name, summary.effort, summary.mode, summary.model, summary.permissions, summary.provider, summary.role"
		if got := smxErrText(err); got != want {
			t.Fatalf("Load 的错误整串不对\n实际: %q\n期望: %q", got, want)
		}
	})
}
