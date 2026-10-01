package repl

// rules.go 实现 /rules 与 /rules reload [--rebuild]（11a 契约 5.1、5.2）。
//
// 本文件里的格式化函数只认 provider.Support 这个三态枚举与
// turn.ProviderRulesInfo / turn.DriftRow 这两个已经算好的结构体，
// 不碰 provider 包里任何 provider 专有的类型或常量——逐家的实测细节
// （谁能隔离、谁不能、跨轮有没有缓存收益）已经在 internal/turn 里翻成了
// 这三个字段，repl 只管把它们排成人话，这是"指令隔离位"那句注释要求的分工。

import (
	"context"
	"fmt"

	"council/internal/config"
	"council/internal/provider"
	"council/internal/turn"
)

// cmdRules 分派 /rules（无参数）与 /rules reload [--rebuild]。
func cmdRules(ctx context.Context, r *REPL, args cmdArgs) error {
	if len(args.fields) == 0 {
		return cmdRulesShow(ctx, r)
	}
	if args.fields[0] != "reload" {
		return r.printf("用法: /rules 或 /rules reload [--rebuild]\n")
	}
	switch len(args.fields) {
	case 1:
		return cmdRulesReload(ctx, r, false)
	case 2:
		if args.fields[1] != "--rebuild" {
			return r.printf("用法: /rules reload [--rebuild]\n")
		}
		return cmdRulesReload(ctx, r, true)
	default:
		return r.printf("用法: /rules reload [--rebuild]\n")
	}
}

// cmdRulesShow 打印 11a 契约 5.1 那张报告：两个规则文件的路径/字节数/
// 合计，逐 provider 一行，外加启动漂移审计同一个函数算出来的当前值。
func cmdRulesShow(ctx context.Context, r *REPL) error {
	rules := r.rulesCtx.Rules
	if err := r.printf("全局规则: %s\n", ruleFileLine(rules.GlobalPath, rules.GlobalFound, len(rules.GlobalText))); err != nil {
		return err
	}
	if err := r.printf("局部规则: %s\n", localRuleFileLine(rules)); err != nil {
		return err
	}
	if err := r.printf("合计 %d 字节\n\n", len(rules.GlobalText)+len(rules.LocalText)); err != nil {
		return err
	}

	for _, row := range r.rn.RulesReport() {
		// 多一列档位（11c 契约第 6 节）：开了 WORKSPACE 之后同一家 provider
		// 可能有两行，只印 ProviderID 分不清这一行报的是哪一档的余量。
		if err := r.printf("%s  %s  %s  %s  %s\n",
			row.ProviderID, row.Tier, isolationLabel(row.CleanInstrEnv), cacheLabel(row.CacheAcrossTurns), headroomLabel(row),
		); err != nil {
			return err
		}
	}

	// 只在 store_thought == false 且至少有一个 team 的 thought_visible_to
	// 非空时才印。印在 /rules 而不是启动横幅：横幅只在进 REPL 前印一次，
	// /config reload --apply 把 store_thought 关掉之后用户看不见横幅再来
	// 一次；/rules 是用户主动来看这一份配置怎么路由的地方。代价是不敲
	// /rules 就看不见这行——接受，因为无条件印在横幅上会给所有人加一行
	// 噪声，而下面那个函数已经要求两个条件同时成立才返回非空。
	if hint := storeThoughtVisibilityHint(r.cfg); hint != "" {
		if err := r.printf("\n%s\n", hint); err != nil {
			return err
		}
	}

	drift, err := r.rn.AuditDrift(ctx)
	if err != nil {
		// 审计本身失败不该让整条 /rules 打不出来：上面的文件与 provider
		// 信息已经打完了，这里只追加报错，不回滚已经写出去的内容。
		return r.reportErr("身份漂移审计失败", err)
	}
	if len(drift) == 0 {
		return nil
	}
	n1, n2 := 0, 0
	for _, row := range drift {
		if row.AskRefused() {
			n2++
		} else {
			n1++
		}
	}
	if n1 > 0 {
		if err := r.printf("\n身份已变的 provider 会话: %d 条（要重建才能接着用，各付一次入场费）\n", n1); err != nil {
			return err
		}
	}
	if n2 > 0 {
		if err := r.printf("\n按当前配置 /ask 会被拒的 provider 会话: %d 条（不花钱）\n", n2); err != nil {
			return err
		}
	}
	return nil
}

// storeThoughtVisibilityHint 在「关了存储、却还开着可见性」时给出一行提示。
// 位置选 /rules 的理由见 cmdRulesShow 里那次调用上方的注释。
//
// 文案不写「恒为空串」：store_thought=false 不洗历史，历史行的 thought
// 照样会被 Router 渲染进打开了可见性的 Mode 的 prompt。
func storeThoughtVisibilityHint(cfg *config.Config) string {
	if cfg == nil || cfg.StoreThought {
		return ""
	}
	for _, team := range cfg.Teams {
		if len(team.ThoughtVisibleTo) > 0 {
			return "store_thought = false：此后新增的轮次不再产生 thought；thought_visible_to 只对历史行生效。"
		}
	}
	return ""
}

// cmdRulesReload 重读两个规则文件。rebuild 为 false 时只报告、不改任何
// 状态；为 true 时应用新规则并对每一条漂移的会话执行 Rebuild。
//
// 预览（rebuild=false）怎么在"不改 Runner 状态"的前提下算出"换成新规则
// 会漂移哪些会话"：11a 契约 3.5 冻结的 AuditDrift(ctx) 没有参数，只认
// r.rules 当前的值，没有留一个"拿别的规则试算"的入口。internal/turn 内部
// 确实有一个按规则参数化的私有实现（auditDriftAgainst），但它是包内私有
// 的，repl 够不到，而且够不到也没关系：这里改用已经在契约里的公开方法
// SetRules（3.3）——临时把新规则换上去、调一次 AuditDrift、再换回来。
// REPL 是唯一的调用者，且这几行之间不会有别的 goroutine 插进来动
// Runner，所以"临时"是真的临时，不会被别的请求看到中间状态。这是本次
// 实现在契约留白处做的选择：没有为 AuditDrift 这个契约冻结的方法另开一个
// 公开的孪生方法，而是完全用它已经承诺的公开接口（SetRules + AuditDrift）
// 拼出预览这件事。
func cmdRulesReload(ctx context.Context, r *REPL, rebuild bool) error {
	newRules, err := config.LoadRules(r.rulesCtx.ConfigPath, r.rulesCtx.RulesFile)
	if err != nil {
		return r.reportErr("重读规则文件失败", err)
	}
	oldRules := r.rulesCtx.Rules

	if err := r.printf("全局规则: %d 字节 %s -> %d 字节 %s\n",
		len(oldRules.GlobalText), orPlaceholder(turn.RulesDigest(oldRules.GlobalText)),
		len(newRules.GlobalText), orPlaceholder(turn.RulesDigest(newRules.GlobalText)),
	); err != nil {
		return err
	}
	if err := r.printf("局部规则: %d 字节 %s -> %d 字节 %s\n",
		len(oldRules.LocalText), orPlaceholder(turn.RulesDigest(oldRules.LocalText)),
		len(newRules.LocalText), orPlaceholder(turn.RulesDigest(newRules.LocalText)),
	); err != nil {
		return err
	}

	newBlocks := turn.RulesBlocks{Global: newRules.GlobalText, Local: newRules.LocalText}
	oldBlocks := r.rn.Rules()
	r.rn.SetRules(newBlocks)
	if rebuild {
		// 从这一刻起新规则就是真的要用的，不是临时探测：立刻同步 REPL
		// 自己缓存的这一份，让「Runner 手里实际生效的规则」与「REPL 显示
		// 给用户看的规则」不可能因为下面任何一步提前 return（AuditDrift
		// 报错、某一行 printf 失败……）而分道扬镳——那样会出现 Runner
		// 已经在用新规则、但 /rules 还在显示旧字节数这种自相矛盾的状态。
		r.rulesCtx.Rules = newRules
	} else {
		// 只是预览：函数无论从哪个分支提前返回都必须换回去，用 defer
		// 而不是在每个 return 前手写一遍——那种写法只要漏掉一处
		// return，预览就会把探测用的规则悄悄留在 Runner 里，用户没有
		// 敲 --rebuild 却已经在用新规则，且没有任何提示。
		defer r.rn.SetRules(oldBlocks)
	}

	drift, auditErr := r.rn.AuditDrift(ctx)
	if auditErr != nil {
		return r.reportErr("计算漂移名单失败", auditErr)
	}

	if len(drift) == 0 {
		if err := r.printf("没有会话会因此漂移\n"); err != nil {
			return err
		}
	} else {
		if err := r.printf("会因此漂移的会话（%d 条）:\n", len(drift)); err != nil {
			return err
		}
		for _, row := range drift {
			if err := r.printf("  %s / %s: %s\n", row.ConvID, row.AgentID, row.Reason); err != nil {
				return err
			}
		}
	}

	if !rebuild {
		n1, n2 := 0, 0
		for _, row := range drift {
			if row.AskRefused() {
				n2++
			} else {
				n1++
			}
		}
		switch {
		case n1 > 0 && n2 == 0:
			return r.printf("要应用请敲 /rules reload --rebuild（会重建上列会话，各付一次入场费）\n")
		case n1 > 0 && n2 > 0:
			return r.printf("要应用请敲 /rules reload --rebuild（其中 %d 条会重建，各付一次入场费；另 %d 条只会被清掉，不花钱）\n", n1, n2)
		case n1 == 0 && n2 > 0:
			return r.printf("要应用请敲 /rules reload --rebuild（上列会话只会被清掉，不花钱）\n")
		default:
			return r.printf("要应用请敲 /rules reload --rebuild\n")
		}
	}

	// --rebuild：上面已经 SetRules 且 r.rulesCtx.Rules 也已经同步，现在
	// 对每一条漂移的会话执行 Rebuild。一条失败不阻止其余的：用户已经
	// 看到了完整的漂移名单，半路停下只会让状态更不清楚。
	for _, row := range drift {
		existed, rebuildErr := r.rn.Rebuild(ctx, row.ConvID, row.AgentID)
		if rebuildErr != nil {
			prefix := fmt.Sprintf("重建 %s/%s 失败", row.ConvID, row.AgentID)
			if row.AskRefused() {
				prefix = fmt.Sprintf("清掉 %s/%s 失败", row.ConvID, row.AgentID)
			}
			if err := r.reportErr(prefix, rebuildErr); err != nil {
				return err
			}
			continue
		}
		if row.AskRefused() {
			if err := r.printf("已清掉 %s/%s（existed=%v；按当前配置 /ask 会被拒，不花钱）\n", row.ConvID, row.AgentID, existed); err != nil {
				return err
			}
			continue
		}
		if err := r.printf("已重建 %s/%s（existed=%v）\n", row.ConvID, row.AgentID, existed); err != nil {
			return err
		}
	}
	return nil
}

// ruleFileLine 渲染"路径 (N 字节)"或"路径 (不存在)"。
func ruleFileLine(path string, found bool, n int) string {
	if !found {
		return fmt.Sprintf("%s (不存在)", path)
	}
	return fmt.Sprintf("%s (%d 字节)", path, n)
}

// localRuleFileLine 是局部规则那一行的特殊之处：配置里根本没写
// rules_file 时 LocalPath 是空串，这与"写了但文件不存在"是两种不同的
// 正常状态，必须分开说，不能都显示成"(不存在)"。
func localRuleFileLine(rules config.Rules) string {
	if rules.LocalPath == "" {
		return "(未配置)"
	}
	return ruleFileLine(rules.LocalPath, rules.LocalFound, len(rules.LocalText))
}

// isolationLabel 把三态 Support 直译成 11a 契约 5.1 给定的三句话。
// Unknown 按最坏情况显示（叠加），这是 fail-closed 的显示面写法——
// 用户看到"未验证，按叠加处理"，不会误以为 Council 的规则就是唯一来源。
func isolationLabel(s provider.Support) string {
	switch s {
	case provider.SupportYes:
		return "唯一指令源"
	case provider.SupportNo:
		return "叠加在用户全局规则之上"
	default:
		return "未验证，按叠加处理"
	}
}

// cacheLabel 把 CacheAcrossTurns 这个三态枚举直译成人话，取代此前逐 provider
// 都印同一句、且点名 agy 的固定文本 pricingNote——那句话本身违反了本文件
// 顶部那条"不写任何 provider 专有话术"的分工，是 11a 契约 5.1 写下时
// Capabilities 还没有这一位的权宜之计，能力位补上之后就该按三态分开说，
// 不能继续假装三家算法一样。措辞本身不是契约面，但三档必须分得开，
// 且不许出现任何 provider 的名字——逐家的实测细节（谁有缓存、谁没有）
// 已经在 provider.Capabilities.CacheAcrossTurns 的注释里写清楚了，
// 想知道"为什么"的人应该去读那份实测记录，不是来读 repl 的输出。
func cacheLabel(s provider.Support) string {
	switch s {
	case provider.SupportYes:
		return "进缓存前缀，首轮付"
	case provider.SupportNo:
		return "无缓存，按轮数乘着付"
	default:
		return "缓存行为未测"
	}
}

// headroomLabel 渲染命令行余量那一列。HeadroomNote 非空表示探针组不出
// 命令行（例如该 provider 拒绝当前权限档），此时 HeadroomBytes 无意义，
// 不能打印它，否则用户会把一个无意义的数字当成真余量。
func headroomLabel(row turn.ProviderRulesInfo) string {
	if row.HeadroomNote != "" {
		return fmt.Sprintf("不适用（%s）", row.HeadroomNote)
	}
	return fmt.Sprintf("命令行余量 %d 字节", row.HeadroomBytes)
}
