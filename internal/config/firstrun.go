package config

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// firstrun.go 给首次运行自动配置用（00_DECISIONS.md #48，OB1）：找出本机装了
// 哪几家工具、渲染只含 data_dir 与 [providers.*] 的配置文本、不覆盖地建文件。/add 的
// 「本机装了、配置里还没有」提示也用这里的检测与渲染。本文件只看 PATH 与几处
// 已知安装位置，不起任何进程。

// DetectedProvider 是一家工具的检测结果。Path 为空表示没找到；非空时是绝对路径。
type DetectedProvider struct {
	ID   string
	Path string
}

// detectOrder 是检测与屏幕显示的固定顺序。
var detectOrder = []string{"claude", "grok", "agy", "codex"}

// DetectProviders 按 claude、grok、agy、codex 的固定顺序检测，每家恰好一项，
// 没找到的那一项 Path 为空。每次调用都重新检测，不缓存。
func DetectProviders() []DetectedProvider {
	out := make([]DetectedProvider, 0, len(detectOrder))
	for _, id := range detectOrder {
		out = append(out, DetectedProvider{ID: id, Path: detectProvider(id)})
	}
	return out
}

// detectProvider 先 exec.LookPath(id)：没有错误、而且是绝对路径才算找到（LookPath
// 在 Windows 上按 PATHEXT 补扩展名，本机 claude 与 codex 解析成 npm 目录下的 .cmd）。
// 找不到再看 knownInstallPath 给出的那一处：是绝对路径、存在、而且是普通文件才算。
func detectProvider(id string) string {
	if p, err := exec.LookPath(id); err == nil && filepath.IsAbs(p) {
		return p
	}
	p := knownInstallPath(id)
	if p == "" || !filepath.IsAbs(p) {
		return ""
	}
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return p
}

// knownInstallPath 是 PATH 上找不到时再看的那一处：claude 与 codex 是
// %APPDATA%\npm\<id>.cmd（npm 全局安装），grok 是 %USERPROFILE%\.grok\bin\grok.exe，
// agy 是 %LOCALAPPDATA%\agy\bin\agy.exe。对应的环境变量为空时返回空串，不拼相对路径。
func knownInstallPath(id string) string {
	var base string
	var rel []string
	switch id {
	case "claude", "codex":
		base, rel = os.Getenv("APPDATA"), []string{"npm", id + ".cmd"}
	case "grok":
		base, rel = os.Getenv("USERPROFILE"), []string{".grok", "bin", "grok.exe"}
	case "agy":
		base, rel = os.Getenv("LOCALAPPDATA"), []string{"agy", "bin", "agy.exe"}
	default:
		return ""
	}
	if base == "" {
		return ""
	}
	return filepath.Join(append([]string{base}, rel...)...)
}

// ProviderBlockTOML 渲染一段 [providers.<id>]：两行，以换行结尾。command 写成
// TOML 字面量字符串（单引号，反斜杠原样，与 维护者 手写的配置同一种写法）；
// 路径里有单引号或控制字符时字面量字符串装不下，退回 tomlBasicString。
func ProviderBlockTOML(id, command string) string {
	return "[providers." + id + "]\ncommand = " + tomlCommandString(command) + "\n"
}

// tomlCommandString 见 ProviderBlockTOML。FirstRunTOML 写 data_dir 也用它。
func tomlCommandString(s string) string {
	for _, c := range s {
		if c == '\'' || c < 0x20 || c == 0x7f {
			return tomlBasicString(s)
		}
	}
	return "'" + s + "'"
}

// firstRunDataDirComment 是生成的配置里 data_dir 那一行之前的注释。
const firstRunDataDirComment = "# 数据目录（本地库与成员的工作目录），council 第一次运行时定在本文件旁边。可以改成别的绝对路径，重启 council 生效（已有的库不会跟着搬）；写了这一行，环境变量 COUNCIL_DATA_DIR 就不再起作用。\n"

// firstRunProvidersComment 是生成的配置里 data_dir 之后、各段 [providers.*] 之前的那一行注释。
const firstRunProvidersComment = "# 下面是 council 第一次运行时在本机找到的工具。\n"

// FirstRunTOML 返回首次运行要写的配置全文：DefaultTOML()，空一行，
// firstRunDataDirComment 与一行 data_dir = <dataDir>（写法同 ProviderBlockTOML 的 command），
// 空一行，firstRunProvidersComment，再按 found 的顺序每家一段 ProviderBlockTOML，段与段之间
// 空一行。Path 为空的项跳过。不写任何成员，不写 [summary]。
//
// 写 data_dir（OB1 v1.2）：内置默认 DefaultDataDir 是一台机器专用的，不写的话，没有那个盘的
// 机器上首次运行生成了配置，接着打开本地库就失败。data_dir 是顶层键，必须在第一个表之前。
func FirstRunTOML(dataDir string, found []DetectedProvider) string {
	var b strings.Builder
	b.WriteString(DefaultTOML())
	b.WriteString("\n")
	b.WriteString(firstRunDataDirComment)
	b.WriteString("data_dir = " + tomlCommandString(dataDir) + "\n")
	b.WriteString("\n")
	b.WriteString(firstRunProvidersComment)
	n := 0
	for _, d := range found {
		if d.Path == "" {
			continue
		}
		if n > 0 {
			b.WriteString("\n")
		}
		b.WriteString(ProviderBlockTOML(d.ID, d.Path))
		n++
	}
	return b.String()
}

// linkFile 是 CreateConfigFile 用的 os.Link。只为测试留的接缝：NTFS 上造不出
// Link 因 ErrExist 以外的原因失败（例如卷不支持硬链接），退回 createExclusive 的那一支
// 只能换掉它来测。生产代码不改它。
var linkFile = os.Link

// CreateConfigFile 先建 path 所在的目录（MkdirAll，0o755），在同一目录里建一个临时文件
// （os.CreateTemp，名字是 <文件名>.tmp-<随机数字>）写入 data、关掉，再 linkFile(临时文件, path)，
// 最后删掉临时文件的名字（成功与失败都删）。硬链接在目标已存在时失败，所以绝不覆盖已有
// 文件：这时返回的错误满足 errors.Is(err, fs.ErrExist)；而 path 一出现就是完整内容，
// 同时启动的另一个 council 撞车改用它时读不到空文件或半个文件。
//
// linkFile 因 ErrExist 以外的原因失败时退回 createExclusive（O_EXCL 直接建 path）：
// 仍然绝不覆盖，代价是文件建出来到写完之间有一个窗口，那时读到的是空文件或半个文件。
func CreateConfigFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	err = linkFile(tmpPath, path)
	if err == nil || errors.Is(err, fs.ErrExist) {
		return err
	}
	return createExclusive(path, data)
}

// createExclusive 是 CreateConfigFile 的退路：用 O_WRONLY|O_CREATE|O_EXCL 以 0o644 建 path、
// 写入 data，绝不覆盖（目标已存在时返回的错误满足 errors.Is(err, fs.ErrExist)）。Write 或
// Close 失败时删掉这个刚由本函数建出来的文件再返回错误：留下半个文件，下次启动既不会再走
// 首次运行，又加载不了。
func createExclusive(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(path)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}
