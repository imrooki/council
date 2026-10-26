package repl

import (
	"context"

	"council/internal/i18n"
	"council/internal/provider"
)

// addlogin.go 是 /add 识别结果之后的「各家的登录状态」（00_DECISIONS.md #48，OB3），以及
// 首次运行与 /add 共用的两样东西：四档登录状态的说法（LoginText）、各家的登录办法
// （LoginCommand）。判据全在 provider 包（LoginFromDiscovery、LoginStatusCommandFor、
// ParseLoginStatus），这里只取数与上屏。
//
// 登录状态是识别快照的一部分：与识别表同一次拍、随快照留到本会话结束，只有
// /add --refresh 或新会话重拍；/config reload --apply 不作废。

// msgLoginHead 是登录段的段头。
const msgLoginHead = "各家的登录状态（本会话的快照，/add --refresh 重新检查）:\n"

// loginCommands 是各家的登录办法。claude 取自 claude auth --help，grok、codex 取自
// 各自 --help 的 login 子命令；agy 的 --help 里没有登录相关的子命令，写兜底句。
var loginCommands = map[string]string{
	"claude": "claude auth login",
	"grok":   "grok login",
	"agy":    "agy（按提示登录）",
	"codex":  "codex login",
}

// LoginCommand 是一家的登录办法；首次运行在结果之后列出的登录办法与 /add 登录段里没登录那一行
// 用的都是它。首次运行一家都没找到时的安装说明（app 包的 msgFirstRunNoneFound）另写着各家的
// 登录办法，不经它（19f §7 (j)）。不认识的 id 返回空串。
func LoginCommand(providerID string) string {
	return LoginCommandLang(i18n.LangZh, providerID)
}

// LoginCommandLang 是 LoginCommand 带语言的那一半。只有 agy 的兜底说明是
// 一句话，其余三家的值本来就是命令行，原样返回。
func LoginCommandLang(lang i18n.Lang, providerID string) string {
	if providerID == "agy" {
		return i18n.TF(lang, "tui.add.login.agyHint", "agy（按提示登录）")
	}
	return loginCommands[providerID]
}

// LoginText 是登录状态的说法，四档之一；claude 已登录时带上订阅的原值，已登录（API key）
// 时带上 key 来源的原值（OB2）。首次运行的结果行与 /add 的登录段用的都是它。
func LoginText(l provider.LoginInfo) string {
	return LoginTextLang(i18n.LangZh, l)
}

// LoginTextLang 是 LoginText 带语言的那一半。
func LoginTextLang(lang i18n.Lang, l provider.LoginInfo) string {
	switch l.State {
	case provider.LoginYes:
		if l.Subscription != "" {
			return i18n.TF(lang, "tui.add.login.yesSubscription", "已登录（订阅：%s）", l.Subscription)
		}
		return i18n.TF(lang, "tui.add.login.yes", "已登录")
	case provider.LoginAPIKey:
		if l.KeySource != "" {
			return i18n.TF(lang, "tui.add.login.yesAPIKeySource", "已登录（API key：%s）", l.KeySource)
		}
		return i18n.TF(lang, "tui.add.login.yesAPIKey", "已登录（API key）")
	case provider.LoginNo:
		return i18n.TF(lang, "tui.add.login.no", "没登录")
	default:
		return i18n.TF(lang, "tui.add.login.unknown", "登录状态查不出")
	}
}

// SetLoginStatusExec 装上 /add 跑登录状态命令（今天只有 codex login status）用的执行器。
// 零值 nil 是不做登录检测：识别快照里没有登录状态，/add 不打登录段，也不多起任何进程，
// 所以直接 New 出来的 REPL（既有测试）输出不变；app 装配时装 turn.RunLoginStatus。
// 只许在 Run 或第一次 Submit 之前调。
func (r *REPL) SetLoginStatusExec(f provider.LoginStatusExec) {
	r.loginStatus = f
}

// snapLogin 是快照里一家的登录状态。
type snapLogin struct {
	provider string
	info     provider.LoginInfo
}

// snapshotLogin 判一家的登录状态，判据与首次运行的 firstRunCheck 相同：claude、grok 从识别
// 那一次截下的 stdout 判；codex 在识别之后、而且这一家的 ctx 还没到期时另跑一次登录状态命令
// （用配置里的 command），执行器报错就是查不出；其余查不出。ctx 是这一家识别用的那一个，
// 识别与登录状态命令共用一个 discoverTimeout。
func snapshotLogin(ctx context.Context, r *REPL, pid string, stdout []byte) provider.LoginInfo {
	info := provider.LoginFromDiscovery(pid, stdout)
	if sc, ok := provider.LoginStatusCommandFor(pid, r.cfg.Providers[pid].Command); ok && ctx.Err() == nil {
		if code, so, se, err := r.loginStatus(ctx, sc); err == nil {
			info = provider.ParseLoginStatus(pid, code, so, se)
		}
	}
	return info
}

// printLoginStatus 在识别表（连同 custom 行、失败行与 --refresh 提示）之后打登录段：
// 段头一行，然后快照里的每一家一行，顺序与「正在识别模型」相同。快照里没有登录状态
// （没装执行器）时什么都不打。
//
// 只照快照打，不按当前 r.cfg 滤：快照之后 /config reload --apply 删掉的 provider 照样
// 有一行，与失败行同一个处置（识别表按当前配置滤的是已经配成成员的模型，不是 provider）。
//
// 只有没登录的那一家在行末带登录办法与「登录之后 /add --refresh」：查不出的不带（agy 永远
// 查不出，每次都提示是噪声），已登录、已登录（API key）的不带。
func printLoginStatus(r *REPL, snap *modelSnapshot) error {
	if len(snap.logins) == 0 {
		return nil
	}
	if err := r.write(r.t("tui.add.login.head", msgLoginHead)); err != nil {
		return err
	}
	for _, l := range snap.logins {
		if l.info.State == provider.LoginNo {
			if err := r.printf("%s", r.t("tui.add.login.notLoggedIn", "  %-6s  %s；在终端里运行 %s，登录之后 /add --refresh\n", l.provider, LoginTextLang(r.lang, l.info), LoginCommandLang(r.lang, l.provider))); err != nil {
				return err
			}
			continue
		}
		if err := r.printf("  %-6s  %s\n", l.provider, LoginTextLang(r.lang, l.info)); err != nil {
			return err
		}
	}
	return nil
}
