package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultPassesValidate 覆盖规格 G：Default() 的产物必须能通过 Validate()。
// 用 err != nil 直接断言，顺带盯住 typed-nil 陷阱。
func TestDefaultPassesValidate(t *testing.T) {
	cfg := Default()
	if cfg == nil {
		t.Fatal("Default() 返回 nil")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Default() 的配置没能通过 Validate: %v", err)
	}
}

// TestDefaultTOMLLoadsAndValidates 把模板文本落到临时目录再 Load，
// 确保发给用户初始化用的模板本身是一份合法配置，而不是只在注释里正确。
func TestDefaultTOMLLoadsAndValidates(t *testing.T) {
	text := DefaultTOML()
	if text == "" {
		t.Fatal("DefaultTOML() 返回空字符串")
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("写模板文件失败: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("DefaultTOML() 的文本没能被 Load 解析: %v\n模板内容:\n%s", err, text)
	}
	if cfg == nil {
		t.Fatal("Load 模板文本后返回的 *Config 是 nil")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("DefaultTOML() 解析出的配置没能通过 Validate: %v\n模板内容:\n%s", err, text)
	}
}

// TestDefaultTOMLHasNoDecorativeSymbols 覆盖符号禁令。
// 模板文本会被原样写进用户的 config.toml，本机控制台默认 GBK，
// 装饰性符号会变乱码并污染之后的 grep 与 diff。
// 禁用区间在这里显式列出，不引用外部正则，也不套用 PowerShell 那条按 UTF-16 代理对写的表达式：
// Go 的 range 迭代出来的是 rune，直接比码点即可。
func TestDefaultTOMLHasNoDecorativeSymbols(t *testing.T) {
	banned := []struct {
		name string
		lo   rune
		hi   rune
	}{
		{name: "Emoji 与象形图 U+1F300-U+1FAFF", lo: 0x1F300, hi: 0x1FAFF},
		{name: "Dingbats U+2700-U+27BF", lo: 0x2700, hi: 0x27BF},
		{name: "杂项符号 U+2600-U+26FF", lo: 0x2600, hi: 0x26FF},
		{name: "杂项符号与箭头 U+2B00-U+2BFF", lo: 0x2B00, hi: 0x2BFF},
		{name: "带圈字母数字 U+2460-U+24FF", lo: 0x2460, hi: 0x24FF},
		{name: "变体选择符 U+FE0F", lo: 0xFE0F, hi: 0xFE0F},
		{name: "零宽连接符 U+200D", lo: 0x200D, hi: 0x200D},
	}

	text := DefaultTOML()
	if text == "" {
		t.Fatal("DefaultTOML() 返回空字符串，空串扫描不出问题不等于合格")
	}

	for _, rangeCase := range banned {
		t.Run(rangeCase.name, func(t *testing.T) {
			for offset, r := range text {
				if r >= rangeCase.lo && r <= rangeCase.hi {
					t.Errorf("DefaultTOML() 的第 %d 字节处出现禁用符号 U+%04X（属于 %s）",
						offset, r, rangeCase.name)
				}
			}
		})
	}
}

// TestDefaultTOMLPermissionsAndModeAreLowercase 是一个顺带的一致性检查：
// 模板里若写了 permissions 或 mode，Load 之后必须落在合法枚举上。
// 这里不假设模板里有几个 agent，只对实际解析出来的逐个检查。
func TestDefaultTOMLPermissionsAndModeAreLowercase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(DefaultTOML()), 0o644); err != nil {
		t.Fatalf("写模板文件失败: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 模板失败: %v", err)
	}

	validPermissions := map[Permission]bool{
		PermissionRead:      true,
		PermissionWorkspace: true,
		PermissionFull:      true,
	}
	validModes := map[Mode]bool{
		ModeDiscussion:          true,
		ModeWorker:              true,
		ModeDiscussionAndWorker: true,
		ModeJudge:               true,
		ModeManager:             true,
	}

	// 按 id 排序后遍历：map 的遍历顺序随机，不排序会让失败信息的行序在不同次运行间变动。
	for _, id := range sortedIDs(cfg.Agents) {
		agent := cfg.Agents[id]
		if !validPermissions[agent.Permissions] {
			t.Errorf("模板里 agents.%s.permissions = %q，不属于合法枚举", id, agent.Permissions)
		}
		if !validModes[agent.Mode] {
			t.Errorf("模板里 agents.%s.mode = %q，不属于合法枚举", id, agent.Mode)
		}
	}
}

// TestDefaultIsIndependentAcrossCalls 断言 Default() 每次返回独立的实例，
// 调用方改一份不应当污染下一次拿到的配置。
func TestDefaultIsIndependentAcrossCalls(t *testing.T) {
	first := Default()
	if first == nil {
		t.Fatal("Default() 返回 nil")
	}
	wantAgents := len(first.Agents)

	first.DataDir = "被改过的值"
	for id := range first.Agents {
		delete(first.Agents, id)
	}

	second := Default()
	if second == nil {
		t.Fatal("第二次 Default() 返回 nil")
	}
	if second.DataDir == "被改过的值" {
		t.Error("两次 Default() 共享了同一个结构体，改第一份污染了第二份")
	}
	if len(second.Agents) != wantAgents {
		t.Errorf("第二次 Default() 的 agents 数量 = %d，期望 %d；两次调用共享了同一张 map",
			len(second.Agents), wantAgents)
	}
}
