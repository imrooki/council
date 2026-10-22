package turn

import (
	"fmt"
	"os/exec"
	"sort"

	"council/internal/config"
	"council/internal/provider"
)

// ProviderRulesInfo 是 /rules 逐 (provider, tier) 一行要显示的东西，只算
// 不显示：具体怎么把 CleanInstrEnv / CacheAcrossTurns 翻成中文措辞、
// HeadroomBytes 怎么拼进一句话，是 internal/repl 的事——本包不认识
// "叠加在用户全局规则之上""进缓存前缀，首轮付"这类给人看的话术，只给出
// repl 需要的原始数据（11a 契约 5.1 明写不许在 repl 里写任何 provider
// 专有的话术）。
//
// Tier 是 11c 契约第 6 节新增的一列：开了 WORKSPACE 之后同一家 provider
// 下可能有两档，且两档的探针命令行不一样长（claude 的 WORKSPACE 多一个
// acceptEdits），只报 ProviderID 会让用户看不出这一行到底是哪一档的余量。
type ProviderRulesInfo struct {
	ProviderID       string
	Tier             config.Permission // 这一行代表的权限档，配套 HeadroomBytes 一起看
	CleanInstrEnv    provider.Support
	CacheAcrossTurns provider.Support // 同一条会话续到下一轮时前缀还算不算钱，取自 Capabilities()
	HeadroomBytes    int              // maxCommandLineBytes 减去这次组出来的命令行长度
	HeadroomNote     string           // 组不出来时的原因；非空时 HeadroomBytes 无意义
}

// rulesReportKey 是 (provider id, 配置里写的权限档) 这一对，RulesReport
// 按它分组，不再只按 provider id 分组（11c 契约第 6 节）。
//
// 第二个字段用 config.Permission（配置里原样写的字符串），不是翻译之后的
// provider.PermissionTier：分组的目的是把"同一家 provider 下、探针命令行
// 长度不同的两档"分开报，这件事在 TranslatePermission 成功与否之前就该
// 成立——一个配错成 full 的 agent 也该单独占一行，报出"这一档不被放行"，
// 而不是被同一 provider 下别的合法档悄悄吞掉（旧实现"每家一行"就是这样：
// grok 若代表 agent 恰好是 workspace 档，那一行会印"不适用"，把同一家里
// 其实好好的 read agent 一起盖掉，见 11c 契约第 6 节原文）。
type rulesReportKey struct {
	providerID string
	tier       config.Permission
}

// RulesReport 为当前配置里出现过的每一个 (provider, tier) 组合算一行，
// 按 provider id 升序、同一 provider 内再按 tier 升序。"出现过"指某个
// agent 的 Provider 字段等于这个 id 且这个 id 确实登记在 provider.registry
// 里——查不到适配器的 provider 拿不到 Capabilities()，连 CleanInstrEnv
// 这一列都报不出来，报一行全是「未知」的记录不比不报更有用，索性跳过
// （不是契约要求的行为，是本函数在"当前配置里出现过的 provider" 这句话
// 本身没覆盖到的情况下做的选择，已在交接报告里说明）。
//
// 每组取字典序最小的那个 agent 来组探针请求，只是为了有一个确定、可复现
// 的代表，不代表"这一档的余量就是这个数"——换一个 agent（不同的
// Role/Model/Effort 长度）算出来的余量会不同。
func (r *Runner) RulesReport() []ProviderRulesInfo {
	// 一进来拷走规则和配置、立刻放读锁。整张表必须用同一份快照，
	// 否则与 SetRules / SetConfig 并发时，两行可能算自两份不同的
	// 规则或配置——未决 #26 的病根就是逐行各取一次锁。不在持有
	// rulesMu 或 cfgMu 时做 IO（下面 probeHeadroom 会调 BuildTurn
	// 与 exec.LookPath）。
	r.rulesMu.RLock()
	rules := r.rules
	r.rulesMu.RUnlock()
	r.cfgMu.RLock()
	cfg := r.cfg
	r.cfgMu.RUnlock()

	repAgent := make(map[rulesReportKey]string) // (provider, tier) -> 字典序最小的 agent id
	for agentID, ac := range cfg.Agents {
		key := rulesReportKey{providerID: ac.Provider, tier: ac.Permissions}
		cur, ok := repAgent[key]
		if !ok || agentID < cur {
			repAgent[key] = agentID
		}
	}

	keys := make([]rulesReportKey, 0, len(repAgent))
	for key := range repAgent {
		if _, err := provider.Lookup(key.providerID); err != nil {
			continue
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].providerID != keys[j].providerID {
			return keys[i].providerID < keys[j].providerID
		}
		return keys[i].tier < keys[j].tier
	})

	out := make([]ProviderRulesInfo, 0, len(keys))
	for _, key := range keys {
		out = append(out, r.rulesInfoFor(key, repAgent[key], cfg, rules))
	}
	return out
}

// rulesInfoFor 给单个 (provider, tier) 组合算一行，agentID 是那个字典序
// 最小的代表。
func (r *Runner) rulesInfoFor(key rulesReportKey, agentID string, cfg *config.Config, rules RulesBlocks) ProviderRulesInfo {
	p, err := provider.Lookup(key.providerID)
	if err != nil {
		// 走到这里说明 RulesReport 的过滤没起作用，属于编码错误，
		// 但仍然 fail-closed 地给出一行「查不到」而不是 panic。
		return ProviderRulesInfo{ProviderID: key.providerID, Tier: key.tier, HeadroomNote: fmt.Sprintf("provider 查不到: %v", err)}
	}
	caps := p.Capabilities()
	info := ProviderRulesInfo{
		ProviderID:       key.providerID,
		Tier:             key.tier,
		CleanInstrEnv:    caps.CleanInstructionEnv,
		CacheAcrossTurns: caps.CacheAcrossTurns,
	}

	bytes, note := r.probeHeadroom(p, agentID, cfg, rules)
	info.HeadroomBytes = bytes
	info.HeadroomNote = note
	return info
}

// probeHeadroom 现组一条**空正文**的命令行，量出命令行总长闸还剩多少余量。
//
// 三个刻意的选择，理由必须留在这里，免得下一个人"优化"掉：
//
//  1. ProviderSession 传空串（首轮），Prompt 传空串：契约 5.1 明写首轮是
//     argv 最长的那一轮（身份块 + 权限档 flag 都要发，且没有 --resume /
//     --conversation 那半段可以省）。传一个假的 ProviderSession 会走
//     resume 分支，量到的是较短的命令行，余量报大了反而危险——用户以为
//     还有余量，实际首轮会先撞上真正的闸。
//  2. 这样调 BuildTurn 是安全的：它只组字符串、不起进程、不落盘
//     （Command.Files 是待落盘的内容，由 Spawner 落，不是 BuildTurn
//     落）。唯一的非纯净处是 grok 在 ProviderSession=="" 时会铸一个 v4
//     UUID（provider/uuid.go，纯 crypto/rand，不写文件不记日志），探针
//     铸出来直接丢给 BuildTurn 用完就丢，不会与任何真实会话冲突——grok
//     的规矩本来就是"没有被输出确认过的 id 一律作废"。
//  3. cmd.Path 解析失败（LookPath 找不到）就用原值，这会让余量*偏乐观*：
//     解析后的绝对路径通常比配置里写的那个裸名字长。与 spawn.go 里
//     CommandLineBytes 本身"偏小"的免责声明是同一类取舍——这道闸本来就
//     不是精确值，宁可少算一点余量的消耗、也不为了"看起来更准"去猜一个
//     从未验证过的修正量。
func (r *Runner) probeHeadroom(p provider.Provider, agentID string, cfg *config.Config, rules RulesBlocks) (headroomBytes int, note string) {
	ac := cfg.Agents[agentID]
	tier, err := TranslatePermission(ac.Permissions)
	if err != nil {
		return 0, fmt.Sprintf("当前权限档不放行: %v", err)
	}

	identity := renderIdentity(agentID, ac, rules)
	req := provider.TurnRequest{
		AgentID:         agentID,
		DisplayName:     ac.DisplayName,
		Model:           ac.Model,
		Effort:          ac.Effort,
		Role:            identity,
		Permission:      tier,
		ProviderSession: "", // 首轮：argv 最长的那一轮，见函数注释第 1 条
		Prompt:          "", // 空正文探针
		// convID 传空串：这是一个跨会话的容量探针，不属于任何一条真实
		// 会话。agentWorkDir 对 READ 档本来就忽略 convID，空串不改变那一支
		// 的结果；对 WORKSPACE 档，filepath.Join 会跳过空段，退化成
		// "<workDir>/<agentID>"（与 READ 那一支同一条路径）——这让
		// WORKSPACE 那一行的余量比真实请求（真实 convID 会让路径更长）
		// 略偏乐观几个字节，与本函数注释第 3 条「cmd.Path 未解析时偏乐观」
		// 是同一类、量级同样很小的取舍，不单独再引入一个"猜一个 convID
		// 长度"的修正量。
		WorkDir:  r.dirFor("", agentID, tier),
		RoleDir:  r.roleDirFor(agentID),
		MaxTurns: 0,
	}
	cmd, err := p.BuildTurn(req)
	if err != nil {
		// 组不出来（例如该 provider 拒绝当前权限档）：契约明写不要让
		// 整条命令失败，印"不适用（原因）"即可，原因就是这个 error。
		return 0, err.Error()
	}
	if cmd.Path == "" && cfg.Providers != nil {
		// 与 Ask 里同一处回填保持一致：适配器不一定认识配置里的可执行文件
		// 路径，空 Path 会让 exec.LookPath 直接落空。
		cmd.Path = cfg.Providers[ac.Provider].Command
	}

	path := cmd.Path
	if resolved, lookErr := exec.LookPath(path); lookErr == nil {
		path = resolved
	}
	remaining := maxCommandLineBytes - CommandLineBytes(path, cmd.Args)
	return remaining, ""
}
