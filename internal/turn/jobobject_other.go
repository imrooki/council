//go:build !windows

package turn

// attachKillOnCloseJob 在非 Windows 上没有 Job Object；进程树的收尾由
// killProcessTree 的进程组负责（S3 的 Windows 兜底不适用）。
func attachKillOnCloseJob(pid int) (processJobCloser, error) {
	return nil, nil
}
