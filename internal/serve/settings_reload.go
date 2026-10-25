package serve

// settings_reload.go 实现 docs/29_FIXES_PLAN.md A2 的后半段：
// 设置页保存之后，只有不改变身份指纹的字段才当场热换；其余一律要求重启。
//
// 分级表（字段名与 settingsAgentRequest / 配置文件一致）：
//
//   - 可热更：mode。它只决定 PlanRound 里这个人发言还是当裁判，既不进
//     renderIdentityCore（身份文本只拼 DisplayName / Provider / Role），
//     也不进 identityFPAt 的 probe 请求，因此改 mode 不会让已有 provider
//     会话的身份指纹变化。turn 包的
//     TestPlanRoundModeChangeMovesMemberButNotFingerprint 已经钉住这一点；
//   - 必须重启：display_name、role、provider、model、effort、prompt、
//     permissions。前两项看着只是展示，实际会进 turn/render.go 的
//     renderIdentityCore（"You are <DisplayName> ..." 与 Role 正文），
//     再进 IdentityFingerprint。Runner 里已经有 in-process provider 会话时，
//     热换这两项会让下一轮直接撞 ErrIdentityDrift，成员突然不能说话；
//     这比多等一次重启糟得多。provider / model / effort / prompt /
//     permissions 同样换前缀或权限语义，也必须重启。
//
// 所以热换只在「本次改动全部落在这个很窄的热更列表里」时发生。若将来
// 连 mode 也进了指纹，把 settingsHotFieldNames 清空即可：没有热更字段时
// 所有真实改动都会落在重启列表，settingsHotChanged 恒为 false，保存走
// settingsRuntimeUnchangedNote 或重启分支，不会留下一条永远进不去的分支。
//
// 判定基准是 RunCfg（Runner 真正在用的那一份），不是展示配置 Cfg：
// 上一次「改 model 但没重启」之后，展示配置已经比 RunCfg 新；此时再改
// mode 若只和展示配置比，就会把上一次的 model 改动一起热换进去。

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"council/internal/config"
)

// settingsHotFieldNames / settingsRestartFieldNames 是唯一一份分级表：
// GET /api/settings 把它交给前端做保存前预览，保存时的判定也用它。
// 两处各写一份的话，预览说「立即生效」而服务端回「需要重启」只是时间问题。
var (
	settingsHotFieldNames     = []string{"mode"}
	settingsRestartFieldNames = []string{"display_name", "role", "provider", "model", "effort", "prompt", "permissions"}
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
	HotFields     []string `json:"hot_fields"`
	RestartFields []string `json:"restart_fields"`
}

// settingsReloadPolicy 返回分级表。每次都造新切片：调用方（JSON 编码）
// 不会拿到可以被别处改掉的共享底层数组。
func settingsReloadPolicy() settingsReloadJSON {
	// 用非 nil 的空切片而不是 append(nil, ...)：热更列表为空时 JSON 必须
	// 是 []（前端据此知道「服务端明确说了没有热更字段」），不能是 null
	// （那会被前端当成「旧响应没给」而退回内置兜底表）。
	return settingsReloadJSON{
		HotFields:     append([]string{}, settingsHotFieldNames...),
		RestartFields: append([]string{}, settingsRestartFieldNames...),
	}
}

// settingsRestartReasons 比较 RunCfg 与刚写盘的新配置，返回「为什么必须
// 重启」的中文短语。空切片表示这次改动只碰了可热更字段，可以热换。
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
		reasons = append(reasons, "新增成员 "+id)
	case hadOld && !hasNew:
		reasons = append(reasons, "删除了成员 "+id)
	case hadOld && hasNew:
		reasons = append(reasons, agentRestartReasons(oldAgent, newAgent)...)
	}

	if !reflect.DeepEqual(configWithoutAgents(runCfg), configWithoutAgents(next)) {
		reasons = append(reasons, "配置的其它部分（providers / teams / summary / data_dir）变了")
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

// settingsHotChanged 报告目标成员有没有「落在热更列表里」的差异。
// 保存流程只在它为真时才尝试热换：热更列表被清空（所有字段都必须重启）
// 时它恒为假，一次只改文件字节、合并后配置没变的保存不会假装热加载过。
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
	for _, field := range settingsHotFieldNames {
		if agentFieldDiffers(field, oldAgent, newAgent) {
			return true
		}
	}
	return false
}

// settingsPendingRestart 回答「展示配置里这个成员有什么改动还没进 RunCfg」。
// runnable=false 表示 RunCfg 里根本没有这个 id（设置页新增、还没重启）。
//
// 保存前的预览必须带上这些字段：上一次改 model 没重启，这一次只改
// display_name，服务端按 RunCfg 判定仍然要重启；预览若只看表单新旧值，
// 会先说「立即生效」、保存后又翻成「需要重启」。
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

// settingsHotReload 把新配置热换给 RunCfg 与 Runner。
//
// 并发保护：调用方已经在 settingsMu 里（设置页写盘串行），这里再抢
// reloadMu。handleAsk 把 job 放进任务表时抢同一把锁，于是「检查没有
// 在跑的轮次」与「换 Runner 配置」之间不可能插进一个新 job；反过来，
// 若检查时已有 job，就明确放弃热换（返回 errSettingsJobsRunning），
// 由调用方退化成「需要重启」。job 的 finished 状态只在 headless.Run
// 返回之后才置上，所以此刻没有任何一轮还在用旧配置。
//
// 失败回滚：SetRunnerConfig 报错时把 RunCfg 换回旧指针。文件已经写盘，
// 展示配置 Cfg 也已经是新的，这两样都不回滚——调用方必须在 500 文案里
// 说清「配置已写入，但热加载失败，重启后可生效」。
func (s *Server) settingsHotReload(cfg *config.Config) error {
	if s.write == nil || cfg == nil {
		return errors.New("没有可热换的写模式配置")
	}
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	if running := s.runningJobs(); running > 0 {
		return fmt.Errorf("%w：有 %d 个 job 尚未结束", errSettingsJobsRunning, running)
	}

	setter := s.write.SetRunnerConfig
	if setter == nil {
		if s.write.Rn == nil {
			return errSettingsNoRunner
		}
		rn := s.write.Rn
		setter = func(next *config.Config) error {
			rn.SetConfig(next)
			return nil
		}
	}

	oldCfg := s.runCfg()
	s.cfgMu.Lock()
	s.write.RunCfg = cfg
	s.cfgMu.Unlock()
	if err := setter(cfg); err != nil {
		s.cfgMu.Lock()
		s.write.RunCfg = oldCfg
		s.cfgMu.Unlock()
		return fmt.Errorf("设置热加载失败，已回滚到旧的 Runner 配置: %w", err)
	}
	return nil
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
)

// settingsRestartNote 是「必须重启」时的 note：先点名原因，再保留原来的
// 那句「Runner 不会重建」。既有界面与测试都依赖 note 里出现「重启」。
func settingsRestartNote(reasons []string) string {
	if len(reasons) == 0 {
		return settingsReloadNote
	}
	return "本次改动需要重启 council serve 后生效：" + strings.Join(reasons, "、") + "。" + settingsReloadNote
}

// settingsHotReloadNote 是热换成功时的 note。成功之后不再说「重启」，
// 否则前端会把已经生效的改动又说成必须重启。
const settingsHotReloadNote = "配置已写入并已热加载：本次改动立即生效，无需重启 council serve"

// settingsRuntimeUnchangedNote 用于「文件字节变了、但合并后的运行配置与
// RunCfg 逐字段相同」这一种保存（例如给本来就用默认值 mode 的段补上显式
// mode 行）。热更列表为空时这条路径会更常见：所有真实字段差异都会走重启
// 分支，只剩这种纯格式变化。它既不是热加载，也不需要重启。
const settingsRuntimeUnchangedNote = "配置已写入；运行配置没有变化，无需重启 council serve"
