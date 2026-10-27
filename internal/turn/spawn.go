package turn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"council/internal/provider"
)

// maxSpawnOutputBytes 是 stdout / stderr 各自的封顶。
// 探针 JSON 是几 KB 到几十 KB；封顶是防失控进程把编排器撑爆，不是截断正常回答。
// 超了就丢弃后半段并继续等进程结束 —— 不能让管道堵住把子进程卡死。
const maxSpawnOutputBytes = 8 << 20

// ErrCommandLineTooLong 表示组出来的整条命令行超过 OS 上限。
var ErrCommandLineTooLong = errors.New("assembled command line exceeds the OS limit")

// maxCommandLineBytes 是 Windows CreateProcess 的命令行上限。
//
// 它与 provider.MaxPromptBytes 不是一回事：那个说「这家的正文通路有上限」，
// 这个说「这一条具体的命令行太长了」。以前那道闸只量正文、不量整条命令行
// （未决 #15），于是正文刚好卡在限下、加上参数段仍可能越界，失败来自
// 操作系统、报错形状不受控制，而且**长度相关**（同一个 agent 对短需求
// 成功、对长需求失败）。规则文件让这个漏风到期了：规则正文在 grok
// （--system-prompt-override）与 agy（首轮正文前缀）两家上直接落进 argv。
//
// 本项目只在 Windows 上跑过。别的平台上这个值偏严（Linux 的 ARG_MAX
// 大得多），真要跨平台时按 GOOS 分档，别在这里编一个折中值。
const maxCommandLineBytes = 32767

// CommandLineBytes 量一条真实命令行的长度：可执行文件路径，
// 加上每个参数前面一个分隔空格。
//
// 这个数**偏小，是有意的、也是已知的**：Go 的 os/exec 在 Windows 上会按
// 自己的规则给含空格的参数加引号，加引号之后比这个数长。我们量的是
// 加引号前的长度，所以这道闸能挡住「规则文件太大」这类整数量级的越界，
// 挡不住刚好卡在边上的那几十字节。
// 不要为了「更准」去编一个余量常数 —— 那会把「未实测」伪装成「已标定」。
func CommandLineBytes(path string, args []string) int {
	n := len(path)
	for _, arg := range args {
		n += 1 + len(arg)
	}
	return n
}

// longestArgLabel 说出「最长的那个参数是谁」，给越界时的报错用。
//
// 它存在的全部理由是把一个操作系统的数字翻译成一句可执行的话：
// 用户该看到的是「--system-prompt-override 占了 28 KB，砍规则文件」，
// 而不是「CreateProcess 失败」。
func longestArgLabel(args []string) (label string, size int) {
	idx := 0
	for i, arg := range args {
		if len(arg) > size {
			size = len(arg)
			idx = i
		}
	}
	if len(args) == 0 {
		return "(无参数)", 0
	}
	// 判定单横杠也算 flag，不只是双横杠。grok 与 agy 的**正文**都绑在 `-p` 上
	// （`-p <prompt>`），而正文正是最容易撑爆命令行的那一项；只认 `--` 的话
	// 这两家最需要点名的时候恰好报成 args[1]，等于什么都没说。
	//
	// 这是**启发式，不是解析**：它会把一个以减号开头的**值**误当成 flag 名。
	// 本项目三个适配器发出的参数里不存在这样的值（逐个看过），
	// 但这条判断本身没有任何语法依据 —— 将来若有适配器要传负数或
	// 以减号开头的自由文本，这里会报错一个不存在的 flag。
	// 那也只是错一句提示，不影响这道闸拦不拦得住，所以接受。
	if idx > 0 && strings.HasPrefix(args[idx-1], "-") {
		return args[idx-1] + " 的值", size
	}
	return fmt.Sprintf("args[%d]", idx), size
}

// spawnProcess 是 Spawner 的生产实现。
//
// 三条硬约束（09a Spawner 注释）：
//  1. 不合并 stderr：合并会让 JSON 解析吃到诊断文本。
//  2. 子进程的标准输入接 null device，绝不接编排器自己的输入句柄：
//     repl.go 的 readLines goroutine 正阻塞在那同一个句柄上，抢起来胜负随时序漂移。
//  3. 超时后杀进程树：Windows 上杀 .cmd 垫片往往杀不掉它拉起的 node.exe。
//     本函数在 ctx 结束的那一刻就走 killProcessTree，而不是等 Wait 返回再补刀 ——
//     等 Wait 的话垫片已经死了，/T 找不到孩子，真正干活的进程会把这一轮写完。
//
// 硬超时是主路径，不是保险。本机实测：grok 拿到一个它本地不认识的会话 id 时
// 不报错，去远端恢复，拉起交互式 OAuth 设备码流程，打印一个 URL 然后
// Waiting for authorization... 无限等下去（实测 4 分钟只吃 0.6 秒 CPU）。
func spawnProcess(ctx context.Context, c provider.Command) (stdout, stderr []byte, exitCode int, killed bool, err error) {
	if c.Dir == "" {
		// 继承编排器的启动目录会让三家 CLI 读到 cwd 下的项目级指令文件，
		// 入场费与行为随启动目录漂移，拿到写盘权限的成员还会把文件写进那个仓库。
		// 适配器本应在 BuildTurn 就因空 WorkDir 返回 ErrNoWorkDir；这里再拦一次，
		// 是因为 Spawner 会被测试换成假实现，生产路径不能把空 Dir 放过去。
		return nil, nil, 0, false, fmt.Errorf("command dir is empty: %w", provider.ErrNoWorkDir)
	}
	if c.Path == "" {
		return nil, nil, 0, false, fmt.Errorf("command path is empty")
	}

	path := c.Path
	if resolved, lookErr := exec.LookPath(path); lookErr == nil {
		path = resolved
	}

	// c.Path 可能是没有扩展名的名字（配置里写 "claude"），先 LookPath 再判。
	// .cmd / .bat 垫片的命令行由 cmd.exe 再解析一遍，argv 元素里的换行是命令
	// 分隔符，整条命令行在第一个换行处截断。截断后若仍然合法，就会静默跑出
	// 一个身份被截短的轮次：退出码 0、JSON 合法、游标照推，没有任何一层会报错。
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".cmd" || ext == ".bat" {
		for i, arg := range c.Args {
			if strings.ContainsAny(arg, "\n\r") {
				return nil, nil, 0, false, fmt.Errorf("executable %s (a %s shim) args[%d] contains a newline: cmd.exe treats newlines as command separators and would truncate the command line; a still-legal remainder would silently run a turn with a truncated identity: %w", path, ext, i, ErrShimNewlineArg)
			}
		}
	}

	// 整条命令行的总长闸。放在换行闸之后、落 Files 盘之前：起进程之前失败，
	// 而且失败在写任何东西之前 —— 越界的那一轮本来就不该留下半个身份文件。
	if n := CommandLineBytes(path, c.Args); n > maxCommandLineBytes {
		label, size := longestArgLabel(c.Args)
		return nil, nil, 0, false, fmt.Errorf(
			"命令行 %d 字节，上限 %d；最长的参数是 %s（%d 字节）: %w",
			n, maxCommandLineBytes, label, size, ErrCommandLineTooLong)
	}

	// Files 由 BuildTurn 填好路径和内容，这里才落盘。写失败直接返回，
	// 不起进程：少了身份块的那一轮跑出来是错的，而且错得看不出来。
	// 每轮覆盖同一个路径，不做清理：路径是确定的，内容逐字节相同
	// （身份块不随轮次变），留着还便于事后核对到底送了什么。
	for name, body := range c.Files {
		dir := filepath.Dir(name)
		if mkdirErr := os.MkdirAll(dir, 0o755); mkdirErr != nil {
			return nil, nil, 0, false, fmt.Errorf("mkdir %s: %w", dir, mkdirErr)
		}
		if writeErr := os.WriteFile(name, body, 0o600); writeErr != nil {
			return nil, nil, 0, false, fmt.Errorf("write %s: %w", name, writeErr)
		}
	}

	cmd := exec.Command(path, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = c.Env // nil = 原样继承；Step 6 一律 nil，不改 HOME（改了会把登录态一起隔离掉）
	prepareCmd(cmd)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &capWriter{buf: &stdoutBuf, cap: maxSpawnOutputBytes}
	cmd.Stderr = &capWriter{buf: &stderrBuf, cap: maxSpawnOutputBytes}

	if len(c.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(c.Stdin)
	} else {
		nullFile, openErr := os.Open(os.DevNull)
		if openErr != nil {
			return nil, nil, 0, false, fmt.Errorf("open null device: %w", openErr)
		}
		defer nullFile.Close()
		cmd.Stdin = nullFile
	}

	if startErr := cmd.Start(); startErr != nil {
		return nil, nil, 0, false, startErr
	}
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	// 「子进程真的起来了」的通知（Plan §4.3 ⑨）。只在这一条生产路径上有；
	// 回调挂在 ctx 上，测试替身走的不是这个函数，所以不会误触发。
	// 顺序在 Start 成功之后、Wait 之前：这一轮还没跑完，PID 就已经有意义了。
	if pid > 0 {
		if notify := processIDObserverFrom(ctx); notify != nil {
			notify(pid)
		}
	}

	waitErr := make(chan error, 1)
	go func() {
		waitErr <- cmd.Wait()
	}()

	select {
	case err = <-waitErr:
		exitCode = exitCodeOf(cmd, err)
		if isExitError(err) {
			err = nil
		}
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), exitCode, false, err
	case <-ctx.Done():
		if pid > 0 {
			_ = killProcessTree(pid)
		}
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		err = <-waitErr
		exitCode = exitCodeOf(cmd, err)
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), exitCode, true, ctx.Err()
	}
}

func exitCodeOf(cmd *exec.Cmd, waitErr error) int {
	if cmd != nil && cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode()
	}
	if ee, ok := waitErr.(*exec.ExitError); ok && ee.ProcessState != nil {
		return ee.ProcessState.ExitCode()
	}
	return 0
}

func isExitError(err error) bool {
	_, ok := err.(*exec.ExitError)
	return ok
}

// capWriter 把超过 cap 的字节丢弃，但回报已经全部吃下。
// 子进程写满管道会被操作系统卡住；丢弃后半段比把 CLI 挂在那一轮里更干净。
type capWriter struct {
	buf *bytes.Buffer
	cap int
}

func (w *capWriter) Write(p []byte) (int, error) {
	if w.buf == nil {
		return len(p), nil
	}
	remain := w.cap - w.buf.Len()
	if remain <= 0 {
		return len(p), nil
	}
	if len(p) > remain {
		_, _ = w.buf.Write(p[:remain])
		return len(p), nil
	}
	_, err := w.buf.Write(p)
	return len(p), err
}
