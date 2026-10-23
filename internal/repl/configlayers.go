package repl

// configlayers.go 是 /config reload 与 /add 共用的「这一次该读哪几层配置」。
//
// b4814b8 起配置是多层合并的（项目层 + 全局层），internal/app 与 internal/serve
// 都走 config.ResolveConfigLayers + config.LoadMerged。但 repl 这两条重读/写入
// 路径一直用单文件 config.Load，读到的只是最高优先层：项目层只写 [agents.x]、
// provider 放在全局层时，reload 会因「引用了不存在的 provider」被拒，/add 会因
// 与内存里的合并结果不等而被拒。两句话都在误导——文件本身没有问题。
//
// 为什么 repl 不能直接把 -config 的原值交给 ResolveConfigLayers：这个值只在
// app.Run 的 Options 里，repl 拿到的是启动时生效的那一份路径
// （RulesContext.ConfigPath = 最高优先层），拿不到 flag 本身。所以这里按默认
// 位置重新求一遍层，再和正在生效的路径对：求出来的最高优先层正是它，说明
// 这次启动走的就是「项目层 + 全局层」，按合并处理；对不上，说明用户用
// -config 或 COUNCIL_CONFIG 点名了一份文件，那种语义是「就用这一份」，只读
// 它——与改动前的单文件行为逐字节一致。这也是任务书 §B 最后一条的要求。

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"council/internal/config"
)

// reloadLayers 求这次重读要读的配置层，高优先级在前。
//
// 返回的 slices 第一项永远是「写回要落的那份文件」：/add 只往它追加，
// 绝不把合并结果整体落盘。多层时它是项目层（或唯一那层全局层），单层时
// 就是 RulesContext.ConfigPath。
//
// ResolveConfigLayers 失败（取不到用户主目录）时回落到单文件：那是一次
// 环境探测失败，不该让一份本来读得动的配置文件跟着读不了。回落之后的行为
// 与改动前完全相同，是这里能给出的最保守的答案。
func reloadLayers(configPath string) []config.Layer {
	if configPath == "" {
		// 空路径原样交下去，由 LoadMerged 的「配置层路径为空」报出来。
		// 不为它造一条错误，免得同一件事有两套措辞。
		return []config.Layer{{Path: ""}}
	}
	resolved, err := config.ResolveConfigLayers("")
	if err != nil || len(resolved) == 0 {
		return []config.Layer{{Path: configPath}}
	}
	if sameConfigPath(resolved[0].Path, configPath) {
		return resolved
	}
	return []config.Layer{{Path: configPath}}
}

// sameConfigPath 判断两个配置路径是不是同一份文件。
//
// 两边都先转成绝对路径再比：项目层与全局层由 ResolveConfigLayers 给出时
// 本来就是绝对路径，但 -config / COUNCIL_CONFIG 可以是相对路径，而 app
// 交给 repl 的是同一串原文，直接比也能对上——转绝对只是让「同一份文件、
// 两种写法」也认得出来。Windows 上路径大小写不敏感，按 EqualFold 比；
// 别的系统上大小写是身份的一部分，不能折叠。
func sameConfigPath(a, b string) bool {
	absOf := func(path string) string {
		if abs, err := filepath.Abs(path); err == nil {
			return filepath.Clean(abs)
		}
		return filepath.Clean(path)
	}
	ca, cb := absOf(a), absOf(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(ca, cb)
	}
	return ca == cb
}

// configLayerPaths 把层列表拍成路径列表，给诊断文案用。
func configLayerPaths(layers []config.Layer) []string {
	paths := make([]string, len(layers))
	for i, layer := range layers {
		paths[i] = layer.Path
	}
	return paths
}

// configLayersNotice 是「这次重读合并了哪几层」那一行；只有真的合并了多层
// 才返回非空——单文件是常态，多印一行只是噪声。
//
// 只在详细档（-verbose）打印：默认档（简洁）下用户没有要求过程信息，
// 层信息仍然会在错误文案与 /config 里出现，不会丢。
func configLayersNotice(paths []string) string {
	if len(paths) < 2 {
		return ""
	}
	return fmt.Sprintf("配置来自 %d 层（高到低）：%s\n", len(paths), strings.Join(paths, "、"))
}

// configReadErrPrefix 在多层时给读配置失败那一行补上层次。
//
// 单层时原样返回 prefix：/add 的既有测试逐字钉住了「读配置文件失败: ...」
// 那一行，不能在常见路径上多出括号。
func configReadErrPrefix(prefix string, layers []config.Layer) string {
	if len(layers) < 2 {
		return prefix
	}
	return prefix + "（合并 " + strings.Join(configLayerPaths(layers), "、") + "）"
}
