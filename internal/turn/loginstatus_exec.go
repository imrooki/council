package turn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"council/internal/provider"
)

// RunLoginStatus 是 provider.LoginStatusExec 的生产实现（OB1 v1.1，今天只给 codex login status 用）。
// 起法与 RunDiscovery 相同：LookPath 解析、工作目录 os.TempDir()、Env 原样继承（要的正是
// 用户真实的登录态）、prepareCmd。stdin 只给 c.Stdin（为空时就是空 stdin，读到即 EOF）；
// CloseStdinAfter 不用。stdout 与 stderr 分开收，各自封顶 maxDiscoveryStderrBytes：
// codex 把状态打在 stderr（实测），RunDiscovery 退出码为 0 时不交回 stderr，所以另写这一个。
//
// 进程自己以非 0 退出不算错误：退出码照实交回，由 provider.ParseLoginStatus 判。
// 起不来、或 ctx 结束（先 killProcessTree 再 Kill，最多等 discoveryKillGrace）时返回错误，
// 这时退出码与输出都不可信，调用方按查不出处理。
func RunLoginStatus(ctx context.Context, c provider.DiscoveryCommand) (int, []byte, []byte, error) {
	if c.Path == "" {
		return 0, nil, nil, errors.New("login status command path is empty")
	}
	path := c.Path
	if resolved, lookErr := exec.LookPath(path); lookErr == nil {
		path = resolved
	}
	cmd := exec.Command(path, c.Args...)
	cmd.Dir = os.TempDir()
	prepareCmd(cmd)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdin = bytes.NewReader(c.Stdin)
	cmd.Stdout = &capWriter{buf: &outBuf, cap: maxDiscoveryStderrBytes}
	cmd.Stderr = &capWriter{buf: &errBuf, cap: maxDiscoveryStderrBytes}
	if err := cmd.Start(); err != nil {
		return 0, nil, nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case waitErr := <-done:
		if waitErr != nil {
			var ee *exec.ExitError
			if errors.As(waitErr, &ee) {
				return ee.ExitCode(), outBuf.Bytes(), errBuf.Bytes(), nil
			}
			return 0, nil, nil, waitErr
		}
		return 0, outBuf.Bytes(), errBuf.Bytes(), nil
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = killProcessTree(cmd.Process.Pid)
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(discoveryKillGrace):
		}
		return 0, nil, nil, fmt.Errorf("login status killed: %w", ctx.Err())
	}
}
