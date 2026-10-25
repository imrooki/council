// gui8_archive_test.go 覆盖 GUI 路线「会话归档」的后端：
//   - 默认列表不含已归档；?archived=1 只含已归档；archived=0 与默认等价；
//   - 归档 / 取消归档幂等，未知与已删会话 404，只读模式 405；
//   - 写闸复用 checkWriteRequest（JSON + 同源）；
//   - **归档之后 /api/ask 续接仍然 202，并且消息真的追加进同一场会话**；
//   - 详情里带 archived，归档不影响详情 / 消息 / 用量三个读入口。
//
// 真实点击、视图切换与刷新持久化由 CDP 验收覆盖（见交付报告）；本文件末尾
// 只钉 index.html 里的结构锚点，防止改前端时把切换或菜单项静默删掉。
package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"council/internal/conversation"
)

// gui8ArchiveRequest 发一次归档 / 取消归档请求。与删除共用同一套写闸，
// 这里复用 gui3bDeleteRequest 的头部控制能力，只把名字换成语义中性的包装。
func gui8ArchiveRequest(t *testing.T, url, contentType, origin, secFetchSite, body string) (int, string, []byte) {
	t.Helper()
	return gui3bDeleteRequest(t, url, contentType, origin, secFetchSite, body)
}

// gui8PostArchive 发一次合法的归档 / 取消归档请求，返回状态与 body。
func gui8PostArchive(t *testing.T, env *writeTestEnv, id, action string) (int, []byte) {
	t.Helper()
	status, _, raw := postRaw(t, env.http.URL+"/api/conversations/"+id+"/"+action,
		"application/json", `{}`)
	return status, raw
}

// gui8ListArchived 拉一次归档视图的原始 JSON 行，便于检查 archived 字段。
func gui8ListArchived(t *testing.T, env *writeTestEnv, query string) []map[string]any {
	t.Helper()
	var rows []map[string]any
	getJSON(t, env.http.URL+"/api/conversations"+query, http.StatusOK, &rows)
	return rows
}

// TestGui8ArchiveListAndUnarchive 是主路径：归档后默认列表消失、归档视图
// 出现、详情 / 消息 / 用量仍可读、取消归档后回到默认列表。
func TestGui8ArchiveListAndUnarchive(t *testing.T) {
	env := newWriteTestEnv(t)
	target := gui3bSeedConversation(t, env, "要归档的会话")
	kept := gui3bSeedConversation(t, env, "留在主列表的会话")

	// 归档之前：两个视图分别只有该有的那一场，每条都带 archived=false。
	main := gui8ListArchived(t, env, "")
	if len(main) != 2 {
		t.Fatalf("归档前默认列表有 %d 条，期望 2 条", len(main))
	}
	for _, row := range main {
		if _, ok := row["archived"]; !ok {
			t.Fatalf("会话 %v 缺少 archived 字段", row["id"])
		}
		if row["archived"] != false {
			t.Errorf("归档前 %v 的 archived = %v，期望 false", row["id"], row["archived"])
		}
	}
	if rows := gui8ListArchived(t, env, "?archived=1"); len(rows) != 0 {
		t.Fatalf("归档前归档视图有 %d 条，期望 0 条", len(rows))
	}

	status, raw := gui8PostArchive(t, env, target.ID, "archive")
	if status != http.StatusOK {
		t.Fatalf("POST archive = %d，期望 200；body=%s", status, raw)
	}
	var reply map[string]json.RawMessage
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("archive 200 响应不是合法 JSON: %v；%s", err, raw)
	}
	if len(reply) != 2 {
		t.Errorf("archive 200 响应应当恰好有 id/archived 两个字段，实际 %v", reply)
	}
	var replyID string
	var replyArchived bool
	if err := json.Unmarshal(reply["id"], &replyID); err != nil || replyID != target.ID {
		t.Errorf("archive 响应 id = %q (err=%v)，期望 %q", replyID, err, target.ID)
	}
	if err := json.Unmarshal(reply["archived"], &replyArchived); err != nil || !replyArchived {
		t.Errorf("archive 响应 archived = %v (err=%v)，期望 true", replyArchived, err)
	}

	t.Run("默认列表不含已归档", func(t *testing.T) {
		rows := gui8ListArchived(t, env, "")
		if len(rows) != 1 || rows[0]["id"] != kept.ID {
			t.Errorf("归档后默认列表 = %v，期望只剩 %s", rows, kept.ID)
		}
	})

	t.Run("archived=1 只含已归档", func(t *testing.T) {
		rows := gui8ListArchived(t, env, "?archived=1")
		if len(rows) != 1 || rows[0]["id"] != target.ID {
			t.Fatalf("归档视图 = %v，期望只有 %s", rows, target.ID)
		}
		if rows[0]["archived"] != true {
			t.Errorf("归档视图里 %v 的 archived = %v，期望 true", target.ID, rows[0]["archived"])
		}
		if rows[0]["title"] != target.Title {
			t.Errorf("归档视图里的标题 = %v，期望 %q", rows[0]["title"], target.Title)
		}
	})

	t.Run("archived=0 与默认等价", func(t *testing.T) {
		rows := gui8ListArchived(t, env, "?archived=0")
		if len(rows) != 1 || rows[0]["id"] != kept.ID {
			t.Errorf("archived=0 列表 = %v，期望与默认列表一致", rows)
		}
	})

	t.Run("详情、消息、用量照常可读", func(t *testing.T) {
		var detail conversationDetail
		getJSON(t, env.http.URL+"/api/conversations/"+target.ID, http.StatusOK, &detail)
		if detail.ID != target.ID || !detail.Archived {
			t.Errorf("归档后详情 = %+v，期望 id=%s 且 archived=true", detail, target.ID)
		}
		var page messagesResponse
		getJSON(t, env.http.URL+"/api/conversations/"+target.ID+"/messages", http.StatusOK, &page)
		if len(page.Messages) != 1 {
			t.Errorf("归档后消息条数 = %d，期望 1（归档不阻止打开）", len(page.Messages))
		}
		var usage usageResponse
		getJSON(t, env.http.URL+"/api/conversations/"+target.ID+"/usage", http.StatusOK, &usage)
		if len(usage.PerAgent) != 1 || usage.PerAgent[0].Turns != 1 {
			t.Errorf("归档后用量 = %+v，期望 c1 的一轮", usage.PerAgent)
		}
	})

	t.Run("重复归档是幂等 200", func(t *testing.T) {
		status, raw := gui8PostArchive(t, env, target.ID, "archive")
		if status != http.StatusOK {
			t.Fatalf("重复 archive = %d，期望 200；body=%s", status, raw)
		}
		if rows := gui8ListArchived(t, env, "?archived=1"); len(rows) != 1 || rows[0]["id"] != target.ID {
			t.Errorf("重复归档后归档视图 = %v，期望仍只有 %s", rows, target.ID)
		}
	})

	status, raw = gui8PostArchive(t, env, target.ID, "unarchive")
	if status != http.StatusOK {
		t.Fatalf("POST unarchive = %d，期望 200；body=%s", status, raw)
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("unarchive 200 响应不是合法 JSON: %v；%s", err, raw)
	}
	if err := json.Unmarshal(reply["archived"], &replyArchived); err != nil || replyArchived {
		t.Errorf("unarchive 响应 archived = %v (err=%v)，期望 false", replyArchived, err)
	}

	t.Run("取消归档后回到默认列表", func(t *testing.T) {
		rows := gui8ListArchived(t, env, "")
		if len(rows) != 2 {
			t.Fatalf("取消归档后默认列表有 %d 条，期望 2 条", len(rows))
		}
		if rows := gui8ListArchived(t, env, "?archived=1"); len(rows) != 0 {
			t.Errorf("取消归档后归档视图 = %v，期望空", rows)
		}
		conv, err := env.store.Conversation(context.Background(), target.ID)
		if err != nil {
			t.Fatalf("取消归档后 Conversation 失败: %v", err)
		}
		if conv.Archived {
			t.Error("取消归档后 Conversation 的 Archived = true，期望 false")
		}
	})

	t.Run("重复取消归档是幂等 200", func(t *testing.T) {
		status, raw := gui8PostArchive(t, env, target.ID, "unarchive")
		if status != http.StatusOK {
			t.Fatalf("重复 unarchive = %d，期望 200；body=%s", status, raw)
		}
	})
}

// TestGui8ArchiveUnknownAndDeleted404 钉住两种 404：未知 id 与已软删会话。
// 已删的会话在库里还有行，但对外不存在，不能让归档把它重新露出来。
func TestGui8ArchiveUnknownAndDeleted404(t *testing.T) {
	env := newWriteTestEnv(t)
	kept := gui3bSeedConversation(t, env, "陪跑的会话")
	gone := gui3bSeedConversation(t, env, "先删掉的会话")

	if status, _, raw := postRaw(t, env.http.URL+"/api/conversations/"+gone.ID+"/delete",
		"application/json", `{}`); status != http.StatusOK {
		t.Fatalf("准备删除会话失败: %d %s", status, raw)
	}

	const unknown = "conv-does-not-exist"
	for _, action := range []string{"archive", "unarchive"} {
		t.Run(action+" 未知 id 404", func(t *testing.T) {
			status, raw := gui8PostArchive(t, env, unknown, action)
			if status != http.StatusNotFound {
				t.Fatalf("%s 未知 id = %d，期望 404；body=%s", action, status, raw)
			}
			var errBody map[string]any
			if err := json.Unmarshal(raw, &errBody); err != nil {
				t.Fatalf("404 响应不是合法 JSON: %v；%s", err, raw)
			}
			message, _ := errBody["error"].(string)
			if !strings.Contains(message, unknown) || !strings.Contains(message, "会话不存在") {
				t.Errorf("404 的说明 = %q，期望点名 id 与会话不存在", message)
			}
		})
		t.Run(action+" 已删会话 404", func(t *testing.T) {
			status, raw := gui8PostArchive(t, env, gone.ID, action)
			if status != http.StatusNotFound {
				t.Fatalf("%s 已删会话 = %d，期望 404；body=%s", action, status, raw)
			}
		})
	}

	t.Run("被拒的请求没有改动列表", func(t *testing.T) {
		rows := gui8ListArchived(t, env, "")
		if len(rows) != 1 || rows[0]["id"] != kept.ID {
			t.Errorf("404 之后默认列表 = %v，期望只剩 %s", rows, kept.ID)
		}
		if rows := gui8ListArchived(t, env, "?archived=1"); len(rows) != 0 {
			t.Errorf("404 之后归档视图 = %v，期望空", rows)
		}
	})
}

// TestGui8ArchiveReadOnly405 钉住只读模式：两个写端点 405 + 说明；
// 读侧的归档视图照常工作（只读看板也能查已归档的会话）。
func TestGui8ArchiveReadOnly405(t *testing.T) {
	env := newServeTestEnv(t, nil)

	for _, action := range []string{"archive", "unarchive"} {
		status, ctype, raw := postRaw(t,
			env.http.URL+"/api/conversations/whatever/"+action, "application/json", `{}`)
		if status != http.StatusMethodNotAllowed {
			t.Fatalf("只读模式 %s = %d，期望 405；body=%s", action, status, raw)
		}
		if ctype != "application/json; charset=utf-8" {
			t.Errorf("%s 405 的 Content-Type = %q", action, ctype)
		}
		var reply map[string]any
		if err := json.Unmarshal(raw, &reply); err != nil {
			t.Fatalf("%s 405 响应不是合法 JSON: %v；%s", action, err, raw)
		}
		message, _ := reply["error"].(string)
		for _, want := range []string{"只读", "-rw", "归档"} {
			if !strings.Contains(message, want) {
				t.Errorf("%s 405 的说明 = %q，期望包含 %q", action, message, want)
			}
		}
	}

	res, err := http.Get(env.http.URL + "/api/conversations?archived=1")
	if err != nil {
		t.Fatalf("只读模式 GET archived=1 失败: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("只读模式 GET archived=1 = %d，期望 200", res.StatusCode)
	}
	var list []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("只读模式归档视图不是合法 JSON 数组: %v", err)
	}
	if list == nil || len(list) != 0 {
		t.Errorf("空库的只读归档视图 = %#v，期望非 nil 的空数组", list)
	}
}

// TestGui8ArchiveWriteGuard 复用 /api/ask 的同一套写闸：非 JSON 415、
// 跨站 403；被拒的请求一条都不许改动会话的归档状态。
func TestGui8ArchiveWriteGuard(t *testing.T) {
	env := newWriteTestEnv(t)
	conv := gui3bSeedConversation(t, env, "写闸测试")
	archiveURL := env.http.URL + "/api/conversations/" + conv.ID + "/archive"

	assertNotArchived := func(t *testing.T) {
		t.Helper()
		got, err := env.store.Conversation(context.Background(), conv.ID)
		if err != nil {
			t.Fatalf("读取会话失败: %v", err)
		}
		if got.Archived {
			t.Error("被拒绝的请求之后会话变成了已归档")
		}
	}

	t.Run("text/plain 415", func(t *testing.T) {
		status, _, raw := gui8ArchiveRequest(t, archiveURL, "text/plain", "", "", `{}`)
		if status != http.StatusUnsupportedMediaType {
			t.Fatalf("text/plain = %d，期望 415；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "application/json") {
			t.Errorf("415 的说明没有告诉用户要 application/json: %s", raw)
		}
		assertNotArchived(t)
	})

	t.Run("无 Content-Type 415", func(t *testing.T) {
		status, _, raw := gui8ArchiveRequest(t, archiveURL, "", "", "", `{}`)
		if status != http.StatusUnsupportedMediaType {
			t.Fatalf("无 Content-Type = %d，期望 415；body=%s", status, raw)
		}
		assertNotArchived(t)
	})

	t.Run("跨站 Origin 403", func(t *testing.T) {
		status, _, raw := gui8ArchiveRequest(t, archiveURL, "application/json", "https://evil.example", "", `{}`)
		if status != http.StatusForbidden {
			t.Fatalf("evil Origin = %d，期望 403；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "跨站") {
			t.Errorf("403 的说明没有提到跨站: %s", raw)
		}
		assertNotArchived(t)
	})

	t.Run("跨站 Sec-Fetch-Site 403", func(t *testing.T) {
		status, _, raw := gui8ArchiveRequest(t, archiveURL, "application/json", "", "cross-site", `{}`)
		if status != http.StatusForbidden {
			t.Fatalf("cross-site = %d，期望 403；body=%s", status, raw)
		}
		assertNotArchived(t)
	})

	t.Run("同源 Origin 200 且归档", func(t *testing.T) {
		status, _, raw := gui8ArchiveRequest(t, archiveURL, "application/json", env.http.URL, "", `{}`)
		if status != http.StatusOK {
			t.Fatalf("同源 Origin = %d，期望 200；body=%s", status, raw)
		}
		got, err := env.store.Conversation(context.Background(), conv.ID)
		if err != nil || !got.Archived {
			t.Errorf("同源请求返回 200 之后 Archived = %v, err=%v，期望 true", got.Archived, err)
		}
	})

	t.Run("空 body 也能取消归档", func(t *testing.T) {
		status, _, raw := postRaw(t,
			env.http.URL+"/api/conversations/"+conv.ID+"/unarchive", "application/json", "")
		if status != http.StatusOK {
			t.Fatalf("空 body unarchive = %d，期望 200；body=%s", status, raw)
		}
		got, err := env.store.Conversation(context.Background(), conv.ID)
		if err != nil || got.Archived {
			t.Errorf("取消归档后 Archived = %v, err=%v，期望 false", got.Archived, err)
		}
	})
}

// TestGui8AskContinuesArchivedConversation 是本片最硬的一条：归档不阻止
// 继续对话。归档之后带着 conversation 的 /api/ask 必须仍然 202，并且真的
// 把追问写进同一场会话，而不是另开一场。
func TestGui8AskContinuesArchivedConversation(t *testing.T) {
	t.Setenv("COUNCIL_FAKE_CLI", "1")
	env := newWriteTestEnv(t)
	_, convID := followupSeed(t, env, "归档前的第一问")

	ctx := context.Background()
	before, err := env.store.Messages(ctx, convID, 0, 0)
	if err != nil {
		t.Fatalf("读归档前消息失败: %v", err)
	}

	if status, raw := gui8PostArchive(t, env, convID, "archive"); status != http.StatusOK {
		t.Fatalf("归档种子会话 = %d，期望 200；body=%s", status, raw)
	}
	conv, err := env.store.Conversation(ctx, convID)
	if err != nil || !conv.Archived {
		t.Fatalf("归档后 Conversation = %+v, err=%v，期望 Archived=true", conv, err)
	}
	if rows := gui8ListArchived(t, env, ""); len(rows) != 0 {
		t.Fatalf("归档后默认列表 = %v，期望空", rows)
	}
	if rows := gui8ListArchived(t, env, "?archived=1"); len(rows) != 1 || rows[0]["id"] != convID {
		t.Fatalf("归档后归档视图 = %v，期望只有 %s", rows, convID)
	}

	// postAsk 自己断言 202；这一句就是「归档后 /api/ask 续接仍 202」的落点。
	jobID := postAsk(t, env, map[string]any{
		"prompt":       "归档后的追问",
		"members":      []string{"c1"},
		"rounds":       1,
		"conversation": convID,
	})
	startedID, res := followupWaitJob(t, env, jobID)
	if startedID != convID {
		t.Errorf("归档后续接的 OnStart convID = %q，期望 %q", startedID, convID)
	}
	if res.Status != "ok" {
		t.Errorf("归档后续接的状态 = %q，期望 ok", res.Status)
	}
	if res.ConversationID != convID {
		t.Errorf("归档后续接的 result.conversation_id = %q，期望 %q", res.ConversationID, convID)
	}

	after, err := env.store.Messages(ctx, convID, 0, 0)
	if err != nil {
		t.Fatalf("读归档后消息失败: %v", err)
	}
	if len(after) <= len(before) {
		t.Fatalf("归档后续接的消息数 = %d，归档前 = %d；追问应当追加进同一场会话", len(after), len(before))
	}
	foundPrompt := false
	for _, m := range after {
		if m.ConvID != convID {
			t.Errorf("消息 seq=%d 落在会话 %q，期望全部落在 %q", m.Seq, m.ConvID, convID)
		}
		if m.Kind == conversation.KindUser && strings.Contains(m.Text, "归档后的追问") {
			foundPrompt = true
		}
	}
	if !foundPrompt {
		t.Errorf("归档后的会话里找不到追问正文；消息 = %+v", after)
	}

	// 继续对话不该顺手把会话从归档里拿出来：可见性只由归档接口改变。
	conv, err = env.store.Conversation(ctx, convID)
	if err != nil || !conv.Archived {
		t.Errorf("续接之后 Archived = %v, err=%v，期望仍然 true", conv.Archived, err)
	}
	if rows := gui8ListArchived(t, env, ""); len(rows) != 0 {
		t.Errorf("续接之后默认列表 = %v，期望仍然不含已归档会话", rows)
	}
	if rows := gui8ListArchived(t, env, "?archived=1"); len(rows) != 1 || rows[0]["id"] != convID {
		t.Errorf("续接之后归档视图 = %v，期望仍然只有 %s", rows, convID)
	}
}

// TestGui8ArchivedQueryRejectsGarbage 钉住视图参数：只认 0/1，别的值 400，
// 不静默退回默认列表（那会让用户以为归档的会话丢了）。
func TestGui8ArchivedQueryRejectsGarbage(t *testing.T) {
	env := newServeTestEnv(t, nil)
	for _, query := range []string{"?archived=yes", "?archived=", "?archived=2", "?archived=1&archived=0"} {
		var errBody map[string]any
		getJSON(t, env.http.URL+"/api/conversations"+query, http.StatusBadRequest, &errBody)
		message, _ := errBody["error"].(string)
		if !strings.Contains(message, "archived") {
			t.Errorf("%s 的 400 说明 = %q，期望点名 archived", query, message)
		}
	}
}

// TestGui8IndexArchiveMarkup 钉 index.html 里的结构锚点：视图切换、菜单项、
// 两种视图的数据源与「切视图不清消息」的实现形状。真实点击由 CDP 验收覆盖。
func TestGui8IndexArchiveMarkup(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		// 左侧顶部的视图切换。
		`id="toggle-archived"`,
		`id="archived-count"`,
		`archivedToggle.addEventListener('click'`,
		`setArchivedView(!state.viewArchived)`,
		// 两个视图的数据源与标题旁的灰字。
		`archivedConversations: []`,
		`state.viewArchived ? state.archivedConversations : state.conversations`,
		`tag.textContent = '已归档'`,
		`'archived-tag'`,
		// ⋯ 菜单两种文案与两个端点。
		`archive.textContent = conversation.archived ? '取消归档' : '归档'`,
		`? '/archive' : '/unarchive'`,
		// 切视图只换列表数据源：不调用 selectConversation / 不清 messageList。
		`function setArchivedView(next)`,
		`// 当前会话不在新视图里时`,
		// 刷新列表时不为「当前会话不在当前视图」自动改选第一条。
		`if (!state.currentId && !state.newTarget && shown.length > 0)`,
		`await loadConversationLists()`,
		// 归档后的提示与失败路径。
		`已归档「' + title + '」`,
		`取消归档失败：`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}
	if strings.Contains(html, "innerHTML") {
		t.Errorf("index.html 里出现了 innerHTML，前端必须保持 0 处")
	}
}
