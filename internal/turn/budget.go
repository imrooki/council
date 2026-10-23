package turn

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
)

// RoundSnap 是一轮 Speakers 共用的规则、配置与 seq 上界。
//
// Budget 与 RunRound 必须用同一份：身份块含两段规则正文，两边各取一次，
// 预报的前缀与真正送出去的就不是同一段。upTo 同理，no_delta 的判据
// 必须是 RunRound 将要用的那个上界。
//
// 片 3d 的 /round 取一次，同时传给 BudgetWith 与 RunRoundWith。
// 分开调公开的 Budget 与 RunRound 仍会各拍一次；那条缝不靠 REPL 串行来堵。
type RoundSnap struct {
	Rules RulesBlocks
	Cfg   *config.Config
	UpTo  int64
}

// BudgetRow 是一个成员本轮的预报。屏幕归 repl，本类型不印任何东西。
//
// 不提供把 CostUSD 加总的方法。只有 claude 报金额，grok 与 agy 只报
// token。为了凑总金额编汇率，是在把「没这个数」写成一个数。
// token 能加：三家的 input_tokens 都归一到「本轮完整 prompt」。
// 合计本身是片 3d 的屏幕活，调用方自己加 InputTokens，本类型不加。
type BudgetRow struct {
	AgentID, Provider string
	// Cold 是预报。SessionInfo 的 ok == false（从没开过口）时必为 true。
	Cold bool
	// InFlight 复用 SessionInfo 的 TryLock 路径：闸拿不到就从库读上一次
	// 提交的状态。在飞的成员仍给预报，不因为在飞就未标定。
	InFlight bool
	// Calibrated 只由「有没有一条能拿来缩放的 token 样本」决定。
	// 样本报了金额才填 CostUSD；没报金额时 Calibrated 仍可以是 true，
	// CostUSD 是 nil。false 时 InputTokens 与 CostUSD 都是 nil。
	Calibrated   bool
	InputTokens  *int
	CostUSD      *float64
	ArgvHeadroom *int
	// Basis 是取样的那个 turn_id。空串表示没取到可用样本。
	Basis string
	// NoDelta：本轮 Select 之后没有可投递的新内容。
	// Skipped 由片 3d 的 /round 据它组，本函数不改入参 Plan。
	// true 时不填 token、金额、Basis、ArgvHeadroom：这一轮不会把 prompt 送出去。
	NoDelta bool
}

// TakeRoundSnap 取一轮共用的规则、配置与 MaxSeq。
// 顺序与 RunRound 抽出来之前相同：先放锁的规则/配置，再一次 MaxSeq。
// MaxSeq 失败的文案保持 "max seq:"，调用方按这个前缀认。
func (r *Runner) TakeRoundSnap(ctx context.Context, convID string) (RoundSnap, error) {
	rules, cfg := r.snapshotRulesConfig()
	upTo, err := r.svc.MaxSeq(ctx, convID)
	if err != nil {
		return RoundSnap{}, fmt.Errorf("max seq: %w", err)
	}
	return RoundSnap{Rules: rules, Cfg: cfg, UpTo: upTo}, nil
}

// Budget 预报 plan.Speakers 这一轮的规模。自己取一次快照。
// 要与随后的 RunRound 共用同一份时，改用 TakeRoundSnap + BudgetWith + RunRoundWith。
func (r *Runner) Budget(ctx context.Context, convID string, plan Plan) ([]BudgetRow, error) {
	snap, err := r.TakeRoundSnap(ctx, convID)
	if err != nil {
		return nil, err
	}
	return r.BudgetWith(ctx, convID, plan, snap)
}

// BudgetWith 用调用方已经取好的 snap 预报。不发面包屑、不起进程、不落库。
// 行序与 plan.Speakers 一致，含 NoDelta 的成员：他们在表上单独一档，
// 不从切片里消失。Judges 本片不预报。
func (r *Runner) BudgetWith(ctx context.Context, convID string, plan Plan, snap RoundSnap) ([]BudgetRow, error) {
	rows := make([]BudgetRow, 0, len(plan.Speakers))
	for _, id := range plan.Speakers {
		row, err := r.budgetOne(ctx, convID, id, snap)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (r *Runner) budgetOne(ctx context.Context, convID, agentID string, snap RoundSnap) (BudgetRow, error) {
	row := BudgetRow{AgentID: agentID, Cold: true}
	cfg := snap.Cfg
	if cfg == nil || cfg.Agents == nil {
		return row, nil
	}
	ac, ok := cfg.Agents[agentID]
	if !ok {
		return row, nil
	}
	row.Provider = ac.Provider
	p, err := provider.Lookup(ac.Provider)
	if err != nil {
		return row, nil
	}
	tier, err := TranslatePermission(ac.Permissions)
	if err != nil {
		return row, nil
	}
	if r.workDir == "" {
		return row, nil
	}

	fp, fpErr := identityFPAt(p, agentID, ac, r.dirFor(convID, agentID, tier), r.roleDirFor(agentID), tier, snap.Rules)
	if fpErr != nil || fp == "" {
		// 指纹算不出就不要拿空串去查。identity_fp 的历史行全是空串，
		// 空串会命中片 2a 之前的空窗，那是在用另一份前缀的数字冒充。
		// 不回填一个猜出来的指纹。
		return row, nil
	}

	view, err := r.forecastSession(ctx, convID, agentID, fp)
	if err != nil {
		return BudgetRow{}, err
	}
	row.Cold = view.cold
	row.InFlight = view.inFlight

	conv, err := r.svc.Conversation(ctx, convID)
	if err != nil {
		return BudgetRow{}, err
	}
	scanned, err := r.readAfter(ctx, convID, view.cursor)
	if err != nil {
		return BudgetRow{}, err
	}
	if snap.UpTo > 0 {
		kept := scanned[:0]
		for _, m := range scanned {
			if m.Seq <= snap.UpTo {
				kept = append(kept, m)
			}
		}
		scanned = kept
	}
	routeThought, visErr := r.thoughtVisibleFor(conv.TeamID, ac.Mode, cfg)
	if visErr != nil {
		return row, nil
	}
	router := NewRouter(routeThought)
	selected := router.Select(scanned, ac, agentID, view.bootstrap)
	prompt := router.Render(selected, conv.Title, view.bootstrap)
	if len(selected) == 0 {
		row.NoDelta = true
		row.Calibrated = false
		row.Cold = view.cold
		return row, nil
	}

	headroom := argvHeadroom(p, agentID, ac, tier, snap.Rules, view.providerSession, prompt, r.dirFor(convID, agentID, tier), r.roleDirFor(agentID), cfg)
	row.ArgvHeadroom = headroom

	samples, err := r.svc.UsageSamples(ctx, agentID, fp, -1)
	if err != nil {
		return BudgetRow{}, err
	}
	sample, found := pickUsageSample(samples, p.Capabilities().CacheAcrossTurns, view.cold)
	if !found || sample.InputTokens == nil || sample.PromptBytes <= 0 {
		return row, nil
	}
	scaled, okScale := scaleTokens(*sample.InputTokens, sample.PromptBytes, len(prompt))
	if !okScale {
		return row, nil
	}
	row.Calibrated = true
	row.InputTokens = &scaled
	row.Basis = sample.TurnID
	if sample.CostUSD != nil {
		cost := *sample.CostUSD * float64(len(prompt)) / float64(sample.PromptBytes)
		if !math.IsInf(cost, 0) && !math.IsNaN(cost) {
			row.CostUSD = &cost
		}
	}
	return row, nil
}

// forecastView 是预报用的会话视图。不写 g.st，不发面包屑，不作废。
type forecastView struct {
	cold            bool
	inFlight        bool
	bootstrap       bool
	cursor          int64
	providerSession string
}

// forecastSession 复用 SessionInfo 的 TryLock 路径，但读库失败上抛。
// SessionInfo 把读失败收成 ok=false，是因为 /session 要整张表。
// 预报把「不知道」收成「必冷」会写成一个确定的冷暖，取样会去拿错的那一支。
//
// 指纹不符按冷 + bootstrap 预报：Ask 在那一支作废重建、按 Fresh 走。
// 这里不删行、不写 g.st。内存里已有一条且指纹相符时用它的游标；
// 不符时不拿那条游标，否则 no_delta 会按一条即将作废的会话来判。
func (r *Runner) forecastSession(ctx context.Context, convID, agentID, fp string) (forecastView, error) {
	now := r.now()
	g := r.gate(sessionKey{convID: convID, agentID: agentID})
	if g.mu.TryLock() {
		defer g.mu.Unlock()
		if g.st != nil {
			if g.st.identityFP == fp && g.st.providerSession != "" {
				return forecastView{
					cold:            coldExpectedAt(now, g.st.lastTurnAt, false),
					inFlight:        false,
					bootstrap:       false,
					cursor:          g.st.cursor,
					providerSession: g.st.providerSession,
				}, nil
			}
			return forecastView{cold: true, bootstrap: true}, nil
		}
	} else {
		loaded, found, err := r.svc.ProviderSession(ctx, convID, agentID)
		if err != nil {
			return forecastView{}, err
		}
		if !found || loaded.IdentityFP != fp || loaded.ProviderSessionID == "" {
			return forecastView{cold: true, inFlight: true, bootstrap: true}, nil
		}
		return forecastView{
			cold:            coldExpectedAt(now, loaded.LastTurnAt, false),
			inFlight:        true,
			bootstrap:       false,
			cursor:          loaded.CursorSeq,
			providerSession: loaded.ProviderSessionID,
		}, nil
	}

	loaded, found, err := r.svc.ProviderSession(ctx, convID, agentID)
	if err != nil {
		return forecastView{}, err
	}
	if !found || loaded.IdentityFP != fp || loaded.ProviderSessionID == "" {
		return forecastView{cold: true, bootstrap: true}, nil
	}
	return forecastView{
		cold:            coldExpectedAt(now, loaded.LastTurnAt, false),
		bootstrap:       false,
		cursor:          loaded.CursorSeq,
		providerSession: loaded.ProviderSessionID,
	}, nil
}

// pickUsageSample 在已经按 created_at、turn_id 倒序的样本里取第一条能缩放的。
// prompt_bytes <= 0 必须跳过：那一列 NOT NULL DEFAULT 0，历史行是 0，
// 拿来当除数是除零。先在 SQL 里 LIMIT 1 再跳过，会把更早的合格行丢掉。
//
// SupportUnknown 不取样。零值是「未实测」，当成 SupportNo 或 SupportYes
// 都是把没测过的说成一个确定的缓存模型。今天三家适配器都填了
// CacheAcrossTurns，这条分支今天走不到。
//
// 样本只要成功轮：Ingested() 为真（OutcomeSpoke 与 OutcomePostRejected），
// SupportNo 与 SupportYes 两支一样。这是 Step 9「成功轮」的本意，
// PostRejected 有意算进来，排除它会往低估偏。
// 失败轮不能当样本。provider.go:279-280 记下 2026-09-20 的失败探针：
// claude 与 agy 的失败输出里 usage 键都在、五项都是货真价实的 0。
// 拿这个 0 去缩放，预报 token 是 0，并且被标成已标定。
// 金额列不会因此变成 0：computeCostDelta 只在成功路径 runner.go:733，
// 失败样本的 CostUSD 仍是 nil，金额格仍是未标定。
// 失败轮为什么能过「input_tokens 非 NULL 且 prompt_bytes > 0」这道旧门，是两件事，别缠在一起读：
//   - 用量非 NULL：runner.go:702 在失败返回之前就吸收了用量（absorbParse 置 UsageParsed，
//     makeTurnUsage 于是把那个 0 写成 &0，不是 NULL）。空会话 id 那条同形，:709 吸收、:710 以 OutcomeUnparsable 返回。
//   - prompt_bytes > 0：runner.go:531 在起进程之前就写下，失败轮也有。
//
// 另有一件与之无关的事：cold_start 是零值 false，因为 :704 的失败返回早于 :737 的冷暖赋值，
// 那个 false 从来没被计算过。
// 用量解析过，不表示冷暖也算过；用量非 NULL，也不表示这一轮正常。
// 这与未决 #23 一致：cold_start 与 prompt_bytes 只在 Ingested 行上有意义。
// 本刀取代 12_STEP9_CONTRACT.md §4.5a 第四条的样本口径，改为
// Ingested() 且 input_tokens 非 NULL 且 prompt_bytes > 0，三支仍然统一。
// 不把 runner.go:737 挪到失败返回之前，它修不全：挪了之后失败轮会按它的 cache_read 被算成冷或暖
// （那个值不一定是一次测量），冷暖筛选只能滤掉对不上的那一半，SupportNo 与冷暖恰好对上时照样选中它。
// Settle 与 /ask 的冷暖行不随 :737 的位置变化：它们经 Record.MeasuredCold 判，非 Ingested 的轮一律按未知（X-36ab）。
func pickUsageSample(samples []conversation.TurnUsage, cache provider.Support, cold bool) (conversation.TurnUsage, bool) {
	switch cache {
	case provider.SupportNo, provider.SupportYes:
	default:
		return conversation.TurnUsage{}, false
	}
	for _, u := range samples {
		if !Outcome(u.Outcome).Ingested() {
			continue
		}
		if u.InputTokens == nil || u.PromptBytes <= 0 {
			continue
		}
		if cache == provider.SupportYes && u.ColdStart != cold {
			continue
		}
		return u, true
	}
	return conversation.TurnUsage{}, false
}

// scaleTokens 做整数缩放：样本 token * 本轮字节 / 样本字节。
// 不用 float64：除零得到 +Inf，而「未标定」必须是 nil 指针，不是 Inf。
// 样本字节 <= 0 拒绝。乘积超出 int64 也拒绝，不静默截断。
// 整数除法向零截断，预报略偏低，与「取样结构上偏低、方向只记录不断言」一致。
func scaleTokens(sampleTokens, sampleBytes, thisBytes int) (int, bool) {
	if sampleBytes <= 0 || sampleTokens < 0 || thisBytes < 0 {
		return 0, false
	}
	prod := int64(sampleTokens) * int64(thisBytes)
	if thisBytes != 0 && prod/int64(thisBytes) != int64(sampleTokens) {
		return 0, false
	}
	return int(prod / int64(sampleBytes)), true
}

// argvHeadroom 对本轮真实 prompt 组一次命令行，量相对 maxCommandLineBytes 的余量。
// 不做整轮拒绝，那是片 3d。余量可以为负。
//
// MaxPromptBytes > 0 时再与「prompt 闸还剩多少」取更紧的那个。
// BuildTurn 因 ErrPromptTooLong 组不出来时，余量只反映 prompt 闸，不含 flag 段。
// 不编一个引号修正量：CommandLineBytes 偏小是既有的、写明了的取舍。
// LookPath 失败用原 path，与 probeHeadroom 同一处偏乐观。
//
// ProviderSession 用预报看到的那条：空串是 bootstrap，agy 会把身份块拼进正文，
// 命令行比 resume 更长。用错形状，余量会偏乐观。
// dir / roleDir 必须由调用方的 dirFor / roleDirFor 解析好：固定副本模式下
// 它们与真正发出去的 TurnRequest 是同一对路径，量出来的余量才作数。
func argvHeadroom(
	p provider.Provider,
	agentID string,
	ac config.AgentConfig,
	tier provider.PermissionTier,
	rules RulesBlocks,
	providerSession, prompt, dir, roleDir string,
	cfg *config.Config,
) *int {
	identity := renderIdentity(agentID, ac, rules)
	req := provider.TurnRequest{
		AgentID:         agentID,
		DisplayName:     ac.DisplayName,
		Model:           ac.Model,
		Effort:          ac.Effort,
		Role:            identity,
		Permission:      tier,
		ProviderSession: providerSession,
		Prompt:          prompt,
		// 与 Ask 的真实请求保持一致：claude 的文件名会带外部提示的哈希，
		// 漏传会让这里的命令行长度比真实请求短十几字节。
		SystemPrompt: ac.Prompt,
		WorkDir:      dir,
		RoleDir:      roleDir,
		MaxTurns:     0,
	}
	caps := p.Capabilities()
	cmd, err := p.BuildTurn(req)
	if err != nil {
		if errors.Is(err, provider.ErrPromptTooLong) && caps.MaxPromptBytes > 0 {
			n := caps.MaxPromptBytes - len(prompt)
			return &n
		}
		return nil
	}
	if cmd.Path == "" && cfg != nil && cfg.Providers != nil {
		cmd.Path = cfg.Providers[ac.Provider].Command
	}
	path := cmd.Path
	if resolved, lookErr := exec.LookPath(path); lookErr == nil {
		path = resolved
	}
	remaining := maxCommandLineBytes - CommandLineBytes(path, cmd.Args)
	if caps.MaxPromptBytes > 0 {
		promptRoom := caps.MaxPromptBytes - len(prompt)
		if promptRoom < remaining {
			remaining = promptRoom
		}
	}
	return &remaining
}
