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

更完整的用法（加成员、对话模式、`@` 点名）见 [docs/README.md](docs/README.md)。

## 文档

- [docs/README.md](docs/README.md)：设计文档索引、路线图、用法。
- [docs/00_DECISIONS.md](docs/00_DECISIONS.md)：所有决策和它们的理由。
- [docs/14_MVP_CHECKLIST.md](docs/14_MVP_CHECKLIST.md)：真机验收清单。
- `.handoff/HANDOFF.md`：开发交接记录，接手先读最后一条。
