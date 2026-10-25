package serve

// a2_settings_reload_test.go 覆盖 docs/29_FIXES_PLAN.md A2 的验收：
//
//  1. 只改 mode → reloaded=true，RunCfg 与展示配置是同一份，随后 /api/ask
//     用该成员能真的跑完一轮；
//  2. 改 model → reloaded=false，restart_reasons 点名 model，执行层仍是旧值；
//  3. 成员已有 provider_sessions 记录时改 display_name → 必须走重启分支，
//     且 SetRunnerConfig 一次都不许被调用（display_name 进身份指纹）；
//  4. 有 job 在跑时改可热更的 mode → 不热换，reloaded=false 且说明
//     「有轮次正在运行」；
//  5. 新增 / 删除成员 → 一律 reloaded=false；
//  6. 注入一个会失败的 SetConfig → 500，文案说明文件已写入、热加载失败，
//     RunCfg 回滚到旧指针；
//  7. 字段分级表与前端预览的结构锚点。
//
// 能跑的 ask 用例走 COUNCIL_FAKE_CLI=1：测试二进制在子进程里扮演
// claude CLI，整条链（serve -> headless -> turn -> spawn）都是真实实现，
// 只有最外层可执行文件是假的。这与 write_test.go 的端到端同一套路。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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

// TestA2SettingsRestartReasonsFieldClass 钉住唯一一份字段分级表：
// 只有 mode 可热更；display_name / role 与其他五个字段各进重启原因
// （display_name 与 role 共用「名称或角色变了」这一条）。
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
	})

	restart := []struct {
		name string
		edit func(*config.AgentConfig)
		want string
	}{
		{"display_name", func(a *config.AgentConfig) { a.DisplayName = "Beta" }, "名称或角色变了"},
		{"role", func(a *config.AgentConfig) { a.Role = "裁判" }, "名称或角色变了"},
		{"provider", func(a *config.AgentConfig) { a.Provider = "grok" }, "provider 变了"},
		{"model", func(a *config.AgentConfig) { a.Model = "opus-5.5" }, "model 变了"},
		{"effort", func(a *config.AgentConfig) { a.Effort = "max" }, "effort 变了"},
		{"prompt", func(a *config.AgentConfig) { a.Prompt = "另一段系统提示" }, "prompt 变了"},
		{"permissions", func(a *config.AgentConfig) { a.Permissions = config.PermissionFull }, "permissions 变了"},
	}
	for _, tc := range restart {
		t.Run("重启 "+tc.name, func(t *testing.T) {
			next := base
			tc.edit(&next)
			reasons := settingsRestartReasons(a2Cfg(base), a2Cfg(next), "p1")
			if !a2Contains(reasons, tc.want) {
				t.Errorf("改 %s 的重启原因 = %v，期望包含 %q", tc.name, reasons, tc.want)
			}
		})
	}
	t.Run("名称与角色同时改只报一条", func(t *testing.T) {
		next := base
		next.DisplayName = "Beta"
		next.Role = "裁判"
		reasons := settingsRestartReasons(a2Cfg(base), a2Cfg(next), "p1")
		count := 0
		for _, reason := range reasons {
			if reason == "名称或角色变了" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("display_name 与 role 同时改，原因 = %v，期望「名称或角色变了」恰好一条", reasons)
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
		if !a2Contains(fields, "model") {
			t.Errorf("待重启字段 = %v，期望包含 model", fields)
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
		known := make(map[string]struct{}, len(settingsHotFieldNames)+len(settingsRestartFieldNames))
		for _, field := range settingsHotFieldNames {
			known[field] = struct{}{}
		}
		for _, field := range settingsRestartFieldNames {
			if _, dup := known[field]; dup {
				t.Errorf("字段 %q 同时在热更与重启列表里", field)
			}
			known[field] = struct{}{}
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

// TestA2DisplayNameWithProviderSessionRequiresRestart 是防回归线：
// 成员已经有 provider_sessions 记录（说明这个进程里/provider 那边已经有
// 一条按旧身份建立的会话）时，保存 display_name 必须走重启分支，绝不能
// 触发 SetRunnerConfig。display_name 进 renderIdentityCore → 身份指纹，
// 热换会让下一轮直接 ErrIdentityDrift；这里用「记录存在 + SetConfig 调用
// 次数为 0」把这条边界钉死，防止以后有人又把它误判成可热更。
func TestA2DisplayNameWithProviderSessionRequiresRestart(t *testing.T) {
	env := newGui6Env(t)
	ctx := context.Background()
	conv := gui3aSeedConversation(t, env.writeTestEnv, "A2 已有 provider 会话")

	ps := conversation.ProviderSession{
		ConvID:            conv.ID,
		AgentID:           "p1",
		ProviderSessionID: "sess-a2-before",
		CursorSeq:         1,
		CostTotalUSD:      0.25,
		HaveCostTotal:     true,
		LastTurnAt:        time.Now().UTC(),
		IdentityFP:        "fp-before-a2",
		Spoken:            true,
	}
	if err := env.store.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		return tx.SetProviderSession(ctx, ps)
	}); err != nil {
		t.Fatalf("种 provider_sessions 记录失败: %v", err)
	}
	if _, ok, err := env.store.ProviderSession(ctx, conv.ID, "p1"); err != nil || !ok {
		t.Fatalf("provider_sessions 里没有 p1 的记录: ok=%v err=%v", ok, err)
	}

	calls := 0
	env.srv.write.SetRunnerConfig = func(*config.Config) error {
		calls++
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
	if res.Reloaded {
		t.Fatalf("已有 provider 会话时改 display_name 的 reloaded = true，期望 false")
	}
	if !a2Contains(res.RestartReasons, "名称或角色变了") {
		t.Errorf("restart_reasons = %v，期望包含「名称或角色变了」", res.RestartReasons)
	}
	if calls != 0 {
		t.Errorf("display_name 改动触发了 SetRunnerConfig %d 次；进指纹的字段绝不热换", calls)
	}
	if got := env.srv.runCfg().Agents["p1"].DisplayName; got != "Project One" {
		t.Errorf("RunCfg 的 display_name = %q，期望仍是旧值", got)
	}
	if got := env.srv.currentCfg().Agents["p1"].DisplayName; got != "会话开着的成员改名" {
		t.Errorf("展示配置的 display_name = %q，期望写盘后的新值", got)
	}
	if _, ok, err := env.store.ProviderSession(ctx, conv.ID, "p1"); err != nil || !ok {
		t.Errorf("保存 display_name 后 provider_sessions 记录不该被碰: ok=%v err=%v", ok, err)
	}
}

// TestA2ModelChangeRequiresRestart 是验收 2：改 model 不热换，
// restart_reasons 点名 model，RunCfg 仍是启动时的 sonnet，ask 仍按旧配置执行。
func TestA2ModelChangeRequiresRestart(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	env := newGui6Env(t)

	res := gui6Save(t, env, map[string]any{
		"agent_id":     "p1",
		"display_name": "Project One",
		"provider":     "claude",
		"model":        "opus-5.5",
		"effort":       "",
		"permissions":  "read",
		"mode":         "discussion",
	})
	if res.Reloaded {
		t.Fatalf("改 model 的 reloaded = true，期望 false")
	}
	if !a2Contains(res.RestartReasons, "model 变了") {
		t.Errorf("改 model 的 restart_reasons = %v，期望包含「model 变了」", res.RestartReasons)
	}
	if !strings.Contains(res.Note, "重启") {
		t.Errorf("note = %q，期望说明需要重启", res.Note)
	}
	if got := env.srv.runCfg().Agents["p1"].Model; got != "sonnet" {
		t.Errorf("RunCfg 里 p1 的 model = %q，期望仍是启动时的 sonnet", got)
	}
	if got := env.srv.currentCfg().Agents["p1"].Model; got != "opus-5.5" {
		t.Errorf("展示配置里 p1 的 model = %q，期望 opus-5.5", got)
	}
	// 保存前的预览要能看见这笔待重启：GET /api/settings 必须把
	// pending_restart_fields 报出来，否则下一次只改 display_name 的预览
	// 会说「立即生效」而服务端回「需要重启」。
	pending := gui6AgentByID(t, gui6Settings(t, env).Agents, "p1")
	if !pending.Runnable {
		t.Errorf("p1 仍在 RunCfg 里，runnable = false，期望 true")
	}
	if !a2Contains(pending.PendingRestartFields, "model") {
		t.Errorf("p1 的 pending_restart_fields = %v，期望包含 model", pending.PendingRestartFields)
	}

	// 旧配置仍然可执行：ask 通过校验并跑完（不会因为展示配置换了 model 就崩）。
	jobID := postAsk(t, env.writeTestEnv, map[string]any{
		"prompt":  "改完 model 但没重启",
		"members": []string{"p1"},
	})
	if status := waitJobStatus(t, env.writeTestEnv, jobID); status != "ok" {
		t.Fatalf("改 model 未重启时 /api/ask 的最终状态 = %q，期望 ok（按旧配置执行）", status)
	}
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

// TestA2IndexSettingsReloadHints 钉住前端新增的结构锚点与两套文案。
// 真正的点击、预览与结果区渲染由 CDP 用真实 Chromium 验收。
func TestA2IndexSettingsReloadHints(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		`id="settings-preview-reload"`,
		`settingsPreviewReload`,
		`settingsPreviewVerdict`,
		`hot_fields`,
		`restart_fields`,
		`pending_restart_fields`,
		`current.runnable === false`,
		`'名称或角色变了'`,
		`'model 变了'`,
		`'本次改动需要重启 serve 后生效：'`,
		`'保存后立即生效`,
		`'已热加载：本次改动立即生效，无需重启 serve。'`,
		`'需要重启 serve 后生效：'`,
		`'运行配置没有变化，无需重启 serve。'`,
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
