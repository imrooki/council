package turn

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"council/internal/config"
	"council/internal/provider"
)

// DriftKind 把一条漂移分成两类后果。AskRefused 为真的前四类，按当前配置
// 对它 Ask 会在读进程内会话状态之前就被拒，不起进程、不花钱，与进程内状态无关。
// 后两类是身份已变：跨重启的下次 Ask 会作废重建、付一次入场费；进程内已开过口的
// 会被 ErrIdentityDrift 拒绝，要 /session rebuild 之后下一轮才按首轮付。
// 零值不属于任何一类，AskRefused 为假。
type DriftKind int

const (
	DriftKindUnset              DriftKind = 0 // 零值：没有任何分支填过，不属于任何一类
	DriftAgentGone              DriftKind = 1 // Reason「配置里已无此 agent」
	DriftProviderUnavailable    DriftKind = 2 // Reason「provider 不可用，算不出指纹: …」
	DriftPermissionRefused      DriftKind = 3 // Reason「权限档不放行，算不出指纹: …」
	DriftFingerprintUnavailable DriftKind = 4 // Reason「算不出指纹: …」
	DriftFingerprintUnparsable  DriftKind = 5 // Reason「指纹格式不可解」
	DriftFieldsChanged          DriftKind = 6 // Reason 是逐字段差异
)

// DriftRow 是一条漂移的 provider 会话。Kind 为 1 到 4（AskRefused 为真）时，
// 按当前配置 Ask 会在读进程内会话状态之前被拒，不起进程、不花钱。
// Kind 为 5 或 6 时身份已变：跨重启的下次 Ask 会作废重建、付一次入场费；
// 进程内已开过口的会被拒绝，要 /session rebuild 之后下一轮才按首轮付。
type DriftRow struct {
	ConvID  string
	AgentID string
	Reason  string // 人话，见 AuditDrift 的注释
	Kind    DriftKind
}

// AskRefused 报告这一行按当前配置 Ask 是否会在读进程内会话状态之前就被拒。
// 只看 Kind：1、2、3、4 为真；5、6、零值以及任何其它值都为假。
func (d DriftRow) AskRefused() bool {
	switch d.Kind {
	case DriftAgentGone, DriftProviderUnavailable, DriftPermissionRefused, DriftFingerprintUnavailable:
		return true
	default:
		return false
	}
}

// AuditDrift 把库里全部 provider 会话逐条与「当前配置 + 当前规则」算出的
// 指纹比对。每一行带 Kind：AskRefused 为真的是按当前配置必被拒、不花钱；
// 为假的是身份已变（跨重启会作废重建并付一次入场费，进程内已开过口的则被拒）。
// 本方法不读进程内会话状态，分不出后一类是进程内还是跨重启。
//
// 纯读，不写任何东西，也不碰 gates 里的 sessionState —— /rules、/config
// reload 与启动横幅都会调它，一个看一眼的命令不该产生副作用
// （runner.go 里 SessionInfo 那条注释同理）。
//
// 本方法不取任何 session 闸。这段理由被改写过四次，每次都必须说当时为真的那条：
//
// 第一次（原文）：「r.rules 和 r.cfg 在本方法运行期间根本不会被 Ask 改写
// （改 r.rules 只有 /rules reload --rebuild 会做，改 r.cfg 本步没有任何
// 命令会做）」。/config reload 就会改 r.cfg，那半句当场失效 —— 它偏偏
// 是整段论证的支柱。
//
// 第二次（8c-3）：真正的理由改成 REPL 的命令派发是单 goroutine 串行的，
// AuditDrift、Ask、SetRules、SetConfig 全部从那一条 goroutine 调，它们
// 之间不可能并发。与当时 SetRules 那条注释同一条理由。
//
// 第三次（本片，按 key 拆闸）：第二条也不成立了。结构上已经允许并发 Ask。
// 新理由：会话状态已不在 AuditDrift 的读集里（它本来就不碰），r.rules
// 由 rulesMu 守 —— 本方法取读锁拷一份快照立刻放开，再拿快照去比指纹，
// 不在持有 rulesMu 时做 IO，也不取任何 g.mu。
//
// 第四次（本片，cfgMu）：第三次只守住了 r.rules。r.cfg 由 cfgMu 守 ——
// 本方法取读锁拷一份指针立刻放开，再拿快照去比指纹，不在持有 cfgMu
// 时做 IO，也不取任何 g.mu。
//
// 不要把「否则一次飞行中的 /ask 会挡住它 5 分钟」当成主要理由 ——
// 拆闸之后本方法本来就不会去等那把闸。
//
// 返回按 (ConvID, AgentID) 升序，保证输出稳定、可以写 golden 用例。
// 不依赖 Service.AllProviderSessions 本身是否已经排好序（它确实排了，
// 但那是它的实现细节，不是本方法能引用的契约）。
func (r *Runner) AuditDrift(ctx context.Context) ([]DriftRow, error) {
	r.rulesMu.RLock()
	rules := r.rules
	r.rulesMu.RUnlock()
	r.cfgMu.RLock()
	cfg := r.cfg
	r.cfgMu.RUnlock()
	return r.auditDriftAgainst(ctx, rules, cfg)
}

// auditDriftAgainst 是 AuditDrift 的实现，把「用哪份规则去比」从
// r.rules 剥离成一个参数，让 AuditDrift(ctx) 这个冻结契约（11a 3.5）的
// 对外签名保持不变——它是包内私有的，repl 包够不到它，够不到也没关系：
// /rules reload（不带 --rebuild）要的「换一份规则试算、不改 Runner 状态」
// 这件事，是靠 repl 那边临时调公开的 SetRules 换上去、调 AuditDrift、
// 再换回来实现的，走的是这个包已经承诺的公开契约，不需要新开一个内部口子
// 被外面直接调用。之所以仍然把这一步拆成参数而不是让 AuditDrift 直接读
// r.rules，纯粹是为了让「指纹怎么算」这段逻辑只写一次：AuditDrift 本身
// 只负责把 r.rules 代进去。
func (r *Runner) auditDriftAgainst(ctx context.Context, rules RulesBlocks, cfg *config.Config) ([]DriftRow, error) {
	sessions, err := r.svc.AllProviderSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("audit drift: %w", err)
	}

	rows := make([]DriftRow, 0, len(sessions))
	for _, ps := range sessions {
		ac, ok := cfg.Agents[ps.AgentID]
		if !ok {
			// 配置里已经没有这个 agent 了，但那条会话还在库里占着位。
			// 这条也要报：用户删了 agent 又加回来（哪怕字段一个字都没变）
			// 时，它会带着一份旧指纹复活，指纹校验那时才会说话，
			// 不如现在就让用户看见这一行是悬空的。
			rows = append(rows, DriftRow{ConvID: ps.ConvID, AgentID: ps.AgentID, Reason: "配置里已无此 agent", Kind: DriftAgentGone})
			continue
		}

		p, lookupErr := provider.Lookup(ac.Provider)
		if lookupErr != nil {
			// 配置里 agent 还在，但 provider 名此刻查不到（写错了、或者
			// 那个适配器被下线了）。这与「配置里已无此 agent」不是一回事，
			// 措辞要分开：那一条是「这个成员已经不存在」，这一条是
			// 「这个成员还在，但我们现在算不出它的指纹该是什么」。
			//
			// 不跳过、不让整批审计失败：这条会话的 Reason 就是「算不出」
			// 本身，一行算不出来不该拖累其余能算的行——AuditDrift 存在的
			// 意义就是把静默的事情变响亮，静默跳过这一行只是把「响亮」
			// 打了个折扣，而不是更安全。
			rows = append(rows, DriftRow{ConvID: ps.ConvID, AgentID: ps.AgentID, Reason: fmt.Sprintf("provider 不可用，算不出指纹: %v", lookupErr), Kind: DriftProviderUnavailable})
			continue
		}
		tier, permErr := TranslatePermission(ac.Permissions)
		if permErr != nil {
			// 同上一支同理：权限档现在不被放行（full、或拼错的档），
			// 指纹算不出来，但会话仍然是活的，要报出来而不是悄悄略过。
			// workspace 已在 11c 第二片放行，不再落进这一支。
			rows = append(rows, DriftRow{ConvID: ps.ConvID, AgentID: ps.AgentID, Reason: fmt.Sprintf("权限档不放行，算不出指纹: %v", permErr), Kind: DriftPermissionRefused})
			continue
		}

		// convID 传 ps.ConvID：这一行本来就是某条会话的 provider 会话，
		// WORKSPACE 档的工作目录要含它才能与 Ask 当时真正用的路径对上
		// （agentWorkDir 的注释）。
		newFP, fpErr := identityFPFor(p, ps.AgentID, ac, r.workDir, ps.ConvID, tier, rules)
		if fpErr != nil {
			// 与 provider 查不到、权限档不放行同体例：一行算不出来不该拖累
			// 其余能算的行，AuditDrift 存在的意义就是把静默的事情变响亮。
			rows = append(rows, DriftRow{ConvID: ps.ConvID, AgentID: ps.AgentID, Reason: fmt.Sprintf("算不出指纹: %v", fpErr), Kind: DriftFingerprintUnavailable})
			continue
		}
		if newFP == ps.IdentityFP {
			continue // 没有漂移
		}
		parts, ok := identityDriftFields(ps.IdentityFP, newFP)
		if !ok {
			// 两个指纹的字符串不相等，但字段解不开——说明库里这一行的
			// identity_fp 格式本身就不对（理论上 v3 迁移之后不该再有这种
			// 行，但 fail-closed 地报出来，好过把它当成没有漂移悄悄放过）。
			rows = append(rows, DriftRow{ConvID: ps.ConvID, AgentID: ps.AgentID, Reason: "指纹格式不可解", Kind: DriftFingerprintUnparsable})
			continue
		}
		if len(parts) == 0 {
			// 字符串不等但逐字段比较没有差异，理论上不应该发生
			// （packFingerprint 是确定性编码），但比字符串相等更谨慎的
			// 判据仍然认为没有漂移，就不报。
			continue
		}
		rows = append(rows, DriftRow{ConvID: ps.ConvID, AgentID: ps.AgentID, Reason: strings.Join(parts, "; "), Kind: DriftFieldsChanged})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ConvID != rows[j].ConvID {
			return rows[i].ConvID < rows[j].ConvID
		}
		return rows[i].AgentID < rows[j].AgentID
	})
	return rows, nil
}
