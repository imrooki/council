package app

// oby_firstrun_app_test.go 是 OB2（00_DECISIONS.md #48 追加裁决：配了 API key 算已登录）在 app 包的验收：
// B0 登录状态的四档说法，B1、B2 首次运行端到端（已登录（API key）那几行不附登录办法），
// B3 补 OB1 审查余项 R1（codex login status 受这一家的 firstRunTimeout 约束）。
// 装置与夹具只读复用 obx_firstrun_app_test.go 的 §2.0（obxIsolate、obxRun、obxTypeFake、obxCodexFake、
// obxFixture、obxSetTimeout、obxSub、obxWantFile、obxReadFile、obxIn、obxEnv、obxHead、obxLoginHead、obxDone、
// obxClaudeOut、obxAgyOut、obxCodexOut）；纪律同契约 19d §2 开头。

import (
	"context"
	"strings"
	"testing"
	"time"

	"council/internal/provider"
)

// obyGrokLocalProxyOut 是本机实测的 grok models 全部 stdout（grok 1.0.41，经 本地转发服务 走 API key；191 字节，LF）。
const obyGrokLocalProxyOut = "Model 'grok-4.7' is using its own API key.\n\nDefault model: grok-4.7\n\nAvailable models:\n  * grok-4.7 (default)\n  - grok-4.6\n  - grok-4.5\n  - grok-4.3\n  - grok-composer-2.5-fast\n  - grok-build\n"

// obyClaudeAccountOld、obyClaudeAccountKey：把 obxClaudeOut 的 account 换成用 ANTHROPIC_API_KEY 的形状。
const (
	obyClaudeAccountOld = `"account":{"apiProvider":"firstParty","subscriptionType":"Claude Obx"}`
	obyClaudeAccountKey = `"account":{"tokenSource":"none","apiKeySource":"ANTHROPIC_API_KEY","apiProvider":"firstParty"}`
)

// obyCodexKeyIn 是 codex login status 用 API key 登录时的输出（程序文本的句子，key 那一段是假的）。
var obyCodexKeyIn = []string{"@echo Logged in using an API key - sk-oby*** 1>&2", "@exit /b 0"}

// obyKeyFakes 装 claude（API key）、grok（本机实测的 本地转发服务 输出）、codex（API key）；withAgy 为真时再装 agy。
func obyKeyFakes(t *testing.T, e *obxEnv, withAgy bool) {
	t.Helper()
	if strings.Count(obxClaudeOut, obyClaudeAccountOld) != 1 {
		t.Fatalf("前提不成立：obxClaudeOut 里的 account 不是恰好一处 %s", obyClaudeAccountOld)
	}
	obxTypeFake(t, e, "claude", strings.Replace(obxClaudeOut, obyClaudeAccountOld, obyClaudeAccountKey, 1))
	obxTypeFake(t, e, "grok", obyGrokLocalProxyOut)
	if withAgy {
		obxTypeFake(t, e, "agy", obxAgyOut)
	}
	xfx := obxFixture(t, e, "codex.out", obxCodexOut)
	obxCodexFake(t, e, []string{`@type "` + xfx + `"`}, obyCodexKeyIn)
}

// obyRowClaude、obyRowGrok、obyRowCodex 是 B1、B2 里 claude、grok、codex 三行（agy 那一行各自给）。
const (
	obyRowClaude = "  claude  <bin>\\claude.cmd  识别到 2 个模型；已登录（API key：ANTHROPIC_API_KEY）\n"
	obyRowGrok   = "  grok    <bin>\\grok.cmd  识别到 6 个模型；已登录（API key）\n"
	obyRowCodex  = "  codex   <bin>\\codex.cmd  识别到 1 个模型；已登录（API key）\n"
)

// TestObyFirstRunLoginText 是 B0：firstRunLoginText 的四档六种说法。
// 【判别力】N21（API key 那一档落到查不出）、N22（不带来源）。
func TestObyFirstRunLoginText(t *testing.T) {
	for _, c := range []struct {
		name string
		in   provider.LoginInfo
		want string
	}{
		{"yes_sub", provider.LoginInfo{State: provider.LoginYes, Subscription: "Claude Oby"}, "已登录（订阅：Claude Oby）"},
		{"yes", provider.LoginInfo{State: provider.LoginYes}, "已登录"},
		{"key_source", provider.LoginInfo{State: provider.LoginAPIKey, KeySource: "/login managed key"}, "已登录（API key：/login managed key）"},
		{"key", provider.LoginInfo{State: provider.LoginAPIKey}, "已登录（API key）"},
		{"no", provider.LoginInfo{State: provider.LoginNo}, "没登录"},
		{"unknown", provider.LoginInfo{}, "登录状态查不出"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := firstRunLoginText(c.in); got != c.want {
				t.Errorf("firstRunLoginText(%+v) = %q，期望 %q", c.in, got, c.want)
			}
		})
	}
}

// TestObyFirstRunAPIKey 是 B1：四家都装，claude、grok、codex 都是已登录（API key），agy 查不出。
// 登录说明照打，但只列 agy。与 B2 只差装不装 agy，是 B2「登录说明不打」的正对照。
// 【判别力】N1、N5、N9、N12、N20、N21、N22。
func TestObyFirstRunAPIKey(t *testing.T) {
	e := obxIsolate(t)
	obyKeyFakes(t, e, true)

	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s\nout:\n%s", err, errOut, out)
	}
	want := obxSub(e, obxHead+"正在检查 claude、grok、agy、codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 20s）……\n"+
		obyRowClaude+obyRowGrok+
		"  agy     <bin>\\agy.cmd  识别到 1 个模型；登录状态查不出\n"+
		obyRowCodex+
		obxLoginHead+"  agy     agy（按提示登录）\n"+obxDone)
	if errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
	if file, wantFile := obxReadFile(t, e.cfgPath), obxWantFile(e, "claude", "grok", "agy", "codex"); file != wantFile {
		t.Errorf("配置文件字节不等\n实际 %q\n期望 %q", file, wantFile)
	}
}

// TestObyFirstRunAPIKeyAllIn 是 B2：同 B1 但不装 agy。找到的全是已登录（API key），登录说明与登录办法都不打。
// 【判别力】N20（已登录（API key）的也列登录办法）、N1、N5、N12、N21、N22。
func TestObyFirstRunAPIKeyAllIn(t *testing.T) {
	e := obxIsolate(t)
	obyKeyFakes(t, e, false)

	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s\nout:\n%s", err, errOut, out)
	}
	want := obxSub(e, obxHead+"正在检查 claude、grok、codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 20s）……\n"+
		obyRowClaude+obyRowGrok+
		"  agy     没找到\n"+
		obyRowCodex+obxDone)
	if errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
	if strings.Contains(errOut, obxLoginHead) {
		t.Errorf("找到的全都已登录（API key）时不该打登录说明，errOut %q", errOut)
	}
}

// obyStatusLimit 是 B3 假 codex 等待循环的计数上限，与 obxLoopLimit 同值；数到上限之后不退出，
// 而是落到循环后面那两行（打 Logged in using a subscription、以 0 退出），这样没被杀掉时结果看得见。
const obyStatusLimit = "60000"

// TestObyFirstRunCodexStatusTimeout 是 B3（R1）：假 codex 的 debug models 照常返回，login status 挂在
// 等待循环里。firstRunTimeout 是 2 s：这一家的 ctx 到期时 login status 被杀掉，codex 那一行是登录状态查不出，
// Run 在 obyB3Limit 之内返回。firstRunStatus 改用父 ctx 时，循环数到上限、打出 Logged in using a subscription，
// 那一行变成已登录，而且 Run 要等远超 obyB3Limit（实测约 24.5 s）。
// 【判别力】N23。
func TestObyFirstRunCodexStatusTimeout(t *testing.T) {
	e := obxIsolate(t)
	obxSetTimeout(t, 2*time.Second)
	loopOn := obxFixture(t, e, "loop.on", "x")
	xfx := obxFixture(t, e, "codex.out", obxCodexOut)
	obxCodexFake(t, e, []string{`@type "` + xfx + `"`}, []string{
		"@echo off", "set n=0", ":obyloop", "set /a n+=1",
		"if %n% geq " + obyStatusLimit + " goto obydone",
		`if exist "` + loopOn + `" goto obyloop`,
		":obydone", "echo Logged in using a subscription 1>&2", "exit /b 0",
	})

	start := time.Now()
	out, errOut, err := obxRun(t, context.Background(), Options{In: obxIn("/quit\n")})
	el := time.Since(start)
	if err != nil {
		t.Fatalf("Run 返回 %v；errOut:\n%s\nout:\n%s", err, errOut, out)
	}
	want := obxSub(e, obxHead+"正在检查 codex 能不能用（只让它们列出模型、报登录状态，不提问、不花钱；每家最多 2s）……\n"+
		"  claude  没找到\n  grok    没找到\n  agy     没找到\n"+
		"  codex   <bin>\\codex.cmd  识别到 1 个模型；登录状态查不出\n"+
		obxLoginHead+"  codex   codex login\n"+obxDone)
	if errOut != want {
		t.Errorf("errOut 不等\n实际 %q\n期望 %q", errOut, want)
	}
	if el >= obyB3Limit {
		t.Errorf("Run 用了 %v 才返回，应当小于 %v（login status 受这一家的 firstRunTimeout 约束）", el, obyB3Limit)
	}
}

// obyB3Limit 是 B3 对 Run 耗时的上限：firstRunTimeout（2 s）加上起进程、杀进程与 REPL 的余量。
const obyB3Limit = 10 * time.Second
