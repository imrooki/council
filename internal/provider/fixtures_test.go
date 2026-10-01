package provider_test

// 本文件是全部测试的唯一事实源之一：七份 Step 0 探针原件的字节内容，逐字内联。
//
// 为什么内联而不是读 testdata/ 或 E:\ 绝对路径：09_STEP6_SPEC.md K 节写明
// "测试不得引用 E:\ 绝对路径（D 盘机上没有那个盘）"。本次任务书进一步要求把内容
// 内联成字符串常量而不是仓库内的 testdata 文件。每个常量上方的注释写明它逐字
// 来自哪一个文件；内容按 Read 工具原样返回的字节抄录——不美化、不重排键、
// 不改缩进。claude 与 agy 的三/两份原件是单行紧凑 JSON，grok 的两份是两空格
// 缩进的美化 JSON，这个差异本身就是原件的一部分，保留它。

// claudeT1JSON 逐字来自 E:\<tmp-root>\council\step0-probe\claude-t1.json（冷启动轮）。
const claudeT1JSON = `{"duration_api_ms":1939,"stop_reason":"end_turn","session_id":"2a90d449-a348-497a-8812-e73e74d6898e","total_cost_usd":0.9551999999999999,"usage":{"input_tokens":2,"cache_creation_input_tokens":47749,"cache_read_input_tokens":0,"output_tokens":4,"output_tokens_details":{"thinking_tokens":0},"server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},"service_tier":"standard","cache_creation":{"ephemeral_1h_input_tokens":47749,"ephemeral_5m_input_tokens":0},"inference_geo":"not_available","iterations":[{"input_tokens":2,"output_tokens":4,"cache_read_input_tokens":0,"cache_creation_input_tokens":47749,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":47749},"type":"message","model":null}],"speed":"standard"},"modelUsage":{"claude-fable-5-1":{"inputTokens":2,"outputTokens":4,"cacheReadInputTokens":0,"cacheCreationInputTokens":47749,"webSearchRequests":0,"costUSD":0.9551999999999999,"contextWindow":1000000,"maxOutputTokens":64000,"thinkingTokens":0,"canonicalModel":"claude-fable-5-1","provider":"firstParty","costBasis":"list"}},"permission_denials":[],"terminal_reason":"completed","fast_mode_state":"off","fast_mode_disabled_reason":"sdk_opt_in_required","subagent_stats":{"spawned":0,"requested":{"background":0,"foreground":0,"unset":0},"started_in_background":0,"max_depth":0,"spawned_by_subagents":0,"completed":0,"failed":0,"killed":{"parent":0,"user":0,"system":0},"refused":{"depth_limit":0,"concurrency_limit":0,"budget":0},"by_type":{}},"is_error":false,"num_turns":1,"subtype":"success","api_error_status":null,"result":"OK","ttft_ms":2016,"type":"result","duration_ms":3566,"uuid":"78205525-edf7-485a-b1d4-c9cbccd75462","ttft_stream_ms":1306,"time_to_request_ms":98,"first_content_frame_ms":1307,"queued_turn_count":0,"result_index":0}`

// claudeT2JSON 逐字来自 E:\<tmp-root>\council\step0-probe\claude-t2.json（--resume 第二轮）。
const claudeT2JSON = `{"duration_api_ms":4197,"stop_reason":"end_turn","session_id":"2a90d449-a348-497a-8812-e73e74d6898e","total_cost_usd":1.349789,"usage":{"input_tokens":2,"cache_creation_input_tokens":19363,"cache_read_input_tokens":28436,"output_tokens":4,"output_tokens_details":{"thinking_tokens":0},"server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},"service_tier":"standard","cache_creation":{"ephemeral_1h_input_tokens":19363,"ephemeral_5m_input_tokens":0},"inference_geo":"not_available","iterations":[{"input_tokens":2,"output_tokens":4,"cache_read_input_tokens":28436,"cache_creation_input_tokens":19363,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":19363},"type":"message","model":null}],"speed":"standard"},"modelUsage":{"claude-fable-5-1":{"inputTokens":4,"outputTokens":8,"thinkingTokens":0,"cacheReadInputTokens":28436,"cacheCreationInputTokens":67112,"webSearchRequests":0,"costUSD":1.349789,"contextWindow":1000000,"maxOutputTokens":64000,"canonicalModel":"claude-fable-5-1","provider":"firstParty","costBasis":"list"}},"permission_denials":[],"terminal_reason":"completed","fast_mode_state":"off","fast_mode_disabled_reason":"sdk_opt_in_required","subagent_stats":{"spawned":0,"requested":{"background":0,"foreground":0,"unset":0},"started_in_background":0,"max_depth":0,"spawned_by_subagents":0,"completed":0,"failed":0,"killed":{"parent":0,"user":0,"system":0},"refused":{"depth_limit":0,"concurrency_limit":0,"budget":0},"by_type":{}},"is_error":false,"num_turns":1,"subtype":"success","api_error_status":null,"result":"4217","ttft_ms":2316,"type":"result","duration_ms":3692,"uuid":"e63d159c-00b7-42f8-a437-4a570a2daa18","ttft_stream_ms":1612,"time_to_request_ms":79,"first_content_frame_ms":1612,"queued_turn_count":0,"result_index":0}`

// claudeT3JSON 逐字来自 E:\<tmp-root>\council\step0-probe\claude-t3.json（--resume 第三轮）。
const claudeT3JSON = `{"duration_api_ms":6467,"stop_reason":"end_turn","session_id":"2a90d449-a348-497a-8812-e73e74d6898e","total_cost_usd":1.3638469999999998,"usage":{"input_tokens":2,"cache_creation_input_tokens":95,"cache_read_input_tokens":47752,"output_tokens":4,"output_tokens_details":{"thinking_tokens":0},"server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},"service_tier":"standard","cache_creation":{"ephemeral_1h_input_tokens":95,"ephemeral_5m_input_tokens":0},"inference_geo":"not_available","iterations":[{"input_tokens":2,"output_tokens":4,"cache_read_input_tokens":47752,"cache_creation_input_tokens":95,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":95},"type":"message","model":null}],"speed":"standard"},"modelUsage":{"claude-fable-5-1":{"inputTokens":6,"outputTokens":12,"thinkingTokens":0,"cacheReadInputTokens":76188,"cacheCreationInputTokens":67207,"webSearchRequests":0,"costUSD":1.3638469999999998,"contextWindow":1000000,"maxOutputTokens":64000,"canonicalModel":"claude-fable-5-1","provider":"firstParty","costBasis":"list"}},"permission_denials":[],"terminal_reason":"completed","fast_mode_state":"off","fast_mode_disabled_reason":"sdk_opt_in_required","subagent_stats":{"spawned":0,"requested":{"background":0,"foreground":0,"unset":0},"started_in_background":0,"max_depth":0,"spawned_by_subagents":0,"completed":0,"failed":0,"killed":{"parent":0,"user":0,"system":0},"refused":{"depth_limit":0,"concurrency_limit":0,"budget":0},"by_type":{}},"is_error":false,"num_turns":1,"subtype":"success","api_error_status":null,"result":"4218","ttft_ms":2326,"type":"result","duration_ms":3812,"uuid":"6fe7529a-ae67-4627-8ca8-21bae84259d1","ttft_stream_ms":1636,"time_to_request_ms":79,"first_content_frame_ms":1636,"queued_turn_count":0,"result_index":0}`

// grokT1JSON 逐字来自 E:\<tmp-root>\council\step0-probe\grok-t1.json（冷启动轮）。
// 原件是两空格缩进的美化 JSON，逐行照抄，包括 thought 字段里的转义引号与 \n\n\n。
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
}
`

// grokT2JSON 逐字来自 E:\<tmp-root>\council\step0-probe\grok-t2.json（--resume 第二轮）。
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
}
`

// agyT1JSON 逐字来自 E:\<tmp-root>\council\step0-probe\agy-t1.json（冷启动轮）。
// response 字段带尾换行 "OK\n"，这是陷阱 9 的原始证据，不要手动去掉它。
const agyT1JSON = `{"conversation_id":"e484a528-68c6-4f30-ac0f-aa69c1a06ede","status":"SUCCESS","response":"OK\n","duration_seconds":3.3927399,"num_turns":1,"usage":{"input_tokens":18180,"output_tokens":55,"thinking_tokens":54,"cache_read_tokens":0,"total_tokens":18235}}`

// agyT2JSON 逐字来自 E:\<tmp-root>\council\step0-probe\agy-t2.json（--conversation 第二轮）。
const agyT2JSON = `{"conversation_id":"e484a528-68c6-4f30-ac0f-aa69c1a06ede","status":"SUCCESS","response":"5192\n","duration_seconds":24.0565513,"num_turns":2,"usage":{"input_tokens":20348,"output_tokens":134,"thinking_tokens":129,"cache_read_tokens":16282,"total_tokens":20482}}`

// ---- Step 6.0 探针第二轮：真实失败样本（2026-09-20 补测，故意传一个不存在的 model 名） ----
//
// Step 0 的七份原件全是成功轮，A 节探针 1（"provider 自报失败"的判据）在写完上面
// 那批常量时仍是 [未验证]。这三份是后补的真实失败样本，本机直接核对过与源文件
// 逐字节一致：
//   E:\<tmp-root>\council\step6-probe\claude-badmodel.out（exits3.txt: A_EXIT=1）
//   E:\<tmp-root>\council\step6-probe\grok-badmodel.out（exits4.txt: G_EXIT=1）
//   E:\<tmp-root>\council\step6-probe\agy-badmodel.out（exits4.txt: A_EXIT=1）
// 三家退出码都是 1。agy 的 error 字段内容较长（列了一份可用模型清单），逐字节抄自
// 文件本身，不是转述。

// claudeBadModelJSON 逐字来自 claude-badmodel.out。
//
// 这是 claude 失败判据里最值钱的一条反例：subtype 单独看是 "success"，
// 但五字段合取里另外四项（is_error=true / terminal_reason="api_error" 而非
// "completed" / api_error_status=404 而非 null / stop_reason="stop_sequence"
// 而非 end_turn|max_tokens）全部不过关。只看 subtype 会把这份失败原件误判成功。
// modelUsage 是 {}（0 个键），usage 的五个数值字段全是真实的 0（这次调用确实
// 没跑到任何 token），不是"provider 没报"。
const claudeBadModelJSON = `{"duration_api_ms":0,"stop_reason":"stop_sequence","session_id":"9568d2d7-12d7-4882-af00-7719e04da6bf","total_cost_usd":0,"usage":{"output_tokens_details":{"thinking_tokens":0},"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":0,"server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},"service_tier":"standard","cache_creation":{"ephemeral_1h_input_tokens":0,"ephemeral_5m_input_tokens":0},"inference_geo":"","iterations":[],"speed":"standard"},"modelUsage":{},"permission_denials":[],"terminal_reason":"api_error","fast_mode_state":"off","fast_mode_disabled_reason":"sdk_opt_in_required","subagent_stats":{"spawned":0,"requested":{"background":0,"foreground":0,"unset":0},"started_in_background":0,"max_depth":0,"spawned_by_subagents":0,"completed":0,"failed":0,"killed":{"parent":0,"user":0,"system":0},"refused":{"depth_limit":0,"concurrency_limit":0,"budget":0},"by_type":{}},"is_error":true,"num_turns":1,"subtype":"success","api_error_status":404,"result":"There's an issue with the selected model (no-such-model-xyz-9). It may not exist or you may not have access to it. Run --model to pick a different model.","type":"result","duration_ms":1004,"uuid":"f3ccf7a3-30ad-4b3f-967f-5e7efe4d97ae","queued_turn_count":0,"result_index":0}`

// grokBadModelJSON 逐字来自 grok-badmodel.out。
//
// 与 Step 0 七份原件（甚至与 claude / agy 的失败样本）都不同的一种形状：没有
// stopReason / sessionId / usage 中的任何一个，只有一对 type/message。grok.go
// 现在的注释写着"grok 连一个错误字段都没有"，这份原件证明这句话在 2026-09-20
// 之后不成立了：type=="error" 是一个可识别的失败判据。
const grokBadModelJSON = `{"type":"error","message":"Couldn't set model 'no-such-grok-model-9': Invalid params: \"unknown model id\". Run 'grok models' to see available models."}`

// agyBadModelJSON 逐字来自 agy-badmodel.out。
//
// conversation_id 是空串、status="ERROR"、response 是空串，外加一个新的 error
// 字段（agyOutput 当前没有这个 json tag）。usage 的五个数值字段全是真实的 0。
const agyBadModelJSON = `{"conversation_id":"","status":"ERROR","response":"","error":"invalid model selection (--model \"No Such Model (High)\" --effort \"\"): model No Such Model (High) is not recognized as a known model or custom model in settings\nAvailable models:\n  Gemini 3.8 Flash (High)\n  Gemini 3.8 Flash (Medium)\n  Gemini 3.8 Flash (Low)\n  Gemini 3.7 Flash (High)\n  Gemini 3.7 Flash (Medium)\n  Gemini 3.7 Flash (Low)\n  Gemini 3.6 Flash (High)\n  Gemini 3.6 Flash (Medium)\n  Gemini 3.6 Flash (Low)\n  Gemini 3.1 Pro (High)\n  Gemini 3.1 Pro (Low)\n  Claude Sonnet 4.6 (Thinking)\n  Claude Opus 4.6 (Thinking)\n  GPT-OSS 120B (Medium)","duration_seconds":0,"num_turns":0,"usage":{"input_tokens":0,"output_tokens":0,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":0}}`

// agyDeniedActionJSON 逐字来自 2026-09-21 复核原件
// E:\<tmp-root>\council\oq-agy-ws\rerun\raw.json：agy 1.2.7 在 WORKSPACE
// 档里被要求做一件界内的事（建 INSIDE.txt）加一件越界的事（建父目录下的
// OUTSIDE.txt）时的真实输出。status 仍是 SUCCESS，response 被整段清空，
// denied_actions 是一个数组（哪怕只有一个元素）——11c 契约第 7 节记的三层
// 机制的第二、三层就是这份原件坐实的。复核时目录里确实多出了 INSIDE.txt，
// 父目录没有 OUTSIDE.txt（第一层：界内能写；边界是真的）。
const agyDeniedActionJSON = `{"conversation_id":"835f4ebf-727b-4695-a1ee-f0a60ac26240","status":"SUCCESS","response":"","duration_seconds":9.8952514,"num_turns":1,"usage":{"input_tokens":16449,"output_tokens":1150,"thinking_tokens":980,"cache_read_tokens":0,"total_tokens":17599},"denied_actions":[{"action":"write_file","display_name":"WriteToFile"}]}`
