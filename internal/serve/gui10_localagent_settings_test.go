package serve

// gui10_localagent_settings_test.go 覆盖设置页第 6 类「本机 agent」的 Go 侧：
// POST /api/settings/local-agent 只改目标文件里的 local_agent 那一行，
// 写前备份，非法值一个字节都不动；GET /api/settings 把当前层级与合法取值
// 一起交出去。
//
// 与语言那条的差别只有一处，但它必须被钉住：local_agent 的改动**不改变本
// 进程的任何行为**（serve 不做进程扫描、不申请租约），所以响应里那句
// 「谁什么时候才会看到新层级」不能省。

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"council/internal/config"
)

// gui10SaveLocal 发一次层级保存，返回状态码与响应体。
func gui10SaveLocal(t *testing.T, env *gui6Env, level string) (int, []byte) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"level": level})
	if err != nil {
		t.Fatalf("编码请求失败: %v", err)
	}
	return gui6Post(t, env.http.URL+"/api/settings/local-agent", string(body))
}

// TestGui10SaveLocalAgentWritesTopLevelKey 是主用例：写进项目层（layers[0]）、
// 只动那一行、备份等于原文、全局层一个字节不变、GET 立刻读到新值。
func TestGui10SaveLocalAgentWritesTopLevelKey(t *testing.T) {
	env := newGui6Env(t)
	before := gui6Read(t, env.projectConfig)
	globalBefore := gui6Read(t, env.globalConfig)

	if got := gui6Settings(t, env).LocalAgent; got != config.LocalAgentOff {
		t.Errorf("初始 local_agent = %q，期望 off（配置里没写这个键）", got)
	}

	status, raw := gui10SaveLocal(t, env, config.LocalAgentObserve)
	if status != http.StatusOK {
		t.Fatalf("保存 observe = %d，期望 200；body=%s", status, raw)
	}
	var res settingsWriteResponse
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("响应不是合法 JSON: %v；%s", err, raw)
	}
	if res.WriteTarget != env.projectConfig {
		t.Errorf("write_target = %q，期望只写第一层 %q", res.WriteTarget, env.projectConfig)
	}
	if res.BackupPath == "" {
		t.Error("没有备份路径：写盘必须留一份原文")
	} else if backup, err := os.ReadFile(res.BackupPath); err != nil {
		t.Errorf("读备份失败: %v", err)
	} else if string(backup) != before {
		t.Error("备份内容与写前原文不一致")
	}
	if !res.Reloaded {
		t.Error("reloaded = false，期望 true（展示配置已换成磁盘上的新内容）")
	}
	for _, want := range []string{"council who", "council claim", "council mcp"} {
		if !strings.Contains(res.Note, want) {
			t.Errorf("note 没有说清生效范围（缺 %q）: %s", want, res.Note)
		}
	}

	after := gui6Read(t, env.projectConfig)
	if !strings.Contains(after, `local_agent = "observe"`) {
		t.Fatalf("目标文件里没有新层级:\n%s", after)
	}
	// 顶层键必须写在第一个表头之前，否则它会被解析成某个 agent 的字段。
	if strings.Index(after, "local_agent =") > strings.Index(after, "[agents.p1]") {
		t.Errorf("local_agent 被写进了表里:\n%s", after)
	}
	if got := gui6Read(t, env.globalConfig); got != globalBefore {
		t.Error("写 local_agent 改动了全局层")
	}
	if got := gui6Settings(t, env).LocalAgent; got != config.LocalAgentObserve {
		t.Errorf("保存后 /api/settings.local_agent = %q，期望 observe", got)
	}

	// 再降回 off：第二次写盘应当原地替换那一行，文件里只会有一处。
	if status, raw := gui10SaveLocal(t, env, config.LocalAgentOff); status != http.StatusOK {
		t.Fatalf("保存 off = %d；body=%s", status, raw)
	}
	final := gui6Read(t, env.projectConfig)
	if got := strings.Count(final, "local_agent ="); got != 1 {
		t.Errorf("local_agent 出现 %d 次，期望 1 次:\n%s", got, final)
	}
	if !strings.Contains(final, `local_agent = "off"`) {
		t.Errorf("降回 off 之后文件里没有 off:\n%s", final)
	}
}

// TestGui10SaveLocalAgentCoordinate 覆盖第三档：coordinate 是合法值，保存后
// 配置与响应都对得上。
func TestGui10SaveLocalAgentCoordinate(t *testing.T) {
	env := newGui6Env(t)
	status, raw := gui10SaveLocal(t, env, " Coordinate ")
	if status != http.StatusOK {
		t.Fatalf("保存 coordinate = %d，期望 200；body=%s", status, raw)
	}
	if got := gui6Settings(t, env).LocalAgent; got != config.LocalAgentCoordinate {
		t.Errorf("local_agent = %q，期望 coordinate", got)
	}
	cfg, err := config.LoadMerged(env.layers)
	if err != nil {
		t.Fatalf("写后重新合并失败: %v", err)
	}
	if cfg.Config.LocalAgent != config.LocalAgentCoordinate {
		t.Errorf("重新加载的 local_agent = %q，期望 coordinate", cfg.Config.LocalAgent)
	}
}

// TestGui10SaveLocalAgentInvalidLeavesFileUntouched 钉住非法取值：400、
// 文件不改、不生成备份、不留临时文件。三档之外的名字（register 等）必须被拒。
func TestGui10SaveLocalAgentInvalidLeavesFileUntouched(t *testing.T) {
	env := newGui6Env(t)
	before := gui6Read(t, env.projectConfig)

	for _, bad := range []string{"register", "collaborate", "on", "true"} {
		status, raw := gui10SaveLocal(t, env, bad)
		if status != http.StatusBadRequest {
			t.Errorf("level=%q 的状态码 = %d，期望 400；body=%s", bad, status, raw)
			continue
		}
		if !strings.Contains(string(raw), "local_agent") {
			t.Errorf("level=%q 的 400 没有点名 local_agent: %s", bad, raw)
		}
	}

	// 空串单独一条：它不会经过下拉到达这里，当成「重置成 off」太隐蔽。
	if status, raw := gui10SaveLocal(t, env, "  "); status != http.StatusBadRequest {
		t.Errorf("空 level 的状态码 = %d，期望 400；body=%s", status, raw)
	}
	if _, raw := gui6Post(t, env.http.URL+"/api/settings/local-agent", `{"level":""}`); !strings.Contains(string(raw), "level 不能为空") {
		t.Errorf("空 level 的说明不对: %s", raw)
	}

	if got := gui6Read(t, env.projectConfig); got != before {
		t.Error("被拒的保存改动了配置文件")
	}
	backups, _ := filepath.Glob(env.projectConfig + ".bak-*")
	if len(backups) != 0 {
		t.Errorf("被拒的保存生成了备份: %v", backups)
	}
}

// TestGui10SaveLocalAgentRejectsExtraShapes 覆盖请求体门槛：多一个对象、
// 缺 level 键、超大请求体都必须是 400，且不改文件。
func TestGui10SaveLocalAgentRejectsExtraShapes(t *testing.T) {
	env := newGui6Env(t)
	before := gui6Read(t, env.projectConfig)
	url := env.http.URL + "/api/settings/local-agent"

	cases := []struct {
		name string
		body string
		want string
	}{
		{"两个对象", `{"level":"observe"}{"level":"off"}`, "只能有一个 JSON 对象"},
		{"非法 JSON", `{"level":`, "不是合法 JSON"},
		{"缺 level 键", `{}`, "level 不能为空"},
		{"超大请求体", `{"level":"` + strings.Repeat("a", maxSettingsBody) + `"}`, "上限"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, raw := gui6Post(t, url, tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("状态码 = %d，期望 400；body=%s", status, raw)
			}
			if !strings.Contains(string(raw), tc.want) {
				t.Errorf("说明里没有 %q: %s", tc.want, raw)
			}
		})
	}
	if got := gui6Read(t, env.projectConfig); got != before {
		t.Error("被拒的保存改动了配置文件")
	}
}

// TestGui10LocalAgentReadOnlyIs405 钉住只读模式：没有 -rw 就没有设置端点。
func TestGui10LocalAgentReadOnlyIs405(t *testing.T) {
	env := newServeTestEnv(t, nil)
	status, _, raw := postRaw(t, env.http.URL+"/api/settings/local-agent", "application/json", `{"level":"observe"}`)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("只读模式状态码 = %d，期望 405；body=%s", status, raw)
	}
	if !strings.Contains(string(raw), "只读") || !strings.Contains(string(raw), "-rw") {
		t.Errorf("405 的说明 = %s，期望说清只读模式并提示 -rw", raw)
	}
}

// TestGui10LocalAgentWriteGuard 复用同一套写闸：非 JSON 415、跨站 403。
func TestGui10LocalAgentWriteGuard(t *testing.T) {
	env := newGui6Env(t)
	before := gui6Read(t, env.projectConfig)
	url := env.http.URL + "/api/settings/local-agent"

	if status, _, raw := postRaw(t, url, "text/plain", `{"level":"observe"}`); status != http.StatusUnsupportedMediaType {
		t.Errorf("text/plain = %d，期望 415；body=%s", status, raw)
	}
	if got := gui6Read(t, env.projectConfig); got != before {
		t.Error("被写闸拒绝的请求改动了配置文件")
	}
}

// TestGui10SettingsExposeSupportedLevels 钉住下拉的取值来自服务端：
// local_agent_supported 必须与 config 的枚举一致（前端不另抄一份）。
func TestGui10SettingsExposeSupportedLevels(t *testing.T) {
	env := newGui6Env(t)
	resp := gui6Settings(t, env)
	want := []string{config.LocalAgentOff, config.LocalAgentObserve, config.LocalAgentCoordinate}
	if len(resp.LocalAgentSupported) != len(want) {
		t.Fatalf("local_agent_supported = %v，期望 %v", resp.LocalAgentSupported, want)
	}
	for i, level := range want {
		if resp.LocalAgentSupported[i] != level {
			t.Errorf("local_agent_supported[%d] = %q，期望 %q", i, resp.LocalAgentSupported[i], level)
		}
	}
	// 每一档都必须过 Validate：下拉里出现一个写不进去的取值，用户会撞 400。
	for _, level := range resp.LocalAgentSupported {
		cfg := config.Default()
		cfg.LocalAgent = level
		if err := cfg.Validate(); err != nil {
			t.Errorf("下拉里的 %q 通不过 Validate: %v", level, err)
		}
	}
}
