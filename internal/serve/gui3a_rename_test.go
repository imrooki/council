package serve

// gui3a_rename_test.go 覆盖 GUI 路线第 3 片前半（3a）的后端一半：
// POST /api/conversations/{id}/rename 的六条路 —— 成功、空标题 400、
// 超长 400、未知 id 404、只读 405、非 JSON 415 —— 以及它与 /api/ask
// 共用的同源闸（403）。
//
// 前端的列表 / 搜索 / 内联输入框行为不在这里测：那部分依赖真实 DOM 与
// 真实点击，由 CDP 真驱动一次的验收覆盖（见交付报告）。本文件末尾只钉
// index.html 里的结构锚点，避免改着改着把搜索框或菜单删掉却没人发现。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"council/internal/conversation"
)

// gui3aSeedConversation 在写模式的真库里建一场会话，作为改名的目标。
// 不经过 /api/ask：改名根本不跑 provider，用一次假议会来造数只会让
// 这条用例的失败面变大。
func gui3aSeedConversation(t *testing.T, env *writeTestEnv, title string) conversation.Conversation {
	t.Helper()
	var conv conversation.Conversation
	err := env.store.WriteTx(context.Background(), func(ctx context.Context, tx conversation.Tx) error {
		var err error
		conv, err = tx.CreateConversation(ctx, title)
		return err
	})
	if err != nil {
		t.Fatalf("建测试会话 %q 失败: %v", title, err)
	}
	return conv
}

// gui3aRenameRequest 带自定义头发一次改名请求，供写闸的用例使用。
func gui3aRenameRequest(t *testing.T, url, contentType, origin, secFetchSite, body string) (int, string, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("构造改名请求失败: %v", err)
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
		t.Fatalf("读改名响应失败: %v", err)
	}
	return res.StatusCode, res.Header.Get("Content-Type"), raw
}

// TestGui3aRenameConversation 钉住成功路径的完整形状：
// 200 + {"id","title"}、标题去空白、库与两个读接口都看到新标题。
func TestGui3aRenameConversation(t *testing.T) {
	env := newWriteTestEnv(t)
	conv := gui3aSeedConversation(t, env, "旧标题")

	status, ctype, raw := postRaw(t,
		env.http.URL+"/api/conversations/"+conv.ID+"/rename",
		"application/json", `{"title":"  新的标题  "}`)
	if status != http.StatusOK {
		t.Fatalf("POST rename = %d，期望 200；Content-Type=%q body=%s", status, ctype, raw)
	}
	if ctype != "application/json; charset=utf-8" {
		t.Errorf("200 的 Content-Type = %q", ctype)
	}

	var reply map[string]json.RawMessage
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("200 响应不是合法 JSON: %v；%s", err, raw)
	}
	if len(reply) != 2 {
		t.Errorf("200 响应应当恰好有 id/title 两个字段，实际 %v", reply)
	}
	var id, title string
	if err := json.Unmarshal(reply["id"], &id); err != nil || id != conv.ID {
		t.Errorf("响应 id = %q (err=%v)，期望 %q", id, err, conv.ID)
	}
	if err := json.Unmarshal(reply["title"], &title); err != nil || title != "新的标题" {
		t.Errorf("响应 title = %q (err=%v)，期望去空白之后的「新的标题」", title, err)
	}

	got, err := env.store.Conversation(context.Background(), conv.ID)
	if err != nil {
		t.Fatalf("改名后读会话失败: %v", err)
	}
	if got.Title != "新的标题" {
		t.Errorf("库里的标题 = %q，期望「新的标题」", got.Title)
	}

	// 列表与详情是前端读取标题的两条真实路径，两条都要同步。
	var list []conversationSummary
	getJSON(t, env.http.URL+"/api/conversations", http.StatusOK, &list)
	if len(list) != 1 || list[0].Title != "新的标题" {
		t.Errorf("GET /api/conversations = %+v，期望唯一一场的标题是「新的标题」", list)
	}
	var detail conversationDetail
	getJSON(t, env.http.URL+"/api/conversations/"+conv.ID, http.StatusOK, &detail)
	if detail.Title != "新的标题" {
		t.Errorf("GET /api/conversations/%s 的 title = %q，期望「新的标题」", conv.ID, detail.Title)
	}

	// 同一路径只认 POST：写模式下的 GET 是 405，不是 404。
	res, err := http.Get(env.http.URL + "/api/conversations/" + conv.ID + "/rename")
	if err != nil {
		t.Fatalf("GET rename 失败: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET rename = %d，期望 405", res.StatusCode)
	}
	if allow := res.Header.Get("Allow"); allow != http.MethodPost {
		t.Errorf("405 的 Allow = %q，期望 POST", allow)
	}
}

// TestGui3aRenameRejectsInvalidTitle 覆盖空标题与超长两条 400，
// 并确认失败的请求不会把旧标题改掉一半。
func TestGui3aRenameRejectsInvalidTitle(t *testing.T) {
	env := newWriteTestEnv(t)
	conv := gui3aSeedConversation(t, env, "原来的标题")
	renameURL := env.http.URL + "/api/conversations/" + conv.ID + "/rename"

	t.Run("空标题", func(t *testing.T) {
		for _, body := range []string{
			`{"title":""}`,
			`{"title":"   \n\t "}`,
			`{}`,
		} {
			status, ctype, raw := postRaw(t, renameURL, "application/json", body)
			if status != http.StatusBadRequest {
				t.Fatalf("body=%s 的 rename = %d，期望 400；body=%s", body, status, raw)
			}
			if ctype != "application/json; charset=utf-8" {
				t.Errorf("400 的 Content-Type = %q", ctype)
			}
			var reply map[string]any
			if err := json.Unmarshal(raw, &reply); err != nil {
				t.Fatalf("400 响应不是合法 JSON: %v；%s", err, raw)
			}
			message, _ := reply["error"].(string)
			if !strings.Contains(message, "标题不能为空") {
				t.Errorf("空标题的 400 = %q，期望说清「标题不能为空」", message)
			}
			if !strings.Contains(message, "120") {
				t.Errorf("空标题的 400 = %q，期望同时给出 1–120 的范围", message)
			}
		}
	})

	t.Run("超长", func(t *testing.T) {
		// 按字符（rune）计：121 个汉字是 363 字节，若实现按字节算，
		// 这条会先撞上长度限制而给出一个错误的计数。
		long := strings.Repeat("寿", maxConversationTitle+1)
		body, err := json.Marshal(map[string]any{"title": long})
		if err != nil {
			t.Fatalf("编码超长标题失败: %v", err)
		}
		status, _, raw := postRaw(t, renameURL, "application/json", string(body))
		if status != http.StatusBadRequest {
			t.Fatalf("121 个汉字 = %d，期望 400；body=%s", status, raw)
		}
		var reply map[string]any
		if err := json.Unmarshal(raw, &reply); err != nil {
			t.Fatalf("400 响应不是合法 JSON: %v；%s", err, raw)
		}
		message, _ := reply["error"].(string)
		if !strings.Contains(message, "最长 120") {
			t.Errorf("超长的 400 = %q，期望点名「最长 120」", message)
		}
		if !strings.Contains(message, "121") {
			t.Errorf("超长的 400 = %q，期望同时报出实际字符数 121", message)
		}
	})

	t.Run("120 个汉字可以", func(t *testing.T) {
		// 反方向上界：恰好 120 个多字节字符必须被接受，证明上限是字符数
		// 而不是字节数（360 字节 > 120，按字节算会误拒）。
		exact := strings.Repeat("寿", maxConversationTitle)
		body, err := json.Marshal(map[string]any{"title": exact})
		if err != nil {
			t.Fatalf("编码边界标题失败: %v", err)
		}
		status, _, raw := postRaw(t, renameURL, "application/json", string(body))
		if status != http.StatusOK {
			t.Fatalf("120 个汉字 = %d，期望 200；body=%s", status, raw)
		}
		got, err := env.store.Conversation(context.Background(), conv.ID)
		if err != nil {
			t.Fatalf("读会话失败: %v", err)
		}
		if got.Title != exact {
			t.Errorf("库里的标题长度 = %d 个字符，期望 120", len([]rune(got.Title)))
		}
	})

	// 三次被拒 / 一次成功都没有留下半截状态：最终标题要么是原来的，
	// 要么是最后那次合法的。这里最后一次合法，正好证明前面几次没生效。
	got, err := env.store.Conversation(context.Background(), conv.ID)
	if err != nil {
		t.Fatalf("读最终会话失败: %v", err)
	}
	if len([]rune(got.Title)) != maxConversationTitle {
		t.Errorf("最终标题 = %q，期望最后一次合法的 120 字符标题", got.Title)
	}
}

// TestGui3aRenameUnknownConversation 钉住 404：URL 指向的会话不存在时
// 不能回 200，也不能把错误说成 400（那是请求格式问题，这是资源不存在）。
func TestGui3aRenameUnknownConversation(t *testing.T) {
	env := newWriteTestEnv(t)
	gui3aSeedConversation(t, env, "陪跑的会话")

	const unknown = "conv-does-not-exist"
	status, _, raw := postRaw(t,
		env.http.URL+"/api/conversations/"+unknown+"/rename",
		"application/json", `{"title":"改成什么都行"}`)
	if status != http.StatusNotFound {
		t.Fatalf("未知会话 rename = %d，期望 404；body=%s", status, raw)
	}
	var reply map[string]any
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("404 响应不是合法 JSON: %v；%s", err, raw)
	}
	message, _ := reply["error"].(string)
	if !strings.Contains(message, "会话不存在") {
		t.Errorf("404 的说明 = %q，期望说清「会话不存在」", message)
	}
	if !strings.Contains(message, unknown) {
		t.Errorf("404 的说明 = %q，期望点名是哪个 id", message)
	}

	var list []conversationSummary
	getJSON(t, env.http.URL+"/api/conversations", http.StatusOK, &list)
	if len(list) != 1 || list[0].Title != "陪跑的会话" {
		t.Errorf("404 之后列表 = %+v，期望只有原来那一场且标题不变", list)
	}
}

// TestGui3aRenameReadOnlyReturns405 钉住第 3 片的共同要求：不带 -rw 时
// 新增写接口同样被 s.write == nil 挡在门外，回 405 并告诉用户怎么开写模式。
func TestGui3aRenameReadOnlyReturns405(t *testing.T) {
	env := newServeTestEnv(t, nil)
	status, ctype, raw := postRaw(t,
		env.http.URL+"/api/conversations/whatever/rename",
		"application/json", `{"title":"只读也想改"}`)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("只读模式 rename = %d，期望 405；body=%s", status, raw)
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

// TestGui3aRenameWriteGuard 复用 /api/ask 的同一套写闸：
// 非 JSON → 415，跨站 → 403，同源 / 无 Origin → 放行。
// 改名是比跑议会便宜得多的写操作，但它仍然能改掉用户看到的事实；
// 任何网页用简单请求打本机端口就能改标题，同样是 CSRF。
func TestGui3aRenameWriteGuard(t *testing.T) {
	env := newWriteTestEnv(t)
	conv := gui3aSeedConversation(t, env, "守卫测试")
	renameURL := env.http.URL + "/api/conversations/" + conv.ID + "/rename"
	const validBody = `{"title":"闸门放行的标题"}`

	t.Run("text/plain 415", func(t *testing.T) {
		status, _, raw := gui3aRenameRequest(t, renameURL, "text/plain", "", "", validBody)
		if status != http.StatusUnsupportedMediaType {
			t.Fatalf("text/plain = %d，期望 415；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "application/json") {
			t.Errorf("415 的说明没有告诉用户要 application/json: %s", raw)
		}
	})

	t.Run("无 Content-Type 415", func(t *testing.T) {
		status, _, raw := gui3aRenameRequest(t, renameURL, "", "", "", validBody)
		if status != http.StatusUnsupportedMediaType {
			t.Fatalf("无 Content-Type = %d，期望 415；body=%s", status, raw)
		}
	})

	t.Run("跨站 Origin 403", func(t *testing.T) {
		status, _, raw := gui3aRenameRequest(t, renameURL, "application/json", "https://evil.example", "", validBody)
		if status != http.StatusForbidden {
			t.Fatalf("evil Origin = %d，期望 403；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "跨站") {
			t.Errorf("403 的说明没有提到跨站: %s", raw)
		}
	})

	t.Run("跨站 Sec-Fetch-Site 403", func(t *testing.T) {
		status, _, raw := gui3aRenameRequest(t, renameURL, "application/json", "", "cross-site", validBody)
		if status != http.StatusForbidden {
			t.Fatalf("cross-site = %d，期望 403；body=%s", status, raw)
		}
	})

	t.Run("坏 JSON 400", func(t *testing.T) {
		status, _, raw := postRaw(t, renameURL, "application/json", `{"title":`)
		if status != http.StatusBadRequest {
			t.Fatalf("坏 JSON = %d，期望 400；body=%s", status, raw)
		}
		if !strings.Contains(string(raw), "JSON") {
			t.Errorf("400 的说明没有提到 JSON: %s", raw)
		}
	})

	t.Run("同源 Origin 200", func(t *testing.T) {
		status, _, raw := gui3aRenameRequest(t, renameURL, "application/json", env.http.URL, "", validBody)
		if status != http.StatusOK {
			t.Fatalf("同源 Origin = %d，期望 200；body=%s", status, raw)
		}
		got, err := env.store.Conversation(context.Background(), conv.ID)
		if err != nil {
			t.Fatalf("读会话失败: %v", err)
		}
		if got.Title != "闸门放行的标题" {
			t.Errorf("同源请求之后标题 = %q，期望「闸门放行的标题」", got.Title)
		}
	})
}

// TestGui3aIndexRenameSearchMarkup 钉 index.html 里的结构锚点。
// 真正的点击 / 输入 / 刷新持久化由 CDP 验收覆盖；这里只保证改前端时
// 不会静默删掉搜索框、计数、菜单或那两个键盘键。
func TestGui3aIndexRenameSearchMarkup(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		`id="conversation-search"`,
		`id="conversation-count"`,
		`placeholder="搜索标题"`,
		// 计数是 N / M，两个数字都在渲染时算，不写死。
		`visible.length + ' / ' + total`,
		// 过滤是纯前端、大小写不敏感。
		`toLowerCase()`,
		`没有匹配的会话`,
		// 每项的 ⋯ 菜单与重命名入口。
		`conv-more`,
		`重命名`,
		`/rename`,
		// 回车确认、Esc 取消。
		`event.key === 'Enter'`,
		`event.key === 'Escape'`,
		// 改名成功后标题栏（会话头）同步，列表重画。
		`messagesTitle.textContent = result.title`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}
}
