package repl

import (
	"time"

	"council/internal/config"
)

// cliadvice.go 是首次运行自动配置（00_DECISIONS.md #48，OB1）落在 repl 的两件事：
// 把识别失败的说法导出给 app 的首次运行用；/add 列完识别结果之后，对本机装了、
// 配置 [providers] 里还没有的工具各打一行提示和要追加的两行。只打提示，不写文件。

// DiscoveryFailureText 是 discoveryFailureText 的导出包装：识别失败的说法只有
// addmodel.go 那一份，app 的首次运行也用它。
func DiscoveryFailureText(err error, timeout time.Duration) string {
	return discoveryFailureText(err, timeout)
}

// SetCLIDetector 装上 /add 用的本机工具检测。零值 nil 是不检测、不打提示，所以直接
// New 出来的 REPL（既有测试）不受本机装了什么影响；app 装配时装 config.DetectProviders。
// 只许在 Run 或第一次 Submit 之前调。
func (r *REPL) SetCLIDetector(f func() []config.DetectedProvider) {
	r.detectCLIs = f
}

// printUnconfiguredCLIs 在 /add 列完识别结果之后调用。检测不缓存：每次现算
// （只看 PATH 与几处安装位置、不起进程），跟着当前 r.cfg 比，所以追加并
// /config reload --apply 之后提示就不再出现。
func printUnconfiguredCLIs(r *REPL) error {
	if r.detectCLIs == nil {
		return nil
	}
	for _, d := range r.detectCLIs() {
		if d.Path == "" {
			continue
		}
		if _, ok := r.cfg.Providers[d.ID]; ok {
			continue
		}
		if err := r.printf("本机装了 %s（%s），配置里还没有：要用它，把下面两行追加到 %s 末尾，再敲 /config reload --apply 与 /add --refresh\n", d.ID, d.Path, r.rulesCtx.ConfigPath); err != nil {
			return err
		}
		if err := r.write(config.ProviderBlockTOML(d.ID, d.Path)); err != nil {
			return err
		}
	}
	return nil
}
