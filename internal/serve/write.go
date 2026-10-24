package serve

// write.go 是 SRV4 的写模式：`council serve -rw` 的库装配、任务表、
// POST /api/ask 与 SSE。
//
// 这里刻意不碰轮次、预算与汇总：那些全部交给 internal/headless.Run，
// 本文件只做三件事——把 HTTP 请求翻译成 headless.Options、把 OnStart /
// OnRecord 回调搬进内存事件表、按 SSE 协议写给浏览器。任何一处开始自己
// 计算「该问谁」「花多少钱」「要不要汇总」，都意味着 serve 与 `council ask`
// 走上了两条会各自漂移的路。

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/headless"
	"council/internal/repl"
	"council/internal/store"
	"council/internal/turn"
)

// defaultJobTTL 是任务结束后事件表保留的时长。任务书 §3 定的是 60 秒：
// 浏览器连上得晚一点、或者断线重连一次，仍然拿得到完整历史；更久则没有
// 意义——job id 只活在一个页面会话里，60 秒足够覆盖 EventSource 的重连退避。
const defaultJobTTL = 60 * time.Second

// 关闭服务时留给在跑议会的两段时间。先等 10 秒自然收尾；到点还没完就
// 显式取消任务（jobCtx），再给 2 秒让它把取消写进库。两段都超时仍然返回，
// 但这时是「明确取消」而不是「关库之后悄悄丢掉」。
//
// 用变量而不是常量，是为了让测试能把它压到毫秒级验证同一条路径；
// 生产值就是任务书复审要求的 10s / 2s。
var (
	jobDrainTimeout = 10 * time.Second
	jobDrainGrace   = 2 * time.Second
)

// maxAskBody 是 POST /api/ask 请求体的上限。问题正文是普通文本，1 MiB
// 已经远超任何正常提问；设上限是为了不让人把一个流式请求挂在这里。
const maxAskBody = 1 << 20

const (
	// maxContextPaths / maxContextPathBytes 是任务书 §2 对 context 的两道
	// 硬上限。它们是请求校验，不是执行限制：超了直接 400，不截断——
	// 悄悄扔掉第 21 条会让用户以为成员看到了全部资料。
	maxContextPaths     = 20
	maxContextPathBytes = 512
)

// WriteDeps 是写模式需要的全部装配结果。它由 serve.Run 在拿到排他锁、
// 加载配置与规则之后填，与 internal/app/app.go 的装配逐项对应
// （docs/21d_SRV4_M2_TASK.md §2）。
type WriteDeps struct {
	Cfg     *config.Config
	Svc     *conversation.Service
	Rn      *turn.Runner
	DataDir string
	// ProjectDir 与 app.Run 里那个同义：从服务进程的启动目录规范化而来，
	// 新会话据此归属项目，CLI 的 /resume 才能按同一个项目把它们列出来。
	ProjectDir string
	// ConfigPath 是本次启动实际解析到的配置文件绝对路径，只给
	// GET /api/options 回给前端显示。空串表示装配方没有提供（直接构造
	// Server 的测试），响应里就是空串，不拿默认路径去猜。
	ConfigPath string
	// Cwd 是 serve 进程的启动目录，也是 /api/ask 不传 cwd 时的默认值。
	// 在 runWritable 里取一次存下来：请求处理期间再调 os.Getwd 拿到的是
	// 同一件事，但进程若被别的代码 chdir 过（本仓库没有，防的是下一个人），
	// 两次请求之间默认值就会漂。
	Cwd string
	// ClaudeAgents 是从 Claude Code 的 .claude/agents 只读导入的成员
	// （key 是 frontmatter 的 name）。生产装配下它们已经合并进 Cfg.Agents；
	// 启动时由 runWritable 扫一次放进这里，只活在内存里，不写回配置。
	// /api/options 需要它来标 source = "claude"：合并之后 Cfg.Agents 里
	// 配置成员与导入成员混在一起，单看那张表已经分不出来源。
	// 直接构造 WriteDeps 的测试不填这几项，行为与以前一致。
	ClaudeAgents map[string]config.AgentConfig
	// ClaudeAgentsMerged 是 ClaudeAgents 的 id 集合，标记哪些 id 是本次启动
	// 合并进来的。合并后它们也在 Cfg.Agents 里，/api/options 靠这一项把
	// 它们画成 source = "claude" 而不是误标成配置成员。
	ClaudeAgentsMerged map[string]struct{}
	// ClaudeAgentsWithoutProvider 是扫到了、但因为配置缺 [providers.claude]
	// 而整体跳过的 Claude agents。它们不在 Cfg.Agents 里、也不出现在
	// /api/options；留着是为了 /api/ask 点到它们时能把「缺 provider」这个
	// 真正的根因说清楚，而不是只回一句「未知成员」。
	ClaudeAgentsWithoutProvider map[string]config.AgentConfig
	// Skills 是从 Claude Code 的 .claude/skills 只读扫到的 skill 元数据。
	// 与 ClaudeAgents 不同，它不合并进 Cfg.Agents、不执行、不注入提示，
	// 只原样交给 /api/options 的 skills 字段展示。扫描不依赖
	// [providers.claude]：没有 provider 也能看到仓库里有哪些 skill。
	Skills []config.Skill
}

// EnableWrite 把 Server 切到写模式。调用方必须先打开可写库、装配好
// Service 与 Runner；本方法只保存引用并建任务表，不做任何装配工作，
// 因为「怎么装配」属于 serve.Run 的职责，不属于 HTTP handler。
func (s *Server) EnableWrite(deps WriteDeps) {
	s.write = &deps
	s.jobs = newJobTable()
	// 后台议会的父 ctx 跟着 Server 的生命周期，不跟任何一条 HTTP 请求。
	s.jobCtx, s.cancelJobs = context.WithCancel(context.Background())
}

// drainJobs 等所有后台议会结束，供 listenAndServe 在 HTTP Shutdown 之后
// 调用。ctx 的期限就是「自然等待」的上限（生产 10 秒）：
//   - 期限前全部结束 → 取消 jobCtx 释放资源，返回 nil；
//   - 到期还没完 → 显式 cancelJobs()，再给 jobDrainGrace（生产 2 秒）让
//     取消写进库；这段宽限也过了才返回错误。
//
// 这样关闭时最坏的结果是「明确取消一轮」，而不是「关库之后那一轮静默
// 消失在数据库错误里」。
func (s *Server) drainJobs(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.jobsWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		if s.cancelJobs != nil {
			s.cancelJobs()
		}
		return nil
	case <-ctx.Done():
		if s.cancelJobs != nil {
			s.cancelJobs()
		}
		grace := time.NewTimer(jobDrainGrace)
		defer grace.Stop()
		select {
		case <-done:
			return nil
		case <-grace.C:
			return fmt.Errorf("等待运行中的议会结束超时（已取消）")
		}
	}
}

// runWritable 是 `council serve -rw` 的进程前半段：加载完整配置与规则 ->
// store.Open（拿 council.lock、跑迁移）-> 照 app.Run 装配 Service/Runner ->
// 交给与只读模式共用的 HTTP 生命周期。
func runWritable(ctx context.Context, opts Options, version string, out, errOut io.Writer) error {
	wc, err := loadWriteConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	if opts.Verbose {
		for _, note := range wc.notices {
			fmt.Fprintf(errOut, "council serve: %s\n", note)
		}
	}
	cfg, rules, dataDir, configPath := wc.cfg, wc.rules, wc.dataDir, wc.path

	st, err := store.Open(ctx, dataDir)
	if err != nil {
		// 任务书 §2 要求错误里出现这两句话：一句说事实（谁占着目录），
		// 一句说下一步（关掉再试）。store.ErrDataDirLocked 自己的英文
		// 文本也保留在 %w 里，排查时还能看到锁文件路径与系统错误。
		if errors.Is(err, store.ErrDataDirLocked) {
			return fmt.Errorf("另一个 council 正在用这个 data_dir（%s）；关掉它再试: %w", dataDir, err)
		}
		return fmt.Errorf("打开本地库失败 (%s): %w", dataDir, err)
	}
	// 关闭会同时释放 council.lock。失败必须说出来：锁没放干净，下一次
	// 启动会被自己上一次的进程挡住，而错误现场已经离开了。
	defer func() {
		if cerr := st.Close(); cerr != nil {
			fmt.Fprintf(errOut, "council serve: 关闭本地库失败: %v\n", cerr)
		}
	}()

	// 启动目录决定新会话的项目归属，与 CLI 用同一个 NormalizeProjectDir，
	// 网页里建的会话才能被同一个项目下的 /resume 看到。
	// 同一个值也是工作台的默认 cwd（/api/options 的 cwd、/api/ask 不传
	// cwd 时成员跑的那个目录），所以只取一次、两处共用。
	projectDir := ""
	cwd := ""
	if wd, wdErr := os.Getwd(); wdErr == nil {
		cwd = wd
		projectDir = repl.NormalizeProjectDir(wd)
	}

	// Claude Code 的 agents 是只读外部来源：启动时扫一次，并在运行时合并
	// 进内存里的 cfg.Agents（不写任何配置文件）。扫的是 serve 进程的启动
	// 目录（与项目归属同一个 cwd），不是每个请求的 cwd —— 工作台的 cwd 可以
	// 按请求变，成员列表不会跟着变，前端拿到的 options 才是稳定的。
	//
	// 必须放在 turn.NewRunner 之前：Runner 持有 cfg 指针，之后原地改
	// Agents 会破坏它「交给 Runner 之后配置不再变」的不变量（见
	// internal/turn/runner.go 的 SetConfig 注释）。
	claude := importClaudeAgents(cfg, cwd)
	if claude.warning != "" {
		// 缺 provider 是一个可处理的配置问题，不藏在 -verbose 后面。
		fmt.Fprintf(errOut, "council serve: %s\n", claude.warning)
	}
	for _, warning := range claude.warnings {
		// 按单个成员跳过（目前只有超长 prompt）同样不藏：用户在 /api/options
		// 里找不到这个成员，必须能在启动日志里看到跳过原因。
		fmt.Fprintf(errOut, "council serve: %s\n", warning)
	}
	if opts.Verbose {
		for _, note := range claude.notices {
			fmt.Fprintf(errOut, "council serve: %s\n", note)
		}
	}
	// skills 只做展示：启动时扫一次，不合并进 cfg.Agents、不执行、不注入任何
	// 提示，也不写文件。扫描与 agents 相互独立——没有 [providers.claude] 时
	// skills 仍然值得列出来，因为它根本不参与执行。
	skills := config.ScanClaudeSkills(cwd)
	// 外部系统提示是安全相关的可见性：哪些成员带着一段不是 Council 写的
	// 系统提示在跑，必须在启动时看得见，不藏在 -verbose 后面（任务书 §4）。
	printClaudePromptNotice(errOut, cfg)

	// 以下装配与 internal/app/app.go 的 write 路径同源。thought 策略必须
	// 接上，否则 store_thought=false 的配置在网页里跑议会时会静默把
	// thought 存进库——同一个配置在 CLI 与网页下产生不同的事实源。
	svc := conversation.NewService(st)
	svc.SetThoughtPolicy(cfg.StoreThought)
	rn := turn.NewRunner(cfg, svc, filepath.Join(dataDir, "workdir"), turn.RulesBlocks{
		Global: rules.GlobalText,
		Local:  rules.LocalText,
	})

	// 启动对账不能省：上一次进程崩在一轮中间时，inflight_turns 里会留着
	// 一行，Runner 的在飞闸会把此后所有同 (会话, 成员) 的 ask 都拒掉
	// （conversation.ErrInflightConflict）。TUI 靠 app.Run 的启动对账清掉
	// 这些行，serve -rw 既然接管了同一个库，就必须做同一件事。
	// 失败只提示、不阻止启动：对账本身读不了库时，看板仍值得开着。
	reconcileLostTurns(ctx, rn, errOut)

	srv := NewServer(st, version, modelsFromConfig(cfg))
	srv.Verbose = opts.Verbose
	srv.EnableWrite(WriteDeps{
		Cfg:                         cfg,
		Svc:                         svc,
		Rn:                          rn,
		DataDir:                     dataDir,
		ProjectDir:                  projectDir,
		ConfigPath:                  configPath,
		Cwd:                         cwd,
		ClaudeAgents:                claude.agents,
		ClaudeAgentsMerged:          claude.merged,
		ClaudeAgentsWithoutProvider: claude.withoutProvider,
		Skills:                      skills,
	})
	// drainJobs 必须在 st.Close() 之前跑完：runWritable 的 defer 在
	// listenAndServe 返回后立刻关库放锁，而这时后台议会可能还在写。
	return listenAndServe(ctx, opts.Addr, srv.Handler(),
		browserOptions{App: opts.App, Open: opts.Open, DataDir: dataDir},
		out, errOut, srv.drainJobs)
}

// reconcileLostTurns 照 internal/app/app.go 的启动对账做一遍，只保留
// AuditInflight / SettleLostTurns 这一对：它们清掉的是会挡住后续 ask 的
// 陈旧在飞行，属于「写模式能不能正常跑」的一部分。
// 漂移审计只是给人看的提醒，不在这里重复一遍。
func reconcileLostTurns(ctx context.Context, rn *turn.Runner, errOut io.Writer) {
	lost, err := rn.AuditInflight(ctx)
	if err != nil {
		fmt.Fprintf(errOut, "council serve: 启动在飞对账失败: %v\n", err)
		return
	}
	if len(lost) == 0 {
		return
	}
	marked, errs := rn.SettleLostTurns(ctx, lost)
	for _, settleErr := range errs {
		fmt.Fprintf(errOut, "council serve: 启动在飞对账补账失败: %v\n", settleErr)
	}
	fmt.Fprintf(errOut, "council serve: 有 %d 轮在上次退出时没记上账，其中 %d 轮已把相关成员的成本基线标为可疑\n",
		len(lost), marked)
}

// writeConfig 是写模式启动前半段解析出来的全部东西。
// notices 是只给 Verbose 档的合并提示（例如项目层写了 data_dir 被忽略），
// 由 runWritable 负责打印：读取配置的代码不该知道详细档开没开。
type writeConfig struct {
	cfg     *config.Config
	rules   config.Rules
	dataDir string
	path    string
	notices []string
}

// loadWriteConfig 是写模式与只读模式的 resolveDataDir 分开的一份：写模式
// 需要完整配置（agents / providers / summary）与规则文件，缺一个都跑不了
// 议会，所以配置缺失必须直接报错，不能像只读模式那样回落到一个空
// models 表。错误文本沿用 app.Run 的措辞，用户在两处看到的是同一句话。
//
// 配置走合并版：项目层可以在不抄 provider 段的前提下只写 [agents.*] /
// [teams.*] / [summary]。规则文件的相对路径按「写 rules_file 的那一层」
// 解析（LoadMerged 交回 RulesFileBase），不是按最高优先层猜。
func loadWriteConfig(configPath string) (writeConfig, error) {
	layers, err := config.ResolveConfigLayers(configPath)
	if err != nil {
		return writeConfig{}, fmt.Errorf("无法确定配置文件位置: %w", err)
	}
	// ResolveConfigLayers 保证至少一条（一份都不存在时是全局位置的占位层）。
	primary := layers[0].Path
	if _, statErr := os.Stat(primary); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return writeConfig{}, fmt.Errorf(
				"配置文件不存在: %s（council serve -rw 需要完整配置：data_dir、agent 与 provider 命令）", primary)
		}
		return writeConfig{}, fmt.Errorf("查看配置文件 %s: %w", primary, statErr)
	}
	merged, err := config.LoadMerged(layers)
	if err != nil {
		return writeConfig{}, fmt.Errorf("加载配置失败 (%s): %w", primary, err)
	}
	rules, err := config.LoadRules(merged.RulesFileBase, merged.Config.RulesFile)
	if err != nil {
		return writeConfig{}, fmt.Errorf("加载规则文件失败: %w", err)
	}
	dataDir, err := config.ResolveDataDir(merged.Config.DataDir)
	if err != nil {
		return writeConfig{}, fmt.Errorf("解析数据目录失败: %w", err)
	}
	// primary 一并交回：GET /api/options 要让前端看到"我读的是哪份配置"，
	// 而 runWritable 手里只有这份解析结果，不该为了一个展示字段再解析一遍。
	// 合并后有多层时展示最高优先层（项目层）；完整层列表不在本片的界面里。
	return writeConfig{
		cfg:     merged.Config,
		rules:   rules,
		dataDir: dataDir,
		path:    primary,
		notices: merged.Notices,
	}, nil
}

// modelsFromConfig 给只读端点用的 agent id -> model 映射；写模式与只读
// 模式共用 NewServer，这个映射的口径必须一致。
func modelsFromConfig(cfg *config.Config) map[string]string {
	if cfg == nil {
		return nil
	}
	models := make(map[string]string, len(cfg.Agents))
	for id, ac := range cfg.Agents {
		models[id] = ac.Model
	}
	return models
}

// ---- GET /api/options ----

// agentOption 是工作台里一个可勾选的成员。permission / mode 取配置原文，
// 不在服务层翻译成中文或布尔：前端的权限闸与成员筛选都按配置的枚举值走，
// 翻译一次就多了一处会与 config 校验漂移的映射。
//
// Source 说明这条定义从哪来：配置里的 agent 是 "config"，从 Claude Code
// 的 .claude/agents 只读导入的是 "claude"。两者启动后都能被 /api/ask 选中
// （Claude 来源的已经合并进 cfg.Agents），但来源不同，前端据此把它们画得
// 不一样，而不是让用户去猜某一条为什么改不了。
type agentOption struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Effort      string `json:"effort"`
	Permission  string `json:"permission"`
	Mode        string `json:"mode"`
	// Active 表示"当前配置里可用"。配置里的 agent 就是启动时校验通过的
	// 全部成员，没有第二个启停状态，所以恒为 true；字段留着是因为前端
	// 要按它渲染禁用态，而"配置里没有"与"配置里有但停用"是两种画法。
	Active bool   `json:"active"`
	Source string `json:"source"`
}

// agentOption 的两个来源。用常量而不是散落的字面量：前端与用例都按这两个
// 值分支，拼错一个字符会变成一条永远匹配不上的规则。
const (
	agentSourceConfig = "config"
	agentSourceClaude = "claude"
)

func newAgentOption(id string, ac config.AgentConfig, source string) agentOption {
	return agentOption{
		ID:          id,
		DisplayName: ac.DisplayName,
		Provider:    ac.Provider,
		Model:       ac.Model,
		Effort:      ac.Effort,
		Permission:  string(ac.Permissions),
		Mode:        string(ac.Mode),
		Active:      true,
		Source:      source,
	}
}

type teamOption struct {
	ID      string   `json:"id"`
	Members []string `json:"members"`
}

// skillOption 是只读导入的一条 Claude Code skill。它只出现在 /api/options，
// 不能被 /api/ask 选中、不参与任何轮次：Council 这一片不执行 skills，也不
// 把正文注入提示。
//
// 字段与 agentOption 刻意分开：skill 没有 provider / model / 权限这些执行
// 期概念，硬塞进 agentOption 会让前端以为它也是一个可选成员。
type skillOption struct {
	// ID 是 .claude/skills 下的目录名（官方定义的身份），Name 是 frontmatter
	// 的 name（缺省等于目录名）。两者可以不同：name 覆盖的是命令名。
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

// skillSourceClaude 与 agentSourceClaude 是同一个来源词表（"claude"），
// 前端按同一套 source 值区分「外部只读导入」与「配置」。
const skillSourceClaude = agentSourceClaude

// optionsResponse 是 GET /api/options 的完整响应。所有切片都先做成非 nil：
// 空配置时前端应该拿到 []，不是 null。
type optionsResponse struct {
	Cwd          string        `json:"cwd"`
	ConfigPath   string        `json:"config_path"`
	SummaryAgent string        `json:"summary_agent"`
	Agents       []agentOption `json:"agents"`
	Teams        []teamOption  `json:"teams"`
	// Skills 是本片新增的只读展示字段。放在最后，避免改变既有 JSON 的
	// 字段顺序（前端与快照用例都可能按位置读）。
	Skills []skillOption `json:"skills"`
}

// handleOptions 一次交出工作台画界面需要的全部静态信息。
//
// 它只在 -rw 下可用：只读模式的 Server 没有 Cfg，也没有"这一轮能问谁"
// 的配置来源，返回一份空表比 405 更容易让前端误以为"配置里没有人"。
func (s *Server) handleOptions(w http.ResponseWriter, r *http.Request) {
	if s.write == nil {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed,
			"只读模式没有工作台接口：用 council serve -rw 启动才能跑议会")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "只支持 GET")
		return
	}

	resp := optionsResponse{
		Cwd:        s.workbenchCwd(),
		ConfigPath: s.write.ConfigPath,
		Agents:     []agentOption{},
		Teams:      []teamOption{},
		Skills:     []skillOption{},
	}
	cfg := s.write.Cfg
	if cfg != nil {
		resp.SummaryAgent = cfg.Summary.Agent
	}

	// 配置里的成员与 Claude 导入的成员合成一张表再排序，前端拿到的仍是
	// 一份按 id 字典序的稳定列表；只排两段再接起来的话，Claude id 落在
	// 配置 id 中间时顺序会乱。
	//
	// 合并之后「配置成员」与「Claude 成员」都在 cfg.Agents 里，来源必须靠
	// ClaudeAgentsMerged 区分：否则导入的成员会被误标成 source = "config"，
	// 前端就再也没法把它们画成只读导入的那一类。
	configAgents := cfgAgents(cfg)
	agents := make([]agentOption, 0, len(configAgents)+len(s.write.ClaudeAgents))
	for _, id := range sortedAgentIDs(configAgents) {
		source := agentSourceConfig
		if _, merged := s.write.ClaudeAgentsMerged[id]; merged {
			source = agentSourceClaude
		}
		agents = append(agents, newAgentOption(id, configAgents[id], source))
	}
	for _, id := range sortedAgentIDs(s.write.ClaudeAgents) {
		if _, ok := configAgents[id]; ok {
			// 配置优先。ScanClaudeAgents 已经按 reserved 跳过了一遍，
			// 这里再挡一次：直接构造 WriteDeps 的测试或将来别的装配方
			// 可能绕过扫描，重复的 id 不该出现在响应里。
			continue
		}
		agents = append(agents, newAgentOption(id, s.write.ClaudeAgents[id], agentSourceClaude))
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	resp.Agents = append(resp.Agents, agents...)

	if cfg != nil {
		teamIDs := make([]string, 0, len(cfg.Teams))
		for id := range cfg.Teams {
			teamIDs = append(teamIDs, id)
		}
		sort.Strings(teamIDs)
		for _, id := range teamIDs {
			members := cfg.Teams[id].Members
			if members == nil {
				members = []string{}
			}
			resp.Teams = append(resp.Teams, teamOption{ID: id, Members: members})
		}
	}

	// skills 只读展示：按 ID 排序复制一份再输出，扫描顺序（优先级顺序）不
	// 泄露到响应里。前端两次请求拿到同一份列表，与本片「界面下一片再做」的
	// 预期一致。空表保持 []，不是 null。
	skills := make([]config.Skill, len(s.write.Skills))
	copy(skills, s.write.Skills)
	sort.Slice(skills, func(i, j int) bool { return skills[i].ID < skills[j].ID })
	for _, skill := range skills {
		resp.Skills = append(resp.Skills, skillOption{
			ID:          skill.ID,
			Name:        skill.Name,
			Description: skill.Description,
			Source:      skillSourceClaude,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// cfgAgents 容忍 nil 配置：只读装配不会走到 /api/options（s.write == nil
// 已经挡在前面），但直接构造 Server 的测试可能给 nil Cfg，这里不让它 panic。
func cfgAgents(cfg *config.Config) map[string]config.AgentConfig {
	if cfg == nil {
		return nil
	}
	return cfg.Agents
}

func sortedAgentIDs(agents map[string]config.AgentConfig) []string {
	ids := make([]string, 0, len(agents))
	for id := range agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ---- POST /api/conversations/{id}/rename ----

const (
	// maxConversationTitle 是会话标题的字符上限，按 rune 数而不是字节数：
	// 一个汉字是 3 个字节，按字节算会让「30 个汉字的标题」被误拒。
	// 120 个字符已经远超左侧列表一行能显示的长度，再长的内容用户也没法
	// 在浏览器里核对改对了没有。
	maxConversationTitle = 120

	// maxRenameBody 是改名请求体的上限。120 个字符的标题即使每个字符
	// 都是 4 字节的 rune 也不到 500 字节，4 KiB 足够容纳 JSON 转义；
	// 超过它的请求体不是一个标题，没必要读完再拒。
	maxRenameBody = 4 << 10
)

// renameRequest 是 POST /api/conversations/{id}/rename 的请求体。
// 只有 title 一个字段：多传的字段按 Go 的 JSON 解码规则忽略，与 /api/ask 一致。
type renameRequest struct {
	Title string `json:"title"`
}

// renameResponse 是改名成功的响应。返回去空白之后的标题，前端据此
// 与列表、标题栏对齐，不必自己再 trim 一遍（两处 trim 迟早会漂）。
type renameResponse struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// handleConversationRename 改一场已有会话的标题。
//
// 门控顺序与其它写接口逐条对齐：
//  1. s.write == nil → 405（只读模式没有任何写能力，先于方法判断）；
//  2. 非 POST → 405 + Allow；
//  3. checkWriteRequest：Content-Type 必须是 application/json（415），
//     Origin / Sec-Fetch-Site 必须同源（403）；
//  4. 请求体必须是单个合法 JSON 对象（400）；
//  5. 标题去空白后 1–120 个字符（400）；
//  6. 会话不存在（404）由 store 的写事务报出来，不在事务外先查一次。
//
// 返回值只有 id 与 title：前端需要的就是「哪一场、现在叫什么」，
// 让它拿整个会话摘要去换标题没有意义。
func (s *Server) handleConversationRename(w http.ResponseWriter, r *http.Request, id string) {
	if s.write == nil {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed,
			"只读模式没有重命名接口：用 council serve -rw 启动才能改会话标题")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "只支持 POST")
		return
	}
	if err := checkWriteRequest(r); err != nil {
		status := http.StatusForbidden
		var guardErr *writeGuardError
		if errors.As(err, &guardErr) {
			status = guardErr.status
		}
		writeError(w, status, err.Error())
		return
	}

	var req renameRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRenameBody))
	if err := dec.Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("请求体超过 %d 字节上限", maxRenameBody))
			return
		}
		writeError(w, http.StatusBadRequest, "请求体不是合法 JSON: "+err.Error())
		return
	}
	// 只认一个对象，理由同 handleAsk：Decode 会停在第一个值上，
	// 静默忽略后半段会让调用方以为自己的第二段参数生效了。
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "请求体只能有一个 JSON 对象")
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "标题不能为空：请给出 1–120 个字符的会话标题")
		return
	}
	if n := utf8.RuneCountInString(title); n > maxConversationTitle {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("标题最长 %d 个字符，实际 %d 个字符", maxConversationTitle, n))
		return
	}

	if err := s.store.RenameConversation(r.Context(), id, title); err != nil {
		if errors.Is(err, conversation.ErrConversationNotFound) {
			writeError(w, http.StatusNotFound, "会话不存在: "+id)
			return
		}
		writeError(w, http.StatusInternalServerError, "改写会话标题失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, renameResponse{ID: id, Title: title})
}

// ---- 任务表 ----

// jobEvent 是 SSE 上的一帧。Name 就是 event: 后面的名字，Data 会被编码成
// 一行的 JSON。
type jobEvent struct {
	Name string
	Data any
}

// job 是一次 /api/ask 的内存状态。
//
// 为什么保留完整历史而不是只开一个 channel：POST 返回 202 与浏览器建立
// EventSource 之间有真实的时间差（React/V8 都要先跑完当前任务），
// started 完全可能先于订阅发生；断线重连时浏览器的 Last-Event-ID 也要求
// 能补发。channel 里没人收的事件会永远丢掉，所以这里用切片做历史，
// changed 通道只承担「有新事件了」的广播。这样做仍然不会让回调阻塞在
// 网络上：emit 只拿一小会儿锁、追加一个值就返回。
type job struct {
	id string

	// ctx / cancel 是这个 job 自己的取消句柄。父 ctx 是 Server.jobCtx
	// （进程级关闭才会取消它），这一层单独一颗取消按钮：POST
	// /api/jobs/{id}/cancel 只能停这一场议会，不能把别人的也停掉。
	// cancel 是 context.CancelFunc，可以重复调用；重复取消因此天然幂等。
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	events   []jobEvent
	changed  chan struct{}
	finished bool
	// result / err 只给测试与清理逻辑读；生产的 done 数据直接来自
	// headless 写出的 JSON，不从这里再拼一遍，避免两处口径。
	result headless.Result
	err    error
	// done 在任务结束（无论成功失败）时关闭，测试用它观察「客户端断开
	// 之后议会仍然跑完」，不必轮询库或硬等。
	done chan struct{}
	// finishOnce 让 finish 幂等：正常收尾与 panic 兜底可能都想结束同一个
	// job，重复 close(done) 会 panic，而兜底逻辑自己不能再把进程带崩。
	finishOnce sync.Once
}

func newJob(id string) *job {
	return &job{
		id:      id,
		changed: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// emit 追加一帧并唤醒等待者。关闭旧 changed、换一个新的，是 Go 里
// 一对多广播的惯用写法：等待者拿到的通道一定会被关闭，不会漏掉唤醒。
func (j *job) emit(name string, data any) {
	j.mu.Lock()
	j.events = append(j.events, jobEvent{Name: name, Data: data})
	close(j.changed)
	j.changed = make(chan struct{})
	j.mu.Unlock()
}

// finish 标记任务结束。done 的关闭放在锁外：等待者被唤醒后要能立刻看到
// result / err，锁内关闭与「先写字段再 close」的顺序由互斥锁保证。
func (j *job) finish(result headless.Result, err error) {
	j.finishOnce.Do(func() {
		j.mu.Lock()
		j.result = result
		j.err = err
		j.finished = true
		close(j.changed)
		j.changed = make(chan struct{})
		j.mu.Unlock()
		close(j.done)
	})
}

// isFinished 给 panic 兜底用：如果 panic 发生在正常 finish 之后，兜底
// 不能再往同一个 job 里补一对 error/done，否则前端会收到互相矛盾的两份结局。
func (j *job) isFinished() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.finished
}

// next 原子地取出第 i 帧：没有新帧时返回的 wait 与 finished 是同一把锁
// 下的快照，SSE handler 不会在「查完没事件、还没开始等」的缝隙里丢掉一帧。
func (j *job) next(i int) (ev jobEvent, ok, finished bool, wait <-chan struct{}) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if i < len(j.events) {
		return j.events[i], true, j.finished, nil
	}
	return jobEvent{}, false, j.finished, j.changed
}

func (j *job) outcome() (headless.Result, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.result, j.err
}

// jobTable 是进程内的任务表。job id 只活在这一个进程里，没有持久化：
// 服务重启后旧 id 一律 404，这是正确的——那些议会进程也已经不在了。
type jobTable struct {
	mu   sync.Mutex
	jobs map[string]*job
}

func newJobTable() *jobTable {
	return &jobTable{jobs: make(map[string]*job)}
}

func (t *jobTable) add(j *job) {
	t.mu.Lock()
	t.jobs[j.id] = j
	t.mu.Unlock()
}

func (t *jobTable) get(id string) (*job, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	j, ok := t.jobs[id]
	return j, ok
}

func (t *jobTable) remove(id string) {
	t.mu.Lock()
	delete(t.jobs, id)
	t.mu.Unlock()
}

// newJobID 铸一个 64 位十六进制 id。用 crypto/rand 而不是时间/自增：
// 端口对本机任何进程开放，可猜的 id 等于把别人的议会进度与正文送给
// 同一台机器上的任何程序。它不做鉴权，但也不主动送路径。
func newJobID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("生成任务 id: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

// ---- POST /api/ask ----

// askRequest 是 POST /api/ask 的请求体。指针字段用来区分「没给」与
// 「给了零值」：rounds 缺席按 1 轮，给了 0 是纯用法错误，不能静默当成 1。
type askRequest struct {
	Prompt         string   `json:"prompt"`
	Members        []string `json:"members"`
	Rounds         *int     `json:"rounds"`
	Summary        string   `json:"summary"`
	BudgetUSD      *float64 `json:"budget_usd"`
	TimeoutSeconds *float64 `json:"timeout_seconds"`

	// Cwd 是这一轮成员的工作目录。空串＝serve 进程的启动目录（与
	// GET /api/options 的 cwd 同一个值），非空必须是绝对且存在的目录。
	// 它只换 Runner 的固定工作目录，不做进程级 chdir。
	Cwd string `json:"cwd"`
	// Workspace 为 true 时把 Cwd 当 git 仓库根，走既有的 -workspace
	// 隔离副本路径。为 true 而 Cwd 不是仓库根在请求阶段就回 400。
	Workspace bool `json:"workspace"`
	// Context 是"相关资料"的路径清单，只拼进 prompt 的一段前置说明，
	// 不读文件内容、不扩权限。最多 20 条、每条不超过 512 字节，
	// 每一条必须是绝对路径。
	Context []string `json:"context"`
	// Conversation 非空表示续接这个已有会话，而不是新建一个。空串保持
	// 既有行为：headless 按 Prompt 建会话并取标题。标题逻辑只在 headless
	// 里，本层不复制一份，否则网页与 `council ask -conversation` 会漂成
	// 两套命名规则。会话是否存在由 handleAsk 在建设任务之前校验。
	Conversation string `json:"conversation"`
}

type askResponse struct {
	JobID string `json:"job_id"`
}

type startedEvent struct {
	ConversationID string `json:"conversation_id"`
}

type turnStartedEvent struct {
	AgentID string `json:"agent_id"`
}

type turnEvent struct {
	AgentID string  `json:"agent_id"`
	Outcome string  `json:"outcome"`
	CostUSD float64 `json:"cost_usd"`
	Text    string  `json:"text"`
}

type doneEvent struct {
	Status  string  `json:"status"`
	Summary string  `json:"summary"`
	CostUSD float64 `json:"cost_usd"`
	Failed  int     `json:"failed"`
	Total   int     `json:"total"`
}

type errorEvent struct {
	Message string `json:"message"`
}

// cancelResponse 与任务书 §3 的 {"cancelled":true} 逐字一致。即使 job
// 已经结束、cancel 是空操作，也回 true：这个端点回答的是「这个 job
// 现在不会再继续跑了」，不是「这次调用真的中断了一个正在跑的进程」。
type cancelResponse struct {
	Cancelled bool `json:"cancelled"`
}

// writeGuardError 让 checkWriteRequest 能同时带回 HTTP 状态码与中文说明。
// 415 与 403 是两类完全不同的拒绝：前者是请求格式不对，后者是浏览器
// 跨站页面在借用户的端口花钱；状态码不能都写成 400。
type writeGuardError struct {
	status  int
	message string
}

func (e *writeGuardError) Error() string { return e.message }

// checkWriteRequest 是写端点专用的两道闸，读端点一条都不加：
//
//  1. Content-Type 必须解析成 application/json。浏览器跨站「简单请求」
//     只能发 text/plain、application/x-www-form-urlencoded 或 multipart，
//     先把它们挡在门外，恶意网页就无法在不触发 CORS 预检的情况下提交。
//  2. Origin 存在时，host 必须与本次请求的 Host 一致；Sec-Fetch-Site 存在
//     时只放行 same-origin / none。没有这两个头的请求（curl、脚本、dsh
//     工具调用）放行——它们不是浏览器跨站，挡了反而误伤本机自动化。
//
// 两道同时上而不是只挑一道：Content-Type 挡的是浏览器不触发预检就能
// 发出的「简单请求」；Origin / Sec-Fetch-Site 是在请求真的到达时再做一次
// 同源确认。漏掉任何一道，都会留下一类浏览器能发起的写请求。
func checkWriteRequest(r *http.Request) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return &writeGuardError{
			status:  http.StatusUnsupportedMediaType,
			message: "写接口要求 Content-Type: application/json",
		}
	}
	return checkWriteOrigin(r)
}

// checkWriteOrigin 只做同源的两道确认，不要求 JSON 请求体。取消端点
// 用它：POST /api/jobs/{id}/cancel 没有字段，浏览器发一个空 body 的
// fetch 是完全正常的写法，要求 Content-Type: application/json 只会
// 把正常前端挡在外面；跨站页面的 Origin / Sec-Fetch-Site 仍然挡得住。
func checkWriteOrigin(r *http.Request) error {
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		u, parseErr := url.Parse(origin)
		if parseErr != nil || u.Host == "" || !strings.EqualFold(u.Host, r.Host) {
			return &writeGuardError{
				status:  http.StatusForbidden,
				message: "拒绝跨站写请求：Origin 与本次请求的 Host 不一致",
			}
		}
	}

	switch strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))) {
	case "", "same-origin", "none":
	default:
		return &writeGuardError{
			status:  http.StatusForbidden,
			message: "拒绝跨站写请求：Sec-Fetch-Site 不是 same-origin",
		}
	}
	return nil
}

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	if s.write == nil {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed,
			"只读模式没有写接口：用 council serve -rw 启动才能跑议会")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "只支持 POST")
		return
	}
	if err := checkWriteRequest(r); err != nil {
		status := http.StatusForbidden
		var guardErr *writeGuardError
		if errors.As(err, &guardErr) {
			status = guardErr.status
		}
		writeError(w, status, err.Error())
		return
	}

	var req askRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAskBody))
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法 JSON: "+err.Error())
		return
	}
	// 只认一个对象：Json 后面再跟一段东西时 Decode 会停在第一个值上，
	// 静默忽略后半段会让调用方以为自己的第二段参数生效了。
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "请求体只能有一个 JSON 对象")
		return
	}

	opts, err := optionsFromAsk(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// 先校验再建任务：未知成员、非法预算、round 模式缺汇总，这些在
	// headless.Run 里也是同一套判据，但在这里挡住可以给调用方一个
	// 干净的 400，不必开一个必然失败的后台任务再让 SSE 去报 error。
	//
	// 校验失败时再交给 claudeProviderHint 看一眼：名字确实来自
	// .claude/agents、只是缺 [providers.claude] 的那几条，要在 400 里
	// 说清根因，而不是让用户对着「配置里没有这个 agent」去改配置。
	if err := headless.Validate(opts, s.write.Cfg); err != nil {
		writeError(w, http.StatusBadRequest, s.claudeProviderHint(err, opts.Members).Error())
		return
	}
	// 续接目标必须在创建任务之前确认：未知 id 若放进去，headless 会在
	// 后台任务里失败，前端先收到 202，只能在 SSE 上看到一个 error。这里
	// 提前回 400，文案点名是哪个 id、以及「留空＝新建」这条出路。
	if status, err := s.checkAskConversation(r.Context(), opts.Conversation); err != nil {
		writeError(w, status, err.Error())
		return
	}
	// cwd / workspace / context 三道扩展在基础校验之后做：
	//   - 先 Validate 保证"空问题 + context"不会因为拼了前置说明就混过去；
	//   - workspace=true 要跑一次 git（子进程 + 文件系统），这是请求里
	//     唯一需要 IO 的校验，放在最便宜的检查后面，坏请求不必等 git。
	if err := s.applyAskScopes(r.Context(), req, &opts); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := newJobID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	j := newJob(id)
	// 每个 job 一颗自己的取消按钮，父 ctx 是进程级的 jobCtx。
	// 先建好再进 goroutine：POST /api/jobs/{id}/cancel 可能在 202 返回的
	// 同一毫秒到达，此刻 j.cancel 必须已经非 nil。
	parent := s.jobCtx
	if parent == nil {
		// EnableWrite 一定会建 jobCtx；这里是给直接构造 Server 的测试
		// 留的兜底，免得一个装配疏漏变成 nil ctx 的 panic。
		parent = context.Background()
	}
	j.ctx, j.cancel = context.WithCancel(parent)
	s.jobs.add(j)
	// 议会挂 s.jobCtx，不挂请求的 ctx：浏览器关掉页面时 r.Context() 会
	// 取消，但已经花出去的钱与正在跑的成员不该跟着作废。s.jobCtx 只在
	// 进程关闭且等满 10 秒仍未收尾时被 cancelJobs 显式取消。
	//
	// Add 必须在 go 之前：关闭流程的 Wait 看到 0 时就会去关库，
	// 那一瞬间才 Add 的任务会立刻撞上已经关掉的 Service/Runner。
	s.jobsWG.Add(1)
	go s.runAskJob(j, opts)
	writeJSON(w, http.StatusAccepted, askResponse{JobID: id})
}

// optionsFromAsk 把 HTTP 请求体翻译成 headless.Options。默认值与
// `council ask` 的 flag 默认值一致（rounds=1、budget=5、timeout=10m），
// 这样网页与命令行对同一句问题的收费上限与超时口径相同。
func optionsFromAsk(req askRequest) (headless.Options, error) {
	rounds := 1
	if req.Rounds != nil {
		rounds = *req.Rounds
	}
	mode := "ask"
	if rounds > 1 {
		// rounds > 1 与 CLI 的 -mode round 是同义词：那边的多轮必须有人
		// 收口，所以这里也走 round，让 headless 的汇总规则原样生效。
		mode = "round"
	}

	budget := 5.0
	if req.BudgetUSD != nil {
		budget = *req.BudgetUSD
	}
	timeout := 10 * time.Minute
	if req.TimeoutSeconds != nil {
		// time.Duration 的上限远大于任何合理会话；这里不做额外上限，
		// 只挡住「非正数」与「乘出来溢出成非正数」两种非法值。
		timeout = time.Duration(*req.TimeoutSeconds * float64(time.Second))
		if timeout <= 0 {
			return headless.Options{}, fmt.Errorf("timeout_seconds 必须大于 0")
		}
	}

	members := make([]string, 0, len(req.Members))
	for _, raw := range req.Members {
		if id := strings.TrimSpace(raw); id != "" {
			members = append(members, id)
		}
	}

	return headless.Options{
		Prompt:    strings.TrimSpace(req.Prompt),
		Members:   members,
		Mode:      mode,
		Rounds:    rounds,
		Summary:   strings.TrimSpace(req.Summary),
		BudgetUSD: budget,
		Timeout:   timeout,
		// 空串＝新建；非空＝续接。两种语义全部由 headless 实现，
		// 这里只做搬运，包括标题：只有新建那一路才把 Prompt 当标题。
		Conversation: strings.TrimSpace(req.Conversation),
		JSON:         true,
	}, nil
}

// checkAskConversation 在 /api/ask 创建任务之前确认续接目标存在。
//
// 不存在回 400 而不是 404：这个端点的资源是「一次新的议会」，
// 请求体里的 conversation 是参数，参数指错目标是请求错误，不是
// 「URL 指向的资源没有」。读库失败则是 500——那不是调用方能修的。
// id 为空时直接放行：空串就是新建，保持既有行为。
func (s *Server) checkAskConversation(ctx context.Context, id string) (int, error) {
	if id == "" {
		return 0, nil
	}
	if _, err := s.store.Conversation(ctx, id); err != nil {
		if errors.Is(err, conversation.ErrConversationNotFound) {
			return http.StatusBadRequest, fmt.Errorf(
				"要续接的会话不存在：%s（请从左侧列表重新选择；留空 conversation 表示新建会话）", id)
		}
		return http.StatusInternalServerError, fmt.Errorf("读取要续接的会话 %s 失败：%w", id, err)
	}
	return 0, nil
}

// applyAskScopes 把 §2 的三个新字段翻译进 headless.Options：
// cwd（成员工作目录，空＝serve 的启动目录）、workspace（在 git 仓库的
// 隔离副本里跑）、context（只拼进 prompt 的路径清单）。
//
// 全部校验都在这里、都在建任务之前完成：这三样里任何一样非法，调用方
// 拿到的必须是一个干净的 400，而不是先收 202 再在 SSE 上看 error。
func (s *Server) applyAskScopes(ctx context.Context, req askRequest, opts *headless.Options) error {
	cwd, err := s.resolveAskCwd(req.Cwd)
	if err != nil {
		return err
	}
	// 空 cwd 也在这一层被填成 serve 的启动目录。headless 那边空串的含义
	// 不同（保持 CLI 的 agentWorkDir 分目录行为），所以默认值必须在服务层
	// 落地，不能留给 headless 猜。
	opts.Cwd = cwd

	contextPaths, err := normalizeAskContext(req.Context)
	if err != nil {
		return err
	}
	// 路径清单交给 headless 拼：会话标题取的是 opts.Prompt，如果在这里
	// 就把清单拼进去，标题会变成「相关资料（请自行按路径读取）…」。
	opts.Context = contextPaths

	if req.Workspace {
		// 复用 -workspace 的那道判据（git rev-parse 与项目路径规范化），
		// 不在这里另写一份"目录里有没有 .git"——那会把子目录、bare 仓库、
		// 符号链接这些情况判成与 CLI 不同的答案。
		root, rootErr := headless.WorkspaceRepoRoot(ctx, cwd)
		if rootErr != nil {
			return fmt.Errorf("workspace=true 要求 cwd 是 git 仓库根：%w", rootErr)
		}
		opts.Workspace = root
	}
	return nil
}

// resolveAskCwd 校验并规范化 cwd。空串取 serve 的启动目录；非空必须是
// 绝对路径、存在的目录。只读端点不需要它，所以不放进 headless.Validate：
// 那个函数被 CLI 共用，不该认识 HTTP 层的默认值。
func (s *Server) resolveAskCwd(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		cwd := s.workbenchCwd()
		if cwd == "" {
			return "", fmt.Errorf("无法确定 serve 的工作目录")
		}
		return cwd, nil
	}
	if !filepath.IsAbs(raw) {
		return "", fmt.Errorf("cwd 必须是绝对路径：%q", raw)
	}
	clean := filepath.Clean(raw)
	info, err := os.Stat(clean)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("cwd 目录不存在：%s", clean)
		}
		return "", fmt.Errorf("cwd 目录不可用：%s：%w", clean, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cwd 不是目录：%s", clean)
	}
	return clean, nil
}

// workbenchCwd 是 serve 的启动目录。runWritable 启动时存了一份；直接
// 构造 Server 的测试没有那一份，回落到 os.Getwd，与生产同一个来源。
func (s *Server) workbenchCwd() string {
	if s.write != nil {
		if cwd := strings.TrimSpace(s.write.Cwd); cwd != "" {
			return cwd
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

// normalizeAskContext 校验 context 清单。只做任务书允许的三件事：
// 条数、单条字节数、绝对路径；不读文件、不判断存在性、不判断是否在
// 成员工作区里——那正是"请自行按路径读取"留给成员的解释空间。
func normalizeAskContext(raw []string) ([]string, error) {
	if len(raw) > maxContextPaths {
		return nil, fmt.Errorf("context 最多 %d 条，实际 %d 条", maxContextPaths, len(raw))
	}
	out := make([]string, 0, len(raw))
	for i, item := range raw {
		path := strings.TrimSpace(item)
		if len(path) > maxContextPathBytes {
			return nil, fmt.Errorf("context 第 %d 条超过 %d 字节", i+1, maxContextPathBytes)
		}
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("context 第 %d 条必须是绝对路径：%q", i+1, path)
		}
		out = append(out, path)
	}
	return out, nil
}

// runAskJob 在后台跑一次议会。轮次、预算、汇总全部由 headless.Run 决定；
// 本函数只把它的三个观察点（开始、每条记录、最终 JSON）搬成 SSE 事件。
func (s *Server) runAskJob(j *job, opts headless.Options) {
	// defer 顺序（LIFO）：job 的 cancel 最后注册、最先执行；然后是
	// scheduleJobCleanup；recover 其次；jobsWG.Done 最先注册、最后执行。
	// 清理排在 recover 之前，是为了让 recover 也能接住清理自身可能出现的
	// panic；而关闭流程以 jobsWG 归零作为「可以关库」的信号，所以它必须
	// 最后发生。
	defer s.jobsWG.Done()
	defer func() {
		if rec := recover(); rec != nil {
			// panic 不能让整个 serve 进程退出：别的会话、只读端点、
			// 已经排队的任务都还活着。把这一轮明确结束掉，前端拿到
			// error + done 就能解锁，库与锁由正常关闭流程负责。
			if j.isFinished() {
				return
			}
			msg := fmt.Sprintf("议会执行时发生 panic: %v", rec)
			j.emit("error", errorEvent{Message: msg})
			j.emit("done", doneEvent{Status: "error"})
			j.finish(headless.Result{Status: "error"}, errors.New(msg))
		}
	}()
	defer s.scheduleJobCleanup(j.id)
	// 正常收尾也释放 job 自己的 ctx：headless 内部的 WithTimeout 已经
	// cancel 过，但那个 cancel 管不到这一层；不释放的话每跑完一个 job
	// 都会在父 ctx 上留一个永远不会触发的子 ctx，直到服务退出。
	if j.cancel != nil {
		defer j.cancel()
	}

	opts.JSON = true
	var buf bytes.Buffer

	deps := headless.Deps{
		Cfg:        s.write.Cfg,
		Svc:        s.write.Svc,
		Rn:         s.write.Rn,
		DataDir:    s.write.DataDir,
		ProjectDir: s.write.ProjectDir,
		// headless 的 stdout 只能是那一个 JSON 结果：这里的 buffer 就是
		// 捕获它的地方，解析出来的 Result 直接变成 done 的字段。
		Out: &buf,
		// 给人看的 stderr 在网页里没有去处；真正的失败会经 error 事件
		// 交给浏览器，不再往服务进程的终端重复一份。
		Err: io.Discard,
		OnStart: func(convID string) {
			j.emit("started", startedEvent{ConversationID: convID})
		},
		// 成员进程真正启动之前推一帧。它只往内存事件表追加，不碰网络；
		// 浏览器断线时 job 仍在跑，事件也还在历史里，重连能补发。
		OnTurnStart: func(agentID string) {
			j.emit("turn_started", turnStartedEvent{AgentID: agentID})
		},
		OnRecord: func(rec turn.Record) {
			j.emit("turn", turnEvent{
				AgentID: rec.AgentID,
				Outcome: eventOutcomeName(rec.Outcome),
				CostUSD: eventCost(rec),
				Text:    eventText(rec),
			})
		},
	}

	// 跑这一场的 ctx 是 job 自己的：取消按钮只停这一场。直接构造 Server
	// 的测试可能没走 handleAsk（因此没有 j.ctx），回落到进程级 jobCtx，
	// 再退到 Background，与既有兜底同一条路。
	runCtx := j.ctx
	if runCtx == nil {
		runCtx = s.jobCtx
	}
	if runCtx == nil {
		runCtx = context.Background()
	}
	runErr := headless.Run(runCtx, deps, opts)

	res, ok := decodeResult(buf.Bytes())
	if !ok {
		// headless 在写 Result 之前就失败（配置为空、装配不完整——本路径
		// 上不会发生，但错误处理不能假设）。仍然发一条 done，让前端一定
		// 能解锁按钮；否则一次早期失败会把输入框永久禁在「运行中」。
		msg := "议会执行失败"
		if runErr != nil {
			msg = runErr.Error()
		}
		j.emit("error", errorEvent{Message: msg})
		j.emit("done", doneEvent{Status: "error"})
		j.finish(headless.Result{Status: "error"}, runErr)
		return
	}

	// status=error 是执行失败，除了 done 里的状态之外再给一条可读的
	// 原因；budget_exceeded / partial 是业务结果，不额外报 error。
	if runErr != nil && res.Status == "error" {
		j.emit("error", errorEvent{Message: runErr.Error()})
	}
	j.emit("done", doneEvent{
		Status:  res.Status,
		Summary: res.Summary,
		CostUSD: res.CostUSD,
		Failed:  res.Failed,
		Total:   res.Total,
	})
	j.finish(res, runErr)
}

// decodeResult 解 headless 写出的那个 JSON 对象。空 buffer 不是错误，
// 只表示 Run 在写结果之前就返回了；调用方据此走 error 分支。
func decodeResult(raw []byte) (headless.Result, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return headless.Result{}, false
	}
	var res headless.Result
	if err := json.Unmarshal(raw, &res); err != nil {
		return headless.Result{}, false
	}
	return res, true
}

// scheduleJobCleanup 在任务结束后按 jobTTL 从内存表删掉它。用
// time.AfterFunc 而不是让 SSE handler 自己延时：没有浏览器连着的任务
// 同样要被清掉，清理不能依赖读者存在。
func (s *Server) scheduleJobCleanup(id string) {
	ttl := s.jobTTL
	if ttl <= 0 {
		ttl = defaultJobTTL
	}
	time.AfterFunc(ttl, func() {
		if s.jobs != nil {
			s.jobs.remove(id)
		}
	})
}

// eventOutcomeName 与 headless 的 outcomeName 逐字一致。两份都从
// turn.Outcome 的同一组常量派生；差别只在包边界——headless 不导出这个
// 纯映射，而 SSE 需要实时给，不能等整趟结束从 Result 里回头取。
func eventOutcomeName(o turn.Outcome) string {
	switch o {
	case turn.OutcomeSpoke:
		return "ok"
	case turn.OutcomePostRejected:
		return "post_rejected"
	case turn.OutcomeProviderFailed:
		return "provider_failed"
	case turn.OutcomeUnparsable:
		return "unparsable"
	case turn.OutcomeTimeout:
		return "timeout"
	case turn.OutcomeSpawnFailed:
		return "spawn_failed"
	case turn.OutcomeRefused:
		return "refused"
	case turn.OutcomeCanceled:
		return "canceled"
	case turn.OutcomeLost:
		return "lost"
	default:
		return "unknown"
	}
}

// eventCost 与 headless.costOf 同口径：没报金额的一轮是 0，不是负数、
// 也不是「免费」的断言——总额可不可信由 done.status 与 CLI 共用的
// Result.cost_complete 决定，SSE 不另造一套。
func eventCost(rec turn.Record) float64 {
	if rec.CostUSDDelta == nil {
		return 0
	}
	return *rec.CostUSDDelta
}

// eventText 取这一轮真正付过钱拿到的那段正文：正常发言在 Message 里，
// post_rejected 的正文在 DroppedText 里，不能让它在网页上消失。
func eventText(rec turn.Record) string {
	if rec.Message != nil {
		return rec.Message.Text
	}
	return rec.DroppedText
}

// ---- /api/jobs/{id}/events 与 /api/jobs/{id}/cancel ----

// handleJobPath 分发 /api/jobs/{id}/ 下面的两个子资源。只读模式的判断
// 放在最前面：两个子资源都属于写模式能力，非 -rw 时统一回 405 并说明
// 原因，不能落进 /api/ 的兜底变成「没有这个接口」的 404。
func (s *Server) handleJobPath(w http.ResponseWriter, r *http.Request) {
	if s.write == nil {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed,
			"只读模式没有任务接口：用 council serve -rw 启动才能跑议会")
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	segments := strings.Split(rest, "/")
	if len(segments) != 2 || segments[0] == "" {
		writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
		return
	}
	switch segments[1] {
	case "events":
		s.handleJobEvents(w, r, segments[0])
	case "cancel":
		s.handleJobCancel(w, r, segments[0])
	default:
		writeError(w, http.StatusNotFound, "没有这个接口: "+r.URL.Path)
	}
}

// handleJobEvents 是 SSE 订阅口。方法不符时按既有口径回 405 + Allow。
func (s *Server) handleJobEvents(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "只支持 GET")
		return
	}
	j, ok := s.jobs.get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "任务不存在: "+id)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "当前 HTTP 连接不支持流式响应")
		return
	}

	// Last-Event-ID 由浏览器在自动重连时带回来。从它开始续传，断线重连
	// 就不会把已经显示的 turn 再打一遍；首次连接没有这个头，从头放。
	start := 0
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			start = n
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		ev, hasEvent, finished, wait := j.next(start)
		if hasEvent {
			start++
			if err := writeSSEEvent(w, start, ev); err != nil {
				return
			}
			flusher.Flush()
			continue
		}
		if finished {
			return
		}
		select {
		case <-wait:
		case <-r.Context().Done():
			// 只取消订阅。任务在 s.runAskJob 的 goroutine 里继续跑，
			// 已经花掉的钱必须换回结果（任务书 §3）。
			return
		}
	}
}

// handleJobCancel 取消一个 job 的 ctx。已经跑完的成员结果不会被撤：
// headless.Run 每轮结束就经 RecordTurn 落库，取消只影响还在飞的那些。
//
// 重复取消幂等：j.cancel 是 context.CancelFunc，第二次调用什么也不做，
// 仍然回 202 + true——"这个 job 现在是取消状态"对两次请求都是真话。
// 已经自然结束的 job 也接受取消（404 只表示这个 id 从未存在或已过
// TTL），否则前端在 done 与 cancel 之间的竞态里会拿到一个吓人的 404。
func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "只支持 POST")
		return
	}
	if err := checkWriteOrigin(r); err != nil {
		status := http.StatusForbidden
		var guardErr *writeGuardError
		if errors.As(err, &guardErr) {
			status = guardErr.status
		}
		writeError(w, status, err.Error())
		return
	}

	j, ok := s.jobs.get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "任务不存在: "+id)
		return
	}
	if j.cancel != nil {
		j.cancel()
	}
	writeJSON(w, http.StatusAccepted, cancelResponse{Cancelled: true})
}

// writeSSEEvent 按 SSE 线格式写一帧：id 用于断线续传，event 是事件名，
// data 是一行 JSON（json.Marshal 会把正文里的换行转义掉，不会破坏分帧）。
func writeSSEEvent(w io.Writer, id int, ev jobEvent) error {
	data, err := json.Marshal(ev.Data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", id, ev.Name, data)
	return err
}
