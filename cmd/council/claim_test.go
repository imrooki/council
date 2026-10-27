package main

// claim_test.go 是 `council claim` / `council release` 的进程边界用例。
// 钉住的四件事，每件都对应一条硬约束：
//   - local_agent 不是 coordinate 时，claim 连库都不开（注入会 fatal 的开库
//     函数来证明），退出码 3；
//   - 拿到租约 → 同一资源再申请被拒 → 交还 → 再申请成功（generation 递增）；
//   - 数据目录被别的进程占着时退出 1，并说清 council.lock 与去哪停；
//   - 两个子命令在全局 FlagSet 之前被分派（-json 不会变成未知 flag）。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"council/internal/localagent"
	"council/internal/store"
)

// claimPanicDeps 的开库入口一被调用就让用例失败：未启用层级时一个字节都
// 不该写进库（S-01 / Plan §3.2 的逐层授权）。
func claimPanicDeps(t *testing.T) leaseDeps {
	t.Helper()
	return leaseDeps{
		Open: func(context.Context, string) (*store.Store, error) {
			t.Fatal("层级未开启时打开了库（应当一个字节都不写）")
			return nil, nil
		},
	}
}

func realLeaseDeps() leaseDeps { return leaseDeps{Open: store.Open} }

// claimRun 跑一次 claim，返回两个流与退出码。
func claimRun(t *testing.T, deps leaseDeps, args ...string) (string, string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := runClaimWith(args, deps, &out, &errOut)
	return out.String(), errOut.String(), code
}

// releaseRun 跑一次 release。
func releaseRun(t *testing.T, deps leaseDeps, args ...string) (string, string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := runReleaseWith(args, deps, &out, &errOut)
	return out.String(), errOut.String(), code
}

// TestClaimRequiresCoordinateLevel 钉住「观察不给协调权」：键缺席 / off /
// observe 三种配置下 claim 都退出 3、一个字节不写、连数据目录都不建。
func TestClaimRequiresCoordinateLevel(t *testing.T) {
	for _, extra := range []string{"", `local_agent = "off"`, `local_agent = "observe"`} {
		name := extra
		if name == "" {
			name = "键缺席"
		}
		t.Run(name, func(t *testing.T) {
			whoTestEnv(t)
			dataDir := filepath.Join(t.TempDir(), "data")
			cfgPath := whoTestConfig(t, dataDir, extra)

			out, errOut, code := claimRun(t, claimPanicDeps(t), "-config", cfgPath, t.TempDir())
			if code != exitLeaseDenied {
				t.Fatalf("退出码 = %d，期望 %d；stdout=%s stderr=%s", code, exitLeaseDenied, out, errOut)
			}
			if !strings.Contains(errOut, "未开启") || !strings.Contains(errOut, "coordinate") {
				t.Errorf("stderr 没有说清怎么开启:\n%s", errOut)
			}
			if _, err := os.Stat(dataDir); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("未启用时数据目录被建了出来（stat 错误: %v）", err)
			}
		})
	}

	// 写坏配置（register 是未实现的层级）是另一条路：Validate 直接拒，退 1。
	t.Run("非法层级退一", func(t *testing.T) {
		whoTestEnv(t)
		dataDir := filepath.Join(t.TempDir(), "data")
		cfgPath := whoTestConfig(t, dataDir, `local_agent = "register"`)
		_, errOut, code := claimRun(t, claimPanicDeps(t), "-config", cfgPath, t.TempDir())
		if code != exitConfigFailed {
			t.Fatalf("退出码 = %d，期望 %d；stderr=%s", code, exitConfigFailed, errOut)
		}
		if !strings.Contains(errOut, "local_agent") {
			t.Errorf("stderr 没有点名 local_agent:\n%s", errOut)
		}
	})
}

// TestClaimGrantConflictRelease 是一次完整往返：
// 拿到 → 同一资源被第二个人拒 → 交还 → 第二个人拿到，generation 递增。
func TestClaimGrantConflictRelease(t *testing.T) {
	whoTestEnv(t)
	dataDir := filepath.Join(t.TempDir(), "data")
	cfgPath := whoTestConfig(t, dataDir, `local_agent = "coordinate"`)
	resource := t.TempDir()

	out, errOut, code := claimRun(t, realLeaseDeps(),
		"-config", cfgPath, "-json", "-owner", "first", resource)
	if code != exitOK {
		t.Fatalf("首次 claim 退出码 = %d，期望 0；stdout=%s stderr=%s", code, out, errOut)
	}
	var first claimReport
	if err := json.Unmarshal([]byte(out), &first); err != nil {
		t.Fatalf("stdout 不是 JSON: %v；%s", err, out)
	}
	if !first.Granted || first.Lease == nil {
		t.Fatalf("报告 = %+v，期望 granted 且有 lease", first)
	}
	if first.LocalAgent != string(localagent.LevelCoordinate) {
		t.Errorf("local_agent = %q，期望 coordinate", first.LocalAgent)
	}
	if first.Mode != string(localagent.AccessWrite) {
		t.Errorf("mode = %q，期望 write（默认）", first.Mode)
	}
	if first.Lease.Generation != 1 {
		t.Errorf("generation = %d，期望 1", first.Lease.Generation)
	}
	if first.Lease.OwnerKind != store.LeaseOwnerCLI {
		t.Errorf("owner_kind = %q，期望 cli", first.Lease.OwnerKind)
	}

	// 第二个人拿同一个资源：退出 3，报告里带持有者，且 granted=false。
	out, errOut, code = claimRun(t, realLeaseDeps(),
		"-config", cfgPath, "-json", "-owner", "second", resource)
	if code != exitLeaseDenied {
		t.Fatalf("冲突时退出码 = %d，期望 %d；stdout=%s stderr=%s", code, exitLeaseDenied, out, errOut)
	}
	var busy claimReport
	if err := json.Unmarshal([]byte(out), &busy); err != nil {
		t.Fatalf("冲突输出不是 JSON: %v；%s", err, out)
	}
	if busy.Granted || busy.Lease != nil {
		t.Errorf("冲突报告 = %+v，期望 granted=false 且没有 lease", busy)
	}
	if busy.Holder == nil || busy.Holder.OwnerID != "first" || busy.Holder.LeaseID != first.Lease.LeaseID {
		t.Errorf("冲突报告里的持有者 = %+v，期望 first 那条", busy.Holder)
	}
	if busy.Denied != "busy" {
		t.Errorf("denied = %q，期望 busy", busy.Denied)
	}

	// 人类可读的冲突输出要说清「不会阻止任何进程」这条边界（走 stderr）。
	_, busyText, code := claimRun(t, realLeaseDeps(), "-config", cfgPath, "-owner", "second", resource)
	if code != exitLeaseDenied {
		t.Fatalf("文本冲突退出码 = %d，期望 %d", code, exitLeaseDenied)
	}
	for _, want := range []string{"资源被占用", "first", "不会阻止任何进程"} {
		if !strings.Contains(busyText, want) {
			t.Errorf("冲突文案里没有 %q:\n%s", want, busyText)
		}
	}

	// 交还之后第二个人能拿到，generation 递增。
	out, errOut, code = releaseRun(t, realLeaseDeps(), "-config", cfgPath, "-json", "-lease", first.Lease.LeaseID)
	if code != exitOK {
		t.Fatalf("release 退出码 = %d，期望 0；stdout=%s stderr=%s", code, out, errOut)
	}
	var rel releaseReport
	if err := json.Unmarshal([]byte(out), &rel); err != nil {
		t.Fatalf("release 输出不是 JSON: %v；%s", err, out)
	}
	if len(rel.Released) != 1 || rel.Released[0].State != string(store.LeaseReleased) {
		t.Fatalf("release 报告 = %+v，期望一条 released", rel)
	}

	out, errOut, code = claimRun(t, realLeaseDeps(),
		"-config", cfgPath, "-json", "-owner", "second", "-mode", "read", resource)
	if code != exitOK {
		t.Fatalf("交还后 claim 退出码 = %d，期望 0；stdout=%s stderr=%s", code, out, errOut)
	}
	var second claimReport
	if err := json.Unmarshal([]byte(out), &second); err != nil {
		t.Fatalf("第二次 claim 输出不是 JSON: %v；%s", err, out)
	}
	if second.Lease == nil || second.Lease.Generation != 2 {
		t.Errorf("第二次 = %+v，期望 generation 2", second.Lease)
	}
	if second.Lease.Mode != string(localagent.AccessRead) {
		t.Errorf("mode = %q，期望 read", second.Lease.Mode)
	}

	// 重复交还不算错误（脚本重试是正常行为），也没有新事件。
	_, _, code = releaseRun(t, realLeaseDeps(), "-config", cfgPath, "-lease", first.Lease.LeaseID)
	if code != exitOK {
		t.Errorf("重复交还退出码 = %d，期望 0", code)
	}
	events := claimEvents(t, dataDir, first.Lease.LeaseID)
	if len(events) != 2 {
		t.Errorf("第一条租约的事件 = %v，期望 granted + released 两条", events)
	}
}

// claimEvents 直接开库读一条租约的事件（用完即关），用于断言事件条数。
func claimEvents(t *testing.T, dataDir, leaseID string) []string {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("读事件时开库失败: %v", err)
	}
	defer func() {
		if cerr := st.Close(); cerr != nil {
			t.Errorf("关闭测试库失败: %v", cerr)
		}
	}()
	events, err := st.LeaseEvents(ctx, leaseID)
	if err != nil {
		t.Fatalf("LeaseEvents 失败: %v", err)
	}
	kinds := make([]string, 0, len(events))
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	return kinds
}

// TestReleaseByResourceOnlyOwnLeases 钉住 -resource 的匹配口径：只释放
// 自己名字的租约，别人的留着。
func TestReleaseByResourceOnlyOwnLeases(t *testing.T) {
	whoTestEnv(t)
	dataDir := filepath.Join(t.TempDir(), "data")
	cfgPath := whoTestConfig(t, dataDir, `local_agent = "coordinate"`)
	resource := t.TempDir()

	for _, owner := range []string{"me", "other"} {
		if _, errOut, code := claimRun(t, realLeaseDeps(),
			"-config", cfgPath, "-mode", "read", "-owner", owner, resource); code != exitOK {
			t.Fatalf("claim(%s) 退出码 = %d；stderr=%s", owner, code, errOut)
		}
	}

	out, errOut, code := releaseRun(t, realLeaseDeps(),
		"-config", cfgPath, "-json", "-resource", resource, "-owner", "me")
	if code != exitOK {
		t.Fatalf("release -resource 退出码 = %d；stdout=%s stderr=%s", code, out, errOut)
	}
	var rel releaseReport
	if err := json.Unmarshal([]byte(out), &rel); err != nil {
		t.Fatalf("输出不是 JSON: %v；%s", err, out)
	}
	if len(rel.Released) != 1 || rel.Released[0].OwnerID != "me" {
		t.Errorf("交还了 %+v，期望只有 me 那条", rel.Released)
	}

	// 拿别人的名字去交还：一条都没匹配到，退出 3（不是错误，是没做成）。
	_, errOut, code = releaseRun(t, realLeaseDeps(), "-config", cfgPath, "-resource", resource, "-owner", "nobody")
	if code != exitLeaseDenied {
		t.Errorf("没匹配时的退出码 = %d，期望 %d；stderr=%s", code, exitLeaseDenied, errOut)
	}
	if !strings.Contains(errOut, "没有") {
		t.Errorf("stderr 没说清没找到:\n%s", errOut)
	}
}

// TestReleaseDoesNotNeedCoordinate 钉住「关小开关不该把授权锁死」：
// local_agent = off 时 release 仍然工作。
func TestReleaseDoesNotNeedCoordinate(t *testing.T) {
	whoTestEnv(t)
	dataDir := filepath.Join(t.TempDir(), "data")
	resource := t.TempDir()
	onPath := whoTestConfig(t, dataDir, `local_agent = "coordinate"`)

	out, errOut, code := claimRun(t, realLeaseDeps(), "-config", onPath, "-json", resource)
	if code != exitOK {
		t.Fatalf("claim 退出码 = %d；stderr=%s", code, errOut)
	}
	var granted claimReport
	if err := json.Unmarshal([]byte(out), &granted); err != nil {
		t.Fatalf("claim 输出不是 JSON: %v；%s", err, out)
	}

	offPath := whoTestConfig(t, dataDir, `local_agent = "off"`)
	out, errOut, code = releaseRun(t, realLeaseDeps(), "-config", offPath, "-json", "-lease", granted.Lease.LeaseID)
	if code != exitOK {
		t.Fatalf("关掉开关后 release 退出码 = %d，期望 0；stdout=%s stderr=%s", code, out, errOut)
	}
	var rel releaseReport
	if err := json.Unmarshal([]byte(out), &rel); err != nil {
		t.Fatalf("release 输出不是 JSON: %v；%s", err, out)
	}
	if len(rel.Released) != 1 || rel.Released[0].State != string(store.LeaseReleased) {
		t.Errorf("交还结果 = %+v，期望一条 released", rel)
	}
	if rel.LocalAgent != string(localagent.LevelOff) {
		t.Errorf("报告里的 local_agent = %q，期望 off（如实报告当时层级）", rel.LocalAgent)
	}
}

// TestClaimUsageErrors 覆盖用法错误：全部退 2，且一行都不写进库。
func TestClaimUsageErrors(t *testing.T) {
	whoTestEnv(t)
	dataDir := filepath.Join(t.TempDir(), "data")
	cfgPath := whoTestConfig(t, dataDir, `local_agent = "coordinate"`)

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"没有资源参数", []string{"-config", cfgPath}, "恰好一个资源参数"},
		{"两个资源参数", []string{"-config", cfgPath, "a", "b"}, "恰好一个资源参数"},
		{"mode 写错", []string{"-config", cfgPath, "-mode", "rw", "."}, "mode"},
		{"ttl 太短", []string{"-config", cfgPath, "-ttl", "1s", "."}, "ttl"},
		{"ttl 太长", []string{"-config", cfgPath, "-ttl", "48h", "."}, "ttl"},
		{"端口不合法", []string{"-config", cfgPath, "tcp:70000"}, "端口"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errOut, code := claimRun(t, claimPanicDeps(t), tc.args...)
			if code != exitUsage {
				t.Fatalf("退出码 = %d，期望 %d；stderr=%s", code, exitUsage, errOut)
			}
			if !strings.Contains(errOut, tc.want) {
				t.Errorf("stderr 没有提到 %q:\n%s", tc.want, errOut)
			}
		})
	}
	if _, err := os.Stat(dataDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("用法错误时数据目录被建了出来（stat 错误: %v）", err)
	}
}

// TestReleaseUsageErrors 覆盖 release 的目标参数：两种都不给、两种都给都退 2。
func TestReleaseUsageErrors(t *testing.T) {
	whoTestEnv(t)
	cfgPath := whoTestConfig(t, filepath.Join(t.TempDir(), "data"), `local_agent = "coordinate"`)

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"没有目标", []string{"-config", cfgPath}, "需要一个目标"},
		{"两个目标", []string{"-config", cfgPath, "-lease", "x", "-resource", "."}, "只接受一个目标"},
		{"位置参数", []string{"-config", cfgPath, "extra"}, "不接受位置参数"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errOut, code := releaseRun(t, claimPanicDeps(t), tc.args...)
			if code != exitUsage {
				t.Fatalf("退出码 = %d，期望 %d；stderr=%s", code, exitUsage, errOut)
			}
			if !strings.Contains(errOut, tc.want) {
				t.Errorf("stderr 没有提到 %q:\n%s", tc.want, errOut)
			}
		})
	}
}

// TestReleaseUnknownLeaseExitsThree 交还一条不存在的租约：退出 3，说清原因。
func TestReleaseUnknownLeaseExitsThree(t *testing.T) {
	whoTestEnv(t)
	dataDir := filepath.Join(t.TempDir(), "data")
	cfgPath := whoTestConfig(t, dataDir, `local_agent = "coordinate"`)

	// 先建库（release 会开库，没有库时 store.Open 会建一个空库）。
	_, errOut, code := releaseRun(t, realLeaseDeps(), "-config", cfgPath, "-lease", "no-such-lease")
	if code != exitLeaseDenied {
		t.Fatalf("退出码 = %d，期望 %d；stderr=%s", code, exitLeaseDenied, errOut)
	}
	if !strings.Contains(errOut, "没有这条租约") {
		t.Errorf("stderr 没说清没找到:\n%s", errOut)
	}
}

// TestClaimLockedDataDir 钉住单写者：库被占着时退 1，并点名 council.lock
// 与「先停下哪个进程」。
func TestClaimLockedDataDir(t *testing.T) {
	whoTestEnv(t)
	dataDir := filepath.Join(t.TempDir(), "data")
	cfgPath := whoTestConfig(t, dataDir, `local_agent = "coordinate"`)
	ctx := context.Background()
	held, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("先占住库失败: %v", err)
	}
	defer func() {
		if cerr := held.Close(); cerr != nil {
			t.Errorf("关闭占位库失败: %v", cerr)
		}
	}()

	_, errOut, code := claimRun(t, realLeaseDeps(), "-config", cfgPath, t.TempDir())
	if code != exitConfigFailed {
		t.Fatalf("退出码 = %d，期望 %d；stderr=%s", code, exitConfigFailed, errOut)
	}
	for _, want := range []string{"council.lock", "serve -rw"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr 没有提到 %q:\n%s", want, errOut)
		}
	}
}

// TestClaimIsDispatchedBeforeGlobalFlags 走一遍 run()：-json 只有 claim 认识，
// 没有分派时它会撞上全局 FlagSet 变成「flag provided but not defined」。
func TestClaimIsDispatchedBeforeGlobalFlags(t *testing.T) {
	whoTestEnv(t)
	dataDir := filepath.Join(t.TempDir(), "data")
	cfgPath := whoTestConfig(t, dataDir, `local_agent = "coordinate"`)

	out, code := commandsRun(t, []string{"claim", "-config", cfgPath, "-json", t.TempDir()})
	if code != exitOK {
		t.Fatalf("run(claim ...) = %d，期望 0；stdout=%s", code, out)
	}
	var report claimReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("stdout 不是 JSON（说明没走到 claim）: %v；%s", err, out)
	}
	if !report.Granted {
		t.Errorf("报告 = %+v，期望 granted", report)
	}
}

// TestClaimUsageEnglish 钉住 -h 的中英分叉：-lang en 下不许出现汉字。
func TestClaimUsageEnglish(t *testing.T) {
	whoTestEnv(t)
	_, errOut, code := claimRun(t, claimPanicDeps(t), "-h", "-lang", "en")
	if code != exitOK {
		t.Fatalf("退出码 = %d，期望 0", code)
	}
	if cliHasHan(errOut) {
		t.Fatalf("-lang en 的 claim 用法里出现汉字:\n%s", errOut)
	}
	if !strings.Contains(errOut, "Usage: council claim") {
		t.Errorf("用法里缺少 Usage: council claim:\n%s", errOut)
	}

	_, errOut, code = releaseRun(t, claimPanicDeps(t), "-h", "-lang", "en")
	if code != exitOK {
		t.Fatalf("release -h 退出码 = %d，期望 0", code)
	}
	if cliHasHan(errOut) {
		t.Fatalf("-lang en 的 release 用法里出现汉字:\n%s", errOut)
	}
	if !strings.Contains(errOut, "Usage: council release") {
		t.Errorf("用法里缺少 Usage: council release:\n%s", errOut)
	}
}
