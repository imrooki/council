package repl

// 本文件覆盖 AT1 冻结契约（E:\multi_model_cowork\at1_contract.md，v1.1 + W1b 修订）§2.1 的 C1、C2：
// @ 补全候选 MentionCandidates，与判据 MentionBoundaryOK、MentionTokenRune 跟真解析器 ParseMentions 的一致性。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD 3d134ee 的源码。
// 本文件引用本刀新增的 MentionCandidate、MentionCandidates、MentionBoundaryOK、MentionTokenRune，
// 在 HEAD 上整包编译不过，这是契约 §2.0 写明的预期红。
// 包级标识符一律以 atx 开头；不加 TestMain（本包已有 session_inflight_test.go 的那一个）。

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
)

// atxCfg 是 §2.1 的配置 cfg(sum)：五个 agent，Summary.Agent = sum。
// alpha 的显示名依次夹着换行、tab、DEL、C1 控制字符 CSI（0x9b），末尾是 ESC（v1.1）。
func atxCfg(sum string) *config.Config {
	alphaName := "A" + string(rune(0x0a)) + "B" + string(rune(0x09)) + "C" + string(rune(0x7f)) +
		"D" + string(rune(0x9b)) + "E" + string(rune(0x1b))
	return &config.Config{
		Agents: map[string]config.AgentConfig{
			"zeta":  {DisplayName: "Zeta 名"},
			"alpha": {DisplayName: alphaName},
			"sum":   {DisplayName: "Sum"},
			"left":  {DisplayName: "Left"},
			"mid":   {DisplayName: "  "},
		},
		Summary: config.SummaryConfig{Agent: sum},
	}
}

// atxCandRig 按 §2.1 的装置建库、建会话、按给定顺序加入、让 leave 里的离开，造 REPL。
// 返回 REPL 与会话 id；/use 由用例自己提交。
func atxCandRig(t *testing.T, cfg *config.Config, join []string, leave []string) (*REPL, string) {
	t.Helper()
	ctx := context.Background()
	svc := newRoundStreamService(t)
	conv, err := svc.Create(ctx, "atx")
	if err != nil {
		t.Fatalf("装置：建会话失败: %v", err)
	}
	for _, id := range join {
		if err := svc.Join(ctx, conv.ID, conversation.Participant{
			AgentID: id, Provider: "claude", JoinedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("装置：加入 %s 失败: %v", id, err)
		}
	}
	for _, id := range leave {
		if err := svc.Leave(ctx, conv.ID, id, time.Now().UTC()); err != nil {
			t.Fatalf("装置：%s 离开失败: %v", id, err)
		}
	}
	r := New(cfg, svc, nil, nil, &bytes.Buffer{}, RulesContext{})
	return r, conv.ID
}

// atxUse 提交 /use；失败是前提不成立。
func atxUse(t *testing.T, r *REPL, convID string) {
	t.Helper()
	if err := r.Submit(context.Background(), "/use "+convID); err != nil {
		t.Fatalf("前提不成立：/use %s 返回错误: %v", convID, err)
	}
}

// TestAtxCandidates 是 C1。
//
// 【判别力】第 2 步一条整值比较同时钉住：去总结成员（C1 变异：sum 出现）、只取活跃（C2：left 出现）、
// 显示名清洗（C4：alpha 带着控制字符原样出现；C9：mid 成了两个空格；C10：只认 r < 0x20 时
// DEL 与 CSI 原样留下）。DEL、CSI 夹在字母中间，每一个都单独看得见。
// 第 1 步：/use 之前没有会话，必须是 (nil, nil)；去掉 r.cur == nil 那道门（C5）会空指针 panic。
// 第 3 步是第 2 步「sum 不在」的正对照：同样的 sum 成员，没配总结成员时就在。
// 第 4 步：只剩总结成员时是长度 0 的非 nil 切片（能为假：返回 nil 就红）。
// 第 5 步：svc 为 nil 时 (nil, nil)。
// 第 6 步：已取消的 ctx 读库失败，错误原样返回、切片 nil（C8 把错误吞掉时 err 为 nil，红）。
// 排序（C3）按 store 的约定等价，本用例看不出，只在审查时看（契约 §4）。
func TestAtxCandidates(t *testing.T) {
	ctx := context.Background()

	// 第 1、2、6 步共用一个 REPL。
	r, convID := atxCandRig(t, atxCfg("sum"), []string{"zeta", "sum", "gone", "alpha", "left", "mid"}, []string{"left"})

	// 第 1 步：/use 之前。
	got, err := r.MentionCandidates(ctx)
	if got != nil || err != nil {
		t.Fatalf("第 1 步：/use 之前应当是 (nil, nil)，实际 (%#v, %v)", got, err)
	}

	// 第 2 步：/use 之后。
	atxUse(t, r, convID)
	got, err = r.MentionCandidates(ctx)
	want := []MentionCandidate{
		{ID: "alpha", Name: "A B C D E"},
		{ID: "gone", Name: ""},
		{ID: "mid", Name: ""},
		{ID: "zeta", Name: "Zeta 名"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("第 2 步：应当是 (%#v, nil)，实际 (%#v, %v)", want, got, err)
	}

	// 第 3 步（对照）：没有总结成员时 sum 照列。
	r3, conv3 := atxCandRig(t, atxCfg(""), []string{"zeta", "sum"}, nil)
	atxUse(t, r3, conv3)
	got3, err3 := r3.MentionCandidates(ctx)
	want3 := []MentionCandidate{{ID: "sum", Name: "Sum"}, {ID: "zeta", Name: "Zeta 名"}}
	if err3 != nil || !reflect.DeepEqual(got3, want3) {
		t.Errorf("第 3 步（对照）：应当是 (%#v, nil)，实际 (%#v, %v)", want3, got3, err3)
	}

	// 第 4 步：只剩总结成员。
	r4, conv4 := atxCandRig(t, atxCfg("sum"), []string{"sum"}, nil)
	atxUse(t, r4, conv4)
	got4, err4 := r4.MentionCandidates(ctx)
	if err4 != nil || got4 == nil || len(got4) != 0 {
		t.Errorf("第 4 步：只剩总结成员时应当是长度 0 的非 nil 切片、err nil，实际 (%#v, nil=%v, %v)", got4, got4 == nil, err4)
	}

	// 第 5 步：svc 为 nil。
	r5 := New(atxCfg("sum"), nil, nil, nil, nil, RulesContext{})
	got5, err5 := r5.MentionCandidates(ctx)
	if got5 != nil || err5 != nil {
		t.Errorf("第 5 步：svc 为 nil 时应当是 (nil, nil)，实际 (%#v, %v)", got5, err5)
	}

	// 第 6 步：第 2 步那个 REPL 用已取消的 ctx 调。
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	got6, err6 := r.MentionCandidates(cctx)
	if got6 != nil || err6 == nil {
		t.Errorf("第 6 步：已取消的 ctx 应当返回 nil 切片与非 nil 错误，实际 (%#v, %v)", got6, err6)
	}
}

// TestAtxCandHelpersMatchParser 是 C2：拿真解析器 ParseMentions 当判据。
//
// 【判别力】左边界与字符类任何一处与 ParseMentions 分叉都红。rune 集合里两类结果都有
// （字母数字与 _@.- 挡左边界、空白与中文不挡；字母数字与 _.- 是 token 字符、其余不是），
// 所以恒真的实现（永远 true 或永远 false）都红。C6：去掉 prev >= 0x80 时 `中`（0x4e2d）的低字节 0x2d 是 `-`，
// 判成被挡，而 ParseMentions 按字节看不挡；C7：去掉 r < 0x80 时 `中` 判成 token 字符。
func TestAtxCandHelpersMatchParser(t *testing.T) {
	var set []rune
	for c := rune(0x20); c <= 0x7e; c++ {
		set = append(set, c)
	}
	set = append(set, 0x4e2d, 0x3000, 0xff0c, 0xe9, 0x0a)

	var nB, nT int
	for _, c := range set {
		s := string(c)
		wantB := len(ParseMentions(s+"@ab")) == 1
		if got := MentionBoundaryOK(c); got != wantB {
			t.Errorf("MentionBoundaryOK(%U) = %v，ParseMentions(%q) 判的是 %v", c, got, s+"@ab", wantB)
		}
		pm := ParseMentions("@a" + s + "b")
		wantT := len(pm) == 1 && pm[0] == strings.ToLower("a"+s+"b")
		if got := MentionTokenRune(c); got != wantT {
			t.Errorf("MentionTokenRune(%U) = %v，ParseMentions(%q) = %q 判的是 %v", c, got, "@a"+s+"b", pm, wantT)
		}
		if wantB {
			nB++
		}
		if wantT {
			nT++
		}
	}
	// 前提：真解析器在这个集合上两种结果都给过（不挡左边界的 / 被挡的；token 字符 / 不是），
	// 排除「判据恒为某一个值」让上面的逐个比较变空。
	// 预期：被挡的是 62 个字母数字与 _@.- 共 66 个，其余 34 个不挡；token 字符 65 个（字母数字与 _.-）。
	if nB != len(set)-66 || nT != 65 {
		t.Errorf("前提不成立：集合 %d 个里 ParseMentions 判不挡左边界 %d 个（应当 %d）、token 字符 %d 个（应当 65）",
			len(set), nB, len(set)-66, nT)
	}
}
