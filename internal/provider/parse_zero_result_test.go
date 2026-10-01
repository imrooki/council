package provider_test

// 本文件钉住冻结契约点 (4) 的不变量——它是 internal/turn 那边「解析失败 ->
// UsageParsed 保持 false -> 七列 NULL」整条判据的地基：
//
//   三家 ParseTurn（claude / grok / agy）返回零值 TurnResult{}，
//   当且仅当 error 匹配 provider.ErrUnparseableOutput；
//   ErrTurnFailed 与 ErrNoSessionID 两支恒返回已经填好 Usage 的 base。
//
// 既有 TestParseTurn_UnknownShape（parse_test.go:594）只断言了 error 是
// ErrUnparseableOutput，没有断言返回值是零值——这正是任务书点名的缺口：
// 如果哪天某个适配器在这一支偷偷塞回了一星半点数据（哪怕只是 Completion
// 或 StopRaw 这类看起来无害的字段），internal/turn 那边「没解析过就必须
// 七列 NULL」的判据就会被这个「半成品」污染，把一份实际不完整的数据误当
// 成「已解析」，而 TestParseTurn_UnknownShape 本身测不出来——它只看 err。
//
// 本文件补上「返回值是零值」这半句，并补上两条反面：ErrTurnFailed 与
// ErrNoSessionID 两支都必须返回非零值、Usage 已经填好，不能因为「这一轮
// 失败了/会话 id 缺席」就顺手也退化成零值——那会让真实花过的钱在
// internal/turn 那边被误判成「没解析过」而写成 NULL，正是背景段落描述的
// 那类 bug 的反面版本：把「实测为 0」误当成「不知道」。

import (
	"errors"
	"reflect"
	"testing"

	"council/internal/provider"
)

// TestParseTurn_UnparseableOutputReturnsZeroResult 补 TestParseTurn_UnknownShape
// 的另外半条腿：认不出形状时不仅 error 要匹配 ErrUnparseableOutput，返回的
// TurnResult 本身必须逐字段是零值。用 reflect.DeepEqual 跟 provider.TurnResult{}
// 整体比较，而不是挑几个字段断言 == ""/nil——逐字段挑着断言的话，将来这个
// struct 加新字段时，一条只检查旧字段的测试会安静地漏掉新字段被污染的情况；
// 整体 DeepEqual 不会有这个盲区。
func TestParseTurn_UnparseableOutputReturnsZeroResult(t *testing.T) {
	stdouts := map[string]string{
		"empty":           "",
		"whitespace_only": "   \n\t  ",
		"not_json":        "not json {",
		"empty_object":    "{}",
		"unrelated_shape": `{"hello":"world"}`,
	}
	for _, id := range []string{"claude", "grok", "agy"} {
		p, err := provider.Lookup(id)
		if err != nil {
			t.Fatalf("Lookup(%s): %v", id, err)
		}
		for name, stdout := range stdouts {
			t.Run(id+"/"+name, func(t *testing.T) {
				res, err := p.ParseTurn([]byte(stdout))
				if !errors.Is(err, provider.ErrUnparseableOutput) {
					t.Fatalf("err = %v, want ErrUnparseableOutput", err)
				}
				if !reflect.DeepEqual(res, provider.TurnResult{}) {
					t.Fatalf("res = %+v, want zero-value TurnResult{}（ErrUnparseableOutput 这一支不许带回任何半成品数据，internal/turn 那边的 UsageParsed 判据建立在这条不变量上）", res)
				}
			})
		}
	}
}

// TestParseTurn_KnownFailureShapesReturnNonZeroResult 覆盖 ErrTurnFailed
// 这一支（provider 自报失败，钱可能已经花了）：必须带回非零值的
// TurnResult，且 Usage 已经填好。三份原件（claudeBadModelJSON /
// grokBadModelJSON / agyBadModelJSON，定义于 fixtures_test.go）的 usage
// 子对象要么真实存在但数值为 0（claude / agy），要么整个成功形状五个字段
// 全是零值、只有 type/message 这个错误信封能认（grok）——三种情况下
// TurnResult 都必须整体非零值：claude 靠 ProviderSession 与
// Usage.CostBasis 非零值区分，agy 靠 Usage.Unreported（["cache_create"]）
// 非 nil 区分，grok 靠 StopRaw="error"（grok.go 在返回 ErrTurnFailed 之前
// 显式写的）与 Usage.Unreported（5 个键）非 nil 区分——不去挑其中一个
// provider 特有的字段单独断言，直接对整个 TurnResult 做 DeepEqual，
// 三家用同一个判据，不必为每家各写一套「哪个字段能证明非零」的特例。
func TestParseTurn_KnownFailureShapesReturnNonZeroResult(t *testing.T) {
	cases := []struct {
		id     string
		stdout string
	}{
		{"claude", claudeBadModelJSON},
		{"grok", grokBadModelJSON},
		{"agy", agyBadModelJSON},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			p, err := provider.Lookup(c.id)
			if err != nil {
				t.Fatalf("Lookup(%s): %v", c.id, err)
			}
			res, err := p.ParseTurn([]byte(c.stdout))
			if !errors.Is(err, provider.ErrTurnFailed) {
				t.Fatalf("err = %v, want ErrTurnFailed", err)
			}
			if reflect.DeepEqual(res, provider.TurnResult{}) {
				t.Fatalf("res 是零值 TurnResult{}，期望非零值（ErrTurnFailed 这一支必须尽量带回已经解析到的用量，钱可能已经花了）")
			}
			if reflect.DeepEqual(res.Usage, provider.Usage{}) {
				t.Errorf("Usage 是零值 provider.Usage{}，期望非零值（Usage 应当已经填好，即便数值恰好是 0）")
			}
		})
	}
}

// TestParseTurn_NoSessionIDReturnsNonZeroResult 覆盖 ErrNoSessionID 这一支，
// 补 TestParseTurn_KnownFailureShapesReturnNonZeroResult 没有覆盖的另一半：
// 契约点 (4) 明写「ErrTurnFailed 与 ErrNoSessionID 两支恒返回已经填好 Usage
// 的 base」，两支都要钉，不能只钉 ErrTurnFailed 那一支就当作整条不变量
// 已经证完——这正是本任务书要求「不要凭记忆猜」的地方：这两支在
// claude.go/grok.go/agy.go 里是各自独立的 if 分支，没有任何语言机制保证
// 改一支的时候另一支不会被漏掉。
//
// 三家各自的成功样本原件（claudeT1JSON/grokT1JSON/agyT1JSON，定义于
// fixtures_test.go）挖空会话 id 字段（mustReplaceOnce，同一份真实原件只改
// 一处，可追溯），success 判据本身不看会话 id 字段，所以挖空之后依然会先
// 判定为「成功形状」、再在会话 id 为空这一步单独失败，正是 ErrNoSessionID
// 要覆盖的那一刻。
func TestParseTurn_NoSessionIDReturnsNonZeroResult(t *testing.T) {
	cases := []struct {
		id     string
		stdout string
		old    string
		new    string
	}{
		{"claude", claudeT1JSON, `"session_id":"2a90d449-a348-497a-8812-e73e74d6898e"`, `"session_id":""`},
		{"grok", grokT1JSON, `"sessionId": "c0117e5a-4b21-4d3f-9f8e-2a6b1d0e7a31"`, `"sessionId": ""`},
		{"agy", agyT1JSON, `"conversation_id":"e484a528-68c6-4f30-ac0f-aa69c1a06ede"`, `"conversation_id":""`},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			p, err := provider.Lookup(c.id)
			if err != nil {
				t.Fatalf("Lookup(%s): %v", c.id, err)
			}
			blanked := mustReplaceOnce(t, c.stdout, c.old, c.new)
			res, err := p.ParseTurn([]byte(blanked))
			if !errors.Is(err, provider.ErrNoSessionID) {
				t.Fatalf("err = %v, want ErrNoSessionID", err)
			}
			if reflect.DeepEqual(res, provider.TurnResult{}) {
				t.Fatalf("res 是零值 TurnResult{}，期望非零值（ErrNoSessionID 这一支必须尽量带回已经解析到的用量——这一轮的钱是真花过的，只是会话 id 缺席）")
			}
			if reflect.DeepEqual(res.Usage, provider.Usage{}) {
				t.Errorf("Usage 是零值 provider.Usage{}，期望非零值")
			}
		})
	}
}
