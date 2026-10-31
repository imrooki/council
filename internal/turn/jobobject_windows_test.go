//go:build windows

package turn

// jobobject_windows_test.go 钉住 S3 的 Windows 兜底：Job Object 的最后一个
// 句柄关掉之后，属于它的进程必须被内核杀掉，不需要父进程执行任何清理代码。

import (
	"os/exec"
	"testing"
	"time"
)

func TestAttachKillOnCloseJobKillsProcess(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "ping -n 30 127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动测试子进程失败: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	job, err := attachKillOnCloseJob(cmd.Process.Pid)
	if err != nil {
		// 受限容器（父进程已在禁止嵌套的 job 里）可能拒绝挂载；
		// 这不是产品路径能控制的，跳过而不是误报。
		t.Skipf("本机不允许挂 Job Object，跳过: %v", err)
	}
	if job == nil {
		t.Fatal("attachKillOnCloseJob 返回 nil job 且没有错误")
	}
	job.Close()

	done := make(chan error, 1)
	go func() { _, err := cmd.Process.Wait(); done <- err }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("关掉 Job Object 之后 10 秒子进程还活着")
	}
}
