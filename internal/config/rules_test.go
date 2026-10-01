// 本文件将来落到 internal/config/rules_test.go。
//
// 覆盖 11a_STEP8A_API_CONTRACT.md 第 1 节：ResolveGlobalRulesPath、LoadRules、
// 三步归一化（1.5）、rules_file 键本身（1.1）。只照契约写，不看
// internal/config/rules.go 的实现——那是本片改动、任务书明令不读的文件。
//
// 复用同包既有测试文件（load_test.go / validate_test.go / paths_test.go）里的
// tstWriteTOML / tstFakeHome / tstUnsetEnv / tstValidConfig 辅助函数与
// tstRequireErrContains，不重新发明一套。
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tstPinGlobalRulesEnv 把 COUNCIL_GLOBAL_RULES 钉成给定值。
// 每一条调 LoadRules / ResolveGlobalRulesPath 的用例都必须先钉这个变量，
// 否则测试会去读开发机上真实的 ~/.council/COUNCIL.md，"文件不存在" 一类的
// 断言会随机通过或失败。
func tstPinGlobalRulesEnv(t *testing.T, value string) {
	t.Helper()
	t.Setenv(EnvGlobalRules, value)
}

// tstIsolateGlobalRules 把 COUNCIL_GLOBAL_RULES 指向一个当前不存在的路径，
// 用来隔离"只关心局部规则"的用例，不让全局规则的读取结果混进断言。
func tstIsolateGlobalRules(t *testing.T) {
	t.Helper()
	tstPinGlobalRulesEnv(t, filepath.Join(t.TempDir(), "isolated-global-does-not-exist.md"))
}

// ---------------------------------------------------------------------------
// ResolveGlobalRulesPath —— 与 ResolveConfigPath 同构，但没有 flag 一级
// ---------------------------------------------------------------------------

func TestResolveGlobalRulesPath(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) (want string)
	}{
		{
			name: "环境变量 COUNCIL_GLOBAL_RULES 优先",
			setup: func(t *testing.T) string {
				tstFakeHome(t)
				p := filepath.Join(t.TempDir(), "from-env.md")
				tstPinGlobalRulesEnv(t, p)
				return p
			},
		},
		{
			name: "环境变量未设置时回落主目录下的 .council/COUNCIL.md",
			setup: func(t *testing.T) string {
				home := tstFakeHome(t)
				tstUnsetEnv(t, EnvGlobalRules)
				return filepath.Join(home, ".council", DefaultGlobalRulesName)
			},
		},
		{
			name: "环境变量为空串时按未设置处理，回落主目录默认",
			setup: func(t *testing.T) string {
				home := tstFakeHome(t)
				tstPinGlobalRulesEnv(t, "")
				return filepath.Join(home, ".council", DefaultGlobalRulesName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.setup(t)
			got, err := ResolveGlobalRulesPath()
			if err != nil {
				t.Fatalf("ResolveGlobalRulesPath() 返回错误: %v", err)
			}
			if got != want {
				t.Errorf("ResolveGlobalRulesPath() = %q，期望 %q", got, want)
			}
		})
	}
}

// TestResolveGlobalRulesPathReturnedPathNeedNotExist 覆盖契约原话：
// "返回的路径不保证文件存在"。回落到默认值时那个文件大概率不存在，
// 函数本身不应当因此报错。
func TestResolveGlobalRulesPathReturnedPathNeedNotExist(t *testing.T) {
	tstFakeHome(t)
	tstUnsetEnv(t, EnvGlobalRules)
	got, err := ResolveGlobalRulesPath()
	if err != nil {
		t.Fatalf("ResolveGlobalRulesPath() 返回错误: %v", err)
	}
	if _, statErr := os.Stat(got); statErr == nil {
		t.Fatalf("测试前提失误：%q 竟然真的存在，换一个更不可能存在的临时目录", got)
	}
}

// ---------------------------------------------------------------------------
// LoadRules —— 四种情形（契约 1.4、1.5，验收表第 4 条）
// ---------------------------------------------------------------------------

// TestLoadRulesFileNotExist 覆盖：文件不存在不是错误，Found=false、Text 为空串、
// 路径照填。全局与局部各测一次，两者不该有第二套判断逻辑。
func TestLoadRulesFileNotExist(t *testing.T) {
	t.Run("局部规则文件不存在", func(t *testing.T) {
		tstIsolateGlobalRules(t)
		dir := t.TempDir()
		configPath := filepath.Join(dir, "config.toml")
		const missingName = "no-such-rules.md"

		rules, err := LoadRules(configPath, missingName)
		if err != nil {
			t.Fatalf("LoadRules 返回错误: %v", err)
		}
		if rules.LocalFound {
			t.Error("LocalFound 应为 false")
		}
		if rules.LocalText != "" {
			t.Errorf("LocalText = %q，期望空串", rules.LocalText)
		}
		wantPath := filepath.Join(dir, missingName)
		if rules.LocalPath != wantPath {
			t.Errorf("LocalPath = %q，期望 %q（路径仍要照填）", rules.LocalPath, wantPath)
		}
	})

	t.Run("全局规则文件不存在", func(t *testing.T) {
		globalPath := filepath.Join(t.TempDir(), "no-such-global.md")
		tstPinGlobalRulesEnv(t, globalPath)
		dir := t.TempDir()

		rules, err := LoadRules(filepath.Join(dir, "config.toml"), "")
		if err != nil {
			t.Fatalf("LoadRules 返回错误: %v", err)
		}
		if rules.GlobalFound {
			t.Error("GlobalFound 应为 false")
		}
		if rules.GlobalText != "" {
			t.Errorf("GlobalText = %q，期望空串", rules.GlobalText)
		}
		if rules.GlobalPath != globalPath {
			t.Errorf("GlobalPath = %q，期望 %q（恒非空，文件不存在也要照填）", rules.GlobalPath, globalPath)
		}
	})
}

// TestLoadRulesEmptyRulesFileMeansNoLocal 覆盖 Rules 类型注释：
// "配置里没写 rules_file 时是空串"。
func TestLoadRulesEmptyRulesFileMeansNoLocal(t *testing.T) {
	tstIsolateGlobalRules(t)
	dir := t.TempDir()

	rules, err := LoadRules(filepath.Join(dir, "config.toml"), "")
	if err != nil {
		t.Fatalf("LoadRules 返回错误: %v", err)
	}
	if rules.LocalPath != "" {
		t.Errorf("LocalPath = %q，rules_file 为空串时期望 LocalPath 也是空串", rules.LocalPath)
	}
	if rules.LocalFound {
		t.Error("LocalFound 应为 false")
	}
	if rules.LocalText != "" {
		t.Errorf("LocalText = %q，期望空串", rules.LocalText)
	}
}

// TestLoadRulesPathIsDirectory 覆盖：路径指向一个目录时是错误，不是"读到空文本"。
// 全局与局部各测一次，隔离另一侧避免它的错误掩盖本用例真正要测的那一侧。
func TestLoadRulesPathIsDirectory(t *testing.T) {
	t.Run("局部 rules_file 指向一个目录", func(t *testing.T) {
		tstIsolateGlobalRules(t)
		dir := t.TempDir()
		subdir := filepath.Join(dir, "this-is-a-directory")
		if err := os.Mkdir(subdir, 0o755); err != nil {
			t.Fatalf("建目录失败: %v", err)
		}

		_, err := LoadRules(filepath.Join(dir, "config.toml"), "this-is-a-directory")
		if err == nil {
			t.Fatal("rules_file 指向一个目录时，LoadRules 应当返回错误，实际 nil")
		}
	})

	t.Run("全局规则路径指向一个目录", func(t *testing.T) {
		dir := t.TempDir()
		subdir := filepath.Join(dir, "global-is-a-directory")
		if err := os.Mkdir(subdir, 0o755); err != nil {
			t.Fatalf("建目录失败: %v", err)
		}
		tstPinGlobalRulesEnv(t, subdir)

		_, err := LoadRules(filepath.Join(t.TempDir(), "config.toml"), "")
		if err == nil {
			t.Fatal("全局规则路径指向一个目录时，LoadRules 应当返回错误，实际 nil")
		}
	})
}

// ---------------------------------------------------------------------------
// 归一化：三步，顺序固定（契约 1.5）。每一步单独一个用例，不合并成一个混合用例。
// ---------------------------------------------------------------------------

// tstLoadLocalRules 是本组归一化用例共用的小工具：把 raw 字节原样写成局部规则
// 文件，隔离全局规则，调 LoadRules，返回归一化之后的 LocalText。
func tstLoadLocalRules(t *testing.T, raw []byte) string {
	t.Helper()
	tstIsolateGlobalRules(t)
	dir := t.TempDir()
	const name = "COUNCIL.md"
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		t.Fatalf("写规则文件失败: %v", err)
	}
	rules, err := LoadRules(filepath.Join(dir, "config.toml"), name)
	if err != nil {
		t.Fatalf("LoadRules 返回错误: %v", err)
	}
	if !rules.LocalFound {
		t.Fatal("LocalFound 应为 true（文件确实存在）")
	}
	return rules.LocalText
}

// TestLoadRulesStripsLeadingBOM 覆盖第 1 步：只去掉开头的 UTF-8 BOM。
func TestLoadRulesStripsLeadingBOM(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	raw := append(append([]byte{}, bom...), []byte("hello\nworld")...)
	got := tstLoadLocalRules(t, raw)
	if got != "hello\nworld" {
		t.Errorf("LocalText = %q，期望 %q（开头 BOM 应被去掉，其余字节不动）", got, "hello\nworld")
	}
}

// TestLoadRulesBOMNotStrippedInMiddle 是上一条的反例：BOM 字节若不在文件开头，
// 只是三个普通字节，不该被特殊处理——归一化只检查前缀，不是全文替换。
func TestLoadRulesBOMNotStrippedInMiddle(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	raw := append(append([]byte("abc"), bom...), []byte("def")...)
	want := "abc" + string(bom) + "def"
	got := tstLoadLocalRules(t, raw)
	if got != want {
		t.Errorf("LocalText = %q，期望 %q（文件中间的 BOM 字节不应被去掉）", got, want)
	}
}

// TestLoadRulesCRLFToLF 覆盖第 2 步：CRLF 换成 LF，且没有 BOM、没有结尾换行时
// 单独生效。
func TestLoadRulesCRLFToLF(t *testing.T) {
	raw := []byte("line1\r\nline2\r\nline3")
	got := tstLoadLocalRules(t, raw)
	want := "line1\nline2\nline3"
	if got != want {
		t.Errorf("LocalText = %q，期望 %q", got, want)
	}
}

// TestLoadRulesTrimsTrailingNewlines 覆盖第 3 步：去掉结尾的全部换行，
// 且只影响结尾，不影响开头或中间的空行。
func TestLoadRulesTrimsTrailingNewlines(t *testing.T) {
	raw := []byte("hello\n\n\n")
	got := tstLoadLocalRules(t, raw)
	if got != "hello" {
		t.Errorf("LocalText = %q，期望 %q", got, "hello")
	}
}

// TestLoadRulesNormalizationOrderMatters 是判别顺序的关键用例：三步的顺序
// 必须是 BOM -> CRLF->LF -> TrimRight("\n")，不能是任意顺序。
//
// 输入 "a\r\n\r\n"：
//   - 按契约顺序（CRLF 先变 LF，再 TrimRight）：CRLF->LF 得 "a\n\n"，
//     再去掉结尾全部 \n 得 "a"。
//   - 若误把 TrimRight 放在 CRLF 转换之前：TrimRight("\n") 只认识裸 \n，
//     "a\r\n\r\n" 结尾字符依次是 \n \r \n \r，从后往前扫，第一个 \n 会被剪掉，
//     下一个是 \r（不在裁剪集合里），立刻停手，结果是 "a\r\n\r"；
//     再做 CRLF->LF 会把其中的 "\r\n" 换成 "\n"，得到 "a\n\r"——
//     与按契约顺序算出来的 "a" 不同，这条用例就是用来分辨这两种实现的。
func TestLoadRulesNormalizationOrderMatters(t *testing.T) {
	got := tstLoadLocalRules(t, []byte("a\r\n\r\n"))
	if got != "a" {
		t.Errorf("LocalText = %q，期望 %q（三步顺序：先 CRLF->LF，再 TrimRight，不是反过来）", got, "a")
	}
}

// TestLoadRulesNormalizationCombinedWithBOM 覆盖三步一起生效的常见真实场景：
// PS 5.1 默认写法插入的 BOM，加上 CRLF 换行，加上结尾空行。
func TestLoadRulesNormalizationCombinedWithBOM(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	raw := append(append([]byte{}, bom...), []byte("line1\r\nline2\r\n\r\n")...)
	got := tstLoadLocalRules(t, raw)
	want := "line1\nline2"
	if got != want {
		t.Errorf("LocalText = %q，期望 %q", got, want)
	}
}

// TestLoadRulesDoesNotTrimLeading 覆盖契约"不做的"一节：不 Trim 开头，
// 不动行内空白。开头的空行与行内空格必须原样保留。
func TestLoadRulesDoesNotTrimLeading(t *testing.T) {
	raw := []byte("\n\n  缩进的第一行\n第二行  行内尾随空格  \n")
	got := tstLoadLocalRules(t, raw)
	want := "\n\n  缩进的第一行\n第二行  行内尾随空格  "
	if got != want {
		t.Errorf("LocalText = %q，期望 %q（开头空行、行内空白都不该被动）", got, want)
	}
}

// ---------------------------------------------------------------------------
// 相对路径解析：相对配置文件所在目录，不是相对 cwd（契约 1.1 与 06 第 4 节）
// ---------------------------------------------------------------------------

// TestLoadRulesRelativePathResolvesAgainstConfigDirNotCWD 是最容易被绕过的一条：
// 若不在 cwd 里放一份"诱饵"规则文件，一个按 cwd 解析的错误实现只会因为
// "文件不存在"而报错或者 Found=false，看起来和正确实现的负例长得一样，
// 分辨不出真正的差异。这里让 cwd（目录 B）与配置文件所在目录（目录 A）
// 都各自放一份内容不同的 COUNCIL.md，只有真正按配置文件目录解析的实现
// 才会读到 A 的内容。
func TestLoadRulesRelativePathResolvesAgainstConfigDirNotCWD(t *testing.T) {
	tstIsolateGlobalRules(t)

	dirA := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirA, "COUNCIL.md"), []byte("A-的规则正文"), 0o644); err != nil {
		t.Fatalf("写 A 的规则文件失败: %v", err)
	}
	configPath := filepath.Join(dirA, "config.toml")

	dirB := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirB, "COUNCIL.md"), []byte("B-诱饵-绝不该被读到"), 0o644); err != nil {
		t.Fatalf("写 B 的诱饵文件失败: %v", err)
	}
	t.Chdir(dirB) // 当前工作目录故意与配置文件所在目录不同

	rules, err := LoadRules(configPath, "COUNCIL.md")
	if err != nil {
		t.Fatalf("LoadRules 返回错误: %v", err)
	}
	wantPath := filepath.Join(dirA, "COUNCIL.md")
	if rules.LocalPath != wantPath {
		t.Errorf("LocalPath = %q，期望 %q（应解析到配置文件所在目录 A，不是 cwd B）", rules.LocalPath, wantPath)
	}
	if rules.LocalText != "A-的规则正文" {
		t.Errorf("LocalText = %q，期望 %q；读到诱饵内容说明按 cwd 解析了", rules.LocalText, "A-的规则正文")
	}
}

// ---------------------------------------------------------------------------
// rules_file 键本身（契约 1.1）：Load 认得这个键、Validate 不校验它
// ---------------------------------------------------------------------------

// TestLoadAcceptsRulesFileKey 覆盖：顶层 rules_file 是已知键，不会被 D-1
// "未知键一律报错" 那条规则拦下，且值被正确读进 Config.RulesFile。
func TestLoadAcceptsRulesFileKey(t *testing.T) {
	path := tstWriteTOML(t,
		`rules_file = "COUNCIL.md"`,
		``,
		`[providers.claude]`,
		`command = "claude"`,
		``,
		`[agents.theory]`,
		`provider = "claude"`,
		`model    = "opus"`,
	)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 顶层 rules_file 失败: %v", err)
	}
	if cfg.RulesFile != "COUNCIL.md" {
		t.Errorf("cfg.RulesFile = %q，期望 %q", cfg.RulesFile, "COUNCIL.md")
	}
}

// TestValidateDoesNotCheckRulesFileExistence 覆盖契约 1.1 的校验一节：
// validate.go 对 rules_file 不做任何校验，它是不是指向一个存在的文件由
// LoadRules 在真正读的时候回答。这里把它指向一个必然不存在的路径，
// 断言 Validate 仍然通过。
func TestValidateDoesNotCheckRulesFileExistence(t *testing.T) {
	cfg := tstValidConfig()
	cfg.RulesFile = filepath.Join("这个文件", "根本不存在.md")
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() 不该检查 rules_file 是否存在，实际报错: %v", err)
	}
}

// TestDefaultTOMLMentionsRulesFile 覆盖契约 1.1：DefaultTOML() 里要有
// rules_file 的注释示例，紧跟在 data_dir 之后。只断言字符串出现，不锁具体措辞
// （第 7 节：具体措辞不属于契约面）。
func TestDefaultTOMLMentionsRulesFile(t *testing.T) {
	got := DefaultTOML()
	if !strings.Contains(got, "rules_file") {
		t.Error("DefaultTOML() 里没有出现 rules_file，期望有一行注释示例")
	}
}
