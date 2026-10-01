package turn

// 本文件覆盖 K 节「发言归属」组与「累计值对照」组里落在 internal/turn 这一层
// 的部分：agentMeta 组装出来的 Meta 键表。09a record.go 一节列出的键名
// 是唯一权威，assertions 直接对着那张表核对，不对着实现的当前输出反推。

import (
	"context"
	"encoding/json"
	"testing"

	"council/internal/provider"
)

// metaFloat64Slice 把一条经过 store 的 JSON 编解码往返之后的 []any（元素是
// float64，见 internal/store/tx.go 的 encodeMeta/decodeMeta：meta 整体走一次
// json.Marshal/Unmarshal，int64 在这个往返里变成 float64）转回 []int64，
// 方便和 Record.Delivered 那种原生 []int64 比较。
func metaFloat64Slice(t *testing.T, v any) []int64 {
	t.Helper()
	raw, ok := v.([]any)
	if !ok {
		t.Fatalf("meta 值不是 []any，实际类型 %T: %+v", v, v)
	}
	out := make([]int64, 0, len(raw))
	for _, e := range raw {
		f, ok := e.(float64)
		if !ok {
			t.Fatalf("元素不是 float64，实际类型 %T: %+v", e, e)
		}
		out = append(out, int64(f))
	}
	return out
}

// metaMapSlice 把经过同样 JSON 往返的 []provider.DroppedOption（原本是
// struct 切片，没有 json 标签，字段名 Option/Value/Reason 原样当 JSON 键）
// 转成 []map[string]any，方便取字段。
func metaMapSlice(t *testing.T, v any) []map[string]any {
	t.Helper()
	raw, ok := v.([]any)
	if !ok {
		t.Fatalf("meta 值不是 []any，实际类型 %T: %+v", v, v)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		m, ok := e.(map[string]any)
		if !ok {
			t.Fatalf("元素不是 map[string]any，实际类型 %T: %+v", e, e)
		}
		out = append(out, m)
	}
	return out
}

// TestModelSourceOfPureFunction 直接调用 modelSourceOf（同包未导出纯函数），
// 不经过 Ask，覆盖两条边界，且不需要伪造任何 provider stdout：
//
//   - ModelKeys==0（这家根本不报实测型号，例如 agy 没有 modelUsage，或
//     claude 失败时 modelUsage 是 {}）-> "requested"，不是 "ambiguous"。
//     这是任务书原话「这条刚修过，专门钉住它」的那一条修复。
//   - ModelKeys>1（claude 的 modelUsage 是累计的，一旦某轮用过第二个型号，
//     此后每轮都是多键）-> "ambiguous"。09 没有一份真实探针字节走到过这个
//     分支（三份成功样本 modelUsage 都恰好一个键），所以这里直接构造
//     provider.TurnResult 的 Go 值来测这条纯函数分支，而不是伪造一段
//     「假装是 CLI 吐出来的」JSON——两者不是一回事：前者是对已解析结果
//     的直接单元测试，后者才是任务书明令禁止的「自造 provider stdout」。
func TestModelSourceOfPureFunction(t *testing.T) {
	claudeCaps, err := provider.Lookup("claude")
	if err != nil {
		t.Fatalf("provider.Lookup(claude) 失败: %v", err)
	}
	caps := claudeCaps.Capabilities()
	if caps.ReportsObservedModel != provider.SupportYes {
		t.Fatalf("前置条件不成立：claude 的 ReportsObservedModel 应当是 SupportYes")
	}

	t.Run("0 个键是 requested 不是 ambiguous", func(t *testing.T) {
		got := modelSourceOf(caps, provider.TurnResult{ModelKeys: 0, ObservedModel: ""})
		if got != modelSourceRequested {
			t.Errorf("modelSourceOf(ModelKeys=0) = %q，期望 %q", got, modelSourceRequested)
		}
	})

	t.Run("2 个键才是 ambiguous", func(t *testing.T) {
		got := modelSourceOf(caps, provider.TurnResult{ModelKeys: 2, ObservedModel: ""})
		if got != modelSourceAmbiguous {
			t.Errorf("modelSourceOf(ModelKeys=2) = %q，期望 %q", got, modelSourceAmbiguous)
		}
	})

	t.Run("恰好 1 个键是 observed", func(t *testing.T) {
		got := modelSourceOf(caps, provider.TurnResult{ModelKeys: 1, ObservedModel: "claude-fable-5-1"})
		if got != modelSourceObserved {
			t.Errorf("modelSourceOf(ModelKeys=1) = %q，期望 %q", got, modelSourceObserved)
		}
	})
}

// TestAgyModelSourceIsRequestedThroughFullAskFlow 用真实的 agy-t1.json 走
// 一遍完整的 Ask 流程（不直接调 modelSourceOf），确认端到端也是这个结论：
// agy 的 Capabilities.ReportsObservedModel 是 SupportNo，modelSourceOf 应当
// 直接短路成 "requested"，不看 ModelKeys（agy 恒为 0）。同时顺带确认
// agy 完全不写 MetaObservedModel 这个键（下面 TestAgyNeverWritesObservedModelKey
// 里有一条更直接的断言，这里只看 model_source 这一项）。
func TestAgyModelSourceIsRequestedThroughFullAskFlow(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentAgy)
	postUser(t, svc, convID, "请走查这段代码")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(agyT1JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentAgy)
	if err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	got, ok := rec.Message.Meta[MetaModelSource]
	if !ok {
		t.Fatal("Meta 里没有 model_source 这个键")
	}
	if got != modelSourceRequested {
		t.Errorf("agy 的 Meta[%q] = %v，期望 %q", MetaModelSource, got, modelSourceRequested)
	}
}

// TestAgyNeverWritesObservedModelKey 覆盖 K 节「发言归属」组：
// agy 不写 observed_model 这个键——不是写一个空字符串，是键本身不存在，
// 09a 原话「agy 不写这个键」。用 map 的双返回值形式区分「空字符串」与
// 「键不存在」，这条区分正是这条用例要钉住的东西。
func TestAgyNeverWritesObservedModelKey(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentAgy)
	postUser(t, svc, convID, "请走查这段代码")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(agyT1JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentAgy)
	if err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	if v, ok := rec.Message.Meta[MetaObservedModel]; ok {
		t.Errorf("agy 的消息不该有 observed_model 这个键，实际存在且值为 %v", v)
	}
}

// TestAgyDeniedActionsFlowThroughAskToRecordAndMeta 覆盖本片的核心行为：
// agy 越界写被拒（denied_actions 非空、response 清空）时，Ask 走的仍然是
// OutcomeSpoke（不判成失败——界内的事可能已经做完，见 record.go
// Record.DeniedActions 的注释与 docs/11c_STEP8B_WORKSPACE_CONTRACT.md 第 7
// 节），rec.DeniedActions 这个类型化字段与 messages.meta 里的
// denied_actions 键都要能看到这件事。
func TestAgyDeniedActionsFlowThroughAskToRecordAndMeta(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentAgy)
	postUser(t, svc, convID, "在工作目录里建个文件")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(agyDeniedActionJSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentAgy)
	if err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	// 不是失败：被拒不等于这一轮失败，界内的事可能已经做完。
	if rec.Outcome != OutcomeSpoke {
		t.Fatalf("Outcome = %v, want OutcomeSpoke（denied_actions 非空不改变结局）", rec.Outcome)
	}
	if rec.Message == nil || rec.Message.Text != "" {
		t.Fatalf("Message.Text 应当是空串（这份原件的 response 确实被清空了），实际 %+v", rec.Message)
	}
	want := provider.DeniedAction{Action: "write_file", DisplayName: "WriteToFile"}
	if len(rec.DeniedActions) != 1 || rec.DeniedActions[0] != want {
		t.Fatalf("rec.DeniedActions = %+v, want [%+v]", rec.DeniedActions, want)
	}

	metaVal, ok := rec.Message.Meta[MetaDeniedActions]
	if !ok {
		t.Fatal("Meta 里没有 denied_actions 这个键")
	}
	// 经 Post/RecordTurn 落库再读回，Meta 里的值已经是泛化的 JSON 结构
	// （[]any 套 map[string]any），不再是 []provider.DeniedAction 这个 Go
	// 类型——这与 TestUsageRawIsProviderOriginalBytes 断言 usage_raw 时
	// 用 map[string]any 接住是同一个道理，不能直接类型断言成写入时的类型。
	list, ok := metaVal.([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("Meta[denied_actions] 不是长度为 1 的 []any，实际类型 %T 值 %+v", metaVal, metaVal)
	}
	entry, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("Meta[denied_actions][0] 不是一个 JSON 对象，实际类型 %T", list[0])
	}
	if entry["Action"] != "write_file" || entry["DisplayName"] != "WriteToFile" {
		t.Errorf("Meta[denied_actions][0] = %+v，期望 Action=write_file DisplayName=WriteToFile", entry)
	}

	if _, ok := rec.Message.Meta[MetaDeniedActionsRaw]; !ok {
		t.Error("Meta 里没有 denied_actions_raw 这个键")
	}
}

// TestGrokMessageMetaEffortFields 覆盖 K 节「发言归属」组：grok 的消息
// sender_effort 记的是请求值，Meta["effort_policy"]="process_external_config"，
// Meta["effort_flag_omitted"]=true，Meta["dropped"] 里有 effort 这一项
// （grok.go 的 BuildTurn 无条件把这一项塞进 Command.Dropped，见 grokProvider
// 的注释：「明知会被吞还发，是在给自己伪造证据」，所以选择完全不发）。
func TestGrokMessageMetaEffortFields(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentGrok)
	postUser(t, svc, convID, "请做数值验算")

	ac := cfg.Agents[testAgentGrok]
	if ac.Effort == "" {
		t.Fatal("前置条件不成立：测试配置里 grok 的 Effort 应当非空，才谈得上「有请求值」")
	}

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(grokT1JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentGrok)
	if err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	msg := rec.Message

	if msg.SenderEffort != ac.Effort {
		t.Errorf("SenderEffort = %q，期望等于请求值 %q（08a：这一列只能抄 AgentConfig.Effort，不当 ground truth）", msg.SenderEffort, ac.Effort)
	}
	if got := msg.Meta[MetaEffortPolicy]; got != "process_external_config" {
		t.Errorf("Meta[%q] = %v，期望 %q", MetaEffortPolicy, got, "process_external_config")
	}
	if got := msg.Meta[MetaEffortFlagOmitted]; got != true {
		t.Errorf("Meta[%q] = %v，期望 true（grok 从不发 effort flag）", MetaEffortFlagOmitted, got)
	}
	if got := msg.Meta[MetaEffortSource]; got != effortSourceRequested {
		t.Errorf("Meta[%q] = %v，期望 %q（恒为 requested）", MetaEffortSource, got, effortSourceRequested)
	}

	droppedRaw, ok := msg.Meta[MetaDropped]
	if !ok {
		t.Fatal("Meta 里没有 dropped 这个键，期望 grok 的 effort 这一项在里面")
	}
	dropped := metaMapSlice(t, droppedRaw)
	foundEffort := false
	for _, d := range dropped {
		if d["Option"] == "effort" {
			foundEffort = true
			break
		}
	}
	if !foundEffort {
		t.Errorf("Meta[dropped] 里没有 Option=effort 这一项，实际: %+v", dropped)
	}
}

// TestClaudeEffortFlagOmittedTracksActualFlagNotPolicy 覆盖 record.go
// MetaEffortFlagOmitted 那行注释钉的坑：claude 虽然是 EffortPerTurnFlag，
// 但当 AgentConfig.Effort 是空串时，BuildTurn 根本不会发 --effort（claude.go
// 原话「每轮都发，包括 Role 为空串……」是针对 --append-system-prompt 说的，
// --effort 那一行是 `if req.Effort != "" { args = append(args, "--effort",
// req.Effort) }`，空值不发）。effort_flag_omitted 记的是「这一轮实际有没有
// 放上命令行」，只看策略（EffortPerTurnFlag）会把这种情况报成 false，
// 而实际这一轮确实没发 flag。
func TestClaudeEffortFlagOmittedTracksActualFlagNotPolicy(t *testing.T) {
	cfg := newTestConfig()
	ac := cfg.Agents[testAgentClaude]
	ac.Effort = "" // 关键：配置里这一档留空
	cfg.Agents[testAgentClaude] = ac

	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "请审阅")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	if got := rec.Message.Meta[MetaEffortFlagOmitted]; got != true {
		t.Errorf("Meta[%q] = %v，期望 true：AgentConfig.Effort 为空串时，即使策略是 per_turn_flag，这一轮实际也没有发 --effort", MetaEffortFlagOmitted, got)
	}
	if got := rec.Message.Meta[MetaEffortPolicy]; got != "per_turn_flag" {
		t.Errorf("Meta[%q] = %v，期望 %q（策略本身没变，变的只是这一轮发没发）", MetaEffortPolicy, got, "per_turn_flag")
	}

	// 交叉核对：Spawner 真正收到的命令行里确实没有 --effort。
	if len(fs.calls) != 1 {
		t.Fatalf("期望 Spawner 被调用 1 次，实际 %d 次", len(fs.calls))
	}
	for _, a := range fs.calls[0].cmd.Args {
		if a == "--effort" {
			t.Error("命令行里出现了 --effort，但 AgentConfig.Effort 是空串，这一轮不该发这个 flag")
		}
	}
}

// TestUsageRawIsProviderOriginalBytes 按 09a 的冻结契约写：Meta["usage_raw"]
// 应当是 TurnResult.UsageRaw 的原始字节，原样，「绝不是归一化后的 Usage」
// （09a record.go 一节，2026-09-21 订正版原话）。
//
// 这条用例原先是故意留红的：provider.TurnResult 那时没有 UsageRaw 字段，
// runner.go 存进 MetaUsageRaw 的是归一化之后的 provider.Usage 结构体
// （字段名是 InputTokens/CacheCreate 这些 Go 名字，不是原始 JSON 键）。
// commit cf01d81 加了 TurnResult.UsageRaw（三个适配器用
// rawTopLevelField(stdout, "usage") 填）并把 runner.go 的 agentMeta 改成写
// res.UsageRaw 之后，这里改回验证契约要求的行为本身：断言原始键
// （不是 Go 结构体字段名）与原始数值都在，而不是只断言"某个键存在"这种
// 弱断言——弱断言即使 runner.go 又悄悄换回归一化结构体也可能凑巧不报错
// （归一化结构体里恰好也没有 cache_creation_input_tokens 这个键，所以原来
// 那条"键不存在就报错"的断言其实已经够强，这里额外加一条数值核对，
// 把"原始"坐实到具体数字上）。
func TestUsageRawIsProviderOriginalBytes(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "请审阅")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	rec, err := r.Ask(context.Background(), convID, testAgentClaude)
	if err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	raw, ok := rec.Message.Meta[MetaUsageRaw]
	if !ok {
		t.Fatal("Meta 里没有 usage_raw 这个键")
	}
	m, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("usage_raw 不是一个 JSON 对象，实际类型 %T: %+v", raw, raw)
	}
	// claude-t1.json 的 usage 子对象里这两个键的原始数值：
	// input_tokens=2、cache_creation_input_tokens=47749。用原始键名 + 原始
	// 数值一起核对，而不是只核对键存在——这样"归一化结构体凑巧也有同名键"
	// 这种巧合没有可乘之机。
	wantInt := map[string]float64{
		"input_tokens":                2,
		"cache_creation_input_tokens": 47749,
		"cache_read_input_tokens":     0,
		"output_tokens":               4,
	}
	for k, want := range wantInt {
		v, ok := m[k]
		if !ok {
			t.Errorf("Meta[usage_raw] 里没有原始键 %q，实际键有: %v", k, mapKeys(m))
			continue
		}
		f, ok := v.(float64)
		if !ok || f != want {
			t.Errorf("Meta[usage_raw][%q] = %v，期望原始值 %v", k, v, want)
		}
	}
	// 反面：归一化结构体的 Go 字段名不该出现在这里——出现就说明又写回了
	// copyUsage(res.Usage) 而不是 res.UsageRaw。
	for _, normalizedFieldName := range []string{"InputTokens", "CacheCreate", "CostUSDTotal"} {
		if _, present := m[normalizedFieldName]; present {
			t.Errorf("Meta[usage_raw] 里出现了归一化结构体的字段名 %q，说明存的又是 copyUsage(res.Usage) 而不是原始字节", normalizedFieldName)
		}
	}
}

// TestAgentMetaOmitsUsageRawKeyWhenEmpty 直接调用同包内未导出的
// r.agentMeta（不经 Ask/ParseTurn），验证 commit cf01d81 那条具体行为：
// len(res.UsageRaw)==0 时 Meta 里完全不写 usage_raw 这个键，不写
// null 占位。
//
// 直接构造 provider.TurnResult{UsageRaw: nil} 而不是找一份「usage 键缺席」
// 的真实探针字节，理由与 TestModelSourceOfPureFunction 一致：这是对已解析
// 结果（一个 Go 值）的直接单元测试，不是伪造一段「假装是 CLI 吐出来的」
// stdout——现有三家的成功探针里 usage 键从未缺席过，找不到对应的真实字节，
// 但 agentMeta 这一层的分支本身不需要真的走一遍解析就能测到。
func TestAgentMetaOmitsUsageRawKeyWhenEmpty(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	r := newTestRunner(t, cfg, svc)

	claudeAdapter, err := provider.Lookup("claude")
	if err != nil {
		t.Fatalf("provider.Lookup(claude) 失败: %v", err)
	}
	caps := claudeAdapter.Capabilities()

	t.Run("UsageRaw 为 nil 时不写这个键", func(t *testing.T) {
		res := provider.TurnResult{ProviderSession: "s1", UsageRaw: nil}
		meta := r.agentMeta(caps, provider.Command{}, Record{}, res, "")
		if v, ok := meta[MetaUsageRaw]; ok {
			t.Errorf("UsageRaw 为 nil 时不该写 usage_raw 这个键，实际写了 %v", v)
		}
	})

	t.Run("UsageRaw 非空时原样写入", func(t *testing.T) {
		res := provider.TurnResult{ProviderSession: "s1", UsageRaw: []byte(`{"input_tokens":2}`)}
		meta := r.agentMeta(caps, provider.Command{}, Record{}, res, "")
		v, ok := meta[MetaUsageRaw]
		if !ok {
			t.Fatal("UsageRaw 非空时应当写 usage_raw 这个键，实际没写")
		}
		raw, ok := v.(json.RawMessage)
		if !ok {
			t.Fatalf("usage_raw 的值类型是 %T，期望 json.RawMessage（写盘前的内存值，尚未经过 store 的 JSON 往返）", v)
		}
		if string(raw) != `{"input_tokens":2}` {
			t.Errorf("usage_raw = %s，期望原样 %s", raw, `{"input_tokens":2}`)
		}
	})
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestUnreportedHasHelper 是 unreportedHas 这个纯辅助函数的直接单测。
//
// 它同时是「UnexpectedCold 在 Unreported 含 cache_read 时必须为假」这条
// K 节要求（session.go/record.go 注释、09 F 节「其余字段」）在本包里能够
// 不伪造 provider stdout 就测到的最深一层：09 三份成功样本（claude/grok/agy
// 各自的真实字节）里 usage 键从未整个缺席过，所以「usage 键完全不在，
// Unreported 里出现 cache_read」这个组合，在不自造一份「usage 键缺席」的
// stdout 的前提下，走不到 Runner.Ask 的完整流程里——自造这样一份 JSON
// 正是任务书要求本包避免的事（「stdout 的字节请用真实探针原件，不要自造
// JSON」）。这里退一步，直接测 unreportedHas 本身作为部分替代，
// 完整的端到端组合留在最终报告的测试缺口里点名，不假装已经覆盖。
func TestUnreportedHasHelper(t *testing.T) {
	if !unreportedHas([]string{"cache_create", "cache_read"}, "cache_read") {
		t.Error("unreportedHas 应当在列表含目标项时返回 true")
	}
	if unreportedHas([]string{"cache_create"}, "cache_read") {
		t.Error("unreportedHas 应当在列表不含目标项时返回 false")
	}
	if unreportedHas(nil, "cache_read") {
		t.Error("unreportedHas 对 nil 列表应当返回 false，不应 panic")
	}
}
