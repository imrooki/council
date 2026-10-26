package repl

import (
	"context"
	"strings"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
)

// addname_existing.go 是按名字加识别出的模型（AD1，addname.go）里「这个模型配置里已有成员」那一道门（D17，v1.2）：
// 单个名字没给强度也没给 --id 时，addByOneName 先调 addNameExisting，配置里有同一个模型的成员就加入、拒绝或列出，
// 都不写配置。这个文件只用到 AD1 之前就有的代码，放在块 A（契约 D16）；调用方是块 B 的 addname.go。

// D17 的说法（§3 冻结）。都是整行，含换行。
const (
	// msgAddNamePresent 是没给强度的名字对上的模型在配置里已有成员、其中有在场的；那一行没有强度可选。
	msgAddNamePresent = "%s 对上的模型已经有成员在当前会话里：%s\n"
	// msgAddNamePresentEffort 同上，那一行有强度可选，句末说怎样另加一档。
	msgAddNamePresentEffort = "%s 对上的模型已经有成员在当前会话里：%s；要另加一档强度，用 /add %s <强度> 或 --effort <强度>\n"
	// msgAddNameConfigured 是没给强度的名字对上的模型在配置里有不止一个成员、都不在场；那一行没有强度可选。
	msgAddNameConfigured = "%s 对上的模型在配置里有这几个成员：%s；用 /add <id> 加入其中一个\n"
	// msgAddNameConfiguredEffort 同上，那一行有强度可选。
	msgAddNameConfiguredEffort = "%s 对上的模型在配置里有这几个成员：%s；用 /add <id> 加入其中一个，或用 /add %s <强度> 带强度新加一档\n"
)

// addNameExisting 是 addByOneName 在没给强度、也没给 --id 时先过的一道门（D17）。S 是配置里
// 与 m 同一个模型的全部 agent（sameModelAgents，判据同 configuredAgentFor：provider 相同，
// model 等于 Model 或非空的 Alias；不看 id 前缀），去掉总结成员（它不当普通成员加，#47）。
// S 为空时 done 为假，接着按默认档新建。其余三种都不写配置：S 里有在场的就拒绝、点名在场
// 的那几个（有强度可选的行另说怎样加一档）；全不在场、恰好一个就加入它，屏幕与 cmdAdd 按
// 配置加入的那一支相同；不止一个就列出来，不猜。所以同一条命令敲两次，第二次说已在会话里。
func addNameExisting(ctx context.Context, r *REPL, name string, m provider.DiscoveredModel, active map[string]bool) (bool, error) {
	var s, present []string
	for _, id := range sameModelAgents(r.cfg, m) {
		if id == r.cfg.Summary.Agent {
			continue
		}
		s = append(s, id)
		if active[id] {
			present = append(present, id)
		}
	}
	switch {
	case len(s) == 0:
		return false, nil
	case len(present) > 0 && len(m.Efforts) > 0:
		return true, r.printf("%s", r.t("tui.addName.presentEffort", msgAddNamePresentEffort, name, strings.Join(present, ", "), name))
	case len(present) > 0:
		return true, r.printf("%s", r.t("tui.addName.present", msgAddNamePresent, name, strings.Join(present, ", ")))
	case len(s) == 1:
		return true, joinConfiguredAgent(ctx, r, s[0])
	case len(m.Efforts) > 0:
		return true, r.printf("%s", r.t("tui.addName.configuredEffort", msgAddNameConfiguredEffort, name, strings.Join(s, ", "), name))
	}
	return true, r.printf("%s", r.t("tui.addName.configured", msgAddNameConfigured, name, strings.Join(s, ", ")))
}

// sameModelAgents 按 configuredAgentFor 的判据（provider 相同，model 等于 Model 或非空的
// Alias）收集配置里与 m 同一个模型的全部 agent，按 id 排序，不看强度。addNameExisting 用它
// 判「这个模型配置里已有哪些成员」。
func sameModelAgents(cfg *config.Config, m provider.DiscoveredModel) []string {
	var out []string
	for _, id := range sortedKeys(cfg.Agents) {
		a := cfg.Agents[id]
		if a.Provider == m.Provider && (a.Model == m.Model || (m.Alias != "" && a.Model == m.Alias)) {
			out = append(out, id)
		}
	}
	return out
}

// joinConfiguredAgent 把配置里的 agent id 加入当前会话：与 cmdAdd 按配置加入的那一支
// （commands.go）同一个 Participant 写法、同一句「已加入会话」；Join 失败时照那一支打
// 「加入失败」（reportErr）后返回。
func joinConfiguredAgent(ctx context.Context, r *REPL, id string) error {
	ac := r.cfg.Agents[id]
	p := conversation.Participant{
		ConvID:      r.cur.ID,
		AgentID:     id,
		DisplayName: ac.DisplayName,
		Provider:    ac.Provider,
		Active:      true,
		JoinedAt:    time.Now().UTC(),
	}
	if err := r.svc.Join(ctx, r.cur.ID, p); err != nil {
		return r.reportErr(r.t("tui.err.join", "加入失败"), err)
	}
	return r.printf("%s", r.t("tui.add.joined", "%s 已加入会话 %s\n", id, r.cur.ID))
}
