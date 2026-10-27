# Council

在同一个终端会话里，让 claude、codex、grok、agy 这几家命令行工具里的多个模型一起讨论。
每个成员由「哪家工具 + 哪个模型 + 多高的思考强度 + 角色 + 权限」自由组合；会话、用量和费用都记在本地的库里。

本地优先，命令行优先：council 不替你登录任何服务，只调用你机器上已经装好、已经登录的那几家工具。

## 状态

- 作者自用的版本，当前版本号 `0.2.0-1001b`。
- 自动测试是全绿的，但最近几批功能（首次运行自动配置、`/add` 按名字加模型、对话模式等）只在测试里验证过，没有逐项在真机上复验。
- 只在 Windows 上实测过。代码里有非 Windows 的分支，Linux 下做过静态检查，没有人在 Linux 或 macOS 上运行过。
- 尚未指定许可证。

## 需要什么

- Go 1.25 或更新，用来编译。
- git。
- claude、codex、grok、agy 里至少一家，已经装好并且已经登录。Council 本身不带任何模型。
- 直接打字会让会话里的成员回复，用的是你自己那几家的订阅额度或 API 费用。

## 让 AI 帮你部署

把下面整段复制，贴给你的 AI 编程助手（Claude Code、Codex 等任何能在你电脑上执行命令的）。

```text
你是我的部署助手。请把 Council（一个本地的多模型协作命令行工具，Go 写的）装到我这台电脑上。仓库：https://github.com/imrooki/council

先看规则，再动手：
1. 只做下面列出的步骤。每一步先说你要做什么，做完说结果。任何一步失败，停下来把原文报错给我，不要自己换办法绕过去。
2. 不要输入任何密码、API key、令牌；不要运行任何 login / logout 之类会改动登录状态的命令；需要登录时把命令告诉我，由我自己在终端里敲。
3. 不要读取、打印或修改各家工具的配置和凭据：~/.claude、~/.codex、~/.grok、~/.gemini，以及任何 .env 文件。
4. 不要改系统环境变量和 PATH，除非我明确同意。
5. 不要启动 council 的交互界面（它是全屏终端界面，你驱动不了），也不要替我在里面提问：在 council 里直接打字会让成员回复，花的是我的订阅额度或 API 费用。

步骤：

A. 检查环境，只读：
 - git --version
 - go version。需要 Go 1.25 或更新。没有或太旧，告诉我去 https://go.dev/dl/ 装，不要替我装，装好后再继续。
 - 检查 claude、codex、grok、agy 是否在 PATH 上（Windows 用 Get-Command，macOS 和 Linux 用 which），只报有或没有，以及路径。council 至少要其中一家，而且是已登录的。
 - 如果这不是 Windows，先告诉我：作者只在 Windows 上实测过，其它系统没有人运行过，问我还要不要继续。

B. 取代码：问我装到哪个目录，在那里 git clone https://github.com/imrooki/council.git，然后进入 council 目录。如果 clone 要求登录或提示没有权限，告诉我，不要自己去找凭据。

C. 编译：
 - Windows：go build -ldflags "-X main.version=0.2.0-local" -o council.exe ./cmd/council
 - macOS 和 Linux：go build -ldflags "-X main.version=0.2.0-local" -o council ./cmd/council
 第一次编译要联网下载依赖，可能要几十秒到几分钟。下载超时就告诉我，经我同意后可以只在当前这一条命令里把 GOPROXY 设成 https://goproxy.cn,direct。
 不要用 go install github.com/imrooki/council/... 的写法，模块名是 council，那样不行，必须先 clone 再编译。

D. 放到能直接运行的位置：告诉我有两个办法，让我选：
 (1) 把可执行文件放进一个已经在 PATH 里的目录；
 (2) 不动 PATH，以后用完整路径运行。
 我选了之后照做，并把最终的完整路径告诉我。

E. 验证：运行 council -version，应该打印一行 council 0.2.0-local。

F. 不要自己跑测试。如果我要求跑：先只在当前这一条命令里，把 USERPROFILE、HOME、APPDATA、LOCALAPPDATA、TEMP、TMP、COUNCIL_DATA_DIR 指到一个临时目录，再运行 go test ./... -count=1，跑完删掉那个临时目录。

G. 收尾，告诉我：
 - council 装在哪、版本是多少；
 - 第 A 步里哪几家工具有、哪几家没有；
 - 我接下来要自己做的事：
   (1) 在终端里登录至少一家：claude auth login、codex login、grok login；agy 运行后按提示登录。
   (2) 在终端里运行 council。第一次运行会自动找本机的工具、生成 ~/.council/config.toml，这一步不花钱。
   (3) 进去之后先敲 /new <标题> 建一个会话，再敲 /add 看能加哪些成员。
   (4) 想先不花钱，就用 council -no-auto 启动：这样直接打的字只记下，不让成员回复。
```

## 自己动手

```bash
git clone https://github.com/imrooki/council.git
cd council
go build -ldflags "-X main.version=0.2.0-local" -o council.exe ./cmd/council
./council.exe -version
```

不能直接 `go install github.com/imrooki/council/...`：模块名是 `council`，必须先 clone。

## 第一次用

1. 在终端里登录好至少一家工具：`claude auth login`、`codex login`、`grok login`，agy 运行后按提示登录。
2. 运行 `council`。`~/.council/config.toml` 不存在时，它会找本机装了哪几家、让每家列一次模型（不提问、不花钱）、报每家的登录状态，并生成一份配置。
3. 敲 `/new 标题` 建会话，`/add` 看能加哪些模型，`/add <模型名>` 或 `/add +编号` 加成员。
4. 想先不花钱：用 `council -no-auto` 启动。默认是对话模式，直接打字会问当前会话的全部成员。
5. 下次再进来：`/resume` 列出以前的会话，`/resume 1` 按编号接上。默认只列你启动 council 时所在项目目录的会话（就是启动时所在的那个目录，不向上找仓库根，从子目录启动算另一个项目），`/resume --all` 看全部。
6. `/help` 看全部命令，`/quit` 退出。

项目结构、构建与测试见 [docs/README.md](docs/README.md)。

## 看板（本地网页）

`council serve` 默认起一个**只读**的本地网页，看已有会话、消息、每个成员的用量与费用。它**不写库、不跑迁移、不抢 `council.lock`**，所以 TUI 开着的时候也能同时看。

```sh
council serve              # 默认 http://127.0.0.1:8765
council serve -open        # 顺手打开浏览器
council serve -app         # 用 Edge/Chrome 的应用窗口打开：没有地址栏、没有标签页（仅 Windows）
council serve -addr 127.0.0.1:9000

council serve -rw          # 写模式：网页底部可以输入问题、跑一次议会
```

- **`-app`（仅 Windows）**：优先找 Edge（`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`，再试 `Program Files` 下的那份），找不到再找 Chrome（`Program Files\Google\Chrome\Application\chrome.exe` 等），用 `--app=<url>` 起一个 1440x900 的窗口，看起来像独立桌面应用。只读与 `-rw` 两种模式都支持。
- `-app` 用独立的 Chromium profile：`<data_dir>\ui-profile`，不碰你日常浏览器的会话与扩展；这个目录留在 `data_dir` 里，下次 `-app` 继续用同一份窗口配置；每次 `serve` 启动会清掉其中超过 7 天没动过的条目（目录本身不删，正在用的窗口不受影响）。
- 找不到 Edge/Chrome（或者不在 Windows 上）时不会报错退出：打印一行说明，退回 `-open` 的行为，服务照常开着。`-app` 与 `-open` 同时给时按 `-app` 处理，并打印一行说明。
- **窗口是独立的浏览器进程**：Ctrl-C 关掉 `serve` 之后窗口仍然开着，这是预期；关掉窗口也不影响 `serve`。要关窗口请自己关它，`serve` 不会替你收走。
- **只绑回环地址**：`-addr` 传非 `127.0.0.1`/`::1` 会直接退 2。
- **没有登录与鉴权**：这是本机工具，本机任何进程都能读这个端口上的数据。别把它暴露到局域网。
- 库的 schema 版本必须与当前二进制一致；库还是旧版本时它会让你先用新版 `council` 打开一次（只读打开不迁移，这是刻意的）。
- 端点：`/api/health`、`/api/conversations`、`/api/conversations/{id}`、`/api/conversations/{id}/messages`、`/api/conversations/{id}/usage`。

### `-rw` 写模式

`council serve -rw` 会拿 `council.lock`、跑迁移，并在网页底部放出输入框、「跑一轮」按钮和成员多选。跑一次议会走的就是 `council ask` 同一套执行路径：建会话、记录问题、按成员跑轮次、可选汇总，花的是你配置里那几家 CLI 的额度。

- **`-rw` 下本机任何进程都能通过这个端口跑议会（花你的额度）。** 不做鉴权，这是本机工具的取舍；写端点只接受 `POST`，读端点仍可用。
- 写端点要求 `Content-Type: application/json`，否则 415；浏览器请求带 `Origin` / `Sec-Fetch-Site` 时必须是同源，否则 403。本机的 curl、脚本、工具调用没有这些头，照常可用。
- 同一个 `data_dir` 只能有一个写进程：拿不到锁时 `serve` 直接退 1，提示你先关掉另一个 council（TUI 或另一个 `serve -rw`）。
- `-rw` 与 TUI 不能同时开；`serve -rw` 开着时不要再启动交互界面。
- 启动时会做 inflight 对账（清掉上次崩溃留下的在飞记录）；**不做** TUI 的 AuditDrift 漂移提醒，所以改 model / effort / 规则文件后不会有启动提示。
- 进度用 SSE 推：`POST /api/ask` 立即返回 `202 {"job_id":"…"}`，`GET /api/jobs/{id}/events` 依次给 `started` / `turn` / `done`。关掉浏览器**不会**取消正在跑的议会；任务结束 60 秒后从内存表清理。
- Ctrl-C 关闭时会先等运行中的议会收尾（10 秒上限）；到点还没完就显式取消，再等 2 秒，然后才关库放锁。
- 已知限制：任务结束 60 秒后 job 从内存删除，如果浏览器在这之后才重连 EventSource，会收到 404 并按浏览器策略持续重试；刷新页面即可恢复。

## 作为 MCP 服务器使用

`council mcp` 在 stdio 上跑一个 MCP 服务器（JSON-RPC 2.0，一行一条消息，协议版本 `2024-11-05`），让 Claude Code 之类的宿主把议会当工具调用。它不引额外依赖，也不另写一套执行路径：`tools/call` 走的就是 `council ask` 的 `headless.Run`，花的是你配置里那几家 CLI 的额度。

Claude Code：

```sh
claude mcp add council -- "C:\Users\you\bin\council.exe" mcp
```

`--` 之后的第一个词必须是 `council.exe` 的完整路径（路径带空格要加引号），第二个词是子命令 `mcp`。加完用 `claude mcp list` 能看到 `council`。要固定用某份配置，再跟 `-config`：

```sh
claude mcp add council -- "C:\Users\you\bin\council.exe" mcp -config "D:\work\project\.council\config.toml"
```

三个工具：

| 工具 | 作用 | 参数 |
|---|---|---|
| `council_ask` | 每个成员答一轮，可选汇总 | `prompt`（必填）、`members`、`rounds`（只能是 1）、`summary`、`budget_usd`、`cwd`、`conversation`、`lang`（`zh` / `en`） |
| `council_models` | 列出可用成员：id、显示名、provider、model、权限；`.claude/agents` 导入的标 `source=claude` | 无 |
| `council_round` | 1–5 轮议会 + 汇总 | `prompt`（必填）、`members`、`rounds`（默认 2）、`summary`、`budget_usd` |

- `members` 省略或给 `[]` 时用配置里的全部成员。`council_ask` 的「全部」不含 `.claude/agents` 导入的成员（与 CLI 的 `council ask` 同口径），要问它们得显式写 id——`council_models` 里 `source=claude` 的那些。
- `budget_usd` 下限 0.5，默认 5；`rounds` 只接受 1–5，`council_ask` 只接受 1。参数不合法时工具返回 `isError` 文本，不是 JSON-RPC error（协议错误才用 error）。
- `cwd` 必须是已存在的绝对路径；`conversation` 续接已有会话（id 从返回文本的 `会话:` 行拿）。
- stdout 只有协议消息，日志全部走 stderr。stdin 读到底之后先让在飞的工具跑完再退出（工具自己有 10 分钟超时）；工具失败时进程退出码仍是 0（宿主不看它），原因在 stderr 与 `isError` 文本里。
- 同一个 `data_dir` 只能有一个写进程：`council mcp` 开着时不要再开 TUI 或 `serve -rw`，否则它会报锁冲突后退出。
- `council mcp` 不触发首次运行的自动配置（不在宿主启动路径上探测本机 CLI、往家目录写文件）。第一次用之前先跑一次 `council` 生成配置。

dsh 那边不用这一节：本仓库里的 dsh 集成是 `contrib/dsh-subagent-council` 的 Cordis 插件（`council` / `council_work` 工具），不是 MCP。

## 桌面版（Electron 外壳）

`desktop/` 是工作台的原生桌面外壳：双击打好的 exe 就是一个 1440×900 的窗口，没有地址栏、没有菜单栏，也不依赖本机装 Edge/Chrome。窗口里的页面仍然是 `council serve -rw`；关窗口时，外壳只会结束**自己拉起的那个** `council.exe`。如果 8765 上已经有一个 council 在跑，外壳直接连过去，退出时不会动它。

### 开发运行

```powershell
cd desktop
npm install
npm start
```

默认先探测 `http://127.0.0.1:8765`（每次 1.5 秒，最多三次）；已经有工作台就直接连，**不会再拉第二个 council**。8765 没响应时才自启：8765 空闲就用 8765，被别的程序占用就退到系统分配的空闲端口。

- `npm start -- --council-exe D:\path\council.exe`：开发时指定另一个 council。
- `COUNCIL_URL=http://127.0.0.1:9000`：额外的连接目标。先看 8765，8765 没工作台时才看它；自启端口仍按上一条，不会用 `COUNCIL_URL` 的端口去抢同一个 `data_dir`。
- `COUNCIL_DESKTOP_PORT=9000`（高级/测试）：把主探测端口从 8765 改成 9000。正常使用不要设置，否则会绕过对 8765 上已有实例的自动 attach。
- `COUNCIL_CWD`：council 的工作目录，默认仓库根；打包后默认是 exe 所在目录（portable 为解压前的位置，安装版为安装目录）。
- `COUNCIL_EXE` / `COUNCIL_CONFIG` / `COUNCIL_DATA_DIR` 等环境变量原样传给子进程。

启动失败（找不到 exe、端口/数据目录被占、20 秒内 health 没通）会弹原生错误框，并附上子进程 stderr 的末尾。

启动路径可以直接看到，便于排查：

- 窗口标题带 `· 已连接 127.0.0.1:8765` 或 `· 自启 127.0.0.1:<port>`；
- 页面根元素上有 `data-council-mode`（`attach` / `spawn`）、`data-council-url`、`data-council-source`；
- `%APPDATA%\Council\desktop.log` 记录每次探测、attach/spawn、端口、pid 与子进程退出码。

### 打包

```powershell
cd desktop
$env:COUNCIL_EXE = 'C:\path\to\council.exe'   # 可选，默认 %USERPROFILE%\bin\council.exe
npm run dist
```

产物在 `desktop/dist/`：

| 文件 | 用途 |
| --- | --- |
| `Council-0.1.0-portable.exe` | 单文件便携版，双击即用，不需要安装 |
| `Council-Setup-0.1.0.exe` | NSIS 安装器，`oneClick: false`，可选安装目录 |
| `win-unpacked\Council.exe` | 未打包的目录版，调试用 |

打包前 `scripts/prepare-council.js` 会把 council.exe 复制到 `desktop/vendor/`，electron-builder 再通过 `extraResources` 放到 `resources\council.exe`；仓库里不存这份二进制。窗口图标来自 `build/icon.ico`，运行时图标额外放一份在 `assets/icon.ico`（避开 NSIS 生成安装器时对 build 目录的读取）。

### 依赖下载源（本次实测）

- npm 包：默认 `https://registry.npmjs.org/`，可用（283 个包约 7 秒）。
- Electron 45.0.0 二进制：默认 `@electron/get` 的 GitHub release，下载成功；解压后约 374 MB。
- electron-builder 的 7zip / NSIS 二进制：默认 GitHub，下载成功。

网络慢时可以只对当前命令换镜像：

```powershell
$env:npm_config_registry='https://registry.npmmirror.com'
$env:ELECTRON_MIRROR='https://npmmirror.com/mirrors/electron/'
$env:ELECTRON_BUILDER_BINARIES_MIRROR='https://npmmirror.com/mirrors/electron-builder-binaries/'
npm install
npm run dist
```

> Electron 45 的 npm 包不再在依赖安装阶段自动下载二进制，而是提供 `install-electron`；`desktop/package.json` 的 `postinstall` 已经接上，`npm start` 第一次也会按需补下载。

### 已知限制

设置页的「切语言」目前会写入 `config.toml`，但界面文案要重启后才换：`council serve` 启动时把解析结果写回了 `COUNCIL_LANG`，设置页会明确提示「环境变量仍优先」。这是 Go 侧既有行为，外壳没有改 Go，也没有在桌面层绕过。

## 文档

- [docs/README.md](docs/README.md)：代码结构、构建、测试与文档边界。
- 决策记录、实施契约、验收记录与交接笔记在私有仓库 [council-notes](https://github.com/imrooki/council-notes) 维护，不随代码公开。
