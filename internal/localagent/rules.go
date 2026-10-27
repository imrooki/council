package localagent

import "strings"

// rules.go 是本机 agent 的唯一识别规则表。
// 判据只有三样：可执行文件名、父进程的可执行文件名、映像完整路径所在的
// 目录（S-09：不读命令行、不读环境变量、不读其它 CLI 的配置）。规则是
// 声明式的，测试直接喂人造的 Process 列表给 Identify，不需要枚举真实进程。
//
// **为什么需要路径**：同名的 exe 不止一个。Claude Desktop 也装了
// claude.exe，codex.exe 也可能来自 IDE / app-server。只看基名会把它们
// 全报成 CLI agent。所以规则分成三档证据：
//   - 路径命中已知 CLI 安装位置 → CertaintyExact；
//   - 只有映像名 / 父进程关系 → CertaintyProbable；
//   - 路径命中已知桌面端安装位置 → KindUnknown（明确不报）。
//
// 匹配不是「第一条命中」而是按证据强度取最好的一条（见 identifyOne）：
// 这样规则表的书写顺序不会让一条宽规则偷偷盖住一条强规则。

// Rule 是一条声明式识别规则。
//
// 一条规则命中需要同时满足：进程的基名等于 ExeName（大小写不敏感）；
// ParentExe 为空或等于父进程的基名；PathContains 为空或映像完整路径
// （小写比较）含有其中至少一个片段。映像路径为空时，带 PathContains 的
// 规则一律不命中 —— 拿不到路径就不许拿路径做判断。
//
// Kind 为 KindUnknown 表示这条规则故意把某个名字/目录钉成「不是 agent」，
// 它不会产出记录。Certainty 只对 Kind 是四家 CLI 的规则有意义，且必须是
// exact 或 probable：空 certainty 的识别规则按 0 分处理，永远不会命中
// （TestRulesHaveCertainty 钉住这条）。
type Rule struct {
	ExeName      string
	ParentExe    string
	PathContains []string
	Kind         Kind
	Certainty    Certainty
}

// cliInstallMarkers 返回「这是一个命令行工具安装位置」的路径片段。
// 全部小写，匹配时路径也折成小写。extra 给某一家专属的位置（例如 codex
// 的 .cargo\bin）。
//
// 每次调用返回新切片，避免多条规则共享底层数组被人 append 改坏。
func cliInstallMarkers(extra ...string) []string {
	markers := []string{
		`\npm\`,          // npm -g 的 bin 目录
		`\node_modules\`, // 项目内 / nvm 里的安装
		`\.local\bin\`,   // Claude Code 原生安装器
		`\.bun\bin\`,     // bun 全局安装
		`\pnpm\`,         // pnpm 全局目录
	}
	return append(markers, extra...)
}

// desktopMarkers 返回「这是桌面客户端，不是 CLI agent」的路径片段。
// Claude Desktop 的默认安装目录是 %LOCALAPPDATA%\AnthropicClaude；
// 系统级 / Store 安装可能是 C:\Program Files\Claude 或
// C:\Program Files\WindowsApps\Claude_<版本>_x64__<发布者>（实测路径）。
func desktopMarkers() []string {
	return []string{
		`\anthropicclaude\`,
		`\program files\claude\`,
		`\program files (x86)\claude\`,
		`\appdata\local\programs\claude\`,
		`\windowsapps\claude`,
	}
}

// codexInstallMarkers 比通用 CLI 安装位置更窄：**不认 node_modules**。
//
// 实测里 codex.exe 也可能来自 IDE / app-server 的 node_modules 打包，
// 那种进程不是用户装的 CLI，只能算 probable。只认全局安装位置与 cargo。
func codexInstallMarkers() []string {
	return []string{
		`\npm\`,
		`\.local\bin\`,
		`\.bun\bin\`,
		`\pnpm\`,
		`\.cargo\bin\`,
	}
}

// Rules 是本机 agent 的识别规则表。
//
// 只放四家 CLI 的可执行文件名。**已知限制**：npm 安装的 CLI 是 .cmd 垫片，
// .cmd 本身不是进程（由 cmd.exe 执行命令行），真正跑起来的是 node.exe；
// 不读命令行就无法判断那个 node.exe 在跑哪一家（S-09 明确禁止），所以
// npm 垫片启动的会话在 L1 看不出来，会表现为「本机没有这个 agent」。这是
// 诚实的能力边界，不是漏判：Council 不猜。
//
// 上限：Claude Desktop / IDE / app-server 里的同名进程一律不标 exact；
// 路径命中桌面端目录的直接不报；路径拿不到的标 probable。
//
// 清单是 Provisional 的：真实机器上的 exe 名与安装目录要按 Plan v3 附录 C
// 第 2 条先跑一次裸的 council who 再补；补的时候只加精确名字/目录片段，
// 不加子串匹配的 exe 名。
var Rules = []Rule{
	// ---- claude ----
	// Claude Desktop 也装了 claude.exe，它不是 CLI agent。这条规则命中
	// 之后按 KindUnknown 处理（不报）；证据强度排在 probable 之上、exact
	// 之下，见 identifyOne 的 ruleRank。
	{ExeName: "claude.exe", PathContains: desktopMarkers(), Kind: KindUnknown},
	// 已知 CLI 安装位置：npm 全局、node_modules、原生安装器的 .local\bin。
	{ExeName: "claude.exe", PathContains: cliInstallMarkers(), Kind: KindClaude, Certainty: CertaintyExact},
	// 父进程是 node.exe：npx / npm 包装器起 CLI。只有进程关系证据，
	// 没有路径证据 → probable。
	{ExeName: "claude.exe", ParentExe: "node.exe", Kind: KindClaude, Certainty: CertaintyProbable},
	{ExeName: "claude.exe", Kind: KindClaude, Certainty: CertaintyProbable},

	// ---- codex ----
	// Rust 二进制通常装在 ~/.cargo/bin，也可能是 npm 全局包；IDE / app-server
	// 的 node_modules 里带出来的 codex.exe 不在这些目录，只能 probable。
	{ExeName: "codex.exe", PathContains: codexInstallMarkers(), Kind: KindCodex, Certainty: CertaintyExact},
	{ExeName: "codex.exe", ParentExe: "node.exe", Kind: KindCodex, Certainty: CertaintyProbable},
	{ExeName: "codex.exe", Kind: KindCodex, Certainty: CertaintyProbable},

	// ---- grok ----
	{ExeName: "grok.exe", PathContains: cliInstallMarkers(), Kind: KindGrok, Certainty: CertaintyExact},
	{ExeName: "grok.exe", ParentExe: "node.exe", Kind: KindGrok, Certainty: CertaintyProbable},
	{ExeName: "grok.exe", Kind: KindGrok, Certainty: CertaintyProbable},

	// ---- agy ----
	{ExeName: "agy.exe", PathContains: cliInstallMarkers(), Kind: KindAgy, Certainty: CertaintyExact},
	{ExeName: "agy.exe", ParentExe: "node.exe", Kind: KindAgy, Certainty: CertaintyProbable},
	{ExeName: "agy.exe", Kind: KindAgy, Certainty: CertaintyProbable},

	// ---- 通用宿主：明确认不出 ----
	// 列出来是为了让「node.exe 本身不是 agent」成为规则表里的一条可测
	// 事实，而不是一条只写在注释里的口头约定。注意 claude.exe 的父进程
	// 是 node.exe 时，被识别的是 claude.exe 那一条，node.exe 父进程自己
	// 仍然走这里、仍然不报。
	{ExeName: "node.exe", Kind: KindUnknown},
	{ExeName: "python.exe", Kind: KindUnknown},
	{ExeName: "python3.exe", Kind: KindUnknown},
	{ExeName: "pwsh.exe", Kind: KindUnknown},
	{ExeName: "powershell.exe", Kind: KindUnknown},
	{ExeName: "cmd.exe", Kind: KindUnknown},
}

// Identify 把一份进程快照按 Rules 归约成 agent 列表。
//
// 这是纯函数：不碰系统、不读文件、不发网络请求，测试可以完全用编造的
// Process 驱动。同一个 PID 只产出一条（枚举本身不会重复，但调用方可能
// 拼错快照）。认不出的进程直接跳过，不生成 KindUnknown 的记录。
//
// 父进程不在快照里时（父进程已经退出），带 ParentExe 的规则不命中，
// 宽规则仍按进程自己的名字判定 —— 不因为父进程没了就丢掉一个 claude。
func Identify(processes []Process) []Agent {
	byPID := make(map[int]Process, len(processes))
	for _, p := range processes {
		if p.PID > 0 {
			byPID[p.PID] = p
		}
	}

	agents := make([]Agent, 0, len(processes))
	seen := make(map[int]struct{}, len(processes))
	for _, p := range processes {
		kind, certainty, matched := identifyOne(p, byPID)
		if !matched || kind == "" || kind == KindUnknown {
			continue
		}
		if _, dup := seen[p.PID]; dup {
			continue
		}
		seen[p.PID] = struct{}{}
		agents = append(agents, Agent{
			Kind:      kind,
			PID:       p.PID,
			PPID:      p.PPID,
			ExeName:   p.ExeName,
			ExePath:   p.ExePath,
			StartedAt: p.StartedAt,
			Source:    SourceObserved,
			Certainty: certainty,
		})
	}
	sortAgents(agents)
	return agents
}

// identifyOne 在所有命中的规则里取证据最强的一条。
//
// 不按表顺序取第一条：路径 exact 规则与「仅仅名字对上」的宽规则会同时
// 命中，靠顺序保证正确性太脆（有人调一下 Rules 的次序就会静默降级）。
// 证据强度见 ruleRank；同分时保留先出现的那条。
func identifyOne(p Process, byPID map[int]Process) (Kind, Certainty, bool) {
	exe := baseName(p.ExeName)
	if exe == "" {
		return "", "", false
	}
	parent := ""
	if pp, ok := byPID[p.PPID]; ok {
		parent = baseName(pp.ExeName)
	}

	bestKind, bestCertainty := Kind(""), Certainty("")
	bestRank := 0
	for _, rule := range Rules {
		if !rule.matches(p, exe, parent) {
			continue
		}
		if rank := ruleRank(rule); rank > bestRank {
			bestKind, bestCertainty, bestRank = rule.Kind, rule.Certainty, rank
		}
	}
	if bestRank == 0 {
		return "", "", false
	}
	return bestKind, bestCertainty, true
}

// ruleRank 是证据强度：exact > 明确不认 › probable > 未命中。
//
// 「明确不认」排在 probable 之上，是为了让 Claude Desktop 的 claude.exe
// 不被同名的宽规则（probable）重新捡起来；exact 又排在它之上，因为路径
// 同时命中 CLI 安装位置时以更强的 CLI 证据为准。
func ruleRank(rule Rule) int {
	if rule.Kind == KindUnknown {
		return 3
	}
	switch rule.Certainty {
	case CertaintyExact:
		return 4
	case CertaintyProbable:
		return 2
	default:
		// 识别规则没写 certainty 是表写错了：宁可漏报，也不猜一个确定性。
		return 0
	}
}

// matches 判断这条规则是否命中一个进程。exe / parent 已经折成小写基名。
func (r Rule) matches(p Process, exe, parent string) bool {
	if baseName(r.ExeName) != exe {
		return false
	}
	if r.ParentExe != "" && baseName(r.ParentExe) != parent {
		return false
	}
	if len(r.PathContains) == 0 {
		return true
	}
	path := strings.ToLower(strings.TrimSpace(p.ExePath))
	if path == "" {
		// 拿不到路径时，依赖路径的判断一律不成立。
		return false
	}
	for _, marker := range r.PathContains {
		if strings.Contains(path, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}
