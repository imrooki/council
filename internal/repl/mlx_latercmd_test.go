package repl

// 本文件覆盖冻结契约 X44 v1.2（E:\multi_model_cowork\x44_contract.md）§2.1 的 L1、L2。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD 80a9385 的源码。
// 本文件引用本刀新增的 HasLaterCommandLine，在 HEAD 上整包编译不过，这是契约 §2.0 写明的预期红。
//
// 不可见字符与特殊空白一律按码位写（契约 §2.0），源码里不放这些字符本身。

import (
	"testing"
)

var (
	mlxBOM  = string(rune(0xFEFF)) // BOM
	mlxZWSP = string(rune(0x200B)) // 零宽空格
	mlxIDSP = string(rune(0x3000)) // 全角空格
	mlxNBSP = string(rune(0x00A0)) // 不换行空格
	mlxFWSL = string(rune(0xFF0F)) // 全角斜杠
)

// TestMlxLaterCommandLineTable：判据的取值表（契约 §2.1 L1）。
//
// 【判别力】每一行都能为假：期望值逐字取自契约表，实现改错一处（§4 X、Z 系列）就有点名的行翻过来。
// J 与 K 成对：J2/K3、J1/K4 只差一个 /；J17/K17 只差 /say 后面那个字符是不是空白；
// J9/K18 都是中间夹空行，只差第 3 行是不是 / 开头。J18、J19 抓「只查最后一行」（Z1），
// J19 另抓「只查第 2 行或最后一行」（Z11）；K18 抓空行算命令的两种写法（Z2、Z16），K19 只抓 Z2。
func TestMlxLaterCommandLineTable(t *testing.T) {
	cases := []struct {
		id   string
		in   string
		want bool
	}{
		{"J1", "/new 真机复验\n/add opus-5.0,grok-4.7", true},
		{"J2", "一句话\n/round rba_a rba_b --go", true},
		{"J3", "a\nb\n/c", true},
		{"J4", "a\n  /c", true},
		{"J5", "a\n\t/c", true},
		{"J6", "a\n" + mlxIDSP + "/c", true},
		{"J7", "a\n" + mlxBOM + "/c", true},
		{"J8", "a\n" + mlxZWSP + "/c", true},
		{"J9", "a\n\n/c", true},
		{"J10", "/say hi\n/round x", true},
		{"J11", "/sayx\n/y", true},
		{"J12", "/SAY\n/y", true},
		{"J13", "a\n//c", true},
		{"J14", "a\n/", true},
		{"J15", "/new t\n/add x\n", true},
		{"J16", "a\n" + mlxNBSP + "/c", true},
		{"J17", "/say" + mlxZWSP + "\n/x", true},
		{"J18", "a\n/b\nc", true},
		{"J19", "a\nb\n/c\nd", true},
		{"K1", "/new 真机复验", false},
		{"K2", "a\nb", false},
		{"K3", "一句话\nround rba_a rba_b --go", false},
		{"K4", "/new 真机复验\nadd opus-5.0,grok-4.7", false},
		{"K5", "a\nb/c", false},
		{"K6", "a\nhttp://x", false},
		{"K7", "\n/new x", false},
		{"K8", mlxBOM + "\n/new x", false},
		{"K9", "/say\n/round x\n/add y", false},
		{"K10", "/say  \n/x", false},
		{"K11", "  /say\n/x", false},
		{"K12", "/new x\n", false},
		{"K13", "a\n" + mlxFWSL + "c", false},
		{"K14", "", false},
		{"K15", "/say", false},
		{"K16", "\t\n  \n", false},
		{"K17", "/say" + mlxNBSP + "\n/x", false},
		{"K18", "a\n\nb", false},
		{"K19", "a\n   \nb", false},
	}
	if len(cases) != 38 {
		t.Fatalf("前提不成立：取值表应有 19 条 J 与 19 条 K，共 38 行，实有 %d 行", len(cases))
	}
	for _, c := range cases {
		if got := HasLaterCommandLine(c.in); got != c.want {
			t.Errorf("%s：HasLaterCommandLine(%q) = %v，契约期望 %v", c.id, c.in, got, c.want)
		}
	}
}

// TestMlxSayExemptionAgreesWithSubmit：出路与 Submit 的真实行为一致（契约 §2.1 L2）。
//
// 【判别力】前五行担保「豁免的输入在 Submit 里真的是一条完整的消息」：出路被删（X7）→ 第一句红。
// 最后一行担保「不豁免的那个边界在 Submit 里真的不是 /say」：豁免改成 TrimFunc(isTrimmable)（X14）→ 第一句红。
// Submit 那几句是 HEAD 行为的交叉核对，能为假：Submit 改了切词规则，输出、MaxSeq、正文会先红。
// 最后一行的「MaxSeq 仍是 1」是缺席断言，对照是前五行（同一装置，MaxSeq 变成 2）。
func TestMlxSayExemptionAgreesWithSubmit(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantBlock bool
		wantOut   string
		wantMax   int64
		wantText  string // wantMax 为 2 时才核对
	}{
		{"say_plain", "/say\n/round x\n/add y", false, "已记下 seq 2\n", 2, "/round x\n/add y"},
		{"say_trail_space", "/say  \n/x", false, "已记下 seq 2\n", 2, "/x"},
		{"say_lead_space", "  /say\n/x", false, "已记下 seq 2\n", 2, "/x"},
		{"say_trail_nbsp", "/say" + mlxNBSP + "\n/x", false, "已记下 seq 2\n", 2, "/x"},
		{"say_maintainer2", "/say\n一句话\n/round rba_a rba_b --go", false, "已记下 seq 2\n", 2, "一句话\n/round rba_a rba_b --go"},
		{"say_zwsp", "/say" + mlxZWSP + "\n/x", true, "未知命令，试 /help\n", 1, ""},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			g := rbaNewRig(t)
			buf := &rbaBuf{}
			r := g.newREPL(t, buf, buf.Reset)
			max0, err := g.svc.MaxSeq(g.ctx, g.convID)
			if err != nil || max0 != 1 {
				t.Fatalf("前提不成立：装置里应只有种子消息（MaxSeq 1），实得 %d，err=%v", max0, err)
			}

			if got := HasLaterCommandLine(c.in); got != c.wantBlock {
				t.Errorf("HasLaterCommandLine(%q) = %v，契约期望 %v", c.in, got, c.wantBlock)
			}
			if err := r.Submit(g.ctx, c.in); err != nil {
				t.Errorf("Submit(%q) 返回 %v，契约期望 nil", c.in, err)
			}
			if got := buf.String(); got != c.wantOut {
				t.Errorf("Submit(%q) 的输出 = %q，契约期望 %q", c.in, got, c.wantOut)
			}
			max1, err := g.svc.MaxSeq(g.ctx, g.convID)
			if err != nil {
				t.Fatalf("读 MaxSeq 失败：%v", err)
			}
			if max1 != c.wantMax {
				t.Errorf("Submit(%q) 之后 MaxSeq = %d，契约期望 %d", c.in, max1, c.wantMax)
			}
			if c.wantMax != 2 {
				return
			}
			msgs, err := g.svc.History(g.ctx, g.convID, 1, 10)
			if err != nil {
				t.Fatalf("读 History 失败：%v", err)
			}
			if len(msgs) != 1 {
				t.Fatalf("Submit(%q) 之后 seq 1 之后应恰好一条消息，实有 %d 条", c.in, len(msgs))
			}
			if msgs[0].Text != c.wantText {
				t.Errorf("Submit(%q) 发出的正文 = %q，契约期望 %q", c.in, msgs[0].Text, c.wantText)
			}
		})
	}
}
