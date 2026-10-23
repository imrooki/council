package repl

// configcmd.go 实现 /config 与 /config reload [--apply]（11d 契约 4.1）。
//
// 新增 agent 有两条路：手改 config.toml -> /config reload --apply -> /join；
// 或者 /add +编号（addmodel.go）把识别出的模型整段追加到配置文件末尾，再走与
// 本文件 --apply 同一套换指针流程（r.rn.SetConfig、r.cfg、SetThoughtPolicy）。
// 11d 当初否决「运行时新增 agent」的两个理由（写回会丢注释与格式；不写回就是
// 第二事实源）被「只追加、落盘」回应了，00_DECISIONS.md #45 推翻了那条裁决。
// 追加之后仍由 Config.Validate 校验，不另写一套运行时校验。

import (
	"context"
	"fmt"
	"strings"

	"council/internal/config"
	"council/internal/turn"
)

// cmdConfig 分派 /config（无参数）与 /config reload [--apply]。
//
// 命令表条目故意不标 needsRunner / needsSvc：无参数这一支只读
// r.rulesCtx.ConfigPath 与 r.cfg 的计数，provider 调用没起来时仍应能看一眼
// 当前配置。reload 才要 Runner（SetConfig + AuditDrift）和 Service
// （扫「已消失但仍是会话成员」、--apply 时改 thought 策略），在
// cmdConfigReload 里自己拦，拦的提示与 runCommand 同一句，不另造口径。
func cmdConfig(ctx context.Context, r *REPL, args cmdArgs) error {
	if len(args.fields) == 0 {
		return cmdConfigShow(r)
	}
	if args.fields[0] != "reload" {
		return r.printf("用法: /config 或 /config reload [--apply]\n")
	}
	switch len(args.fields) {
	case 1:
		return cmdConfigReload(ctx, r, false)
	case 2:
		if args.fields[1] != "--apply" {
			return r.printf("用法: /config reload [--apply]\n")
		}
		return cmdConfigReload(ctx, r, true)
	default:
		return r.printf("用法: /config reload [--apply]\n")
	}
}

// cmdConfigShow 打印生效的配置文件路径与三类计数。
// 路径用 r.rulesCtx.ConfigPath：那是启动时解析出来的生效路径，
// 也是 reload 要重读的那同一个文件，不能另解析一遍免得和正在生效的对不上。
//
// 多层合并时把其余层补在同一行（与 app 启动横幅同一个写法）：否则用户只会
// 看到项目文件，以为家目录那份配置没参与，进而以为项目里凭空多出了 provider。
// 层列表优先用 r.cfgLayers（重读成功时记下的那一份，与内存里的 r.cfg 对应）；
// 还没重读过就按默认位置现求一次，只读不写任何状态。
func cmdConfigShow(r *REPL) error {
	line := "配置文件: " + r.rulesCtx.ConfigPath
	if layers := r.knownConfigLayers(); len(layers) > 1 {
		line += "（合并 " + strings.Join(layers[1:], "、") + "）"
	}
	if err := r.printf("%s\n", line); err != nil {
		return err
	}
	return r.printf("agent %d，provider %d，team %d\n",
		len(r.cfg.Agents), len(r.cfg.Providers), len(r.cfg.Teams))
}

// knownConfigLayers 是 /config 展示用的层列表，高优先级在前。
// 空值（还没重读过、而且不是多层）时返回 nil。
func (r *REPL) knownConfigLayers() []string {
	if len(r.cfgLayers) > 0 {
		return r.cfgLayers
	}
	layers := reloadLayers(r.rulesCtx.ConfigPath)
	if len(layers) < 2 {
		return nil
	}
	return configLayerPaths(layers)
}

// cmdConfigReload 重读配置。apply 为 false 时只报告、不改任何状态；
// 为 true 时换掉 REPL 与 Runner 两处持有的 *config.Config。
//
// 「重读」的范围与启动时一致：默认位置下是项目层 + 全局层（合并后再校验），
// 用户显式 -config / COUNCIL_CONFIG 时仍是那一份文件。求层的规则见
// configlayers.go；写回（/add）永远只落最高优先层，本命令不写任何文件。
//
// 预览（apply=false）怎么在「不改 Runner 状态」的前提下算出「换成新配置
// 会漂移哪些会话」：AuditDrift(ctx) 没有参数，只认 Runner 当前的 r.cfg。
// internal/turn 内部确实有一个按规则参数化的私有实现（auditDriftAgainst），
// 但它是包内私有的，repl 够不到，而且够不到也没关系：这里改用已经在契约
// 里的公开方法 SetConfig——临时把新配置换上去、调一次 AuditDrift、再换回来。
// REPL 是唯一的调用者，且这几行之间不会有别的 goroutine 插进来动 Runner，
// 所以「临时」是真的临时。不给 internal/turn 加任何新函数来做这件事。
//
// 不带 --apply 时必须用 defer 把 Runner 换回去，不许在每个 return 前手写
// 一遍——那种写法只要漏掉一处 return，预览就会把探测用的配置悄悄留在
// Runner 里，用户没有敲 --apply 却已经在用新配置，而 /agents 仍读 REPL
// 自己那份旧指针，形成一个没有任何提示的脑裂。
//
// --apply 必须同时换掉两处持有者：r.rn.SetConfig(newCfg) 与 r.cfg = newCfg。
// app.Run 把同一个 *config.Config 指针分别交给 turn.NewRunner 与 repl.New，
// 全仓只有这两个持有者。只换 Runner 的话，/agents、/providers、/teams、
// /join、/who 全部继续显示旧配置，而 /ask 已经按新配置跑。两处都在 REPL
// 这一条 goroutine 上，是一个不可分的步骤；不允许只换一处并把它登记成
// 已知缺陷。从这一刻起新配置就是真的要用的，立刻同步，让下面任何一步
// 提前 return（AuditDrift 报错、某一行 printf 失败）都不可能让两处分道扬镳。
func cmdConfigReload(ctx context.Context, r *REPL, apply bool) error {
	if r.svc == nil {
		return r.printf("%s\n", msgNoStore)
	}
	if r.rn == nil {
		return r.printf("%s\n", msgNoRunner)
	}

	// 配置是多层合并的：这次读哪几层见 configlayers.go。用户显式用
	// -config / COUNCIL_CONFIG 点名一份文件时，这里退回单层，行为与改动前
	// 逐字节一致；默认位置下才是「项目层 + 全局层」。
	layers := reloadLayers(r.rulesCtx.ConfigPath)
	merged, err := config.LoadMerged(layers)
	if err != nil {
		// 校验失败与读/解析失败分开处置：reload 是唯一一条让一份非法配置
		// 进到运行中进程的路。启动时的处置是「写 Err、不进 REPL」，运行中
		// 不能照搬（不能把用户踢出 REPL），所以必须拒绝并留旧配置，而且不
		// 部分应用——部分应用会让内存里的 Config 变成一份 Validate 没签过
		// 字的东西，后续 /join /ask 的前置假设全碎。
		//
		// LoadMerged 已经把「合并 + 填缺省 + Validate」做完，校验错误的
		// Msg 末尾还带着「来自 <哪一份文件>」：合并之后光看错误路径已经
		// 推不出该改哪个文件了。这里只负责把同一份 ValidationErrors 逐条
		// 打出来，口径与启动时（app.reportConfigError）一致。
		ves, ok := err.(config.ValidationErrors)
		if !ok {
			// 读失败旧配置原样生效：连一份新对象都没有，无从换。
			return r.reportErr(configReadErrPrefix("重读配置文件失败", layers), err)
		}
		if err := r.printf("配置非法，旧配置原样生效:\n"); err != nil {
			return err
		}
		// Error() 已按 Path 字典序（同 Path 再按 Msg）排好。逐条打，
		// 不把整坨塞进 reportErr：启动时也是逐行写出全部错误，口径对齐。
		for _, line := range strings.Split(ves.Error(), "\n") {
			if line == "" {
				continue
			}
			if err := r.printf("  %s\n", line); err != nil {
				return err
			}
		}
		return nil
	}
	newCfg := merged.Config

	// 多层时把「读的是哪几份」说出来。放这里而不是最后：下面每一步都可能
	// 提前 return，层信息是理解后面所有报告的前提。只在详细档（-verbose）
	// 打印，默认档的错误文案与 /config 里仍有层信息，不会丢。
	if !r.concise {
		if notice := configLayersNotice(merged.Layers); notice != "" {
			if err := r.printf("%s", notice); err != nil {
				return err
			}
		}
	}

	// 根级键的比较必须在校验通过之后、换任何状态之前。data_dir 在启动时
	// 已经决定了打开哪个库、在哪个路径上持有 council.lock（进程级排他锁）。
	// reload 换掉它等于让进程内两份状态指向两个库，而锁还在旧的那个上。
	// rules_file 换掉则与 /rules reload 抢同一份规则正文的事实源，两条命令
	// 会各执一词。两个键都是「只能重启生效」。
	if newCfg.DataDir != r.cfg.DataDir || newCfg.RulesFile != r.cfg.RulesFile {
		return r.printf("data_dir 与 rules_file 只能重启生效，本轮一个字都不换；规则正文改动请用 /rules reload\n")
	}

	added := agentIDsOnlyIn(newCfg, r.cfg)
	gone := agentIDsOnlyIn(r.cfg, newCfg)
	brain := brainColumnChanges(r.cfg, newCfg)
	cmdChanged := providerCommandChanges(r.cfg, newCfg)

	// 「已消失但仍是会话成员」AuditDrift 答不上来：没开过口的成员根本没有
	// provider_sessions 行。要自己拿新旧 Agents 去扫 participants。含已退出
	// 的——退出是软删除，行还在，/who 仍列它们；配置里删掉之后这些行不会
	// 自己消失，正是这条报告要让人看见的。扫失败则整条命令失败、一个字
	// 都不换：报告出不来还去 --apply，用户看不见代价。
	stillMembers, err := goneButStillMembers(ctx, r, gone)
	if err != nil {
		return r.reportErr("列出会话成员失败", err)
	}

	for _, w := range cmdChanged {
		if err := r.printf("%s\n", w); err != nil {
			return err
		}
	}

	oldCfg := r.cfg
	r.rn.SetConfig(newCfg)
	if apply {
		r.cfg = newCfg
		// 层与 rules_file 基准跟配置一起换：它们描述的就是「内存里这份
		// 配置从哪来」，只换 Config 会把 /config 与 /rules reload 留在
		// 启动时那套坐标上，两处说法不一致。
		r.cfgLayers = merged.Layers
		r.rulesFileBase = merged.RulesFileBase
		// 与 app.go 装配点同一个调用：存储策略活在 Service 里，不换这一处
		// 的话 --apply 之后新写的消息仍按启动时的开关落 thought。
		r.svc.SetThoughtPolicy(newCfg.StoreThought)
	} else {
		defer r.rn.SetConfig(oldCfg)
	}

	drift, auditErr := r.rn.AuditDrift(ctx)
	if auditErr != nil {
		return r.reportErr("计算漂移名单失败", auditErr)
	}

	if err := printConfigDrift(r, drift); err != nil {
		return err
	}
	if err := printIDList(r, "配置里新增的 agent", added); err != nil {
		return err
	}
	if err := printGoneMembers(r, stillMembers); err != nil {
		return err
	}
	if err := printBrainChanges(r, brain); err != nil {
		return err
	}

	if !apply {
		n1 := 0
		for _, row := range drift {
			if !row.AskRefused() {
				n1++
			}
		}
		if n1 > 0 {
			return r.printf("要应用请敲 /config reload --apply（会换掉 REPL 与 Runner 两处持有的配置；其中 %d 条身份已变的会话要重建才能接着用，各付一次入场费）\n", n1)
		}
		return r.printf("要应用请敲 /config reload --apply（会换掉 REPL 与 Runner 两处持有的配置）\n")
	}
	return r.printf("已应用新配置\n")
}

// providerCommandChanges 找出两边都有、但 command 字符串变了的 provider。
//
// 这是一个指纹补不到的洞，必须响亮告警：身份指纹里的前缀摘要只摘
// cmd.Args，而 Command.Path 是在 BuildTurn 之后从 cfg.Providers 回填的，
// 从来不进摘要。把某家的 exe 换成另一个版本、或把 claude 从 .cmd 换成
// 别的垫片，前缀完全可能真的变了而指纹一个字不动。本命令只告警、建议
// /session rebuild，不改摘要的定义域（那是 11b 的冻结面）。
//
// 遍历前先把 key 排序，否则输出顺序随机、测试会随机失败。
func providerCommandChanges(oldCfg, newCfg *config.Config) []string {
	var lines []string
	for _, id := range sortedKeys(newCfg.Providers) {
		oldP, ok := oldCfg.Providers[id]
		if !ok {
			continue
		}
		newP := newCfg.Providers[id]
		if oldP.Command == newP.Command {
			continue
		}
		lines = append(lines, fmt.Sprintf(
			"警告: provider %s 的 command 从 %q 改为 %q。身份指纹的前缀摘要只摘命令行参数，Command.Path 是 BuildTurn 之后从 cfg.Providers 回填的、从来不进摘要，所以这次改动指纹一个字都不会动。若可执行文件确实换了，请对相关成员敲 /session rebuild",
			id, oldP.Command, newP.Command,
		))
	}
	return lines
}

// agentIDsOnlyIn 返回 a.Agents 有、b.Agents 没有的 id，已按字典序排好。
func agentIDsOnlyIn(a, b *config.Config) []string {
	var ids []string
	for _, id := range sortedKeys(a.Agents) {
		if _, ok := b.Agents[id]; !ok {
			ids = append(ids, id)
		}
	}
	return ids
}

type brainChange struct {
	id  string
	old config.AgentConfig
	new config.AgentConfig
}

// brainColumnChanges 比对两边都有的 agent 的 provider / model / effort。
// 只看这三列：它们是 /who 的 CONFIG_BRAIN 与 /log 的 BRAIN 在说的那件事。
// 权限档、角色、模式的变化若进了指纹，会走 AuditDrift 那一类，不混进这里。
func brainColumnChanges(oldCfg, newCfg *config.Config) []brainChange {
	var out []brainChange
	for _, id := range sortedKeys(oldCfg.Agents) {
		newA, ok := newCfg.Agents[id]
		if !ok {
			continue
		}
		oldA := oldCfg.Agents[id]
		if oldA.Provider == newA.Provider && oldA.Model == newA.Model && oldA.Effort == newA.Effort {
			continue
		}
		out = append(out, brainChange{id: id, old: oldA, new: newA})
	}
	return out
}

type goneMember struct {
	id      string
	convIDs []string
}

// goneButStillMembers 在 gone 这份「旧有新无」的 id 里，留下至少还挂在
// 某一场会话 participants 表上的那些。gone 必须已经排好序；返回值保持
// 同样的顺序，会话 id 各自再按字典序排，输出才稳定。
func goneButStillMembers(ctx context.Context, r *REPL, gone []string) ([]goneMember, error) {
	if len(gone) == 0 {
		return nil, nil
	}
	want := make(map[string]struct{}, len(gone))
	for _, id := range gone {
		want[id] = struct{}{}
	}

	convs, err := r.svc.Conversations(ctx)
	if err != nil {
		return nil, err
	}

	found := make(map[string]map[string]struct{}, len(want))
	for _, conv := range convs {
		ps, err := r.svc.Participants(ctx, conv.ID, true)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			if _, ok := want[p.AgentID]; !ok {
				continue
			}
			if found[p.AgentID] == nil {
				found[p.AgentID] = make(map[string]struct{})
			}
			found[p.AgentID][conv.ID] = struct{}{}
		}
	}

	var out []goneMember
	for _, id := range gone {
		convsOf := found[id]
		if len(convsOf) == 0 {
			continue
		}
		out = append(out, goneMember{id: id, convIDs: sortedKeys(convsOf)})
	}
	return out, nil
}

func printConfigDrift(r *REPL, drift []turn.DriftRow) error {
	if len(drift) == 0 {
		return r.printf("没有成员会因此漂移\n")
	}
	if err := r.printf("%d 个成员会漂移:\n", len(drift)); err != nil {
		return err
	}
	for _, row := range drift {
		if err := r.printf("  %s / %s: %s\n", row.ConvID, row.AgentID, row.Reason); err != nil {
			return err
		}
	}
	return nil
}

func printIDList(r *REPL, title string, ids []string) error {
	if err := r.printf("%s（%d）:\n", title, len(ids)); err != nil {
		return err
	}
	for _, id := range ids {
		if err := r.printf("  %s\n", id); err != nil {
			return err
		}
	}
	return nil
}

func printGoneMembers(r *REPL, members []goneMember) error {
	if err := r.printf("配置里已消失但仍是会话成员的 agent（%d）:\n", len(members)); err != nil {
		return err
	}
	for _, m := range members {
		if err := r.printf("  %s（会话 %s）\n", m.id, strings.Join(m.convIDs, ", ")); err != nil {
			return err
		}
	}
	return nil
}

func printBrainChanges(r *REPL, changes []brainChange) error {
	if err := r.printf("脑子三列有变化的 agent（%d）:\n", len(changes)); err != nil {
		return err
	}
	for _, c := range changes {
		if err := r.printf("  %s: %s -> %s\n",
			c.id,
			brainCell(c.old.Provider, c.old.Model, c.old.Effort),
			brainCell(c.new.Provider, c.new.Model, c.new.Effort),
		); err != nil {
			return err
		}
	}
	return nil
}
