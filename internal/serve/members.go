package serve

// members.go 是 GUI 路线第 4 片的后端：会话成员（participants 表）的
// 加人与踢人。它与 /api/ask 的 members 字段不是一回事——那个是「这一轮
// 谁发言」的临时名单，本文件改的是持久成员表：刷新页面、换一轮、明天
// 再打开，改动都还在。
//
// 两个端点都只做三件事：过写闸、把领域动作交给 conversation.Service
// （Join / Leave，内部走已有的 AddParticipant / DeactivateParticipant）、
// 把改完之后的完整成员表回给前端。本文件不新开 store 方法，也不碰
// participants 的 SQL：退出是软删除、重新加入是原地激活，这两条语义
// 已经由 store 的实现钉住，serve 层再写一遍只会多一个会漂的事实源。

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"council/internal/conversation"
)

// maxMemberBody 是加人请求体的上限。body 里只有一个 agent id，即使 id
// 全是 4 字节 rune 也远小于 1 KiB；4 KiB 足够容纳 JSON 转义，超过它的
// 请求体不是一个成员 id，没必要读完再拒。
const maxMemberBody = 4 << 10

// memberAddRequest 是 POST /api/conversations/{id}/members 的请求体。
// 多传的字段按 Go 的 JSON 解码规则忽略，与 /api/ask、rename 一致。
type memberAddRequest struct {
	AgentID string `json:"agent_id"`
}

// membersResponse 是加人 / 踢人两个端点的统一响应，也是前端重绘右栏
// 的唯一数据来源：conversation_id 让前端确认这份表属于当前选中的会话
// （请求在飞时用户可能已经切走），members 是改完之后的完整成员表（含
// 已退出的行，active 字段区分）。
type membersResponse struct {
	ConversationID string       `json:"conversation_id"`
	Members        []memberJSON `json:"members"`
}

// memberFor 把 participant 行转成 API 形状。model 不在 participants 表里
// （join 时只快照 display_name / provider），只能从配置取的模型名补上；
// 配置里已经删掉的 id 回空串，前端会退回用量表里的旧模型名。
func (s *Server) memberFor(p conversation.Participant) memberJSON {
	return memberJSON{
		AgentID:     p.AgentID,
		DisplayName: p.DisplayName,
		Provider:    p.Provider,
		Model:       s.agentModel(p.AgentID),
		Active:      p.Active,
	}
}

// agentModel 先读当前生效的配置，再回退到启动时从配置拍下来的 s.models
// （NewServer 的入参，写模式装配时已经含 Claude 导入并合并进 cfg.Agents 的
// 成员）。两次都不中回空串：这是「配置里已经没这个 id」，不是错误。
//
// A3 起 model 是能热换的身份字段：若先查 s.models，改完模型不重启时右侧
// 「成员与费用」仍会显示旧模型——用户刚被告知「立即生效」，界面却在说别的。
// 读模式没有 currentCfg（s.write 为 nil），自然走 s.models 这条回退。
func (s *Server) agentModel(agentID string) string {
	if cfg := s.currentCfg(); cfg != nil {
		if model := cfg.Agents[agentID].Model; model != "" {
			return model
		}
	}
	return s.models[agentID]
}

// memberWriteGate 是成员两个写端点共用的前四道门，顺序与 rename /
// delete / ask 逐条一致：
//
//  1. s.write == nil → 405（只读模式没有任何写能力，先于方法判断）；
//  2. 非 POST → 405 + Allow；
//  3. checkWriteRequest：Content-Type 必须是 application/json（415），
//     Origin / Sec-Fetch-Site 必须同源（403）。
//
// 返回 false 时已经写好错误响应。只读模式的说明里点名 -rw：只会说
// 「不允许」而不说怎么开写模式，用户会以为这个功能不存在。
func (s *Server) memberWriteGate(w http.ResponseWriter, r *http.Request) bool {
	if s.write == nil {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed,
			"只读模式没有成员管理接口：用 council serve -rw 启动才能加人或移除成员")
		return false
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "只支持 POST")
		return false
	}
	if err := checkWriteRequest(r); err != nil {
		status := http.StatusForbidden
		var guardErr *writeGuardError
		if errors.As(err, &guardErr) {
			status = guardErr.status
		}
		writeError(w, status, err.Error())
		return false
	}
	return true
}

// conversationMembers 读出改完之后的完整成员表并写 200。读失败只可能是
// 库出问题（会话可见性已经在 handler 里查过），按 500 报，不拿半份表
// 冒充成功。
func (s *Server) conversationMembers(w http.ResponseWriter, r *http.Request, convID string) {
	parts, err := s.store.Participants(r.Context(), convID, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("读取会话 %s 的成员失败: %v", convID, err))
		return
	}
	members := make([]memberJSON, 0, len(parts))
	for _, p := range parts {
		members = append(members, s.memberFor(p))
	}
	writeJSON(w, http.StatusOK, membersResponse{
		ConversationID: convID,
		Members:        members,
	})
}

// handleConversationMemberAdd 把配置里的一个 agent 加入会话。
//
// 门控顺序：memberWriteGate → 请求体是单个合法 JSON 对象（400）→
// agent_id 非空（400）→ 会话可见（404，已软删按不存在处理）→ agent
// 在当前配置里（400）→ 已是活跃成员则跳过写（200 幂等）→ 否则 Join
// （已退出过的同一个 agent 由 store 原地激活）→ 回完整成员表。
//
// 先查会话再查 agent：路径上的资源不存在比 body 里的字段不合法更根本，
// 两个都错时报 404 与 rename/delete 的资源优先口径一致。
func (s *Server) handleConversationMemberAdd(w http.ResponseWriter, r *http.Request, id string) {
	if !s.memberWriteGate(w, r) {
		return
	}

	var req memberAddRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxMemberBody))
	if err := dec.Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("请求体超过 %d 字节上限", maxMemberBody))
			return
		}
		writeError(w, http.StatusBadRequest, "请求体不是合法 JSON: "+err.Error())
		return
	}
	// 只认一个对象，理由同 handleAsk / rename：Decode 停在第一个值上，
	// 静默忽略后半段会让调用方以为第二个参数生效了。
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "请求体只能有一个 JSON 对象")
		return
	}

	agentID := strings.TrimSpace(req.AgentID)
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id 不能为空：请给出配置里的一个成员 id")
		return
	}

	if _, ok := s.lookupConversation(w, r, id); !ok {
		return
	}

	ac, ok := cfgAgents(s.currentCfg())[agentID]
	if !ok {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("未知成员 %q：它不在当前配置的 [agents] 里（Claude 导入的成员会合并进来）", agentID))
		return
	}

	// 已是活跃成员就什么都不写：AddParticipant 的 ON CONFLICT 会把
	// joined_at 刷成这一次请求的时刻，对重复点击而言那不是幂等——
	// 用户看到的“加入时间”会被一次误点改掉。先读一次再决定是否写，
	// 这也是唯一能区分「已在场」与「退出过需要重新激活」的地方。
	parts, err := s.store.Participants(r.Context(), id, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("读取会话 %s 的成员失败: %v", id, err))
		return
	}
	for _, p := range parts {
		if p.AgentID == agentID && p.Active {
			s.conversationMembers(w, r, id)
			return
		}
	}

	err = s.write.Svc.Join(r.Context(), id, conversation.Participant{
		AgentID:     agentID,
		DisplayName: ac.DisplayName,
		Provider:    ac.Provider,
		Active:      true,
	})
	if err != nil {
		if errors.Is(err, conversation.ErrConversationNotFound) {
			writeError(w, http.StatusNotFound, "会话不存在: "+id)
			return
		}
		writeError(w, http.StatusInternalServerError, "加入成员失败: "+err.Error())
		return
	}
	s.conversationMembers(w, r, id)
}

// handleConversationMemberRemove 让成员软退出：active=0 并记 left_at，
// 不删任何消息。
//
// 幂等口径与加人相反但同样明确：
//   - 成员从未加入过 → 404（拼错 id 时不能静默成功）；
//   - 已经退出过 → 200，且不再写 left_at（重复点击不刷新退出时间）；
//   - 还活跃 → Leave，成功后回完整成员表。
//
// 不读请求体：这个端点没有参数，空 body 与 {} 都合法（与 delete 一致）。
// Content-Type 仍须是 application/json，因为 checkWriteRequest 是统一闸，
// 不给某一个端点开“这次可以不带”的先例。
func (s *Server) handleConversationMemberRemove(w http.ResponseWriter, r *http.Request, id, agentID string) {
	if !s.memberWriteGate(w, r) {
		return
	}
	if agentID == "" {
		writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
		return
	}
	if _, ok := s.lookupConversation(w, r, id); !ok {
		return
	}

	parts, err := s.store.Participants(r.Context(), id, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("读取会话 %s 的成员失败: %v", id, err))
		return
	}
	var target *conversation.Participant
	for i := range parts {
		if parts[i].AgentID == agentID {
			target = &parts[i]
			break
		}
	}
	if target == nil {
		writeError(w, http.StatusNotFound,
			fmt.Sprintf("会话 %s 里没有成员 %q", id, agentID))
		return
	}
	if target.Active {
		if err := s.write.Svc.Leave(r.Context(), id, agentID, time.Now().UTC()); err != nil {
			if errors.Is(err, conversation.ErrConversationNotFound) ||
				errors.Is(err, conversation.ErrParticipantNotFound) {
				writeError(w, http.StatusNotFound, "会话或成员不存在: "+id+" / "+agentID)
				return
			}
			writeError(w, http.StatusInternalServerError, "移除成员失败: "+err.Error())
			return
		}
	}
	s.conversationMembers(w, r, id)
}
