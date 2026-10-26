package serve

// settings_reload.go 实现 docs/29_FIXES_PLAN.md A2 的完整版：设置页保存之后，
// 能当场热换的字段一律热换，热换的代价（重置该成员的 provider 会话）写清楚。
//
// 分级表（字段名与 settingsAgentRequest / 配置文件一致）：
//
//   - 可热更、不碰会话：mode。它只决定 PlanRound 里这个人发言还是当裁判，
//     既不进 renderIdentityCore（身份文本只拼 DisplayName / Provider / Role），
//     也不进 identityFPAt 的 probe 请求，因此改 mode 不会让任何 provider
//     会话的身份指纹变化。turn 包的
//     TestPlanRoundModeChangeMovesMemberButNotFingerprint 已经钉住这一点；
//
//   - 可热更、但必须重置该成员的 provider 会话：display_name、role、provider、
//     model、effort、prompt、permissions。它们全部会改变
//     turn.IdentityFingerprint：display_name / role 进 renderIdentityCore
//     （"You are <DisplayName> ..." 与 Role 正文），provider 也拼在那块身份
//     文本里，model / effort 是第二三段，permissions 决定权限档（第四段），
//     prompt 让 claude 的 --append-system-prompt-file 路径哈希变（第七段
//     前缀摘要），provider 还换掉命令本身。
//
//     指纹变了就不能再拿旧会话 id 去 --resume：Runner 的内存快照若还在，
//     下一轮直接撞 ErrIdentityDrift，成员突然不能说话；库里那条若还在，
//     下一轮会静默作废重建（白烧一次入场费，且用户不知情）。所以热换这些
//     字段时必须把该成员**在所有会话里**的 provider_sessions 删掉（不是只删
//     当前会话：指纹不匹配在任何一条会话里都会触发漂移），并在 Runner 里
//     清掉对应闸的内存快照。代价是上下文不再续接，这一点必须让用户看见
//     （响应 note + 前端预览框都写明）。
//
//   - 仍然必须重启：没有单个字段属于这一类。settingsRestartFieldNames 保持
//     空表，因为还有三类改动绕不开重启：新增成员、删除成员，以及「其它成员
//     的配置在磁盘上被人手改了 / 顶层配置变了」。它们不是字段级差异，而是
//     否决整次热换的成员级原因，仍然由 settingsRestartReasons 报出来。
//     将来若出现一个真正无法热换的字段，把它加进 settingsRestartFieldNames
//     即可：settingsHotChanged 会跟着返回 false，保存走重启分支，不需要改
//     别处。
//
// language 不在本表里：它是顶层键、有自己的 /api/settings/language 端点，
// 保存后立即换 /api/i18n 的文案表，既不进 Runner 配置也不进身份指纹。
//
// 判定基准是 RunCfg（Runner 真正在用的那一份），不是展示配置 Cfg：
// 上一次「改 model 但没重启」之后，展示配置已经比 RunCfg 新；此时再改
// mode 若只和展示配置比，就会把上一次的 model 改动一起热换进去。
//
// 有轮次在跑时既不热换、也不重置会话。settingsHotReload 沿用既有的
// runningCount 闸（handleAsk 把 job 放进任务表时抢同一把 reloadMu）；
// 身份字段的保存更严一层：在**写盘之前**就拒绝（见
// handleSettingsAgentSave 对这个判断的调用），因为「文件已写、会话已旧、
// Runner 还没换」正是这次改动要消灭的那半个状态。

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"council/internal/config"
)

// settingsHotFieldNames / settingsIdentityFieldNames / settingsRestartFieldNames
// 是唯一一份分级表：GET /api/settings 把它交给前端做保存前预览，保存时的
// 判定也用它。三处各写一份的话，预览说「立即生效」而服务端回「需要重启」
// 只是时间问题。
var (
	// settingsHotFieldNames 里的字段热换时不碰 provider 会话。
	settingsHotFieldNames = []string{"mode"}
	// settingsIdentityFieldNames 里的字段热换时必须重置该成员的 provider
	// 会话（进身份指纹）。前端靠同一张表在预览框里说「上下文会丢」。
	settingsIdentityFieldNames = []string{"display_name", "role", "provider", "model", "effort", "prompt", "permissions"}
	// settingsRestartFieldNames 当前为空：所有 agent 字段都能热换。它保留
	// 下来有两个用处——「其它成员在磁盘上变了」这类成员级判定仍然要用
	// agentRestartFieldDiffs 的顺序，以及将来加一个真正无法热换的字段时
	// 有地方放（加进来即可，不需要改保存流程）。
	settingsRestartFieldNames []string
)

// settingsRestartLabels 是字段名到给人看的原因短语的映射。字段分级表与
// 标签放在同一个文件里：加一个必须重启的字段时，漏掉标签会在这里得到
// 一个显眼的 fallback（"<field> 变了"），而不是静默少一条原因。
//
// display_name 与 role 共用一个标签：它们对用户是同一件事（这个人是谁），
// 分开报两行 "display_name 变了、role 变了" 只会让原因列表变长。
var settingsRestartLabels = map[string]string{
	"display_name": "名称或角色变了",
	"role":         "名称或角色变了",
	"provider":     "provider 变了",
	"model":        "model 变了",
	"effort":       "effort 变了",
	"prompt":       "prompt 变了",
	"permissions":  "permissions 变了",
}

// settingsReloadJSON 是 GET /api/settings 里的分级表。role / prompt 不在
// 设置页表单里，但仍然列出来：将来表单补上它们时，前端不需要再改判定逻辑。
type settingsReloadJSON struct {
	HotFields []string `json:"hot_fields"`
	// IdentityFields 是「热更但要重置会话」的字段。前端预览框据此把
	// 「上下文不再续接」提前说出来；缺了它，用户会在保存之后才发现会话没了。
	IdentityFields []string `json:"identity_fields"`
	RestartFields  []string `json:"restart_fields"`
}

// settingsReloadPolicy 返回分级表。每次都造新切片：调用方（JSON 编码）
// 不会拿到可以被别处改掉的共享底层数组。
func settingsReloadPolicy() settingsReloadJSON {
	// 用非 nil 的空切片而不是 append(nil, ...)：某张表为空时 JSON 必须是
	// []（前端据此知道「服务端明确说了这一类没有字段」），不能是 null
	// （那会被前端当成「旧响应没给」而退回内置兜底表）。
	return settingsReloadJSON{
		HotFields:      append([]string{}, settingsHotFieldNames...),
		IdentityFields: append([]string{}, settingsIdentityFieldNames...),
		RestartFields:  append([]string{}, settingsRestartFieldNames...),
	}
}

// settingsRestartReasons 比较 RunCfg 与刚写盘的新配置，返回「为什么必须
// 重启」的中文短语。空切片表示这次改动只碰了可热更字段（含身份字段），
// 可以热换。
//
// 除了目标成员，还比较其它成员与顶层配置：WriteAgentSection 只改目标段，
// 但合并是从磁盘重新读的——运行期间有人手改了另一份配置文件时，那份改动
// 会随这次保存一起进入新配置。把它一起热换上等于替别人做了一次隐形 reload；
// 报出来、要求重启，比静默生效安全。
func settingsRestartReasons(runCfg, next *config.Config, id string) []string {
	if runCfg == nil || next == nil {
		return []string{"没有可比较的运行配置"}
	}
	reasons := []string{}

	oldAgent, hadOld := runCfg.Agents[id]
	newAgent, hasNew := next.Agents[id]
	switch {
	case !hadOld && hasNew:
		// 设置页新增成员走这条路：Runner 没有这个 id，只能重启后可用。
		// 它不是身份字段的热换场景——没有历史会话可重置，也还没有执行配置。
		reasons = append(reasons, "新增成员 "+id)
	case hadOld && !hasNew:
		reasons = append(reasons, "删除了成员 "+id)
	case hadOld && hasNew:
		reasons = append(reasons, agentRestartReasons(oldAgent, newAgent)...)
	}

	if !reflect.DeepEqual(configWithoutAgents(runCfg), configWithoutAgents(next)) {
		reasons = append(reasons, "配置的其它部分（providers / teams / summary / data_dir / language）变了")
	}

	// id 集合取并集并排序，让同一份差异每次得到同样顺序的原因列表。
	ids := make(map[string]struct{}, len(runCfg.Agents)+len(next.Agents))
	for other := range runCfg.Agents {
		ids[other] = struct{}{}
	}
	for other := range next.Agents {
		ids[other] = struct{}{}
	}
	others := make([]string, 0, len(ids))
	for other := range ids {
		if other != id {
			others = append(others, other)
		}
	}
	sort.Strings(others)
	for _, other := range others {
		if !reflect.DeepEqual(runCfg.Agents[other], next.Agents[other]) {
			reasons = append(reasons, fmt.Sprintf("成员 %s 的配置在磁盘上变了", other))
		}
	}
	return reasons
}

// agentFieldDiffers 是字段名到比较逻辑的唯一映射。分级表只写字段名，
// 比较逻辑在这里：两边分开写的话，加字段时很容易把名字加进表却忘了比较
// （或反过来），而那种错不会在编译期报出来。
func agentFieldDiffers(field string, oldAgent, newAgent config.AgentConfig) bool {
	switch field {
	case "display_name":
		return oldAgent.DisplayName != newAgent.DisplayName
	case "role":
		return oldAgent.Role != newAgent.Role
	case "provider":
		return oldAgent.Provider != newAgent.Provider
	case "model":
		return oldAgent.Model != newAgent.Model
	case "effort":
		return oldAgent.Effort != newAgent.Effort
	case "prompt":
		return oldAgent.Prompt != newAgent.Prompt
	case "permissions":
		return oldAgent.Permissions != newAgent.Permissions
	case "mode":
		return oldAgent.Mode != newAgent.Mode
	default:
		// 分级表里出现未知字段时按「有差异」处理：宁可要求重启，也不
		// 静默放行一个没人分析过的字段。
		return true
	}
}

// agentRestartFieldDiffs 按 settingsRestartFieldNames 的顺序返回有差异的
// 字段名（不是原因短语）。设置页的待重启字段展示与保存判定共用这一份。
func agentRestartFieldDiffs(oldAgent, newAgent config.AgentConfig) []string {
	var fields []string
	for _, field := range settingsRestartFieldNames {
		if agentFieldDiffers(field, oldAgent, newAgent) {
			fields = append(fields, field)
		}
	}
	return fields
}

// agentIdentityFieldDiffs 按 settingsIdentityFieldNames 的顺序返回有差异的
// 身份字段名。保存流程用它决定「热换之后要不要重置这个成员的会话」，
// 写盘之前也用它决定「有轮次在跑时要不要直接拒绝」。
func agentIdentityFieldDiffs(oldAgent, newAgent config.AgentConfig) []string {
	var fields []string
	for _, field := range settingsIdentityFieldNames {
		if agentFieldDiffers(field, oldAgent, newAgent) {
			fields = append(fields, field)
		}
	}
	return fields
}

// agentRestartReasons 把字段差异翻译成原因短语。display_name 与 role
// 共用一个标签，去重时保留第一次出现的顺序；映射漏项时仍然报出字段名，
// 不允许一条真实差异因为少写了一行 map 而消失。
func agentRestartReasons(oldAgent, newAgent config.AgentConfig) []string {
	fields := agentRestartFieldDiffs(oldAgent, newAgent)
	reasons := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		label, ok := settingsRestartLabels[field]
		if !ok {
			label = field + " 变了"
		}
		if _, dup := seen[label]; dup {
			continue
		}
		seen[label] = struct{}{}
		reasons = append(reasons, label)
	}
	return reasons
}

// settingsHotChanged 报告目标成员有没有「能热换的差异」：可热更字段或身份
// 字段。保存流程只在它为真时才尝试热换；它同时是「运行配置没有变化」那条
// 纯格式保存分支的判据。
func settingsHotChanged(runCfg, next *config.Config, id string) bool {
	if runCfg == nil || next == nil {
		return false
	}
	oldAgent, hadOld := runCfg.Agents[id]
	newAgent, hasNew := next.Agents[id]
	if !hadOld || !hasNew {
		// 新增 / 删除已经在 settingsRestartReasons 里变成重启原因，
		// 走到这里只表示「没有可热的成员差异」。
		return false
	}
	if len(agentIdentityFieldDiffs(oldAgent, newAgent)) > 0 {
		return true
	}
	for _, field := range settingsHotFieldNames {
		if agentFieldDiffers(field, oldAgent, newAgent) {
			return true
		}
	}
	return false
}

// settingsIdentityChanged 报告目标成员有没有身份字段差异——也就是热换之后
// 必须重置 provider 会话的那一类。判定基准必须是 RunCfg（Runner 手里那份），
// 不是用户表单的旧值：上一次保存可能已经把展示配置写新了而 Runner 没换。
func settingsIdentityChanged(runCfg, next *config.Config, id string) bool {
	if runCfg == nil || next == nil {
		return false
	}
	oldAgent, hadOld := runCfg.Agents[id]
	newAgent, hasNew := next.Agents[id]
	if !hadOld || !hasNew {
		return false
	}
	return len(agentIdentityFieldDiffs(oldAgent, newAgent)) > 0
}

// settingsAgentAfterSave 推算「这次请求写盘之后，合并配置里这个成员会是什么
// 样」。role / prompt 不在表单里：目标文件已有这一段时那两行原样保留，
// 没有这一段时整条覆盖要从 fallback（当前生效的那一份）带过去——这与
// config.renderAgentSection 的规则逐字一致。
//
// 只用于写盘之前的那道判断（settingsSaveResetsSessions）。写盘之后一律以
// 合并出来的新配置为准，不拿这个推算值当事实。
func settingsAgentAfterSave(agent, fallback config.AgentConfig) config.AgentConfig {
	if agent.Role == "" {
		agent.Role = fallback.Role
	}
	if agent.Prompt == "" {
		agent.Prompt = fallback.Prompt
	}
	return agent
}

// settingsSaveResetsSessions 回答「按这次请求写盘之后，目标成员的身份字段
// 会不会与 Runner 当前那份不同」——也就是「这次保存会不会重置它的会话」。
//
// RunCfg 里没有这个 id 时返回 false：新增成员没有历史会话可重置，走既有的
// 「新增成员需要重启」分支。这里只比身份字段，不比 mode：mode 改动既不需要
// 重置会话，也不需要为它拦下一次保存。
func settingsSaveResetsSessions(run *config.Config, id string, next config.AgentConfig) bool {
	if run == nil {
		return false
	}
	old, ok := run.Agents[id]
	if !ok {
		return false
	}
	return len(agentIdentityFieldDiffs(old, next)) > 0
}

// settingsPendingRestart 回答「展示配置里这个成员有什么改动还没进 RunCfg」。
// runnable=false 表示 RunCfg 里根本没有这个 id（设置页新增、还没重启）。
//
// 只有「必须重启」类字段进这份列表。身份字段改动现在是热换的（代价是重置
// 会话），不该被画成「待重启」；保存前预览的判定以服务端响应为准，这里
// 只补「上次保存被拒 / 失败、RunCfg 落后于展示配置」留下的那一份。
func settingsPendingRestart(run, display *config.Config, id string) (runnable bool, fields []string) {
	if run == nil {
		return false, nil
	}
	oldAgent, ok := run.Agents[id]
	if !ok {
		return false, nil
	}
	current, ok := display.Agents[id]
	if !ok {
		return true, nil
	}
	return true, agentRestartFieldDiffs(oldAgent, current)
}

// configWithoutAgents 拷一份去掉 Agents 的配置，用来比较顶层字段。
// 浅拷贝够用：这里只读，不写 map / 切片内部。
func configWithoutAgents(cfg *config.Config) config.Config {
	clone := *cfg
	clone.Agents = nil
	return clone
}

// settingsHotReload 把新配置热换给 RunCfg 与 Runner；resetSessions 为真时
// 先重置 id 这个成员的 provider 会话，返回删掉的行数。
//
// 并发保护：调用方已经在 settingsMu 里（设置页写盘串行），这里再抢
// reloadMu。handleAsk 把 job 放进任务表时抢同一把锁，于是「检查没有
// 在跑的轮次」与「换 Runner 配置 / 删会话」之间不可能插进一个新 job；
// 反过来，若检查时已有 job，就明确放弃热换（返回 errSettingsJobsRunning），
// 由调用方退化成「需要重启」，并且一行会话都不删。job 的 finished 状态
// 只在 headless.Run 返回之后才置上，所以此刻没有任何一轮还在用旧配置。
//
// 身份字段的顺序是「清内存 -> 删库 -> 换配置」，与任务书列的顺序相反。
// 理由：三段里只有 SetConfig 有失败出口（生产默认实现不会失败，测试会注入
// 一个会失败的 setter）。若先换配置再删会话，删除失败就留下「新身份 +
// 旧会话」，下一轮必然 ErrIdentityDrift；反过来，会话重置成功而 SetConfig
// 失败只损失一次上下文，成员下一轮按旧身份新建会话照常能用。两害相权，
// 把不可回滚的那一步放在会失败的那一步之前。
//
// 失败回滚：SetConfig 报错时把 RunCfg 换回旧指针。文件已经写盘，展示配置
// Cfg 也已经是新的，这两样都不回滚——调用方必须在 500 文案里说清「配置已
// 写入，但热加载失败，重启后可生效」；若会话已经重置过，还要说出那件事。
func (s *Server) settingsHotReload(ctx context.Context, cfg *config.Config, id string, resetSessions bool) (int, error) {
	if s.write == nil || cfg == nil {
		return 0, errors.New("没有可热换的写模式配置")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	if running := s.runningJobs(); running > 0 {
		return 0, fmt.Errorf("%w：有 %d 个 job 尚未结束", errSettingsJobsRunning, running)
	}

	// setter 必须在重置会话之前确定：没有可热换的 Runner 时直接退化成
	// 「需要重启」，不能先把会话删了再发现换不了配置。
	setter := s.write.SetRunnerConfig
	if setter == nil {
		if s.write.Rn == nil {
			return 0, errSettingsNoRunner
		}
		rn := s.write.Rn
		setter = func(next *config.Config) error {
			rn.SetConfig(next)
			return nil
		}
	}

	sessionsReset := 0
	if resetSessions {
		if s.write.Rn != nil {
			// 内存快照与库里的行是两份状态，缺一不可：只删库的话，
			// 进程内那条旧快照会让下一轮在拿闸之后直接撞 ErrIdentityDrift，
			// 根本不会回头读库（见 turn/agent_reset.go）。
			if _, err := s.write.Rn.ForgetAgentSessions(id); err != nil {
				// 有闸拿不到说明还有一轮在飞。与 runningCount 同一处置：
				// 退化成「等它结束再保存」，什么都不删。
				return 0, fmt.Errorf("%w：成员 %s 还有一轮在飞（%v）", errSettingsJobsRunning, id, err)
			}
		}
		if s.store == nil {
			return 0, fmt.Errorf("%w：当前进程没有可写的本机库", errSettingsSessionReset)
		}
		n, err := s.store.ResetProviderSessions(ctx, id)
		if err != nil {
			return 0, fmt.Errorf("%w（成员 %s）: %v", errSettingsSessionReset, id, err)
		}
		sessionsReset = n
	}

	oldCfg := s.runCfg()
	s.cfgMu.Lock()
	s.write.RunCfg = cfg
	s.cfgMu.Unlock()
	if err := setter(cfg); err != nil {
		s.cfgMu.Lock()
		s.write.RunCfg = oldCfg
		s.cfgMu.Unlock()
		if sessionsReset > 0 {
			return sessionsReset, fmt.Errorf(
				"设置热加载失败，已回滚到旧的 Runner 配置；成员 %s 的 %d 条 provider 会话已按新配置重置: %w",
				id, sessionsReset, err)
		}
		return 0, fmt.Errorf("设置热加载失败，已回滚到旧的 Runner 配置: %w", err)
	}
	return sessionsReset, nil
}

// runningJobs 返回任务表里尚未结束的 job 数。s.jobs 为 nil（只读模式或
// 直接构造 Server 的测试）时没有轮次在跑。
func (s *Server) runningJobs() int {
	if s.jobs == nil {
		return 0
	}
	return s.jobs.runningCount()
}

var (
	// errSettingsJobsRunning 表示检查时还有 job 在跑。调用方把它翻译成
	// reloaded=false + restart_reasons，而不是 500：这是正常的保护性退化。
	errSettingsJobsRunning = errors.New("有轮次正在运行")
	// errSettingsNoRunner 表示装配里没有 Runner，无法热换。同样退化成
	// 「需要重启」，不是服务端故障。
	errSettingsNoRunner = errors.New("当前进程没有可热换的 Runner")
	// errSettingsSessionReset 表示删 provider_sessions 失败。它发生在
	// SetConfig 之前：RunCfg 与 Runner 都还是旧的，配置只写在磁盘上。
	// 调用方的 500 文案要说清「没有热加载」，不能套用「已回滚」那句
	// ——这里根本没换过。
	errSettingsSessionReset = errors.New("重置成员 provider 会话失败")
)

// settingsRestartNote 是「必须重启」时的 note：先点名原因，再保留原来的
// 那句「Runner 不会重建」。既有界面与测试都依赖 note 里出现「重启」。
func settingsRestartNote(reasons []string) string {
	if len(reasons) == 0 {
		return settingsReloadNote
	}
	return "本次改动需要重启 council serve 后生效：" + strings.Join(reasons, "、") + "。" + settingsReloadNote
}

// settingsHotReloadNote 是热换成功、且没有重置会话时的 note。成功之后
// 不再说「重启」，否则前端会把已经生效的改动又说成必须重启。
const settingsHotReloadNote = "配置已写入并已热加载：本次改动立即生效，无需重启 council serve"

// settingsHotReloadResetNote 是身份字段热换成功时的 note。必须让用户看到
// 代价：provider 会话被重置、上下文不再续接。计数器放在最后，主句保持
// 与界面文案逐字一致（测试与前端都按那句话对）。
func settingsHotReloadResetNote(reset int) string {
	if reset == 0 {
		return "已热加载；本次改动立即生效，无需重启 council serve；该成员还没有历史 provider 会话（重置 0 条记录），下一轮按新配置建立新会话。"
	}
	return fmt.Sprintf("已热加载；该成员的 provider 会话已重置，下一轮用新模型重新开始（重置 %d 条记录）。", reset)
}

// settingsRuntimeUnchangedNote 用于「文件字节变了、但合并后的运行配置与
// RunCfg 逐字段相同」这一种保存（例如给本来就用默认值 mode 的段补上显式
// mode 行）。它既不是热加载，也不需要重启。
const settingsRuntimeUnchangedNote = "配置已写入；运行配置没有变化，无需重启 council serve"
