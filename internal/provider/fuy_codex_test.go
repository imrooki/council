package provider_test

// fu2：CX1 独立审查余项在 provider 包的补测。只加用例，不动生产代码。
// 装置复用 cxx_codex_test.go 的 cxxCodex / cxxReq / cxxArgs / cxxTh / cxxDone / cxxU1 / cxxFail / cxxThreadID。
// ParseTurn 的输入是合成的（turn.failed 的形状本机零观测，与 cxx_codex_test.go 的 P6 同一类）。
// 期望值一律取自实跑；每条用例杀的变异写在它自己的注释里。

import (
	"errors"
	"reflect"
	"testing"

	"council/internal/provider"
)

// TestFuyResumeRejectsBadEffort 钉续会话形状上的非法强度也在适配器被拒。
// 请求带 ProviderSession（任意非空 thread id）与 Effort="bogus"：BuildTurn 返回包着 ErrUnsupportedEffort 的错误，
// Command 为零值。对照是同请求、Effort 合法（low）时成功，argv 是续会话形状。
//
// 【判别力】变异：codex.go 里强度校验只在 ProviderSession 为空时做
// （if req.Effort != "" && !codexEffortAllowed(req.Effort) 前面再与上 req.ProviderSession == ""）。
// 这时 bogus 在续会话上放行，errors.Is 红。既有的 TestCxxBuildTurnEffort 全部走空会话，杀不到这条
// （这条变异下 provider 包只有本用例红，fu2 实跑）。
func TestFuyResumeRejectsBadEffort(t *testing.T) {
	p := cxxCodex(t)
	t.Run("bogus", func(t *testing.T) {
		req := cxxReq()
		req.ProviderSession = cxxThreadID
		req.Effort = "bogus"
		got, err := p.BuildTurn(req)
		if !errors.Is(err, provider.ErrUnsupportedEffort) {
			t.Fatalf("续会话、强度 bogus：err = %v，期望包着 ErrUnsupportedEffort；Command = %#v", err, got)
		}
		if !reflect.DeepEqual(got, provider.Command{}) {
			t.Errorf("续会话、强度 bogus 被拒时 Command 应为零值，实际 %#v", got)
		}
	})
	t.Run("low_control", func(t *testing.T) {
		req := cxxReq()
		req.ProviderSession = cxxThreadID
		req.Effort = "low"
		got, err := p.BuildTurn(req)
		if err != nil {
			t.Fatalf("续会话、合法强度 low 被拒: %v", err)
		}
		if want := cxxArgs(cxxThreadID, "low"); !reflect.DeepEqual(got.Args, want) {
			t.Errorf("Args = %q，期望 %q", got.Args, want)
		}
	})
}

// TestFuyFailedWithTwoCompletedIsUnparseable 钉 ParseTurn 的判定顺序：completed > 1 的判定排在 turn.failed 之前。
// thread.started + turn.failed + 两条 turn.completed 判认不出（ErrUnparseableOutput，零值 TurnResult）；
// 对照是只差少一条 turn.completed 的同一输入，判失败（ErrTurnFailed，带回 thread_id 与 UsageRaw）。
//
// 【判别力】变异：把 codex.go 里 if completed > 1 { ... } 那一块挪到 if failed { ... } 之后。
// 这时两条 turn.completed 的输入先走 failed 分支，得到 ErrTurnFailed，two_completed 一行红。
// 既有 TestCxxParseShapes 的 two_completed 不带 turn.failed，在这条变异下仍然绿；
// 全仓 go test ./... 在这条变异下只有本用例红（fu2 实跑）。
func TestFuyFailedWithTwoCompletedIsUnparseable(t *testing.T) {
	const failed = "{\"type\":\"turn.failed\",\"error\":{\"message\":\"boom\"}}\n"
	p := cxxCodex(t)
	t.Run("two_completed", func(t *testing.T) {
		got, err := p.ParseTurn([]byte(cxxTh + failed + cxxDone + cxxDone))
		if !errors.Is(err, provider.ErrUnparseableOutput) {
			t.Fatalf("err = %v，期望包着 ErrUnparseableOutput", err)
		}
		if !reflect.DeepEqual(got, provider.TurnResult{}) {
			t.Errorf("TurnResult 应为零值，实际 %#v", got)
		}
	})
	t.Run("one_completed_control", func(t *testing.T) {
		got, err := p.ParseTurn([]byte(cxxTh + failed + cxxDone))
		if !errors.Is(err, provider.ErrTurnFailed) {
			t.Fatalf("err = %v，期望包着 ErrTurnFailed", err)
		}
		if want := cxxFail("turn.failed", cxxU1); !reflect.DeepEqual(got, want) {
			t.Errorf("TurnResult =\n%#v\n期望\n%#v", got, want)
		}
	})
}
