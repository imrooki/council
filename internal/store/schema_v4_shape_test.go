// 本文件覆盖 docs/12_STEP9_CONTRACT.md 3.9 第 3 条（片 2a 的一部分）：
// "v4Shape 不是别名：改 v4Shape 之后断言 v2Shape 与 v3Shape 不变"。
//
// 背景见 schema.go:330 的既有注释——`var v3Shape = v2Shape` 是两个变量指向
// 同一张 map。12 契约 3.3 节点名了同一个坑：若 v4Shape 照抄写成
// `v4Shape := v3Shape` 再改键，会连 v2Shape / v3Shape 一起改掉。
//
// map 是引用类型，这里有两种独立的坏法，缺一个都测不出另一个：
//
//   - 改键（对 v4Shape 的某个 map 键整体重新赋值，比如
//     `v4Shape["conversations"] = append(...)`）：只有在 v4Shape 与
//     v3Shape/v2Shape 是**同一张 map**（`v4Shape := v3Shape` 这种写法）
//     时才会连带改到对方，因为改的是那张共享 map 本身的键值对。
//   - 改元素（对 v4Shape 某个键取到的 slice 按下标赋值，比如
//     `v4Shape["conversations"][0] = x`）：哪怕 v4Shape 是一张全新的
//     map（`make` 出来的），只要建它的时候把内层 slice 直接搬过去
//     （`shape[table] = v3Shape[table]`，没有再 `make`+`copy` 一次），
//     v4Shape 与 v3Shape 在这张表上的 slice 仍然共享同一段底层数组，
//     按下标改元素就会连带改到 v3Shape 读到的内容——这条路 buildV2Shape
//     现有的写法（schema.go 里 v1Shape -> v2Shape 那条）已经用
//     `make([]columnShape, len(cols))` + `copy` 正确地避开了，是 v4Shape
//     应当照抄的那条路，不是 `v4Shape := v3Shape` 那条。
//
// 只测其中一种会漏掉另一种，所以下面两个子用例都要有。
package store

import (
	"fmt"
	"sort"
	"testing"
)

// cloneColumnShapes 造 cols 的一份深拷贝：全新的底层数组，之后无论 cols
// 本身还是这份拷贝谁被改动，都不会牵连另一方。用来在破坏性操作之前留一份
// 不会被连带改动的基准，供比对用。
func cloneColumnShapes(cols []columnShape) []columnShape {
	out := make([]columnShape, len(cols))
	copy(out, cols)
	return out
}

// assertColumnShapesEqual 逐元素比较两份列定义，第几个位置不对、具体是
// 长度不对还是内容不对都点名，免得只看到一个笼统的 "不相等"。
func assertColumnShapesEqual(t *testing.T, label string, got, want []columnShape) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s 长度 = %d，期望 %d\n实际: %+v\n期望: %+v", label, len(got), len(want), got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s 第 %d 列 = %+v，期望 %+v", label, i, got[i], want[i])
		}
	}
}

// TestV4ShapeConversationsIsNotAliasOfV2OrV3 钉住 12 契约 3.3 节点名的坑
// 里的一种坏法：改键——对 v4Shape 的某个 map 键整体重新赋值（比如
// `v4Shape["conversations"] = append(...)`），只有在 v4Shape 与
// v3Shape/v2Shape 是同一张 map 时才会连带改到对方。改
// v4Shape["conversations"] 之后，v2Shape["conversations"] 与
// v3Shape["conversations"] 必须一个字节都不变。用 "conversations" 这张
// 表来钉这一种坏法，是因为它是 12 契约 3.3 节点名的那张表——v4DDL 其实
// 有三条 ALTER、碰了两张老表（conversations 加 team_id，turn_usage 加
// identity_fp 与 round_id），不是只有一张，这里选的是契约点名的那张。
//
// 这个函数里"改元素"那个子用例结构性地测不出另一种坏法（浅拷贝、内层
// slice 共享底层数组），不是运气好：v4 用 append 往 "conversations"
// 里加了 team_id 列（buildV4Shape 里 `append(shape["conversations"],
// columnShape{...})`）。append 到一个 len == cap 的 slice（buildV2Shape
// 用 make(len(cols)) 造出来的，len 与 cap 总是相等）必然触发扩容、重新
// 分配一块新的底层数组——不管 buildV4Shape 那一步的拷贝写对了还是写成
// 了浅拷贝，append 这一步本身就已经把 v4Shape["conversations"] 与
// v3Shape["conversations"] 的底层数组切开了，按下标改元素不可能连带
// 改到 v3Shape/v2Shape。浅拷贝这种坏法要靠
// TestV4ShapeUntouchedTableElementIsNotAliasOfV2OrV3 去钉：它专门动态
// 挑一张 v4 没有 append 过新列的表，那张表上的 slice 才可能真的与
// v3Shape/v2Shape 共享同一段底层数组。
//
// 两个子用例改完都要用 t.Cleanup 还原，否则会污染同包其它读 v2Shape /
// v3Shape 的用例（TestSchemaShapeCoversEveryTable 等）。
func TestV4ShapeConversationsIsNotAliasOfV2OrV3(t *testing.T) {
	// 深拷贝当前的 v2Shape / v3Shape 留作比对基准。这份拷贝必须与
	// v2Shape/v3Shape 本身彻底独立：如果只是再取一次
	// `v2Shape["conversations"]`（不深拷贝），一旦 v4Shape 真的是别名，
	// 这份"基准"会跟着 v2Shape 一起被污染，用来比对就成了自己证自己。
	wantV2 := cloneColumnShapes(v2Shape["conversations"])
	wantV3 := cloneColumnShapes(v3Shape["conversations"])
	if len(wantV2) == 0 || len(wantV3) == 0 {
		t.Fatal("v2Shape/v3Shape 里 conversations 是空的，本用例的前提不成立")
	}

	t.Run("改键（对 v4Shape 的某个键整体重新赋值）不影响 v2Shape 与 v3Shape", func(t *testing.T) {
		if _, ok := v4Shape["conversations"]; !ok {
			t.Fatal(`v4Shape 里没有 "conversations" 这张表，本用例的前提不成立`)
		}
		// 故意存的是原 slice 本身（引用），不是深拷贝：下面重新赋值
		// v4Shape["conversations"] 之前从不往这个原 slice 的底层数组写字节
		// （append 到的是 cloneColumnShapes 另开的一块新内存），所以恢复
		// 这个引用是安全的。若这里存成深拷贝，一旦实现真的按浅拷贝的坏法
		// 让 v4Shape["conversations"] 与 v3Shape["conversations"] 共享同一段
		// 底层数组，cleanup 时就会把那段共享数组换成一份全新的、互不相干
		// 的拷贝——下面"改元素"那个子用例再读到的 v4Shape["conversations"]
		// 就已经不共享任何东西了，测的东西会被这次 cleanup 悄悄抹掉。
		orig := v4Shape["conversations"]
		t.Cleanup(func() {
			v4Shape["conversations"] = orig
		})

		mutated := append(cloneColumnShapes(v4Shape["conversations"]),
			columnShape{name: "tst_v4_shape_alias_probe_key", typ: "TEXT", notnull: false})
		v4Shape["conversations"] = mutated

		assertColumnShapesEqual(t, `v2Shape["conversations"]`, v2Shape["conversations"], wantV2)
		assertColumnShapesEqual(t, `v3Shape["conversations"]`, v3Shape["conversations"], wantV3)
	})

	t.Run("改元素（对 v4Shape 某个键的 slice 按下标赋值）不影响 v2Shape 与 v3Shape", func(t *testing.T) {
		cols, ok := v4Shape["conversations"]
		if !ok || len(cols) == 0 {
			t.Fatal(`v4Shape["conversations"] 为空或不存在，本用例的前提不成立`)
		}
		origElem := cols[0]
		t.Cleanup(func() {
			v4Shape["conversations"][0] = origElem
		})

		v4Shape["conversations"][0] = columnShape{
			name: "tst_v4_shape_alias_probe_elem", typ: origElem.typ, notnull: origElem.notnull,
		}

		assertColumnShapesEqual(t, `v2Shape["conversations"]`, v2Shape["conversations"], wantV2)
		assertColumnShapesEqual(t, `v3Shape["conversations"]`, v3Shape["conversations"], wantV3)
	})
}

// TestV4ShapeUntouchedTableElementIsNotAliasOfV2OrV3 补上
// TestV4ShapeConversationsIsNotAliasOfV2OrV3 留下的一个结构性盲点，
// 单独起一个新函数而不是往那个函数里再塞一个 t.Run：这条用例要在改动
// 之前动态挑表、单独取一份基准，跟已有两个子用例共用同一套开头
// （写死表名 "conversations" 的 wantV2/wantV3）合不到一起；分成新函数
// 之后，上面那两个子用例一个字节都不用动，天然满足"不许删掉或改弱
// 既有子用例"这条硬约束，不需要在合并与不动之间做取舍。
//
// 背景（已用变异验证实测过：把 buildV4Shape 临时改成 `shape[table] = cols`
// 这种浅拷贝，结果记在交付报告里，本文件不重复贴）：
// TestV4ShapeConversationsIsNotAliasOfV2OrV3 的"改元素"那个子用例探的是
// "conversations" 表，而 "conversations" 正是 v4Shape 用 append 往里
// 追加新列的表之一（buildV4Shape 给它 append 了 team_id）。
// buildV2Shape 造 v1Shape 到 v2Shape 时，非 turn_usage/provider_sessions
// 的表都是 `make([]columnShape, len(cols))` + `copy` 出来的，len 与 cap
// 相等；再对这样一个 slice 执行 append，一定放不下新元素，Go 运行时
// 必然重新分配一块更大的底层数组，把内容搬过去。不管 buildV4Shape
// 那一步的拷贝是深拷贝还是浅拷贝，append 这个动作本身就已经把
// v4Shape["conversations"] 与 v3Shape["conversations"] 的底层数组
// 彻底切开了——于是"改元素"探针不管改哪个下标，动的都只是 v4Shape
// 自己独有的那块内存，v2Shape/v3Shape 结构性地不可能被牵连。这条子
// 用例在 "conversations" 上今天是绿的，不是因为 buildV4Shape 的拷贝
// 逻辑是对的，是因为它问错了表——把 buildV4Shape 里的深拷贝换成
// `shape[table] = cols` 这种浅拷贝，全包照样绿。
//
// 真正会暴露"浅拷贝、不新开数组"这条坏法的，是 v4 完全没有 append 过
// 的表：这些表在 buildV4Shape 里只经过"深拷贝或浅拷贝"这一步，没有
// 后续的 append 把内存切开，浅拷贝的话 v4Shape[t] 就会与 v3Shape[t]
// （继而 v2Shape[t]——v3Shape 与 v2Shape 是同一张 map，见 schema.go:331
// 的注释，二者对同一个表读到的永远是同一个 slice）共享同一段底层数组。
//
// 不写死具体是哪张表：现在 v4 没碰的表，不代表 v5 也不碰。若硬编码一个
// 表名，v5 哪天给这张表也加了列，这条用例会悄悄重蹈 conversations 的
// 覆辙——用 append 切断共享、从此再也测不出浅拷贝，而且不会有任何报错
// 提示这件事发生了。动态挑选的条件是"v4Shape 与 v3Shape 在这张表上
// 列数相同"：列数没变，说明 v4 没有对它 append 过，只要这个前提还成立，
// 这条用例就一直有代表性；前提不成立（v4 把所有表都改过列数了）就
// t.Fatal 说清楚，不悄悄跳过、不悄悄测别的东西。
func TestV4ShapeUntouchedTableElementIsNotAliasOfV2OrV3(t *testing.T) {
	// 动态挑一张 v4 没有 append 过新列的表。候选可能不止一个（截至 v4，
	// participants、messages、cursors、provider_sessions、
	// schema_migrations 这几张都没被 append 过），排序后取字典序第一个，
	// 让失败信息里报的表名每次运行都一样，
	// 方便复现，不必每次都在一串候选里重新确认这次抽中的是哪张。
	var candidates []string
	for table, v4cols := range v4Shape {
		v3cols, ok := v3Shape[table]
		if !ok {
			continue
		}
		if len(v4cols) == len(v3cols) && len(v4cols) > 0 {
			candidates = append(candidates, table)
		}
	}
	if len(candidates) == 0 {
		t.Fatal("v4Shape 里找不到一张列数与 v3Shape 相同的非空表，" +
			"本用例依赖的前提（v4 至少留一张表没有 append 新列）不成立")
	}
	sort.Strings(candidates)
	probeTable := candidates[0]

	// 深拷贝当前的 v2Shape[probeTable] / v3Shape[probeTable] 留作比对
	// 基准，必须在下面的下标赋值之前取，而且必须是深拷贝：如果浅拷贝的
	// 坏法真的存在，下面一改 v4Shape[probeTable][0] 就会通过共享的底层
	// 数组连带改到 v3Shape[probeTable][0]；这时如果基准只是再取一次
	// v2Shape[probeTable]（不深拷贝），取到的基准会跟着被污染成"已经
	// 被改过之后的样子"，拿它去比对就成了自己证自己，不管实现对不对
	// 测试永远绿。
	wantV2 := cloneColumnShapes(v2Shape[probeTable])
	wantV3 := cloneColumnShapes(v3Shape[probeTable])

	t.Run("改元素（对 v4Shape 里 v4 未新增列的表按下标赋值）不影响 v2Shape 与 v3Shape", func(t *testing.T) {
		cols, ok := v4Shape[probeTable]
		if !ok || len(cols) == 0 {
			t.Fatalf("v4Shape[%q] 为空或不存在，本用例的前提不成立", probeTable)
		}

		// origElem 是 columnShape 的一份值拷贝：这个结构体只有 name/typ
		// 两个 string 字段和一个 notnull bool，没有 slice/map/指针这类
		// 引用字段，string 本身又是不可变的，所以 `cols[0]` 赋值给
		// origElem 的这一刻，origElem 就已经与"v4Shape[probeTable] 这个
		// slice 的第 0 个存储位置"彻底脱钩了，不存在上面"改键"那条子
		// 用例里"存的是引用还是深拷贝"那层顾虑——那是因为那条子用例存的
		// 是整个 slice 头（可能与别的 slice 共享底层数组），这里存的是
		// 一个不含引用语义的标量值，两者不是同一类问题。
		//
		// Cleanup 只需要把这一个下标的值写回去，理由是：本用例自始至终
		// 只对 v4Shape[probeTable] 做下标赋值（v4Shape[probeTable][0] =
		// ...），从未重新赋值过 v4Shape[probeTable] 这个 map 键本身——
		// 也就是说底层数组到底是哪一块、有没有与 v3Shape/v2Shape 共享，
		// 从测试开始到 Cleanup 跑之前从未变过。分两种情况看都成立：
		// 若浅拷贝的坏法确实存在，v4Shape[probeTable] 与
		// v3Shape[probeTable]（继而 v2Shape[probeTable]）指向同一块
		// 底层数组，Cleanup 对 v4Shape[probeTable][0] 的写回会通过这块
		// 共享内存一并把 v3Shape[probeTable][0] 与 v2Shape[probeTable][0]
		// 也写回原值；若实现是对的，各自持有独立的底层数组，Cleanup 就
		// 只精确复原 v4Shape 自己那一份，不会误伤别人。两种情况下"只写
		// 这一个下标"都足以让状态回到测试开始之前，不需要像"改键"那条
		// 子用例那样分情况讨论该存引用还是该存深拷贝。
		origElem := cols[0]
		t.Cleanup(func() {
			v4Shape[probeTable][0] = origElem
		})

		v4Shape[probeTable][0] = columnShape{
			name: "tst_v4_shape_untouched_probe_elem", typ: origElem.typ, notnull: origElem.notnull,
		}

		assertColumnShapesEqual(t, fmt.Sprintf("v2Shape[%q]", probeTable), v2Shape[probeTable], wantV2)
		assertColumnShapesEqual(t, fmt.Sprintf("v3Shape[%q]", probeTable), v3Shape[probeTable], wantV3)
	})
}
