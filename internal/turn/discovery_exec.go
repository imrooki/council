package turn

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"council/internal/provider"
)

// maxDiscoveryStderrBytes 是 stderr 单独收的封顶。stderr 不并进 stdout：
// agy 成功时也往 stderr 打 Fetching available models...。
// 这些字节只在退出码非 0 时取最后一个非空行拼进错误。
const maxDiscoveryStderrBytes = 64 << 10

// discoveryKillGrace 是 ctx 结束、杀掉进程之后，等读 goroutine 收尾的上限。
const discoveryKillGrace = 5 * time.Second

// RunDiscovery 是 provider.DiscoveryExec 的生产实现。
// 由 repl.New 装上。不复用 spawnProcess：它的 stdin 只能一次给定
// （给定字节或 null device），进程结束才返回，给不了「收到回应再关 stdin」。
func RunDiscovery(ctx context.Context, c provider.DiscoveryCommand) ([]byte, error) {
	if c.Path == "" {
		return nil, errors.New("discovery command path is empty")
	}
	path := c.Path
	if resolved, lookErr := exec.LookPath(path); lookErr == nil {
		path = resolved
	}

	cmd := exec.Command(path, c.Args...)
	// 工作目录是 os.TempDir()，不继承编排器的启动目录。理由与 spawnProcess
	// 拦空 Dir（spawn.go:98-104）相同：cwd 下的项目级指令文件会被读进去。
	// 识别用不着 Council 的 workdir，也不该为它建目录。
	// Env 不设（零值，原样继承）：识别要用户真实的登录态，改 HOME 或
	// CLAUDE_CONFIG_DIR 会把登录态一起带走（与 spawn.go:152 同一个理由）。
	cmd.Dir = os.TempDir()
	prepareCmd(cmd)
	var stderrBuf bytes.Buffer
	cmd.Stderr = &capWriter{buf: &stderrBuf, cap: maxDiscoveryStderrBytes}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}

	var closeOnce sync.Once
	closeStdin := func() { closeOnce.Do(func() { _ = stdin.Close() }) }

	// CloseStdinAfter 为 nil：写完 Stdin 立刻关 stdin。非 nil：逐行读 stdout，
	// 第一次有一行让它返回 true 才关。成功路径在 Wait 返回后再关一次，
	// 超时路径在杀进程时关一次；sync.Once，已关过就是空操作。
	if len(c.Stdin) > 0 {
		_, _ = stdin.Write(c.Stdin)
	}
	if c.CloseStdinAfter == nil {
		closeStdin()
	}

	var outBuf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		// stdout 逐行读，封顶复用 maxSpawnOutputBytes。行宽没有上界：
		// claude 的 system 帧带着整份 commands 清单，随安装的命令增长，
		// 实测已有 12 KB 与 17 KB 的行。bufio.Scanner 超过单行上限会整体报错，
		// 所以用 ReadBytes，再按 maxSpawnOutputBytes 封顶。封顶只截写入
		// outBuf 的字节，CloseStdinAfter 仍看这一行读到的全部。
		br := bufio.NewReader(stdout)
		for {
			line, readErr := br.ReadBytes('\n')
			if len(line) > 0 {
				if remain := maxSpawnOutputBytes - outBuf.Len(); remain > 0 {
					if len(line) > remain {
						outBuf.Write(line[:remain])
					} else {
						outBuf.Write(line)
					}
				}
				if c.CloseStdinAfter != nil && c.CloseStdinAfter(line) {
					closeStdin()
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					_, _ = io.Copy(io.Discard, stdout)
				}
				break
			}
		}
		done <- cmd.Wait()
	}()

	select {
	case waitErr := <-done:
		closeStdin()
		if waitErr != nil {
			// 退出码非 0 返回 exit code N；stderr 有最后一个非空行时再接「: 该行」。
			// stdout 照样返回，调用方不用。
			var ee *exec.ExitError
			if errors.As(waitErr, &ee) {
				if line := lastNonEmptyLine(stderrBuf.String()); line != "" {
					return outBuf.Bytes(), fmt.Errorf("exit code %d: %s", ee.ExitCode(), line)
				}
				return outBuf.Bytes(), fmt.Errorf("exit code %d", ee.ExitCode())
			}
			return outBuf.Bytes(), waitErr
		}
		return outBuf.Bytes(), nil
	case <-ctx.Done():
		// 超时是主路径：ctx 结束的那一刻先 killProcessTree 再 Kill
		// （Windows 上 claude.cmd 垫片之下才是干活的 node），再最多等
		// discoveryKillGrace 让读 goroutine 收尾。错误用 %w 包住 ctx.Err()，
		// 上层据此判超时。
		if pid > 0 {
			_ = killProcessTree(pid)
		}
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		closeStdin()
		select {
		case <-done:
		case <-time.After(discoveryKillGrace):
		}
		return nil, fmt.Errorf("model discovery killed: %w", ctx.Err())
	}
}

// lastNonEmptyLine 取 stderr 最后一个非空行，供退出码非 0 的错误文本使用。
func lastNonEmptyLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return ""
}
