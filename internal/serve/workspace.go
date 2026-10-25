package serve

// workspace.go 是「工作区」这层纯分组在 HTTP 上的四个端点：
//
//	GET  /api/workspaces                       列出全部工作区（带会话数）
//	POST /api/workspaces                       新建
//	POST /api/workspaces/{id}/rename           改名
//	POST /api/workspaces/{id}/delete           删除（成员会话移出分组）
//	POST /api/conversations/{id}/workspace     把会话放进/移出工作区
//
// 工作区不绑定目录，也不影响会话的可见性：删一个工作区只是把成员的
// workspace_id 置 NULL，会话、消息与用量一条都不动（见 store.DeleteWorkspace）。
//
// 门控顺序与 rename / delete / archive 逐条一致：只读模式的 405 先于方法判断，
// 方法判断先于写闸（415 / 403），写闸先于请求体，请求体先于资源存在性。
// GET /api/workspaces 是读端点，不受写闸约束 —— 只读看板也要能看到分组，
// 与 ?archived=1 的归档视图同一个口径。

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"council/internal/conversation"
	"council/internal/store"
)

// maxWorkspaceBody 是工作区三个写端点（新建 / 改名 / 改归属）的请求体上限。
// body 里只有一个名字或一个 id，60 个字符即使每个都是 4 字节的 rune 也不到
// 1 KiB；4 KiB 足够容纳 JSON 转义，超过它的请求体不是一个名字，没必要读完再拒。
const maxWorkspaceBody = 4 << 10

// workspaceJSON 是 GET /api/workspaces 的数组元素，也是新建成功后的响应。
//
// conversation_count 的口径见 store.Workspaces：已软删的会话不计，
// 已归档的照计 —— 归档只换列表，不把会话移出分组。
type workspaceJSON struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	SortOrder         int    `json:"sort_order"`
	ConversationCount int    `json:"conversation_count"`
}

func newWorkspaceJSON(w conversation.Workspace) workspaceJSON {
	return workspaceJSON{
		ID:                w.ID,
		Name:              w.Name,
		SortOrder:         w.SortOrder,
		ConversationCount: w.ConversationCount,
	}
}

// workspaceNameRequest 是新建与改名共用的请求体。
// 多传的字段按 Go 的 JSON 解码规则忽略，与 /api/ask、rename 一致。
type workspaceNameRequest struct {
	Name string `json:"name"`
}

// workspaceRenameResponse 是改名成功的响应：只回 id 与新名字。
// name 是去空白之后**真正存进库**的那个，前端不必自己再 trim 一遍
// （两处 trim 迟早会漂）。
type workspaceRenameResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// workspaceDeleteResponse 是删除成功的响应。字段名与既有的 delete 端点一致：
// 前端要的就是「哪一个已经没了」。
type workspaceDeleteResponse struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// conversationWorkspaceRequest 是 POST /api/conversations/{id}/workspace 的请求体。
//
// WorkspaceID 用指针：JSON 的 null 与字段缺席都解成 nil，两种都表示「移出」。
// 空串同样表示移出（见 store.SetConversationWorkspace 的注释）。
// 非空时必须是已存在的工作区 id，未知 id 回 404 而不是静默接受。
type conversationWorkspaceRequest struct {
	WorkspaceID *string `json:"workspace_id"`
}

// conversationWorkspaceResponse 回显这场会话**现在**的归属：
// workspace_id 为空串表示未分组。前端据此直接改本地状态，不必再拉一次列表。
type conversationWorkspaceResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
}

// handleWorkspaces 处理 /api/workspaces：GET 列表、POST 新建。
func (s *Server) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleWorkspaceList(w, r)
	case http.MethodPost:
		s.handleWorkspaceCreate(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "只支持 GET 与 POST")
	}
}

// handleWorkspaceList 列出全部工作区，按 sort_order、name 升序。
// 空结果是 [] 不是 null —— 前端要稳定地遍历它。
//
// 只读模式同样可用：分组是库里的既有事实，只读看板没有理由看不见它。
func (s *Server) handleWorkspaceList(w http.ResponseWriter, r *http.Request) {
	ws, err := s.store.Workspaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取工作区列表失败: "+err.Error())
		return
	}
	out := make([]workspaceJSON, 0, len(ws))
	for _, one := range ws {
		out = append(out, newWorkspaceJSON(one))
	}
	writeJSON(w, http.StatusOK, out)
}

// workspaceWriteGate 是工作区三个写端点共用的前四道门，顺序与
// memberWriteGate / rename / delete / archive 逐条一致：
//
//  1. s.write == nil → 405（只读模式没有任何写能力，先于方法判断）；
//  2. 非 POST → 405 + Allow；
//  3. checkWriteRequest：Content-Type 必须是 application/json（415），
//     Origin / Sec-Fetch-Site 必须同源（403）。
//
// 返回 false 时已经写好错误响应。
func (s *Server) workspaceWriteGate(w http.ResponseWriter, r *http.Request) bool {
	if s.write == nil {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed,
			"只读模式没有工作区接口：用 council serve -rw 启动才能管理工作区")
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

// handleWorkspaceCreate 新建一个工作区。
//
// 门控顺序：workspaceWriteGate → 请求体是单个合法 JSON 对象（400）→
// 名字去空白后 1–60 个字符（400）→ 判重（409）→ 建（200）。
//
// 200 而不是 201：本仓库所有写端点都回 200，前端按「有 body 就说明成功」
// 处理；为这一个端点引入 201 会让前端的成功判定多一种分支。
func (s *Server) handleWorkspaceCreate(w http.ResponseWriter, r *http.Request) {
	if !s.workspaceWriteGate(w, r) {
		return
	}
	name, ok := s.readWorkspaceName(w, r)
	if !ok {
		return
	}

	ws, err := s.store.CreateWorkspace(r.Context(), name)
	if err != nil {
		if writeWorkspaceError(w, err, name, "") {
			return
		}
		writeError(w, http.StatusInternalServerError, "新建工作区失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, newWorkspaceJSON(ws))
}

// handleWorkspaceRename 改工作区的名字。
//
// 与新建同一套门控；判重时 store 会排除自己，所以「改成现在这个名字
// （哪怕只改大小写）」是幂等成功而不是 409。
func (s *Server) handleWorkspaceRename(w http.ResponseWriter, r *http.Request, id string) {
	if !s.workspaceWriteGate(w, r) {
		return
	}
	name, ok := s.readWorkspaceName(w, r)
	if !ok {
		return
	}

	ws, err := s.store.RenameWorkspace(r.Context(), id, name)
	if err != nil {
		if writeWorkspaceError(w, err, name, id) {
			return
		}
		writeError(w, http.StatusInternalServerError, "改写工作区名失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, workspaceRenameResponse{ID: ws.ID, Name: ws.Name})
}

// handleWorkspaceDelete 删掉一个工作区。它只清成员会话的 workspace_id，
// 不删任何会话 —— 这条语义在 store.DeleteWorkspace 里由同一个事务保证。
//
// 不读请求体：这个端点没有参数，空 body 与 {} 都合法（与 delete / archive 一致）。
// 未知 id 是 404 而不是幂等 200：删一个不存在的组只可能是调用方拿错了 id。
func (s *Server) handleWorkspaceDelete(w http.ResponseWriter, r *http.Request, id string) {
	if !s.workspaceWriteGate(w, r) {
		return
	}
	if err := s.store.DeleteWorkspace(r.Context(), id); err != nil {
		if writeWorkspaceError(w, err, "", id) {
			return
		}
		writeError(w, http.StatusInternalServerError, "删除工作区失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, workspaceDeleteResponse{ID: id, Deleted: true})
}

// handleConversationWorkspace 改一场会话的归属。workspace_id 为 null 或空串
// 表示移出分组。
//
// 门控顺序与 rename / delete 逐条对齐：
//  1. workspaceWriteGate（只读 405 → 非 POST 405 → JSON / 同源）；
//  2. 请求体是单个合法 JSON 对象（400）；
//  3. 会话可见（404，已软删按不存在处理）——先用 Conversation 做可见性预检，
//     理由与 rename 相同：归属事务只查行是否存在，没有这一步已删会话的归属
//     仍会被改掉；
//  4. 目标工作区存在（404）；空串/nil 不需要目标；
//  5. store.SetConversationWorkspace 在写事务里完成。
func (s *Server) handleConversationWorkspace(w http.ResponseWriter, r *http.Request, id string) {
	if !s.workspaceWriteGate(w, r) {
		return
	}

	var req conversationWorkspaceRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxWorkspaceBody))
	if err := dec.Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("请求体超过 %d 字节上限", maxWorkspaceBody))
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

	target := ""
	if req.WorkspaceID != nil {
		target = strings.TrimSpace(*req.WorkspaceID)
	}

	if _, err := s.store.Conversation(r.Context(), id); err != nil {
		if errors.Is(err, conversation.ErrConversationNotFound) {
			writeError(w, http.StatusNotFound, "会话不存在: "+id)
			return
		}
		writeError(w, http.StatusInternalServerError, "读取会话失败: "+err.Error())
		return
	}

	// 空串/nil 传 nil：store 把它写成 NULL，不查工作区是否存在。
	var workspaceID *string
	if target != "" {
		workspaceID = &target
	}
	if err := s.store.SetConversationWorkspace(r.Context(), id, workspaceID); err != nil {
		switch {
		case errors.Is(err, conversation.ErrConversationNotFound):
			writeError(w, http.StatusNotFound, "会话不存在: "+id)
		case errors.Is(err, store.ErrWorkspaceNotFound):
			writeError(w, http.StatusNotFound, "工作区不存在: "+target)
		default:
			writeError(w, http.StatusInternalServerError, "改会话归属失败: "+err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, conversationWorkspaceResponse{ID: id, WorkspaceID: target})
}

// readWorkspaceName 读并校验工作区名字请求体。返回 false 时已经写好 400。
//
// 去空白与长度上限在这里先判一次，只为给出比 store 更具体的说明（空 vs 超长）。
// 规则本身仍以 store.normalizeWorkspaceName 为准：store 也校验，任何绕过
// 本函数的调用方同样写不进非法名字。
func (s *Server) readWorkspaceName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req workspaceNameRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxWorkspaceBody))
	if err := dec.Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("请求体超过 %d 字节上限", maxWorkspaceBody))
			return "", false
		}
		writeError(w, http.StatusBadRequest, "请求体不是合法 JSON: "+err.Error())
		return "", false
	}
	// 只认一个对象，理由同 handleAsk / rename。
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "请求体只能有一个 JSON 对象")
		return "", false
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("工作区名不能为空：请给出 1–%d 个字符的名称", store.MaxWorkspaceNameChars))
		return "", false
	}
	if n := utf8.RuneCountInString(name); n > store.MaxWorkspaceNameChars {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("工作区名最长 %d 个字符，实际 %d 个字符", store.MaxWorkspaceNameChars, n))
		return "", false
	}
	return name, true
}

// writeWorkspaceError 把 store 的工作区哨兵翻成 HTTP 状态码。
// 返回 true 表示已经写过响应。id 是请求路径上的工作区 id（新建时为空串），
// name 是这次请求里的名字（删除时为空串）。
//
// 409 的文案要点名是哪个名字、以及「大小写不敏感」这条判重口径：
// 用户建了 `工作区` 再建 `工作区`，若只说「重名」而不说口径，会以为
// 是两次请求撞了同一个名字；说清之后他才知道 `Work` 与 `work` 也只能有一个。
//
// 404 要点名 id（与 rename / delete / archive 对会话的 404 同一体例）：
// 只说「工作区不存在」而库里有好几个组时，用户不知道自己点错了哪一个。
func writeWorkspaceError(w http.ResponseWriter, err error, name, id string) bool {
	switch {
	case errors.Is(err, store.ErrWorkspaceNameTaken):
		writeError(w, http.StatusConflict,
			fmt.Sprintf("工作区名 %q 已被占用（大小写不敏感，去空白后比较）", name))
		return true
	case errors.Is(err, store.ErrInvalidWorkspaceName):
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("工作区名不合法：去空白后必须是 1–%d 个字符", store.MaxWorkspaceNameChars))
		return true
	case errors.Is(err, store.ErrWorkspaceNotFound):
		if id != "" {
			writeError(w, http.StatusNotFound, "工作区不存在: "+id)
			return true
		}
		writeError(w, http.StatusNotFound, "工作区不存在")
		return true
	default:
		return false
	}
}

// handleWorkspacePath 处理 /api/workspaces/{id} 一棵子树：
// /{id}/rename 与 /{id}/delete。只认这两段固定尾巴，别的一律 404。
func (s *Server) handleWorkspacePath(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/workspaces/")
	if rest == "" || strings.HasSuffix(rest, "/") {
		writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
		return
	}
	segments := strings.Split(rest, "/")
	if len(segments) != 2 {
		writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
		return
	}
	switch segments[1] {
	case "rename":
		// 写接口：只读模式下也要先走到自己的 405 + 说明，不能被 404 掉。
		s.handleWorkspaceRename(w, r, segments[0])
	case "delete":
		s.handleWorkspaceDelete(w, r, segments[0])
	default:
		writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
	}
}
