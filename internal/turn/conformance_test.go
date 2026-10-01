package turn

// 本文件是 09 D 节与 09a 明写的两条 conformance 测试，以及 K 节「子进程契约」
// 组里能在不真正 spawn 的情况下钉住的那部分：Command.Dir 非空且不等于进程
// 当前工作目录、Command.Env 为 nil。这两条不需要源码扫描，走一次假 Ask 流程
// 用 fakeSpawner 把 Runner 真正传给 Spawner 的 Command 捕获下来即可验证。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSessionIDPolicyNeverAppearsInOrchestrationLayer 是 09_STEP6_SPEC.md D 节
// 明写的那条 conformance 测试：「有一条 conformance 测试扫 internal/turn 的
// 源文件，断言这个类型名一次都不出现在编排层里」——编排层判「要不要新建会话」
// 只看 TurnRequest.ProviderSession 是不是空串，SessionIDPolicy 是给 Step 9
// 排并发、给 conformance 测试用的元数据，本包一次都不该读它。
//
// 扫描范围明确排除本包自己的 *_test.go：这条规则约束的是编排层的实现，
// 不是测试文件——测试文件里必然会在注释与字符串里提到这个类型名（就像这一句
// 文档注释本身），把测试文件也扫进去只会让这条用例扫到自己，那是假阳性，
// 不是真的违反。
func TestSessionIDPolicyNeverAppearsInOrchestrationLayer(t *testing.T) {
	dir := turnPackageSourceDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读目录 %s 失败: %v", dir, err)
	}
	const needle = "SessionID" + "Policy" // 拼接而不是字面量，避免这行本身被下面的扫描命中
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读 %s 失败: %v", path, err)
		}
		checked++
		if strings.Contains(string(data), needle) {
			t.Errorf("%s 里出现了 %s：编排层判定「要不要新建会话」只许看 TurnRequest.ProviderSession 是不是空串（09 D 节）", path, needle)
		}
	}
	if checked == 0 {
		t.Fatal("一个非测试的 .go 文件都没扫到，这条用例本身没有意义，检查 turnPackageSourceDir 是否指对了目录")
	}
}

// TestSpawnSourceNeverReadsOSStdin 是 K 节「子进程契约」组里唯一一条只能靠
// 源码扫描才能钉住的规则：runner.go 顶部注释与 09a 都写明子进程的标准输入
// 必须接 null device，绝不能接 os.Stdin——repl.go 的 readLines goroutine
// 正阻塞在同一个 os.Stdin 上，抢起来胜负随时序漂移。行为上很难在单测里
// 稳定复现这种竞态，扫源码是唯一可靠的钉法。
func TestSpawnSourceNeverReadsOSStdin(t *testing.T) {
	path := turnSourceFile(t, "spawn.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败: %v", path, err)
	}
	if strings.Contains(string(data), "os.Stdin") {
		t.Errorf("%s 里出现了 os.Stdin：子进程的 stdin 必须接 null device，绝不能接编排器自己的输入句柄（runner.go 顶部注释、09a Spawner 一节）", path)
	}
}

// turnPackageSourceDir 复用 render_test.go 里 turnSourceFile 同一套定位逻辑，
// 只是这里要的是目录本身而不是某个具体文件。
func turnPackageSourceDir(t *testing.T) string {
	t.Helper()
	return filepath.Dir(turnSourceFile(t, "runner.go"))
}

// TestSpawnedCommandDirAndEnv 覆盖 K 节子进程契约的另外两条：
//   - Command.Dir 非空且不等于进程当前工作目录（继承启动目录会让三家 CLI
//     读到 cwd 下的项目级指令文件，09 B 节/06 第 4 节）；
//   - Command.Env 为 nil（Step 6 一律不改环境，nil 表示原样继承，
//     spawn.go 顶部注释：「改了会把登录态一起隔离掉」）。
//
// 走一次完整的 Ask（用假 Spawner 捕获真正传下去的 Command），比只读
// runner.go 的源码更可信：这里验证的是 Runner 组装出来的实际值，
// 不是「代码看起来应该这样写」。
func TestSpawnedCommandDirAndEnv(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService(t)
	convID := newConv(t, svc)
	joinAgent(t, svc, convID, testAgentClaude)
	postUser(t, svc, convID, "验证子进程契约")

	r := newTestRunner(t, cfg, svc)
	fs := &fakeSpawner{}
	fs.enqueueSuccess(claudeT1JSON)
	r.spawn = fs.spawn

	if _, err := r.Ask(context.Background(), convID, testAgentClaude); err != nil {
		t.Fatalf("Ask 失败: %v", err)
	}
	if len(fs.calls) != 1 {
		t.Fatalf("期望 Spawner 恰好被调用 1 次，实际 %d 次", len(fs.calls))
	}
	cmd := fs.calls[0].cmd

	if cmd.Dir == "" {
		t.Error("Command.Dir 为空：子进程会继承 council 自己的启动目录")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() 失败: %v", err)
	}
	if sameDir(cmd.Dir, cwd) {
		t.Errorf("Command.Dir = %q 等于进程当前工作目录 %q，期望是 <workDir>/<agentID> 这个独立目录", cmd.Dir, cwd)
	}
	if cmd.Env != nil {
		t.Errorf("Command.Env = %v，期望 nil（Step 6 一律原样继承环境，不改 HOME）", cmd.Env)
	}
}

func sameDir(a, b string) bool {
	ca, err1 := filepath.Abs(a)
	cb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	return filepath.Clean(ca) == filepath.Clean(cb)
}
