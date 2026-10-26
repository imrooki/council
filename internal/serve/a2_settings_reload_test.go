package serve

// a2_settings_reload_test.go 覆盖 docs/29_FIXES_PLAN.md A2 的验收，以及
// A3（身份字段也能热更，代价是重置该成员的 provider 会话）：
//
//  1. 只改 mode → reloaded=true、sessions_reset=0，RunCfg 与展示配置是同一份，
//     随后 /api/ask 用该成员能真的跑完一轮；
//  2. 改 model / display_name 等身份字段 → reloaded=true、sessions_reset>0，
//     该成员在全库所有会话里的 provider_sessions 被清空，其它成员一行不动；
//     改完立刻用同一个会话跑一轮不撞身份漂移，且假 provider 收到的 argv 里
//     --model 是新值、没有 --resume（证明是重新开始的新会话）；
//  3. 有 job 在跑时保存身份字段 → 写盘之前就 409 拒绝，文件与会话都没动；
//     只改 mode 时仍走既有降级（reloaded=false + 有轮次正在运行）；
//  4. 新增 / 删除成员 → 一律 reloaded=false；
//  5. 注入一个会失败的 SetConfig → 500，文案说明文件已写入、热加载失败，
//     RunCfg 回滚到旧指针；
//  6. 字段分级表与前端预览的结构锚点。
//
// 能跑的 ask 用例走 COUNCIL_FAKE_CLI=1：测试二进制在子进程里扮演
// claude CLI，整条链（serve -> headless -> turn -> spawn）都是真实实现，
// 只有最外层可执行文件是假的。这与 write_test.go 的端到端同一套路。
// COUNCIL_FAKE_ARGV_FILE 让假 CLI 把自己收到的 argv 追加进一个文件，
// 用来断言「真正发出去的 model 是新值」——配置与指纹都可以在内存里对上，
// argv 是唯一能证明模型真的换了的证据。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/headless"
)

// a2Agent 造一个字段齐全的 agent，供分级表用例改单字段。
func a2Agent() config.AgentConfig {
	return config.AgentConfig{
		DisplayName: "Alpha",
		Provider:    "claude",
		Model:       "sonnet",
		Effort:      "high",
		Role:        "讨论者",
		Permissions: config.PermissionRead,
		Mode:        config.ModeDiscussion,
		Prompt:      "外部系统提示",
	}
}

func a2Cfg(agent config.AgentConfig) *config.Config {
	return &config.Config{
		DataDir: "data",
		Providers: map[string]config.ProviderConfig{
			"claude": {Command: "claude"},
		},
		Agents: map[string]config.AgentConfig{"p1": agent},
		Teams: map[string]config.TeamConfig{
			"t1": {DisplayName: "T1", Members: []string{"p1"}},
		},
		Summary: config.SummaryConfig{Agent: "p1"},
	}
}

// TestA2SettingsRestartReasonsFieldClass 钉住唯一一份字段分级表（A3 之后）：
// mode 可热更且不碰会话；七个身份字段可热更、但必须重置该成员的 provider
// 会话；字段级「必须重启」的列表为空（新增 / 删除 / 其它成员在磁盘上变了
// 仍然由 settingsRestartReasons 报出来）。
func TestA2SettingsRestartReasonsFieldClass(t *testing.T) {
	base := a2Agent()
	t.Run("热更 mode", func(t *testing.T) {
		next := base
		next.Mode = config.ModeJudge
		reasons := settingsRestartReasons(a2Cfg(base), a2Cfg(next), "p1")
		if len(reasons) != 0 {
			t.Errorf("只改 mode 的重启原因 = %v，期望空", reasons)
		}
		if !settingsHotChanged(a2Cfg(base), a2Cfg(next), "p1") {
			t.Errorf("只改 mode 时 settingsHotChanged = false，期望 true")
		}
		if settingsIdentityChanged(a2Cfg(base), a2Cfg(next), "p1") {
			t.Errorf("只改 mode 时 settingsIdentityChanged = true，期望 false（不该重置会话）")
		}
	})

	identity := []struct {
		name  string
		edit  func(*config.AgentConfig)
		field string
	}{
		{"display_name", func(a *config.AgentConfig) { a.DisplayName = "Beta" }, "display_name"},
		{"role", func(a *config.AgentConfig) { a.Role = "裁判" }, "role"},
		{"provider", func(a *config.AgentConfig) { a.Provider = "grok" }, "provider"},
		{"model", func(a *config.AgentConfig) { a.Model = "opus-5.5" }, "model"},
		{"effort", func(a *config.AgentConfig) { a.Effort = "max" }, "effort"},
		{"prompt", func(a *config.AgentConfig) { a.Prompt = "另一段系统提示" }, "prompt"},
		{"permissions", func(a *config.AgentConfig) { a.Permissions = config.PermissionFull }, "permissions"},
	}
	for _, tc := range identity {
		t.Run("热更但不重启 "+tc.name, func(t *testing.T) {
			next := base
			tc.edit(&next)
			if reasons := settingsRestartReasons(a2Cfg(base), a2Cfg(next), "p1"); len(reasons) != 0 {
				t.Errorf("改 %s 的重启原因 = %v，期望空（身份字段热更，代价是重置会话）", tc.name, reasons)
			}
			if !settingsHotChanged(a2Cfg(base), a2Cfg(next), "p1") {
				t.Errorf("改 %s 时 settingsHotChanged = false，期望 true", tc.name)
			}
			if !settingsIdentityChanged(a2Cfg(base), a2Cfg(next), "p1") {
				t.Errorf("改 %s 时 settingsIdentityChanged = false，期望 true（必须重置会话）", tc.name)
			}
			if diffs := agentIdentityFieldDiffs(base, next); !a2Contains(diffs, tc.field) {
				t.Errorf("改 %s 的身份字段差异 = %v，期望包含 %s", tc.name, diffs, tc.field)
			}
		})
	}
	t.Run("名称与角色同时改只算一次重置", func(t *testing.T) {
		next := base
		next.DisplayName = "Beta"
		next.Role = "裁判"
		if !settingsIdentityChanged(a2Cfg(base), a2Cfg(next), "p1") {
			t.Error("display_name 与 role 同时改，settingsIdentityChanged = false，期望 true")
		}
	})

	t.Run("新增成员", func(t *testing.T) {
		next := a2Cfg(base)
		next.Agents["p2"] = base
		reasons := settingsRestartReasons(a2Cfg(base), next, "p2")
		if !a2Contains(reasons, "新增成员 p2") {
			t.Errorf("新增成员的重启原因 = %v，期望包含「新增成员 p2」", reasons)
		}
	})
	t.Run("删除成员", func(t *testing.T) {
		next := a2Cfg(base)
		delete(next.Agents, "p1")
		reasons := settingsRestartReasons(a2Cfg(base), next, "p1")
		if !a2Contains(reasons, "删除了成员 p1") {
			t.Errorf("删除成员的重启原因 = %v，期望包含「删除了成员 p1」", reasons)
		}
	})
	t.Run("待重启字段与 runnable", func(t *testing.T) {
		display := a2Agent()
		display.Model = "opus-5.5"
		runnable, fields := settingsPendingRestart(a2Cfg(base), a2Cfg(display), "p1")
		if !runnable {
			t.Errorf("p1 在 RunCfg 里，runnable = false，期望 true")
		}
		// 身份字段现在热换（代价是重置会话），不该再被画成「待重启」：
		// 预览里说「需要重启」而服务端当场生效，是这一片要消灭的不一致。
		if len(fields) != 0 {
			t.Errorf("model 改动后的待重启字段 = %v，期望空（身份字段热更，不重启）", fields)
		}

		displayAdd := a2Cfg(base)
		displayAdd.Agents["p9"] = base
		runnable, fields = settingsPendingRestart(a2Cfg(base), displayAdd, "p9")
		if runnable {
			t.Errorf("只存在于展示配置的 p9 runnable = true，期望 false")
		}
		if len(fields) != 0 {
			t.Errorf("RunCfg 里没有的成员 pending = %v，期望空（runnable=false 已说明）", fields)
		}
	})
	t.Run("分级表覆盖全部 AgentConfig 字段", func(t *testing.T) {
		known := make(map[string]struct{},
			len(settingsHotFieldNames)+len(settingsIdentityFieldNames)+len(settingsRestartFieldNames))
		for _, list := range [][]string{settingsHotFieldNames, settingsIdentityFieldNames, settingsRestartFieldNames} {
			for _, field := range list {
				if _, dup := known[field]; dup {
					t.Errorf("字段 %q 同时出现在多张分级表里", field)
				}
				known[field] = struct{}{}
			}
		}
		typ := reflect.TypeOf(config.AgentConfig{})
		for i := 0; i < typ.NumField(); i++ {
			tag := strings.Split(typ.Field(i).Tag.Get("toml"), ",")[0]
			if tag == "" || tag == "-" {
				continue
			}
			if _, ok := known[tag]; !ok {
				t.Errorf("AgentConfig.%s 的字段名 %q 不在任何分级列表里：改它会被静默忽略",
					typ.Field(i).Name, tag)
			}
			delete(known, tag)
		}
		for field := range known {
			t.Errorf("分级表里的 %q 不是 AgentConfig 的字段", field)
		}
	})
	t.Run("其它成员在磁盘上变了", func(t *testing.T) {
		next := a2Cfg(base)
		other := base
		other.Model = "opus"
		next.Agents["p2"] = other
		reasons := settingsRestartReasons(a2Cfg(base), next, "p1")
		if !a2Contains(reasons, "成员 p2 的配置在磁盘上变了") {
			t.Errorf("其它成员变化的 reasons = %v，期望点名 p2", reasons)
		}
	})
	t.Run("写盘之前的会话重置判定", func(t *testing.T) {
		// 设置页请求里没有 role / prompt：与当前生效值相同就不算身份变化。
		run := a2Cfg(base)
		same := settingsAgentAfterSave(config.AgentConfig{
			DisplayName: base.DisplayName,
			Provider:    base.Provider,
			Model:       base.Model,
			Effort:      base.Effort,
			Permissions: base.Permissions,
			Mode:        base.Mode,
		}, base)
		if same.Role != base.Role || same.Prompt != base.Prompt {
			t.Fatalf("settingsAgentAfterSave 没把 role / prompt 从 fallback 带过来: %+v", same)
		}
		if settingsSaveResetsSessions(run, "p1", same) {
			t.Error("只改 mode（其余字段与当前一致）时判定成了「会重置会话」")
		}
		same.Model = "opus-5.5"
		if !settingsSaveResetsSessions(run, "p1", same) {
			t.Error("改 model 时没有判定成「会重置会话」")
		}
		// RunCfg 里没有的 id 是新增成员：没有历史会话可重置。
		if settingsSaveResetsSessions(run, "p9", same) {
			t.Error("RunCfg 里没有的新成员被判定成「会重置会话」")
		}
	})
}

func a2Contains(list []string, want string) bool {
	for _, item := range list {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}

// TestA2ModeSaveHotReloads 是验收 1：唯一可热更字段 mode 改动时
// reloaded=true，RunCfg 与展示配置变成同一份，随后用该成员真的跑完一轮。
func TestA2ModeSaveHotReloads(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	env := newGui6Env(t)

	oldRun := env.srv.runCfg()
	if got := oldRun.Agents["p1"].Mode; got != config.ModeDiscussion {
		t.Fatalf("启动时 p1 的 mode = %q，期望 discussion", got)
	}

	res := gui6Save(t, env, map[string]any{
		"agent_id":     "p1",
		"display_name": "Project One",
		"provider":     "claude",
		"model":        "sonnet",
		"effort":       "",
		"permissions":  "read",
		"mode":         "worker",
	})
	if !res.Reloaded {
		t.Fatalf("只改 mode 的 reloaded = false，期望 true；reasons=%v note=%q",
			res.RestartReasons, res.Note)
	}
	if len(res.RestartReasons) != 0 {
		t.Errorf("热更成功却带了 restart_reasons = %v", res.RestartReasons)
	}
	// mode 不进身份指纹：这条路径一个 provider 会话都不许动。
	if res.SessionsReset != 0 {
		t.Errorf("只改 mode 的 sessions_reset = %d，期望 0", res.SessionsReset)
	}
	if strings.Contains(res.Note, "provider 会话已重置") {
		t.Errorf("只改 mode 的 note 里出现了「会话已重置」: %q", res.Note)
	}
	if !strings.Contains(res.Note, "立即生效") || strings.Contains(res.Note, "下次重启") {
		t.Errorf("热更 note = %q，期望说明立即生效且不再要求重启", res.Note)
	}

	run := env.srv.runCfg()
	if run.Agents["p1"].Mode != config.ModeWorker {
		t.Errorf("RunCfg 里 p1 的 mode = %q，热换没有生效", run.Agents["p1"].Mode)
	}
	if run.Agents["p1"].DisplayName != "Project One" {
		t.Errorf("RunCfg 里 p1 的 display_name = %q，期望不变", run.Agents["p1"].DisplayName)
	}
	if run.Agents["p1"].Model != "sonnet" {
		t.Errorf("RunCfg 里 p1 的 model = %q，期望仍是 sonnet", run.Agents["p1"].Model)
	}
	if env.srv.currentCfg() != run {
		t.Errorf("热换之后 RunCfg 与展示配置不是同一个指针：配置可能只换了一处")
	}
	hot := gui6AgentByID(t, gui6Settings(t, env).Agents, "p1")
	if !hot.Runnable || len(hot.PendingRestartFields) != 0 {
		t.Errorf("热换后 p1 runnable=%v pending=%v，期望 true / 空",
			hot.Runnable, hot.PendingRestartFields)
	}

	// 热换真的交给 Runner 了吗：/api/ask 能通过校验并跑完一轮，证明
	// Runner 认识 p1、用的是同一份新配置（不是只改了展示）。
	jobID := postAsk(t, env.writeTestEnv, map[string]any{
		"prompt":  "热换后跑一轮",
		"members": []string{"p1"},
	})
	if status := waitJobStatus(t, env.writeTestEnv, jobID); status != "ok" {
		t.Fatalf("热换后 /api/ask 用 p1 的最终状态 = %q，期望 ok", status)
	}
}

// TestA2HotReloadCallsRunnerSetConfig 钉住生产路径：SetRunnerConfig 为空时
// settingsHotReload 必须把新配置交给 Runner（默认 setter 就是 Rn.SetConfig）。
// 这里装一个 spy，既记录参数又真的调用 SetConfig，避免「只换 RunCfg、没换
// Runner」这种一半热换。
func TestA2HotReloadCallsRunnerSetConfig(t *testing.T) {
	env := newGui6Env(t)
	var got *config.Config
	env.srv.write.SetRunnerConfig = func(cfg *config.Config) error {
		got = cfg
		env.srv.write.Rn.SetConfig(cfg)
		return nil
	}
	t.Cleanup(func() { env.srv.write.SetRunnerConfig = nil })

	res := gui6Save(t, env, map[string]any{
		"agent_id":     "p1",
		"display_name": "Project One",
		"provider":     "claude",
		"model":        "sonnet",
		"effort":       "",
		"permissions":  "read",
		"mode":         "worker",
	})
	if !res.Reloaded {
		t.Fatalf("只改 mode 的 reloaded = false，热换没有走到 SetRunnerConfig")
	}
	if got == nil {
		t.Fatal("SetRunnerConfig 没有被调用")
	}
	if got.Agents["p1"].Mode != config.ModeWorker {
		t.Errorf("交给 Runner 的 mode = %q，期望新值", got.Agents["p1"].Mode)
	}
	if env.srv.runCfg() != got {
		t.Errorf("RunCfg 与交给 Runner 的配置不是同一个指针")
	}
}

// TestA2EmptyHotListDegradesToRestart 钉住「热更列表允许为空」这条退化路径：
// 把 mode 也临时移进重启列表、清空热更列表，保存 mode 必须变成需要重启，
// 且 SetRunnerConfig 不被调用。防止以后有人把列表清空后留下一段永远走不到
// 的热换分支，或让 null/[] 的 JSON 形状把前端兜底表又带回来。
func TestA2EmptyHotListDegradesToRestart(t *testing.T) {
	oldHot, oldRestart := settingsHotFieldNames, settingsRestartFieldNames
	settingsHotFieldNames = nil
	settingsRestartFieldNames = append(append([]string{}, oldRestart...), "mode")
	t.Cleanup(func() {
		settingsHotFieldNames = oldHot
		settingsRestartFieldNames = oldRestart
	})

	policy := settingsReloadPolicy()
	if policy.HotFields == nil || len(policy.HotFields) != 0 {
		t.Errorf("空热更列表的 policy.HotFields = %#v，期望非 nil 空切片（JSON 必须是 []）", policy.HotFields)
	}

	env := newGui6Env(t)
	calls := 0
	env.srv.write.SetRunnerConfig = func(*config.Config) error {
		calls++
		return nil
	}
	t.Cleanup(func() { env.srv.write.SetRunnerConfig = nil })

	res := gui6Save(t, env, map[string]any{
		"agent_id":     "p1",
		"display_name": "Project One",
		"provider":     "claude",
		"model":        "sonnet",
		"effort":       "",
		"permissions":  "read",
		"mode":         "worker",
	})
	if res.Reloaded {
		t.Errorf("热更列表为空时改 mode 的 reloaded = true，期望 false")
	}
	if !a2Contains(res.RestartReasons, "mode 变了") {
		t.Errorf("restart_reasons = %v，期望包含「mode 变了」", res.RestartReasons)
	}
	if calls != 0 {
		t.Errorf("热更列表为空时仍调用了 SetRunnerConfig %d 次", calls)
	}
	if got := env.srv.runCfg().Agents["p1"].Mode; got != config.ModeDiscussion {
		t.Errorf("RunCfg 的 mode = %q，期望未被热换", got)
	}
}

// seedA2ProviderSession 种一条 provider_sessions 记录，模拟「这个成员在
// 这条会话里已经按旧身份开过口」。同一份 helper 给 p1 / p2 各写一条，
// 是为了让「只重置被改的那个成员」有可比对象。
func seedA2ProviderSession(t *testing.T, env *gui6Env, convID, agentID string) {
	t.Helper()
	ctx := context.Background()
	ps := conversation.ProviderSession{
		ConvID:            convID,
		AgentID:           agentID,
		ProviderSessionID: "sess-a2-" + agentID,
		CursorSeq:         1,
		CostTotalUSD:      0.25,
		HaveCostTotal:     true,
		LastTurnAt:        time.Now().UTC(),
		IdentityFP:        "fp-before-a2-" + agentID,
		Spoken:            true,
	}
	if err := env.store.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.SetProviderSession(ctx, ps)
	}); err != nil {
		t.Fatalf("种 %s 的 provider_sessions 记录失败: %v", agentID, err)
	}
}

// TestA2DisplayNameChangeHotReloadsAndResetsSessions 是验收 3：display_name
// 进 renderIdentityCore → 身份指纹，所以它和 model 一样是「热更 + 重置
// 会话」。一次种两条会话里的 p1 记录、另一场里 p2 的记录，钉住三件事：
// 该成员**所有会话**都被清、别的成员一行不动、Runner 真的拿到了新配置。
func TestA2DisplayNameChangeHotReloadsAndResetsSessions(t *testing.T) {
	env := newGui6Env(t)
	ctx := context.Background()
	convA := gui3aSeedConversation(t, env.writeTestEnv, "A2 改名会话 A")
	convB := gui3aSeedConversation(t, env.writeTestEnv, "A2 改名会话 B")
	seedA2ProviderSession(t, env, convA.ID, "p1")
	seedA2ProviderSession(t, env, convB.ID, "p1")
	seedA2ProviderSession(t, env, convA.ID, "p2")

	setConfigCalls := 0
	env.srv.write.SetRunnerConfig = func(cfg *config.Config) error {
		setConfigCalls++
		env.srv.write.Rn.SetConfig(cfg)
		return nil
	}
	t.Cleanup(func() { env.srv.write.SetRunnerConfig = nil })

	res := gui6Save(t, env, map[string]any{
		"agent_id":     "p1",
		"display_name": "会话开着的成员改名",
		"provider":     "claude",
		"model":        "sonnet",
		"effort":       "",
		"permissions":  "read",
		"mode":         "discussion",
	})
	if !res.Reloaded {
		t.Fatalf("改 display_name 的 reloaded = false，期望 true；reasons=%v", res.RestartReasons)
	}
	if len(res.RestartReasons) != 0 {
		t.Errorf("热换成功却带了 restart_reasons = %v", res.RestartReasons)
	}
	if res.SessionsReset != 2 {
		t.Errorf("sessions_reset = %d，期望 2（p1 在两条会话里各一行）", res.SessionsReset)
	}
	for _, want := range []string{"已热加载", "该成员的 provider 会话已重置", "下一轮用新模型重新开始"} {
		if !strings.Contains(res.Note, want) {
			t.Errorf("note = %q，期望包含 %q", res.Note, want)
		}
	}
	if setConfigCalls != 1 {
		t.Errorf("SetRunnerConfig 被调用 %d 次，期望恰好 1 次", setConfigCalls)
	}

	for _, conv := range []conversation.Conversation{convA, convB} {
		if _, ok, err := env.store.ProviderSession(ctx, conv.ID, "p1"); err != nil || ok {
			t.Errorf("会话 %s 里 p1 的 provider_sessions 仍在: ok=%v err=%v", conv.ID, ok, err)
		}
	}
	other, ok, err := env.store.ProviderSession(ctx, convA.ID, "p2")
	if err != nil || !ok {
		t.Fatalf("p2 的记录被一起删掉了: ok=%v err=%v", ok, err)
	}
	if other.ProviderSessionID != "sess-a2-p2" || other.IdentityFP != "fp-before-a2-p2" {
		t.Errorf("p2 的记录被改动了: %+v", other)
	}

	if got := env.srv.runCfg().Agents["p1"].DisplayName; got != "会话开着的成员改名" {
		t.Errorf("RunCfg 的 display_name = %q，期望写盘后的新值", got)
	}
	if env.srv.currentCfg() != env.srv.runCfg() {
		t.Error("热换之后 RunCfg 与展示配置不是同一个指针")
	}
	if got := gui6Read(t, env.projectConfig); !strings.Contains(got, `display_name = "会话开着的成员改名"`) {
		t.Errorf("配置文件里没有新 display_name:\n%s", got)
	}
}

// TestA2ModelChangeHotReloadsAndResetsSessions 是验收 1 + 2：改 model 当场
// 生效、清掉该成员的全部 provider_sessions、其它成员不动、配置文件已写；
// 随后用**同一个会话**立刻再跑一轮，不撞身份漂移，且假 provider 收到的
// argv 里 --model 是新值、没有 --resume（证明拿到的是重建后的新会话，
// 而不是接着旧会话说话）。
//
// 首轮必须真跑一次：只有真跑过，Runner 的内存闸里才会有按 sonnet 算出来的
// 快照——那正是「不重置会话就必然漂移」的那份状态。只种库行测不出闸这一半。
func TestA2ModelChangeHotReloadsAndResetsSessions(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	argvPath := filepath.Join(t.TempDir(), "fake-argv.log")
	t.Setenv("COUNCIL_FAKE_ARGV_FILE", argvPath)
	env := newGui6Env(t)
	ctx := context.Background()

	firstJob := postAsk(t, env.writeTestEnv, map[string]any{
		"prompt":  "改 model 之前的一轮",
		"members": []string{"p1"},
	})
	if status := waitJobStatus(t, env.writeTestEnv, firstJob); status != "ok" {
		t.Fatalf("首轮最终状态 = %q，期望 ok", status)
	}
	first := a2LastArgv(t, argvPath)
	if got := a2ArgValue(first, "--model"); got != "sonnet" {
		t.Fatalf("首轮 argv 的 --model = %q，期望 sonnet；argv=%v", got, first)
	}

	before, err := env.store.AllProviderSessions(ctx)
	if err != nil {
		t.Fatalf("读 provider_sessions 失败: %v", err)
	}
	var firstPS conversation.ProviderSession
	for _, ps := range before {
		if ps.AgentID == "p1" {
			firstPS = ps
		}
	}
	if firstPS.ConvID == "" || firstPS.ProviderSessionID == "" {
		t.Fatalf("首轮之后没有 p1 的 provider_sessions 记录: %+v", before)
	}
	// 同场会话里的另一个成员：重置只许作用于被改的那个成员。
	seedA2ProviderSession(t, env, firstPS.ConvID, "p2")

	res := gui6Save(t, env, map[string]any{
		"agent_id":     "p1",
		"display_name": "Project One",
		"provider":     "claude",
		"model":        "opus-5.5",
		"effort":       "",
		"permissions":  "read",
		"mode":         "discussion",
	})
	if !res.Reloaded {
		t.Fatalf("改 model 的 reloaded = false，期望 true；reasons=%v", res.RestartReasons)
	}
	if res.SessionsReset != 1 {
		t.Errorf("sessions_reset = %d，期望 1（只有 p1 那一行）", res.SessionsReset)
	}
	for _, want := range []string{"已热加载", "该成员的 provider 会话已重置", "下一轮用新模型重新开始"} {
		if !strings.Contains(res.Note, want) {
			t.Errorf("note = %q，期望包含 %q", res.Note, want)
		}
	}
	if got := env.srv.runCfg().Agents["p1"].Model; got != "opus-5.5" {
		t.Errorf("RunCfg 里 p1 的 model = %q，期望 opus-5.5", got)
	}
	if env.srv.currentCfg() != env.srv.runCfg() {
		t.Error("热换之后 RunCfg 与展示配置不是同一个指针")
	}
	if got := gui6Read(t, env.projectConfig); !strings.Contains(got, `model = "opus-5.5"`) {
		t.Errorf("配置文件里没有新 model:\n%s", got)
	}
	// 会话详情里的成员模型也要跟着换：热换之后「成员与费用」若还显示 sonnet，
	// 用户会以为这次保存没有生效（participants 表没有 model 列，模型名从
	// 当前配置取）。
	var detail conversationDetail
	getJSON(t, env.http.URL+"/api/conversations/"+firstPS.ConvID, http.StatusOK, &detail)
	var memberModel string
	for _, m := range detail.Members {
		if m.AgentID == "p1" {
			memberModel = m.Model
		}
	}
	if memberModel != "opus-5.5" {
		t.Errorf("会话详情里 p1 的 model = %q，期望热换后的 opus-5.5", memberModel)
	}
	if _, ok, err := env.store.ProviderSession(ctx, firstPS.ConvID, "p1"); err != nil || ok {
		t.Errorf("保存 model 之后 p1 的 provider_sessions 仍在: ok=%v err=%v", ok, err)
	}
	if other, ok, err := env.store.ProviderSession(ctx, firstPS.ConvID, "p2"); err != nil || !ok {
		t.Errorf("p2 的记录被一起删掉了: ok=%v err=%v", ok, err)
	} else if other.ProviderSessionID != "sess-a2-p2" {
		t.Errorf("p2 的记录被改动了: %+v", other)
	}

	// 立即用同一个会话跑一轮：不重置会话的话，这一轮会拿着内存里的旧指纹
	// 直接 ErrIdentityDrift，job 状态不可能是 ok。
	secondJob := postAsk(t, env.writeTestEnv, map[string]any{
		"prompt":       "热换后同一会话再跑一轮",
		"members":      []string{"p1"},
		"conversation": firstPS.ConvID,
	})
	if status := waitJobStatus(t, env.writeTestEnv, secondJob); status != "ok" {
		t.Fatalf("热换后用同一会话跑一轮的最终状态 = %q，期望 ok（出现身份漂移会失败）；job=%s", status, secondJob)
	}
	second := a2LastArgv(t, argvPath)
	if got := a2ArgValue(second, "--model"); got != "opus-5.5" {
		t.Errorf("第二轮 argv 的 --model = %q，期望 opus-5.5；argv=%v", got, second)
	}
	if a2HasArg(second, "--resume") {
		t.Errorf("第二轮 argv 里出现了 --resume：会话没有被重置，仍然在续接旧会话；argv=%v", second)
	}

	// 第二轮之后的那一行是新身份写下的：指纹与旧值不同、会话 id 是假 CLI
	// 这一轮新建的那条。它证明「重建」真的发生了，而不是旧行被悄悄改成新值。
	rebuilt, ok, err := env.store.ProviderSession(ctx, firstPS.ConvID, "p1")
	if err != nil || !ok {
		t.Fatalf("第二轮之后应当有一条新的 provider_sessions 记录: ok=%v err=%v", ok, err)
	}
	if rebuilt.ProviderSessionID != "fake-session" {
		t.Errorf("新记录的会话 id = %q，期望假 CLI 新建的 fake-session", rebuilt.ProviderSessionID)
	}
	if rebuilt.IdentityFP == firstPS.IdentityFP {
		t.Errorf("新记录的 identity_fp 与旧值相同（%q）：新 model 没有进指纹", rebuilt.IdentityFP)
	}
}

// a2LastArgv 读假 provider 记录的最后一次 argv（COUNCIL_FAKE_ARGV_FILE）。
func a2LastArgv(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读假 CLI 的 argv 记录失败: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var args []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(lines[len(lines)-1])), &args); err != nil {
		t.Fatalf("argv 记录不是合法 JSON 行: %v\n%s", err, data)
	}
	return args
}

// a2ArgValue 在 argv 里找 flag 后面紧跟的那个值；找不到返回空串。
func a2ArgValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// a2HasArg 报告 argv 里有没有这个元素。
func a2HasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// TestA2RunningJobBlocksHotReload 是验收 3：有 job 在跑时改唯一可热更的
// mode，必须退化成 reloaded=false，且原因里出现「有轮次正在运行」。
func TestA2RunningJobBlocksHotReload(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	t.Setenv("COUNCIL_FAKE_SLEEP_ALL_MS", "3000")
	env := newGui6Env(t)

	jobID := postAsk(t, env.writeTestEnv, map[string]any{
		"prompt":  "慢轮次占住运行态",
		"members": []string{"p1"},
	})
	// postAsk 返回时 job 已经在任务表里；等它真的起来（子进程开始跑）再保存，
	// 避免「刚 add、还没执行」这种边界被误当成已结束。
	a2WaitRunning(t, env, jobID)

	res := gui6Save(t, env, map[string]any{
		"agent_id":     "p1",
		"display_name": "Project One",
		"provider":     "claude",
		"model":        "sonnet",
		"effort":       "",
		"permissions":  "read",
		"mode":         "worker",
	})
	if res.Reloaded {
		t.Fatalf("有 job 在跑时 reloaded = true，期望 false")
	}
	if !a2Contains(res.RestartReasons, "有轮次正在运行") {
		t.Errorf("restart_reasons = %v，期望说明「有轮次正在运行」", res.RestartReasons)
	}
	if !strings.Contains(res.Note, "有轮次正在运行") {
		t.Errorf("note = %q，期望写明有轮次正在运行", res.Note)
	}
	if got := env.srv.runCfg().Agents["p1"].Mode; got != config.ModeDiscussion {
		t.Errorf("RunCfg 里 p1 的 mode = %q，期望未被热换", got)
	}

	// 停掉慢轮次，别把 3 秒的等待留给清理流程。
	if status, _, raw := postRaw(t,
		env.http.URL+"/api/jobs/"+jobID+"/cancel", "application/json", `{}`); status != http.StatusAccepted {
		t.Fatalf("取消慢轮次 = %d，期望 202；body=%s", status, raw)
	}
	if status := waitJobStatus(t, env.writeTestEnv, jobID); status == "ok" {
		t.Errorf("被取消的慢轮次最终状态 = ok，期望非 ok")
	}
}

// TestA2JobTableRunningCountIgnoresFinished 钉住热换的判据：结束后仍留在
// 表里等 TTL 清理的 job 不算「正在跑」，否则一次 60 秒内结束过的议会会
// 一直挡住设置页热换。
func TestA2JobTableRunningCountIgnoresFinished(t *testing.T) {
	table := newJobTable()
	finished := newJob("finished")
	table.add(finished)
	finished.finish(headlessResultOK(), nil)
	if got := table.runningCount(); got != 0 {
		t.Errorf("只有已结束 job 时 runningCount = %d，期望 0（TTL 内的残留不算运行中）", got)
	}

	running := newJob("running")
	table.add(running)
	if got := table.runningCount(); got != 1 {
		t.Errorf("加入未结束 job 后 runningCount = %d，期望 1", got)
	}
	running.finish(headlessResultOK(), nil)
	if got := table.runningCount(); got != 0 {
		t.Errorf("job 结束后 runningCount = %d，期望 0", got)
	}
}

// headlessResultOK 只给 runningCount 用例一个合法的结束状态；runningCount
// 只关心 finished 标志，不读 Result 的其它字段。
func headlessResultOK() headless.Result {
	return headless.Result{Status: "ok"}
}

// a2WaitRunning 等到任务表里的 job 进入「未结束」状态。job 是在 goroutine
// 启动之前就进表的，所以这个条件几乎立刻为真；留一个短轮询是为了不依赖
// 调度顺序，也不引入固定 sleep。
func a2WaitRunning(t *testing.T, env *gui6Env, jobID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		j, ok := env.srv.jobs.get(jobID)
		if ok && !j.isFinished() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s 5 秒内没有进入运行态", jobID)
}

// TestA2AddAndDeleteRequireRestart 是验收 4：新增 / 删除成员一律不热换。
func TestA2AddAndDeleteRequireRestart(t *testing.T) {
	env := newGui6Env(t)

	add := gui6Save(t, env, map[string]any{
		"agent_id":     "p9",
		"display_name": "Project Nine",
		"provider":     "claude",
		"model":        "opus",
		"effort":       "",
		"permissions":  "read",
		"mode":         "discussion",
	})
	if add.Reloaded {
		t.Errorf("新增成员 reloaded = true，期望 false")
	}
	if !a2Contains(add.RestartReasons, "新增成员 p9") {
		t.Errorf("新增成员 restart_reasons = %v，期望包含「新增成员 p9」", add.RestartReasons)
	}
	if _, ok := env.srv.runCfg().Agents["p9"]; ok {
		t.Errorf("新增成员被热换进了 RunCfg，期望只能重启后可用")
	}

	status, raw := gui6Post(t, env.http.URL+"/api/settings/agents/p2/delete", `{}`)
	if status != http.StatusOK {
		t.Fatalf("删除 p2 = %d，期望 200；body=%s", status, raw)
	}
	var del settingsWriteResponse
	if err := json.Unmarshal(raw, &del); err != nil {
		t.Fatalf("删除响应不是合法 JSON: %v；%s", err, raw)
	}
	if del.Reloaded {
		t.Errorf("删除成员 reloaded = true，期望 false")
	}
	if !a2Contains(del.RestartReasons, "删除了成员 p2") {
		t.Errorf("删除成员 restart_reasons = %v，期望包含「删除了成员 p2」", del.RestartReasons)
	}
	if _, ok := env.srv.runCfg().Agents["p2"]; !ok {
		t.Errorf("删除成员被热换出了 RunCfg，期望删除也只在重启后生效")
	}
}

// TestA3IdentitySaveWithRunningJobRefusedBeforeWrite 是验收 4：有轮次在跑
// 时保存身份字段，在写盘之前就 409 拒绝——配置文件、备份、另一层的文件、
// provider_sessions、RunCfg 一个都不许动。
//
// 为什么选「写之前拒绝」而不是「写完但不热换」：身份字段必须与「重置该成员
// 的 provider 会话」同时生效，而重置只能在没有轮次时做。先写盘再退化，会
// 留下「磁盘上是新 model、Runner 还是旧的、会话还在」的中间态，而且下一次
// 保存之前谁也看不出它没生效。拒绝时什么都没动，用户等这一轮跑完再点一次
// 保存即可。
func TestA3IdentitySaveWithRunningJobRefusedBeforeWrite(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	t.Setenv("COUNCIL_FAKE_SLEEP_ALL_MS", "3000")
	env := newGui6Env(t)
	ctx := context.Background()
	conv := gui3aSeedConversation(t, env.writeTestEnv, "有轮次在跑时保存身份字段")
	// 种子放在 p2 上：正在跑的这轮是 p1 的，它会自己写 p1 的记录；
	// 用 p2 才能干净地断言「这次被拒绝的保存没有删任何会话」。
	seedA2ProviderSession(t, env, conv.ID, "p2")

	before := gui6Read(t, env.projectConfig)
	globalBefore := gui6Read(t, env.globalConfig)

	jobID := postAsk(t, env.writeTestEnv, map[string]any{
		"prompt":  "慢轮次占住运行态",
		"members": []string{"p1"},
	})
	a2WaitRunning(t, env, jobID)

	status, raw := gui6Post(t, env.http.URL+"/api/settings/agents",
		`{"agent_id":"p1","display_name":"Project One","provider":"claude","model":"opus-5.5","effort":"","permissions":"read","mode":"discussion"}`)
	if status != http.StatusConflict {
		t.Fatalf("有轮次在跑时改 model = %d，期望 409；body=%s", status, raw)
	}
	var reply struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("409 不是合法 JSON: %v；%s", err, raw)
	}
	for _, want := range []string{"有轮次正在运行", "provider 会话", "没有写入任何配置文件"} {
		if !strings.Contains(reply.Error, want) {
			t.Errorf("409 文案 = %q，期望包含 %q", reply.Error, want)
		}
	}

	if got := gui6Read(t, env.projectConfig); got != before {
		t.Errorf("被拒绝的保存改动了配置文件")
	}
	if got := gui6Read(t, env.globalConfig); got != globalBefore {
		t.Errorf("被拒绝的保存改动了另一层配置文件")
	}
	if backups := gui6Backups(t, env.projectConfig); len(backups) != 0 {
		t.Errorf("被拒绝的保存生成了备份: %v", backups)
	}
	if _, ok, err := env.store.ProviderSession(ctx, conv.ID, "p2"); err != nil || !ok {
		t.Errorf("被拒绝的保存动了 provider_sessions: ok=%v err=%v", ok, err)
	}
	if got := env.srv.runCfg().Agents["p1"].Model; got != "sonnet" {
		t.Errorf("RunCfg 里 p1 的 model = %q，期望仍是 sonnet", got)
	}

	// 停掉慢轮次，别把 3 秒的等待留给清理流程。
	if status, _, raw := postRaw(t,
		env.http.URL+"/api/jobs/"+jobID+"/cancel", "application/json", `{}`); status != http.StatusAccepted {
		t.Fatalf("取消慢轮次 = %d，期望 202；body=%s", status, raw)
	}
	if status := waitJobStatus(t, env.writeTestEnv, jobID); status == "ok" {
		t.Errorf("被取消的慢轮次最终状态 = ok，期望非 ok")
	}
}

// TestA3ModeSaveDoesNotResetSessions 是验收 5：mode 不影响身份指纹，热换它
// 不触发会话重置——sessions_reset 必须是 0，已有记录原样保留，note 里
// 也不能出现「会话已重置」。
func TestA3ModeSaveDoesNotResetSessions(t *testing.T) {
	env := newGui6Env(t)
	ctx := context.Background()
	conv := gui3aSeedConversation(t, env.writeTestEnv, "只改 mode 不重置会话")
	seedA2ProviderSession(t, env, conv.ID, "p1")

	res := gui6Save(t, env, map[string]any{
		"agent_id":     "p1",
		"display_name": "Project One",
		"provider":     "claude",
		"model":        "sonnet",
		"effort":       "",
		"permissions":  "read",
		"mode":         "worker",
	})
	if !res.Reloaded {
		t.Fatalf("只改 mode 的 reloaded = false，期望 true；reasons=%v", res.RestartReasons)
	}
	if res.SessionsReset != 0 {
		t.Errorf("只改 mode 的 sessions_reset = %d，期望 0", res.SessionsReset)
	}
	if strings.Contains(res.Note, "已重置") {
		t.Errorf("只改 mode 的 note 里出现了会话重置: %q", res.Note)
	}
	if got := env.srv.runCfg().Agents["p1"].Mode; got != config.ModeWorker {
		t.Errorf("RunCfg 里 p1 的 mode = %q，期望 worker", got)
	}
	got, ok, err := env.store.ProviderSession(ctx, conv.ID, "p1")
	if err != nil || !ok {
		t.Fatalf("只改 mode 时 provider_sessions 被删了: ok=%v err=%v", ok, err)
	}
	if got.ProviderSessionID != "sess-a2-p1" || got.IdentityFP != "fp-before-a2-p1" {
		t.Errorf("只改 mode 时 provider_sessions 被改动了: %+v", got)
	}
}

// TestA3SettingsReloadPolicyJSONShape 钉住分级表的 JSON 形状：identity_fields
// 必须非 nil（前端据此说「上下文会丢」），restart_fields 为空时必须是 []
// 而不是 null（null 会让前端退回内置兜底表，把身份字段又说成需要重启）。
func TestA3SettingsReloadPolicyJSONShape(t *testing.T) {
	policy := settingsReloadPolicy()
	if policy.IdentityFields == nil {
		t.Error("policy.IdentityFields = nil，期望非 nil 切片")
	}
	if policy.RestartFields == nil {
		t.Error("policy.RestartFields = nil，期望非 nil 空切片（JSON 必须是 []）")
	}
	if len(policy.RestartFields) != 0 {
		t.Errorf("policy.RestartFields = %v，期望空（所有 agent 字段都能热更）", policy.RestartFields)
	}
	for _, field := range []string{"display_name", "role", "provider", "model", "effort", "prompt", "permissions"} {
		if !a2Contains(policy.IdentityFields, field) {
			t.Errorf("identity_fields = %v，期望包含 %s", policy.IdentityFields, field)
		}
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("分级表编码失败: %v", err)
	}
	for _, want := range []string{
		`"hot_fields":["mode"]`,
		`"identity_fields":[`,
		`"restart_fields":[]`,
		`"display_name"`,
		`"permissions"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("分级表 JSON = %s，期望包含 %s", raw, want)
		}
	}
	// 三张表互不重叠：同一个字段不能又是「不碰会话」又是「重置会话」。
	seen := map[string]string{}
	for name, list := range map[string][]string{
		"hot": policy.HotFields, "identity": policy.IdentityFields, "restart": policy.RestartFields,
	} {
		for _, field := range list {
			if prev, dup := seen[field]; dup {
				t.Errorf("字段 %q 同时出现在 %s 与 %s 表里", field, prev, name)
			}
			seen[field] = name
		}
	}
}

// TestA2HotReloadFailureRollsBack 是验收 5：注入一个会失败的 SetConfig，
// 必须 500、说明文件已写入，并把 RunCfg 回滚到旧指针。
func TestA2HotReloadFailureRollsBack(t *testing.T) {
	env := newGui6Env(t)
	oldRun := env.srv.runCfg()
	if env.srv.write.SetRunnerConfig != nil {
		t.Fatalf("测试装配不应预置 SetRunnerConfig")
	}
	env.srv.write.SetRunnerConfig = func(*config.Config) error {
		return errors.New("注入的 SetConfig 失败")
	}
	t.Cleanup(func() { env.srv.write.SetRunnerConfig = nil })

	body := `{"agent_id":"p1","display_name":"Project One","provider":"claude","model":"sonnet","effort":"","permissions":"read","mode":"worker"}`
	status, raw := gui6Post(t, env.http.URL+"/api/settings/agents", body)
	if status != http.StatusInternalServerError {
		t.Fatalf("热换失败 = %d，期望 500；body=%s", status, raw)
	}
	var reply struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("500 不是合法 JSON: %v；%s", err, raw)
	}
	for _, want := range []string{"配置已写入", "热加载失败", "回滚", "重启 council serve 后"} {
		if !strings.Contains(reply.Error, want) {
			t.Errorf("500 文案 = %q，期望包含 %q", reply.Error, want)
		}
	}

	// 文件已经写盘（不能回滚文件）；RunCfg 回到旧指针；展示配置是新值。
	if got := gui6Read(t, env.projectConfig); !strings.Contains(got, `mode = "worker"`) {
		t.Errorf("配置文件里没有新 mode，期望文件已写入")
	}
	if env.srv.runCfg() != oldRun {
		t.Errorf("热换失败后 RunCfg 没有回滚到旧指针")
	}
	if got := env.srv.runCfg().Agents["p1"].Mode; got != config.ModeDiscussion {
		t.Errorf("回滚后 RunCfg 的 mode = %q，期望 discussion", got)
	}
	if got := env.srv.currentCfg().Agents["p1"].Mode; got != config.ModeWorker {
		t.Errorf("展示配置的 mode = %q，期望保持写盘后的新值", got)
	}
}

// TestA2IndexSettingsReloadHints 钉住前端新增的结构锚点与三套文案（热更 /
// 热更但重置会话 / 需要重启）。真正的点击、预览与结果区渲染由 CDP 用真实
// Chromium 验收。
func TestA2IndexSettingsReloadHints(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		`id="settings-preview-reload"`,
		`settingsPreviewReload`,
		`settingsPreviewVerdict`,
		`hot_fields`,
		`identity_fields`,
		`restart_fields`,
		`pending_restart_fields`,
		`current.runnable === false`,
		`'名称或角色变了'`,
		`'model 变了'`,
		`t('ui.settings.previewRestart', '本次改动需要重启 serve 后生效：{reasons}'`,
		`t('ui.settings.previewHot',`,
		`'保存后立即生效`,
		// A3：身份字段的预览与结果两处都必须说出「上下文不再续接」。
		`t('ui.settings.previewReset', '保存后立即生效；该成员的 provider 会话会被重置，之前的上下文不再续接。')`,
		`t('ui.settings.resetResult', '已热加载；该成员的 provider 会话已重置，下一轮用新模型重新开始。')`,
		`resetSessions`,
		`result.sessions_reset`,
		`'is-reset'`,
		`t('ui.settings.hotResult', '已热加载：本次改动立即生效，无需重启 serve。')`,
		`t('ui.settings.restartResult', '需要重启 serve 后生效：{reasons}。'`,
		`t('ui.settings.noRuntimeChange', '运行配置没有变化，无需重启 serve。')`,
		`result.restart_reasons`,
		`settingsResult.className = 'settings-result '`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}
	if strings.Contains(html, "innerHTML") {
		t.Errorf("index.html 里出现了 innerHTML，前端必须保持 0 处")
	}
}
