package command

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// Registry 是一张命令表：元数据在这里，处理函数由调用方以类型参数 H 决定。
//
// 零值可用（第一次 Register 时建索引）。Register 之后再读是安全的：
// Lookup 与 List 只读，不写任何共享状态，并发调用无竞态。Register 本身不是
// 并发安全的 —— 它只在装配时调用一次，那正是撞名 panic 该发生的时候。
//
// 注册顺序不对外：它在内部只用来保存全部条目。对外一律以 List 的 Name 字典序为准，
// 因为 map 的随机顺序与「谁先注册」都不能出现在 /help 或 GUI 面板里。
type Registry[H any] struct {
	specs []Spec         // 注册顺序的条目；List 会自己排序
	runs  []H            // 与 specs 同下标
	index map[string]int // 命令名与别名 -> specs 下标
}

// Register 登记一条命令：元数据 spec，负载 run。
//
// 撞名、撞别名、空名字、带斜杠的名字、含空白的名字一律 panic。这些都不是运行时
// 错误，而是编程错误：注册表是编译期就该确定的静态表，一个查不到的名字只会在
// 用户敲下它时变成「未知命令」，那时离出错的地方已经很远了，没人能顺着那行提示
// 找到漏写的那一处。panic 发生在启动装配或首个测试用例里，改一行就好。
//
// 名字与别名共用一个命名空间：别名 "r" 会挡住另一条命令的名字 "r"，
// 反之亦然。它们本来就该互相冲突 —— 查找时两者没有任何区别。
func (r *Registry[H]) Register(spec Spec, run H) {
	keys := make([]string, 0, 1+len(spec.Aliases))
	keys = append(keys, spec.Name)
	keys = append(keys, spec.Aliases...)
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		switch {
		case key == "":
			panic(fmt.Sprintf("command: 命令 %q 有空名字或空别名", spec.Name))
		case strings.HasPrefix(key, "/"):
			// 表里不带斜杠：屏幕上写作 /round，查表时由分发方剥掉斜杠，
			// 注册成 "/round" 的那条永远查不到，而且肉眼看不出哪里错了。
			panic(fmt.Sprintf("command: %q 带斜杠，注册表里的名字不带斜杠", key))
		case strings.ContainsFunc(key, unicode.IsSpace):
			// 输入行按空白切出第一个词作为命令名，含空白的名字不可能被查中。
			panic(fmt.Sprintf("command: %q 含空白，按空白切分的命令名永远查不到它", key))
		case key != strings.ToLower(key):
			// 查找大小写敏感，屏幕上抄下来的是小写；注册成 "Help" 的命令
			// 只能被 /Help 敲中，而 /help 会落到别处（或未知命令）。
			panic(fmt.Sprintf("command: %q 不是小写，命令名只有一个小写拼法", key))
		}
		if _, dup := seen[key]; dup {
			panic(fmt.Sprintf("command: 命令 %q 把 %q 同时写进了名字与别名", spec.Name, key))
		}
		if _, dup := r.index[key]; dup {
			panic(fmt.Sprintf("command: %q 已经被另一条命令占用（名字与别名共用一个命名空间）", key))
		}
		seen[key] = struct{}{}
	}

	if r.index == nil {
		r.index = make(map[string]int, len(keys))
	}
	// 元数据存副本：调用方手里的 spec 之后被改，不该影响已经登记的命令。
	i := len(r.specs)
	r.specs = append(r.specs, spec.clone())
	r.runs = append(r.runs, run)
	r.index[spec.Name] = i
	for _, alias := range spec.Aliases {
		r.index[alias] = i
	}
}

// Lookup 按命令名或别名查一条命令。别名解析只在这里：分发的调用方不知道
// 哪些名字是别名，也不需要知道 —— 它拿到的要么是针对这条命令的那一份 Spec，
// 要么是 ok == false。
//
// 大小写敏感，不做小写折叠：命令名只有一个小写拼法（与 dsh 一致）。
// 折叠会让 /Round 也执行 /round，而用户从 /help 上抄下来的永远是小写。
func (r *Registry[H]) Lookup(name string) (Spec, H, bool) {
	i, ok := r.index[name]
	if !ok {
		var zero H
		return Spec{}, zero, false
	}
	return r.specs[i].clone(), r.runs[i], true
}

// List 返回属于 scope 的命令元数据，按 Name 的字节序排列。
//
// 排序是规格要求的：/help、GUI 面板与 council commands -json 每次都要同样的顺序，
// 测试才能逐字节断言。slice 已排序，不依赖 map 迭代。
//
// Scopes 为空表示三个入口都有。scope 传空串时只返回 Scopes 显式包含它的命令 ——
// 空不是「全部」的意思，调用方要写 List(command.ScopeGUI) 这样的显式值；
// 要整张表请用 ListAll。
func (r *Registry[H]) List(scope Scope) []Spec {
	specs := make([]Spec, 0, len(r.specs))
	for _, spec := range r.specs {
		if spec.InScope(scope) {
			specs = append(specs, spec.clone())
		}
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	return specs
}

// ListAll 返回全部命令，不分入口，按 Name 的字节序排列。
//
// 给「列一遍命令」的消费者用（council commands -json 与将来的 GUI 面板）：
// 它们要的是命令目录本身，不是某个入口的可见集。与 List 分开是有意的——
// 混成一个「传空串就是全部」的约定，会让过滤失效时没人发现。
func (r *Registry[H]) ListAll() []Spec {
	specs := make([]Spec, 0, len(r.specs))
	for _, spec := range r.specs {
		specs = append(specs, spec.clone())
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	return specs
}
