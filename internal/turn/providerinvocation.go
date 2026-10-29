package turn

// providerinvocation.go 把 [providers.<id>] 里的 args / env 落到真实进程上，
// 并让它们进入身份指纹 —— 两者必须是同一个决定：改了参数就等于换了一种
// 调用方式，旧会话的前缀是按旧参数建的，不能继续沿用。

import (
	"os"
	"runtime"
	"strings"

	"council/internal/config"
	"council/internal/provider"
)

// applyProviderInvocation 把 provider 级的 args / env 落到这轮要起的进程上。
//
// args 追加在适配器生成的 argv **之后**：适配器已经决定了自己的参数顺序，
// 用户加的东西排最后最不容易把适配器自己的旗标挤到错的位置；需要精确控制
// 顺序的场合应该改适配器，而不是在这里插队。
//
// env 是「覆盖」而不是「替换」：provider.Command.Env 的语义是完整环境，
// 所以这里先拿 os.Environ()，去掉与覆盖项同名的旧值，再接上新值。Windows
// 的 CreateProcess 拿到重复变量名的块时行为不确定，去重是必须的。
func applyProviderInvocation(cmd *provider.Command, pc config.ProviderConfig) {
	if cmd == nil || (len(pc.Args) == 0 && len(pc.Env) == 0) {
		return
	}
	if len(pc.Args) > 0 {
		cmd.Args = append(cmd.Args, pc.Args...)
	}
	if len(pc.Env) > 0 {
		cmd.Env = mergeProviderEnv(os.Environ(), pc.Env)
	}
}

// mergeProviderEnv 返回 base 去掉被 overrides 覆盖的键之后，再接上 overrides。
func mergeProviderEnv(base, overrides []string) []string {
	names := make(map[string]bool, len(overrides))
	for _, entry := range overrides {
		if eq := strings.IndexByte(entry, '='); eq > 0 {
			names[envKey(entry[:eq])] = true
		}
	}
	out := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		eq := strings.IndexByte(entry, '=')
		if eq > 0 && names[envKey(entry[:eq])] {
			continue
		}
		out = append(out, entry)
	}
	return append(out, overrides...)
}

// envKey 归一化环境变量名。Windows 上 PATH 与 Path 是同一个变量，拿它去重
// 必须按大小写不敏感比较；其它平台保持原样（大小写敏感是 POSIX 的事实）。
func envKey(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(name)
	}
	return name
}
