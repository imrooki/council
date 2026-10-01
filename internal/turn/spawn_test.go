package turn

// 本文件覆盖 acfabb7 那次改动给 spawn.go 加的两块新代码，直接测
// spawnProcess 本身（同包可见的未导出函数），不经过 Runner：
//
//  1. ErrShimNewlineArg 闸：.cmd / .bat 垫片的可执行文件，任一 argv 元素
//     含换行就在起进程之前拒绝。防的是真机事故：Windows 上 claude.cmd 由
//     cmd.exe 再解析一次命令行，换行被当成命令分隔符，命令行在第一个换行处
//     被截断——硬失败不是最坏的，最坏的是截断后剩下的部分仍然合法，
//     claude 会带着被截短的系统提示跑完，退出码 0，JSON 合法，游标照推，
//     没有任何一层报错。
//  2. Command.Files 的落盘：起进程之前把 Files 逐个写到磁盘，写失败就不
//     起进程——少了身份块的那一轮跑出来是错的，而且错得看不出来。
//
// 两处都不需要真的跑一个有意义的子进程：正例用系统自带的 cmd.exe（一定
// 存在、瞬间退出），反例（.cmd/.bat 判据）用一个压根不存在的假路径就够——
// 闸如果生效，函数在 exec.Command 出现之前就返回了，文件存不存在无关紧要；
// 闸如果没生效，得到的会是「找不到文件」那一类错误而不是 ErrShimNewlineArg，
// 两者用 errors.Is 就能分清，不需要真的启动一个进程来分辨。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"council/internal/provider"
)

// realShortLivedExe 是本机一定存在的 cmd.exe，`/c exit 0` 让它立刻退出。
// 用真实存在的可执行文件而不是假路径，是因为「起没起进程」这条断言在
// Command.Files 那组用例里必须有真凭实据（文件真的落盘了、或者反过来
// 真的没有任何子进程side effect），假路径证明不了这件事。
const realShortLivedExe = `C:\Windows\System32\cmd.exe`

func requireRealShortLivedExe(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(realShortLivedExe); err != nil {
		t.Skipf("本机没有 %s，跳过需要真实进程的用例: %v", realShortLivedExe, err)
	}
}

// ---------------------------------------------------------------------------
// ErrShimNewlineArg
// ---------------------------------------------------------------------------

// TestSpawnProcessRejectsNewlineInShimArgs 覆盖 .cmd / .bat（含大小写变体）
// 任一 argv 元素含 \n 或 \r 时的拒绝，且断言进程没有被起来、错误信息点得出
// 是第几个参数。
//
// 用一个压根不存在的可执行文件路径：spawnProcess 内部先 exec.LookPath，
// 找不到时 path 保持原样（仍然带着 .cmd/.bat 后缀），换行检查只看这个
// 字符串本身的扩展名，不需要文件真的存在——如果检查没生效，函数会试着
// exec.Command(...).Start()，那时才会因为文件不存在而报另一类错误，
// 与 ErrShimNewlineArg 用 errors.Is 一分就清楚，不需要真的起一个进程
// 才能分辨「起没起」。
func TestSpawnProcessRejectsNewlineInShimArgs(t *testing.T) {
	cases := []struct {
		name    string
		ext     string // 拼在假路径末尾
		badArg  string
		argIdx  int
		wantHit bool // 是否期望被这道闸拦下
	}{
		{name: ".cmd 含换行", ext: ".cmd", badArg: "line1\nline2", argIdx: 1, wantHit: true},
		{name: ".bat 含换行", ext: ".bat", badArg: "line1\nline2", argIdx: 1, wantHit: true},
		{name: "大写 .CMD 同样拦（实现用 strings.ToLower）", ext: ".CMD", badArg: "line1\nline2", argIdx: 1, wantHit: true},
		{name: "混合大小写 .Bat 同样拦", ext: ".Bat", badArg: "line1\nline2", argIdx: 1, wantHit: true},
		{name: "只含 \\r 也拦，不只是 \\n", ext: ".cmd", badArg: "line1\rline2", argIdx: 1, wantHit: true},
		{name: ".exe 反例：同样含换行但不是垫片，不拦", ext: ".exe", badArg: "line1\nline2", argIdx: 1, wantHit: false},
		{name: "无扩展名反例：同样含换行，不拦", ext: "", badArg: "line1\nline2", argIdx: 1, wantHit: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakePath := filepath.Join(t.TempDir(), "does-not-exist"+tc.ext)
			cmd := provider.Command{
				Path: fakePath,
				Args: []string{"--flag", tc.badArg},
				Dir:  t.TempDir(),
			}
			_, _, exitCode, killed, err := spawnProcess(context.Background(), cmd)

			hit := errors.Is(err, ErrShimNewlineArg)
			if hit != tc.wantHit {
				t.Fatalf("errors.Is(err, ErrShimNewlineArg) = %v，期望 %v；err=%v", hit, tc.wantHit, err)
			}
			if tc.wantHit {
				// 进程没有被起来：这道闸在 exec.Command 出现之前就返回，
				// exitCode/killed 停在零值，且错误信息里点得出是第几个参数。
				if exitCode != 0 || killed {
					t.Errorf("命中这道闸时 exitCode/killed 应当是零值，实际 exitCode=%d killed=%v", exitCode, killed)
				}
				wantSubstr := "args[" + strconv.Itoa(tc.argIdx) + "]"
				if !strings.Contains(err.Error(), wantSubstr) {
					t.Errorf("错误信息 %q 里没有点名 %q", err.Error(), wantSubstr)
				}
			} else {
				// 反例：假路径本来就不存在，走到这里必然是另一类「起进程
				// 失败」的错误（文件找不到），但它绝不能是 ErrShimNewlineArg——
				// 这才是这条反例真正要证明的事：.exe / 无扩展名的可执行文件，
				// 这道闸完全不参与判断，错误原因必须来自别处（这里是路径
				// 不存在），而不是被这道闸拦下。
				if err == nil {
					t.Fatal("假路径不存在，spawnProcess 应当报错（原因是路径找不到，不是被这道闸拦下），实际返回 nil")
				}
			}
		})
	}
}

// TestSpawnProcessNewlineGuardIgnoresNonShimRealExecutable 是上面反例的
// 加强版：用一个真实存在、会真的被启动的 .exe（cmd.exe），确认「不是垫片」
// 这条路径不仅仅是「没被这道闸拦下」，而是确确实实被交给了 exec.Command 去
// 启动——真实存在的路径 + 含换行的 argv 元素，若这道闸误伤了 .exe，
// spawnProcess 会在 Start 之前就返回 ErrShimNewlineArg；若没有误伤，
// 它会真的尝试起进程（这里用一个必然存在的可执行文件，断言 err 不是
// ErrShimNewlineArg，且要么进程真的跑完，要么失败原因与这道闸无关）。
func TestSpawnProcessNewlineGuardIgnoresNonShimRealExecutable(t *testing.T) {
	requireRealShortLivedExe(t)

	cmd := provider.Command{
		Path: realShortLivedExe,
		// 换行放在一个 cmd.exe 不会因此报出「文件找不到」之外错误的位置：
		// 只要 spawnProcess 没有在这里被 ErrShimNewlineArg 拦下，本用例的
		// 断言目的就达到了，不需要要求 cmd.exe 一定把这行内容执行成功。
		Args: []string{"/c", "exit 0 & rem line1\nrem line2"},
		Dir:  t.TempDir(),
	}
	_, _, _, _, err := spawnProcess(context.Background(), cmd)
	if errors.Is(err, ErrShimNewlineArg) {
		t.Fatalf(".exe 不是垫片，换行检查不该参与判断，实际仍被 ErrShimNewlineArg 拦下: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Command.Files 落盘
// ---------------------------------------------------------------------------

// TestSpawnProcessWritesFilesBeforeExec 覆盖正例：Files 里的每个路径都必须
// 在进程启动前落盘，内容逐字节等于给的值。
func TestSpawnProcessWritesFilesBeforeExec(t *testing.T) {
	requireRealShortLivedExe(t)

	dir := t.TempDir()
	rolePath := filepath.Join(dir, "council-role.txt")
	const content = "You are Alice (id=alice).\nProvider: claude\n只做审阅"

	cmd := provider.Command{
		Path:  realShortLivedExe,
		Args:  []string{"/c", "exit 0"},
		Dir:   dir,
		Files: map[string][]byte{rolePath: []byte(content)},
	}
	_, _, exitCode, killed, err := spawnProcess(context.Background(), cmd)
	if err != nil {
		t.Fatalf("spawnProcess 失败: %v", err)
	}
	if killed || exitCode != 0 {
		t.Fatalf("cmd.exe /c exit 0 应当正常退出，实际 exitCode=%d killed=%v", exitCode, killed)
	}

	got, readErr := os.ReadFile(rolePath)
	if readErr != nil {
		t.Fatalf("读回 %s 失败: %v", rolePath, readErr)
	}
	if string(got) != content {
		t.Errorf("文件内容 = %q，期望逐字节等于 %q", string(got), content)
	}
}

// TestSpawnProcessFilesOverwriteNotAppend 覆盖覆盖语义：同一路径先后写两次
// 不同内容，最终内容是第二次的，不是两次拼接——runner.go 的注释原话
// 「每轮覆盖同一个路径」，靠的正是 os.WriteFile 本身「覆盖」而不是
// os.OpenFile(O_APPEND) 那种「追加」的语义。
func TestSpawnProcessFilesOverwriteNotAppend(t *testing.T) {
	requireRealShortLivedExe(t)

	dir := t.TempDir()
	rolePath := filepath.Join(dir, "council-role.txt")

	first := provider.Command{
		Path: realShortLivedExe, Args: []string{"/c", "exit 0"}, Dir: dir,
		Files: map[string][]byte{rolePath: []byte("第一轮的身份块，很长很长，用来确认覆盖之后不会有残留字节")},
	}
	if _, _, _, _, err := spawnProcess(context.Background(), first); err != nil {
		t.Fatalf("第一次 spawnProcess 失败: %v", err)
	}

	second := provider.Command{
		Path: realShortLivedExe, Args: []string{"/c", "exit 0"}, Dir: dir,
		Files: map[string][]byte{rolePath: []byte("短")},
	}
	if _, _, _, _, err := spawnProcess(context.Background(), second); err != nil {
		t.Fatalf("第二次 spawnProcess 失败: %v", err)
	}

	got, err := os.ReadFile(rolePath)
	if err != nil {
		t.Fatalf("读回 %s 失败: %v", rolePath, err)
	}
	if string(got) != "短" {
		t.Errorf("文件内容 = %q，期望恰好是第二次写入的 %q（覆盖，不是追加；若是追加，长度会比这长得多）", string(got), "短")
	}
}

// TestSpawnProcessWritesAllFiles 覆盖「多个文件时全部落盘」。
func TestSpawnProcessWritesAllFiles(t *testing.T) {
	requireRealShortLivedExe(t)

	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.txt")
	pathB := filepath.Join(dir, "b.txt")

	cmd := provider.Command{
		Path: realShortLivedExe, Args: []string{"/c", "exit 0"}, Dir: dir,
		Files: map[string][]byte{
			pathA: []byte("内容 A"),
			pathB: []byte("内容 B"),
		},
	}
	if _, _, _, _, err := spawnProcess(context.Background(), cmd); err != nil {
		t.Fatalf("spawnProcess 失败: %v", err)
	}
	for path, want := range map[string]string{pathA: "内容 A", pathB: "内容 B"} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读回 %s 失败: %v", path, err)
		}
		if string(got) != want {
			t.Errorf("%s 内容 = %q，期望 %q", path, string(got), want)
		}
	}
}

// TestSpawnProcessFilesWriteFailureAbortsBeforeExec 覆盖失败分支：
// 构造一个必然写不成的路径（把一个已存在的普通文件当成父目录的一段，
// os.MkdirAll 在这种路径上必然失败），断言 spawnProcess 返回错误，
// 且进程没有真的起来——用一个会留下可观测痕迹的命令（写一个 marker 文件）
// 来证明「没起来」，而不是仅凭「返回了错误」去推断，那样证明力不够：
// 如果 Files 写入失败之后代码没有 fail-closed、继续往下走并起了进程，
// marker 文件依然会被创建，只是函数最终返回值里带着一个无关的错误——
// 这条用例要挡的正是这种「返回了错误，但钱已经花了」的半吊子失败。
func TestSpawnProcessFilesWriteFailureAbortsBeforeExec(t *testing.T) {
	requireRealShortLivedExe(t)

	dir := t.TempDir()
	// blocker 是一个普通文件，不是目录。把它当成待写文件的父目录的一段
	// （blocker/subdir/role.txt），os.MkdirAll(blocker/subdir, ...) 会因为
	// blocker 不是目录而失败。
	blocker := filepath.Join(dir, "i-am-a-file-not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("准备 blocker 文件失败: %v", err)
	}
	badPath := filepath.Join(blocker, "subdir", "role.txt")

	markerPath := filepath.Join(dir, "marker.txt")
	// 如果进程真的被起来了，这个命令会把 marker.txt 写出来；断言它不存在
	// 就是断言进程没有被启动。
	cmd := provider.Command{
		Path: realShortLivedExe,
		Args: []string{"/c", "echo ran > \"" + markerPath + "\""},
		Dir:  dir,
		Files: map[string][]byte{
			badPath: []byte("这份身份块永远不该被真的送进子进程"),
		},
	}
	_, _, exitCode, killed, err := spawnProcess(context.Background(), cmd)
	if err == nil {
		t.Fatal("Files 写盘失败时 spawnProcess 应当返回错误，实际返回 nil")
	}
	if exitCode != 0 || killed {
		t.Errorf("写盘失败应当在 exec.Command 之前就返回，exitCode/killed 应当是零值，实际 exitCode=%d killed=%v", exitCode, killed)
	}
	if _, statErr := os.Stat(markerPath); !os.IsNotExist(statErr) {
		t.Errorf("marker 文件存在（或 stat 返回了非「不存在」的错误 %v），说明子进程被启动了，Files 写盘失败没有 fail-closed", statErr)
	}
}
