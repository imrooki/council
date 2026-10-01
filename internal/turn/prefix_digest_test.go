package turn

// 本文件覆盖 docs/11b_PREFIX_DIGEST_CONTRACT.md 第 4 节的验收：
//   4.1 conformance 测试（判据是行为，不是声明），对每一个已注册的 provider
//       各跑一遍；
//   4.2 golden 钉住规范化之后的 argv 列表本身，摘要由它派生。
//
// 两节都直接调 council/internal/provider 的真实 Lookup/BuildTurn（provider
// 包不在本片改动范围内，属允许读的既有代码），不拿假 Provider 顶替——
// 摘要机制真正会被三家实际发出去的 flag 检验，才是这条契约要的东西。

import (
	"path/filepath"
	"strings"
	"testing"

	"council/internal/provider"
)

// prefixDigestConformanceBaseReq 给每个 provider 造一份共同的基线请求：
// 一档三家都放行的权限（read）、非空 Model / Effort / Role。WorkDir 确实
// 会进 claude 的 rolePath、grok 的 --cwd、agy 的 --add-dir，不是无关字段——
// 这里能用 t.TempDir() 而不必像 identityFPFor 的调用方那样跨多次调用共享
// 同一条路径，只是因为下面每个子测试都从同一个 base 值派生 reqA/reqB/…，
// WorkDir 在一次子测试内部原样不变，之间比的从头到尾是同一条路径。
func prefixDigestConformanceBaseReq(t *testing.T) provider.TurnRequest {
	t.Helper()
	return provider.TurnRequest{
		AgentID:    "conformance-agent",
		Model:      "model-a",
		Effort:     "high",
		Role:       "conformance role text",
		Permission: provider.PermissionRead,
		WorkDir:    t.TempDir(),
	}
}

// TestPrefixArgsDigestConformance 是契约 4.1 的四条判据，对 provider.IDs()
// 里注册的每一个 provider 各跑一遍（判据第 5 条：新增 provider 时这条测试
// 必须逼人表态——遍历 provider.IDs()，不是写死三个 id）。
func TestPrefixArgsDigestConformance(t *testing.T) {
	ids := provider.IDs()
	if len(ids) == 0 {
		t.Fatal("provider.IDs() 返回空：registry 里一个 provider 都没有，这条 conformance 测试没有意义")
	}

	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			p, err := provider.Lookup(id)
			if err != nil {
				t.Fatalf("provider.Lookup(%q) 失败: %v", id, err)
			}
			base := prefixDigestConformanceBaseReq(t)

			// 第 1 条：同一档、只换会话 id 与正文，摘要必须相同——否则每轮都会
			// 误报漂移。PrefixArgsDigest 内部把这两样都规范化掉，传什么进来
			// 都不该影响结果，这里刻意传两个明显不同的值来证明这一点。
			reqA := base
			reqA.ProviderSession = ""
			reqA.Prompt = "这一轮的真实正文，逐轮都不一样"
			reqB := base
			reqB.ProviderSession = "some-real-provider-session-id-from-a-resume"
			reqB.Prompt = "另一轮完全不同的正文，长度也不一样啊"
			digestA, err := PrefixArgsDigest(p, reqA)
			if err != nil {
				t.Fatalf("PrefixArgsDigest(reqA) 失败: %v", err)
			}
			digestB, err := PrefixArgsDigest(p, reqB)
			if err != nil {
				t.Fatalf("PrefixArgsDigest(reqB) 失败: %v", err)
			}
			if digestA != digestB {
				t.Errorf("只换会话 id 与正文，摘要却不同: %q vs %q", digestA, digestB)
			}

			// 第 3 条：换 model，摘要必须不同——三家的 --model 都会带上这个值。
			reqModelB := base
			reqModelB.Model = "model-b"
			digestModelB, err := PrefixArgsDigest(p, reqModelB)
			if err != nil {
				t.Fatalf("PrefixArgsDigest(换 model) 失败: %v", err)
			}
			if digestModelB == digestA {
				t.Errorf("换了 model，摘要却相同: %q", digestModelB)
			}

			// 第 4 条：对同一份输入连算两次，结果必须相同——grok 每次都现铸
			// 新 UUID（SessionIDPreassignable），这一条直接钉住 PlannedSessionID
			// 的占位符替换真的生效了，不是巧合过了前三条。
			digestAgain, err := PrefixArgsDigest(p, reqA)
			if err != nil {
				t.Fatalf("PrefixArgsDigest(reqA 第二次) 失败: %v", err)
			}
			if digestAgain != digestA {
				t.Errorf("同一份输入连算两次，结果不同: %q vs %q（grok 的 UUID 归一化没有生效）", digestA, digestAgain)
			}

			// 第 2 条：换权限档，摘要必须不同——这是 6530839 那次该红而没红的
			// 情形（tier 名字没变，适配器实际发出去的 flag 变了）。
			//
			// 11c 契约落地（Step 8 第二片）之后 claude 与 agy 各声明了两档
			// （read、workspace），下面这条分支对它们真的会跑起来并断言
			// 摘要不同——工作目录从 <base>/<agentID> 变成
			// <base>/<convID>/<agentID>，命令行确实变了。grok 仍然只声明
			// read 一档（3.1 节：它证明不了写入边界，不列 workspace 是有意
			// 的，不是遗漏），继续走 t.Logf 那一支，这不是遗留缝隙，是
			// grok 这一家目前唯一放行的档就只有一个、没有第二个可比较的
			// 输入。t.Logf 而不是 t.Skip：跳过要在详细输出里看得见，
			// grok 哪天多一档时这里会立刻真正跑起来，不会被永久静默略过。
			caps := p.Capabilities()
			if len(caps.Permissions) < 2 {
				t.Logf("跳过「换权限档摘要必须不同」：provider %q 的 Capabilities().Permissions 目前只声明了 %d 档 (%v)，没有第二个已放行的 tier 可比较", id, len(caps.Permissions), caps.Permissions)
			} else {
				reqTierB := base
				reqTierB.Permission = caps.Permissions[1]
				digestTierB, err := PrefixArgsDigest(p, reqTierB)
				if err != nil {
					t.Fatalf("PrefixArgsDigest(换权限档) 失败: %v", err)
				}
				if digestTierB == digestA {
					t.Errorf("换了权限档，摘要却相同: %q", digestTierB)
				}
			}

			// 这条不算 4 项判据之一，但值得钉住：请求一个这家从未声明放行的
			// tier 时，PrefixArgsDigest 必须诚实地返回错误，不能悄悄给出某个
			// 摘要——那会让「这一档到底支不支持」这件事在指纹这一层被抹平。
			reqUnsupported := base
			reqUnsupported.Permission = provider.PermissionFull
			if _, err := PrefixArgsDigest(p, reqUnsupported); err == nil {
				t.Errorf("provider %q 对未放行的权限档 %q 应当返回错误，实际返回 nil", id, provider.PermissionFull)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 4.2 golden：钉 argv 列表本身，不是钉摘要
// ---------------------------------------------------------------------------

// prefixDigestGoldenWorkDir 是三份 golden 共用的固定 WorkDir——不能用
// t.TempDir()：那个每次跑都不一样，golden 比的是逐字节相同的 argv，
// 必须是一个跨多次运行、跨机器都稳定的字面量。不要求这个目录真实存在，
// BuildTurn 本身不摸文件系统（落盘是 spawnProcess 的事）。
const prefixDigestGoldenWorkDir = `C:\council-golden\workdir`

// prefixDigestGoldenReq 是三份 golden 共用的输入：固定 Model / Effort / Role /
// 权限档，只有 provider 不同。Effort 非空是为了让 claude 那份 golden 里
// 出现 --effort（EffortPerTurnFlag 才会发，agy/grok 不看这个字段）。
func prefixDigestGoldenReq() provider.TurnRequest {
	return provider.TurnRequest{
		AgentID:    "golden-agent",
		Model:      "model-golden",
		Effort:     "effort-golden",
		Role:       "role-golden-text",
		Permission: provider.PermissionRead,
		WorkDir:    prefixDigestGoldenWorkDir,
	}
}

// goldenDigestFor 把一份 golden argv 列表按 PrefixArgsDigest 同一条公式
// （packFingerprint 逐段编码再 sha256 前 6 字节）转成期望的摘要串，
// 让"摘要由 argv 派生"这句话在用例里也成立，不是只在注释里说说。
func goldenDigestFor(args []string) string {
	fp, _ := PrefixArgsDigest(goldenDigestStubProvider{args: args}, provider.TurnRequest{})
	return fp
}

// goldenDigestStubProvider 只用于 goldenDigestFor 从一份已经算好的 argv
// 反推期望摘要，不经过任何真实适配器的 BuildTurn——它不是被测对象，
// 真正的被测对象是下面三个 TestPrefixArgsGolden_* 用例里对 normalizedPrefixArgs
// 的逐元素比较；这个桩子只是为了不在测试里手写第二份哈希算法。
type goldenDigestStubProvider struct{ args []string }

func (s goldenDigestStubProvider) ID() string { return "golden-stub" }
func (s goldenDigestStubProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{}
}
func (s goldenDigestStubProvider) BuildTurn(provider.TurnRequest) (provider.Command, error) {
	return provider.Command{Args: s.args}, nil
}
func (s goldenDigestStubProvider) ParseTurn([]byte) (provider.TurnResult, error) {
	return provider.TurnResult{}, nil
}

// TestPrefixArgsGoldenClaude 钉住 claude 规范化之后的 argv：身份走文件
// （--append-system-prompt-file <rolePath>），命令行里没有正文、没有会话 id，
// 只有一条固定长度的路径——这正是未决 #16 那次改造的意义所在。
func TestPrefixArgsGoldenClaude(t *testing.T) {
	p, err := provider.Lookup("claude")
	if err != nil {
		t.Fatalf("provider.Lookup(claude) 失败: %v", err)
	}
	rolePath := filepath.Join(prefixDigestGoldenWorkDir, "council-role.txt")
	want := []string{
		"-p", "--output-format", "json",
		"--model", "model-golden",
		"--effort", "effort-golden",
		"--append-system-prompt-file", rolePath,
		"--safe-mode", "--restricted", "--permission-mode", "default",
	}

	got, err := normalizedPrefixArgs(p, prefixDigestGoldenReq())
	if err != nil {
		t.Fatalf("normalizedPrefixArgs 失败: %v", err)
	}
	assertArgsEqual(t, "claude", got, want)

	digest, err := PrefixArgsDigest(p, prefixDigestGoldenReq())
	if err != nil {
		t.Fatalf("PrefixArgsDigest 失败: %v", err)
	}
	if want := goldenDigestFor(want); digest != want {
		t.Errorf("PrefixArgsDigest = %q，期望等于由 golden argv 派生的 %q", digest, want)
	}
}

// TestPrefixArgsGoldenGrok 钉住 grok 规范化之后的 argv：身份直接进
// --system-prompt-override 的值，会话 id 段被换成 prefixProbeSessionID
// 占位符（grok 是 SessionIDPreassignable，fresh 支现铸 UUID）。
func TestPrefixArgsGoldenGrok(t *testing.T) {
	p, err := provider.Lookup("grok")
	if err != nil {
		t.Fatalf("provider.Lookup(grok) 失败: %v", err)
	}
	want := []string{
		"-p", prefixProbePrompt,
		"--output-format", "json",
		"--model", "model-golden",
		"--system-prompt-override", "role-golden-text",
		"--permission-mode", "default",
		"--deny", "Write(*)",
		"--deny", "Edit(*)",
		"--deny", "Bash(*)",
		"--cwd", prefixDigestGoldenWorkDir,
		"--session-id", prefixProbeSessionID,
	}

	got, err := normalizedPrefixArgs(p, prefixDigestGoldenReq())
	if err != nil {
		t.Fatalf("normalizedPrefixArgs 失败: %v", err)
	}
	assertArgsEqual(t, "grok", got, want)

	digest, err := PrefixArgsDigest(p, prefixDigestGoldenReq())
	if err != nil {
		t.Fatalf("PrefixArgsDigest 失败: %v", err)
	}
	if want := goldenDigestFor(want); digest != want {
		t.Errorf("PrefixArgsDigest = %q，期望等于由 golden argv 派生的 %q", digest, want)
	}
}

// TestPrefixArgsGoldenAgy 钉住 agy 规范化之后的 argv：READ 档不发任何权限
// flag（agyPermissionArgs(PermissionRead) 返回 nil），身份与正文拼在一起
// 紧跟 -p（IdentityPromptPreamble，只在 ProviderSession == "" 这一支拼）。
func TestPrefixArgsGoldenAgy(t *testing.T) {
	p, err := provider.Lookup("agy")
	if err != nil {
		t.Fatalf("provider.Lookup(agy) 失败: %v", err)
	}
	want := []string{
		"-p", "role-golden-text\n" + prefixProbePrompt,
		"--output-format", "json",
		"--model", "model-golden",
		"--add-dir", prefixDigestGoldenWorkDir,
	}

	got, err := normalizedPrefixArgs(p, prefixDigestGoldenReq())
	if err != nil {
		t.Fatalf("normalizedPrefixArgs 失败: %v", err)
	}
	assertArgsEqual(t, "agy", got, want)

	digest, err := PrefixArgsDigest(p, prefixDigestGoldenReq())
	if err != nil {
		t.Fatalf("PrefixArgsDigest 失败: %v", err)
	}
	if want := goldenDigestFor(want); digest != want {
		t.Errorf("PrefixArgsDigest = %q，期望等于由 golden argv 派生的 %q", digest, want)
	}
}

func assertArgsEqual(t *testing.T, providerID string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: normalizedPrefixArgs 返回 %d 个参数，期望 %d 个\n实际: %s\n期望: %s",
			providerID, len(got), len(want), strings.Join(got, " | "), strings.Join(want, " | "))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: 第 %d 个参数 = %q，期望 %q\n完整实际: %s\n完整期望: %s",
				providerID, i, got[i], want[i], strings.Join(got, " | "), strings.Join(want, " | "))
		}
	}
}
