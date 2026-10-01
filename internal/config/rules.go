package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	// EnvGlobalRules 覆盖全局规则文件的位置。
	EnvGlobalRules = "COUNCIL_GLOBAL_RULES"
	// DefaultGlobalRulesName 是全局规则文件在 ~/.council/ 下的文件名。
	DefaultGlobalRulesName = "COUNCIL.md"
)

// utf8BOM 是 UTF-8 字节序标记的三个字节 EF BB BF。
// 必须按字节剥，不能按「第一个非空白 rune」剥：BOM 不是空白，TrimSpace 去不掉。
const utf8BOM = "\xEF\xBB\xBF"

// Rules 是两级规则文件的内容与出处。Text 字段已归一化。
type Rules struct {
	GlobalPath  string // 解析后的路径，恒非空（文件不存在时也有值，给 /rules 显示）
	GlobalText  string // 归一化后的正文；文件不存在时是空串
	GlobalFound bool
	LocalPath   string // 配置里没写 rules_file 时是空串
	LocalText   string
	LocalFound  bool
}

// ResolveGlobalRulesPath 按环境变量 COUNCIL_GLOBAL_RULES -> ~/.council/COUNCIL.md 取值。
// 空字符串视为未设置，继续向下回落。返回的路径不保证文件存在。
// 取用户主目录失败时返回带上下文的错误。
// 与 ResolveConfigPath 同构，但没有 flag 一级：全局规则没有命令行开关。
//
// 全局那份不允许由配置文件指定，这是有意的：一份从别处拿来的项目配置
// 不该能重新定义「什么算全局」，那正是「全局」这个词的全部含义。
func ResolveGlobalRulesPath() (string, error) {
	if env := os.Getenv(EnvGlobalRules); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve default global rules path: user home dir: %w", err)
	}
	return filepath.Join(home, ".council", DefaultGlobalRulesName), nil
}

// LoadRules 读两份规则文件。
//
// configPath 是生效的配置文件路径，只用来解析 rulesFile 的相对路径
// （取它的 filepath.Dir），不读它本身。
//
// rulesFile 空串表示没有局部规则：LocalPath 留空串、LocalText 空串、
// LocalFound 为 false。这是正常情况，不是错误。
//
// rulesFile 非空且是相对路径时，相对配置文件所在目录解析，不是相对
// 当前工作目录 —— 按 cwd 解析会让入场费与行为随启动目录漂移、不可复现。
// 绝对路径原样用。解析后的结果放进 LocalPath。
//
// 全局那份的路径由 ResolveGlobalRulesPath 给出，恒填进 GlobalPath，
// 哪怕文件不存在：/rules 需要把「去哪找」显示出来，不能因为缺文件就把路径抹掉。
//
// 文件不存在不是错误：Found 为 false、Text 为空串、路径照填。
// 用 errors.Is(err, fs.ErrNotExist) 判定，不靠字符串匹配。
//
// 文件存在但读不出来是错误（权限、是目录、I/O 错）。把读失败当成空规则，
// 会让一次 I/O 故障静默地把成员的指令环境掏空，而下一轮照样花钱跑完、
// 退出码 0。
//
// Text 按 normalizeRulesText 的固定三步归一化后再写入。
func LoadRules(configPath string, rulesFile string) (Rules, error) {
	globalPath, err := ResolveGlobalRulesPath()
	if err != nil {
		return Rules{}, err
	}

	globalText, globalFound, err := readRulesFile(globalPath)
	if err != nil {
		return Rules{}, err
	}

	r := Rules{
		GlobalPath:  globalPath,
		GlobalText:  globalText,
		GlobalFound: globalFound,
	}

	if rulesFile == "" {
		return r, nil
	}

	localPath := rulesFile
	if !filepath.IsAbs(rulesFile) {
		localPath = filepath.Join(filepath.Dir(configPath), rulesFile)
	}
	r.LocalPath = localPath

	localText, localFound, err := readRulesFile(localPath)
	if err != nil {
		return Rules{}, err
	}
	r.LocalText = localText
	r.LocalFound = localFound
	return r, nil
}

// readRulesFile 读一份规则文件并做归一化。
//
// 用 os.ReadFile 的错误做 errors.Is(..., fs.ErrNotExist)，不用 os.Stat 先行探测：
// 少一次系统调用，也避免 Stat 与 Read 之间的 TOCTOU。不存在时 found 为 false、
// text 为空串、error 为 nil。
//
// 在 Windows 上对一个目录调 os.ReadFile 返回的错误不是 fs.ErrNotExist，
// 所以这条判定不会把目录误判成「不存在」；目录、权限、其它 I/O 错一律带路径返回。
func readRulesFile(path string) (text string, found bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read rules file %s: %w", path, err)
	}
	return normalizeRulesText(string(data)), true, nil
}

// normalizeRulesText 按固定顺序去掉三种看不见的噪声。顺序不能改：
// BOM 必须在换行归一化之前剥，否则 UTF-8 BOM 会变成正文第一个字符；
// CRLF 必须在 TrimRight 之前换成 LF，否则 "\r\n" 结尾只会 Trim 掉 "\n" 留下 "\r"，
// 指纹仍然随行尾漂移。
//
//  1. 去掉开头的 UTF-8 BOM（字节 EF BB BF）。理由：PS 5.1 的默认写法会插 BOM，
//     BOM 落进系统提示是纯垃圾字节，还会让两台机器上「同一份」规则文件算出不同的指纹。
//  2. CRLF 换成 LF。理由：编辑器换一次行尾就让全队重建一次会话，那是一次看不见的
//     字节变化换一笔真金白银。
//  3. 去掉结尾的全部换行（strings.TrimRight(s, "\n")）。理由：段间分隔由渲染层负责，
//     文件末尾有几个空行不该影响指纹。
//
// 不做的：不 Trim 开头、不动行内空白、不做 markdown 解析、不做变量替换。
// 规则正文是用户的，除了上面三样看不见的噪声之外原样送。
func normalizeRulesText(s string) string {
	s = strings.TrimPrefix(s, utf8BOM)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimRight(s, "\n")
}
