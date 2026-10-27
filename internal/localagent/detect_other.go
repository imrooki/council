//go:build !windows

package localagent

// detect_other.go 是非 Windows 平台的桩：没有进程枚举实现，返回
// ErrUnsupported，不 panic、不尝试用 ps / /proc 之类的替代实现。
//
// 为什么不做一个 Linux/macOS 版本：Council 的发布面与用户界面只覆盖
// Windows（Plan v3 §1.3 明确不用 WSL / Docker / Linux 服务作为依赖）。
// 现在写一个没人验收的 /proc 读取路径，等于多一份需要维护、却没有任何
// 真实用例覆盖的采集面。
func enumerate() ([]Process, error) {
	return nil, ErrUnsupported
}
