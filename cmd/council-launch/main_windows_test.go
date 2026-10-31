//go:build windows

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestB16DashboardRunningUsesHealth(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			t.Errorf("探测路径 = %q，期望 /api/health", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer ok.Close()
	if !dashboardRunningAt(ok.URL) {
		t.Error("健康检查返回 ok=true 时应判为在跑")
	}

	notCouncil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html>another app</html>")
	}))
	defer notCouncil.Close()
	if dashboardRunningAt(notCouncil.URL) {
		t.Error("200 的 HTML 不是 Council 看板，不该判为在跑")
	}

	if dashboardRunningAt("http://127.0.0.1:1") {
		t.Error("没有服务时不该判为在跑")
	}
}
