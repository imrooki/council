package serve

// gui6c_settingsedit_test.go：原来「只读展示」的三处（汇总成员、团队、数据目录）
// 现在都能改。用例的重点不是「返回 200」，而是「写进了哪一份、别的段有没有被
// 顺手改动、不该通过的值有没有落盘」。

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func postSettings(t *testing.T, env *gui6Env, path string, payload any) (int, []byte) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("编码请求失败: %v", err)
	}
	status, raw := gui6Post(t, env.http.URL+path, string(body))
	return status, raw
}

func TestGui6cSummaryAgentIsEditable(t *testing.T) {
	env := newGui6Env(t)
	// 测试配置里 summary.agent = g1，g1 是只读成员。
	status, raw := postSettings(t, env, "/api/settings/summary", map[string]any{"agent": "p1"})
	if status != http.StatusOK {
		t.Fatalf("设置汇总成员 = %d；body=%s", status, raw)
	}
	project := gui6Read(t, env.projectConfig)
	if !strings.Contains(project, "agent = 'p1'") {
		t.Errorf("项目层里没有新的 summary.agent:\n%s", project)
	}
	if strings.Contains(project, "data_dir") {
		t.Errorf("写 summary 时把全局层的 data_dir 搬到了项目层:\n%s", project)
	}

	// 关掉汇总：整张表消失。
	if status, raw := postSettings(t, env, "/api/settings/summary", map[string]any{"agent": ""}); status != http.StatusOK {
		t.Fatalf("关闭汇总 = %d；body=%s", status, raw)
	}
	if strings.Contains(gui6Read(t, env.projectConfig), "[summary]") {
		t.Errorf("关闭汇总后表还在")
	}

	// 不存在的成员要被拒，且不落盘。
	before := gui6Read(t, env.projectConfig)
	status, raw = postSettings(t, env, "/api/settings/summary", map[string]any{"agent": "nobody"})
	if status != http.StatusBadRequest {
		t.Fatalf("不存在的成员 = %d，期望 400；body=%s", status, raw)
	}
	if gui6Read(t, env.projectConfig) != before {
		t.Errorf("被拒绝的保存改了文件")
	}
}

func TestGui6cSummaryRejectsWritableAgent(t *testing.T) {
	env := newGui6Env(t)
	// p2 是 workspace 权限（测试配置里写的是 worker 模式 + workspace? 用它自己）。
	gui6Save(t, env, map[string]any{
		"agent_id":     "writer",
		"display_name": "Writer",
		"provider":     "claude",
		"model":        "opus",
		"permissions":  "workspace",
		"mode":         "worker",
	})
	status, raw := postSettings(t, env, "/api/settings/summary", map[string]any{"agent": "writer"})
	if status != http.StatusBadRequest {
		t.Fatalf("用可写成员当汇总 = %d，期望 400；body=%s", status, raw)
	}
	if !strings.Contains(string(raw), "read") {
		t.Errorf("错误消息没有说明必须是只读成员: %s", raw)
	}
}

func TestGui6cTeamCRUD(t *testing.T) {
	env := newGui6Env(t)
	// 新增
	status, raw := postSettings(t, env, "/api/settings/teams", map[string]any{
		"team_id":      "nightshift",
		"display_name": "Night Shift",
		"members":      []string{"p2", "p1"},
	})
	if status != http.StatusOK {
		t.Fatalf("新增团队 = %d；body=%s", status, raw)
	}
	project := gui6Read(t, env.projectConfig)
	for _, want := range []string{"[teams.nightshift]", "display_name = 'Night Shift'", "members = ['p1', 'p2']"} {
		if !strings.Contains(project, want) {
			t.Errorf("项目层里缺少 %q:\n%s", want, project)
		}
	}
	// 顺序会被排序（p1 在 p2 前），这是刻意的：同一份名单不该因为勾选顺序不同而字节不同。

	// 改成员
	if status, raw := postSettings(t, env, "/api/settings/teams", map[string]any{
		"team_id":      "nightshift",
		"display_name": "Night Shift",
		"members":      []string{"p2"},
	}); status != http.StatusOK {
		t.Fatalf("改团队 = %d；body=%s", status, raw)
	}
	if !strings.Contains(gui6Read(t, env.projectConfig), "members = ['p2']") {
		t.Errorf("成员没更新")
	}

	// 删除
	status, raw = gui6Post(t, env.http.URL+"/api/settings/teams/nightshift/delete", "{}")
	if status != http.StatusOK {
		t.Fatalf("删团队 = %d；body=%s", status, raw)
	}
	project = gui6Read(t, env.projectConfig)
	if strings.Contains(project, "[teams.nightshift]") {
		t.Errorf("删除后段还在:\n%s", project)
	}
	// 删团队不能删成员。
	if !strings.Contains(project, "[agents.p1]") || !strings.Contains(project, "[agents.p2]") {
		t.Errorf("删团队把成员删了:\n%s", project)
	}
}

func TestGui6cTeamValidation(t *testing.T) {
	env := newGui6Env(t)
	before := gui6Read(t, env.projectConfig)
	cases := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{"空 id", map[string]any{"team_id": " ", "members": []string{"p1"}}, "不能为空"},
		{"空成员", map[string]any{"team_id": "x", "members": []string{}}, "至少要勾一个成员"},
		{"未知成员", map[string]any{"team_id": "x", "members": []string{"nobody"}}, "没有这个成员"},
		{"重复成员", map[string]any{"team_id": "x", "members": []string{"p1", "p1"}}, "出现了两次"},
		{"非法 id", map[string]any{"team_id": "Bad ID", "members": []string{"p1"}}, "team id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, raw := postSettings(t, env, "/api/settings/teams", tc.payload)
			if status != http.StatusBadRequest {
				t.Fatalf("= %d，期望 400；body=%s", status, raw)
			}
			if !strings.Contains(string(raw), tc.want) {
				t.Errorf("错误消息 = %s，期望包含 %q", raw, tc.want)
			}
			if gui6Read(t, env.projectConfig) != before {
				t.Errorf("校验失败却改了文件")
			}
		})
	}
}

func TestGui6cDataDirIsEditable(t *testing.T) {
	env := newGui6Env(t)
	target := filepath.Join(t.TempDir(), "data2")
	status, raw := postSettings(t, env, "/api/settings/data-dir", map[string]any{"data_dir": target})
	if status != http.StatusOK {
		t.Fatalf("改数据目录 = %d；body=%s", status, raw)
	}
	project := gui6Read(t, env.projectConfig)
	if !strings.Contains(project, "data_dir = ") {
		t.Errorf("项目层里没有 data_dir:\n%s", project)
	}
	if !strings.Contains(strings.ToLower(project), strings.ToLower(strings.ReplaceAll(target, `\`, `\\`))) &&
		!strings.Contains(project, target) {
		t.Errorf("data_dir 的值不是新路径:\n%s", project)
	}
	// 全局层不能被动。
	if strings.Contains(gui6Read(t, env.globalConfig), target) {
		t.Errorf("写到了全局层")
	}
}

func TestGui6cDataDirValidation(t *testing.T) {
	env := newGui6Env(t)
	before := gui6Read(t, env.projectConfig)
	filePath := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("建测试文件: %v", err)
	}
	for _, tc := range []struct{ value, want string }{
		{"", "不能为空"},
		{"relative/path", "绝对路径"},
		{filePath, "不是目录"},
	} {
		status, raw := postSettings(t, env, "/api/settings/data-dir", map[string]any{"data_dir": tc.value})
		if status != http.StatusBadRequest {
			t.Errorf("%q = %d，期望 400；body=%s", tc.value, status, raw)
			continue
		}
		if !strings.Contains(string(raw), tc.want) {
			t.Errorf("%q 的错误消息 = %s，期望包含 %q", tc.value, raw, tc.want)
		}
	}
	if gui6Read(t, env.projectConfig) != before {
		t.Errorf("校验失败却改了文件")
	}
}
