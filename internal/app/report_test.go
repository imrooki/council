package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"council/internal/config"
)

// templateMarker 是 config.DefaultTOML() 里必然出现、而普通错误信息里不会出现的片段。
// 用它判断「模板有没有被打出来」，比断言整段文本稳。
const templateMarker = "[providers.claude]"

// TestMissingConfigTemplateOnlyOnDefaultPath 锁住一条取舍：
// 只有在用户没指定过配置位置时（真正的首次运行）才打印整份模板；
// 用户自己写了 --config 或设了环境变量却找不到文件，多半是打错字，
// 这时刷几十行模板会把那一行真正有用的路径淹掉。
func TestMissingConfigTemplateOnlyOnDefaultPath(t *testing.T) {
	t.Run("显式 --config 指向不存在的文件，不打印模板", func(t *testing.T) {
		t.Setenv(config.EnvConfigPath, "")
		missing := filepath.Join(t.TempDir(), "typo.toml")

		var out, errOut bytes.Buffer
		err := Run(context.Background(), Options{
			ConfigPath: missing,
			In:         strings.NewReader(""),
			Out:        &out,
			Err:        &errOut,
		})
		if err == nil {
			t.Fatalf("配置文件不存在时 Run 应当返回错误")
		}

		got := errOut.String()
		if !strings.Contains(got, missing) {
			t.Errorf("错误输出里应当含那个写错的路径 %q，实际:\n%s", missing, got)
		}
		if strings.Contains(got, templateMarker) {
			t.Errorf("显式指定路径时不应当打印整份模板，实际:\n%s", got)
		}
	})

	t.Run("环境变量指向不存在的文件，同样不打印模板", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "from-env.toml")
		t.Setenv(config.EnvConfigPath, missing)

		var out, errOut bytes.Buffer
		err := Run(context.Background(), Options{
			In:  strings.NewReader(""),
			Out: &out,
			Err: &errOut,
		})
		if err == nil {
			t.Fatalf("配置文件不存在时 Run 应当返回错误")
		}
		if strings.Contains(errOut.String(), templateMarker) {
			t.Errorf("环境变量指定路径时不应当打印整份模板，实际:\n%s", errOut.String())
		}
	})

	t.Run("完全没指定过，走默认路径时进首次运行（本机一家工具都没有）", func(t *testing.T) {
		home := t.TempDir()
		empty := t.TempDir()
		// os.UserHomeDir 在 Windows 上读 USERPROFILE，在类 Unix 上读 HOME。
		// 两个都设上，这个用例就不挑平台。
		// #48 起默认位置没有文件会进首次运行、检测本机装了哪些工具：APPDATA、
		// LOCALAPPDATA、PATH 也指到空的临时目录，不许看见跑测试这台机器上真装着的 CLI；
		// COUNCIL_DATA_DIR 指到临时目录，万一走到 store.Open 也不碰内置默认的数据目录。
		t.Setenv("USERPROFILE", home)
		t.Setenv("HOME", home)
		t.Setenv("APPDATA", empty)
		t.Setenv("LOCALAPPDATA", empty)
		t.Setenv("PATH", empty)
		t.Setenv(config.EnvConfigPath, "")
		t.Setenv(config.EnvDataDir, filepath.Join(t.TempDir(), "data"))

		var out, errOut bytes.Buffer
		err := Run(context.Background(), Options{
			In:  strings.NewReader(""),
			Out: &out,
			Err: &errOut,
		})
		if err == nil {
			t.Fatalf("默认位置没有配置文件、本机一家工具都没有时 Run 应当返回错误")
		}

		got := errOut.String()
		if strings.Contains(got, templateMarker) {
			t.Errorf("#48 起不再打印模板，实际:\n%s", got)
		}
		if !strings.Contains(got, filepath.Join(home, ".council", "config.toml")) {
			t.Errorf("错误输出里应当含默认配置路径，实际:\n%s", got)
		}
		if !strings.Contains(got, "一个都没找到，没有生成配置文件。") {
			t.Errorf("应当走首次运行「一家都没找到」的结局，实际:\n%s", got)
		}
		if _, statErr := os.Stat(filepath.Join(home, ".council")); !os.IsNotExist(statErr) {
			t.Errorf("一家都没找到时不该建 .council 目录，Stat 结果: %v", statErr)
		}
	})
}

// TestBannerOmitsVersionWhenEmpty 锁住 Options.Version 的语义：
// 空串表示不显示版本行，而不是显示一个空的 council 前缀。
func TestBannerOmitsVersionWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	// strconv.Quote 产出的转义形式与 TOML 基本字符串一致，
	// Windows 路径里的反斜杠会被正确转义，不用手拼。
	body := "data_dir = " + strconv.Quote(dir) + "\n\n[providers.claude]\ncommand = \"claude\"\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}

	cases := []struct {
		name    string
		version string
		wantHas bool
	}{
		{name: "给了版本号就显示", version: "1.2.3", wantHas: true},
		{name: "空串不显示版本行", version: "", wantHas: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if err := Run(context.Background(), Options{
				ConfigPath: cfgPath,
				Version:    tc.version,
				In:         strings.NewReader(""),
				Out:        &out,
				Err:        &errOut,
			}); err != nil {
				t.Fatalf("Run 应当成功，实际: %v  stderr:\n%s", err, errOut.String())
			}

			has := strings.Contains(out.String(), "council "+tc.version) && tc.version != ""
			if has != tc.wantHas {
				t.Errorf("版本行的出现与否不符预期（version=%q）；Out:\n%s", tc.version, out.String())
			}
			if tc.version == "" && strings.HasPrefix(out.String(), "council ") {
				t.Errorf("版本号为空时不应当留下 council 前缀行；Out:\n%s", out.String())
			}
		})
	}
}
