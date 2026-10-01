package app

// x39f_driftnotice_test.go 覆盖 X-39f 契约 2.2：启动漂移提示按后果分两句
// （L1 会重建类、L2 被拒类），以及「/session rebuild 清掉已删 agent 的
// 悬空行、重启后提示消失」的端到端闭环。
//
// 不引用新 API（DriftKind / AskRefused），在 7343f0c 上能编译。
// 字面量断言一律整行相等；否定断言用 strings.Contains 在整段输出上判。

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
	"council/internal/provider"
	"council/internal/store"
	"council/internal/turn"
)

// x39fReplPrompt 是 REPL 的提示符（internal/repl/repl.go 的 prompt）。
const x39fReplPrompt = "council> "

// x39fL1 / x39fL2 是契约第 3 节冻结的两句启动提示，%d 代入后去掉结尾换行。
func x39fL1(n int) string {
	return fmt.Sprintf("注意: 有 %d 条 provider 会话的身份已变，下次 /ask 会各重建一次（每条一次入场费）", n)
}

func x39fL2(n int) string {
	return fmt.Sprintf("注意: 有 %d 条 provider 会话按当前配置 /ask 会被拒，不花钱；其中原因是「配置里已无此 agent」的，先 /use 那个会话再敲 /session rebuild <成员> 即可清掉", n)
}

// x39fS2 是契约第 3 节 S2（/session rebuild 成功且该行被拒类）。
func x39fS2(agentID string) string {
	return fmt.Sprintf("已清掉 %s 的 provider 会话（会话 id / 游标 / 成本基线 / 上轮时刻 / 指纹 五样全丢）；按当前配置对它 /ask 会被拒，不花钱", agentID)
}

// x39fLines 把输出按 "\n" 切行，每行反复剥掉开头的提示符。
func x39fLines(out string) []string {
	raw := strings.Split(out, "\n")
	lines := make([]string, len(raw))
	for i, l := range raw {
		for strings.HasPrefix(l, x39fReplPrompt) {
			l = strings.TrimPrefix(l, x39fReplPrompt)
		}
		lines[i] = l
	}
	return lines
}

// x39fLineIndex 返回第一行全等于 want 的行号，没有则 -1。
func x39fLineIndex(out, want string) int {
	for i, l := range x39fLines(out) {
		if l == want {
			return i
		}
	}
	return -1
}

// x39fKind6FP 造一份能解、但各字段都与当前配置不同的指纹（第 6 类种子）。
func x39fKind6FP() string {
	return turn.IdentityFingerprint("old role", "old-model", "", provider.PermissionRead, turn.RulesBlocks{}, "old-digest")
}

// x39fSeedProviderSessionFP 与 drift_test.go 的 seedMismatchedProviderSession
// 同一个做法（每次新建一个会话），只是指纹由调用方给。
func x39fSeedProviderSessionFP(t *testing.T, dataDir, agentID, fp string) (convID string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("打开 store 失败: %v", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Errorf("关闭 store 失败: %v", err)
		}
	}()

	svc := conversation.NewService(st)
	conv, err := svc.Create(ctx, "x39f 启动提示用例")
	if err != nil {
		t.Fatalf("建会话失败: %v", err)
	}
	err = st.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.SetProviderSession(ctx, conversation.ProviderSession{
			ConvID:            conv.ID,
			AgentID:           agentID,
			ProviderSessionID: "x39f-seeded-session",
			CursorSeq:         0,
			LastTurnAt:        time.Now().UTC(),
			IdentityFP:        fp,
		})
	})
	if err != nil {
		t.Fatalf("写入种子 provider 会话失败: %v", err)
	}
	return conv.ID
}

// x39fBootstrap 写配置、跑第一次 Run 建库，返回配置路径与数据目录。
func x39fBootstrap(t *testing.T) (cfgPath, dataDir, dir string) {
	t.Helper()
	isolateAppEnv(t)
	dir = t.TempDir()
	dataDir = filepath.Join(dir, "data")
	cfgPath = writeTestConfig(t, dir, validConfigBody(dataDir))
	if _, errOut, err := runAppForTest(t, context.Background(), cfgPath, strings.NewReader("")); err != nil {
		t.Fatalf("第一次 Run（建库）应当返回 nil，实际 %v；Err:\n%s", err, errOut)
	}
	return cfgPath, dataDir, dir
}

// x39fRun 跑一次 Run，要求返回 nil，返回 Out。
func x39fRun(t *testing.T, cfgPath, stdin string) string {
	t.Helper()
	out, errOut, err := runAppForTest(t, context.Background(), cfgPath, strings.NewReader(stdin))
	if err != nil {
		t.Fatalf("Run 应当返回 nil，实际 %v；Err:\n%s\nOut:\n%s", err, errOut, out)
	}
	return out
}

// x39fForbid 断言整段输出不含任何一个禁用词。
func x39fForbid(t *testing.T, out string, words ...string) {
	t.Helper()
	for _, w := range words {
		if strings.Contains(out, w) {
			t.Errorf("输出不该含 %q；完整输出:\n%s", w, out)
		}
	}
}

// TestX39fStartupOnlyAgentGone 是 A1：库里只有一行第 1 类（ghost）。
func TestX39fStartupOnlyAgentGone(t *testing.T) {
	cfgPath, dataDir, _ := x39fBootstrap(t)
	convID := seedMismatchedProviderSession(t, dataDir, "ghost")

	out := x39fRun(t, cfgPath, "")
	if x39fLineIndex(out, x39fL2(1)) < 0 {
		t.Errorf("应当有一行全等于 %q；完整输出:\n%s", x39fL2(1), out)
	}
	x39fForbid(t, out, "入场费", "身份已变")
	found := false
	for _, l := range x39fLines(out) {
		if strings.Contains(l, convID) && strings.Contains(l, "ghost") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("明细里应当有一行同时点名会话 %q 与成员 ghost；完整输出:\n%s", convID, out)
	}
}

// TestX39fStartupOnlyPermissionRefused 是 A1b：只有一行被拒类，而且是第 3 类
// （theory 权限档改成 full）。专杀 app 里按 Kind==DriftAgentGone 分类。
func TestX39fStartupOnlyPermissionRefused(t *testing.T) {
	cfgPath, dataDir, dir := x39fBootstrap(t)
	seedMismatchedProviderSession(t, dataDir, "theory")

	body := validConfigBody(dataDir)
	const from, to = `permissions = "read"`, `permissions = "full"`
	if n := strings.Count(body, from); n != 1 {
		t.Fatalf("前提不成立：validConfigBody 里 %q 应当恰好出现 1 次，实际 %d 次", from, n)
	}
	if got := writeTestConfig(t, dir, strings.Replace(body, from, to, 1)); got != cfgPath {
		t.Fatalf("改写的配置路径 %q 与原路径 %q 不同", got, cfgPath)
	}

	out := x39fRun(t, cfgPath, "")
	if x39fLineIndex(out, x39fL2(1)) < 0 {
		t.Errorf("应当有一行全等于 %q；完整输出:\n%s", x39fL2(1), out)
	}
	x39fForbid(t, out, "入场费", "身份已变")
	if !strings.Contains(out, "权限档不放行") {
		t.Errorf("前提检查：明细里应当出现第 3 类的 Reason「权限档不放行」；完整输出:\n%s", out)
	}
}

// TestX39fStartupBothKindsCountsAndOrder 是 A2：第 5 类一行 + 第 1 类两行，
// L1(1) 在 L2(2) 之前，三个会话都列在 L2 之后。
func TestX39fStartupBothKindsCountsAndOrder(t *testing.T) {
	cfgPath, dataDir, _ := x39fBootstrap(t)
	convs := []string{
		seedMismatchedProviderSession(t, dataDir, "theory"),
		seedMismatchedProviderSession(t, dataDir, "ghost"),
		seedMismatchedProviderSession(t, dataDir, "ghost2"),
	}

	out := x39fRun(t, cfgPath, "")
	i1 := x39fLineIndex(out, x39fL1(1))
	i2 := x39fLineIndex(out, x39fL2(2))
	if i1 < 0 {
		t.Errorf("应当有一行全等于 %q；完整输出:\n%s", x39fL1(1), out)
	}
	if i2 < 0 {
		t.Errorf("应当有一行全等于 %q；完整输出:\n%s", x39fL2(2), out)
	}
	if i1 >= 0 && i2 >= 0 && i1 >= i2 {
		t.Errorf("L1 应当在 L2 之前（L1 在第 %d 行，L2 在第 %d 行）；完整输出:\n%s", i1, i2, out)
	}
	if i2 >= 0 {
		after := x39fLines(out)[i2+1:]
		for _, c := range convs {
			hit := false
			for _, l := range after {
				if strings.Contains(l, c) {
					hit = true
					break
				}
			}
			if !hit {
				t.Errorf("会话 %q 应当出现在 L2 之后的明细里；完整输出:\n%s", c, out)
			}
		}
	}
}

// TestX39fStartupSessionRebuildClearsDanglingRow 是 A3：端到端清理闭环。
// 第二次 Run 里 /session rebuild ghost（ghost 不是成员）打 S2；第三次 Run
// 的横幅里不再有任何漂移提示。
func TestX39fStartupSessionRebuildClearsDanglingRow(t *testing.T) {
	cfgPath, dataDir, _ := x39fBootstrap(t)
	convID := seedMismatchedProviderSession(t, dataDir, "ghost")

	out2 := x39fRun(t, cfgPath, "/use "+convID+"\n/session rebuild ghost\n")
	if x39fLineIndex(out2, x39fS2("ghost")) < 0 {
		t.Errorf("/session rebuild ghost 应当打出一行全等于 %q；完整输出:\n%s", x39fS2("ghost"), out2)
	}

	out3 := x39fRun(t, cfgPath, "")
	x39fForbid(t, out3, "注意:", "会被拒")
}

// TestX39fStartupOnlyUnparsable 是 A4（A1 的对照）：只有第 5 类一行。
func TestX39fStartupOnlyUnparsable(t *testing.T) {
	cfgPath, dataDir, _ := x39fBootstrap(t)
	seedMismatchedProviderSession(t, dataDir, "theory")

	out := x39fRun(t, cfgPath, "")
	if x39fLineIndex(out, x39fL1(1)) < 0 {
		t.Errorf("应当有一行全等于 %q；完整输出:\n%s", x39fL1(1), out)
	}
	x39fForbid(t, out, "会被拒")
}

// TestX39fStartupOnlyFieldsChanged 是 A5（A4 只差类别）：只有第 6 类一行。
func TestX39fStartupOnlyFieldsChanged(t *testing.T) {
	cfgPath, dataDir, _ := x39fBootstrap(t)
	x39fSeedProviderSessionFP(t, dataDir, "theory", x39fKind6FP())

	out := x39fRun(t, cfgPath, "")
	if x39fLineIndex(out, x39fL1(1)) < 0 {
		t.Errorf("应当有一行全等于 %q；完整输出:\n%s", x39fL1(1), out)
	}
	x39fForbid(t, out, "会被拒")
	// 前提检查：明细是逐字段差异（第 6 类），不是「指纹格式不可解」。
	if strings.Contains(out, "指纹格式不可解") || !strings.Contains(out, "old-model") {
		t.Errorf("前提不成立：种子应当落第 6 类（明细点名 old-model），完整输出:\n%s", out)
	}
}
