package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// append.go 给 /add +编号 用（00_DECISIONS.md #45）：把一段 [agents."<id>"]
// 追加到生效配置文件末尾。只追加，不重写：原有字节一个不动，注释与 Griffin
// 的单引号写法都留在原处。写之前先备份，再经同目录临时文件加改名做原子替换。
// 写完之后由调用方 LoadAndValidate；加载或校验失败时用 RestoreFile 把读到的
// 原字节写回去。docs/11d_STEP8C_CONTRACT.md:626-632 否决过运行时新增 agent
// 与写回 toml，#45 推翻了那条裁决。

// windowsDeviceNames 是 #37(h) 列的 Windows 保留设备名。判的是 id 里第一个
// 点号之前那一段，所以 nul.1、com1.x 也拦。本机 Windows 11 上带扩展名的已不算
// 保留名（#37(h) 审查员核对过），这里偏保守，代价只是这几个名字不能用。
var windowsDeviceNames = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true, "com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// NewAgentIDProblem 判断 id 能不能当一个新 agent 的 id。五道检查按这个顺序，
// 命中就返回那一句，句末都提示 --id；五道都过返回空串。
//  1. 正则：validate.go 的 idPattern / idRegexp（:25）。
//  2. 保留名：validate.go 的 reservedAgentIDs（:38-41），user 与 system。
//  3. 设备名：#37(h)，见 windowsDeviceNames。
//  4. 已存在：配置里已经有这个 id。
//  5. 折叠后撞名：#36(g)，见 foldAgentID。只比新 id 与配置里已有的 id。
func NewAgentIDProblem(c *Config, id string) string {
	const hint = "；用 --id <新 id> 换一个"
	if !idRegexp.MatchString(id) {
		return fmt.Sprintf("id %s 不合法（要匹配 %s）", id, idPattern) + hint
	}
	if reservedAgentIDs[id] {
		return fmt.Sprintf("id %s 是保留名", id) + hint
	}
	if windowsDeviceNames[strings.SplitN(id, ".", 2)[0]] {
		return fmt.Sprintf("id %s 是 Windows 保留设备名，不能当工作目录名", id) + hint
	}
	if _, ok := c.Agents[id]; ok {
		return fmt.Sprintf("配置里已经有 agent %s", id) + hint
	}
	fold := foldAgentID(id)
	for _, other := range sortedIDs(c.Agents) {
		if foldAgentID(other) == fold {
			return fmt.Sprintf("id %s 与已有的 agent %s 只差点号、下划线或连字符，claude 会把两者的工作目录落进同一个项目目录", id, other) + hint
		}
	}
	return ""
}

// foldAgentID 把 id 折成 claude 项目目录会撞上的那个名字。claude 给工作目录
// 起项目名时把字母数字之外的字符都换成 -（#36(g) 的实证是点号，本机也见过
// 下划线），所以 [a-z0-9] 之外一律换 -。大小写不在这里折：id 正则已经只放行
// 小写。
func foldAgentID(id string) string {
	b := []byte(id)
	for i, c := range b {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			b[i] = '-'
		}
	}
	return string(b)
}

// AgentBlockTOML 渲染追加到配置文件末尾的一段。表名只在 id 含点号时加引号，
// 与 Griffin 文件的写法一致：[agents."grok-4.7"]、[agents.agy-flash-high]。
// 值一律用 TOML 基本字符串，转义见 tomlBasicString。effort 为空时不写这一行，
// 与 grok / agy 成员的现有写法一致。role 照传入的值写，空串也写：#45 没定
// role，新成员的 role 留给 Griffin 自己填，填了会改指纹、要付一次入场费
// （open item (d)，待 Griffin）。comment 非空时写成一行 # 注释，其中的控制
// 字符换成空格。
func AgentBlockTOML(id string, a AgentConfig, comment string) string {
	var b strings.Builder
	if comment != "" {
		b.WriteString("# ")
		for _, c := range comment {
			if c < 0x20 || c == 0x7f {
				c = ' '
			}
			b.WriteRune(c)
		}
		b.WriteString("\n")
	}
	if strings.Contains(id, ".") {
		b.WriteString("[agents.\"" + id + "\"]\n")
	} else {
		b.WriteString("[agents." + id + "]\n")
	}
	kv := func(k, v string) {
		b.WriteString(k + " = " + tomlBasicString(v) + "\n")
	}
	kv("display_name", a.DisplayName)
	kv("provider", a.Provider)
	kv("model", a.Model)
	if a.Effort != "" {
		kv("effort", a.Effort)
	}
	kv("role", a.Role)
	kv("permissions", string(a.Permissions))
	kv("mode", string(a.Mode))
	return b.String()
}

// tomlBasicString 把 s 编成 TOML 基本字符串：反斜杠与双引号按 TOML 转义，
// 控制字符（含 DEL）写成 \uXXXX，其余原样。
func tomlBasicString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range s {
		switch {
		case c == '\\':
			b.WriteString(`\\`)
		case c == '"':
			b.WriteString(`\"`)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, c)
		default:
			b.WriteRune(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

const maxBackupAttempts = 99

// AppendBlock 把 block 追加到 path 末尾：先读原文件，再 writeBackup，再
// writeFileAtomic。分隔不检测 CRLF，原文件是 CRLF 时追加段仍是 LF，TOML
// 两种都认：原文件空则直接接；以 \n 结尾则再补一个 \n（空一行）；否则补 \n\n。
// 三个阶段的错误各自加前缀「读原文件: 」「备份: 」「写入: 」。写入失败时备份
// 已经在了，返回它的路径；原文件没有被替换，不需要恢复。orig 给调用方在写后校验
// 失败时 RestoreFile 用。
func AppendBlock(path, block string) (backupPath string, orig []byte, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", nil, fmt.Errorf("读原文件: %w", err)
	}
	orig, err = os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("读原文件: %w", err)
	}
	backupPath, err = writeBackup(path, orig, time.Now())
	if err != nil {
		return "", nil, fmt.Errorf("备份: %w", err)
	}
	sep := ""
	if len(orig) > 0 {
		if orig[len(orig)-1] == '\n' {
			sep = "\n"
		} else {
			sep = "\n\n"
		}
	}
	content := make([]byte, 0, len(orig)+len(sep)+len(block))
	content = append(content, orig...)
	content = append(content, sep...)
	content = append(content, block...)
	if err := writeFileAtomic(path, content, info.Mode().Perm()); err != nil {
		return backupPath, orig, fmt.Errorf("写入: %w", err)
	}
	return backupPath, orig, nil
}

// RestoreFile 把 path 原子地写回 orig。path 还在就沿用它的权限位，否则用 0644。
func RestoreFile(path string, orig []byte) error {
	perm := fs.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	return writeFileAtomic(path, orig, perm)
}

// writeBackup 把 orig 写成 path 旁边的一份新备份，名字是
// <配置文件>.bak-add-<本地时间 YYYYMMDD-HHMMSS>。撞名依次试 -2 到 -99，
// 用 O_EXCL 建，绝不覆盖已有备份：同一秒里连加两个时，第二个备份的是第一次
// 追加之后的文件，第一个备份才是原件。这个名字与 Griffin 手工备份
// config.toml.bak-20260923 不相交。备份在追加成功之后也保留。99 个名字全被
// 占时，错误文本同时写出不带后缀的名字与 -2..99。某个名字已经建好、Write 或
// Close 写到一半失败时，那个残缺的文件不删，返回空串（open item (p)，待主代理）。
func writeBackup(path string, orig []byte, now time.Time) (string, error) {
	base := path + ".bak-add-" + now.Format("20060102-150405")
	for i := 1; i <= maxBackupAttempts; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.Write(orig); err != nil {
			f.Close()
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		return name, nil
	}
	return "", fmt.Errorf("backup names %s and %s-2..%d all taken", base, base, maxBackupAttempts)
}

// writeFileAtomic 在 path 的同一目录建 <名字>.tmp-add-*，写完、Sync、关闭、
// 按 perm Chmod，再 Rename 覆盖 path。Windows 上 os.Rename 会替换已存在的目标。
// 任何一步失败都删掉临时文件；成功之后临时文件已经改名为 path，不再删。
func writeFileAtomic(path string, data []byte, perm fs.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-add-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
