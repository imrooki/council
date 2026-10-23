package repl

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
)

// addmodel.go 是 /add 的「识别出的模型」一半（00_DECISIONS.md #45 第一节）。
//
// 识别只在不带参数的 /add、/add --refresh，以及按名字加识别出的模型而本会话
// 还没有快照时（AD1，addname.go）做，不在启动时做。每家一次、串行，各自带
// discoverTimeout；一家失败不拦别家，失败行随快照在不带参数的 /add 里打，每家
// 一行。识别中途被取消时不存快照，见 takeModelSnapshot 与 ensureModelSnapshot。

// discoveryTimeout 是每一家识别的超时。New 把它装进 REPL.discoverTimeout。
const discoveryTimeout = 20 * time.Second

// msgAddModelUsage 是 +编号 与 --refresh 的参数形状不对时打的那一行。
const msgAddModelUsage = "用法: /add +<编号> [--id <新 id>] [--effort <强度>]；/add --refresh 重新识别"

// modelSnapshot 是本会话的识别快照。识别被取消的那一次不存。
//
// 编号在拍快照时定：provider 按字典序，每家按 CLI 的输出顺序，只给拍快照那
// 一刻还不在配置里的行编号。之后每次显示都按当前 r.cfg 滤掉已经在配置里的行，
// 不重新编号，所以两次 /add 之间编号不漂。/config reload --apply 不作废快照，
// 只影响过滤；/add --refresh 才重拍。
//
// logins 是各家的登录状态（OB3，addlogin.go），与识别同一次拍，顺序与 pids 相同；
// 没装登录状态执行器时为 nil。
type modelSnapshot struct {
	rows     []snapRow
	failures []snapFailure
	okCount  int
	custom   int
	logins   []snapLogin
}

// snapRow 是快照里的一行。no 为 0 表示拍快照时这一行已经在配置里，不给编号。
type snapRow struct {
	no int
	m  provider.DiscoveredModel
}

// snapFailure 是一家识别失败的记录，随快照留到本会话结束。
type snapFailure struct {
	provider string
	err      error
}

// discoverableProviders 只看配置里写了的 provider，按字典序。
// provider.Lookup 找不到（没有适配器）或 ModelDiscovery 不可用的，一律静默
// 跳过。四家已登记的适配器今天都声明了能跑的识别方式。
func discoverableProviders(cfg *config.Config) []string {
	var out []string
	for _, pid := range sortedKeys(cfg.Providers) {
		p, err := provider.Lookup(pid)
		if err != nil {
			continue
		}
		if !p.Capabilities().ModelDiscovery.Available() {
			continue
		}
		out = append(out, pid)
	}
	return out
}

// configuredAgentFor 判这一行要不要显示，只看 provider 与 model。model 等于
// DiscoveredModel.Model，或 Alias 非空且 model 等于 Alias，都算已在配置里
// （别名不等于识别结果，见契约 §0 第 6 条）。
//
// sameBrainAgent 判这次加入是不是与已有成员同一个脑子，在上面的判据上再加强度。
// 所以某一行只要拍快照那一刻还不在配置里、因而拿到了编号，它的模型以某个强度
// 加入、行被藏起来之后，/add +编号 --effort 另一档仍然能用同一个编号再加一个
// 强度。这类行包括本会话 /add +编号 加进来的，也包括快照之后手改配置、再
// /config reload --apply 进来的。拍快照时已在配置里的行根本没有编号，没有 +N
// 可用；/add --refresh 或开新会话重拍之后，上面那类行也算已在配置里，同样失去
// 编号。这些模型换强度用 /add <ID>（按名字加识别出的模型，AD1，addname.go）：
// 快照里的每一行都能按名字加，不看有没有编号。开关关着时（直接 New 出来的 REPL）
// 仍然只能手改配置（16a open item (c)）。
func configuredAgentFor(cfg *config.Config, m provider.DiscoveredModel) string {
	for _, id := range sortedKeys(cfg.Agents) {
		a := cfg.Agents[id]
		if a.Provider != m.Provider {
			continue
		}
		if a.Model == m.Model || (m.Alias != "" && a.Model == m.Alias) {
			return id
		}
	}
	return ""
}

// sameBrainAgent 的判据见 configuredAgentFor：多比一项强度。
func sameBrainAgent(cfg *config.Config, m provider.DiscoveredModel, effort string) string {
	for _, id := range sortedKeys(cfg.Agents) {
		a := cfg.Agents[id]
		if a.Provider != m.Provider || a.Effort != effort {
			continue
		}
		if a.Model == m.Model || (m.Alias != "" && a.Model == m.Alias) {
			return id
		}
	}
	return ""
}

// takeModelSnapshot 按调用方给出的 provider 顺序串行识别。
//
// 循环开头判父 ctx，已取消就 break，后面几家不再起进程。判的必须是父 ctx，
// 不能是每家自己的 cctx：cancel 之后 cctx.Err 永远非 nil，判它等于每家识别完
// 就停。
//
// 这一处只管取消之后不再起后面几家。取消落在最后一家识别期间时，循环不会再
// 回到开头，不存快照要靠 ensureModelSnapshot 拍完之后那一处。这时前几家的
// 成功行已经在快照里，照样整份丢掉，不能只在一行都没拿到、或循环提前结束时
// 才算取消（R14 late）。两处判断缺一不可。
//
// 装了登录状态执行器时（OB3），每家识别之后接着判登录状态，codex 还要另跑一次登录
// 状态命令；它算在这一家里，期间的取消同样由上面两处接住，整份快照丢掉。
func takeModelSnapshot(ctx context.Context, r *REPL, pids []string) *modelSnapshot {
	snap := &modelSnapshot{}
	next := 1
	for _, pid := range pids {
		if ctx.Err() != nil {
			break
		}
		cctx, cancel := context.WithTimeout(ctx, r.discoverTimeout)
		// 经一层闭包截下 stdout（DiscoverModels 不交回原始输出），登录状态从它判（OB3）。
		var stdout []byte
		capture := func(c context.Context, cmd provider.DiscoveryCommand) ([]byte, error) {
			out, err := r.discover(c, cmd)
			stdout = out
			return out, err
		}
		models, err := provider.DiscoverModels(cctx, pid, r.cfg.Providers[pid].Command, capture)
		// 识别失败的那一家也判。cancel 必须在这之后：登录状态命令与识别共用 cctx。
		if r.loginStatus != nil {
			snap.logins = append(snap.logins, snapLogin{provider: pid, info: snapshotLogin(cctx, r, pid, stdout)})
		}
		cancel()
		if err != nil {
			snap.failures = append(snap.failures, snapFailure{provider: pid, err: err})
			continue
		}
		snap.okCount++
		for _, m := range models {
			// custom/ 开头的 codex 行不编号、不进快照的行，只计数（printModelSnapshot 表后说明）。
			if m.Custom {
				snap.custom++
				continue
			}
			row := snapRow{m: m}
			if configuredAgentFor(r.cfg, m) == "" {
				row.no = next
				next++
			}
			snap.rows = append(snap.rows, row)
		}
	}
	return snap
}

// printDiscoveredModels 只在本会话还没有快照时识别（ensureModelSnapshot）。一家能
// 识别的都没有时直接返回，不打「正在识别模型」。识别被取消时只有「识别被取消」
// 那一行，不打表、不打登录段。
func printDiscoveredModels(ctx context.Context, r *REPL) error {
	pids := discoverableProviders(r.cfg)
	if len(pids) == 0 {
		return nil
	}
	if canceled, err := ensureModelSnapshot(ctx, r, pids); canceled || err != nil {
		return err
	}
	if err := printModelSnapshot(r, r.modelSnap); err != nil {
		return err
	}
	return printLoginStatus(r, r.modelSnap)
}

// ensureModelSnapshot 在本会话还没有快照时按 pids 识别一次、存下；已有快照什么都
// 不做（/add --refresh 才重拍）。不带参数的 /add（printDiscoveredModels）与按名字加
// 识别出的模型（AD1，addname.go）共用它：两条路是同一份快照、同一句「正在识别模型」、
// 同一个 discoverTimeout、同一套取消语义。
//
// 拍完之后父 ctx 已取消，就不赋给 r.modelSnap，只打「识别被取消」那一行、返回
// canceled 为真，下次 /add 重新识别。TUI 里提交在跑时按 Ctrl-C，取消的是这一次提交
// 的 ctx（tui/model.go 里忙时按 Ctrl-C 那一支调 m.cancel）。
//
// 小竞态：三家都跑完之后才到的取消，也会丢掉这一份完整结果。代价只是下次
// 多识别一次，接受。
//
// err 只来自写屏幕；canceled 为真时 err 是打「识别被取消」那一行的结果。
func ensureModelSnapshot(ctx context.Context, r *REPL, pids []string) (canceled bool, err error) {
	if r.modelSnap != nil {
		return false, nil
	}
	if err := r.printf("正在识别模型: %s\n", strings.Join(pids, ", ")); err != nil {
		return false, err
	}
	snap := takeModelSnapshot(ctx, r, pids)
	if ctx.Err() != nil {
		return true, r.printf("识别被取消；下次 /add 会重新识别\n")
	}
	r.modelSnap = snap
	return false, nil
}

// printModelSnapshot 按当前 r.cfg 再滤一次，只显示有编号且现在仍不在配置里的行，
// 编号沿用拍快照时的值，不重排。
//
// 没登录、瞬时超时这类失败随快照留到本会话结束，所以有失败时在失败行之后再打
// 一行 --refresh 提示。这一行随快照每次重印：用户照「没登录？」登录之后再敲
// /add，看到的仍是旧结果，得知道怎么重来。
func printModelSnapshot(r *REPL, snap *modelSnapshot) error {
	var visible []snapRow
	for _, row := range snap.rows {
		if row.no > 0 && configuredAgentFor(r.cfg, row.m) == "" {
			visible = append(visible, row)
		}
	}
	if len(visible) > 0 {
		if err := r.printf("识别出、配置里还没有的模型（本会话的快照，/add --refresh 重新识别）:\n"); err != nil {
			return err
		}
		tw := r.newTabWriter()
		fmt.Fprintln(tw, "NO\tPROVIDER\tID\tMODEL\tEFFORTS\tSOURCE")
		for _, row := range visible {
			m := row.m
			fmt.Fprintf(tw, "+%d\t%s\t%s\t%s\t%s\t%s\n",
				row.no, m.Provider, m.SuggestedID(m.DefaultEffort()), m.Model, effortsCell(m.Efforts), m.Source.Label())
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		// 可见行里有 codex 的行时，提示里带上 codex；没有时与 MD1 的原句逐字相同。
		who := "claude"
		for _, row := range visible {
			if row.m.Provider == "codex" {
				who = "claude 与 codex"
				break
			}
		}
		if err := r.printf("用 /add +编号 加入；--id <新 id> 改名；%s 可加 --effort <强度> 换档，EFFORTS 列是它支持的档\n", who); err != nil {
			return err
		}
	} else if snap.okCount > 0 {
		if err := r.printf("识别出的模型都已经在配置里了\n"); err != nil {
			return err
		}
	}
	// 只在确实跳过了 custom/ 行时说明一句，带数量。
	if snap.custom > 0 {
		if err := r.printf("codex 目录里还有 %d 个 custom/ 开头的模型没有列出：它们是你在 codex 配置里自定义转发的模型，多半是别家的（例如 grok、gemini），要用就加那一家的成员\n", snap.custom); err != nil {
			return err
		}
	}
	for _, f := range snap.failures {
		if err := r.printf("识别 %s 的模型失败: %s\n", f.provider, discoveryFailureText(f.err, r.discoverTimeout)); err != nil {
			return err
		}
	}
	if len(snap.failures) > 0 {
		return r.printf("失败的结果也会留在本会话的快照里；要重新识别用 /add --refresh\n")
	}
	return nil
}

func effortsCell(efforts []string) string {
	if len(efforts) == 0 {
		return emptyCell
	}
	return strings.Join(efforts, "|")
}

// discoveryFailureText 是四种失败的中文说法。ErrDiscoveryFailed 与其它未归类
// 的错误原样打 err.Error()。
func discoveryFailureText(err error, timeout time.Duration) string {
	switch {
	case errors.Is(err, provider.ErrDiscoveryTimeout):
		return fmt.Sprintf("超时（%v 内没有回来）", timeout)
	case errors.Is(err, provider.ErrDiscoveryEmpty):
		return "一个模型都没列出来（没登录？）"
	case errors.Is(err, provider.ErrDiscoveryUnparseable):
		return "输出认不出来（CLI 版本变了？）"
	default:
		return err.Error()
	}
}

// row 按拍快照时定下的编号找行。n < 1 或没有这个编号都算没有。
func (s *modelSnapshot) row(n int) (snapRow, bool) {
	if n < 1 {
		return snapRow{}, false
	}
	for _, row := range s.rows {
		if row.no == n {
			return row, true
		}
	}
	return snapRow{}, false
}

// cmdAddDiscovered 处理 /add +编号。顺序冻结：参数形状，Runner 在不在，有没有
// 快照（没有时先看配置里有没有能识别的 provider，一家都没有就直说，不叫用户
// 先敲 /add），编号；从强度起交给 addDiscoveredModel（按名字加识别出的模型，AD1，
// 走的也是它）。每一步失败都只打一行。
func cmdAddDiscovered(ctx context.Context, r *REPL, fields []string) error {
	tok := fields[0]
	if !isASCIIDigits(tok[1:]) {
		return r.printf("%s\n", msgAddModelUsage)
	}
	var newID, effort string
	haveID, haveEffort := false, false
	for i := 1; i < len(fields); i++ {
		switch fields[i] {
		case "--id":
			if haveID || i+1 >= len(fields) {
				return r.printf("%s\n", msgAddModelUsage)
			}
			haveID = true
			newID = fields[i+1]
			i++
		case "--effort":
			if haveEffort || i+1 >= len(fields) {
				return r.printf("%s\n", msgAddModelUsage)
			}
			haveEffort = true
			effort = fields[i+1]
			i++
		default:
			return r.printf("%s\n", msgAddModelUsage)
		}
	}
	if r.rn == nil {
		return r.printf("%s\n", msgNoRunner)
	}
	if r.modelSnap == nil {
		if len(discoverableProviders(r.cfg)) == 0 {
			return r.printf("配置里没有能识别模型的 provider，/add +编号 用不了\n")
		}
		return r.printf("还没有识别结果：先敲 /add 列出识别出的模型，再用 /add +编号 加入\n")
	}
	n, err := strconv.Atoi(tok[1:])
	row, ok := snapRow{}, false
	if err == nil {
		row, ok = r.modelSnap.row(n)
	}
	if !ok {
		return r.printf("识别结果里没有 %s，不带参数的 /add 能列出现有编号\n", tok)
	}
	return addDiscoveredModel(ctx, r, row.m, effort, haveEffort, newID, haveID, tok)
}

// addDiscoveredModel 把识别出的一行 m 以某个强度写进配置并加入当前会话：/add +编号
// （cmdAddDiscovered）与按名字加（AD1，addname.go 的 addByOneName）共用它，所以
// 两条路的屏幕输出与写进文件的字节相同。调用方已经判过参数形状、Runner 在不在、
// 快照与这一行。haveEffort 为假时 effort 不看；haveID 为假时 newID 不看。tok 是
// 守卫拒绝时「再 /add <tok>」里的那一段：+编号 是 +N 那个词，按名字加是敲的名字，
// 两种都不带 --id、--effort 与第二个词。
//
// 顺序冻结（接着调用方的那几步）：强度，同脑子，id，配置文件路径，守卫（先
// data_dir 与 rules_file，再整份比），追加，加载校验，换指针，Join。每一步失败
// 都只打一行。
//
// 失败之后留下什么，按阶段分：
//
// 备份之前（参数到守卫，以及 AppendBlock 的读原文件阶段）：盘上文件与内存一个不动。
//
// AppendBlock 的备份阶段：一般不留文件，屏幕是「写配置文件失败: 备份: 」后接
// 备份错误。R16 走的是 99 个名字全被占这一种。例外：备份文件已经建好、写到一半
// 失败（例如磁盘满）时，writeBackup 返回空串，那个残缺文件留在盘上，路径不上屏
// （B1 注释要点 7、open item (p)）。
//
// 备份之后：写入阶段失败时原文件没有被替换（临时文件加改名，改名失败原文件原样）；
// 写后加载校验不过时用 RestoreFile 写回原字节。两种都留着备份，屏幕那一行带上
// 它的路径（R15、R8）。
//
// 换指针之后只剩 Join。它失败时配置已经写进去、指针已经换了，不回滚：配置本身
// 合法，用户改天 /add <id> 就能加入。
//
// 新成员固定 permissions 为 read、mode 为 discussion（#45）。role 为空串
// （见 append.go 里 AgentBlockTOML 的注释）。display_name 与 id 取 provider 包
// 的建议，id 可用 --id 改。
func addDiscoveredModel(ctx context.Context, r *REPL, m provider.DiscoveredModel, effort string, haveEffort bool, newID string, haveID bool, tok string) error {
	switch {
	// claude 与 codex 的强度在 council 里选，只给该模型自己支持的档（codex 的档已由识别层按模型收窄）。
	case m.Provider != "claude" && m.Provider != "codex":
		if haveEffort {
			return r.printf("%s 的强度不在 council 里选（grok 写在它自己的配置里，agy 写在模型名里），不能带 --effort\n", m.Provider)
		}
		effort = ""
	case !haveEffort:
		effort = m.DefaultEffort()
	case len(m.Efforts) == 0:
		return r.printf("%s 没有可选的强度，不能带 --effort\n", m.Model)
	default:
		found := false
		for _, e := range m.Efforts {
			if e == effort {
				found = true
				break
			}
		}
		if !found {
			return r.printf("%s 支持的强度是 %s，没有 %s\n", m.Model, strings.Join(m.Efforts, "|"), effort)
		}
	}

	if dup := sameBrainAgent(r.cfg, m, effort); dup != "" {
		return r.printf("配置里已经有同一个模型、同一强度的 agent %s，直接 /add %s\n", dup, dup)
	}
	if !haveID {
		newID = m.SuggestedID(effort)
	}
	if problem := config.NewAgentIDProblem(r.cfg, newID); problem != "" {
		return r.printf("%s\n", problem)
	}

	path := r.rulesCtx.ConfigPath
	if path == "" {
		return r.printf("不知道配置文件在哪，没法写入\n")
	}
	// 写回永远只落最高优先层那一份文件（默认就是项目配置），绝不把合并
	// 结果整体落盘——那会把全局层的 providers 抄进项目文件。reloadLayers
	// 保证第一项就是那份文件，且与启动时生效的规则同一套（见
	// configlayers.go）。不在这里另拼路径：路径只有一个出处。
	layers := reloadLayers(path)
	writePath := layers[0].Path
	// 守卫写之前重读同一组层（合并版），与 r.cfg 比，分两支，顺序冻结。
	//
	// 先比 DataDir 与 RulesFile。不同就打「只能重启生效」那一行，不带编号：
	// 这两个键 /config reload 会拒绝（configcmd.go 的 cmdConfigReload 里 data_dir /
	// rules_file 那一支），叫用户先 reload
	// 是死胡同；重启之后没有快照，编号要重新看。
	//
	// 再做整份 reflect.DeepEqual。不相等说明文件在上次加载之后被改过（改的是
	// 注释之外的东西），这时追加会把用户没应用的改动一起生效，所以拒绝，让用户
	// 先 /config reload --apply。只改注释不算改过：按加载结果比，不按字节比。
	// 必须整份比。漏比哪个字段，那个字段在文件里没应用的改动就会随这次追加
	// 悄悄换进内存。Providers 尤其要紧：/config reload 换 command 时会响亮告警
	// （providerCommandChanges），这里一个字都不会打。
	//
	// 比较基准是合并后的配置（LoadMerged），不是单文件 Load：项目层只写
	// [agents.x] 时单文件读出来缺 provider、与内存里的合并结果永远不等，
	// 守卫就会把一次本来合法的追加拒掉。
	//
	// R7 的六个子测试各只动 Config 的一个字段：agent、store_thought、teams、
	// providers 钉整份比，data_dir、rules_file 钉前一支。Summary 字段由
	// fux_review_test.go 的 TestFuxAddGuardCoversSummary 钉着；以后再给 Config
	// 加字段，仍要各配一条用例。
	//
	// 守卫不在 LoadMerged 之外另跑一遍 Validate：校验只发生在 LoadMerged
	// 内部（与 app/serve 同一个口径）。文件被改得通不过校验时，这里打的是
	// 加载失败那一行，而不是「先 reload」——reload 同样会拒绝并列出校验
	// 错误（open item (r)）。
	onDisk, err := config.LoadMerged(layers)
	if err != nil {
		return r.reportErr(configReadErrPrefix("读配置文件失败", layers), err)
	}
	if onDisk.Config.DataDir != r.cfg.DataDir || onDisk.Config.RulesFile != r.cfg.RulesFile {
		return r.printf("配置文件里改了 data_dir 或 rules_file，这两个键只能重启生效：重启 council 之后先敲 /add 看编号，再用 /add +编号 加入\n")
	}
	if !reflect.DeepEqual(onDisk.Config, r.cfg) {
		if len(layers) > 1 {
			// 多层时补一句比较范围：否则用户不知道该去改哪一份文件。
			return r.printf("配置文件在上次加载之后被改过（比较的是合并后的 %d 层：%s），先 /config reload --apply 让它生效，再 /add %s\n",
				len(layers), strings.Join(onDisk.Layers, "、"), tok)
		}
		return r.printf("配置文件在上次加载之后被改过，先 /config reload --apply 让它生效，再 /add %s\n", tok)
	}

	ac := config.AgentConfig{
		DisplayName: m.SuggestedDisplayName(effort),
		Provider:    m.Provider,
		Model:       m.Model,
		Effort:      effort,
		Role:        "",
		Permissions: config.PermissionRead,
		Mode:        config.ModeDiscussion,
	}
	comment := fmt.Sprintf("council /add 追加：%s 识别出的 %s（%s）", m.Provider, m.Model, m.Source.Label())
	backup, orig, err := config.AppendBlock(writePath, config.AgentBlockTOML(newID, ac, comment))
	if err != nil {
		if backup != "" {
			return r.printf("写配置文件失败（原文件备份在 %s）: %v\n", backup, err)
		}
		return r.reportErr("写配置文件失败", err)
	}

	// 写后再合并一次（不是 LoadAndValidate 单文件）：追加块引用的 provider
	// 可能只在全局层，单文件校验会把这次合法的写入判成非法并回滚。合并版
	// 与 app/serve 校验的是同一份东西。
	merged, err := config.LoadMerged(layers)
	if err != nil {
		if rerr := config.RestoreFile(writePath, orig); rerr != nil {
			return r.printf("写入后的配置没通过加载或校验，恢复原文件也失败了（备份在 %s）: %v；%v\n", backup, err, rerr)
		}
		return r.printf("写入后的配置没通过加载或校验，已恢复原文件（备份在 %s）: %v\n", backup, err)
	}
	newCfg := merged.Config

	// 写后换指针与 /config reload --apply 同一套三句（configcmd.go 的 cmdConfigReload
	// 里 --apply 的换指针三句）：
	// SetConfig、换 r.cfg、SetThoughtPolicy。两处持有者一起换，绝不原地改
	// r.cfg.Agents。不跑 AuditDrift：追加的只是一张新表，守卫又保证写前文件与
	// 内存一致，已有成员的配置不变。
	//
	// 层与 rules_file 基准也一起换：重读的是合并结果，r.cfg 里现在带着全局层
	// 的 provider，若不同步这两个字段，/config 与 /rules reload 会继续按启动
	// 时那套坐标说话，与内存里的配置对不上。
	//
	// 新 id 若与库里遗留的旧 agent 同名（以前用过、后来从配置删掉，库里还留着
	// 参与者行与 provider 会话），Join 是 upsert，会把那一行重新激活。旧 provider
	// 会话看身份指纹与这次写的配置对不对得上，两种都不在这里跑 AuditDrift：
	// 对得上时（当初也是同一模型、同一强度，写出的那段逐字相同），下次 /ask
	// 续上旧会话，不打「新开了一条 provider 会话」。对不上时（role、display_name、
	// model 或强度不同，例如用 --id 以另一个强度占回同一个 id），下次 /ask
	// 按漂移拒绝（outcome refused），要重建才能接着用。
	r.rn.SetConfig(newCfg)
	r.cfg = newCfg
	r.cfgLayers = merged.Layers
	r.rulesFileBase = merged.RulesFileBase
	r.svc.SetThoughtPolicy(newCfg.StoreThought)

	if err := r.printf("已把 agent %s 追加进 %s（原文件备份在 %s）\n", newID, writePath, backup); err != nil {
		return err
	}
	joined := newCfg.Agents[newID]
	p := conversation.Participant{
		ConvID:      r.cur.ID,
		AgentID:     newID,
		DisplayName: joined.DisplayName,
		Provider:    joined.Provider,
		Active:      true,
		JoinedAt:    time.Now().UTC(),
	}
	if err := r.svc.Join(ctx, r.cur.ID, p); err != nil {
		return r.reportErr("加入失败", err)
	}
	return r.printf("%s 已加入会话 %s\n", newID, r.cur.ID)
}
