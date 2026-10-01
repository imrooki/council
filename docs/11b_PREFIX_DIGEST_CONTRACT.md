# 冻结契约：身份指纹的第七段（未决 #18）

日期：2026-09-21 · 机器：E 盘机

配套 `00_DECISIONS.md` 未决问题 #18 的裁决。**这一条阻塞 Step 8 第二片**：
那一片要改 WORKSPACE 的 flag，不先补上这一段就是重犯同一个错。

本文件是实现方与测试方**共同的唯一权威**。改本文件要留记录。

## 0. 要解决的是什么

今天的指纹覆盖的全是「**用户配置**决定的东西」（role / model / effort / tier /
两段规则摘要）。而 `6530839` 那次提交把 claude 的 READ 档从一个 flag 改成四个、
给 grok 加了三条 `--deny` —— **发出去的 argv 真变了，指纹一个字没动**，
因为 `fpFieldTier` 记的是档名 `read`，改前改后都是 `read`。

复核证实：`git show --stat 6530839 -- internal/turn/session.go` **输出为空**。

所以指纹的定义域从一开始就只覆盖了一半：**「适配器代码决定的东西」没人看着**。
它的变化发生在编译期，运行时没有任何一层会发现。

## 1. 设计的关键洞察：指纹从不跨行比

这一条决定了整个方案的复杂度，先说清楚：

**`identity_fp` 只与「同一个 `(conv_id, agent_id)` 上一次记下的值」比较**
（`runner.go` 里进程内那支、从库读回那支，都是按 `sessionKey` 取的）。
**它从不与别的会话、别的成员、别的机器的值比。**

推论：一个值只要**在同一条会话的各轮之间稳定**，就可以放进摘要，
哪怕它在不同会话之间不同、哪怕它跟前缀其实没关系 —— 那不会造成任何误报。

于是「要归一化掉什么」这个清单缩到极小：**只有在同一条会话内逐轮变化的两样**。

| 要归一化 | 为什么 |
|---|---|
| provider 会话 id | grok 每次 fresh 都现铸一个 UUID；不归一化则每轮摘要都不同 |
| 正文（prompt） | 每轮都不一样 |

**其余一律原样进摘要**，包括工作目录路径、`--cwd`、`--add-dir`、身份文件路径。
它们在同一个 `(conv, agent)` 上是稳定的，进来零代价，而漏掉它们就是继续瞎。

## 2. 规范化探针：固定用 fresh 形状

**指纹必须在一条会话的整个生命期里稳定**：首轮算一次、之后每轮拿来比。
而 fresh 与 resume 两种形状的 argv 天然不同（`--session-id` vs `--resume`），
所以**不能按当轮的实际形状算** —— 那会让每一条会话的第二轮都漂移。

**固定用 fresh 形状**（`ProviderSession == ""`）。理由有二：
它是 argv 最长的那一支；而且 **agy 的身份拼接只在这一支**
（`agy.go` 里 `req.ProviderSession == ""` 时才把 Role 拼进正文最前面），
用 resume 形状算的话，agy 的前缀结构永远进不了摘要 ——
**而那正是本条要防的那类走失**。

正文传一个**固定哨兵串**，不传空串：agy 会把 Role 与正文拼在一起，
传空串会让拼接的形状（分隔符、顺序）观察不到。

## 3. 契约

### 3.1 新函数

```go
// prefixProbePrompt 是算前缀摘要时喂给 BuildTurn 的固定正文。
// 不能是空串：agy 把身份块拼在正文最前面，空正文会让拼接的形状观察不到。
const prefixProbePrompt = "<council-prefix-probe>"

// prefixProbeSessionID 是归一化占位符。
const prefixProbeSessionID = "<council-session-id>"

// PrefixArgsDigest 摘要「这一轮会发出去的命令行里，与缓存前缀有关的那部分」。
//
// 做法是用一个**规范化的探针请求**现组一次命令行再摘要，而不是让每个适配器
// 自己声明哪些参数与前缀有关 —— 后者要求三个（将来更多）适配器各自维护一份
// 分类清单，而分类错了不会有任何一层报错。探针法把分类问题整个取消掉：
// 见第 1 节，指纹从不跨行比，所以只需要归一化「同一条会话内逐轮变化」的两样。
//
// 规范化的两样：
//   - ProviderSession 固定为空串（fresh 形状，理由见第 2 节）
//   - Prompt 固定为 prefixProbePrompt
//   - 组出来之后，argv 里等于 cmd.PlannedSessionID 的元素换成 prefixProbeSessionID
//     （grok 在 fresh 支现铸 UUID；这一步不需要任何适配器专有知识）
//
// 返回 "<参数个数>/<sha256 前 12 位小写十六进制>"，例如 "14/9c1b7e0a2f44"。
// 参数个数放在前面是给人看的：flag 增减时这个数会变，一眼能看出方向。
//
// **失败必须返回 error，不许吞**。BuildTurn 会因为权限档不放行、
// 将来新加的校验而失败，吞掉错误就是静默降级。
func PrefixArgsDigest(p provider.Provider, req provider.TurnRequest) (string, error)
```

### 3.2 `identityFPFor` 改成返回 error，并自己组规范化请求

```go
// identityFPFor 是 Ask 与 AuditDrift 唯一的指纹计算入口。
// 它自己组那个规范化请求 —— 两处各组一份的话，任何一处漏改都会让审计
// 报出不存在的漂移或漏报真实的漂移，而这种不一致不会在编译期或运行时报错。
//
// baseWorkDir 是 Runner 的工作目录**基**目录，agent 目录由本函数自己拼，
// 理由同上：拼法只许有一处。
func identityFPFor(
	p provider.Provider,
	agentID string,
	ac config.AgentConfig,
	baseWorkDir string,
	tier provider.PermissionTier,
	rules RulesBlocks,
) (string, error)
```

**两个调用方的处置**：

- `Ask`：出错走 `r.refuse(...)`，与其它起进程之前的拒绝同一条路。
- `AuditDrift`：出错**不让整批审计失败**，落成那一行的 `Reason`
  （形如 `算不出指纹: <原因>`），与已有的「provider 不可用」「权限档不放行」同体例。

### 3.3 指纹六段变七段，并把位置解码换成结构体

```go
const (
	fpFieldRole        = 0
	fpFieldModel       = 1
	fpFieldEffort      = 2
	fpFieldTier        = 3
	fpFieldRulesGlobal = 4
	fpFieldRulesLocal  = 5
	fpFieldPrefixArgs  = 6
	fpFieldCount       = 7
)

func IdentityFingerprint(
	role, model, effortFlagEmitted string,
	tier provider.PermissionTier,
	rules RulesBlocks,
	prefixArgsDigest string,
) string
```

**`fingerprintFields` 改成返回结构体，不再返回一串位置参数**：

```go
// fingerprintParts 是解码后的指纹。用结构体而不是一串返回值：
// 六段时已经是 7 个返回值，七段就是 8 个，位置传错编译器拦不住，
// 而传错的后果是 drift 报错点名了错的字段 —— 一种看起来正常的谎。
type fingerprintParts struct {
	Role        string
	Model       string
	Effort      string
	Tier        string
	RulesGlobal string
	RulesLocal  string
	PrefixArgs  string
}

func parseFingerprint(fp string) (fingerprintParts, bool)
```

`identityDriftFields` 改成**遍历一张 (标签, 旧值, 新值) 的表**，
不再逐字段写 if。加第八段时是往表里加一行，不是在三个地方各改一处。
标签沿用给用户看的措辞：`role` / `model` / `effort` / `permission` /
`全局规则` / `局部规则` / **`命令行`**。

### 3.4 不做迁移，改成让作废变响亮

库里现存的是**六段**指纹，`parseFingerprint` 会判 `ok=false`。
上一次（六段那回）配了 schema v3 的纯追加迁移，**这一次不配**，因为追加一个
空段对不上：新算出来的第七段是一个真实摘要，不是空串。

**所以升级之后既有的 provider 会话会全部重建一次。** 这是**对的**：
那些会话建立时我们根本没在看 argv，现在也无从证明它们的前缀没变过。
本机受害者仍是隔离根冒烟库里的那几条，`E:\council-data` 与
`~/.council/config.toml` 都不存在（复核再次确认过）。

**但「静默重建」这件事本身要修掉**，这是本条附带的真正改进：

`runner.go` 里从库读回那一支，指纹不符时**静默作废重建**。
改成**在作废的那一刻响亮地说出来**：那时旧指纹还在手上，
`identityDriftFields` 现成可用，能点名到底哪一项变了。
解不开的旧格式指纹就说「指纹格式已升级」。

写在屏幕上，形如：

```
作废重建 <agentID> 的 provider 会话：命令行 "12/3f2a1b9c4d5e" -> "15/9c1b7e0a2f44"（下一轮按首轮计价）
```

**这不只是为这次升级**：从库读回时的静默重建今天就存在，
只是除了改 model / effort 之外几乎没什么会触发它。

## 4. 验收

### 4.1 conformance 测试（判据是行为，不是声明）

对**每一个已注册的 provider** 各跑一遍，这几条一起构成「摘要真的在量前缀」：

1. **同一档、只换会话 id 与正文，摘要必须相同**
   （否则每轮都会误报漂移）。
2. **换权限档，摘要必须不同**（这是 `6530839` 那次该红而没红的情形）。
3. **换 model，摘要必须不同**。
4. **对同一份输入连算两次，结果必须相同**（grok 每次铸新 UUID，
   这一条直接钉住归一化真的生效了）。
5. **新增 provider 时这条测试必须逼人表态**：遍历 `provider.IDs()`，
   不是写死三个 id。

### 4.2 golden 钉 argv 列表本身，不是钉摘要

摘要是个哈希，变了看不出哪里变了。golden 存**规范化之后的 argv 列表**
（三家各一份），摘要由它派生。于是将来谁改了一个 flag，
用例红的时候屏幕上直接是那一行 diff。

### 4.3 回归

`go build` / `go vet` / `go test -count=1 ./...` 三个全绿。
既有用例里凡是手工造六段指纹的，都要跟着改成七段
（上一次六段那回踩过这个：手写段数的夹具会在段数变化时整体判 `!ok`，
用例回退成笼统报错，验不到它想验的东西）。

## 5. 一并做掉：未决 #19

同一件事（起进程之前拒绝），grok 记 `spawn_failed`、agy 记 `refused`，
因为命令行总长闸住在 `spawnProcess` 里、`MaxPromptBytes` 闸住在 `BuildTurn` 里。
`spawn_failed` 字面读作「试着起了进程但失败了」，而那里一个进程都没起。

**改法**：让 Runner 把 `ErrCommandLineTooLong` 归到 `refused`。
配一条用例钉住：那道闸触发时 `Record.Outcome` 是 `refused` 且 `wall_ms` 为 0。

## 6. 符号禁令

代码文件里禁用装饰性符号：emoji（U+1F300–U+1FAFF）、dingbats（U+2700–U+27BF）、
杂项符号（U+2600–U+26FF、U+2B00–U+2BFF）、带圈数字（U+2460–U+24FF）、
U+FE0F、U+200D。注释、字符串字面量、变量名都算。
放行中文、希腊字母、数学算符、普通箭头 U+2190–U+21FF。
