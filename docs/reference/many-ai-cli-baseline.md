# many-ai-cli 基线验证报告（Step 1 部分）

日期：2026-09-20
机器：E 盘机（COMPUTERNAME = LONG）
执行范围：仅基线健康验证（构建 + 测试 + 静态分析）。未切 remote，未建分支，未改任何代码。

## A. BASELINE STATUS

**PASS WITH WARNINGS**

唯一的测试失败已定性为上游测试在本机环境下的缺陷，与 Go 版本无关，与我们无关（未做任何修改）。

| 项 | 结果 |
|---|---|
| go mod verify | PASS（all modules verified） |
| bun install | PASS（11 包，11.07s） |
| bun run build | PASS（dist 270 文件） |
| go build ./... | PASS（exit 0，54s） |
| go vet ./... | PASS（exit 0） |
| go test ./... | **21 包 ok / 7 包无测试 / 1 包 FAIL** |
| web 测试（bun run test） | PASS（206/206，0 fail） |
| 工作树洁净度 | PASS（git status 0 条） |

## B. 架构摘要

Go 后端 + TypeScript 前端，单一二进制内嵌前端资源。

- `internal/hub/`（277 文件，约 78k 行）：HTTP/WebSocket 服务、VT 模拟器、审批检测、编排、relay
- `internal/wrapper/`（54 文件，7.8k 行）：PTY 包装，Windows 走 ConPTY（aymanbagabas/go-pty），Unix 走 creack/pty
- `internal/launcher/`、`internal/config/`、`internal/sessionstore/`（SQLite）、`internal/subscription/`（多订阅 profile）
- `internal/orchestrate/`、`internal/handoff/`、`internal/approval/`、`internal/autoapproval/`
- `web/src/`（约 75k 行 TS）：xterm.js 终端、审批动作条、文件/Git 视图
- 7 个内置 provider：claude / codex / copilot / cursor-agent / opencode / grok / command-code
- 已存在一个硬编码的多 agent 引擎：`internal/hub/relay*.go`（5475 行），plan -> implement -> review -> fix

## C. 重要目录/文件

| 路径 | 作用 |
|---|---|
| `internal/hub/relay*.go` | 已有的多 agent 状态机，Council 最相关 |
| `internal/hub/orchestration.go` | 子会话 spawn、board.md、注入 |
| `internal/orchestrate/` | conductor AI 调用的 CLI 子命令 |
| `internal/hub/approval_detector.go` | 审批检测的全部正则表 |
| `internal/hub/vt_buffer.go` | 自研 VT 模拟器 |
| `internal/config/custom_provider.go` | 自定义 provider（仅 4 字段） |
| `internal/wrapper/approval_rules.go` | 往 CLAUDE.md / AGENTS.md 注入规则 |
| `web/web.go` | `//go:embed all:dist` — 构建前置依赖的根源 |
| `CLAUDE.md` + `CLAUDE/*.md` | 仓库自带的开发规约（5 份） |
| `instrumentation.json` | 观测代码台账，CI 强制 |

## D. 构建命令（本机实测可用）

Go 不在任何 shell 的 PATH 上，必须用全路径。

```
# 1) 前端必须先构建，否则 go build 挂在 embed pattern 上
cd D:\DeskTop\My_Project\many-ai-cli\web
bun install
bun run build

# 2) Go 侧
cd D:\DeskTop\My_Project\many-ai-cli
& "C:\Program Files\Go\bin\go.exe" build ./...
```

**不要用 `make build`**：它链到 `deploy-wsl`，本机未配 WSL 会失败。

## E. 测试命令

```
& "C:\Program Files\Go\bin\go.exe" test ./...     # 约 5 分钟
& "C:\Program Files\Go\bin\go.exe" vet ./...
cd web; bun run test                                # 206 个 fixture 测试
```

## F. Council 修改前已存在的问题

### F-1（唯一的测试失败）上游测试缺陷，非管理员 Windows 必然触发

```
--- FAIL: TestSeedLinksRuleFileWhenDefaultIsSymlink (0.01s)
    seed_test.go:188: CLAUDE.md was not seeded; applied=[] failed=[]
FAIL    many-ai-cli/internal/subscription   12.121s
```

**根因链（逐环确证）**

1. `isolateVendorHome`（seed_test.go:16-25）把 HOME/USERPROFILE 指向临时目录
2. 测试在 `$HOME/.claude/CLAUDE.md` 处调 `os.Symlink`（seed_test.go:178）
3. 本机实测：当前账户**不能创建文件 symlink**（"Administrator privilege required"），
   开发者模式未启用（AppModelUnlock 的 AllowDevelopmentWithoutDevLicense 为空），进程非管理员
4. 于是 `linked = false`，**且源文件 `$HOME/.claude/CLAUDE.md` 根本没被创建**
   —— 测试没有在 symlink 失败时退回写一个普通文件
5. `EnsureProfileDir` 找不到源，既不 Applied 也不 Failed，返回空
6. seed_test.go:187 的 `!appliedContains(result, "CLAUDE.md")` 触发 Fatalf
7. 测试第 197-209 行精心准备的 `!linked` 回退分支**永远到不了**

**定性**：上游测试的缺陷，不是产品代码缺陷，不是 Go 版本差异，不是我们的修改。
作者 CI 跑 GitHub `windows-latest`（runner 以管理员身份），因此永远碰不到这条路径。
产品代码本身是处理了这个问题的 —— `internal/subscription/linkdir_windows.go` 正是因为
`os.Symlink` 需要 SeCreateSymbolicLinkPrivilege 才改用 NTFS junction 回退。测试没跟上。

**修法（若要修）**：在 `linked = false` 分支里补一行写普通文件作为源，或直接 `t.Skip`。
一行改动，可回报上游。本次未改。

### F-2 构建前置未文档化

`web.go` 的 `//go:embed all:dist` 依赖 `web/dist`，而该目录在 .gitignore 里、不随仓库分发。
从干净克隆直接 `go build ./...` **必然失败**。Step 1 的提示词没有这一步。

### F-3 版本差异（未触发问题，但需记录）

本机 go1.27.0，go.mod 声明 `go 1.25.13`，CI 用 `go-version-file: go.mod`（即 1.25.13）。
GOTOOLCHAIN=auto 且本机版本更高，故直接用 1.27.0 编译。`go vet` 在更严格的 1.27 下仍然全过。
若日后出现只在本机复现的失败，先怀疑这个差异。

### F-4 环境

GOPATH=`E:\Go\gopath`、GOCACHE=`E:\Go\cache`、GOMODCACHE=`E:\Go\modcache`，均存在。
CGO_ENABLED=0。Go 与 conda 都不在两个 shell 的 PATH 上（会话 PATH 在启动时缓存）。

## G. 基线 commit SHA

```
dbb863cac268b90718d46b481b817de70cfcd9f6
2026-09-11 19:00:56 +0900  ishizakahiroshi
style(web): 将运行中指示器改为旋转 spinner，并移到 #N 之前
```
上一行是该提交说明的中文译文。`git log -1` 的 subject 原文是日文，这里不再抄录。
即 upstream `main` 的 tip。工作树洁净，无本地修改。

## H. Git remote 状态

**当前与 Step 1 的目标状态不符**：

```
origin  https://github.com/ishizakahiroshi/many-ai-cli.git (fetch/push)
```

只有一个 remote，指向 **upstream**。没有 `imrooki` 的 fork。
原因：本仓库是上一轮为做源码审计而克隆的 upstream，不是从 fork 克隆的。

Step 1 期望：origin = imrooki/many-ai-cli，upstream = ishizakahiroshi/many-ai-cli。
**未执行切换**（理由见 J）。

## I. 开发分支状态

当前分支 `main`，**未创建 `dev/council`**。理由见 J。

## J. 是否可安全进入 Step 2

**技术上：可以。** 基线健康，唯一失败已定性且与 Council 无关。
Step 2 是只读架构审计，不需要 remote 或分支就绪。

**但有两条前置建议：**

1. **Step 2 的提示词必须补一句**，点名 `internal/hub/relay*.go`（5475 行）、
   `internal/hub/orchestration.go`、`internal/orchestrate/`，要求先刻画已有的多 agent 引擎。
   不补这句，执行者极可能漏掉全仓与 Council 最相关的部分，而 Step 3 会在不知其存在的前提下做架构设计。
2. **remote 切换与 `dev/council` 分支未做**，因为那是承诺"在 many-ai-cli 上建 Council"这条路线的动作，
   而上一轮评估的结论是先裁决 runtime-vs-UI 的岔路。这两步都廉价可逆，决定后随时可做。
