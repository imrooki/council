package turn

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"council/internal/config"
	"council/internal/provider"
)

// CacheTTL 来自 D-4 实测：claude 三轮的 cache_creation 全部落在
// ephemeral_1h_input_tokens，ephemeral_5m 恒为 0。
//
// 硬性规则（09_STEP6_SPEC.md G 节）：CacheTTL 绝不进入「是否重建会话」的判据，
// 只作为下一轮的价格预报。缓存过期不是会话过期。
const CacheTTL = time.Hour

// sessionKey 是 (会话, agent)。绝不以 provider 为键：两个 agent 即使同一家
// provider 也各有各的流（04 第 0 节规则 2）。
//
// 这是包内私有类型，不是导出契约，但它的字段形状决定了 Runner 状态表的正确性
// 论证是否成立，所以仍然写在这里供实现参考。
type sessionKey struct {
	convID  string
	agentID string
}

// sessionState 是一条 provider 会话在本进程内的全部状态。
//
// 这几个字段同生共死是有意的：会话 id 活在内存里，若把游标落了库，重启之后
// agent 会拿着「我已经读过 seq 1..N」的账本走进一条全空的新会话，而库里没有
// 任何一层看得出不对。Step 7 把它们放进同一个 WriteTx，那时它们才一起获得持久性
// （09_STEP6_SPEC.md F 节、L 节 #7）。同样是包内私有类型，理由同上。
//
// 本步故意不写 cursors 表：Tx.SetCursor 已经存在，顺手落库看起来白赚，
// 但游标一旦落库就活过重启，而 provider 会话只活在这个 map 里。一起丢比丢一半干净。
type sessionState struct {
	providerSession string
	cursor          int64
	costTotal       float64 // claude 的会话累计金额基线
	haveCostTotal   bool
	baselineSuspect bool // 上一轮结局未知，基线可能已经落后
	lastTurnAt      time.Time
	identityFP      string // 建立这条会话时的身份指纹
	spoken          bool   // 这个 agent 在这条会话里已经说过话（决定面包屑的措辞）
}

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

// prefixProbePrompt 是算前缀摘要时喂给 BuildTurn 的固定正文。
//
// 不能是空串：agy 把身份块拼在正文最前面（agy.go 里 ProviderSession == ""
// 那一支），空正文会让拼接的形状（分隔符、顺序）观察不到，等于没探到 agy
// 那条前缀最容易走失的地方。
const prefixProbePrompt = "<council-prefix-probe>"

// prefixProbeSessionID 是归一化占位符，换掉 grok 在 fresh 支现铸的那个 UUID。
// 不这样做的话，同一份配置每次探测都会因为 UUID 不同而摘出不同的指纹，
// 把「配置没变」误报成「前缀变了」。
const prefixProbeSessionID = "<council-session-id>"

// IdentityFingerprint 是「这条 provider 会话的前缀由什么决定」的摘要。
// 前缀不只是对话内容：身份文本、model、权限档（它决定 claude 的工具定义）都在
// 系统提示里，换任一项都让缓存作废，而换个 flag 继续 --resume 恰恰是最容易悄悄
// 干的事。
//
// effortFlagEmitted 是「实际放上命令行的那个值」，不是 AgentConfig.Effort。
// 这个词的选择独立地杜绝了一种情况：EffortProcessExternalConfig 的 provider
// 这一位恒为空串，用户怎么改配置都不触发漂移；而 EffortPerTurnFlag 的改了就该漂移。
//
// 绝不含 ObservedModel：provider 换个型号别名（opus -> claude-fable-5-1）就
// 触发一次重建，那是白烧钱。
//
// role 收的必须是 renderIdentityCore 的产出，**不是**带规则的整块 ——
// 规则以摘要单独成段，role 段里再夹一份全文就把摘要白设了。理由写在
// renderIdentityCore 的注释里。
//
// 规则两段以摘要追加在第 5、6 段，第七段是 PrefixArgsDigest —— 三家适配器
// 实际会发出去的命令行里，与缓存前缀有关的那部分（未决 #18）。**前四个下标
// 保持不变**，schema v3 那次迁移才能是一次纯追加；这一次改的是第七段，
// 库里现存的六段指纹会整体判 !ok，不配纯追加迁移，理由见契约第 3.4 节：
// 追加一个空段对不上，新算出来的第七段是一个真实摘要，不是空串。
// 摘要在本函数内部算：只有这一处算，调用方不可能算错。
func IdentityFingerprint(
	role, model, effortFlagEmitted string,
	tier provider.PermissionTier,
	rules RulesBlocks,
	prefixArgsDigest string,
) string {
	return packFingerprint(role, model, effortFlagEmitted, string(tier),
		RulesDigest(rules.Global), RulesDigest(rules.Local), prefixArgsDigest)
}

// agentWorkDir 是工作目录路径的唯一出处（11c 契约第 4 节）。Ask 与
// identityFPFor 都必须调它，不许各自拼一遍：#18 的实现方已经指出过这个
// 结构缺口——那时两处都是 filepath.Join(base, agentID)，一致靠的是注释
// 不是结构，现在路径还要看档位，两份拷贝分叉的后果会从「审计报错一行」
// 升级成「审计与真实请求用了不同的目录」。
//
// READ 档不带 convID：实测换一个工作目录就是一次完整入场费
// （E:\<tmp-root>\council\oq-cwd-prefix\RESULTS.md，其余参数逐字节
// 相同、只换 --cwd，input_tokens 从 1（命中缓存）弹回 18945），而只读成员
// 不写盘，共用同一个目录省下那次冷启动。claude 会往这个目录写
// council-role.txt，但同一个 agent 的身份块与规则不随会话变，内容逐字节
// 相同（spawn.go 里「每轮覆盖同一个路径」那条注释说的是同一件事）。
//
// WORKSPACE 及以上带 convID：这一档真会写盘，两条会话共用一个目录就是
// 互相盖文件——`06` 第 2 节的写入边界只挡得住「写到目录外」，挡不住
// 「写到另一场会话的东西上」。代价是那次冷启动，但只有真会写盘的成员才付。
func agentWorkDir(base, convID, agentID string, tier provider.PermissionTier) string {
	if tier == provider.PermissionRead {
		return filepath.Join(base, agentID)
	}
	return filepath.Join(base, convID, agentID)
}

// identityFPFor 是 Ask 与 AuditDrift 唯一的指纹计算入口，用 Ask 里那一套
// 完全相同的算法，给 (agentID, ac) 这一对在给定的 p / tier / rules 之下
// 算一遍指纹。
//
// Ask 与 AuditDrift 共用这一个函数，是因为它们回答的是同一个问题
// 「这个 agent 现在应该有一份什么样的指纹」，只是一个在起进程之前问、
// 一个在事后审计时问。两处各写一份的话，任何一处漏改（加字段、改
// effortFlagEmitted 的取法、改探针请求怎么组……）都会让审计报出一堆
// 不存在的漂移，或者反过来漏报真实的漂移，而这种不一致不会在编译期或
// 运行时报错，只会体现成「/rules 审计的数字」与「Ask 实际拒绝与否」对不上。
//
// role 段用 renderIdentityCore 的产出而不是 renderIdentity：规则以摘要
// 单独成段，role 段里再夹一份规则全文就把摘要白设了，理由见
// renderIdentityCore 的注释。
//
// baseWorkDir 是 Runner 的工作目录**基**目录，agent 目录由 agentWorkDir
// 按档位拼（READ 忽略 convID，WORKSPACE 及以上把它拼进路径）——探针请求
// 要用的 WorkDir 与真正发给这个 agent 的 TurnRequest.WorkDir 必须是同一条
// 路径，claude 的 rolePath、grok 的 --cwd、agy 的 --add-dir 都直接把它塞进
// Args，拼法不统一会让指纹与真实发出去的命令行对不上。
//
// convID 是 11c 契约第 5 节新增的参数：WORKSPACE 的工作目录含 convID，
// 指纹要进前缀摘要，所以指纹计算必须知道 convID 才能拼出与真实请求相同的
// WorkDir。调用方：Ask 传当前会话的 convID，AuditDrift 传 ps.ConvID
// （ProviderSession 那一行自带的会话 id）。
//
// 探针失败（权限档不放行、prompt 超长——agy 把 Role 拼进探针正文，超大
// 规则文件会先在这里被 ErrPromptTooLong 挡住、而不是拖到 spawn 才响）时
// 把错误原样交回：调用方各自决定怎么处置，本函数不吞。
func identityFPFor(
	p provider.Provider,
	agentID string,
	ac config.AgentConfig,
	baseWorkDir string,
	convID string,
	tier provider.PermissionTier,
	rules RulesBlocks,
) (string, error) {
	caps := p.Capabilities()
	emitted := effortFlagEmitted(caps.Effort, ac.Effort)
	core := renderIdentityCore(agentID, ac)
	identity := renderIdentity(agentID, ac, rules)

	probeReq := provider.TurnRequest{
		AgentID:     agentID,
		DisplayName: ac.DisplayName,
		Model:       ac.Model,
		Effort:      ac.Effort,
		Role:        identity,
		Permission:  tier,
		WorkDir:     agentWorkDir(baseWorkDir, convID, agentID, tier),
	}
	prefixDigest, err := PrefixArgsDigest(p, probeReq)
	if err != nil {
		return "", fmt.Errorf("prefix args digest for %s: %w", agentID, err)
	}
	return IdentityFingerprint(core, ac.Model, emitted, tier, rules, prefixDigest), nil
}

// PrefixArgsDigest 摘要「这一轮会发出去的命令行里，与缓存前缀有关的那部分」。
//
// 做法是用一个规范化的探针请求现组一次命令行再摘要，而不是让每个适配器
// 自己声明哪些参数与前缀有关——后者要求三个（将来更多）适配器各自维护一份
// 分类清单，分类错了不会有任何一层报错。探针法把分类问题整个取消掉：
// 指纹从不跨行比（同一个 (conv_id, agent_id) 只与它自己上一次的值比，
// 见契约第 1 节），所以只需要归一化「同一条会话内逐轮变化」的两样，
// 其余（工作目录路径、--cwd、--add-dir、身份文件路径、Role 本身……）
// 原样进摘要，进来零代价，漏掉才是继续瞎。
//
// 规范化的两样：
//   - req.ProviderSession 固定为空串（fresh 形状）：指纹必须在一条会话的
//     整个生命期里稳定，首轮算一次、之后每轮拿来比，按当轮实际形状算
//     会让每条会话的第二轮都漂移；固定用 fresh 形状还因为 agy 的身份拼接
//     只在这一支发生（ProviderSession == "" 时才把 Role 拼进正文最前面），
//     用 resume 形状算的话 agy 的前缀结构永远进不了摘要。
//   - req.Prompt 固定为 prefixProbePrompt。
//   - 组出来之后，argv 里等于 cmd.PlannedSessionID 的元素换成
//     prefixProbeSessionID（grok 在 fresh 支现铸一个 UUID，这一步不需要
//     任何适配器专有知识，只是找相等的字符串）。
//
// 返回 "<参数个数>/<sha256 前 12 位小写十六进制>"，例如 "14/9c1b7e0a2f44"。
// 参数个数放在前面是给人看的：flag 增减时这个数会变，一眼能看出方向。
// 摘要复用 packFingerprint 逐段编码：参数内容本身可能包含任意字节
// （规则正文、路径），直接拼接再哈希会有「a"+"bc" 与 "ab"+"c"」这类边界
// 歧义，长度前缀编码从根上消掉这个问题。
//
// **失败必须返回 error，不许吞**。BuildTurn 会因为权限档不放行、将来
// 新加的校验而失败，吞掉错误就是静默降级。
func PrefixArgsDigest(p provider.Provider, req provider.TurnRequest) (string, error) {
	args, err := normalizedPrefixArgs(p, req)
	if err != nil {
		return "", err
	}
	encoded := packFingerprint(args...)
	sum := sha256.Sum256([]byte(encoded))
	return strconv.Itoa(len(args)) + "/" + hex.EncodeToString(sum[:6]), nil
}

// normalizedPrefixArgs 组一次探针命令行，归一化 ProviderSession / Prompt /
// PlannedSessionID 之后把 Args 交回。单独拆出来是给 golden 用例钉「规范化
// 之后的 argv 列表本身」用（契约 4.2：钉 argv，不是钉一个哈希，哈希变了
// 看不出哪里变了）。
func normalizedPrefixArgs(p provider.Provider, req provider.TurnRequest) ([]string, error) {
	probe := req
	probe.ProviderSession = ""
	probe.Prompt = prefixProbePrompt

	cmd, err := p.BuildTurn(probe)
	if err != nil {
		return nil, err
	}
	args := append([]string(nil), cmd.Args...)
	if cmd.PlannedSessionID != "" {
		for i, a := range args {
			if a == cmd.PlannedSessionID {
				args[i] = prefixProbeSessionID
			}
		}
	}
	return args, nil
}

// RulesDigest 把一段规则正文压成 "<字节数>/<sha256 前 12 位小写十六进制>"，
// 例如 "4821/3f2a1b9c4d5e"。
//
// 空串必须返回空串，不是空串的哈希。schema v3 的迁移靠这条对上，别「修」它。
//
// 规则存摘要而 role 存全文，是因为两者要回答的问题不同。drift 报错要能
// 点名 role 的旧值（那是一行字，看得过来），而一段几 KB 的旧规则原文在
// 终端上没法给人看，能给人看的是「变了多少、变成了什么」；全文进
// identity_fp 这一列只会把库撑大而换不到表达力。
func RulesDigest(text string) string {
	if text == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	return strconv.Itoa(len(text)) + "/" + hex.EncodeToString(sum[:6])
}

// packFingerprint 用「十进制长度 + 冒号 + 正文 + 分号」逐段编码。
// role 是自由文本，里面完全可能出现分隔符；长度前缀让解码不依赖内容字节。
// 不用哈希：Ask 要在 ErrIdentityDrift 里点名哪一项变了，得能把旧值拆回来。
func packFingerprint(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(strconv.Itoa(len(p)))
		b.WriteByte(':')
		b.WriteString(p)
		b.WriteByte(';')
	}
	return b.String()
}

// unpackFingerprint 是 packFingerprint 的逆。格式不对时 ok 为 false，
// 调用方不得猜字段，只能退回「指纹整体不符」—— fail-closed，不把坏编码
// 拆成四个碰巧能用的空串，那会让一次身份漂移被显示成「什么都没变」。
func unpackFingerprint(s string) (parts []string, ok bool) {
	i := 0
	for i < len(s) {
		j := i
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j == i || j >= len(s) || s[j] != ':' {
			return nil, false
		}
		n, err := strconv.Atoi(s[i:j])
		if err != nil || n < 0 {
			return nil, false
		}
		start := j + 1
		end := start + n
		if end >= len(s) || s[end] != ';' {
			return nil, false
		}
		parts = append(parts, s[start:end])
		i = end + 1
	}
	return parts, true
}

// fingerprintParts 是解码后的指纹。用结构体而不是一串位置返回值：六段时
// 已经是 7 个返回值，七段就是 8 个，位置传错编译器拦不住，而传错的后果是
// drift 报错点名了错的字段——一种看起来正常的谎（契约第 3.3 节）。
type fingerprintParts struct {
	Role        string
	Model       string
	Effort      string
	Tier        string
	RulesGlobal string
	RulesLocal  string
	PrefixArgs  string
}

// parseFingerprint 是 packFingerprint 编码的指纹的解码入口，取代旧的
// fingerprintFields。段数不对（含库里现存的六段旧指纹）一律整体判 ok=false，
// 调用方不许猜字段：把一次身份漂移显示成「什么都没变」比拒绝识别更危险。
func parseFingerprint(fp string) (fingerprintParts, bool) {
	parts, ok := unpackFingerprint(fp)
	if !ok || len(parts) != fpFieldCount {
		return fingerprintParts{}, false
	}
	return fingerprintParts{
		Role:        parts[fpFieldRole],
		Model:       parts[fpFieldModel],
		Effort:      parts[fpFieldEffort],
		Tier:        parts[fpFieldTier],
		RulesGlobal: parts[fpFieldRulesGlobal],
		RulesLocal:  parts[fpFieldRulesLocal],
		PrefixArgs:  parts[fpFieldPrefixArgs],
	}, true
}

func coldExpectedAt(now, lastTurnAt time.Time, fresh bool) bool {
	if fresh {
		return true
	}
	if lastTurnAt.IsZero() {
		// 有会话记录却没有 lastTurnAt：状态表自己坏了。预报成冷启动，
		// 让屏幕先把「这一轮会按入场费计价」说出来，而不是假装还暖着。
		return true
	}
	return now.Sub(lastTurnAt) > CacheTTL
}
