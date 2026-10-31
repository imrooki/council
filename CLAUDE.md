# Council 项目记忆

## 项目速览

- 本机多模型议会：同一个问题并行交给本机已登录的 `claude` / `codex` / `grok` / `agy` CLI 子进程，可多轮、可汇总；会话、token、费用记在本地 SQLite（modernc 纯 Go 驱动）。
- 入口：`council`（TUI/REPL）、`council serve [-rw] [-app]`（本地看板，默认 127.0.0.1:8765）、`council mcp`（stdio JSON-RPC，自实现协议）、`council ask`（一次性 JSON）、`council who/claim/release/init`、Electron 桌面版 `desktop/`、dsh 插件 `contrib/dsh-subagent-council/`。
- 包职责见 `docs/README.md`。一个 `data_dir` 同时只允许一个写进程（`council.lock`）。
- 设计记录、契约、验收报告在私有仓库 `imrooki/council-notes`；代码注释里的 `#45`、`未决 #8`、`S-01`、`Plan §3.2` 指向那里。**本仓库是公开的**，不要把设计记录或未修复漏洞的利用细节写进公开文件。

## 构建与测试

```sh
go build ./cmd/council          # go build ./... 在非 Windows 上会被 cmd/council-launch 卡住（见审查 T1）
go test ./... -count=1
cd desktop && npm ci && npm test
cd contrib/dsh-subagent-council && npm test
```

- 作者只在 Windows 上实测；Linux 上 `go test ./...` 目前有约 30 个用例失败，几乎全是测试写死了 Windows 前提（见审查 T2），不是产品回归。改动后对比失败清单，别把它们当成自己引入的问题。
- `-race` 在 Windows 开发机上没跑过（没有 gcc）；在 Linux 上跑 `go test -race` 能抓到真实竞态（见 B2）。
- 版本号：仓库根 `VERSION` 与 `desktop/package.json` 必须一致，release 流水线会校验；`scripts/build.ps1` 把 git 短哈希拼进版本串。

## 写代码的约定

- 提交信息：`type(scope): 中文一句话`，例如 `fix(serve): …`、`feat(localagent): …`。
- 注释用中文，讲「为什么」与边界，密度与现有代码一致。
- 用户可见文案走 `internal/i18n`，中英两份都要加；有测试钉住「文案都被引用」。
- `local_agent = off` 是零行为承诺：不枚举进程、不起后台、不读库。新代码路径要守住。
- 写用户配置只改一段：先写临时文件校验，再备份（O_EXCL），最后 Rename。

---

## 2026-10-31 全仓审查：发现与修改方案

### 交接说明（先读这一段）

- **当前状态**：只做了审查，**一行产品代码都没改**。本文件是唯一产出。仓库 <https://github.com/imrooki/council>，分支 [`claude/fervent-meitner-eqwuu0`](https://github.com/imrooki/council/tree/claude/fervent-meitner-eqwuu0)。
- **审查方式**：按模块分 7 路逐文件读代码（turn+provider、store+conversation、serve、config+headless+mcp、cmd+localagent、repl+tui+app、desktop+前端+contrib+CI），在仓库副本里写临时测试、跑 `-race`、用假 CLI 脚本复现。每条都对过代码；标「疑似」的是缺 Windows 真机或真实 CLI 的证据。
- **复现脚本不在仓库里**：它们写在云端容器的 `/tmp`，容器回收后就没了。每条都写了触发条件，照着在本地补一个回归用例即可——修复时**先写失败用例，再改代码**。
- **没审完的**：看板（`internal/serve`）那一路中途中断，只完成了 Host 校验（S1）；导出、`/api/fs/dirs`、SSE / 长连接、设置页热加载还需要补审。前端 `index.html` 已由桌面那一路看过 XSS（没发现问题）。
- **测试注释里提到的 `.handoff` 验收记录**不在公开仓库里（在本机或私有仓库），本次审查没有看到。
- **编号**：S = 安全 / 失控，B = 正确性，T = 构建测试；P2 是小问题。修完一条就在下面删掉或标上提交哈希。

### P0：安全 / 失控花钱 / 失控改文件（先修）

- **S1 看板不校验 Host 头，DNS rebinding 可绕过全部写闸**（高）
  - 位置：`internal/serve/handler.go:139`（所有路由无 Host 白名单），`internal/serve/write.go:1286-1306`（只比 Origin 与 `r.Host`）。
  - 后果：恶意网页把自己的域名重绑到 127.0.0.1 后，Origin 和 Host 都是它自己的域名，同源检查通过；`-rw` 下能改 provider command/env、跑 `full` 权限议会；只读模式下也能读全部会话和 `/api/settings` 里的 provider env（`settings.go:395`，可能含密钥）。
  - 方案：在 `logRequests` 外再包一层中间件，对**所有**请求（含 GET 和 `/`）要求 Host ∈ {`127.0.0.1:<实际端口>`、`localhost:<端口>`、`[::1]:<端口>`}，否则 421/403；`-addr 127.0.0.1:0` 时用 listener 的实际端口。加用例：伪造 Host 的 GET/POST 都被拒；桌面版与 `council who -server` 仍能用。

- **S2 项目层配置可覆盖 providers / rules_file：不可信仓库即可执行任意命令**（高）
  - 位置：`internal/config/merge.go:169-172`（rules_file）、`:194-197`（providers 整段覆盖），层来源 `:67-79`。
  - 后果：仓库里放 `.council/config.toml`，写 `[providers.claude] command = …` 或 `env = ['ANTHROPIC_BASE_URL=…']`，在该目录跑 `council` / `council ask` / 宿主拉起的 `council mcp` 就会执行它、或把 prompt 与凭据引走；`rules_file = '/abs/secret'` 能把本机任意文件塞进系统提示发给模型。data_dir 已经对项目层做了忽略，providers 却没有。
  - 方案：项目层只放行 `agents` / `teams` / `summary` / `language`；项目层出现 `providers`、`rules_file`、`local_agent` 时忽略并给 notice（照 data_dir 的写法）。另外 `rules_file` 要求落在配置文件目录内。补用例覆盖这几个键。

- **S3 桌面版退出 / 升级时硬杀 serve，CLI 子进程成孤儿继续跑**（高）
  - 位置：`desktop/main.js:1177-1224`（`child.kill()`，2 秒后 `taskkill /T`），`cmd/council/main.go:300`（只监听 `os.Interrupt`）。
  - 后果：Windows 上 `child.kill()` 就是 TerminateProcess，serve 来不及 drain jobs、杀进程树，claude/codex 继续花钱、继续改文件；serve 已死，2 秒后的 `taskkill /T` 也找不到子树。Unix 上 SIGTERM 同样不走优雅退出。
  - 方案：Go 侧 serve 增加「stdin EOF 即优雅退出」（桌面版 spawn 时 stdin 用 pipe，退出时关 pipe），Unix 上同时监听 SIGTERM；Windows 上给 CLI 子进程挂 Job Object（`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`），父进程怎么死都能带走子树。兜底的 taskkill 改成在 kill 之前、用 System32 绝对路径执行。

- **S4 单成员 5 分钟超时不是硬上限**（高）
  - 位置：`internal/turn/spawn.go:186-206`。
  - 后果：stdout/stderr 是 capWriter（不是文件），`cmd.Wait()` 要等管道 EOF；CLI 留下的后台子进程继承了管道就会卡住 Wait。同进程组时：CLI 正常 exit 0 也被记成超时，已付费的回答被丢；用 setsid 逃出进程组时：`<-waitErr` 无限阻塞，整轮、取消、headless 总超时全卡住。
  - 方案：`cmd.WaitDelay = 2*time.Second`，把 `exec.ErrWaitDelay` 当正常退出；超时分支的 `<-waitErr` 也加上限。用假 CLI 脚本（`sleep 6 &` / `setsid sleep 6 &`）写回归用例（非 Windows 加 build tag）。

- **S5 provider 级 args 不过滤，可静默放大所有成员的权限**（中）
  - 位置：`internal/turn/providerinvocation.go`（args 原样追加在权限 flag 之后）。
  - 例子：agy 加 `--dangerously-skip-permissions`，codex 加 `-c sandbox_mode=danger-full-access`，claude 加 `--add-dir`；READ 档成员也会受影响。设置页能改 args，S1 修好之前网页也能改。
  - 方案：在 `config.Validate` 里按 provider 维护权限类 flag 黑名单并拒绝；指纹变化提示保留。

- **S6 `.claude/agents` 导入的成员名不校验就当 id 用**（中）
  - 位置：`internal/config/claude.go:253`、`:89`，`internal/mcp/claude.go:74-88`（serve 导入同样）。
  - 后果：`name: ../../../x` 会让 workdir 越出 data_dir 并在那里写文件；`name: user` 撞保留 sender。
  - 方案：扫描时套 `NewAgentIDProblem`，不合规就跳过并给 notice；没写 model 时不要传 `--model ""`。

### P1：钱算错 / 数据损坏 / 承诺不成立

- **B1 预算少算**：`internal/turn/runner.go:816-829, 1533-1537`、`internal/headless/headless.go:836-841`。claude 的 delta 为 nil（失败轮、基线可疑后的下一轮、续接旧会话继承的可疑状态）时按 0 计，预算看到的比实际花的少。spawn 失败（进程根本没起来，`runner.go:810-814 → 1121-1125`）也会把基线标成可疑。方案：预算单独一个保守口径（累计 − 最后可信基线）；没 Start 的失败不标可疑。另外 ask 模式预算只在轮后检查，工具描述写的 "Hard cost cap" 要改口径或改实现。
- **B2 身份指纹读 `r.cfg` 而不是本轮快照**：`runner.go:499`、`budget.go:117`、`drift.go:150`。这是数据竞争（`-race` 已抓到），而且 argv 用的是快照、指纹用的是新配置，provider args 改了会话照样续。方案：三处都传快照 cfg。
- **B3 旧版程序能以写模式打开新版库**：`internal/store/schema.go:752` 只跳过 `version <= current`，不判 `current > expected`，而只读路径会拒绝，口径相反。方案：`migrate` 遇到 `current > expected` 返回 `ErrSchemaVersionMismatch`。
- **B4 Unix 上删掉 `council.lock` 后排他失效**：`internal/store/lock_other.go:34-39` flock 锁的是文件 inode。方案：改为 flock data_dir 目录 fd；文案写明不要删锁文件。
- **B5 设置页按行改 TOML 时不跟踪多行字符串**：`internal/config/agentwrite.go:347-361, 478-489`。role 里有 `[输出格式]` 或 `model = …` 这样的行时，新值会被插进字符串里，返回成功但改动没生效，role 也坏了。多行数组的 args/env/models 也存不了（`:419-440`）。方案：扫描维护多行字符串 / 数组状态；写后解析比对结构，不一致就拒绝写入。
- **B6 原子写把符号链接换成普通文件**：`internal/config/append.go:244`、`agentwrite.go:192`。方案：先 `EvalSymlinks` 再写。
- **B7 contrib 插件超时杀不到 provider CLI**：`contrib/dsh-subagent-council/lib/run.js:176` 只杀 council 的进程组，而 CLI 是 `Setpgid` 起在另一个组里（`internal/turn/killtree_other.go:15`）；默认 570s 比 council 自身 10 分钟超时还短，又没传 `-timeout`。方案：先 SIGINT 宽限再 SIGKILL，并传略小于 timeoutMs 的 `-timeout`。
- **B8 `council release` 在 off 档会建库、跑迁移、拿锁**：`cmd/council/claim.go:432-441`，违反 off 零行为。方案：先 OpenReadOnly，库不存在就答「无可交还」；dataDir 为空时单独报错。
- **B9 租约冲突只按 key 全等**：`internal/localagent/lease.go:84-119`、`internal/store/localagent.go:494`。`/repo` 与 `/repo/sub` 能同时拿写租约；`tcp:8765` / `tcp:localhost:8765` / `tcp:[::1]:8765` 是不同的 key，`tcp:not a host!:80` 也能通过。方案：claim 时做路径重叠检查；tcp 用 SplitHostPort/ParseIP 校验，回环别名归一。
- **B10 租约事件 detail 用 `%q` 拼 JSON**：`internal/store/localagent.go:408-410, 567`，控制字符会生成非法 JSON。方案：`json.Marshal`。
- **B11 租约 TTL 先乘后校验会溢出**：`internal/serve/localagent.go:296`，`ttl_seconds=18446744079` 变成 5 秒左右并被接受。方案：先按秒比较上下限。
- **B12 默认数据目录写死 `E:\council-data`**：`internal/config/schema.go:11`。README 的最小配置不带 data_dir，在没有 E 盘的 Windows 和所有 Linux/macOS 上直接起不来。方案：默认改为配置文件旁的 `data/`，或 `os.UserConfigDir()/council`。
- **B13 `init` 的目标可能落到项目层**：`cmd/council/init.go:156-161` 取 `layers[0].Path`，cwd 下有 `.council/config.toml` 时指向它，全局配置永远建不出来，桌面向导以为已配好。方案：改用 `config.ResolveConfigPath`。同类问题：`WriteDataDir`（`internal/config/teamwrite.go:52-56`）会写进项目层，而合并时项目层 data_dir 被忽略。
- **B14 `council ask -workspace … -conversation …` 追问必定失败**：`internal/headless/workspace.go:77-79` 副本路径只按 convID 拼，contrib 插件追问时正好这样传。方案：副本名加序号或时间戳。
- **B15 桌面版版本检查与只读看板**：`desktop/main.js:1851` + `desktop/lib/version.js`，`--version` 失败时返回 `'(未知)'`，会被判成不同版本、拒绝 attach 并弹「旧版本」退出；`resolveCouncilExeSync`（`:1624`）不认 `COUNCIL_EXE`；primary-retry 那条 attach（`:1450-1456`）跳过版本检查。只读看板的 health 里没有 `writable` 键（omitempty），`:1271` 的 `writable === false` 是死代码，会静默挂到只读看板（例如 contrib `council_ui` 拉起的那个）。方案：未知版本放行；所有 attach 路径都做版本检查；`writable !== true` 时另起一个或明确提示。
- **B16 `council-launch` 只看进程名判断看板在跑**：`cmd/council-launch/main.go:48-51, 116-122`，任何 council.exe（`council mcp`、TUI、`council who`）都会让它不起 serve，只开一个连不上的 8765 窗口；tasklist 失败又会重复起 serve 并静默撞锁。方案：照桌面版探测 `/api/health`。
- **B18 相对路径的 rules_file 被解析到配置目录的上一级**：`internal/app/app.go:208`、`internal/serve/write.go:387` 把 `merged.RulesFileBase`（已经是目录，`config/merge.go:171`）传给 `config.LoadRules`，而它又取一次 `filepath.Dir`（`config/rules.go:96`）。把模板里的 `rules_file = 'COUNCIL.md'` 取消注释即可触发：启动时局部规则 found=false，被静默当成没有规则；在任何 `--apply` 之前，`/rules reload`（`internal/repl/rules.go:149-153`）回落用 ConfigPath（文件路径），又能读到规则，于是报全部会话漂移，`--rebuild` 还要付入场费；之后 `/config reload --apply` 或 `/add` 把 base 换成目录，规则又丢。方案：统一 LoadRules 的参数语义（改成接收目录并改名），三个调用点一起改，并加「rules_file 相对路径」的回归用例。
- **B19 复制模式「列 → rune」的映射与渲染用的宽度算法不一致**：`internal/tui/copyscreen.go:443, 669, 688` 用 go-runewidth（东亚宽度随 `LANG` 或控制台代码页 936 变化），渲染（`:582` 的 `ansi.Truncate` 和 bubbletea）用 x/ansi（东亚宽度固定为 false）。`LANG=zh_CN.UTF-8` 下，行「“中”abc」拖选 ab，复制得到 `"”"`；带 VS16 的 emoji（如「⚠️abc」）在任何 locale 下都错位。方案：映射改用与渲染相同的 ansi 宽度算法，按字形簇遍历。
- **B20 非 Windows 的 IsTerminal 把 /dev/null 当终端**：`internal/repl/console_other.go:19` 只看 `ModeCharDevice`。`council < /dev/null` 进入 TUI 后报「REPL 异常结束: bubbletea: could not create cancelable reader…」并退 1；Windows 下 NUL 走行式模式，正常退 0。方案：改用 `golang.org/x/term.IsTerminal`。
- **B21 `/copy` 与复制屏加载期间不置 busy**：`internal/tui/copy.go:204-214`、`copyscreen.go:161, 271, 327`。`/copy` 回车后、或 Ctrl+G 加载中按 Esc 后马上提交别的命令，CopyRecent / CopyScreen 与 Submit 会并发读写 `r.cur` / `r.cfg`（`-race` 抓到 `repl/registry.go:149/156` 对 `commands.go:735/739`）；`/use` 之后可能复制到旧会话的内容；旧的加载结果没有代号，会被下一次 Ctrl+G 当成自己的结果；`copyDoneMsg` 在复制屏里被吞，剪贴板没写。方案：这两条路径置 busy 或串行执行，结果消息带代号。
- **B17 便携版问题**（读源码核实，未在 Windows 实测）：`desktop/main.js:1813-1827` 把 `%TEMP%` 下会被删除的 exe 注册成 `council://` 处理程序，并覆盖安装版的注册；`:1889` 自动更新会给便携版装一份 Setup，便携版本身永远停在旧版。方案：检测 `PORTABLE_EXECUTABLE_DIR` 时跳过协议注册并禁用 updater。

### P2：小问题与文案

- `council claim <资源> -mode …`：帮助写资源在前，Go flag 遇到位置参数就停，按帮助敲会退 2（`claim.go:152-166, 549`）。改帮助为 `[flags] <资源>` 或支持交错。
- claim 成功 / 冲突提示写 `council release <id>`，实际要 `-lease <id>`（`claim.go:314, 327`，`messages_cli.go` 中英各两处）。
- `-server` 路径与本地路径并不「完全一样」（`cmd/council/leaseclient.go`）：owner_kind 写死 `dashboard`、冲突 JSON 没有 holder、英文模式中英混排；release `-server` 把任何 404 当「没有这条租约」。要么对齐，要么改 README:96。
- `who -server` 在看板 off 档时打印三段「没有…」，像「本机很干净」（`leaseclient.go:301-334`）。
- `init -config my.toml`（相对路径）写出 `data_dir = "data"` 后校验失败（`init.go:173, 356`）；文本模式总打印「已存在，不会被覆盖」（`:215`）；用法错误退 1 而不是 2。
- 显式 `-config` 指向不存在的文件时被当成 off（`who.go:294-297`）。
- MCP：不带 id 的 `tools/call` 被当 notification 照样执行、花钱、不回包（`internal/mcp/server.go:263, 348-352`）；工具参数未知字段被静默忽略，与 schema 的 `additionalProperties:false` 不符（`tools.go:224`）。
- 单成员 `-mode round` 必然 partial，汇总被静默跳过（`headless.go:543-575`）。
- `git status --porcelain` 未处理引号与重命名（`headless/workspace.go:114-131`），改用 `-z`。
- 首行带 BOM 的顶层键会被重复插入（`config/langwrite.go:61`）。
- 时间列按 TEXT 排序，`RFC3339Nano` 去尾零导致顺序错（`store/open.go:301-303` 等），改定宽格式并迁移旧数据。
- 只读 Store 只在打开时探测一次 schema（`store/open_readonly.go:92-102`）；OpenReadOnly 接受 v1–v4 但部分面板依赖 v5；`WriteTx` 在只读 Store 上 nil panic。
- `RecordTurn` 在事务外用会过滤软删的读路径预检（`store/service.go:348`），轮次中会话被软删时回复正文丢失；`requireConversation` 不判 `deleted_at`（`store/tx.go:145-147`）。
- UNC / `\\?\` 形式的 data_dir 打不开库（疑似，`store/open.go:108-114`）。
- `unpackFingerprint` 中 `start+n` 溢出导致 panic（`turn/session.go:397-399`），先判 `n > len(s)-start`。
- grok/agy 首轮正文以问题开头，问题以 `-` 开头时会被当成 flag（`provider/grok.go:100`、`agy.go:104`，未用真 CLI 复现）。
- READ 档 `council-role.txt` 在 serve 并发 job 下被并发截断重写（疑似，`turn/spawn.go:140-148`）。
- 疑似：claude 的 `total_cost_usd` 在共用 cwd、交错续接多个会话时是否仍是会话累计值，需真机验证（`provider/claude.go:325`）。
- 桌面版：读回地址的正则不要求行尾，chunk 被切开会读错端口（`main.js:407`）；窗口只露底部 40px 就判可见（`:911-917`）；裸名 `taskkill` / `council` 在 Windows 上会先搜 cwd（`main.js:1211`、`contrib/.../run.js:73, 159`），改绝对路径。
- 发版流水线（`.github/workflows/release.yml`）：dispatch 时没 checkout 指定 tag 却 `--clobber` 覆盖该 tag 的产物；concurrency 按 `github.ref` 分组，同一版本可能并发上传导致 latest.yml 与安装包不对应；签名密钥是 job 级环境变量。改为按 tag 分组、checkout `ref: <tag>`、latest.yml 最后上传、密钥只给打包步骤、`persist-credentials: false`。
- 配置模板注释引用了已移走的 `docs/06_SECURITY_MODEL.md`（`internal/config/defaults.go:98`）。
- `--config` 恰好指向自动探测到的项目层文件时，启动只读这一层，`reloadLayers` 却返回 [项目层, 全局层]（`internal/repl/configlayers.go:47`）：`/config reload` 报出全局层的假「新增」，`/add +编号` 的 DeepEqual 比较（`addmodel.go:479`）永远不等、一直拒绝，`--apply` 之后语义悄悄变成合并。把启动时的层列表传给 REPL，不要重算。
- @ 补全时光标在 token 中间，右边的残余字符会留着：`hi @alice` 按两次 ← 再回车，得到 `hi @alice ce`（`internal/tui/model.go:826-838`）。
- 命令只认半角 `/`（`internal/repl/commands.go:183`、`seam.go:68`）：全角 `／use 123` 被当成发言落库，TUI 默认开自动回复时还会问全员一轮、花钱。`/to` 的 id 列表也不认全角逗号（`commands.go:420`）。
- `/cost --round` 要求完整 round_id，但界面只显示末 6 位（`repl/roundobs.go:190`），照抄一定得到「没有记账行」（`repl/cost.go:332`）。支持后缀匹配，或打印完整 id。
- TUI 小问题：runCtx 正常结束时不调 cancel，子 context 一直挂在根 ctx 上（`tui/model.go:417, 898`）；复制屏底栏不按宽度截断，终端高度 ≤ 2 时帧高超出（`copyscreen.go:596`）；复制屏提示、`ErrCopyEmpty`、`ErrCopyUsage`、`repl/copyscreen.go:47`、`app/firstrun.go`、`app.go:178-179` 的文案没有走 i18n。

### T：构建 / 测试可移植性

- **T1** `cmd/council-launch/main.go` 用了 Windows 才有的 `SysProcAttr` 字段，缺 `//go:build windows`，`go build ./...` 在 Linux/macOS 失败，而 `docs/README.md` 写的就是 `go build ./...`。
- **T2** Linux 上失败的测试（`cmd/council`、`internal/app`、`internal/config`、`internal/store`、`internal/turn`）：`.cmd` 假脚本没有执行位、写死 `C:\` / `E:\` / `D:\` 路径、依赖 Windows 的 ErrNotExist 语义。按平台加 build tag 或 `t.Skip`；`TestMergeProviderEnvOverridesCaseInsensitivelyOnWindows` 非 Windows 分支的期望本身写错（两条都是 `PATH`）。
- **T3** CI 只在发版时跑，不跑 desktop / contrib 的测试，也从没跑过 `-race`。加一个 push/PR 触发的 Linux job：`go vet`、`go test -race ./...`、两处 `npm test`。contrib 测试在 Node 22 下有 13 个被 cancelled，原因是 `run.js:140/211` 对计时器调了 unref。

### 修复顺序

1. **S1 + S2 + S5**：一组安全修复，互相独立，各自带回归用例；修完再补审 serve 剩余部分。
2. **S3 + S4 + B7**：进程生命周期。共同验收：任何退出路径（桌面退出、升级、超时、插件超时）之后，假 CLI 及其子进程都不残留。
3. **B1 + B2**：费用与指纹，先在 Linux 上开 `-race`（T3）再动。
4. **B3 + B4 + B5 + B6 + B10 + B11**：数据完整性。
5. **B18**：影响所有用相对路径 rules_file 的人，改动小，可以提前到第 1 步之后。
6. **B8 + B9 + B12 + B13 + B14 + B15 + B16 + B17 + B19 + B20 + B21** 与 P2 里的 CLI / TUI 小问题。
7. **T1–T3** 可以最先做（改动小、让后续每一步都能在 Linux CI 上验证），建议和第 1 步并行。

每修一条，在这里把它划掉或删掉，并写上提交哈希。
