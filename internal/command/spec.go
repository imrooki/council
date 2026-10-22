// Package command 是 Council 全部斜杠命令的元数据与注册表。
//
// 单独成包，是因为命令的消费者不止一处：TUI 的 /help、M2 的 GUI 命令面板、
// council commands -json（给 dsh 工具）要读同一份 Spec。如果元数据留在
// internal/repl，任何只想列一遍命令的调用方都得把 REPL 的整条依赖链
// （turn、provider、SQLite）一起拖进来。
//
// 处理函数不在这里：它的第一个有意义的参数是 *repl.REPL，放进本包会把依赖
// 方向反过来。所以 Registry 用类型参数把「跟元数据一起登记的那个东西」留给
// 调用方自己定：repl 登记的是它的命令表项（前置条件 + 处理函数），本包只保证
// 名字、别名、顺序与查找规则只有一份。
package command

// Scope 是一个入口的名字：同一条命令在 TUI 里可能可用，在 GUI 面板里未必。
//
// 只有确实要按入口区分的命令才填 Spec.Scopes；为空表示三个入口都能用 ——
// 这是绝大多数命令的情形，也是新命令不需要想这件事的原因。
type Scope string

const (
	// ScopeTUI 是行式 REPL 与 TUI 这一边（/help 列的就是它）。
	ScopeTUI Scope = "tui"
	// ScopeGUI 是 M2 的 GUI 命令面板与 dsh 工具那一边。
	ScopeGUI Scope = "gui"
	// ScopeHeadless 是 council ask 这类不读命令行的入口。今天没有命令填它，
	// 常量先留着：它的含义由 Spec.Scopes 决定，不需要另一个开关。
	ScopeHeadless Scope = "headless"
)

// Spec 是一条命令的元数据。
//
// 字段顺序就是 council commands -json 里的键顺序（encoding/json 按结构体顺序
// 输出字段），改顺序会改 JSON 的字节，不只是排版。
type Spec struct {
	// Name 是命令名，不带斜杠：命令在屏幕上写作 /round，在表里就是 "round"。
	// 查找大小写敏感（与 dsh 一致：命令名小写）。
	Name string `json:"name"`
	// Aliases 是语义完全一致的另一个名字，不带斜杠，可空。
	// 别名不单独出现在 /help 与 JSON 的条目里：它解析到同一条 Spec。
	Aliases []string `json:"aliases"`
	// Args 是参数形状，给 GUI 面板与 JSON 消费者显示用，例如 "[id...] [--go]"。
	// 它不参与解析：真正的参数由各个处理函数自己读。
	Args string `json:"args"`
	// Summary 是一行说明。/help 打印的就是它，改动会直接改变屏幕输出。
	Summary string `json:"summary"`
	// Detail 是给 GUI 展开的较长说明，可空。
	// /help 不打印它：帮助只有一行，展开说明是面板的事。
	Detail string `json:"detail"`
	// Scopes 是这条命令可用的入口；为空表示全部。
	// 空在 JSON 里写成 []（而不是 null），消费者不必先判空。
	Scopes []Scope `json:"scopes"`
}

// InScope 报告这条命令是否属于 scope。Scopes 为空表示三个入口都算。
//
// 空 scope 不是「全部」：它不是一个入口的名字，只是零值。把它当全部会让
// List("") 悄悄返回整张表，调用方以为自己按入口过滤过了，实际没有。
func (s Spec) InScope(scope Scope) bool {
	if scope == "" {
		return false
	}
	if len(s.Scopes) == 0 {
		return true
	}
	for _, got := range s.Scopes {
		if got == scope {
			return true
		}
	}
	return false
}

// clone 返回一份不共享底层数组的副本。
//
// 注册表登记的是命令表，任何读它的人都只该拿到快照：列表会被打印、
// 被 JSON 编码，调用方一旦原地改个名字，下一次 /help 就跟着变，
// 而那种错误在屏幕上看不出是哪来的。切片只有几个元素，复制可以忽略不计。
//
// 空的 Aliases / Scopes 复制成空切片而不是 nil：JSON 里是 [] 而不是 null。
func (s Spec) clone() Spec {
	s.Aliases = append([]string{}, s.Aliases...)
	s.Scopes = append([]Scope{}, s.Scopes...)
	return s
}
