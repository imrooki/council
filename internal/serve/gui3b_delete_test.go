// gui3b_delete_test.go 覆盖 GUI 路线第 3 片后半（3b）的后端：
// POST /api/conversations/{id}/delete 的成功、未知 / 已删 404、只读 405、
// 非 JSON 415、跨站 403，以及删除之后四个读入口全都看不到那一场会话。
//
// 前端点击与确认框由 CDP 真驱动（交付报告里记录实际输出）；本文件末尾只钉
// index.html 里的结构锚点，防止改着改着把确认框或「删除」菜单项删掉。
package serve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"council/internal/conversation"
)

// gui3bSeedConversation 造一场带成员、消息与一轮用量的会话，作为删除目标。
// 四张表都有数据，删除之后才能验证「四个读入口都看不见」不是靠空会话蒙混
// 过关。不走 /api/ask：删除不跑 provider，用假议会造数只会扩大失败面。
func gui3bSeedConversation(t *testing.T, env *writeTestEnv, title string) conversation.Conversation {
	t.Helper()
	var conv conversation.Conversation
	err := env.store.WriteTx(context.Background(), func(ctx context.Context, tx conversation.Tx) error {
		var err error
		conv, err = tx.CreateConversation(ctx, title)
		if err != nil {
			return err
		}
		if err := tx.AddParticipant(ctx, conversation.Participant{
			ConvID: conv.ID, AgentID: "c1", DisplayName: "C1",
			Provider: "claude", Active: true,
		}); err != nil {
			return err
		}
		if _, err := tx.AppendMessage(ctx, conversation.Message{
			ConvID: conv.ID, SenderID: "c1",
			SenderProvider: "claude", SenderModel: "opus",
			Kind: conversation.KindAgent, Text: "删除目标里的回复",
		}); err != nil {
			return err
		}
		return tx.RecordUsage(ctx, conversation.TurnUsage{
			TurnID: "turn-" + conv.ID, ConvID: conv.ID, AgentID: "c1",
			Outcome: 1, InputTokens: intPtr(10), OutputTokens: intPtr(5),
			CostUSD: floatPtr(0.01),
		})
	})
	if err != nil {
		t.Fatalf("造删除测试会话 %q 失败: %v", title, err)
	}
	return conv
}

// gui3bDeleteRequest 带自定义头发一次删除请求，供写闸的用例使用。
func gui3bDeleteRequest(t *testing.T, url, contentType, origin, secFetchSite, body string) (int, string, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("构造删除请求失败: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if secFetchSite != "" {
		req.Header.Set("Sec-Fetch-Site", secFetchSite)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s 失败: %v", url, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("读删除响应失败: %v", err)
	}
	return res.StatusCode, res.Header.Get("Content-Type"), raw
}

// TestGui3bDeleteConversation 钉住成功路径：200 + {"id","deleted":true}，
// 然后 /api/conversations、/{id}、/messages、/usage 四个入口全部看不到它，
// 而另一场会话照常可读。
func TestGui3bDeleteConversation(t *testing.T) {
	env := newWriteTestEnv(t)
	target := gui3bSeedConversation(t, env, "要删掉的会话")
	kept := gui3bSeedConversation(t, env, "留下的会话")

	status, ctype, raw := postRaw(t,
		env.http.URL+"/api/conversations/"+target.ID+"/delete",
		"application/json", `{}`)
	if status != http.StatusOK {
		t.Fatalf("POST delete = %d，期望 200；Content-Type=%q body=%s", status, ctype, raw)
	}
	if ctype != "application/json; charset=utf-8" {
		t.Errorf("200 的 Content-Type = %q", ctype)
	}

	var reply map[string]json.RawMessage
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("200 响应不是合法 JSON: %v；%s", err, raw)
	}
	if len(reply) != 2 {
		t.Errorf("200 响应应当恰好有 id/deleted 两个字段，实际 %v", reply)
	}
	var id string
	var deleted bool
	if err := json.Unmarshal(reply["id"], &id); err != nil || id != target.ID {
		t.Errorf("响应 id = %q (err=%v)，期望 %q", id, err, target.ID)
	}
	if err := json.Unmarshal(reply["deleted"], &deleted); err != nil || !deleted {
		t.Errorf("响应 deleted = %v (err=%v)，期望 true", deleted, err)
	}

	t.Run("列表看不到", func(t *testing.T) {
		var list []conversationSummary
		getJSON(t, env.http.URL+"/api/conversations", http.StatusOK, &list)
		if len(list) != 1 || list[0].ID != kept.ID {
			t.Errorf("删除后列表 = %+v，期望只剩 %s", list, kept.ID)
		}
	})

	t.Run("详情 404", func(t *testing.T) {
		var errBody map[string]any
		getJSON(t, env.http.URL+"/api/conversations/"+target.ID, http.StatusNotFound, &errBody)
		message, _ := errBody["error"].(string)
		if !strings.Contains(message, "会话不存在") || !strings.Contains(message, target.ID) {
			t.Errorf("详情 404 的说明 = %q，期望点名会话不存在与 id", message)
		}
	})

	t.Run("消息 404", func(t *testing.T) {
		getJSON(t, env.http.URL+"/api/conversations/"+target.ID+"/messages", http.StatusNotFound, nil)
	})

	t.Run("用量 404", func(t *testing.T) {
		getJSON(t, env.http.URL+"/api/conversations/"+target.ID+"/usage", http.StatusNotFound, nil)
	})

	t.Run("另一场会话照常可读", func(t *testing.T) {
		var detail conversationDetail
		getJSON(t, env.http.URL+"/api/conversations/"+kept.ID, http.StatusOK, &detail)
		if detail.ID != kept.ID {
			t.Errorf("留下的会话详情 id = %q，期望 %q", detail.ID, kept.ID)
		}
		var page messagesResponse
		getJSON(t, env.http.URL+"/api/conversations/"+kept.ID+"/messages", http.StatusOK, &page)
		if len(page.Messages) != 1 {
			t.Errorf("留下的会话消息条数 = %d，期望 1", len(page.Messages))
		}
		var usage usageResponse
		getJSON(t, env.http.URL+"/api/conversations/"+kept.ID+"/usage", http.StatusOK, &usage)
		if len(usage.PerAgent) != 1 || usage.PerAgent[0].Turns != 1 {
			t.Errorf("留下的会话用量 = %+v，期望 c1 的一轮", usage.PerAgent)
		}
	})

	t.Run("store 里读不到", func(t *testing.T) {
		if _, err := env.store.Conversation(context.Background(), target.ID); !errors.Is(err, conversation.ErrConversationNotFound) {
			t.Errorf("删除后 store.Conversation 的 err = %v，期望 ErrConversationNotFound", err)
		}
	})

	// 再删一次：已删的 id 对外是 404，不是幂等 200。
	t.Run("已删的再删是 404", func(t *testing.T) {
		status, ctype, raw := postRaw(t,
			env.http.URL+"/api/conversations/"+target.ID+"/delete",
			"application/json", `{}`)
		if status != http.StatusNotFound {
			t.Fatalf("重复删除 = %d，期望 404；body=%s", status, raw)
		}
		if ctype != "application/json; charset=utf-8" {
			t.Errorf("404 的 Content-Type = %q", ctype)
		}
		var errBody map[string]any
		if err := json.Unmarshal(raw, &errBody); err != nil {
			t.Fatalf("404 响应不是合法 JSON: %v；%s", err, raw)
		}
		message, _ := errBody["error"].(string)
		if !strings.Contains(message, "会话不存在") {
			t.Errorf("404 的说明 = %q，期望说清会话不存在", message)
		}
	})

	// 同一路径只认 POST：GET 是 405 + Allow，不是 404。
	res, err := http.Get(env.http.URL + "/api/conversations/" + kept.ID + "/delete")
	if err != nil {
		t.Fatalf("GET delete 失败: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET delete = %d，期望 405", res.StatusCode)
	}
	if allow := res.Header.Get("Allow"); allow != http.MethodPost {
		t.Errorf("405 的 Allow = %q，期望 POST", allow)
	}
}

// TestGui3bDeleteUnknownConversation 钉住未知 id 404，且不会误删别的会话。
func TestGui3bDeleteUnknownConversation(t *testing.T) {
	env := newWriteTestEnv(t)
	kept := gui3bSeedConversation(t, env, "陪跑的会话")

	const unknown = "conv-does-not-exist"
	status, _, raw := postRaw(t,
		env.http.URL+"/api/conversations/"+unknown+"/delete",
		"application/json", `{}`)
	if status != http.StatusNotFound {
		t.Fatalf("未知会话 delete = %d，期望 404；body=%s", status, raw)
	}
	var errBody map[string]any
	if err := json.Unmarshal(raw, &errBody); err != nil {
		t.Fatalf("404 响应不是合法 JSON: %v；%s", err, raw)
	}
	message, _ := errBody["error"].(string)
	if !strings.Contains(message, unknown) {
		t.Errorf("404 的说明 = %q，期望点名 id %q", message, unknown)
	}

	var list []conversationSummary
	getJSON(t, env.http.URL+"/api/conversations", http.StatusOK, &list)
	if len(list) != 1 || list[0].ID != kept.ID {
		t.Errorf("未知 id 的 404 之后列表 = %+v，期望原来的会话还在", list)
	}
}

// TestGui3bDeleteReadOnlyReturns405 钉住共同要求：不带 -rw 时删除接口
// 同样被 s.write == nil 挡在门外，回 405 并告诉用户怎么开写模式。
func TestGui3bDeleteReadOnlyReturns405(t *testing.T) {
	env := newServeTestEnv(t, nil)
	status, ctype, raw := postRaw(t,
		env.http.URL+"/api/conversations/whatever/delete",
		"application/json", `{}`)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("只读模式 delete = %d，期望 405；body=%s", status, raw)
	}
	if ctype != "application/json; charset=utf-8" {
		t.Errorf("405 的 Content-Type = %q", ctype)
	}
	var reply map[string]any
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("405 响应不是合法 JSON: %v；%s", err, raw)
	}
	message, _ := reply["error"].(string)
	if !strings.Contains(message, "-rw") {
		t.Errorf("405 的说明 = %q，期望告诉用户用 council serve -rw", message)
	}
	if !strings.Contains(message, "只读") {
		t.Errorf("405 的说明 = %q，期望先说清当前是只读模式", message)
	}
}

// TestGui3bDeleteWriteGuard 复用 /api/ask 的同一套写闸：
// 非 JSON → 415，跨站 → 403；被拒的请求一条都不许删掉会话。
// 删除是比改名更重的写操作，任何网页用简单请求打本机端口就能删会话，
// 同样是 CSRF。
func TestGui3bDeleteWriteGuard(t *testing.T) {
	env := newWriteTestEnv(t)
	conv := gui3bSeedConversation(t, env, "守卫测试")
	deleteURL := env.http.URL + "/api/conversations/" + conv.ID + "/delete"

	assertStillThere := func(t *testing.T) {
		t.Helper()
		if _, err := env.store.Conversation(context.Background(), conv.ID); err != nil {
			t.Errorf("被拒绝的请求之后会话不见了: %v", err)
		}
	}

	t.Run("text/plain 415", func(t *testing.T) {
		status, _, raw := gui3bDeleteRequest(t, deleteURL, "text/plain", "", "", `{}`)
		if status != http.StatusUnsupportedMediaType {
			t.Fatalf("text/plain = %d，期望 415；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "application/json") {
			t.Errorf("415 的说明没有告诉用户要 application/json: %s", raw)
		}
		assertStillThere(t)
	})

	t.Run("无 Content-Type 415", func(t *testing.T) {
		status, _, raw := gui3bDeleteRequest(t, deleteURL, "", "", "", `{}`)
		if status != http.StatusUnsupportedMediaType {
			t.Fatalf("无 Content-Type = %d，期望 415；body=%s", status, raw)
		}
		assertStillThere(t)
	})

	t.Run("跨站 Origin 403", func(t *testing.T) {
		status, _, raw := gui3bDeleteRequest(t, deleteURL, "application/json", "https://evil.example", "", `{}`)
		if status != http.StatusForbidden {
			t.Fatalf("evil Origin = %d，期望 403；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "跨站") {
			t.Errorf("403 的说明没有提到跨站: %s", raw)
		}
		assertStillThere(t)
	})

	t.Run("跨站 Sec-Fetch-Site 403", func(t *testing.T) {
		status, _, raw := gui3bDeleteRequest(t, deleteURL, "application/json", "", "cross-site", `{}`)
		if status != http.StatusForbidden {
			t.Fatalf("cross-site = %d，期望 403；body=%s", status, raw)
		}
		assertStillThere(t)
	})

	t.Run("同源 Origin 200 且删除", func(t *testing.T) {
		status, _, raw := gui3bDeleteRequest(t, deleteURL, "application/json", env.http.URL, "", `{}`)
		if status != http.StatusOK {
			t.Fatalf("同源 Origin = %d，期望 200；body=%s", status, raw)
		}
		if _, err := env.store.Conversation(context.Background(), conv.ID); err == nil {
			t.Errorf("同源请求返回 200 之后会话仍然可见")
		}
	})

	// 这个端点没有参数：空 body 也必须能删。用一场新会话单独验证，
	// 避免上面那次成功删除把它变成 404。
	t.Run("空 body 也能删", func(t *testing.T) {
		emptyConv := gui3bSeedConversation(t, env, "空 body 目标")
		status, _, raw := postRaw(t,
			env.http.URL+"/api/conversations/"+emptyConv.ID+"/delete",
			"application/json", "")
		if status != http.StatusOK {
			t.Fatalf("空 body delete = %d，期望 200；body=%s", status, raw)
		}
	})
}

// TestGui3bIndexDeleteMarkup 钉 index.html 里的结构锚点。
// 真正的点击、取消、确认与刷新持久化由 CDP 验收覆盖；这里只保证改前端时
// 不会静默删掉确认框、它的两个按钮、默认焦点或只读模式的门。
func TestGui3bIndexDeleteMarkup(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		// 菜单项与它调用的入口。
		`remove.textContent = t('ui.conversation.delete', '删除')`,
		`openDeleteConfirm(conversation, more)`,
		// 只读模式不画会话菜单（与重命名同一个门）。
		`if (state.writable) appendConversationMenu(item, conversation)`,
		// 确认框的结构与文案。
		`id="delete-confirm"`,
		`id="delete-cancel"`,
		`id="delete-confirm-button"`,
		`deleteCancel.focus()`,
		`确定要删除`,
		// 删除成功后的列表移除与消息区清空。
		`/delete`,
		`forgetConversation(id)`,
		`会话已删除`,
		// 二次确认的三个退出点。
		`deleteOverlay.addEventListener`,
		`commitDelete`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}
	if strings.Contains(html, "innerHTML") {
		t.Errorf("index.html 里出现了 innerHTML，前端必须保持 0 处")
	}
}
