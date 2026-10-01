package turn

// 本文件是全部 internal/turn 测试共用的原件、假实现与建造函数。
//
// 原件说明（任务书硬性要求：不逐字节改，注释注明来源文件，测试本身不读 E 盘）：
//   成功样本来自 E:\<tmp-root>\council\step0-probe\{claude,grok,agy}-t{1,2}.json
//   （claude 另有 t3），解读见同目录 STEP0-RESULTS.md。
//   失败样本来自 E:\<tmp-root>\council\step6-probe\{claude,grok,agy}-badmodel.out，
//   解读见同目录 STEP6-PROBE-RESULTS.md 与 docs/09_STEP6_SPEC.md A 节「探针 1 闭合后
//   得到了什么」。三份都是给一个不存在的 model 名触发，三家退出码都是 1。
//
// internal/provider 那边的执行者在写自己包的测试时会内联同一批原件，
// 各包各内联各的，不共享、不跨包引用（任务书原话）。

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"council/internal/config"
	"council/internal/conversation"
	"council/internal/provider"
	"council/internal/store"
)

// ---------------------------------------------------------------------------
// 成功样本
// ---------------------------------------------------------------------------

// claudeT1JSON 来自 step0-probe\claude-t1.json。冷启动首轮：
// cache_creation_input_tokens=47749、cache_read_input_tokens=0、
// total_cost_usd=0.9551999999999999，session_id 三轮恒为
// 2a90d449-a348-497a-8812-e73e74d6898e。
const claudeT1JSON = `{"duration_api_ms":1939,"stop_reason":"end_turn","session_id":"2a90d449-a348-497a-8812-e73e74d6898e","total_cost_usd":0.9551999999999999,"usage":{"input_tokens":2,"cache_creation_input_tokens":47749,"cache_read_input_tokens":0,"output_tokens":4,"output_tokens_details":{"thinking_tokens":0},"server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},"service_tier":"standard","cache_creation":{"ephemeral_1h_input_tokens":47749,"ephemeral_5m_input_tokens":0},"inference_geo":"not_available","iterations":[{"input_tokens":2,"output_tokens":4,"cache_read_input_tokens":0,"cache_creation_input_tokens":47749,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":47749},"type":"message","model":null}],"speed":"standard"},"modelUsage":{"claude-fable-5-1":{"inputTokens":2,"outputTokens":4,"cacheReadInputTokens":0,"cacheCreationInputTokens":47749,"webSearchRequests":0,"costUSD":0.9551999999999999,"contextWindow":1000000,"maxOutputTokens":64000,"thinkingTokens":0,"canonicalModel":"claude-fable-5-1","provider":"firstParty","costBasis":"list"}},"permission_denials":[],"terminal_reason":"completed","fast_mode_state":"off","fast_mode_disabled_reason":"sdk_opt_in_required","subagent_stats":{"spawned":0,"requested":{"background":0,"foreground":0,"unset":0},"started_in_background":0,"max_depth":0,"spawned_by_subagents":0,"completed":0,"failed":0,"killed":{"parent":0,"user":0,"system":0},"refused":{"depth_limit":0,"concurrency_limit":0,"budget":0},"by_type":{}},"is_error":false,"num_turns":1,"subtype":"success","api_error_status":null,"result":"OK","ttft_ms":2016,"type":"result","duration_ms":3566,"uuid":"78205525-edf7-485a-b1d4-c9cbccd75462","ttft_stream_ms":1306,"time_to_request_ms":98,"first_content_frame_ms":1307,"queued_turn_count":0,"result_index":0}`

// claudeT2JSON 来自 step0-probe\claude-t2.json。同一会话第二轮：
// cache_creation_input_tokens=19363、cache_read_input_tokens=28436，
// total_cost_usd=1.349789（会话累计值，不是单轮差值）。
const claudeT2JSON = `{"duration_api_ms":4197,"stop_reason":"end_turn","session_id":"2a90d449-a348-497a-8812-e73e74d6898e","total_cost_usd":1.349789,"usage":{"input_tokens":2,"cache_creation_input_tokens":19363,"cache_read_input_tokens":28436,"output_tokens":4,"output_tokens_details":{"thinking_tokens":0},"server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},"service_tier":"standard","cache_creation":{"ephemeral_1h_input_tokens":19363,"ephemeral_5m_input_tokens":0},"inference_geo":"not_available","iterations":[{"input_tokens":2,"output_tokens":4,"cache_read_input_tokens":28436,"cache_creation_input_tokens":19363,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":19363},"type":"message","model":null}],"speed":"standard"},"modelUsage":{"claude-fable-5-1":{"inputTokens":4,"outputTokens":8,"thinkingTokens":0,"cacheReadInputTokens":28436,"cacheCreationInputTokens":67112,"webSearchRequests":0,"costUSD":1.349789,"contextWindow":1000000,"maxOutputTokens":64000,"canonicalModel":"claude-fable-5-1","provider":"firstParty","costBasis":"list"}},"permission_denials":[],"terminal_reason":"completed","fast_mode_state":"off","fast_mode_disabled_reason":"sdk_opt_in_required","subagent_stats":{"spawned":0,"requested":{"background":0,"foreground":0,"unset":0},"started_in_background":0,"max_depth":0,"spawned_by_subagents":0,"completed":0,"failed":0,"killed":{"parent":0,"user":0,"system":0},"refused":{"depth_limit":0,"concurrency_limit":0,"budget":0},"by_type":{}},"is_error":false,"num_turns":1,"subtype":"success","api_error_status":null,"result":"4217","ttft_ms":2316,"type":"result","duration_ms":3692,"uuid":"e63d159c-00b7-42f8-a437-4a570a2daa18","ttft_stream_ms":1612,"time_to_request_ms":79,"first_content_frame_ms":1612,"queued_turn_count":0,"result_index":0}`

// claudeT3JSON 来自 step0-probe\claude-t3.json。同一会话第三轮，验收标尺：
// cache_creation_input_tokens=95（09 G 节「验收标尺只用 T3」：必须是 O(95)），
// cache_read_input_tokens=47752，total_cost_usd=1.3638469999999998。
const claudeT3JSON = `{"duration_api_ms":6467,"stop_reason":"end_turn","session_id":"2a90d449-a348-497a-8812-e73e74d6898e","total_cost_usd":1.3638469999999998,"usage":{"input_tokens":2,"cache_creation_input_tokens":95,"cache_read_input_tokens":47752,"output_tokens":4,"output_tokens_details":{"thinking_tokens":0},"server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},"service_tier":"standard","cache_creation":{"ephemeral_1h_input_tokens":95,"ephemeral_5m_input_tokens":0},"inference_geo":"not_available","iterations":[{"input_tokens":2,"output_tokens":4,"cache_read_input_tokens":47752,"cache_creation_input_tokens":95,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":95},"type":"message","model":null}],"speed":"standard"},"modelUsage":{"claude-fable-5-1":{"inputTokens":6,"outputTokens":12,"thinkingTokens":0,"cacheReadInputTokens":76188,"cacheCreationInputTokens":67207,"webSearchRequests":0,"costUSD":1.3638469999999998,"contextWindow":1000000,"maxOutputTokens":64000,"canonicalModel":"claude-fable-5-1","provider":"firstParty","costBasis":"list"}},"permission_denials":[],"terminal_reason":"completed","fast_mode_state":"off","fast_mode_disabled_reason":"sdk_opt_in_required","subagent_stats":{"spawned":0,"requested":{"background":0,"foreground":0,"unset":0},"started_in_background":0,"max_depth":0,"spawned_by_subagents":0,"completed":0,"failed":0,"killed":{"parent":0,"user":0,"system":0},"refused":{"depth_limit":0,"concurrency_limit":0,"budget":0},"by_type":{}},"is_error":false,"num_turns":1,"subtype":"success","api_error_status":null,"result":"4218","ttft_ms":2326,"type":"result","duration_ms":3812,"uuid":"6fe7529a-ae67-4627-8ca8-21bae84259d1","ttft_stream_ms":1636,"time_to_request_ms":79,"first_content_frame_ms":1636,"queued_turn_count":0,"result_index":0}`

// grokT1JSON 来自 step0-probe\grok-t1.json。冷启动首轮：cache_read_input_tokens=0，
// sessionId=c0117e5a-4b21-4d3f-9f8e-2a6b1d0e7a31（本测试文件里下面的
// grokBuiltSessionID 与它不同——grok 首轮由 BuildTurn 现铸一个 uuid，CLI 原样回显的
// 是这个 c0117e5a 开头的值，两者对不上正是本包 Drifted 检测要抓的那类漂移，
// 用真实字节就能免费构造，见 session_test.go）。
const grokT1JSON = `{
  "text": "OK",
  "stopReason": "end_turn",
  "sessionId": "c0117e5a-4b21-4d3f-9f8e-2a6b1d0e7a31",
  "requestId": "9582f1ad-3946-401e-8d0c-9315b94c86fe",
  "thought": "The user wants me to remember the number 8635 and reply with exactly \"OK\". There's also a user rule that says the first sentence of my reply must be \"收到，维护者。\"\n\n\n",
  "usage": {
    "input_tokens": 20625,
    "cache_read_input_tokens": 0,
    "cache_creation_input_tokens": 0,
    "output_tokens": 723,
    "reasoning_tokens": 722,
    "total_tokens": 21348
  },
  "num_turns": 1,
  "modelUsage": {
    "grok-4.6-build": {
      "inputTokens": 20625,
      "outputTokens": 723,
      "cacheReadInputTokens": 0,
      "cacheCreationInputTokens": 0,
      "modelCalls": 1
    }
  }
}`

// grokT2JSON 来自 step0-probe\grok-t2.json。同一会话第二轮，
// cache_read_input_tokens=20608（陷阱 7：cache_creation_input_tokens 仍是 0，
// 不能拿它反推冷暖）。sessionId 与 t1 相同，--resume 得上。
const grokT2JSON = `{
  "text": "8635",
  "stopReason": "end_turn",
  "sessionId": "c0117e5a-4b21-4d3f-9f8e-2a6b1d0e7a31",
  "requestId": "0cd23532-5783-408f-83bf-6c0cd67c1601",
  "thought": "The user asked me to remember the number 8635 and reply with just the digits. There's also a user rule that says the first sentence of every response after receiving instructions must be \"收到，维护者。\"...\n\n",
  "usage": {
    "input_tokens": 250,
    "cache_read_input_tokens": 20608,
    "cache_creation_input_tokens": 0,
    "output_tokens": 359,
    "reasoning_tokens": 357,
    "total_tokens": 21217
  },
  "num_turns": 1,
  "modelUsage": {
    "grok-4.6-build": {
      "inputTokens": 250,
      "outputTokens": 359,
      "cacheReadInputTokens": 20608,
      "cacheCreationInputTokens": 0,
      "modelCalls": 1
    }
  }
}`

// agyT1JSON 来自 step0-probe\agy-t1.json。response 带尾换行 "OK\n"，
// cache_read_tokens=0（键名没有 "_input_"，陷阱 3）。
const agyT1JSON = `{"conversation_id":"e484a528-68c6-4f30-ac0f-aa69c1a06ede","status":"SUCCESS","response":"OK\n","duration_seconds":3.3927399,"num_turns":1,"usage":{"input_tokens":18180,"output_tokens":55,"thinking_tokens":54,"cache_read_tokens":0,"total_tokens":18235}}`

// agyT2JSON 来自 step0-probe\agy-t2.json。cache_read_tokens=16282 < input_tokens
// 20348，证明 agy 的 input_tokens 已经含缓存命中（陷阱 2）。
const agyT2JSON = `{"conversation_id":"e484a528-68c6-4f30-ac0f-aa69c1a06ede","status":"SUCCESS","response":"5192\n","duration_seconds":24.0565513,"num_turns":2,"usage":{"input_tokens":20348,"output_tokens":134,"thinking_tokens":129,"cache_read_tokens":16282,"total_tokens":20482}}`

// ---------------------------------------------------------------------------
// 失败样本（2026-09-21 探针 1，触发方式都是给一个不存在的 model 名）
// ---------------------------------------------------------------------------

// claudeBadModelOut 来自 step6-probe\claude-badmodel.out。
// 最值钱的一条：subtype 仍是 "success"，只有 is_error/terminal_reason/
// api_error_status/stop_reason 四道独立拦下这份样本（09 A 节）。
const claudeBadModelOut = `{"duration_api_ms":0,"stop_reason":"stop_sequence","session_id":"9568d2d7-12d7-4882-af00-7719e04da6bf","total_cost_usd":0,"usage":{"output_tokens_details":{"thinking_tokens":0},"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":0,"server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},"service_tier":"standard","cache_creation":{"ephemeral_1h_input_tokens":0,"ephemeral_5m_input_tokens":0},"inference_geo":"","iterations":[],"speed":"standard"},"modelUsage":{},"permission_denials":[],"terminal_reason":"api_error","fast_mode_state":"off","fast_mode_disabled_reason":"sdk_opt_in_required","subagent_stats":{"spawned":0,"requested":{"background":0,"foreground":0,"unset":0},"started_in_background":0,"max_depth":0,"spawned_by_subagents":0,"completed":0,"failed":0,"killed":{"parent":0,"user":0,"system":0},"refused":{"depth_limit":0,"concurrency_limit":0,"budget":0},"by_type":{}},"is_error":true,"num_turns":1,"subtype":"success","api_error_status":404,"result":"There's an issue with the selected model (no-such-model-xyz-9). It may not exist or you may not have access to it. Run --model to pick a different model.","type":"result","duration_ms":1004,"uuid":"f3ccf7a3-30ad-4b3f-967f-5e7efe4d97ae","queued_turn_count":0,"result_index":0}`

// grokBadModelOut 来自 step6-probe\grok-badmodel.out。grok 的错误信封，
// 成功形状的五个字段（stopReason/sessionId/text/usage/modelUsage）全是零值。
const grokBadModelOut = `{"type":"error","message":"Couldn't set model 'no-such-grok-model-9': Invalid params: \"unknown model id\". Run 'grok models' to see available models."}`

// agyBadModelOut 来自 step6-probe\agy-badmodel.out。失败正文在 error 键里，
// status="ERROR"，conversation_id 与 response 都是空串。
const agyBadModelOut = `{"conversation_id":"","status":"ERROR","response":"","error":"invalid model selection (--model \"No Such Model (High)\" --effort \"\"): model No Such Model (High) is not recognized as a known model or custom model in settings\nAvailable models:\n  Gemini 3.8 Flash (High)\n  Gemini 3.8 Flash (Medium)\n  Gemini 3.8 Flash (Low)\n  Gemini 3.7 Flash (High)\n  Gemini 3.7 Flash (Medium)\n  Gemini 3.7 Flash (Low)\n  Gemini 3.6 Flash (High)\n  Gemini 3.6 Flash (Medium)\n  Gemini 3.6 Flash (Low)\n  Gemini 3.1 Pro (High)\n  Gemini 3.1 Pro (Low)\n  Claude Sonnet 4.6 (Thinking)\n  Claude Opus 4.6 (Thinking)\n  GPT-OSS 120B (Medium)","duration_seconds":0,"num_turns":0,"usage":{"input_tokens":0,"output_tokens":0,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":0}}`

// agyDeniedActionJSON 来自 2026-09-21 复核原件
// E:\<tmp-root>\council\oq-agy-ws\rerun\raw.json：agy 在 WORKSPACE 档
// 被要求做一件界内的事加一件越界的事时的真实输出，status 仍是 SUCCESS，
// response 被整段清空，denied_actions 是一个数组（11c 契约第 7 节）。
const agyDeniedActionJSON = `{"conversation_id":"835f4ebf-727b-4695-a1ee-f0a60ac26240","status":"SUCCESS","response":"","duration_seconds":9.8952514,"num_turns":1,"usage":{"input_tokens":16449,"output_tokens":1150,"thinking_tokens":980,"cache_read_tokens":0,"total_tokens":17599},"denied_actions":[{"action":"write_file","display_name":"WriteToFile"}]}`

// claudeNotFoundResumeStderr 是 09 A 节「探针推翻了什么」里原样引用的真实 stderr
// 文本：claude --resume <不存在的id> 时 stdout 是 0 字节，错误只在 stderr 的纯文本里。
// 用它当"stdout 不是 JSON"这条负路径的输入——它不是我们编的假字符串，是探针记录下的
// 真实观测（docs/09_STEP6_SPEC.md A 节 / docs/05_MESSAGE_PROTOCOL.md 第 3 节陷阱 7）。
const claudeNotFoundResumeStderr = `No conversation found with session ID: 00000000-0000-0000-0000-000000000000`

// ---------------------------------------------------------------------------
// 假 Spawner：整包测试不需要真的起任何进程。
// ---------------------------------------------------------------------------

// spawnResult 是一次 Spawner 调用要回放的结果。
type spawnResult struct {
	stdout   []byte
	stderr   []byte
	exitCode int
	killed   bool
	err      error
}

// spawnRecord 记下一次真实发生的调用，供子进程契约类的断言使用。
type spawnRecord struct {
	ctx context.Context
	cmd provider.Command
}

// fakeSpawner 是 Spawner 的测试替身：不起任何进程，从一个先进先出的队列里
// 吐预先准备好的结果，同时把每次调用的参数记下来。
//
// 计数器（len(calls)）是「起进程之前就拒绝」那组用例的核心断言：
// 09 G 节第 9 行明写「失败要在花钱之前失败」，验证它的唯一办法就是证明
// Spawner 一次都没被调用过。
type fakeSpawner struct {
	calls   []spawnRecord
	results []spawnResult
}

func (f *fakeSpawner) enqueue(r spawnResult) {
	f.results = append(f.results, r)
}

// enqueueSuccess 是最常用的一支：退出码 0，没有 stderr，没有被杀。
func (f *fakeSpawner) enqueueSuccess(stdout string) {
	f.enqueue(spawnResult{stdout: []byte(stdout), exitCode: 0})
}

func (f *fakeSpawner) spawn(ctx context.Context, c provider.Command) ([]byte, []byte, int, bool, error) {
	f.calls = append(f.calls, spawnRecord{ctx: ctx, cmd: c})
	if len(f.results) == 0 {
		// 队列空了还被调用，说明某条用例的假设错了——测试要立刻看见这个失败，
		// 而不是拿到一个安静的零值，看起来像「碰巧」通过。
		return nil, nil, 0, false, errNoMoreCannedResponses
	}
	r := f.results[0]
	f.results = f.results[1:]
	return r.stdout, r.stderr, r.exitCode, r.killed, r.err
}

// errNoMoreCannedResponses 只在测试自己的假设翻车时出现，不属于任何契约。
var errNoMoreCannedResponses = errors.New("fakeSpawner: queue exhausted, this test asked for more spawns than it enqueued results for")

// ---------------------------------------------------------------------------
// 可控时钟
// ---------------------------------------------------------------------------

// fakeClock 让 CacheTTL / ColdExpected 这类依赖「现在几点」的判据变得可控。
type fakeClock struct {
	t time.Time
}

func newFakeClock(t time.Time) *fakeClock {
	return &fakeClock{t: t}
}

func (c *fakeClock) now() time.Time { return c.t }

func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// ---------------------------------------------------------------------------
// 配置与持久化建造函数
// ---------------------------------------------------------------------------

// testAgentClaude / testAgentGrok / testAgentAgy 是三条固定的测试 agent id，
// 分别挂在三家 provider 上，好让同一套建造函数能覆盖 EffortPerTurnFlag
// （claude / agy）与 EffortProcessExternalConfig（grok）两条身份指纹分支。
const (
	testAgentClaude = "alice"
	testAgentGrok   = "zulu"
	testAgentAgy    = "nova"
)

// newTestConfig 造一份三个 agent（一家 provider 一个）的最小配置，权限档
// 全部是 read——09 permission.go 的裁决是 Step 6 只放行这一档，用别的档
// 会在起进程之前就被拒绝，不是本函数要覆盖的场景。
func newTestConfig() *config.Config {
	return &config.Config{
		Providers: map[string]config.ProviderConfig{
			"claude": {Command: "claude-fake"},
			"grok":   {Command: "grok-fake"},
			"agy":    {Command: "agy-fake"},
		},
		Agents: map[string]config.AgentConfig{
			testAgentClaude: {
				DisplayName: "Alice",
				Provider:    "claude",
				Model:       "claude-fable-5-1",
				Effort:      "high",
				Role:        "只做结构力学审阅",
				Permissions: config.PermissionRead,
				Mode:        config.ModeDiscussion,
			},
			testAgentGrok: {
				DisplayName: "Zulu",
				Provider:    "grok",
				Model:       "grok-4.6",
				Effort:      "high",
				Role:        "只做数值验算",
				Permissions: config.PermissionRead,
				Mode:        config.ModeDiscussion,
			},
			testAgentAgy: {
				DisplayName: "Nova",
				Provider:    "agy",
				Model:       "gemini-3.8-flash-high",
				Role:        "只做代码走查",
				Permissions: config.PermissionRead,
				Mode:        config.ModeDiscussion,
			},
		},
	}
}

// newTestService 在 t.TempDir() 下开一个真实的 SQLite store，包一层
// conversation.Service 交给测试用。任务书要求 internal/turn 的测试不能像
// conversation 包那样自己造一个 fakeStore（那个类型是 conversation 包内部的，
// 跨包访问不到），所以照 internal/store 现有测试的样子，用真库
// （见 internal/store/open_test.go 的 tstStore 系列辅助函数）。
func newTestService(t *testing.T) *conversation.Service {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "council-data")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatalf("store.Open(%q) 失败: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("关闭测试 store 失败: %v", err)
		}
	})
	return conversation.NewService(st)
}

// newConv 建一个新会话，返回它的 id。
func newConv(t *testing.T, svc *conversation.Service) string {
	t.Helper()
	conv, err := svc.Create(context.Background(), "turn 包测试会话")
	if err != nil {
		t.Fatalf("Create 会话失败: %v", err)
	}
	return conv.ID
}

// joinAgent 把 agentID 加入 convID，provider 只是标签，Ask 不读这个字段
// （它只读 cfg.Agents），随便填一个非空值即可。
func joinAgent(t *testing.T, svc *conversation.Service, convID, agentID string) {
	t.Helper()
	err := svc.Join(context.Background(), convID, conversation.Participant{
		AgentID:  agentID,
		Provider: "test",
		JoinedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Join %q 失败: %v", agentID, err)
	}
}

// postUser 用保留 sender "user" 发一条消息，不需要先 join。
func postUser(t *testing.T, svc *conversation.Service, convID, text string) conversation.Message {
	t.Helper()
	m, err := svc.Post(context.Background(), conversation.Message{
		ConvID:   convID,
		SenderID: conversation.ReservedSenderUser,
		Kind:     conversation.KindUser,
		Text:     text,
	})
	if err != nil {
		t.Fatalf("postUser(%q) 失败: %v", text, err)
	}
	return m
}

// newTestRunner 造一个 Runner，工作目录是 t.TempDir() 之下的一个干净子目录
// （与生产路径一致：workDir 是 Council 自己的目录，不是进程启动目录）。
// 调用方自己按需替换 spawn / now 字段——这是本包能不依赖任何真实 CLI 的
// 唯一原因（runner.go 顶部注释：「抽成字段是为了测试能塞一个回放探针 JSON
// 的假实现」）。
func newTestRunner(t *testing.T, cfg *config.Config, svc *conversation.Service) *Runner {
	t.Helper()
	workDir := filepath.Join(t.TempDir(), "workdir")
	// 默认空规则：绝大多数用例验的不是规则文件，而空规则必须让身份块与
	// 指纹与加规则之前完全一样，正好是这些既有用例要继续成立的前提。
	// 要验规则的用例自己构造 Runner 或改 r.rules。
	return NewRunner(cfg, svc, workDir, RulesBlocks{})
}

// newTestRunnerSameWorkDir 造一个与 like 共用 workDir 的新 Runner，
// gates 仍是全新的空 map。给"模拟重启"的用例用：真实重启是同一个进程
// 用同一份配置（同一个 data_dir，从而同一条 workDir）重新起来，gates 是空的
// 但库还是那个库；用 newTestRunner 各自现铸一个 t.TempDir() 会让指纹第七段
// （PrefixArgsDigest 依赖 WorkDir）在两个 Runner 之间对不上，把「同一进程
// 重启」误判成「配置真的变了」。
func newTestRunnerSameWorkDir(like *Runner, cfg *config.Config, svc *conversation.Service) *Runner {
	return NewRunner(cfg, svc, like.workDir, RulesBlocks{})
}

// fingerprintFor 直接调 identityFPFor——与 Ask/AuditDrift 唯一的指纹计算
// 入口同一份代码——给「白盒预置这条 (会话, 成员) 的 gate.st 或库里一行」的
// 用例用：预置的 identityFP 必须与 Ask 真正算出来的一致，否则用例会在还没
// 走到想测的分支之前就先撞上 ErrIdentityDrift，测的就不是它自称要测的东西了。
//
// convID 必须与调用方后续 Ask(ctx, convID, agentID) 用的同一个：11c 契约
// 第 5 节之后，WORKSPACE 档的工作目录含 convID，传错的 convID 会让预置的
// 指纹与 Ask 真正算出来的对不上。本包既有用例的 agent 全是 read 档，
// agentWorkDir 对 read 忽略 convID，所以传什么都不影响这些用例的结果，
// 但签名仍然要求调用方原样传，免得将来有用例换成 workspace 档时踩到这个坑。
//
// 必须传 r 本身而不是重新造一个 workDir：指纹第七段（PrefixArgsDigest）
// 现在依赖 WorkDir（claude 的身份文件路径、grok 的 --cwd、agy 的
// --add-dir 都把它塞进 Args），若这里另起一个 t.TempDir()，算出来的指纹会
// 与 r.workDir 对应的真实指纹不同，用例会在还没走到想测的分支之前就被一次
// 「命令行」漂移挡下——而这次漂移只是测试自己的两个临时目录不一样，
// 不是任何真实配置变化。
func fingerprintFor(t *testing.T, r *Runner, convID, agentID string, ac config.AgentConfig) string {
	t.Helper()
	p, err := provider.Lookup(ac.Provider)
	if err != nil {
		t.Fatalf("provider.Lookup(%q) 失败: %v", ac.Provider, err)
	}
	tier, err := TranslatePermission(ac.Permissions)
	if err != nil {
		t.Fatalf("TranslatePermission(%q) 失败: %v", ac.Permissions, err)
	}
	fp, err := identityFPFor(p, agentID, ac, r.workDir, convID, tier, r.rules)
	if err != nil {
		t.Fatalf("identityFPFor(%q) 失败: %v", agentID, err)
	}
	return fp
}
