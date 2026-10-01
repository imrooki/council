# many-ai-cli 审计报告（2026-09-20）

仓库：https://github.com/ishizakahiroshi/many-ai-cli · Go 1.25 · MIT · v0.8.0 · 9 stars / 3 forks
方法：完整克隆后源码精读。8 个子系统并行深读，每个子系统配一轮对抗验证（专门去证伪），最后一轮完整性批评。
共 17 个 agent、1050 次工具调用。验证结果：187 条 CONFIRMED、55 条 OVERSTATED、7 条 REFUTED。
批评轮收到的摘要被长度截断（只覆盖 2/8 子系统），其结论均由它自行回源核实；下列关键门控与默认值由我本人逐条复核。

## 1. 它是什么

不是看板，是本地控制面。把 7 个 AI 编码 CLI（claude / codex / copilot / cursor-agent / grok / opencode / command-code）
各自包进 PTY，用一个 127.0.0.1:47777 的 Hub 收拢：实时终端、统一审批、文件浏览编辑、git 提交、语音输入、手机 PWA 推送。
Gemini CLI 与 Antigravity 故意排除（README 引 Google 2026-02 因第三方工具借 Gemini CLI OAuth 封号一事）。

## 2. 核心机制（源码确证）

审批检测是屏幕刮取，不是协议：
- 自研 VT 模拟器 internal/hub/vt_buffer.go（含光标寻址、500 行回滚环、对未终止 CSI/OSC 的 DoS 上限）
- 对渲染后尾部 120 行取 90 行，跑 5 条正则加 OpenCode 的子串搜索（approval_detector.go:64-78）
- 答案以字面按键注入 PTY：数字加回车、单字母快捷键、Tab、Esc、OpenCode 的方向键序列

第二条协作通道（仅在审批按钮开启时）：往 ~/.claude/CLAUDE.md 追加一行 import，往 AGENTS.md 注入标记块，
指示 AI 用 [MANY-AI-CLI] 文本块代替原生 TUI 选择器，并明令禁止 Claude 调用 AskUserQuestion。

## 3. 对 Griffin 工作流的直接影响

1. 审批按钮默认关闭（config.go:190 ApprovalConfig.Enabled 是零值 false，另有 FirstLaunchShown 字段供首启询问）。
   一旦开启，Hub 会改 ~/.claude/CLAUDE.md 与 ~/.codex/AGENTS.md；被强杀则残留，
   作者注释记录该残留曾被 commit 进两个公开仓库（approval_rules_state.go:15）。与 OneDrive 双向同步纪律直接冲突。
2. token 状态栏默认开启（config.go:593，Enabled 为 nil 或 true 皆算开）。因此 Hub token 明文写进
   ~/.codex/config.toml 的 Stop hook 块是默认行为（usage_hooks.go:282），且该文件不在 Windows DACL 加固覆盖范围内。
   Claude 侧不同，走 %TEMP% 下 O_EXCL 0600 的每会话临时 JSON，不碰共享文件。
3. 多订阅 profile 会整份复制 ~/.codex/config.toml；作者注释承认 [mcp_servers.*.env] 里的 key 随之复制
   （codex.go:77-80），但 auth.json 从不复制（codex.go:85）。实质风险低，只是包文档那句「从不读写凭据」表述过宽。
4. 不支持 agy / Antigravity，三路合议缺一腿。
5. 形状不同：现有 ask-* 子代理是 headless（codex exec 加结果文件），many-ai-cli 是交互式 TUI 加人在环审批。
   互补，非替代。
6. 会读 ~/.claude/projects 下的 journal.jsonl 与 Workflow task output 文件（只解 type / agentId / workflowProgress，可关）。

## 4. 风险（对抗验证后仍成立）

- Hub token 是唯一边界，等价于以当前用户身份任意执行代码；token 被刻意放进 URL（banner、QR、Tailscale 广告），
  会进浏览器历史、会话恢复、截图。POST /api/spawn 接受 provider 为 shell，直接给交互式 pwsh。
- 每个 provider 子进程的环境里都有 MANY_AI_CLI_HUB_TOKEN（wrapper.go:1159-1161），无条件。
  作者在上方写了 14 行注释明确接受该后果：持 token 的会话能叩任何只靠 token guard 保护的 Hub API，
  理由是「所有会话跑在同一用户同一机器上，不给 token 不构成 OS 级防御」，定性为护栏而非对恶意的边界。
  该论证对「已能执行代码的攻击者」成立；对提示注入场景只部分成立，因为此时 token 就在 env 里唾手可得。
- SQLite 存审批问题、原始审批块、用户答案，未脱敏。且保留期清理有一条失效链：会话日志默认关闭，
  <log_dir>/sessions 目录因而可能不存在，os.ReadDir 失败即在 maintenance.go:98-101 早返回，
  而 PruneOlderThan 写在该返回之后（:104），于是聊天库不被清理。
- relay 引擎（5475 LOC）无人值守跑 实现到审查到修复 的循环，建分支与 worktree，往 .git/info/exclude 追加行。
  编排子会话默认全权限绕过（codex 用 danger-full-access 加 ask-for-approval never）。
- 用户可调的 approval-pattern 只驱动浏览器端 fallback 检测器：调完网页动作条会生效，
  但推送通知、一键审批、自动审批仍然瞎（它们走 Go 主检测器的另一套硬编码提示词），且无任何代码或注释说明这一点。
- 网页点按钮的主路径绕过全部防竞态保护，CLI 若已移动光标则按键落到别处。属误触风险，非提权
  （能点按钮的人本来就能 spawn shell）。
- Files API 的机密文件拒绝表漏了 auth.json：~/.codex/auth.json、~/.grok/auth.json 可经 loopback 路径读出，
  而这正是本仓自己称为「the credential」的文件。边际风险低（调用者已持 token），但是个确凿缺口。
- 无 Authenticode 签名；npm 路径的注释自述是为了不带 MOTW、不触发 SmartScreen。
- setup 会装登录自启；每次 Hub 启动无条件从 raw.githubusercontent.com 拉 pattern。
- main 分支既非发布指针也非 tip；bus factor 1。

## 5. 优点（经确认，非 README 自述）

- instrumentation.json：9 条观测代码台账，全部标记 removed，附移除 commit 与事后分析，CI 两处强制加产物扫描。
  起因是三次同型事故（审批正文被记录、未脱敏击键以 0644 无限持久化、调试端点）。
- loopback 绑定是结构性的（地址字面量编译进 http.Server 与 listener，无 bind flag）。
- WebSocket 同时校验 Host 与 Origin，拒绝无 origin 的远程样连接；DNS rebinding 防御覆盖 GET；
  SSRF guard 的 TOCTOU 已闭合（解析后直连已验证 IP）。
- internal/securefile 重建 Windows DACL（作者知道 chmod 在 NTFS 上只切 READONLY 属性）。
- Whisper 下载是全仓加固最好的路径：SHA256 固定、拒非 https、basename 白名单解压、0700、JobObject。
- npm 包无 postinstall；git 调用全 argv 直传无 shell，revision 正则拒前导 dash，路径在双横线之后。
- 自动审批四道闸且只放行 Low 风险；AddRule 拒绝从多命令审批学规则，注释点名 prompt injection。
- 测试文件以过去的审计发现命名（audit_ssh_arginjection_test.go 等）；三平台 CI 加 pinned govulncheck。

## 6. 结论

工程质量显著高于 9 stars 的表象，是持续防御性工程的产物：大部分缺陷是枚举漏了一项，而非根本没有边界。
README 的坦诚也超出常见水平，自己写明「能连上 Hub 就等于在该主机上任意执行命令」。
风险不在于恶意或粗心，而在于它很强、装持久化、每次启动连一个已知主机、把全部权限交给一个印在 URL 里的 token。

建议：可读、可借鉴（VT 模拟器、instrumentation 台账、securefile 的 Windows DACL 处理值得抄）。
若要实跑，三件事：审批按钮保持关闭，或接受它改你的 CLAUDE.md 并纳入同步纪律；
确认 ~/.codex/config.toml 不进 OneDrive 同步（token 状态栏默认开）；第一次在隔离测试根而非真实项目目录里跑。
