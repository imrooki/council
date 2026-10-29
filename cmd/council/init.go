package main

// init.go 是 `council init`：把 TUI 的「首次运行自动配置」搬到命令行，
// 好让没有终端的人（桌面版、脚本、CI）也能把配置建出来。
//
// 为什么需要它：`council` 不带参数时会在缺配置的情况下自己检测四家 CLI、
// 生成配置再进 TUI（internal/app/firstrun.go）。但桌面版直接跑的是
// `council serve -rw`，而写模式要求完整配置 —— 于是「第一次双击」在干净
// 机器上只会得到一个错误对话框。有了这条命令，桌面向导可以先 `init --json`
// 问「本机有什么」，再 `init --apply` 把用户勾选的结果写下去。
//
// 三条边界：
//   - **只读检测**：默认只检测、只打印，一个字节都不写；写只发生在 --apply；
//   - **不覆盖**：配置文件已存在就拒绝（与 TUI 的「只在默认位置、文件不存在时」一致），
//     绝不静默改掉用户手写的那份；
//   - **先验证再落盘**：拼好的文本先写进临时目录、按正常路径加载并 Validate，
//     通过了才原子地建成真文件 —— 宁可不写，也不留一份加载不了的配置。

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"council/internal/config"
	"council/internal/i18n"
	"council/internal/provider"
	"council/internal/repl"
	"council/internal/turn"
)

// initProbeTimeout 是每一家的检测超时（识别与登录状态共用），与 TUI 首次运行同值。
// 测试可以改短。
var initProbeTimeout = 20 * time.Second

// initExec 与 initLoginExec 是检测用的执行器，生产用 turn 里的实现（与 TUI 一致）。
// initDetect 是「本机装了哪几家」的探测入口，测试用它注入固定的四家，
// 免得用例结果随开发机装了什么而变。
var (
	initExec      provider.DiscoveryExec   = turn.RunDiscovery
	initLoginExec provider.LoginStatusExec = turn.RunLoginStatus
	initDetect                             = config.DetectProviders
)

// initModel 是检测到的一个模型。
type initModel struct {
	// ID 是写进配置的 model 值（provider 认的那个标识）。
	ID     string `json:"id"`
	Name   string `json:"name"`
	Custom bool   `json:"custom"`
}

// initProvider 是一家的检测结果。
type initProvider struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	// Found=false 时 Path 为空，其余字段没有意义。
	Found  bool        `json:"found"`
	Models []initModel `json:"models"`
	// Login 是 provider.LoginState 的字符串形式；前端据此提示「先去终端登录」。
	Login string `json:"login"`
	// LoginText 是给用户看的那句话（repl.LoginText，与 /add 同一份来源）。
	LoginText string `json:"login_text"`
	// Error 非空表示这一家识别失败（工具在、但跑不起来），依然是「找到了」。
	Error string `json:"error,omitempty"`
}

// initReport 是 --json 的输出形状。
type initReport struct {
	ConfigPath   string         `json:"config_path"`
	ConfigExists bool           `json:"config_exists"`
	DataDir      string         `json:"data_dir"`
	Providers    []initProvider `json:"providers"`
	// Error 只在整体失败时出现（此时退出码非 0）。
	Error string `json:"error,omitempty"`
}

// initAgentRequest 是 --apply 里一个成员的写法。
type initAgentRequest struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Model       string `json:"model"`
	// Permissions 只有 read / write；空串按 read（首次运行不该默认给写权限）。
	Permissions string `json:"permissions"`
	// Mode 空串按 discussion。
	Mode string `json:"mode"`
}

// initProviderRequest 是 --apply 里一家的写法。
type initProviderRequest struct {
	ID     string             `json:"id"`
	Agents []initAgentRequest `json:"agents"`
}

// initApplyRequest 是 --apply 的输入。
type initApplyRequest struct {
	Providers []initProviderRequest `json:"providers"`
}

// initApplyReport 是 --apply 的输出。
type initApplyReport struct {
	OK         bool     `json:"ok"`
	ConfigPath string   `json:"config_path"`
	Providers  []string `json:"providers"`
	Agents     []string `json:"agents"`
	Error      string   `json:"error,omitempty"`
}

func runInit(args []string) int {
	return runInitWith(args, os.Stdin, os.Stdout, os.Stderr)
}

func runInitWith(args []string, stdin io.Reader, out, errOut io.Writer) int {
	lang := applyLanguageFlag(prescanFlag(args, "lang"), prescanFlag(args, "config"))
	text := func(key, fallback string, args ...any) string {
		return i18n.TF(lang, key, fallback, args...)
	}

	fs := flag.NewFlagSet("council init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "", text("cli.init.flag.config",
		"配置文件路径（默认 ~/.council/config.toml）；与全局 -config 含义相同"))
	langFlag := fs.String("lang", "auto", text("cli.init.flag.lang",
		"输出语言：zh / en / auto（默认 auto）"))
	asJSON := fs.Bool("json", false, text("cli.init.flag.json",
		"按 JSON 输出检测结果（给桌面向导与脚本用），不写任何文件"))
	applyPath := fs.String("apply", "", text("cli.init.flag.apply",
		"从文件读 JSON 并写配置（用 - 表示 stdin）；配置文件已存在时拒绝"))
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(errOut, initUsageLang(lang))
			return exitOK
		}
		fmt.Fprintf(errOut, "council init: %v\n", err)
		fmt.Fprint(errOut, initUsageLang(lang))
		return exitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprint(errOut, text("cli.init.error.positional",
			"council init 不接受位置参数，多出来的是: %v\n", fs.Args()))
		return exitUsage
	}

	lang = applyLanguageFlag(*langFlag, *configPath)
	text = func(key, fallback string, args ...any) string {
		return i18n.TF(lang, key, fallback, args...)
	}

	layers, err := config.ResolveConfigLayers(*configPath)
	if err != nil {
		fmt.Fprint(errOut, text("cli.init.error.layers", "无法确定配置文件位置: %v\n", err))
		return exitConfigFailed
	}
	target := layers[0].Path

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if path := strings.TrimSpace(*applyPath); path != "" {
		return runInitApply(ctx, path, target, stdin, out, errOut, text, *asJSON)
	}
	return runInitDetect(ctx, target, out, errOut, text, *asJSON)
}

// initDataDir 是首次运行要写进配置的 data_dir：配置文件旁边的 data（与 TUI 一致）。
func initDataDir(target string) string {
	return filepath.Join(filepath.Dir(target), "data")
}

// runInitDetect 检测四家并打印（--json 时给结构化结果）。
func runInitDetect(ctx context.Context, target string, out, errOut io.Writer,
	text func(string, string, ...any) string, asJSON bool) int {
	report := initReport{
		ConfigPath:   target,
		ConfigExists: fileExists(target),
		DataDir:      initDataDir(target),
		Providers:    []initProvider{},
	}

	detected := initDetect()
	report.Providers = probeProviders(ctx, detected)

	if asJSON {
		if err := writeInitJSON(out, report); err != nil {
			fmt.Fprint(errOut, text("cli.init.error.write", "写输出失败: %v\n", err))
			return exitConfigFailed
		}
		return exitOK
	}

	// 文本模式：给人看的一份摘要（--json 才是给程序看的）。
	fmt.Fprint(out, text("cli.init.head", "本机 CLI 检测（不会写任何文件）：\n"))
	found := 0
	for _, p := range report.Providers {
		if !p.Found {
			fmt.Fprintf(out, "  %-6s  %s\n", p.ID, text("cli.init.notFound", "没找到"))
			continue
		}
		found++
		if p.Error != "" {
			fmt.Fprintf(out, "  %-6s  %s  %s\n", p.ID, p.Path,
				text("cli.init.probeFailed", "识别失败：%v", p.Error))
			continue
		}
		fmt.Fprintf(out, "  %-6s  %s  %s  %s\n", p.ID, p.Path,
			text("cli.init.modelCount", "识别到 %d 个模型", len(p.Models)), p.LoginText)
	}
	fmt.Fprintf(out, "  %s\n", text("cli.init.configPath", "配置文件：%s（%s）", target,
		text("cli.init.configExists", "已存在，不会被覆盖")))
	if found == 0 {
		fmt.Fprint(out, text("cli.init.noneFound",
			"一个都没找到。至少装一个（claude / codex / grok / agy）并在终端里登录，再回来。\n"))
	}
	return exitOK
}

// probeProviders 并行检测四家：识别模型 + 登录状态，顺序固定。
func probeProviders(ctx context.Context, detected []config.DetectedProvider) []initProvider {
	rows := make([]initProvider, len(detected))
	var wg sync.WaitGroup
	for i, d := range detected {
		rows[i] = initProvider{ID: d.ID, Path: d.Path, Found: d.Path != "", Models: []initModel{}}
		if d.Path == "" {
			continue
		}
		wg.Add(1)
		go func(i int, d config.DetectedProvider) {
			defer wg.Done()
			rows[i] = probeProvider(ctx, d)
		}(i, d)
	}
	wg.Wait()
	return rows
}

// probeProvider 是单独一家的识别与登录状态（与 internal/app 的 firstRunCheck 同一条路）。
func probeProvider(ctx context.Context, d config.DetectedProvider) initProvider {
	cctx, cancel := context.WithTimeout(ctx, initProbeTimeout)
	defer cancel()

	row := initProvider{ID: d.ID, Path: d.Path, Found: true, Models: []initModel{}}
	var stdout []byte
	capture := func(c context.Context, cmd provider.DiscoveryCommand) ([]byte, error) {
		out, err := initExec(c, cmd)
		stdout = out
		return out, err
	}
	models, err := provider.DiscoverModels(cctx, d.ID, d.Path, capture)
	if err != nil {
		row.Error = err.Error()
	}
	for _, m := range models {
		row.Models = append(row.Models, initModel{ID: m.Model, Name: m.Name, Custom: m.Custom})
	}
	login := provider.LoginFromDiscovery(d.ID, stdout)
	if sc, ok := provider.LoginStatusCommandFor(d.ID, d.Path); ok && cctx.Err() == nil {
		if code, so, se, serr := initLoginExec(cctx, sc); serr == nil {
			login = provider.ParseLoginStatus(d.ID, code, so, se)
		}
	}
	row.Login = loginStateString(login)
	row.LoginText = repl.LoginText(login)
	return row
}

// loginStateString 把 LoginState 折成稳定的字符串（写进 JSON 给前端判断）。
func loginStateString(info provider.LoginInfo) string {
	switch info.State {
	case provider.LoginYes:
		return "yes"
	case provider.LoginAPIKey:
		return "api_key"
	case provider.LoginNo:
		return "no"
	default:
		return "unknown"
	}
}

// runInitApply 读入用户的勾选、拼出配置、验证通过后再落盘。
func runInitApply(ctx context.Context, source, target string, stdin io.Reader, out, errOut io.Writer,
	text func(string, string, ...any) string, asJSON bool) int {
	fail := func(code int, message string) int {
		if asJSON {
			if err := writeInitJSON(out, initApplyReport{OK: false, ConfigPath: target, Error: message}); err != nil {
				fmt.Fprint(errOut, text("cli.init.error.write", "写输出失败: %v\n", err))
				return exitConfigFailed
			}
			return code
		}
		fmt.Fprintln(errOut, message)
		return code
	}

	if fileExists(target) {
		return fail(exitConfigFailed, text("cli.init.error.exists",
			"配置文件已存在，不覆盖：%s（要改它请用 council serve -rw 的设置页，或自己编辑）", target))
	}

	body, err := readInitInput(source, stdin)
	if err != nil {
		return fail(exitConfigFailed, text("cli.init.error.read", "读输入失败: %v", err))
	}
	var req initApplyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return fail(exitUsage, text("cli.init.error.json", "输入不是合法 JSON: %v", err))
	}

	// 拼配置：DefaultTOML + data_dir + 选中的 [providers.*] + [agents.*]。
	// 选中的 provider 必须是真的检测到路径的，否则写进去也跑不起来。
	detected := initDetect()
	known := make(map[string]string, len(detected))
	for _, d := range detected {
		known[d.ID] = d.Path
	}
	chosen := make([]config.DetectedProvider, 0, len(req.Providers))
	agentBlocks := make([]string, 0, 4)
	agentIDs := make([]string, 0, 4)
	providerIDs := make([]string, 0, 4)
	seenAgent := make(map[string]bool, 4)
	for _, p := range req.Providers {
		path, ok := known[p.ID]
		if !ok || path == "" {
			return fail(exitUsage, text("cli.init.error.unknownProvider",
				"没有检测到这个 provider（或它没装）：%s", p.ID))
		}
		chosen = append(chosen, config.DetectedProvider{ID: p.ID, Path: path})
		providerIDs = append(providerIDs, p.ID)
		for _, a := range p.Agents {
			agent, err := buildInitAgent(p.ID, a)
			if err != nil {
				return fail(exitUsage, err.Error())
			}
			if seenAgent[agent.id] {
				return fail(exitUsage, text("cli.init.error.duplicateAgent",
					"成员 id 重复：%s", agent.id))
			}
			seenAgent[agent.id] = true
			agentBlocks = append(agentBlocks, config.AgentBlockTOML(agent.id, agent.cfg, ""))
			agentIDs = append(agentIDs, agent.id)
		}
	}
	if len(chosen) == 0 {
		return fail(exitUsage, text("cli.init.error.noProvider", "至少要选一个 provider"))
	}

	// 空一行再写成员段：没有它的话 [agents.x] 会紧贴上一个 [providers.y] 的
	// command 行，读起来像同一个表（TOML 合法，但人看会误会）。
	content := config.FirstRunTOML(initDataDir(target), chosen) + "\n" + strings.Join(agentBlocks, "\n")

	// 先验证再落盘：写到临时目录、按真实路径加载并 Validate。
	// 任何一步失败都直接返回，绝不留半份配置在 ~/.council 下。
	if err := validateInitConfig(content); err != nil {
		return fail(exitConfigFailed, text("cli.init.error.invalid", "生成的配置没有通过校验: %v", err))
	}

	if err := config.CreateConfigFile(target, []byte(content)); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fail(exitConfigFailed, text("cli.init.error.exists",
				"配置文件已存在，不覆盖：%s", target))
		}
		return fail(exitConfigFailed, text("cli.init.error.writeFile",
			"写不进配置文件 %s: %v", target, err))
	}

	report := initApplyReport{OK: true, ConfigPath: target, Providers: providerIDs, Agents: agentIDs}
	if asJSON {
		if err := writeInitJSON(out, report); err != nil {
			fmt.Fprint(errOut, text("cli.init.error.write", "写输出失败: %v\n", err))
			return exitConfigFailed
		}
		return exitOK
	}
	fmt.Fprint(out, text("cli.init.written",
		"已生成配置文件：%s\n  provider：%s\n  成员：%s\n接下来：council serve -rw 打开看板，或直接 council 进终端界面。\n",
		target, strings.Join(providerIDs, "、"), strings.Join(agentIDs, "、")))
	return exitOK
}

// initBuiltAgent 是拼好的一个成员。
type initBuiltAgent struct {
	id  string
	cfg config.AgentConfig
}

// buildInitAgent 把请求里的一个成员折成 config.AgentConfig，同时做最小校验。
//
// 默认值刻意保守：permissions=read、mode=discussion。首次运行不该一上来就
// 给写权限；用户可以在设置页里改。
func buildInitAgent(providerID string, req initAgentRequest) (initBuiltAgent, error) {
	id := strings.ToLower(strings.TrimSpace(req.ID))
	if id == "" {
		return initBuiltAgent{}, fmt.Errorf("成员 id 不能为空（provider %s）", providerID)
	}
	if !initAgentIDOK(id) {
		return initBuiltAgent{}, fmt.Errorf("成员 id 只能用字母、数字、下划线与短横线：%s", id)
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return initBuiltAgent{}, fmt.Errorf("成员 %s 需要选一个模型", id)
	}
	perms := strings.TrimSpace(req.Permissions)
	if perms == "" {
		perms = string(config.PermissionRead)
	}
	switch config.Permission(perms) {
	case config.PermissionRead, config.PermissionWorkspace, config.PermissionFull:
	default:
		return initBuiltAgent{}, fmt.Errorf("成员 %s 的 permissions 只能是 read / workspace / full：%s", id, perms)
	}
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = string(config.ModeDiscussion)
	}
	switch config.Mode(mode) {
	case config.ModeDiscussion, config.ModeWorker, config.ModeDiscussionAndWorker,
		config.ModeJudge, config.ModeManager:
	default:
		return initBuiltAgent{}, fmt.Errorf("成员 %s 的 mode 不认识：%s", id, mode)
	}
	display := strings.TrimSpace(req.DisplayName)
	if display == "" {
		display = id
	}
	return initBuiltAgent{id: id, cfg: config.AgentConfig{
		DisplayName: display,
		Provider:    providerID,
		Model:       model,
		Permissions: config.Permission(perms),
		Mode:        config.Mode(mode),
	}}, nil
}

// initAgentIDOK 是成员 id 的白名单：与设置页新建成员同一条规则（小写字母开头，
// 只允许 a-z 0-9 _ -）。这里不追求完整，只挡住会写出坏 TOML 的值。
func initAgentIDOK(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for i, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
		if i == 0 && (r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// validateInitConfig 把文本写进临时目录、按正常加载路径读回来并 Validate。
func validateInitConfig(content string) error {
	dir, err := os.MkdirTemp("", "council-init-")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}
	cfg, err := config.LoadAndValidate(path)
	if err != nil {
		return err
	}
	if len(cfg.Agents) == 0 {
		return errors.New("生成的配置里没有任何成员")
	}
	return nil
}

// readInitInput 读 --apply 的输入：- 表示 stdin，其余按文件。
func readInitInput(source string, stdin io.Reader) ([]byte, error) {
	if source == "-" {
		return io.ReadAll(io.LimitReader(stdin, 1<<20))
	}
	return os.ReadFile(source)
}

// writeInitJSON 输出一行 JSON（不缩进、不转义 HTML，与 who -json 同体例）。
func writeInitJSON(out io.Writer, payload any) error {
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	return enc.Encode(payload)
}

// fileExists 判断一个路径是不是已经存在的文件。
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// initUsageLang 是 -h 与用法错误共用的那一段。
func initUsageLang(lang i18n.Lang) string {
	return i18n.TF(lang, "cli.init.usage", `用法: council init [flags]

检测本机装了哪几家 CLI（claude / codex / grok / agy）、能不能列出模型、登录状态如何；
默认只打印，不写任何文件。桌面版的首次运行向导就是靠它拿到这份结果的。

  council init --json              给程序看的检测结果（不写文件）
  council init --apply <file|->    按 JSON 写配置（配置文件已存在时拒绝）

退出码: 0 成功；1 环境/写盘失败；2 用法错误。

flags:
  -json                按 JSON 输出检测结果
  -apply <file|->      读入勾选结果并写配置；- 表示 stdin
  -config <path>       配置文件路径，默认 ~/.council/config.toml
  -lang zh|en|auto     输出语言，默认 auto
`)
}
