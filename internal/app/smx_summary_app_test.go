package app

// SM1 §2.6：真 app.Run、真 store、真 config.Load、假 CLI（本包 smoke_test.go 的 TestMain）。
// [summary] 追加在配置文件末尾，总结成员没写 role（由 Load 补默认值）。
// 引用 CO1 的 Options.Verbose，在两刀都落地之前编译不过是预期的红；不用 runAppForTest（它固定 Verbose: true）。

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// smxAppCfgBody 是 §2.6 冻结的配置文本。
func smxAppCfgBody(dataDir string) string {
	return fmt.Sprintf(`data_dir = %s

[providers.claude]
command = %s

[agents.c1]
display_name = "成员一"
provider = "claude"
model = "opus"
effort = "high"
role = "smx"
permissions = "read"
mode = "discussion"

[agents.c2]
display_name = "成员二"
provider = "claude"
model = "opus"
effort = "high"
role = "smx"
permissions = "read"
mode = "discussion"

[agents.sm]
display_name = "总结"
provider = "claude"
model = "opus"
effort = "high"
permissions = "read"
mode = "discussion"

[summary]
agent = "sm"
`, tomlLiteral(dataDir), tomlLiteral(os.Args[0]))
}

// smxAppRoleText 是 §3.1 的默认 role（测试侧独立抄写）。
const smxAppRoleText = "你是这场讨论的总结人，不参与讨论。每次被叫到时，读用户的问题和各成员的发言，做一致性判断：他们在哪些点上一致，在哪些点上有分歧、各自的依据是什么，有没有人答非所问或明显出错。你的话只给用户看，其余成员看不到；不要替任何一方继续论证，不要提出新的方案。用中文回答：先用一句话给出总的判断，再分条列出一致之处与分歧之处，写得简短。"

var (
	smxAppHeadRe  = regexp.MustCompile(`（\d+\.\ds）`)
	smxAppTotalRe = regexp.MustCompile(`^总用时 \d+\.\ds$`)
)

// smxAppTail 取「尾部」：按行切，每行反复去掉行首 council> ，从以「c2 已加入会话 」开头的那一行之后开始收集，
// 每行做成员头规范化。
func smxAppTail(out string) []string {
	var tail []string
	started := false
	for _, l := range strings.Split(out, "\n") {
		for strings.HasPrefix(l, "council> ") {
			l = strings.TrimPrefix(l, "council> ")
		}
		if !started {
			if strings.HasPrefix(l, "c2 已加入会话 ") {
				started = true
			}
			continue
		}
		tail = append(tail, smxAppHeadRe.ReplaceAllString(l, "（T）"))
	}
	return tail
}

func smxAppHas(lines []string, want string) bool {
	for _, l := range lines {
		if l == want {
			return true
		}
	}
	return false
}

func smxReadFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("读 %s 失败: %v", p, err)
	}
	return string(b)
}

// TestSmxAppSummaryEndToEnd 是 A1：[summary] 追加在末尾经真 Load / Validate 进了真 Runner。
//
// 【判别力】verbose=false 钉 app 默认简洁档下的总结格式与位置（总结在总用时之前）；verbose=true 钉详细档照裁判轮格式
// （outcome 行、总结轮结算，不含总结头与总用时）；两档都钉 /log 里总结那一行的 TO 列是 sm（Recipients 写成 nil 或 user 时
// 这一列变，红）、默认 role 经真 Load 进了真正写给 CLI 的身份文件（Load 不补或措辞改动时红），而 c1 的 role 没被动
// （给所有 agent 都补时 c1 的身份文件不变，这条是「只补总结成员」在真链路上的对照）。两个子测试只差 Verbose。
func TestSmxAppSummaryEndToEnd(t *testing.T) {
	const script = "/new t\n/join c1\n/join c2\n裸文本\n/round c1 c2 --go\n/log 30\n/quit\n"
	for _, tc := range []struct {
		name    string
		verbose bool
	}{{"verbose=false", false}, {"verbose=true", true}} {
		t.Run(tc.name, func(t *testing.T) {
			isolateAppEnv(t)
			t.Setenv("COUNCIL_FAKE_CLI", "1")
			dir := t.TempDir()
			dataDir := filepath.Join(dir, "data")
			cfgPath := writeTestConfig(t, dir, smxAppCfgBody(dataDir))
			var out, errOut bytes.Buffer
			done := make(chan error, 1)
			go func() {
				done <- Run(context.Background(), Options{ConfigPath: cfgPath, In: strings.NewReader(script), Out: &out, Err: &errOut, Verbose: tc.verbose, NoAuto: true})
			}()
			var err error
			select {
			case err = <-done:
			case <-time.After(60 * time.Second):
				t.Fatalf("app.Run 在 60 秒内没有返回")
			}
			if err != nil {
				t.Fatalf("app.Run 返回 %v；Out:\n%s\nErr:\n%s", err, out.String(), errOut.String())
			}
			tail := smxAppTail(out.String())
			if !tc.verbose {
				if len(tail) < 6 {
					t.Fatalf("尾部太短: %q", tail)
				}
				heads := []string{tail[1], tail[2]}
				sort.Strings(heads)
				if tail[0] != "本轮问 2 个: c1, c2" || heads[0] != "成员一（T）: OK" || heads[1] != "成员二（T）: OK" ||
					tail[3] != "本轮问 1 个: sm" || tail[4] != "总的来看（总结 做总结）：OK" || !smxAppTotalRe.MatchString(tail[5]) {
					t.Fatalf("简洁档尾部前 6 行不对: %q", tail[:6])
				}
			} else {
				joined := strings.Join(tail, "\n")
				if !smxAppHas(tail, "outcome spoke  agent sm") || !smxAppHas(tail, "总结轮结算") {
					t.Fatalf("详细档尾部应当含整行「outcome spoke  agent sm」与「总结轮结算」；尾部: %q", tail)
				}
				if strings.Contains(joined, "总的来看") || strings.Contains(joined, "总用时") {
					t.Fatalf("详细档尾部不应当含「总的来看」或「总用时」；尾部: %q", tail)
				}
			}
			if !smxAppHas(tail, "10   sm      agent   sm  claude/opus/high  OK") {
				t.Fatalf("/log 里应当有总结那一行（seq 10、TO 列 sm）；尾部: %q", tail)
			}
			if got, want := smxReadFile(t, filepath.Join(dataDir, "workdir", "sm", "council-role.txt")), "You are 总结 (id=sm).\nProvider: claude\n"+smxAppRoleText; got != want {
				t.Fatalf("sm 的身份文件不对\n实际: %q\n期望: %q", got, want)
			}
			if got, want := smxReadFile(t, filepath.Join(dataDir, "workdir", "c1", "council-role.txt")), "You are 成员一 (id=c1).\nProvider: claude\nsmx"; got != want {
				t.Fatalf("c1 的身份文件不对\n实际: %q\n期望: %q", got, want)
			}
		})
	}
}
