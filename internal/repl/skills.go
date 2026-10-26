package repl

// skills.go 实现只读的 /skills。
//
// skill 的正文按官方语义只在被调用时才进上下文，Council 没有调用语义；这里
// 只把 internal/serve 已经在用的 config.ScanClaudeSkills 结果排成两列给人看：
// 不执行、不把正文拼进任何 prompt、不写文件，也不合并进 cfg。类型直接复用
// config.Skill，不为展示另造一份结构 —— 两份结构早晚在字段上走失同步。
//
// 扫的是启动目录（r.projectDir），与 /new 的盖章、/resume 的过滤同一个值：
// 项目级 skill 从这个目录沿父目录找到仓库根，和 Claude Code 从启动目录出发
// 的规则一致。零值空串时只扫用户级，不在命令里另读 os.Getwd —— 命令行为
// 不该依赖「构造 REPL 之后进程又切过目录」这种看不见的状态。

import (
	"context"
	"fmt"

	"council/internal/config"
)

// msgNoSkills 是一个 skill 都没扫到时的答复。空列表不是错误，用户需要知道的
// 是去哪儿找：.claude/skills 下每个目录要有一份 SKILL.md。
const msgNoSkills = "没有发现 skills（.claude/skills 下没有 SKILL.md）"

// cmdSkills 打印 .claude/skills 扫到的 skill，每条一行：名字 + 描述。
//
// 顺序照 ScanClaudeSkills 的自带优先级（用户级在前，项目层由近到远），不重排：
// 重名已经由扫描器按优先级去掉，剩下的顺序本身就是「哪一条胜出」的说明。
//
// 没有描述时第二列显示目录名（Skill.ID）：Name 可以被 frontmatter 覆盖，目录名
// 才是这台机器上的身份；留空会让这一行只剩一个名字，而两个来源不同的名字
// 光看一列分不清是哪一个。名字与描述都过 oneLine：description 允许 | / >
// 块标量，原样写进 tabwriter 会把一条 skill 拆成几行、并打乱整张表的对齐。
func cmdSkills(_ context.Context, r *REPL, _ cmdArgs) error {
	skills := config.ScanClaudeSkills(r.projectDir)
	if len(skills) == 0 {
		return r.printf("%s\n", r.t("tui.skills.none", msgNoSkills))
	}
	tw := r.newTabWriter()
	for _, skill := range skills {
		desc := skill.Description
		if desc == "" {
			desc = skill.ID
		}
		// 描述通常是中文，放最后一列才不会被 tabwriter 按 rune 数错算的宽度
		// 撑歪（与 /agents 不显示 display_name 同一个原因，见 newTabWriter）。
		fmt.Fprintf(tw, "%s\t%s\n", oneLine(skill.Name), oneLine(desc))
	}
	return tw.Flush()
}
