# Step 8 第二片 冻结契约：WORKSPACE 档按实测的写入边界放行

日期：2026-09-21 · 机器：E 盘机

配套 `11_STEP8_SPEC.md` 第 3 节。前置未决 **#18 已落地**（`621bada`），
所以这一片改 flag 时指纹会跟着动 —— 那正是 #18 存在的理由，别再把它分开改。

本文件是实现方与测试方**共同的唯一权威**。改本文件要留记录。

## 0. 本片改动的包

| 包 | 改什么 |
|---|---|
| `internal/provider` | `Capabilities` 加 `WriteBoundary`；claude / agy 的 `Permissions` 加 workspace；`validateRequest` 加写入边界闸 |
| `internal/turn` | `TranslatePermission` 不再硬拒 workspace；工作目录按档位分；`RulesReport` 按 (provider, tier) 分组 |
| `internal/app` | 去掉启动时一次建齐工作目录的循环 |

## 1. 先说一条实测，它改掉了原规格里的前置

`11` 第 3.2 节原本写着「工作目录一律改成 `<dataDir>/workdir/<convID>/<agentID>`」。
**实测推翻了它**（原件 `E:\Isolating_test\council\oq-cwd-prefix\RESULTS.md`）：
其余参数逐字节相同、只换 `--cwd`，

| 轮次 | cwd | input_tokens | cache_read |
|---|---|---|---|
| run1 | alpha | 18945 | 128 |
| run2 | alpha | **1** | **19072** |
| run3 | **beta** | 18945 | 128 |

run2 是对照组（未决 #1 测过前缀按内容命中），它命中；run3 只换 cwd 就打回冷启动。
**工作目录进模型可见的前缀。** 一律隔离 = 5 个成员 3 条会话付 15 次入场费而不是 5 次。

**所以按档位分**（见第 4 节）。附带好处：READ 的路径不变，
**这一片落地时既有会话一条都不会漂移**。

## 2. `Capabilities.WriteBoundary`

```go
// WriteBoundary 说的是「这家能不能证明自己把文件写限制在指定目录内」。
//
// 这一位不是「有没有权限 flag」，是「那道 flag 实测围不围得住」 ——
// 06 第 2 节那张表被推翻过一次就是因为混淆了这两件事：flag 写上了，
// 有效权限却是「flag 与用户配置文件的合集」，READ 档照样建得出文件。
//
// 零值是 SupportUnknown，所以一个新适配器若在 Permissions 里列了 workspace
// 却没声明这一位，会被 validateRequest 拒掉 —— 漏填的后果是「未实测」
// 而不是「支持」。这是有意的。
WriteBoundary Support
```

**三家的取值与实测出处**（写进各自的注释，数字与措辞不许改）：

| provider | 取值 | 实测 |
|---|---|---|
| claude | `SupportYes` | `--restricted` 明写 confines file tools to the working directories；实测目录内写得进、兄弟目录 `..\outside` 被拒，它自述 file tools are restricted to the current working directory；拒绝记进输出的 `permission_denials` |
| agy | `SupportYes` | `--mode accept-edits` + `--add-dir <workdir>`，界外被拒，拒绝记在 `denied_actions`。**Council 现在解析这个字段**（第二片，见第 7 节） |

**订正（2026-09-21，实测）**：本表原先把 agy 的边界记成「`--add-dir` 之外被拒」，
**归因错了**。`--add-dir` 自己不是边界，它只划出「哪里算界内」；
起作用的是**权限模式**。原件 `E:\Isolating_test\council\oq-agy-denied-count\RESULTS.md`：
同一个 `--add-dir` 配上 `--dangerously-skip-permissions`（那是 **FULL** 档的映射，
`agy.go:173`）时毫无约束力 —— 两个越界文件真的落盘了，内容就是探针写的 `LEAK1` / `LEAK2`，
而 `denied_actions` 这个键**整个缺席**。

这与 `06` 第 2 节那张表被推翻的那一次是同一类错误：把边界归因给一个看起来像边界的 flag。
本表所有「实测」栏的措辞都要点名**是哪个 flag 在起作用**，不是列出当时传了哪些 flag。
| grok | `SupportNo` | `--deny` 是**工具级**黑名单，路径模式（`Write(../**)`）实测**静默不生效**，同一轮里它把兄弟目录的文件照样写出来了；`--sandbox` 的取值集合查不出来（非法 profile 名被静默接受） |

## 3. 放行规则

### 3.1 `Permissions` 列表

- claude：`{PermissionRead, PermissionWorkspace}`
- agy：`{PermissionRead, PermissionWorkspace}`
- grok：`{PermissionRead}` —— **不列 workspace**，它证明不了边界

三家都**不列** `PermissionFull`。

### 3.2 闸放在 `provider.validateRequest`，不放在编排层

```
tier 属于 {WORKSPACE, FULL} 且 caps.WriteBoundary != SupportYes  ->  拒绝
```

**为什么放在 provider 包**：那里是结构性的，加第四家适配器时不可能绕过去；
而且这是一句关于**适配器自身契约**的话（「不许接受一个你围不住的写档」），
不是编排策略。放编排层的话，将来谁新写一条调用路径就绕开了。

错误文本要**区分两种「不行」**，别混成一句：

- **证明不了边界**（grok）：`permission tier %q: no verified write boundary`
- **本家没有这个档**（`Permissions` 没列）：沿用已有的 `ErrUnsupportedTier` 措辞

### 3.3 `turn.TranslatePermission`

不再硬拒 `workspace`。**仍然硬拒 `full`** —— 那是 `06` 第 2 节的编排策略
（「FULL 档在 MVP 里不提供给编排器自动使用」），不是 provider 能力问题，
所以留在编排层，与 3.2 那道闸并存。两道闸拦的不是同一件事。

哨兵 `ErrTierNotInStep6` 的名字已经不准（现在放行 workspace 了）。
改名为 `ErrTierNotOffered`，并在 `09a` 留订正记录。

## 4. 工作目录按档位分（这是前置，顺序写死）

```go
// agentWorkDir 是工作目录路径的**唯一**出处。Ask 与 identityFPFor 都必须调它。
//
// 两处各拼一次是 #18 的实现方指出过的结构缺口：那时两处都是
// filepath.Join(base, agentID)，一致靠的是注释不是结构。现在路径还要看档位，
// 两份拷贝分叉的后果会从「审计报错一行」升级成「审计与真实请求用了不同的目录」。
//
// READ 档不带 convID：实测换一个工作目录就是一次完整入场费
// （oq-cwd-prefix 探针），而只读成员不写盘，共用省下那笔钱。
// claude 会往那里写 council-role.txt，但同一个 agent 的身份块与规则不随会话变，
// 内容逐字节相同 —— spawn.go 里那条「每轮覆盖同一个路径」的注释已经说过这件事。
//
// WORKSPACE 及以上带 convID：它们真会写盘，两条会话共用一个目录就是互相盖文件。
// 代价就是那次冷启动，但只有真会写盘的成员才付。
func agentWorkDir(base, convID, agentID string, tier provider.PermissionTier) string
```

**目录创建改成每轮起进程之前 `MkdirAll`**，`internal/app` 里启动时
一次建齐的那个循环整段删掉 —— 有了会话维度之后它建不全。
建不出来按 refuse 处理（起进程之前失败）。

## 5. `identityFPFor` 要拿到 convID

工作目录进前缀（第 1 节），而 WORKSPACE 的工作目录含 convID，
所以指纹计算必须知道 convID。签名加一个参数，`AuditDrift` 从库里那一行取
（`ProviderSession.ConvID` 现成）。

**这一节有一个必须钉住的后果**：同一个 agent 配成 READ 与配成 WORKSPACE 时，
工作目录不同 -> 前缀摘要不同 -> 改档位会触发一次漂移。
**那是对的**（前缀真的变了，第 1 节实测），用例要正面断言它，
别当成误报去「修」。

## 6. `/rules` 的余量探针按 (provider, tier) 分组

今天 `RulesReport` 是「每家 provider 取 agent id 字典序最小的那个 agent」。
开了 WORKSPACE 之后同一家下会有两档，而**两档的 flag 段不一样长**
（claude 的 WORKSPACE 多一个 `acceptEdits`）；更难看的是 grok
**拒绝 WORKSPACE**，字典序撞上的话那一行会印「不适用」，
把同一家里其实好好的 READ agent 一起盖掉。

改成按 `(provider, tier)` 分组，输出多一列档位。

## 7. `denied_actions`：原记成缺口，真机实测证明是阻塞，第二片已修

本节原文把「Council 不解析 agy 的 `denied_actions`」记成「已知缺口，本片
不修」。2026-09-21 的真机验收推翻了这个判断：agy 配成 WORKSPACE 跑一轮，
屏幕上是「本轮为空」，烧掉 2813 个 output token，一个文件都没建（其中包括
本该允许的那个）。直接调 agy CLI 复现，定位到三层机制（原件
`E:\Isolating_test\council\oq-agy-ws\RESULTS.md`，本次复核原件
`E:\Isolating_test\council\oq-agy-ws\rerun\raw.json`）：

1. **agy 在 WORKSPACE 下确实能写界内**（只做一件界内的事时正常建文件并回
   `DONE`，能力声明本身是对的）。
2. **越界写被自动拒绝**（headless 没人可问），边界是真的，**证据在
   `denied_actions` 字段里，不在正文里**：

   ```
   response: []
   status:   SUCCESS
   denied_actions: [{"action":"write_file","display_name":"WriteToFile"}]
   ```

3. **一次自动拒绝会把整轮的正文清空**，而 `status` 仍是 `SUCCESS`；
   界内已经做完的事照样做了，只是模型的汇报没了。

不解析这个字段，Council 对这一轮只能报「本轮为空」——而实际发生的是
「越界被拒、界内可能已经写了一半」。一个 WORKSPACE 成员到底写没写文件，
用户从屏幕上完全看不出来，而写文件正是这个档存在的全部意义。这与未决
#17 记的同形陷阱是同一个东西的两种触发方式（那次是 READ 档下写被拒），
表现完全一样：`status=SUCCESS` 加空正文。**不解析它，契约 8.2 第 9 条
验收永远过不了**——这就是把它从「缺口」改判成「阻塞」的理由。

**第二片已修**：

- `internal/provider/turn.go` 新增 `DeniedAction{Action, DisplayName}` 与
  `TurnResult.DeniedActions []DeniedAction` / `DeniedActionsRaw
  json.RawMessage`（后者原样保留顶层键的字节，用法与 `UsageRaw` 同一个
  理由）。这是一个通用形状，但今天只有 agy 会填：claude 的
  `permission_denials`（`claude.go` 的 `claudePermissionArgs` 注释与本次
  任务书都说实测过它非空）元素形状没有留下可归档的原始字节（本机每一份
  归档进 fixtures_test.go 的 claude 原件里这个键都是空数组 `[]`），本片
  不猜它的结构；grok 没有对应字段。
- `internal/provider/agy.go` 的 `ParseTurn` 解析 `denied_actions`，实测
  形状是**数组**（哪怕只有一个元素）——PowerShell 管道对单元素数组的
  显示折叠会让它看起来像单个对象，那不是 agy 真实吐出的字节，Council
  直接读子进程 stdout 不经过这层折叠。
- `internal/turn/record.go` 的 `Record` 新增 `DeniedActions` 字段，
  `runner.go` 的 `absorbParse`/`agentMeta`/`attachSessionMeta` 把它透传
  进 `messages.meta["denied_actions"]`（只追加的事实源，重启后 `/log`
  仍能看到），**不改变 Outcome**——被拒不等于这一轮失败，界内的事可能
  已经做完，判成失败是另一种谎。
- `internal/repl/ask.go` 在屏幕上响亮地说出来：「有 N 个工具调用被拒，
  分别是 ...（界内的事可能已经做完，只是正文被清空了，不代表整轮
  失败）」，与 `rec.Message` 是否为空无关，`turn_failed` / `post_rejected`
  路径同样会显示。

**8.2 第 9 条的验收判据不变**：被拒的那一次仍然不能只看正文为空，
仍然要同时核对目标文件确实不存在——`denied_actions` 现在提供的是
「为什么正文是空的」这条解释，不是替代文件系统核验的第二重证据。

### 7.1 `denied_actions` 的长度是下界，不是清点

**这条必须写清楚，否则那个数字会因为反复出现而被当成实测过的清点。**

实测（原件 `E:\Isolating_test\council\oq-agy-denied-count\RESULTS.md`）：
一条指令要求做 **1 件界内 + 2 件越界**，WORKSPACE 档跑完之后

- 界内 `INSIDE.txt` 建了；
- 两个越界文件**一个都没落盘**（边界守住）；
- 而 `denied_actions` **只有 1 条**。

两种机制都与观测相符，本次探针分辨不出来：(a) agy 在第一次被拒时就停下这一轮；
(b) agy 按 `action` 类型去重，两次 `write_file` 折成一条。
要分辨得用两种不同类型的越界动作，**本片不做**。

**可以写进文档与屏幕的只有一句**：那个长度是 provider 报的条数，
是**下界**，不代表这一轮意图做多少件越界的事。
`deniedActionsNote` 的措辞按这条限定，不许读成完整清点。

## 8. 验收

### 8.1 单测

1. **`WriteBoundary` 三家的取值有 conformance 测试钉住**，形状照
   `CleanInstructionEnv` / `CacheAcrossTurns` 那两位：遍历 `provider.IDs()`，
   **新增 provider 没声明这一位时 `t.Fatalf`**。
2. **零值 fail-closed**：构造一个 `Permissions` 含 workspace 而
   `WriteBoundary` 为零值的假 `Capabilities`，`validateRequest` 必须拒。
3. **grok 请求 workspace 被拒，且错误文本说的是「证明不了边界」**，
   不是「本家没这个档」——两种「不行」不许混。
4. **FULL 对三家一律拒**。
5. **`agentWorkDir` 两档各一例**，并断言 READ 那一档的路径
   **与本片改动之前逐字节相同**（这是「既有会话不漂移」的直接断言）。
6. **改档位触发漂移**：同一个 agent 从 read 改成 workspace，指纹必须不同，
   且 `identityDriftError` 点名的是「命令行」而不是别的字段。
7. **#18 那条一直跑不了的 conformance 现在能跑了**：
   「换权限档摘要必须不同」—— 之前三家都只声明 read 一档，
   `validateRequest` 在更早处就拒了，实现方当时用 `t.Logf` 打印原因跳过。
   **现在 claude 与 agy 有两档，这条必须真的跑起来并通过。**

### 8.2 真机（判据是行为，不是退出码、不是模型自述）

8. **claude WORKSPACE**：工作目录内建一个文件 -> 成功；
   往兄弟目录 `..\outside` 建 -> 被拒，且 `permission_denials` 非空。
9. **agy WORKSPACE**：同上。**但被拒那一次不能只看正文为空**（第 7 节），
   必须同时核对目标文件确实不存在。
10. **grok WORKSPACE**：Council 在**起进程之前**拒绝，错误文本说「证明不了边界」。
11. **READ 档的既有会话不漂移**：改动落地后用一个已有的库启动，
    启动漂移审计应当报 0 条。

## 9. 符号禁令

代码文件里禁用装饰性符号：emoji（U+1F300–U+1FAFF）、dingbats（U+2700–U+27BF）、
杂项符号（U+2600–U+26FF、U+2B00–U+2BFF）、带圈数字（U+2460–U+24FF）、
U+FE0F、U+200D。注释、字符串字面量、变量名都算。
放行中文、希腊字母、数学算符、普通箭头 U+2190–U+21FF。
