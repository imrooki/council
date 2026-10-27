package localagent

// lease_test.go 钉住资源键与租约参数的规范化。这一层是纯函数，所以用例
// 用表驱动逐条列；数据库那一侧的语义在 internal/store/localagent_test.go。

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNormalizeResourcePath 覆盖路径类的三种写法：同一份目录的大小写、
// 分隔符、末尾分隔符变体必须落到同一个键；相对路径要展开成绝对路径。
func TestNormalizeResourcePath(t *testing.T) {
	dir := t.TempDir()
	want := strings.ToLower(filepath.Clean(dir))

	t.Run("同一目录的写法都折成同一个键", func(t *testing.T) {
		variants := []string{
			dir,
			dir + string(filepath.Separator),
			strings.ToUpper(dir),
			strings.ReplaceAll(dir, string(filepath.Separator), "/"),
		}
		for _, variant := range variants {
			got, err := NormalizeResource(variant)
			if err != nil {
				t.Fatalf("NormalizeResource(%q): %v", variant, err)
			}
			if got != want {
				t.Errorf("NormalizeResource(%q) = %q，期望 %q", variant, got, want)
			}
		}
	})

	t.Run("相对路径展开成绝对路径", func(t *testing.T) {
		got, err := NormalizeResource(".")
		if err != nil {
			t.Fatalf("NormalizeResource(.): %v", err)
		}
		if !filepath.IsAbs(got) {
			t.Errorf("NormalizeResource(.) = %q，期望绝对路径", got)
		}
		if got != strings.ToLower(got) {
			t.Errorf("NormalizeResource(.) = %q，期望全是小写", got)
		}
	})

	t.Run("点与点点被 Clean 掉", func(t *testing.T) {
		got, err := NormalizeResource(filepath.Join(dir, "sub", ".."))
		if err != nil {
			t.Fatalf("NormalizeResource: %v", err)
		}
		if got != want {
			t.Errorf("NormalizeResource(<dir>/sub/..) = %q，期望 %q", got, want)
		}
	})

	t.Run("空资源被拒", func(t *testing.T) {
		if _, err := NormalizeResource("   "); !errors.Is(err, ErrInvalidResource) {
			t.Errorf("空资源的错误 = %v，期望 ErrInvalidResource", err)
		}
	})
}

// TestNormalizeResourcePort 覆盖端口类资源。键的形状与 Plan §8.1 的示例
// （tcp:127.0.0.1:8765）一致；裸端口补 127.0.0.1，因为看板只绑回环。
func TestNormalizeResourcePort(t *testing.T) {
	ok := map[string]string{
		"tcp:8765":           "tcp:127.0.0.1:8765",
		"TCP:8765":           "tcp:127.0.0.1:8765",
		"tcp:127.0.0.1:8765": "tcp:127.0.0.1:8765",
		"tcp:LocalHost:8765": "tcp:localhost:8765",
		"tcp:[::1]:8765":     "tcp:[::1]:8765",
		"tcp::8765":          "tcp:127.0.0.1:8765",
		"  tcp:6432  ":       "tcp:127.0.0.1:6432",
	}
	for raw, want := range ok {
		got, err := NormalizeResource(raw)
		if err != nil {
			t.Errorf("NormalizeResource(%q): %v", raw, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeResource(%q) = %q，期望 %q", raw, got, want)
		}
	}

	for _, bad := range []string{"tcp:", "tcp:0", "tcp:65536", "tcp:abc", "tcp:127.0.0.1:"} {
		if _, err := NormalizeResource(bad); !errors.Is(err, ErrInvalidResource) {
			t.Errorf("NormalizeResource(%q) 的错误 = %v，期望 ErrInvalidResource", bad, err)
		}
	}
}

// TestParseAccessMode 钉住默认是 write：空串按「我要动它」处理，否则一次
// 手滑会得到一条没有任何保护力的读租约。
func TestParseAccessMode(t *testing.T) {
	ok := map[string]AccessMode{
		"":         AccessWrite,
		"w":        AccessWrite,
		"write":    AccessWrite,
		"  WRITE ": AccessWrite,
		"r":        AccessRead,
		"read":     AccessRead,
		" READ  ":  AccessRead,
	}
	for raw, want := range ok {
		got, err := ParseAccessMode(raw)
		if err != nil {
			t.Errorf("ParseAccessMode(%q): %v", raw, err)
			continue
		}
		if got != want {
			t.Errorf("ParseAccessMode(%q) = %q，期望 %q", raw, got, want)
		}
	}
	for _, bad := range []string{"rw", "x", "both"} {
		if _, err := ParseAccessMode(bad); !errors.Is(err, ErrInvalidResource) {
			t.Errorf("ParseAccessMode(%q) 的错误 = %v，期望 ErrInvalidResource", bad, err)
		}
	}
}

// TestNormalizeTTL 覆盖 0（未给）、下界、上界与越界：越界直接报错，不夹到
// 边界 —— 悄悄改用户给的数比拒绝更难排查。
func TestNormalizeTTL(t *testing.T) {
	if got, err := NormalizeTTL(0); err != nil || got != DefaultLeaseTTL {
		t.Errorf("NormalizeTTL(0) = %s, %v；期望默认 %s", got, err, DefaultLeaseTTL)
	}
	for _, ok := range []time.Duration{MinLeaseTTL, 2 * time.Minute, MaxLeaseTTL} {
		if got, err := NormalizeTTL(ok); err != nil || got != ok {
			t.Errorf("NormalizeTTL(%s) = %s, %v；期望原值", ok, got, err)
		}
	}
	for _, bad := range []time.Duration{time.Second, MinLeaseTTL - time.Second, MaxLeaseTTL + time.Second, 48 * time.Hour} {
		if _, err := NormalizeTTL(bad); err == nil {
			t.Errorf("NormalizeTTL(%s) 应当报错", bad)
		}
	}
}

// TestLevelAuthorization 是这一层的授权矩阵：observe 只扫不 claim，
// coordinate 两者都可，其它一切取值两样都不行（fail closed）。
func TestLevelAuthorization(t *testing.T) {
	cases := []struct {
		level   Level
		scans   bool
		claims  bool
		comment string
	}{
		{LevelOff, false, false, "默认关了"},
		{"", false, false, "空串按 off"},
		{"register", false, false, "未实现的层级不许顺手开门"},
		{"collaborate", false, false, "未实现的层级不许顺手开门"},
		{LevelObserve, true, false, "观察不给协调权"},
		{LevelCoordinate, true, true, "协调包含观察"},
	}
	for _, tc := range cases {
		if got := tc.level.Scans(); got != tc.scans {
			t.Errorf("Level(%q).Scans() = %v，期望 %v（%s）", tc.level, got, tc.scans, tc.comment)
		}
		if got := tc.level.Claims(); got != tc.claims {
			t.Errorf("Level(%q).Claims() = %v，期望 %v（%s）", tc.level, got, tc.claims, tc.comment)
		}
	}
}

// TestResourceOverlaps 钉住「同一处或父子关系才算重叠」：宁可漏报，
// 也不要把两个不相干的目录说成冲突（那是狼来了）。
func TestResourceOverlaps(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{`c:\work`, `c:\work`, true},
		{`c:\work`, `c:\work\sub`, true},
		{`c:\work\sub`, `c:\work`, true},
		{`c:\work`, `c:\work\sub\deep`, true},
		// 分隔符混用也要认（旧键可能是 / 写的）。
		{`c:\work`, `c:/work/sub`, true},
		// 同前缀但不是父子：不重叠。
		{`c:\work`, `c:\workshop`, false},
		{`c:\work`, `c:\work2\sub`, false},
		{`c:\work`, `d:\work`, false},
		// 端口资源只按相等判。
		{`tcp:127.0.0.1:8765`, `tcp:127.0.0.1:8765`, true},
		{`tcp:127.0.0.1:8765`, `tcp:127.0.0.1:8766`, false},
		{`tcp:127.0.0.1:8765`, `c:\work`, false},
		// 空键不与任何东西重叠。
		{"", `c:\work`, false},
		{`c:\work`, "", false},
	}
	for _, tc := range cases {
		if got := ResourceOverlaps(tc.a, tc.b); got != tc.want {
			t.Errorf("ResourceOverlaps(%q, %q) = %v，期望 %v", tc.a, tc.b, got, tc.want)
		}
		if got := ResourceOverlaps(tc.b, tc.a); got != tc.want {
			t.Errorf("ResourceOverlaps(%q, %q) = %v，期望 %v（应当对称）", tc.b, tc.a, got, tc.want)
		}
	}
}
