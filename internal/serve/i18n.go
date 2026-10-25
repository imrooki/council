package serve

// i18n.go 是看板的中英文切面：GET /api/i18n 把当前语言的界面文案表交给
// 浏览器；设置页的 language 写盘在 settings.go。
//
// 语言只在启动时解析一次（serve.Run -> SetLanguage），之后设置页保存
// language 成功时会再调用一次。它就是「界面文案用哪张表」，不参与任何
// 执行路径：不进 Runner 配置、不改变 provider 的 argv，也不进身份指纹。

import (
	"net/http"
	"strings"

	"council/internal/i18n"
)

// SetLanguage 按与 CLI 相同的优先级求一次生效语言，并记住显式的 -lang。
// flagValue 只用于展示「启动参数覆盖了配置」：非法值在这里不报错，
// i18n.Resolve 会按默认语言处理，与 /api/i18n 的行为一致。
func (s *Server) SetLanguage(flagValue, configValue string) {
	lang := i18n.Resolve(flagValue, configValue)
	flag := strings.ToLower(strings.TrimSpace(flagValue))
	if flag != string(i18n.LangZh) && flag != string(i18n.LangEn) {
		// auto / 空串 / 乱写都不构成覆盖：它们要么继续往下一级看，
		// 要么按默认处理，设置页不该显示一句「启动参数优先」的假警告。
		flag = ""
	}
	s.langMu.Lock()
	s.lang = lang
	s.langFlag = flag
	s.langMu.Unlock()
}

// Language 返回本次进程实际生效的语言。未设置时按默认值，绝不留空串
// 给 /api/i18n 或设置页。
func (s *Server) Language() i18n.Lang {
	s.langMu.RLock()
	defer s.langMu.RUnlock()
	if s.lang == "" {
		return i18n.Default
	}
	return s.lang
}

// LanguageFlag 返回启动时显式有效的 -lang 值（zh / en），没有则为空串。
// 设置页用它说明「配置里的选择为什么没有马上生效」。
func (s *Server) LanguageFlag() string {
	s.langMu.RLock()
	defer s.langMu.RUnlock()
	return s.langFlag
}

// i18nResponse 是 GET /api/i18n 的响应。messages 只含 "ui." 前缀的界面
// 文案；服务端错误信息不进这张表。
type i18nResponse struct {
	Lang     string            `json:"lang"`
	Messages map[string]string `json:"messages"`
}

func (s *Server) handleI18n(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	lang := s.Language()
	writeJSON(w, http.StatusOK, i18nResponse{
		Lang:     string(lang),
		Messages: i18n.UIMessages(lang),
	})
}
