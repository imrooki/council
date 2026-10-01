package turn

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"council/internal/provider"
)

// oby_loginstatus_exec_test.go 是 OB2（契约 §2.3）补 OB1 审查余项 R0、R4 在 turn 包的验收：
// RunLoginStatus 被 ctx 杀掉时及时返回（杀进程那两行起作用），子进程的工作目录是 os.TempDir()。
// 假 CLI 复用 obx_loginstatus_exec_test.go 的 obxStatusFake、obxLoopLines（只读复用）。PATH 一律不改；L5b 换掉包级接缝
// taskkillPath（CL1 起 killProcessTree 只起系统目录里的 taskkill.exe，不再按 PATH 找），模拟找不到 taskkill。

// TestObyRunLoginStatusKilledPromptly 是 L5（R0）：ctx 300 ms 到期，RunLoginStatus 必须在
// discoveryKillGrace 的一半之内返回。杀进程那两行都删掉时，它要等满 discoveryKillGrace 才返回。
// 【判别力】N30。只删 killProcessTree 那一行（N31）是等价变异：Kill 照样杀得掉没有孙进程的 cmd 循环；
// 只删 Kill 那一行（N32）在这里看不出来（系统目录里的 taskkill 照样杀树），由 L5b 杀。
func TestObyRunLoginStatusKilledPromptly(t *testing.T) {
	if discoveryKillGrace/2 <= 300*time.Millisecond {
		t.Fatalf("前提不成立：discoveryKillGrace/2 = %v 不大于 ctx 的 300 ms，断言分不开杀没杀", discoveryKillGrace/2)
	}
	dir := t.TempDir()
	on := filepath.Join(dir, "loop.on")
	if err := os.WriteFile(on, []byte("x"), 0o644); err != nil {
		t.Fatalf("写 loop.on: %v", err)
	}
	p := obxStatusFake(t, obxLoopLines(`exist "`+on+`"`)...)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, _, err := RunLoginStatus(ctx, provider.DiscoveryCommand{Path: p, Args: []string{"login", "status"}})
	el := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("被 ctx 杀掉时 err 应当满足 errors.Is(err, context.DeadlineExceeded)，实际 %v", err)
	}
	if el >= discoveryKillGrace/2 {
		t.Errorf("RunLoginStatus 用了 %v 才返回，应当小于 %v（杀进程之后不该等满宽限期）", el, discoveryKillGrace/2)
	}
}

// TestObyRunLoginStatusKilledWithoutTaskkill 是 L5b（R0）：同 L5，但 taskkillPath 换成返回错误的函数，
// killProcessTree 取不到 taskkill、什么都不起（CL1 改法，原来是把 PATH 设成空目录），只剩 cmd.Process.Kill 那一行在杀。
// 前提：换掉之后 taskkillPath() 必须返回错误。本包的测试没有 t.Parallel，换包级变量不会与别的用例并发。
// 【判别力】N30、N32（只删 Kill 那一行：L5 里 taskkill 还在，看不出来）。
func TestObyRunLoginStatusKilledWithoutTaskkill(t *testing.T) {
	old := taskkillPath
	t.Cleanup(func() { taskkillPath = old })
	taskkillPath = func() (string, error) { return "", errors.New("oby: no taskkill") }
	if _, err := taskkillPath(); err == nil {
		t.Fatalf("前提不成立：换掉接缝之后 taskkillPath() 仍然没有返回错误")
	}
	dir := t.TempDir()
	on := filepath.Join(dir, "loop.on")
	if err := os.WriteFile(on, []byte("x"), 0o644); err != nil {
		t.Fatalf("写 loop.on: %v", err)
	}
	p := obxStatusFake(t, obxLoopLines(`exist "`+on+`"`)...)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, _, err := RunLoginStatus(ctx, provider.DiscoveryCommand{Path: p, Args: []string{"login", "status"}})
	el := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("被 ctx 杀掉时 err 应当满足 errors.Is(err, context.DeadlineExceeded)，实际 %v", err)
	}
	if el >= discoveryKillGrace/2 {
		t.Errorf("RunLoginStatus 用了 %v 才返回，应当小于 %v（找不到 taskkill 时也要靠 Kill 杀掉）", el, discoveryKillGrace/2)
	}
}

// TestObyRunLoginStatusDir 是 L6（R4）：假 CLI 打出当前目录，应当是 os.TempDir()。
// 前提：测试进程自己的当前目录不是 os.TempDir()，否则删掉 cmd.Dir 那一行也看不出来。
// 【判别力】N33。
func TestObyRunLoginStatusDir(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("前提不成立：取不到当前目录：%v", err)
	}
	tmp := filepath.Clean(os.TempDir())
	if strings.EqualFold(filepath.Clean(wd), tmp) {
		t.Fatalf("前提不成立：测试进程的当前目录 %s 就是 os.TempDir()，看不出 cmd.Dir 设没设", wd)
	}
	p := obxStatusFake(t, "@echo %CD%", "@exit /b 0")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code, so, se, err := RunLoginStatus(ctx, provider.DiscoveryCommand{Path: p, Args: []string{"login", "status"}})
	if err != nil {
		t.Fatalf("err 应当是 nil，实际 %v", err)
	}
	if code != 0 || len(se) != 0 {
		t.Errorf("退出码应当是 0、stderr 为空，实际 %d、%q", code, se)
	}
	got := strings.TrimSuffix(string(so), "\r\n")
	if !strings.EqualFold(filepath.Clean(got), tmp) {
		t.Errorf("子进程的当前目录应当是 os.TempDir() %q，实际 stdout %q", tmp, so)
	}
}
