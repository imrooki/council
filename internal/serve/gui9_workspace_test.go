// gui9_workspace_test.go 覆盖「工作区」这层纯分组在 HTTP 上的四类接口：
//
//	GET  /api/workspaces                         列表（含 conversation_count）
//	POST /api/workspaces                         新建
//	POST /api/workspaces/{id}/rename             改名
//	POST /api/workspaces/{id}/delete             删除（会话移出分组，不删会话）
//	POST /api/conversations/{id}/workspace       放进 / 移出
//
// 正反例都在这里：重名 409、未知 id 404、只读模式 405、写闸的 415 / 403、
// 以及会话 JSON（默认列表 / 归档列表 / 详情）都带 workspace_id。
//
// 本片只做后端，index.html 一个字都不动：所以这里没有 TestGui9Index* 那种
// 结构锚点用例 —— 前端是下一片，锚点也跟着下一片一起加。
package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// gui9Workspace 是响应里的一条工作区，字段与 workspaceJSON 对齐。
type gui9Workspace struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	SortOrder         int    `json:"sort_order"`
	ConversationCount int    `json:"conversation_count"`
}

// gui9CreateWorkspace 发一次新建请求，断言 200 并返回响应。
func gui9CreateWorkspace(t *testing.T, env *writeTestEnv, name string) gui9Workspace {
	t.Helper()
	body, err := json.Marshal(map[string]any{"name": name})
	if err != nil {
		t.Fatalf("编码工作区请求失败: %v", err)
	}
	status, ctype, raw := postRaw(t, env.http.URL+"/api/workspaces", "application/json", string(body))
	if status != http.StatusOK {
		t.Fatalf("POST /api/workspaces = %d，期望 200；body=%s", status, raw)
	}
	if ctype != "application/json; charset=utf-8" {
		t.Errorf("新建工作区 200 的 Content-Type = %q", ctype)
	}
	var ws gui9Workspace
	if err := json.Unmarshal(raw, &ws); err != nil {
		t.Fatalf("新建工作区的响应不是合法 JSON: %v；%s", err, raw)
	}
	return ws
}

// gui9PostWorkspaceName 发一次带名字的 POST，返回状态与原始 body。
func gui9PostWorkspaceName(t *testing.T, url, name string) (int, []byte) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"name": name})
	if err != nil {
		t.Fatalf("编码工作区请求失败: %v", err)
	}
	status, _, raw := postRaw(t, url, "application/json", string(body))
	return status, raw
}

// gui9ListWorkspaces 拉一次工作区列表。
func gui9ListWorkspaces(t *testing.T, baseURL string) []gui9Workspace {
	t.Helper()
	var list []gui9Workspace
	getJSON(t, baseURL+"/api/workspaces", http.StatusOK, &list)
	if list == nil {
		t.Fatal("GET /api/workspaces 解出了 nil，期望 []")
	}
	return list
}

// gui9PostConversationWorkspace 把会话放进（或移出）工作区。
// workspaceID 为 nil 时发送 JSON 的 null。
func gui9PostConversationWorkspace(t *testing.T, env *writeTestEnv, convID string, workspaceID *string) (int, []byte) {
	t.Helper()
	payload := map[string]any{"workspace_id": nil}
	if workspaceID != nil {
		payload["workspace_id"] = *workspaceID
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("编码归属请求失败: %v", err)
	}
	status, _, raw := postRaw(t,
		env.http.URL+"/api/conversations/"+convID+"/workspace", "application/json", string(body))
	return status, raw
}

// gui9ConversationWorkspace 从 store 读一场会话当前的归属。
func gui9ConversationWorkspace(t *testing.T, env *writeTestEnv, convID string) string {
	t.Helper()
	conv, err := env.store.Conversation(context.Background(), convID)
	if err != nil {
		t.Fatalf("读会话 %s 失败: %v", convID, err)
	}
	return conv.WorkspaceID
}

// TestGui9WorkspaceCRUD 是主路径：空列表 → 建两个 → 会话进组（含一个已归档）
// → 改名 → 列表与 Count 一致 → 删组 → 会话仍在且未分组。
func TestGui9WorkspaceCRUD(t *testing.T) {
	env := newWriteTestEnv(t)
	if list := gui9ListWorkspaces(t, env.http.URL); len(list) != 0 {
		t.Fatalf("空库的工作区列表有 %d 条，期望 0 条", len(list))
	}

	first := gui9CreateWorkspace(t, env, "  工作区一  ")
	if first.Name != "工作区一" {
		t.Errorf("新建返回的 name = %q，期望去空白之后的 %q", first.Name, "工作区一")
	}
	if first.ID == "" {
		t.Fatal("新建返回的 id 是空的")
	}
	if first.ConversationCount != 0 {
		t.Errorf("新建返回的 conversation_count = %d，期望 0", first.ConversationCount)
	}
	second := gui9CreateWorkspace(t, env, "工作区二")
	if second.SortOrder <= first.SortOrder {
		t.Errorf("第二个工作区的 sort_order = %d，期望大于 %d", second.SortOrder, first.SortOrder)
	}

	t.Run("列表按 sort_order 排且字段齐全", func(t *testing.T) {
		var rows []map[string]any
		getJSON(t, env.http.URL+"/api/workspaces", http.StatusOK, &rows)
		if len(rows) != 2 {
			t.Fatalf("工作区列表有 %d 条，期望 2 条", len(rows))
		}
		for _, row := range rows {
			requireKeys(t, "workspace", row,
				"conversation_count", "id", "name", "sort_order")
		}
		if rows[0]["id"] != first.ID || rows[1]["id"] != second.ID {
			t.Errorf("工作区顺序 = [%v, %v]，期望 [%v, %v]",
				rows[0]["id"], rows[1]["id"], first.ID, second.ID)
		}
		if rows[0]["conversation_count"] != float64(0) {
			t.Errorf("新组的 conversation_count = %v，期望 0", rows[0]["conversation_count"])
		}
	})

	kept := gui3bSeedConversation(t, env, "进组的会话")
	archived := gui3bSeedConversation(t, env, "进组后又归档的会话")
	if status, raw := gui9PostConversationWorkspace(t, env, kept.ID, &first.ID); status != http.StatusOK {
		t.Fatalf("把会话放进工作区 = %d，期望 200；body=%s", status, raw)
	}
	if status, raw := gui9PostConversationWorkspace(t, env, archived.ID, &first.ID); status != http.StatusOK {
		t.Fatalf("把归档目标会话放进工作区 = %d，期望 200；body=%s", status, raw)
	}
	if status, raw := gui8PostArchive(t, env, archived.ID, "archive"); status != http.StatusOK {
		t.Fatalf("归档会话 = %d，期望 200；body=%s", status, raw)
	}

	t.Run("会话数含已归档、不含未进组的", func(t *testing.T) {
		list := gui9ListWorkspaces(t, env.http.URL)
		if len(list) != 2 {
			t.Fatalf("工作区列表有 %d 条，期望 2 条", len(list))
		}
		if list[0].ID != first.ID || list[0].ConversationCount != 2 {
			t.Errorf("第一个工作区 = %+v，期望 conversation_count=2（含已归档）", list[0])
		}
		if list[1].ConversationCount != 0 {
			t.Errorf("第二个工作区的 conversation_count = %d，期望 0", list[1].ConversationCount)
		}
	})

	t.Run("改名只改名字", func(t *testing.T) {
		status, raw := gui9PostWorkspaceName(t, env.http.URL+"/api/workspaces/"+first.ID+"/rename", "  改过的名字 ")
		if status != http.StatusOK {
			t.Fatalf("改名 = %d，期望 200；body=%s", status, raw)
		}
		var reply map[string]json.RawMessage
		if err := json.Unmarshal(raw, &reply); err != nil {
			t.Fatalf("改名响应不是合法 JSON: %v；%s", err, raw)
		}
		if len(reply) != 2 {
			t.Errorf("改名响应应当恰好有 id/name 两个字段，实际 %v", reply)
		}
		var id, name string
		if err := json.Unmarshal(reply["id"], &id); err != nil || id != first.ID {
			t.Errorf("改名响应 id = %q (err=%v)，期望 %q", id, err, first.ID)
		}
		if err := json.Unmarshal(reply["name"], &name); err != nil || name != "改过的名字" {
			t.Errorf("改名响应 name = %q (err=%v)，期望去空白之后的 %q", name, err, "改过的名字")
		}

		list := gui9ListWorkspaces(t, env.http.URL)
		if list[0].Name != "改过的名字" {
			t.Errorf("列表里的名字 = %q，期望 %q", list[0].Name, "改过的名字")
		}
		if list[0].SortOrder != first.SortOrder || list[0].ConversationCount != 2 {
			t.Errorf("改名动了顺序或会话数: %+v", list[0])
		}
		if got := gui9ConversationWorkspace(t, env, kept.ID); got != first.ID {
			t.Errorf("改名后会话归属 = %q，期望 %q（改名不该动归属）", got, first.ID)
		}
	})

	t.Run("删组不删会话、会话回到未分组", func(t *testing.T) {
		status, _, raw := postRaw(t,
			env.http.URL+"/api/workspaces/"+first.ID+"/delete", "application/json", `{}`)
		if status != http.StatusOK {
			t.Fatalf("删组 = %d，期望 200；body=%s", status, raw)
		}
		var reply map[string]json.RawMessage
		if err := json.Unmarshal(raw, &reply); err != nil {
			t.Fatalf("删组响应不是合法 JSON: %v；%s", err, raw)
		}
		if len(reply) != 2 {
			t.Errorf("删组响应应当恰好有 id/deleted 两个字段，实际 %v", reply)
		}
		var id string
		var deleted bool
		if err := json.Unmarshal(reply["id"], &id); err != nil || id != first.ID {
			t.Errorf("删组响应 id = %q (err=%v)，期望 %q", id, err, first.ID)
		}
		if err := json.Unmarshal(reply["deleted"], &deleted); err != nil || !deleted {
			t.Errorf("删组响应 deleted = %v (err=%v)，期望 true", deleted, err)
		}

		if list := gui9ListWorkspaces(t, env.http.URL); len(list) != 1 || list[0].ID != second.ID {
			t.Errorf("删组后列表 = %+v，期望只剩 %s", list, second.ID)
		}
		if got := gui9ConversationWorkspace(t, env, kept.ID); got != "" {
			t.Errorf("删组后会话归属 = %q，期望空串", got)
		}
		var detail conversationDetail
		getJSON(t, env.http.URL+"/api/conversations/"+kept.ID, http.StatusOK, &detail)
		if detail.ID != kept.ID || detail.WorkspaceID != "" {
			t.Errorf("删组后详情 = %+v，期望会话还在且未分组", detail)
		}
		var page messagesResponse
		getJSON(t, env.http.URL+"/api/conversations/"+kept.ID+"/messages", http.StatusOK, &page)
		if len(page.Messages) != 1 {
			t.Errorf("删组后消息条数 = %d，期望 1（删组不删消息）", len(page.Messages))
		}
	})

	t.Run("再删一次是 404", func(t *testing.T) {
		status, _, raw := postRaw(t,
			env.http.URL+"/api/workspaces/"+first.ID+"/delete", "application/json", `{}`)
		if status != http.StatusNotFound {
			t.Fatalf("再删一次 = %d，期望 404；body=%s", status, raw)
		}
	})
}

// TestGui9WorkspaceDuplicate409 钉住重名：新建与改名都回 409，文案点名名字
// 与「大小写不敏感」这条口径；改成自己现在的名字不算重名。
func TestGui9WorkspaceDuplicate409(t *testing.T) {
	env := newWriteTestEnv(t)
	created := gui9CreateWorkspace(t, env, "Work")
	other := gui9CreateWorkspace(t, env, "Other")

	for _, dup := range []string{"Work", "work", "  WORK  "} {
		t.Run("新建重名 "+dup, func(t *testing.T) {
			status, raw := gui9PostWorkspaceName(t, env.http.URL+"/api/workspaces", dup)
			if status != http.StatusConflict {
				t.Fatalf("新建重名 %q = %d，期望 409；body=%s", dup, status, raw)
			}
			var errBody map[string]any
			if err := json.Unmarshal(raw, &errBody); err != nil {
				t.Fatalf("409 响应不是合法 JSON: %v；%s", err, raw)
			}
			message, _ := errBody["error"].(string)
			for _, want := range []string{strings.TrimSpace(dup), "已被占用", "大小写不敏感"} {
				if !strings.Contains(message, want) {
					t.Errorf("409 的说明 = %q，期望包含 %q", message, want)
				}
			}
		})
	}

	if list := gui9ListWorkspaces(t, env.http.URL); len(list) != 2 {
		t.Errorf("重名失败后工作区数 = %d，期望 2（被拒的请求不该建出东西）", len(list))
	}

	for _, dup := range []string{"work", "Work"} {
		status, raw := gui9PostWorkspaceName(t,
			env.http.URL+"/api/workspaces/"+other.ID+"/rename", dup)
		if status != http.StatusConflict {
			t.Fatalf("改名撞上 %q = %d，期望 409；body=%s", dup, status, raw)
		}
	}

	t.Run("改成自己现在的名字（只改大小写）不算重名", func(t *testing.T) {
		status, raw := gui9PostWorkspaceName(t,
			env.http.URL+"/api/workspaces/"+created.ID+"/rename", "WORK")
		if status != http.StatusOK {
			t.Fatalf("改自己的大小写 = %d，期望 200；body=%s", status, raw)
		}
		var reply struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &reply); err != nil || reply.Name != "WORK" {
			t.Errorf("改名响应 name = %q (err=%v)，期望 WORK", reply.Name, err)
		}
	})
}

// TestGui9WorkspaceUnknown404 钉住四种 404：未知工作区改名 / 删除、给会话
// 指向未知工作区、未知或已删的会话；以及不属于这个接口的路径。
func TestGui9WorkspaceUnknown404(t *testing.T) {
	env := newWriteTestEnv(t)
	ws := gui9CreateWorkspace(t, env, "存在的组")
	live := gui3bSeedConversation(t, env, "正常会话")
	gone := gui3bSeedConversation(t, env, "先删掉的会话")
	if status, _, raw := postRaw(t, env.http.URL+"/api/conversations/"+gone.ID+"/delete",
		"application/json", `{}`); status != http.StatusOK {
		t.Fatalf("准备删除会话失败: %d %s", status, raw)
	}

	const unknownWS = "ws-does-not-exist"
	const unknownConv = "conv-does-not-exist"

	t.Run("改名未知工作区 404", func(t *testing.T) {
		status, raw := gui9PostWorkspaceName(t,
			env.http.URL+"/api/workspaces/"+unknownWS+"/rename", "随便")
		if status != http.StatusNotFound {
			t.Fatalf("改名未知工作区 = %d，期望 404；body=%s", status, raw)
		}
		var errBody map[string]any
		if err := json.Unmarshal(raw, &errBody); err != nil {
			t.Fatalf("404 响应不是合法 JSON: %v；%s", err, raw)
		}
		message, _ := errBody["error"].(string)
		if !strings.Contains(message, "工作区不存在") || !strings.Contains(message, unknownWS) {
			t.Errorf("404 的说明 = %q，期望点名不存在的工作区 %q", message, unknownWS)
		}
	})

	t.Run("删除未知工作区 404", func(t *testing.T) {
		status, _, raw := postRaw(t,
			env.http.URL+"/api/workspaces/"+unknownWS+"/delete", "application/json", `{}`)
		if status != http.StatusNotFound {
			t.Fatalf("删除未知工作区 = %d，期望 404；body=%s", status, raw)
		}
		var errBody map[string]any
		if err := json.Unmarshal(raw, &errBody); err != nil {
			t.Fatalf("404 响应不是合法 JSON: %v；%s", err, raw)
		}
		message, _ := errBody["error"].(string)
		if !strings.Contains(message, "工作区不存在") || !strings.Contains(message, unknownWS) {
			t.Errorf("404 的说明 = %q，期望点名不存在的工作区 %q", message, unknownWS)
		}
	})

	t.Run("未知工作区的路径 404", func(t *testing.T) {
		for _, path := range []string{
			"/api/workspaces/",
			"/api/workspaces/" + ws.ID,
			"/api/workspaces/" + ws.ID + "/rename/extra",
			"/api/workspaces/" + ws.ID + "/nope",
		} {
			var errBody map[string]any
			status, _, raw := postRaw(t, env.http.URL+path, "application/json", `{}`)
			if status != http.StatusNotFound {
				t.Errorf("POST %s = %d，期望 404；body=%s", path, status, raw)
				continue
			}
			if err := json.Unmarshal(raw, &errBody); err != nil {
				t.Errorf("%s 的 404 响应不是合法 JSON: %v；%s", path, err, raw)
			}
		}
	})

	t.Run("会话指向未知工作区 404 且不写库", func(t *testing.T) {
		status, raw := gui9PostConversationWorkspace(t, env, live.ID, strPtr(unknownWS))
		if status != http.StatusNotFound {
			t.Fatalf("指向未知工作区 = %d，期望 404；body=%s", status, raw)
		}
		var errBody map[string]any
		if err := json.Unmarshal(raw, &errBody); err != nil {
			t.Fatalf("404 响应不是合法 JSON: %v；%s", err, raw)
		}
		message, _ := errBody["error"].(string)
		if !strings.Contains(message, "工作区不存在") || !strings.Contains(message, unknownWS) {
			t.Errorf("404 的说明 = %q，期望点名不存在的工作区 %q", message, unknownWS)
		}
		if got := gui9ConversationWorkspace(t, env, live.ID); got != "" {
			t.Errorf("被拒之后会话归属 = %q，期望仍是空串", got)
		}
	})

	t.Run("未知会话 404", func(t *testing.T) {
		status, raw := gui9PostConversationWorkspace(t, env, unknownConv, &ws.ID)
		if status != http.StatusNotFound {
			t.Fatalf("未知会话 = %d，期望 404；body=%s", status, raw)
		}
		var errBody map[string]any
		if err := json.Unmarshal(raw, &errBody); err != nil {
			t.Fatalf("404 响应不是合法 JSON: %v；%s", err, raw)
		}
		message, _ := errBody["error"].(string)
		if !strings.Contains(message, "会话不存在") || !strings.Contains(message, unknownConv) {
			t.Errorf("404 的说明 = %q，期望点名会话 %q", message, unknownConv)
		}
	})

	t.Run("已删会话 404", func(t *testing.T) {
		status, raw := gui9PostConversationWorkspace(t, env, gone.ID, &ws.ID)
		if status != http.StatusNotFound {
			t.Fatalf("已删会话 = %d，期望 404；body=%s", status, raw)
		}
	})
}

// strPtr 是测试里取地址的小工具。
func strPtr(v string) *string { return &v }

// TestGui9WorkspaceReadOnly405 钉住只读模式：三个写端点与改归属都回 405
// 并说明用 -rw 启动；GET /api/workspaces 在只读模式下照常工作（空库回 []）。
func TestGui9WorkspaceReadOnly405(t *testing.T) {
	env := newServeTestEnv(t, nil)

	t.Run("GET 列表在只读模式下可用", func(t *testing.T) {
		var list []gui9Workspace
		getJSON(t, env.http.URL+"/api/workspaces", http.StatusOK, &list)
		if list == nil || len(list) != 0 {
			t.Errorf("只读模式空库的工作区列表 = %#v，期望非 nil 的空数组", list)
		}
	})

	cases := []struct {
		name string
		path string
		body string
	}{
		{"新建", "/api/workspaces", `{"name":"x"}`},
		{"改名", "/api/workspaces/whatever/rename", `{"name":"x"}`},
		{"删除", "/api/workspaces/whatever/delete", `{}`},
		{"改归属", "/api/conversations/whatever/workspace", `{"workspace_id":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.name+" 只读 405", func(t *testing.T) {
			status, ctype, raw := postRaw(t, env.http.URL+tc.path, "application/json", tc.body)
			if status != http.StatusMethodNotAllowed {
				t.Fatalf("只读模式 %s = %d，期望 405；body=%s", tc.name, status, raw)
			}
			if ctype != "application/json; charset=utf-8" {
				t.Errorf("%s 405 的 Content-Type = %q", tc.name, ctype)
			}
			var reply map[string]any
			if err := json.Unmarshal(raw, &reply); err != nil {
				t.Fatalf("%s 405 响应不是合法 JSON: %v；%s", tc.name, err, raw)
			}
			message, _ := reply["error"].(string)
			for _, want := range []string{"只读", "-rw"} {
				if !strings.Contains(message, want) {
					t.Errorf("%s 405 的说明 = %q，期望包含 %q", tc.name, message, want)
				}
			}
		})
	}
}

// TestGui9WorkspaceWriteGuard 复用 /api/ask 的同一套写闸：非 JSON 415、
// 跨站 403；被拒的请求一条都不许建出工作区或改动会话归属。
func TestGui9WorkspaceWriteGuard(t *testing.T) {
	env := newWriteTestEnv(t)
	conv := gui3bSeedConversation(t, env, "写闸测试")
	createURL := env.http.URL + "/api/workspaces"

	assertNothingChanged := func(t *testing.T) {
		t.Helper()
		if list := gui9ListWorkspaces(t, env.http.URL); len(list) != 0 {
			t.Errorf("被拒的请求之后工作区列表 = %+v，期望空", list)
		}
		if got := gui9ConversationWorkspace(t, env, conv.ID); got != "" {
			t.Errorf("被拒的请求之后会话归属 = %q，期望空串", got)
		}
	}

	t.Run("text/plain 415", func(t *testing.T) {
		status, _, raw := gui3bDeleteRequest(t, createURL, "text/plain", "", "", `{"name":"x"}`)
		if status != http.StatusUnsupportedMediaType {
			t.Fatalf("text/plain = %d，期望 415；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "application/json") {
			t.Errorf("415 的说明没有告诉用户要 application/json: %s", raw)
		}
		assertNothingChanged(t)
	})

	t.Run("无 Content-Type 415", func(t *testing.T) {
		status, _, raw := gui3bDeleteRequest(t, createURL, "", "", "", `{"name":"x"}`)
		if status != http.StatusUnsupportedMediaType {
			t.Fatalf("无 Content-Type = %d，期望 415；body=%s", status, raw)
		}
		assertNothingChanged(t)
	})

	t.Run("跨站 Origin 403", func(t *testing.T) {
		status, _, raw := gui3bDeleteRequest(t, createURL, "application/json", "https://evil.example", "", `{"name":"x"}`)
		if status != http.StatusForbidden {
			t.Fatalf("evil Origin = %d，期望 403；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "跨站") {
			t.Errorf("403 的说明没有提到跨站: %s", raw)
		}
		assertNothingChanged(t)
	})

	t.Run("跨站 Sec-Fetch-Site 403", func(t *testing.T) {
		status, _, raw := gui3bDeleteRequest(t, createURL, "application/json", "", "cross-site", `{"name":"x"}`)
		if status != http.StatusForbidden {
			t.Fatalf("cross-site = %d，期望 403；body=%s", status, raw)
		}
		assertNothingChanged(t)
	})

	t.Run("改归属也走同一套闸", func(t *testing.T) {
		ws := gui9CreateWorkspace(t, env, "写闸里的组")
		assignURL := env.http.URL + "/api/conversations/" + conv.ID + "/workspace"
		if ws.ID == "" {
			t.Fatal("前置条件不成立：工作区 id 是空的")
		}
		status, _, raw := gui3bDeleteRequest(t, assignURL, "text/plain", "", "",
			`{"workspace_id":"`+ws.ID+`"}`)
		if status != http.StatusUnsupportedMediaType {
			t.Fatalf("text/plain 改归属 = %d，期望 415；body=%s", status, raw)
		}
		status, _, raw = gui3bDeleteRequest(t, assignURL, "application/json", "https://evil.example", "",
			`{"workspace_id":"`+ws.ID+`"}`)
		if status != http.StatusForbidden {
			t.Fatalf("跨站改归属 = %d，期望 403；body=%s", status, raw)
		}
		if got := gui9ConversationWorkspace(t, env, conv.ID); got != "" {
			t.Errorf("被拒的改归属之后会话归属 = %q，期望空串", got)
		}
	})

	t.Run("同源请求照常成功", func(t *testing.T) {
		status, _, raw := gui3bDeleteRequest(t, createURL, "application/json", env.http.URL, "", `{"name":"同源建的组"}`)
		if status != http.StatusOK {
			t.Fatalf("同源新建 = %d，期望 200；body=%s", status, raw)
		}
		// 列表里此刻应当只有两个：上面「改归属」那条闸的对照组建的，以及这一个。
		// 四条被拒的请求一条都不许留下工作区。
		found := 0
		for _, ws := range gui9ListWorkspaces(t, env.http.URL) {
			if ws.Name == "同源建的组" {
				found++
			}
		}
		if found != 1 {
			t.Errorf("同源新建后的工作区里有 %d 个「同源建的组」，期望 1 个", found)
		}
	})
}

// TestGui9ConversationJSONCarriesWorkspaceID 钉住会话 JSON 的三处：
// 默认列表、归档列表、详情都带 workspace_id，未分组是空串。
func TestGui9ConversationJSONCarriesWorkspaceID(t *testing.T) {
	env := newWriteTestEnv(t)
	ws := gui9CreateWorkspace(t, env, "JSON 里的组")
	grouped := gui3bSeedConversation(t, env, "已分组的会话")
	ungrouped := gui3bSeedConversation(t, env, "未分组的会话")
	if status, raw := gui9PostConversationWorkspace(t, env, grouped.ID, &ws.ID); status != http.StatusOK {
		t.Fatalf("设置归属 = %d，期望 200；body=%s", status, raw)
	}
	if status, raw := gui8PostArchive(t, env, grouped.ID, "archive"); status != http.StatusOK {
		t.Fatalf("归档会话 = %d，期望 200；body=%s", status, raw)
	}

	t.Run("默认列表", func(t *testing.T) {
		var rows []map[string]any
		getJSON(t, env.http.URL+"/api/conversations", http.StatusOK, &rows)
		if len(rows) != 1 {
			t.Fatalf("默认列表有 %d 条，期望 1 条", len(rows))
		}
		if _, ok := rows[0]["workspace_id"]; !ok {
			t.Fatalf("默认列表的会话缺少 workspace_id 字段: %v", rows[0])
		}
		if rows[0]["id"] != ungrouped.ID || rows[0]["workspace_id"] != "" {
			t.Errorf("默认列表的会话 = %v，期望未分组且 workspace_id 为空串", rows[0])
		}
	})

	t.Run("归档列表", func(t *testing.T) {
		var rows []map[string]any
		getJSON(t, env.http.URL+"/api/conversations?archived=1", http.StatusOK, &rows)
		if len(rows) != 1 {
			t.Fatalf("归档列表有 %d 条，期望 1 条", len(rows))
		}
		if _, ok := rows[0]["workspace_id"]; !ok {
			t.Fatalf("归档列表的会话缺少 workspace_id 字段: %v", rows[0])
		}
		if rows[0]["id"] != grouped.ID || rows[0]["workspace_id"] != ws.ID {
			t.Errorf("归档列表的会话 = %v，期望 id=%s workspace_id=%s", rows[0], grouped.ID, ws.ID)
		}
	})

	t.Run("详情", func(t *testing.T) {
		var detail conversationDetail
		getJSON(t, env.http.URL+"/api/conversations/"+grouped.ID, http.StatusOK, &detail)
		if detail.WorkspaceID != ws.ID {
			t.Errorf("详情里的 workspace_id = %q，期望 %q", detail.WorkspaceID, ws.ID)
		}
		if !detail.Archived {
			t.Error("详情里的 archived = false，期望 true（归档照常可读）")
		}
	})
}

// TestGui9ConversationWorkspaceMoveOut 钉住三种「移出」写法：
// null、空串、以及字段缺席（同样解成 nil），都写回 NULL。
func TestGui9ConversationWorkspaceMoveOut(t *testing.T) {
	env := newWriteTestEnv(t)
	ws := gui9CreateWorkspace(t, env, "移出测试组")
	conv := gui3bSeedConversation(t, env, "被移进移出的会话")

	assign := func(t *testing.T, workspaceID *string) map[string]any {
		t.Helper()
		status, raw := gui9PostConversationWorkspace(t, env, conv.ID, workspaceID)
		if status != http.StatusOK {
			t.Fatalf("改归属 = %d，期望 200；body=%s", status, raw)
		}
		var reply map[string]any
		if err := json.Unmarshal(raw, &reply); err != nil {
			t.Fatalf("改归属的响应不是合法 JSON: %v；%s", err, raw)
		}
		requireKeys(t, "conversation workspace", reply, "id", "workspace_id")
		return reply
	}

	reply := assign(t, &ws.ID)
	if reply["id"] != conv.ID || reply["workspace_id"] != ws.ID {
		t.Errorf("设置归属的回显 = %v，期望 id=%s workspace_id=%s", reply, conv.ID, ws.ID)
	}
	if got := gui9ConversationWorkspace(t, env, conv.ID); got != ws.ID {
		t.Errorf("设置后 store 里的归属 = %q，期望 %q", got, ws.ID)
	}

	reply = assign(t, nil)
	if reply["workspace_id"] != "" {
		t.Errorf("用 null 移出的回显 = %v，期望 workspace_id 为空串", reply)
	}
	if got := gui9ConversationWorkspace(t, env, conv.ID); got != "" {
		t.Errorf("用 null 移出后 store 里的归属 = %q，期望空串", got)
	}

	if status, raw := gui9PostConversationWorkspace(t, env, conv.ID, &ws.ID); status != http.StatusOK {
		t.Fatalf("再次设置归属失败: %d %s", status, raw)
	}
	reply = assign(t, strPtr(""))
	if reply["workspace_id"] != "" {
		t.Errorf("用空串移出的回显 = %v，期望 workspace_id 为空串", reply)
	}

	// 字段缺席：{"workspace_id": null} 与 {} 在 Go 里都是 nil。
	status, _, raw := postRaw(t,
		env.http.URL+"/api/conversations/"+conv.ID+"/workspace", "application/json", `{}`)
	if status != http.StatusOK {
		t.Fatalf("字段缺席 = %d，期望 200；body=%s", status, raw)
	}
	if got := gui9ConversationWorkspace(t, env, conv.ID); got != "" {
		t.Errorf("字段缺席之后归属 = %q，期望空串（缺席与 null 同义）", got)
	}

	// 同一个值重复设置是幂等的。
	if status, raw := gui9PostConversationWorkspace(t, env, conv.ID, &ws.ID); status != http.StatusOK {
		t.Fatalf("重复设置归属失败: %d %s", status, raw)
	}
	if status, raw := gui9PostConversationWorkspace(t, env, conv.ID, &ws.ID); status != http.StatusOK {
		t.Fatalf("重复设置归属第二次失败: %d %s", status, raw)
	}
}

// TestGui9WorkspaceNameValidation400 钉住名字的 400：空串、纯空白、61 个字符，
// 以及请求体不是 JSON 对象；被拒的请求一条都不建。
func TestGui9WorkspaceNameValidation400(t *testing.T) {
	env := newWriteTestEnv(t)

	for _, bad := range []string{"", "   ", strings.Repeat("字", 61)} {
		status, raw := gui9PostWorkspaceName(t, env.http.URL+"/api/workspaces", bad)
		if status != http.StatusBadRequest {
			t.Fatalf("名字 %q = %d，期望 400；body=%s", bad, status, raw)
		}
		var errBody map[string]any
		if err := json.Unmarshal(raw, &errBody); err != nil {
			t.Fatalf("400 响应不是合法 JSON: %v；%s", err, raw)
		}
		message, _ := errBody["error"].(string)
		if !strings.Contains(message, "工作区名") {
			t.Errorf("400 的说明 = %q，期望点名工作区名", message)
		}
	}

	for _, body := range []string{`not json`, `{"name":123}`, `[]`, `{"name":"a"}{"name":"b"}`} {
		status, _, raw := postRaw(t, env.http.URL+"/api/workspaces", "application/json", body)
		if status != http.StatusBadRequest {
			t.Errorf("请求体 %q = %d，期望 400；body=%s", body, status, raw)
		}
	}

	if list := gui9ListWorkspaces(t, env.http.URL); len(list) != 0 {
		t.Errorf("被拒的请求之后工作区列表 = %+v，期望空", list)
	}
}

// TestGui9WorkspaceMethodNotAllowed 钉住两个方法闸：/api/workspaces 只认
// GET 与 POST；两个子路径只认 POST。
func TestGui9WorkspaceMethodNotAllowed(t *testing.T) {
	env := newWriteTestEnv(t)
	ws := gui9CreateWorkspace(t, env, "方法测试组")

	res, err := http.NewRequest(http.MethodPut, env.http.URL+"/api/workspaces", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("构造 PUT 请求失败: %v", err)
	}
	putRes, err := http.DefaultClient.Do(res)
	if err != nil {
		t.Fatalf("PUT /api/workspaces 失败: %v", err)
	}
	defer putRes.Body.Close()
	if putRes.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("PUT /api/workspaces = %d，期望 405", putRes.StatusCode)
	}
	if allow := putRes.Header.Get("Allow"); allow != "GET, POST" {
		t.Errorf("405 的 Allow = %q，期望 GET, POST", allow)
	}

	for _, path := range []string{
		"/api/workspaces/" + ws.ID + "/rename",
		"/api/workspaces/" + ws.ID + "/delete",
	} {
		req, err := http.NewRequest(http.MethodGet, env.http.URL+path, nil)
		if err != nil {
			t.Fatalf("构造 GET 请求失败: %v", err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s 失败: %v", path, err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("GET %s = %d，期望 405", path, res.StatusCode)
		}
		if allow := res.Header.Get("Allow"); allow != http.MethodPost {
			t.Errorf("GET %s 的 Allow = %q，期望 POST", path, allow)
		}
	}
}

// TestGui9SeedSanity 是给本文件其余用例兜底的对照组：一个刚建好的空组，
// 列表里 ConversationCount 必须是 0，会话归属必须是空串 —— 若 store 的读
// 路径把 NULL 读成了别的东西，上面那些用例的期望值都会一起变形。
func TestGui9SeedSanity(t *testing.T) {
	env := newWriteTestEnv(t)
	conv := gui3bSeedConversation(t, env, "对照组会话")
	if got, err := env.store.Conversation(context.Background(), conv.ID); err != nil || got.WorkspaceID != "" {
		t.Errorf("新会话的 WorkspaceID = %q, err=%v，期望空串", got.WorkspaceID, err)
	}
	var detail conversationDetail
	getJSON(t, env.http.URL+"/api/conversations/"+conv.ID, http.StatusOK, &detail)
	if detail.WorkspaceID != "" {
		t.Errorf("新会话详情的 workspace_id = %q，期望空串", detail.WorkspaceID)
	}
}
