// gui9b_workspace_move_test.go 覆盖 B1 的 POST /api/workspaces/{id}/move：
//
//	POST /api/workspaces/{id}/move  {"direction":"up"|"down"}
//
// 正反例：上移 / 下移真的改变顺序、到顶 / 到底回 200 + moved:false 且不动库、
// 未知 id 404、只读 405、方法 405、direction / JSON / 写闸的 400·415·403，
// 以及「移动永远不是 409」——409 是改名撞名的专属状态，move 不碰名字。
//
// 这里不测前端按钮；按钮、快捷键与刷新持久化由 CDP 验收覆盖，结构锚点
// 在 gui9b_workspace_move_ui_test.go。
package serve

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// gui9MoveResponse 是 move 端点的响应，字段与 workspaceMoveResponse 对齐。
type gui9MoveResponse struct {
	ID    string `json:"id"`
	Moved bool   `json:"moved"`
}

// gui9MoveWorkspace 发一次移动请求，返回状态、解析后的响应与原始 body。
// 解析失败不回 null：状态码之外的断言要看 body 才能分清 404 与 500。
func gui9MoveWorkspace(t *testing.T, baseURL, id, direction string) (int, gui9MoveResponse, []byte) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"direction": direction})
	if err != nil {
		t.Fatalf("编码 move 请求失败: %v", err)
	}
	status, _, raw := postRaw(t, baseURL+"/api/workspaces/"+id+"/move",
		"application/json", string(body))
	var resp gui9MoveResponse
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &resp)
	}
	return status, resp, raw
}

// gui9WorkspaceOrder 返回左栏顺序（名字列表）。
func gui9WorkspaceOrder(t *testing.T, baseURL string) []string {
	t.Helper()
	list := gui9ListWorkspaces(t, baseURL)
	names := make([]string, 0, len(list))
	for _, ws := range list {
		names = append(names, ws.Name)
	}
	return names
}

// gui9RequireOrder 断言当前顺序，失败时把整个列表打出来（只报「不相等」
// 看不出是顺序错了还是少了一行）。
func gui9RequireOrder(t *testing.T, baseURL string, want []string) {
	t.Helper()
	got := gui9WorkspaceOrder(t, baseURL)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("工作区顺序 = %v，期望 %v", got, want)
	}
}

// TestGui9WorkspaceMoveSwapsNeighbors 是主路径：三组依次上移 / 下移，
// 每一步都断言完整顺序，而不只是被移动那一行——交换写错一边时，只查
// 目标行仍可能通过（另一行被覆盖成同一个 sort_order）。
func TestGui9WorkspaceMoveSwapsNeighbors(t *testing.T) {
	env := newWriteTestEnv(t)
	first := gui9CreateWorkspace(t, env, "甲")
	second := gui9CreateWorkspace(t, env, "乙")
	third := gui9CreateWorkspace(t, env, "丙")
	gui9RequireOrder(t, env.http.URL, []string{"甲", "乙", "丙"})

	// 丙上移一次：甲 丙 乙。
	status, resp, raw := gui9MoveWorkspace(t, env.http.URL, third.ID, "up")
	if status != http.StatusOK {
		t.Fatalf("丙上移 = %d，期望 200；body=%s", status, raw)
	}
	if resp.ID != third.ID || !resp.Moved {
		t.Errorf("丙上移的响应 = %+v，期望 id=%s moved=true", resp, third.ID)
	}
	gui9RequireOrder(t, env.http.URL, []string{"甲", "丙", "乙"})

	// 丙再上移一次：丙 甲 乙。
	if status, resp, raw := gui9MoveWorkspace(t, env.http.URL, third.ID, "up"); status != http.StatusOK || !resp.Moved {
		t.Fatalf("丙第二次上移 = %d / %+v，期望 200 / moved=true；body=%s", status, resp, raw)
	}
	gui9RequireOrder(t, env.http.URL, []string{"丙", "甲", "乙"})

	// 丙下移：甲 丙 乙；再下移：甲 乙 丙。
	if status, resp, raw := gui9MoveWorkspace(t, env.http.URL, third.ID, "down"); status != http.StatusOK || !resp.Moved {
		t.Fatalf("丙下移 = %d / %+v，期望 200 / moved=true；body=%s", status, resp, raw)
	}
	gui9RequireOrder(t, env.http.URL, []string{"甲", "丙", "乙"})
	if status, resp, raw := gui9MoveWorkspace(t, env.http.URL, third.ID, "down"); status != http.StatusOK || !resp.Moved {
		t.Fatalf("丙第二次下移 = %d / %+v，期望 200 / moved=true；body=%s", status, resp, raw)
	}
	gui9RequireOrder(t, env.http.URL, []string{"甲", "乙", "丙"})

	// 最前面的下移：乙 甲 丙。
	if status, resp, raw := gui9MoveWorkspace(t, env.http.URL, first.ID, "down"); status != http.StatusOK || !resp.Moved {
		t.Fatalf("甲下移 = %d / %+v，期望 200 / moved=true；body=%s", status, resp, raw)
	}
	gui9RequireOrder(t, env.http.URL, []string{"乙", "甲", "丙"})

	// 移动只改 sort_order：id、名字、会话数一条都不许变。
	list := gui9ListWorkspaces(t, env.http.URL)
	byID := map[string]gui9Workspace{}
	for _, ws := range list {
		byID[ws.ID] = ws
	}
	if got := byID[first.ID]; got.Name != "甲" || got.ConversationCount != 0 {
		t.Errorf("移动后甲 = %+v，期望名字与会话数不变", got)
	}
	if got := byID[second.ID]; got.Name != "乙" || got.ConversationCount != 0 {
		t.Errorf("移动后乙 = %+v，期望名字与会话数不变", got)
	}
	if got := byID[third.ID]; got.Name != "丙" || got.ConversationCount != 0 {
		t.Errorf("移动后丙 = %+v，期望名字与会话数不变", got)
	}
}

// TestGui9WorkspaceMoveAtEdgesIsNoOp 钉「到顶 / 到底回 200 + moved:false」：
// 不报错（4xx 会让前端弹红字），也不写库（sort_order 原样返回）。
func TestGui9WorkspaceMoveAtEdgesIsNoOp(t *testing.T) {
	env := newWriteTestEnv(t)
	first := gui9CreateWorkspace(t, env, "第一组")
	second := gui9CreateWorkspace(t, env, "第二组")
	before := gui9ListWorkspaces(t, env.http.URL)

	status, resp, raw := gui9MoveWorkspace(t, env.http.URL, first.ID, "up")
	if status != http.StatusOK {
		t.Fatalf("第一组上移 = %d，期望 200（到顶不是错误）；body=%s", status, raw)
	}
	if resp.Moved {
		t.Errorf("第一组上移的 moved = true，期望 false")
	}
	if resp.ID != first.ID {
		t.Errorf("到顶响应的 id = %q，期望 %q", resp.ID, first.ID)
	}

	status, resp, raw = gui9MoveWorkspace(t, env.http.URL, second.ID, "down")
	if status != http.StatusOK {
		t.Fatalf("最后一组下移 = %d，期望 200（到底不是错误）；body=%s", status, raw)
	}
	if resp.Moved {
		t.Errorf("最后一组下移的 moved = true，期望 false")
	}

	after := gui9ListWorkspaces(t, env.http.URL)
	for i := range before {
		if before[i].ID != after[i].ID || before[i].SortOrder != after[i].SortOrder {
			t.Errorf("到顶/到底的 no-op 改动了库：before=%+v after=%+v", before, after)
			break
		}
	}
	gui9RequireOrder(t, env.http.URL, []string{"第一组", "第二组"})
}

// TestGui9WorkspaceMoveUnknownID 钉未知 id 的 404（与 rename / delete 同口径）。
func TestGui9WorkspaceMoveUnknownID(t *testing.T) {
	env := newWriteTestEnv(t)
	gui9CreateWorkspace(t, env, "唯一组")

	status, resp, raw := gui9MoveWorkspace(t, env.http.URL, "no-such-workspace", "up")
	if status != http.StatusNotFound {
		t.Fatalf("未知 id 上移 = %d，期望 404；body=%s", status, raw)
	}
	if resp.ID != "" {
		t.Errorf("404 响应里不该有 id，实际 %+v", resp)
	}
	if !strings.Contains(string(raw), "no-such-workspace") {
		t.Errorf("404 文案没有点名 id：%s", raw)
	}
	var errResp errorResponse
	if err := json.Unmarshal(raw, &errResp); err != nil || errResp.Error == "" {
		t.Errorf("404 响应不是 {\"error\":...}：%s（err=%v）", raw, err)
	}
}

// TestGui9WorkspaceMoveReadOnly405 钉只读模式的 405：move 是写端点，
// 即使请求本身完全合法也不能走到 store。POST 与 GET 都要 405 —— 只读
// 判断先于方法判断，这是本仓库所有写端点的统一顺序。
func TestGui9WorkspaceMoveReadOnly405(t *testing.T) {
	env := newServeTestEnv(t, nil)
	ctx := env.ctx(t)
	ws, err := env.store.CreateWorkspace(ctx, "只读组")
	if err != nil {
		t.Fatalf("在只读测试库里建工作区失败: %v", err)
	}

	status, ctype, raw := postRaw(t,
		env.http.URL+"/api/workspaces/"+ws.ID+"/move", "application/json",
		`{"direction":"down"}`)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("只读实例的 move = %d，期望 405；body=%s", status, raw)
	}
	if ctype != "application/json; charset=utf-8" {
		t.Errorf("405 的 Content-Type = %q", ctype)
	}
	if !strings.Contains(string(raw), "只读模式") {
		t.Errorf("405 没有说明只读模式：%s", raw)
	}

	// 非 POST 在只读模式下同样是 405，而且文案仍是只读那条：
	// 「没有这个接口」的 404 会让人以为路由写错了。
	req, err := http.NewRequest(http.MethodGet,
		env.http.URL+"/api/workspaces/"+ws.ID+"/move", nil)
	if err != nil {
		t.Fatalf("构造 GET 请求失败: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET move 失败: %v", err)
	}
	defer res.Body.Close()
	raw, err = io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("读 GET move 响应失败: %v", err)
	}
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("只读实例 GET move = %d，期望 405", res.StatusCode)
	}
	if got := res.Header.Get("Allow"); got != http.MethodPost {
		t.Errorf("只读 405 的 Allow = %q，期望 POST", got)
	}
	if !strings.Contains(string(raw), "只读模式") {
		t.Errorf("只读 GET 的文案没有说明只读模式：%s", raw)
	}

	// 库没有被写：顺序还是单组。
	gui9RequireOrder(t, env.http.URL, []string{"只读组"})
}

// TestGui9WorkspaceMoveRejectsBadRequests 覆盖请求体与方法两道门，
// 并显式确认「move 永远不是 409」：409 只属于改名撞名，移动不碰名字。
func TestGui9WorkspaceMoveRejectsBadRequests(t *testing.T) {
	env := newWriteTestEnv(t)
	first := gui9CreateWorkspace(t, env, "甲组")
	second := gui9CreateWorkspace(t, env, "乙组")
	moveURL := env.http.URL + "/api/workspaces/" + second.ID + "/move"

	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"缺 direction", `{}`, http.StatusBadRequest},
		{"direction 为空", `{"direction":""}`, http.StatusBadRequest},
		{"direction 大小写不符", `{"direction":"UP"}`, http.StatusBadRequest},
		{"direction 未知", `{"direction":"left"}`, http.StatusBadRequest},
		{"direction 不是字符串", `{"direction":-1}`, http.StatusBadRequest},
		{"请求体不是 JSON", `up`, http.StatusBadRequest},
		{"两个 JSON 对象", `{"direction":"up"}{"direction":"down"}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, raw := postRaw(t, moveURL, "application/json", tc.body)
			if status != tc.want {
				t.Errorf("move(%s) = %d，期望 %d；body=%s", tc.body, status, tc.want, raw)
			}
			if status == http.StatusConflict {
				t.Errorf("move 回了 409：移动不碰名字，409 只该来自改名撞名")
			}
			var errResp errorResponse
			if err := json.Unmarshal(raw, &errResp); err != nil || errResp.Error == "" {
				t.Errorf("错误响应不是 {\"error\":...}：%s", raw)
			}
		})
	}

	// Content-Type / 同源两道写闸。
	if status, _, raw := postRaw(t, moveURL, "text/plain", `{"direction":"up"}`); status != http.StatusUnsupportedMediaType {
		t.Errorf("text/plain 的 move = %d，期望 415；body=%s", status, raw)
	}
	req, err := http.NewRequest(http.MethodPost, moveURL, strings.NewReader(`{"direction":"up"}`))
	if err != nil {
		t.Fatalf("构造跨站请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("跨站 move 失败: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("跨站 move = %d，期望 403", res.StatusCode)
	}

	// 非 POST 405（写模式下）。
	req, err = http.NewRequest(http.MethodGet, moveURL, nil)
	if err != nil {
		t.Fatalf("构造 GET 请求失败: %v", err)
	}
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET move 失败: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("写模式 GET move = %d，期望 405", res.StatusCode)
	}
	if got := res.Header.Get("Allow"); got != http.MethodPost {
		t.Errorf("写模式 405 的 Allow = %q，期望 POST", got)
	}

	// 409 无关：先把乙组改名撞上甲组（真正的 409），再移动它。
	// 移动必须成功，且顺序真的变了——这条把「move 也可能撞名」的猜测钉死。
	if status, raw := gui9PostWorkspaceName(t, env.http.URL+"/api/workspaces/"+second.ID+"/rename", "甲组"); status != http.StatusConflict {
		t.Fatalf("把乙组改名成甲组 = %d，期望 409；body=%s", status, raw)
	}
	if status, resp, raw := gui9MoveWorkspace(t, env.http.URL, second.ID, "up"); status != http.StatusOK || !resp.Moved {
		t.Fatalf("撞名之后移动乙组 = %d / %+v，期望 200 / moved=true；body=%s", status, resp, raw)
	}
	gui9RequireOrder(t, env.http.URL, []string{"乙组", "甲组"})

	// 移动没有改名字：撞名的那个名字仍是乙组。
	list := gui9ListWorkspaces(t, env.http.URL)
	for _, ws := range list {
		if ws.ID == second.ID && ws.Name != "乙组" {
			t.Errorf("移动之后乙组的名字变成了 %q，move 不该改名字", ws.Name)
		}
		if ws.ID == first.ID && ws.Name != "甲组" {
			t.Errorf("移动之后甲组的名字变成了 %q", ws.Name)
		}
	}
}
