package i18n

// catalogs.go 把三段前缀的目录合并成 T / TF 用的那一张表。
//
// 三张表分开写，是为了让「加一条文案」时不用在西文表里翻几百行找位置，
// 也让 /api/i18n 的过滤（UIMessages 只看 ui.）与目录的组织方式对得上。
// 合并发生在包初始化：T / TF 每次查表仍然只是一次 map 查找。

// mergeCatalogs 按顺序合并若干目录，后面的覆盖前面的同名 key。
// key 跨段重复是写错了前缀，直接 panic：初始化期炸掉比线上静默覆盖好定位。
func mergeCatalogs(parts ...Catalog) Catalog {
	size := 0
	for _, part := range parts {
		size += len(part)
	}
	out := make(Catalog, size)
	for _, part := range parts {
		for key, text := range part {
			if _, dup := out[key]; dup {
				panic("i18n: 目录里出现重复 key: " + key)
			}
			out[key] = text
		}
	}
	return out
}

// zh / en 是合并后的完整目录。测试直接读它们（key 集合一致性、单复数变体），
// 生产代码只经 T / TF 查表。
var (
	zh = mergeCatalogs(zhUI, zhCLI, zhTUI)
	en = mergeCatalogs(enUI, enCLI, enTUI)
)
