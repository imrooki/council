# Council 文档

<!-- public-docs-readme -->

本目录只保留公开的代码结构与构建说明。决策记录、实施契约、真机验收记录与交接笔记属于私有资料，已移到私有仓库：

<https://github.com/imrooki/council-notes>

## 代码结构

- `cmd/council/`：CLI 入口与子命令。
- `internal/app/`：启动、配置加载、TUI 主循环的装配。
- `internal/repl/`：交互式 REPL、命令解析、会话与成员管理。
- `internal/tui/`：终端界面渲染、复制模式与输入处理。
- `internal/provider/`：本机 CLI 适配器（claude / codex / grok / agy）与模型发现。
- `internal/turn/`：一轮执行的调度、预算、超时与结果归一化。
- `internal/store/`：SQLite 持久化与 schema 迁移。
- `internal/serve/`：本地 Web 看板与写模式 API。
- `internal/config/`：TOML 配置、项目层合并与 Claude 资产导入。
- `internal/headless/`：`council ask` 的无界面执行路径。
- `desktop/`：Electron 桌面外壳。
- `contrib/dsh-subagent-council/`：dsh 插件。
- `scripts/`：构建与打包辅助脚本。

## 构建与测试

需要 Go 1.25 或更新版本：

```sh
go build ./...
go test ./... -count=1
```

构建可执行文件：

```sh
go build -o council.exe ./cmd/council
```

桌面外壳：

```sh
cd desktop
npm install
npm start
```

## 文档边界

公开仓库只保留代码、构建说明与用法。所有设计裁决、实施契约、验收记录和交接笔记都在私有仓库维护，不随代码公开。
