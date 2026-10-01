package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"council/internal/config"
	"council/internal/provider"
	"council/internal/repl"
	"council/internal/turn"
)

// firstrun.go 是首次运行自动配置（00_DECISIONS.md #48，OB1）。只在默认配置位置、
// 文件不存在时由 Run 调用：找出本机装了哪几家工具，用现有的模型识别（零花费）
// 检查能不能用、看各家的登录状态，生成只含 data_dir 与 [providers.*] 的配置，然后 Run 照常加载它、
// 进 REPL。这推翻了 Step 4「不替用户创建配置文件」，但只在默认位置、文件不存在时；
// 用户用 --config 或 COUNCIL_CONFIG 指定了路径却找不到，照旧报错。
//
// 全部输出写 Run 的 errOut：TUI 模式下 Run 的 out 是出口为 nil 的 LineWriter，
// 行要等 tui.Run 才吐出来，两种失败结局在那之前就返回了，写进 out 的字到不了屏幕；
// errOut 在 Program 起来之前直写终端，而且单流，顺序不会乱。

// firstRunTimeout 是首次运行每一家的超时（识别与登录状态命令共用一个），与 /add 的识别同值。
// 测试可以改短。
var firstRunTimeout = 20 * time.Second

// firstRunDiscover 是首次运行识别用的执行器，生产是 turn.RunDiscovery。
var firstRunDiscover provider.DiscoveryExec = turn.RunDiscovery

// firstRunStatus 是首次运行跑登录状态命令用的执行器，生产是 turn.RunLoginStatus。
var firstRunStatus provider.LoginStatusExec = turn.RunLoginStatus

// errFirstRunCanceled 表示识别期间 ctx 被取消、没有写文件。Run 见到它返回 nil，
// 与 REPL 被取消同一个处置。
var errFirstRunCanceled = errors.New("首次运行被取消")

// msgFirstRunNoneFound 是一家都没找到时的说明。含 %，用 io.WriteString 写：
// 交给 fmt.Fprint 时 go vet 的 printf 检查会把 %U 当成格式动词报错（go test 默认跑这一项）。
// npm 包名是本机 npm 全局目录里 claude.cmd、codex.cmd 对应的包；grok 与 agy 在本机
// 没有安装方式的证据，不写网址。
const msgFirstRunNoneFound = "一个都没找到，没有生成配置文件。至少装一个、在终端里登录好，再运行 council：\n" +
	"  claude  npm install -g @anthropic-ai/claude-code，装好后运行 claude auth login\n" +
	"  grok    照 grok 官方的说明安装，装好后运行 grok login\n" +
	"  agy     照 agy 官方的说明安装，装好后运行 agy，按提示登录\n" +
	"  codex   npm install -g @openai/codex，装好后运行 codex login\n" +
	`council 找的是 PATH，以及这几处默认安装位置：%APPDATA%\npm、%USERPROFILE%\.grok\bin、%LOCALAPPDATA%\agy\bin` + "\n"

// firstRunRow 是一家的检测、识别与登录状态。models 只数不是 custom/ 的行，与 /add 的识别表一致。
type firstRunRow struct {
	d      config.DetectedProvider
	models int
	err    error
	login  provider.LoginInfo
}

// firstRunLoginText 是结果行末尾的登录状态。它就是 repl.LoginText（OB3）：四档说法只有
// 那一份，/add 的登录段用的是同一个函数。结果之后列出的登录办法同样只有 repl.LoginCommand
// 那一份；msgFirstRunNoneFound 里的安装说明另写着各家的登录办法，首次运行的输出不变，不动它。
var firstRunLoginText = repl.LoginText

// firstRunCheck 是一家的识别与登录状态，在各自的 goroutine 里跑，识别与登录状态命令共用
// 一个 firstRunTimeout。识别经一层闭包截下 stdout（DiscoverModels 不交回原始输出），
// claude 与 grok 的登录状态从它判；codex 另跑一次 codex login status（识别之后、而且
// 这一家的时间还没用完时才跑），其余查不出。
func firstRunCheck(ctx context.Context, d config.DetectedProvider) firstRunRow {
	cctx, cancel := context.WithTimeout(ctx, firstRunTimeout)
	defer cancel()
	var stdout []byte
	capture := func(c context.Context, cmd provider.DiscoveryCommand) ([]byte, error) {
		out, err := firstRunDiscover(c, cmd)
		stdout = out
		return out, err
	}
	models, err := provider.DiscoverModels(cctx, d.ID, d.Path, capture)
	row := firstRunRow{d: d, err: err, login: provider.LoginFromDiscovery(d.ID, stdout)}
	for _, m := range models {
		if !m.Custom {
			row.models++
		}
	}
	if sc, ok := provider.LoginStatusCommandFor(d.ID, d.Path); ok && cctx.Err() == nil {
		if code, so, se, serr := firstRunStatus(cctx, sc); serr == nil {
			row.login = provider.ParseLoginStatus(d.ID, code, so, se)
		}
	}
	return row
}

// firstRun 的顺序冻结：打头行；检测；一家都没有就列四行「没找到」、打说明、返回错误；
// 否则打「正在检查」，四家并行识别并看登录状态（各自带 firstRunTimeout），全部回来之后
// ctx 已取消就打一行、返回 errFirstRunCanceled，不写文件；再按固定顺序打四行结果；
// 找到的里面有没登录或查不出的，打一句说明和那几家的登录办法；最后建文件。文件已存在
// （检查期间被别人建了）就说一句、返回 nil，由 Run 加载那个文件；写不进去就把整段内容
// 打出来、返回错误；写成了就打上手三步、返回 nil。
func firstRun(ctx context.Context, w io.Writer, path string, noAuto bool) error {
	fmt.Fprintf(w, "第一次运行：还没有配置文件 %s，council 来生成一份。\n", path)
	detected := config.DetectProviders()
	var ids []string
	for _, d := range detected {
		if d.Path != "" {
			ids = append(ids, d.ID)
		}
	}
	if len(ids) == 0 {
		for _, d := range detected {
			fmt.Fprintf(w, "  %-6s  没找到\n", d.ID)
		}
		_, _ = io.WriteString(w, msgFirstRunNoneFound)
		return errors.New("首次运行：本机没找到 claude、grok、agy、codex 中的任何一个")
	}

	fmt.Fprintf(w, "正在检查 %s 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 %v）……\n", strings.Join(ids, "、"), firstRunTimeout)
	rows := make([]firstRunRow, len(detected))
	var wg sync.WaitGroup
	for i, d := range detected {
		rows[i].d = d
		if d.Path == "" {
			continue
		}
		wg.Add(1)
		go func(i int, d config.DetectedProvider) {
			defer wg.Done()
			rows[i] = firstRunCheck(ctx, d)
		}(i, d)
	}
	wg.Wait()
	if ctx.Err() != nil {
		fmt.Fprintf(w, "识别被取消，没有生成配置文件；下次运行 council 会重新检查\n")
		return errFirstRunCanceled
	}

	var needLogin []string
	for _, row := range rows {
		switch {
		case row.d.Path == "":
			fmt.Fprintf(w, "  %-6s  没找到\n", row.d.ID)
			continue
		case row.err != nil:
			fmt.Fprintf(w, "  %-6s  %s  识别失败：%s；%s\n", row.d.ID, row.d.Path, repl.DiscoveryFailureText(row.err, firstRunTimeout), firstRunLoginText(row.login))
		default:
			fmt.Fprintf(w, "  %-6s  %s  识别到 %d 个模型；%s\n", row.d.ID, row.d.Path, row.models, firstRunLoginText(row.login))
		}
		if row.login.State != provider.LoginYes && row.login.State != provider.LoginAPIKey {
			needLogin = append(needLogin, row.d.ID)
		}
	}
	if len(needLogin) > 0 {
		fmt.Fprintf(w, "能列出模型不代表已经登录：没登录的 claude、grok、codex 也会列出一部分模型。没登录或登录状态查不出的，要登录在终端里运行：\n")
		for _, id := range needLogin {
			fmt.Fprintf(w, "  %-6s  %s\n", id, repl.LoginCommand(id))
		}
	}

	// data_dir 写配置文件旁边的 data（OB1 v1.2）：path 是默认位置 <home>\.council\config.toml，
	// Run 已经要求它是绝对路径，所以这就是 <home>\.council\data。
	text := config.FirstRunTOML(filepath.Join(filepath.Dir(path), "data"), detected)
	if err := config.CreateConfigFile(path, []byte(text)); err != nil {
		if errors.Is(err, fs.ErrExist) {
			fmt.Fprintf(w, "配置文件在检查期间已经被建好了，改用它: %s\n\n", path)
			return nil
		}
		fmt.Fprintf(w, "写不进配置文件 %s: %v\n", path, err)
		fmt.Fprintf(w, "下面是本来要写的内容，请自己存成 %s（UTF-8），再运行 council：\n\n", path)
		fmt.Fprintln(w, strings.TrimRight(text, "\n"))
		return fmt.Errorf("首次运行：写不进配置文件 %s: %w", path, err)
	}
	fmt.Fprintf(w, "已生成配置文件: %s（只写了找到的工具，还没有成员）\n", path)
	fmt.Fprintf(w, "上手三步：\n")
	fmt.Fprintf(w, "  1. /new <标题>  开一个会话\n")
	fmt.Fprintf(w, "  2. /add  看识别出的模型，再用 /add +编号 挑成员\n")
	if noAuto {
		fmt.Fprintf(w, "  3. 直接打字只记下（你用了 -no-auto）；/auto on 之后，直接打字会让会话里的成员回复\n")
	} else {
		fmt.Fprintf(w, "  3. 直接打字，会话里的成员都会回复；@<成员 id> 只问那一个\n")
	}
	fmt.Fprintln(w)
	return nil
}
