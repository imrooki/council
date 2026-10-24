package serve

// gui4_members_test.go 覆盖 GUI 路线第 4 片的后端：会话成员（participants
// 表）的加人与踢人。两个端点共用 /api/ask 的写闸，这里逐条钉住成功、
// 未知 agent 400、已删 / 未知会话 404、重复加入与重复退出的幂等、
// 退出后重新加入的原地激活、只读 405 与 CSRF 各分支。
//
// 前端点击（加人、移除确认框、刷新后仍在、只读无按钮）由 CDP 真驱动，
// 结果记录在交付报告；本文件末尾只钉 index.html 的结构锚点，防止
// 后续改动静默删掉按钮、确认框或只读门。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
)

// gui4SeedJoinedAt 是造数时固定下来的加入时间。它是一个远期过去值，
// 这样「重复加入不该刷新 joined_at」与「退出后重新加入会刷新」都能
// 用同一个参照物区分出来。
var gui4SeedJoinedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// gui4AddAgents 在既有写模式配置上补两个成员：
//   - c2 是普通配置成员，model = sonnet；
//   - cc 模拟从 Claude Code 导入并合并进 cfg.Agents 的成员（source=claude，
//     model = haiku）。serve.runWritable 在装配时做的就是这件事，测试里
//     直接写同样形状的内存配置，不启动文件扫描。
//
// 只往内存里加，不碰 TOML：本文件测的是 HTTP 端点的校验与写库语义，
// 配置加载已经由 config 包自己的用例覆盖。
func gui4AddAgents(t *testing.T, env *writeTestEnv) {
	t.Helper()
	cfg := env.srv.write.Cfg
	cfg.Agents["c2"] = config.AgentConfig{
		DisplayName: "C2",
		Provider:    "claude",
		Model:       "sonnet",
		Effort:      "medium",
		Permissions: "read",
		Mode:        "discussion",
	}
	cfg.Agents["cc"] = config.AgentConfig{
		DisplayName: "Claude Critic",
		Provider:    "claude",
		Model:       "haiku",
		Effort:      "low",
		Permissions: "read",
		Mode:        "discussion",
	}
	env.srv.models["c2"] = "sonnet"
	env.srv.models["cc"] = "haiku"
	if env.srv.write.ClaudeAgents == nil {
		env.srv.write.ClaudeAgents = make(map[string]config.AgentConfig)
	}
	env.srv.write.ClaudeAgents["cc"] = cfg.Agents["cc"]
	if env.srv.write.ClaudeAgentsMerged == nil {
		env.srv.write.ClaudeAgentsMerged = make(map[string]struct{})
	}
	env.srv.write.ClaudeAgentsMerged["cc"] = struct{}{}
}

// gui4SeedConversation 造一场带指定成员的会话，并追加一条来自首位成员
// 的发言。发言是为了验证「成员退出是软删除，说过的话仍读得到」：只造
// 空会话时，这条语义无论实现删不删消息都验不出来。
func gui4SeedConversation(t *testing.T, env *writeTestEnv, title string, agentIDs ...string) conversation.Conversation {
	t.Helper()
	var conv conversation.Conversation
	err := env.store.WriteTx(context.Background(), func(ctx context.Context, tx conversation.Tx) error {
		var err error
		conv, err = tx.CreateConversation(ctx, title)
		if err != nil {
			return err
		}
		for _, id := range agentIDs {
			ac := env.srv.write.Cfg.Agents[id]
			if err := tx.AddParticipant(ctx, conversation.Participant{
				ConvID:      conv.ID,
				AgentID:     id,
				DisplayName: ac.DisplayName,
				Provider:    ac.Provider,
				Active:      true,
				JoinedAt:    gui4SeedJoinedAt,
			}); err != nil {
				return err
			}
		}
		if len(agentIDs) > 0 {
			if _, err := tx.AppendMessage(ctx, conversation.Message{
				ConvID:   conv.ID,
				SenderID: agentIDs[0],
				Kind:     conversation.KindAgent,
				Text:     "成员管理之前的发言",
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("造成员测试会话 %q 失败: %v", title, err)
	}
	return conv
}

// gui4MemberRequest 带自定义头发一次成员写请求，供写闸用例使用。
func gui4MemberRequest(t *testing.T, url, contentType, origin, secFetchSite, body string) (int, string, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("构造成员请求失败: %v", err)
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
		t.Fatalf("读成员响应失败: %v", err)
	}
	return res.StatusCode, res.Header.Get("Content-Type"), raw
}

// gui4Parts 读会话的全部成员行（含已退出）。
func gui4Parts(t *testing.T, env *writeTestEnv, convID string) []conversation.Participant {
	t.Helper()
	parts, err := env.store.Participants(context.Background(), convID, true)
	if err != nil {
		t.Fatalf("读会话 %s 的成员失败: %v", convID, err)
	}
	return parts
}

// gui4Find 在成员行里找 id；找不到返回 nil。
func gui4Find(parts []conversation.Participant, agentID string) *conversation.Participant {
	for i := range parts {
		if parts[i].AgentID == agentID {
			return &parts[i]
		}
	}
	return nil
}

// gui4DecodeMembers 解出两个端点统一的响应形状，并顺带钉住顶层字段集。
func gui4DecodeMembers(t *testing.T, raw []byte) membersResponse {
	t.Helper()
	var gotMap map[string]any
	if err := json.Unmarshal(raw, &gotMap); err != nil {
		t.Fatalf("成员响应不是合法 JSON: %v；%s", err, raw)
	}
	requireKeys(t, "members response", gotMap, "conversation_id", "members")
	var got membersResponse
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("成员响应不是合法 JSON: %v；%s", err, raw)
	}
	// 成员项的形状逐字钉住：前端直接拿它重绘，多一个少一个字段都会
	// 让右栏的某一列悄悄变成空。
	rawMembers, _ := gotMap["members"].([]any)
	if len(rawMembers) > 0 {
		first, _ := rawMembers[0].(map[string]any)
		requireKeys(t, "member", first,
			"active", "agent_id", "display_name", "model", "provider")
	}
	return got
}

// TestGui4MemberAdd 钉住加人成功的完整形状：200、conversation_id 正确、
// 回表里能直接找到新成员（display_name / provider / model / active），
// 库里也只多这一行。另钉 Claude 导入的成员同样可加：生产装配会把它们
// 合并进 cfg.Agents，端点只认那张表。
func TestGui4MemberAdd(t *testing.T) {
	env := newWriteTestEnv(t)
	gui4AddAgents(t, env)
	conv := gui4SeedConversation(t, env, "加人成功", "c1")
	addURL := env.http.URL + "/api/conversations/" + conv.ID + "/members"

	status, ctype, raw := postRaw(t, addURL, "application/json", `{"agent_id":"c2"}`)
	if status != http.StatusOK {
		t.Fatalf("POST members = %d，期望 200；Content-Type=%q body=%s", status, ctype, raw)
	}
	if ctype != "application/json; charset=utf-8" {
		t.Errorf("200 的 Content-Type = %q", ctype)
	}
	got := gui4DecodeMembers(t, raw)
	if got.ConversationID != conv.ID {
		t.Errorf("conversation_id = %q，期望 %q", got.ConversationID, conv.ID)
	}
	if len(got.Members) != 2 {
		t.Fatalf("响应里成员数 = %d，期望 2（c1 与 c2）: %+v", len(got.Members), got.Members)
	}
	var c2 *memberJSON
	for i := range got.Members {
		if got.Members[i].AgentID == "c2" {
			c2 = &got.Members[i]
		}
	}
	if c2 == nil {
		t.Fatalf("响应里没有 c2: %+v", got.Members)
	}
	if c2.DisplayName != "C2" || c2.Provider != "claude" || c2.Model != "sonnet" || !c2.Active {
		t.Errorf("c2 = %+v，期望 C2/claude/sonnet/active", *c2)
	}

	parts := gui4Parts(t, env, conv.ID)
	if len(parts) != 2 {
		t.Fatalf("库里的成员行数 = %d，期望 2: %+v", len(parts), parts)
	}
	added := gui4Find(parts, "c2")
	if added == nil || !added.Active || added.LeftAt != nil {
		t.Errorf("库里的 c2 = %+v，期望活跃且没有 left_at", added)
	}

	// Claude 导入的成员走同一条路：写模式装配后它也只在 cfg.Agents 里。
	status, _, raw = postRaw(t, addURL, "application/json", `{"agent_id":"cc"}`)
	if status != http.StatusOK {
		t.Fatalf("加 Claude 导入成员 = %d，期望 200；body=%s", status, raw)
	}
	got = gui4DecodeMembers(t, raw)
	if len(got.Members) != 3 {
		t.Errorf("加 cc 之后成员数 = %d，期望 3", len(got.Members))
	}
	if p := gui4Find(gui4Parts(t, env, conv.ID), "cc"); p == nil || !p.Active {
		t.Errorf("库里的 cc = %+v，期望活跃成员", p)
	}

	// 详情读接口与成员写接口的形状必须一致：前端刷新页面后靠详情重绘。
	var detail conversationDetail
	getJSON(t, env.http.URL+"/api/conversations/"+conv.ID, http.StatusOK, &detail)
	for _, m := range detail.Members {
		if m.AgentID == "c2" && (m.DisplayName != "C2" || m.Model != "sonnet" || !m.Active) {
			t.Errorf("详情里的 c2 = %+v，期望与成员响应一致", m)
		}
	}
}

// TestGui4MemberAddRejectsUnknownAgent 钉住未知 agent 400：不写库、响应
// 点出是哪个 id。c9 不在配置里，也不是任何一场会话的历史成员。
func TestGui4MemberAddRejectsUnknownAgent(t *testing.T) {
	env := newWriteTestEnv(t)
	gui4AddAgents(t, env)
	conv := gui4SeedConversation(t, env, "未知成员", "c1")

	status, ctype, raw := postRaw(t,
		env.http.URL+"/api/conversations/"+conv.ID+"/members",
		"application/json", `{"agent_id":"c9"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("未知 agent = %d，期望 400；body=%s", status, raw)
	}
	if ctype != "application/json; charset=utf-8" {
		t.Errorf("400 的 Content-Type = %q", ctype)
	}
	if !strings.Contains(string(raw), "c9") {
		t.Errorf("400 的说明没有点名 c9: %s", raw)
	}
	if parts := gui4Parts(t, env, conv.ID); len(parts) != 1 {
		t.Errorf("被拒的请求之后成员行数 = %d，期望仍是 1: %+v", len(parts), parts)
	}

	// 空 agent_id 与坏 JSON、第二个 JSON 值都是 400，且同样不写库。
	for _, tc := range []struct {
		name string
		body string
	}{
		{"空 agent_id", `{"agent_id":"   "}`},
		{"坏 JSON", `{"agent_id":`},
		{"两个 JSON 值", `{"agent_id":"c2"}{"agent_id":"c2"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, raw := postRaw(t,
				env.http.URL+"/api/conversations/"+conv.ID+"/members",
				"application/json", tc.body)
			if status != http.StatusBadRequest {
				t.Errorf("%s = %d，期望 400；body=%s", tc.name, status, raw)
			}
			if parts := gui4Parts(t, env, conv.ID); len(parts) != 1 {
				t.Errorf("%s 之后成员行数 = %d，期望仍是 1", tc.name, len(parts))
			}
		})
	}
}

// TestGui4MemberAddDeletedConversation 钉住已软删的会话按不存在处理：
// 加人回 404，且不会往 participants 表里补一行“幽灵成员”。
func TestGui4MemberAddDeletedConversation(t *testing.T) {
	env := newWriteTestEnv(t)
	gui4AddAgents(t, env)
	conv := gui4SeedConversation(t, env, "要删掉的会话", "c1")
	kept := gui4SeedConversation(t, env, "留下的会话", "c1")

	if err := env.store.SoftDeleteConversation(context.Background(), conv.ID); err != nil {
		t.Fatalf("软删会话失败: %v", err)
	}
	status, _, raw := postRaw(t,
		env.http.URL+"/api/conversations/"+conv.ID+"/members",
		"application/json", `{"agent_id":"c2"}`)
	if status != http.StatusNotFound {
		t.Fatalf("已删会话加人 = %d，期望 404；body=%s", status, raw)
	}
	if !strings.Contains(string(raw), "会话不存在") {
		t.Errorf("404 的说明没有说清会话不存在: %s", raw)
	}
	// 已删会话的成员读入口本来就什么都看不到（visibleConv），不再拿
	// Participants 的 0 行去反推“没写库”；这里能钉住的是：404 之后
	// 会话仍然不可见，且陪跑的会话照常能加人。
	if _, err := env.store.Conversation(context.Background(), conv.ID); err == nil {
		t.Errorf("被拒的加人请求之后已删会话重新可见了")
	}

	// 未知会话 id 同样 404。
	status, _, raw = postRaw(t,
		env.http.URL+"/api/conversations/conv-no-such-id/members",
		"application/json", `{"agent_id":"c2"}`)
	if status != http.StatusNotFound {
		t.Fatalf("未知会话加人 = %d，期望 404；body=%s", status, raw)
	}

	// 陪跑的会话不受影响，仍然可以加人。
	status, _, raw = postRaw(t,
		env.http.URL+"/api/conversations/"+kept.ID+"/members",
		"application/json", `{"agent_id":"c2"}`)
	if status != http.StatusOK {
		t.Fatalf("留下的会话加人 = %d，期望 200；body=%s", status, raw)
	}
}

// TestGui4MemberAddIdempotent 钉住「已是活跃成员 → 200 且什么都不写」：
// 不插第二行、不改 joined_at。后者是幂等与“能跑通”的分界线——
// AddParticipant 的 ON CONFLICT 会刷新 joined_at，直接无条件调用会把
// 一次重复点击变成一次加入时间改写。
func TestGui4MemberAddIdempotent(t *testing.T) {
	env := newWriteTestEnv(t)
	gui4AddAgents(t, env)
	conv := gui4SeedConversation(t, env, "重复加人", "c1")
	before := gui4Find(gui4Parts(t, env, conv.ID), "c1")
	if before == nil {
		t.Fatal("造数后找不到 c1")
	}
	joinedBefore := before.JoinedAt

	for i := 0; i < 2; i++ {
		status, _, raw := postRaw(t,
			env.http.URL+"/api/conversations/"+conv.ID+"/members",
			"application/json", `{"agent_id":"c1"}`)
		if status != http.StatusOK {
			t.Fatalf("第 %d 次重复加人 = %d，期望 200；body=%s", i+1, status, raw)
		}
	}
	parts := gui4Parts(t, env, conv.ID)
	if len(parts) != 1 {
		t.Fatalf("重复加人之后成员行数 = %d，期望仍是 1: %+v", len(parts), parts)
	}
	if !parts[0].JoinedAt.Equal(joinedBefore) {
		t.Errorf("重复加人之后 joined_at = %v，期望保持 %v", parts[0].JoinedAt, joinedBefore)
	}
}

// TestGui4MemberAddReactivates 钉住「退出过的人再加回来 = 原地重新激活」：
// 还是同一行（不插第二行），active 回 1，left_at 清空，joined_at 记这一次
// 重新加入的时间。
func TestGui4MemberAddReactivates(t *testing.T) {
	env := newWriteTestEnv(t)
	gui4AddAgents(t, env)
	conv := gui4SeedConversation(t, env, "退出后再加入", "c1")

	err := env.store.WriteTx(context.Background(), func(ctx context.Context, tx conversation.Tx) error {
		return tx.DeactivateParticipant(ctx, conv.ID, "c1", time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC))
	})
	if err != nil {
		t.Fatalf("先让 c1 退出失败: %v", err)
	}
	if before := gui4Find(gui4Parts(t, env, conv.ID), "c1"); before == nil || before.Active {
		t.Fatalf("退出之后 c1 = %+v，期望 inactive", before)
	}

	status, _, raw := postRaw(t,
		env.http.URL+"/api/conversations/"+conv.ID+"/members",
		"application/json", `{"agent_id":"c1"}`)
	if status != http.StatusOK {
		t.Fatalf("退出后再加入 = %d，期望 200；body=%s", status, raw)
	}
	parts := gui4Parts(t, env, conv.ID)
	if len(parts) != 1 {
		t.Fatalf("重新加入之后成员行数 = %d，期望仍是同一行 1: %+v", len(parts), parts)
	}
	got := parts[0]
	if !got.Active {
		t.Errorf("重新加入之后 active = false，期望 true")
	}
	if got.LeftAt != nil {
		t.Errorf("重新加入之后 left_at = %v，期望清空", got.LeftAt)
	}
	if !got.JoinedAt.After(gui4SeedJoinedAt) {
		t.Errorf("重新加入之后 joined_at = %v，期望晚于造数时的 %v", got.JoinedAt, gui4SeedJoinedAt)
	}
	// 响应也必须是改完后的状态，前端拿它直接重绘。
	resp := gui4DecodeMembers(t, raw)
	if len(resp.Members) != 1 || !resp.Members[0].Active {
		t.Errorf("响应里的成员 = %+v，期望 c1 active", resp.Members)
	}
}

// TestGui4MemberRemove 钉住踢人的软退出：active=0、left_at 有值、消息
// 一条不少；再踢一次 200 且 left_at 不被刷新；未知成员 404；已删会话 404。
func TestGui4MemberRemove(t *testing.T) {
	env := newWriteTestEnv(t)
	gui4AddAgents(t, env)
	conv := gui4SeedConversation(t, env, "踢人", "c1", "c2")
	removeURL := env.http.URL + "/api/conversations/" + conv.ID + "/members/c2/remove"

	status, ctype, raw := postRaw(t, removeURL, "application/json", `{}`)
	if status != http.StatusOK {
		t.Fatalf("POST remove = %d，期望 200；Content-Type=%q body=%s", status, ctype, raw)
	}
	if ctype != "application/json; charset=utf-8" {
		t.Errorf("200 的 Content-Type = %q", ctype)
	}
	resp := gui4DecodeMembers(t, raw)
	if resp.ConversationID != conv.ID {
		t.Errorf("conversation_id = %q，期望 %q", resp.ConversationID, conv.ID)
	}
	if len(resp.Members) != 2 {
		t.Fatalf("移除之后响应成员数 = %d，期望 2（含已退出的行）: %+v", len(resp.Members), resp.Members)
	}
	for _, m := range resp.Members {
		if m.AgentID == "c2" && (m.Active || m.Model != "sonnet") {
			t.Errorf("响应里的 c2 = %+v，期望已退出且 model 仍是 sonnet", m)
		}
	}

	parts := gui4Parts(t, env, conv.ID)
	removed := gui4Find(parts, "c2")
	if removed == nil {
		t.Fatal("移除之后 participants 里找不到 c2（软删除不能删行）")
	}
	if removed.Active {
		t.Errorf("移除之后 c2 active = true，期望 false")
	}
	if removed.LeftAt == nil {
		t.Errorf("移除之后 c2 的 left_at 为空，期望记下退出时间")
	}
	leftAt := *removed.LeftAt

	// 消息仍读得到：软删除只动 participants，不动 messages。
	var page messagesResponse
	getJSON(t, env.http.URL+"/api/conversations/"+conv.ID+"/messages", http.StatusOK, &page)
	if len(page.Messages) != 1 || page.Messages[0].Text != "成员管理之前的发言" {
		t.Errorf("移除成员之后消息 = %+v，期望原样保留那一条", page.Messages)
	}

	// 再踢一次：200 幂等，left_at 不刷新。
	time.Sleep(2 * time.Millisecond)
	status, _, raw = postRaw(t, removeURL, "application/json", `{}`)
	if status != http.StatusOK {
		t.Fatalf("重复移除 = %d，期望 200；body=%s", status, raw)
	}
	again := gui4Find(gui4Parts(t, env, conv.ID), "c2")
	if again == nil || again.LeftAt == nil || !again.LeftAt.Equal(leftAt) {
		t.Errorf("重复移除之后 c2 = %+v，期望 left_at 保持 %v", again, leftAt)
	}

	// 从未加入过的 id：404，不能静默成功。
	status, _, raw = postRaw(t,
		env.http.URL+"/api/conversations/"+conv.ID+"/members/c9/remove",
		"application/json", `{}`)
	if status != http.StatusNotFound {
		t.Errorf("未知成员移除 = %d，期望 404；body=%s", status, raw)
	}
	if !strings.Contains(string(raw), "c9") {
		t.Errorf("未知成员 404 的说明没有点名 c9: %s", raw)
	}

	// 已删会话：404。
	if err := env.store.SoftDeleteConversation(context.Background(), conv.ID); err != nil {
		t.Fatalf("软删会话失败: %v", err)
	}
	status, _, raw = postRaw(t, removeURL, "application/json", `{}`)
	if status != http.StatusNotFound {
		t.Errorf("已删会话移除成员 = %d，期望 404；body=%s", status, raw)
	}
}

// TestGui4MembersReadOnly405 钉住共同要求：不带 -rw 时两个成员写端点
// 都回 405，并在说明里告诉用户怎么开写模式。
func TestGui4MembersReadOnly405(t *testing.T) {
	env := newServeTestEnv(t, nil)
	for _, path := range []string{
		"/api/conversations/whatever/members",
		"/api/conversations/whatever/members/c1/remove",
	} {
		status, ctype, raw := postRaw(t, env.http.URL+path, "application/json", `{"agent_id":"c1"}`)
		if status != http.StatusMethodNotAllowed {
			t.Errorf("只读模式 POST %s = %d，期望 405；body=%s", path, status, raw)
			continue
		}
		if ctype != "application/json; charset=utf-8" {
			t.Errorf("只读模式 %s 的 Content-Type = %q", path, ctype)
		}
		message := string(raw)
		if !strings.Contains(message, "-rw") || !strings.Contains(message, "只读") {
			t.Errorf("只读模式 %s 的说明 = %q，期望点明只读与 -rw", path, message)
		}
	}
}

// TestGui4MembersWriteGuard 复用 /api/ask 的同一套写闸：非 JSON → 415，
// 跨站 → 403；被拒的请求一条都不许改 participants。同源与无 Origin
// 的本机请求放行。
func TestGui4MembersWriteGuard(t *testing.T) {
	env := newWriteTestEnv(t)
	gui4AddAgents(t, env)
	conv := gui4SeedConversation(t, env, "守卫测试", "c1")
	addURL := env.http.URL + "/api/conversations/" + conv.ID + "/members"
	removeURL := env.http.URL + "/api/conversations/" + conv.ID + "/members/c2/remove"

	assertOnlyC1 := func(t *testing.T) {
		t.Helper()
		parts := gui4Parts(t, env, conv.ID)
		if len(parts) != 1 || parts[0].AgentID != "c1" || !parts[0].Active {
			t.Errorf("被拒的请求之后成员 = %+v，期望只有活跃的 c1", parts)
		}
	}

	addRejected := []struct {
		name         string
		contentType  string
		origin       string
		secFetchSite string
	}{
		{"text/plain 415", "text/plain", "", ""},
		{"无 Content-Type 415", "", "", ""},
		{"跨站 Origin 403", "application/json", "https://evil.example", ""},
		{"跨站 Sec-Fetch-Site 403", "application/json", "", "cross-site"},
	}
	for _, tc := range addRejected {
		t.Run("加人 "+tc.name, func(t *testing.T) {
			status, _, raw := gui4MemberRequest(t, addURL, tc.contentType, tc.origin, tc.secFetchSite, `{"agent_id":"c2"}`)
			want := http.StatusForbidden
			if strings.Contains(tc.name, "415") {
				want = http.StatusUnsupportedMediaType
			}
			if status != want {
				t.Fatalf("%s = %d，期望 %d；body=%s", tc.name, status, want, raw)
			}
			if want == http.StatusForbidden && !strings.Contains(string(raw), "跨站") {
				t.Errorf("403 的说明没有提到跨站: %s", raw)
			}
			if want == http.StatusUnsupportedMediaType && !strings.Contains(string(raw), "application/json") {
				t.Errorf("415 的说明没有告诉用户要 application/json: %s", raw)
			}
			assertOnlyC1(t)
		})
	}

	// 同源请求放行。
	status, _, raw := gui4MemberRequest(t, addURL, "application/json", env.http.URL, "", `{"agent_id":"c2"}`)
	if status != http.StatusOK {
		t.Fatalf("同源 Origin 加人 = %d，期望 200；body=%s", status, raw)
	}
	if c2 := gui4Find(gui4Parts(t, env, conv.ID), "c2"); c2 == nil || !c2.Active {
		t.Fatalf("同源请求之后 c2 = %+v，期望已加入", c2)
	}

	// 踢人的闸：拒绝时 c2 仍然活跃。
	for _, tc := range []struct {
		name         string
		contentType  string
		origin       string
		secFetchSite string
	}{
		{"text/plain 415", "text/plain", "", ""},
		{"无 Content-Type 415", "", "", ""},
		{"跨站 Origin 403", "application/json", "https://evil.example", ""},
		{"跨站 Sec-Fetch-Site 403", "application/json", "", "cross-site"},
	} {
		t.Run("踢人 "+tc.name, func(t *testing.T) {
			status, _, raw := gui4MemberRequest(t, removeURL, tc.contentType, tc.origin, tc.secFetchSite, `{}`)
			if status != http.StatusForbidden && status != http.StatusUnsupportedMediaType {
				t.Fatalf("%s = %d，期望 403/415；body=%s", tc.name, status, raw)
			}
			if c2 := gui4Find(gui4Parts(t, env, conv.ID), "c2"); c2 == nil || !c2.Active {
				t.Errorf("被拒的踢人请求之后 c2 = %+v，期望仍然活跃", c2)
			}
		})
	}

	// 无 Origin 的本机脚本请求与同源请求都放行；两次之间重新加回来，
	// 让两次移除都有真实的活跃成员可退。
	status, _, raw = gui4MemberRequest(t, removeURL, "application/json", "", "", `{}`)
	if status != http.StatusOK {
		t.Fatalf("无 Origin 踢人 = %d，期望 200；body=%s", status, raw)
	}
	if c2 := gui4Find(gui4Parts(t, env, conv.ID), "c2"); c2 == nil || c2.Active {
		t.Errorf("无 Origin 请求之后 c2 = %+v，期望已退出", c2)
	}
	status, _, raw = gui4MemberRequest(t, addURL, "application/json", "", "", `{"agent_id":"c2"}`)
	if status != http.StatusOK {
		t.Fatalf("重新加入 c2 = %d，期望 200；body=%s", status, raw)
	}
	status, _, raw = gui4MemberRequest(t, removeURL, "application/json", env.http.URL, "", `{}`)
	if status != http.StatusOK {
		t.Fatalf("同源 Origin 踢人 = %d，期望 200；body=%s", status, raw)
	}
	if c2 := gui4Find(gui4Parts(t, env, conv.ID), "c2"); c2 == nil || c2.Active {
		t.Errorf("同源请求之后 c2 = %+v，期望已退出", c2)
	}
}

// TestGui4MembersMethodNotAllowed 钉住写模式下非 POST 是 405 + Allow，
// 而不是被路由兜底成 404：路径存在、只是方法不对。
func TestGui4MembersMethodNotAllowed(t *testing.T) {
	env := newWriteTestEnv(t)
	gui4AddAgents(t, env)
	conv := gui4SeedConversation(t, env, "方法", "c1")
	for _, path := range []string{
		"/api/conversations/" + conv.ID + "/members",
		"/api/conversations/" + conv.ID + "/members/c1/remove",
	} {
		res, err := http.Get(env.http.URL + path)
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

// TestGui4MemberRemovedMessageStillReadable 是消息保留的另一个入口：
// 通过 Service.Leave 之外的 HTTP 端点移除之后，/messages 仍然按原 seq
// 读得到已退出成员说过的那一条。上面的 Remove 用例已经顺带查过一次；
// 这里单独成条，失败时能直接看出是“消息没了”而不是“active 没改”。
func TestGui4MemberRemovedMessageStillReadable(t *testing.T) {
	env := newWriteTestEnv(t)
	gui4AddAgents(t, env)
	conv := gui4SeedConversation(t, env, "消息保留", "c1")

	status, _, raw := postRaw(t,
		env.http.URL+"/api/conversations/"+conv.ID+"/members/c1/remove",
		"application/json", `{}`)
	if status != http.StatusOK {
		t.Fatalf("移除 c1 = %d，期望 200；body=%s", status, raw)
	}
	var page messagesResponse
	getJSON(t, env.http.URL+"/api/conversations/"+conv.ID+"/messages", http.StatusOK, &page)
	if len(page.Messages) != 1 || page.Messages[0].SenderID != "c1" {
		t.Errorf("移除之后消息 = %+v，期望 c1 的那一条仍在", page.Messages)
	}
	if _, err := env.store.Conversation(context.Background(), conv.ID); err != nil {
		t.Errorf("移除成员不应影响会话本身: %v", err)
	}
}

// TestGui4IndexMembersMarkup 钉 index.html 里的结构锚点。真正的点击、
// 刷新持久化与只读态由 CDP 验收覆盖；这里只保证改前端时不会静默删掉
// 添加按钮、成员操作列、确认框或只读门。
func TestGui4IndexMembersMarkup(t *testing.T) {
	html := string(indexHTML)
	for _, want := range []string{
		// 右栏顶部的加人入口与它用的接口。
		`id="member-add"`,
		`+ 添加成员`,
		`/members`,
		`{ agent_id: agentID }`,
		// 每行的移除 / 重新加入按钮与二次确认。
		`member-remove`,
		`重新加入`,
		`id="member-remove-confirm"`,
		`id="member-remove-cancel"`,
		`id="member-remove-confirm-button"`,
		`memberRemoveCancel.focus()`,
		// 只读模式不画这些按钮。
		`state.writable`,
		`memberAdd.hidden`,
		// 关闭确认框的三个退出点。
		`memberRemoveOverlay.addEventListener`,
		`commitMemberRemove`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html 里缺少 %q", want)
		}
	}
	if strings.Contains(html, "innerHTML") {
		t.Errorf("index.html 里出现了 innerHTML，前端必须保持 0 处")
	}
}
