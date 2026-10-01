# Step 4 API 契约（冻结）

> **订正（2026-09-30，OB1 `3317940`，#48）**：本文「council 不替用户创建配置文件 / 不创建目录」在**默认位置、文件不存在**时已被推翻：首次运行会检测本机工具、生成 `~/.council/config.toml`（见 `19d_OB1_FIRST_RUN_CONTRACT.md`）。用 `--config` 或 COUNCIL_CONFIG 指定的路径找不到时照旧报错。`DefaultTOML()` 从 #48 起是全注释的中性说明，不再是 FSDT 示例；`Default()` 没改。

`07_STEP4_SPEC.md` 说的是「做什么」，本文件说的是「导出的符号长什么样」。
Step 4 由多个执行者并行写不同的包，这份契约是它们之间唯一的接口约定。
**实现者不得改动本文件里的签名**；觉得签名有问题，退回来说，不要自行改。

## internal/config

```go
package config

// DefaultDataDir 是 D-9 定的默认数据目录。这是一个机器相关的值，
// 本项目当前只在一台机器上跑；将来跨机器时改这里，不要在调用处散落判断。
const DefaultDataDir = `E:\council-data`

const (
    EnvConfigPath = "COUNCIL_CONFIG"
    EnvDataDir    = "COUNCIL_DATA_DIR"
)

type Permission string

const (
    PermissionRead      Permission = "read"
    PermissionWorkspace Permission = "workspace"
    PermissionFull      Permission = "full"
)

type Mode string

const (
    ModeDiscussion          Mode = "discussion"
    ModeWorker              Mode = "worker"
    ModeDiscussionAndWorker Mode = "discussion_and_worker"
    ModeJudge               Mode = "judge"
    ModeManager             Mode = "manager"
)

type Config struct {
    DataDir   string                    `toml:"data_dir"`
    Providers map[string]ProviderConfig `toml:"providers"`
    Agents    map[string]AgentConfig    `toml:"agents"`
    Teams     map[string]TeamConfig     `toml:"teams"`
}

type ProviderConfig struct {
    Command string `toml:"command"`
}

type AgentConfig struct {
    DisplayName string     `toml:"display_name"`
    Provider    string     `toml:"provider"`
    Model       string     `toml:"model"`
    Effort      string     `toml:"effort"`
    Role        string     `toml:"role"`
    Permissions Permission `toml:"permissions"`
    Mode        Mode       `toml:"mode"`
}

type TeamConfig struct {
    DisplayName string   `toml:"display_name"`
    Members     []string `toml:"members"`
}

// ValidationError 的 Path 用点分路径定位到出错的键，例如 agents.theory.provider。
type ValidationError struct {
    Path string
    Msg  string
}

func (e ValidationError) Error() string

// ValidationErrors 一次带回全部错误（规格 D-10）。Error() 按 Path 字典序逐行拼接。
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string

// ResolveConfigPath 按 flagValue > 环境变量 COUNCIL_CONFIG > ~/.council/config.toml 取值。
// 空字符串视为未设置，继续向下回落。返回的路径不保证存在。
func ResolveConfigPath(flagValue string) (string, error)

// ResolveDataDir 按 cfgDataDir > 环境变量 COUNCIL_DATA_DIR > DefaultDataDir 取值。
// 取到的值必须是绝对路径，否则返回错误。不创建目录。
func ResolveDataDir(cfgDataDir string) (string, error)

// Load 读文件、解析 TOML、拒绝未知键、填缺省值、把 Permissions 与 Mode 规范成小写。
// 不做跨引用校验。
func Load(path string) (*Config, error)

// Validate 做规格 D 的全部校验，一次返回所有错误。无错时返回 nil。
func (c *Config) Validate() error

// LoadAndValidate = Load 之后 Validate。
func LoadAndValidate(path string) (*Config, error)

// Default 返回一份能通过 Validate 的最小配置。
func Default() *Config

// DefaultTOML 返回给用户初始化用的带注释配置模板文本。
func DefaultTOML() string
```

### 给实现者的硬性说明

- 未知键检测用 `toml.MetaData.Undecoded()`，错误信息里要列出每个未知键的完整点分路径。
- `Permissions` 与 `Mode` 在 `Load` 里就规范成小写；`Validate` 只判断是否属于合法枚举，不再做大小写处理。
- 缺省：`Permissions` 空 -> `read`；`Mode` 空 -> `discussion`。缺省在 `Load` 里填，不在 `Validate` 里填。
- `Load` 对「文件不存在」要返回信息里含完整路径的错误，调用方据此提示用户先建配置。
  **而且必须用 `%w` 把底层错误包进去，让 `errors.Is(err, fs.ErrNotExist)` 能穿透。**
  第一版契约只写了「含完整路径」，于是 `app` 那边为了稳妥又加了一次 `os.Stat` 兜底 ——
  多一次文件系统 I/O，还带 TOCTOU。契约这里说死，兜底就可以删。
- 依赖只许用 `github.com/BurntSushi/toml` 与标准库。不引入其它第三方包。

## internal/repl

```go
package repl

// New 构造一个 REPL。out 是唯一的输出出口，实现里禁止直接用 fmt.Print 系列写 os.Stdout。
func New(cfg *config.Config, in io.Reader, out io.Writer) *REPL

// Run 阻塞直到 /quit、输入流结束、或 ctx 被取消。正常结束返回 nil。
func (r *REPL) Run(ctx context.Context) error
```

- 遍历 `Providers` / `Agents` / `Teams` 输出时**必须先把 key 排序**。
  Go 的 map 遍历顺序随机，不排序会让测试随机失败。
- 读 stdin 放独立 goroutine，行送 channel，主循环 `select` 于该 channel 与 `ctx.Done()`。

## internal/app

```go
package app

type Options struct {
    ConfigPath string    // 来自 --config，未给则空串
    Version    string    // 打进启动横幅的版本号，由 cmd/council 传入；空串表示不显示
    In         io.Reader
    Out        io.Writer
    Err        io.Writer
}

// Run 解析路径、加载校验配置、打印启动横幅、起 REPL。
// 配置有问题时把全部错误写进 Err 并返回错误，不进 REPL。
func Run(ctx context.Context, opts Options) error
```

**启动横幅**是契约的一部分（第一版契约漏写了，补上）：写到 `Out`，含版本号、
生效的配置文件路径、生效的数据目录、agent 与 team 的数量。

**版本号走 `Options.Version`，不要用包级变量。** 第一版实现用的是 `app.Version` 包级可变变量，
`main` 写、`Run` 读 —— 那是第三个导出符号，契约没列；更实际的问题是它可变且跨包共享，
将来有并行测试同时改它与调 `Run` 时 `-race` 会报数据竞争。改成参数，依赖变显式。

## cmd/council

- flag：`--config <path>`、`--version`。**解析后若还有位置参数（`fs.NArg() > 0`）
  必须报错并退 2**，不要静默忽略 —— `council foo` 应该告诉用户 foo 没有意义。
- 信号处理见 `07_STEP4_SPEC.md` E 节，两种形状都可以，但 `os.Exit` 只许出现在本包。
- 退出码：正常 0；配置加载或校验失败 1；**flag 用法错误 / 多余位置参数 2**；第二次 Ctrl+C 130。
  （2 是按 Go 惯例补的，`07` E 节只定义了 0 / 1 / 130。）

## 不属于契约面的东西

下面这些实现可以自由决定，**测试不得对它们做字面断言**，否则会把取舍锁死：

- REPL 的提示符长什么样（当前是 `council> `）
- 告别语、未知命令提示、非命令提示的具体措辞（规格 F 只规定语义）
- 表格的列名、列序、对齐方式、空字段的占位符
- 斜杠命令是否区分大小写（当前区分，`/HELP` 走未知命令）
- `/teams` 成员的排列顺序跟随配置书写顺序（**这不违反「遍历 map 先排序」**，
  那条约束针对的是三张表的 key，成员是 TOML 数组，本就有序）

## 符号禁令（写代码时必须遵守）

代码文件里（含注释、docstring、字符串字面量、打印输出、标识符）**禁用装饰性符号**：

| 禁 | 码位 |
|---|---|
| Emoji / 象形图 | U+1F300–U+1FAFF |
| Dingbats | U+2700–U+27BF |
| 杂项符号 | U+2600–U+26FF |
| 杂项符号与箭头 | U+2B00–U+2BFF |
| 带圈字母数字 | U+2460–U+24FF |
| 隐形伴随字符 | U+FE0F、U+200D |

**放行**（不是要求纯 ASCII）：希腊字母 U+0370–U+03FF、数学算符 U+2200–U+22FF、
`°` `±` `×` `÷` `µ`、上下标、普通箭头 U+2190–U+21FF、中文、ASCII 制表符。
注释写中文可以，写 `输入 -> 输出` 或 `输入 → 输出` 都可以，写 `✅ 完成` 不可以。

原因：本机控制台默认 GBK，打印 emoji 会 `UnicodeEncodeError` 或乱码，落进日志后污染 grep 与 diff。
