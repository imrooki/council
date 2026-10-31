package repl

// P2：启动时实际读的层要传给 REPL，不能在 reload 时按默认位置重算。

import (
	"path/filepath"
	"reflect"
	"testing"

	"council/internal/config"
)

func TestReloadLayersUsesStartupLayers(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, ".council", "config.toml")
	global := filepath.Join(dir, "config.toml")
	want := []config.Layer{{Path: project}, {Path: global}}
	got := reloadLayers(RulesContext{ConfigPath: project, Layers: want})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reloadLayers = %+v，期望启动层 %+v", got, want)
	}
	// 不能把调用方的切片别名交出去。
	got[0].Path = "mutated"
	if want[0].Path != project {
		t.Fatal("reloadLayers 返回的切片与启动层共享底层数组")
	}
}
