package main

// leaseclient_test.go 覆盖 `council claim -server` / `council release -server`：
// 看板握着 council.lock 时，命令行走那条已持锁的进程，而不是自己开库。
//
// 这里的服务端是一个**手写的小 handler**，不是真的 serve.Server：本包要测的
// 是客户端的请求形状与退出码映射，「看板那边怎么实现」由 internal/serve 的
// 用例负责。两个契约点（路径、JSON 字段）在两边各自钉一次 —— 漂了会同时红。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// leaseFakeDashboard 是一个最小的看板替身：把收到的请求记下来，返回给定的
// 状态码与响应体。
type leaseFakeDashboard struct {
	status   int
	respBody string
	// 收到的最后一次请求
	path    string
	reqBody map[string]any
}

func (f *leaseFakeDashboard) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.path = r.URL.Path
		f.reqBody = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&f.reqBody)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.respBody))
	})
}

func newLeaseFakeDashboard(t *testing.T, status int, body string) (*leaseFakeDashboard, string) {
	t.Helper()
	fake := &leaseFakeDashboard{status: status, respBody: body}
	ts := httptest.NewServer(fake.handler())
	t.Cleanup(ts.Close)
	return fake, ts.URL
}

// leaseGrantedBody 是一份合法的「申请成功」响应（字段与 serve 侧一致）。
func leaseGrantedBody(leaseID string) string {
	return fmt.Sprintf(`{"granted":true,"local_agent":"coordinate","lease":{
		"lease_id":%q,"resource_key":"c:\\work\\proj","owner_kind":"dashboard",
		"owner_id":"dashboard","mode":"write","generation":2,
		"granted_at":"2026-10-27T10:00:00Z","expires_at":"2026-10-27T10:02:00Z",
		"overlaps_cwd":false}}`, leaseID)
}

// TestClaimOverServer 覆盖成功路径：请求形状（路径 / 字段）与输出。
func TestClaimOverServer(t *testing.T) {
	whoTestEnv(t)
	fake, url := newLeaseFakeDashboard(t, http.StatusOK, leaseGrantedBody("lease-abc"))

	_, errOut, code := claimRun(t, claimPanicDeps(t),
		"-server", url, "-mode", "read", "-ttl", "90s", "-owner", "me", `D:\work\proj`)
	if code != exitOK {
		t.Fatalf("退出码 = %d，期望 0；stderr=%s", code, errOut)
	}
	if fake.path != "/api/local-agents/lease" {
		t.Errorf("请求路径 = %q，期望 /api/local-agents/lease", fake.path)
	}
	wantBody := map[string]any{
		"resource":    `d:\work\proj`,
		"mode":        "read",
		"ttl_seconds": float64(90),
		"owner":       "me",
	}
	for key, want := range wantBody {
		if got := fake.reqBody[key]; got != want {
			t.Errorf("请求体 %s = %v，期望 %v（完整 body: %v）", key, got, want, fake.reqBody)
		}
	}
}

// TestClaimOverServerJSON 钉住 -json 的形状：granted、lease、以及 via=server。
func TestClaimOverServerJSON(t *testing.T) {
	whoTestEnv(t)
	_, url := newLeaseFakeDashboard(t, http.StatusOK, leaseGrantedBody("lease-json"))

	out, errOut, code := claimRun(t, claimPanicDeps(t),
		"-server", url, "-json", `D:\work\proj`)
	if code != exitOK {
		t.Fatalf("退出码 = %d，期望 0；stderr=%s", code, errOut)
	}
	var report claimReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("stdout 不是 JSON: %v；%s", err, out)
	}
	if !report.Granted || report.Lease == nil || report.Lease.LeaseID != "lease-json" {
		t.Fatalf("报告 = %+v", report)
	}
	if report.Via != "server" {
		t.Errorf("via = %q，期望 server（脚本据此分辨走的是哪条路）", report.Via)
	}
	if report.Lease.Generation != 2 || report.Lease.OwnerKind != "dashboard" {
		t.Errorf("租约行 = %+v", report.Lease)
	}
}

// TestClaimOverServerDenied 覆盖三种「看板拒绝」：409 / 403 → 退出 3（被拒），
// 400 → 退出 2（用法）。看板那句话要原样出现。
func TestClaimOverServerDenied(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   int
		wantMs string
	}{
		{
			name:   "409 被占用",
			status: http.StatusConflict,
			body:   `{"error":"资源 d:\\work 已被占用：cli/first 持有 write 租约（lease x，到 2026-10-27T10:02:00Z，generation 1）"}`,
			want:   exitLeaseDenied,
			wantMs: "cli/first",
		},
		{
			name:   "403 层级未开",
			status: http.StatusForbidden,
			body:   `{"error":"本机 agent 协调未开启：把设置页第 6 类的层级改成「资源协调」"}`,
			want:   exitLeaseDenied,
			wantMs: "资源协调",
		},
		{
			name:   "400 资源不合法",
			status: http.StatusBadRequest,
			body:   `{"error":"端口 \"70000\" 不合法"}`,
			want:   exitUsage,
			wantMs: "不合法",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			whoTestEnv(t)
			_, url := newLeaseFakeDashboard(t, tc.status, tc.body)
			_, errOut, code := claimRun(t, claimPanicDeps(t), "-server", url, `D:\work`)
			if code != tc.want {
				t.Fatalf("退出码 = %d，期望 %d；stderr=%s", code, tc.want, errOut)
			}
			if !strings.Contains(errOut, tc.wantMs) {
				t.Errorf("stderr 没有转述看板那句话（缺 %q）:\n%s", tc.wantMs, errOut)
			}
		})
	}
}

// TestClaimOverServerUnreachable 连不上 → 退出 1，并提示地址从看板启动行里拿。
func TestClaimOverServerUnreachable(t *testing.T) {
	whoTestEnv(t)
	ts := httptest.NewServer(http.NotFoundHandler())
	url := ts.URL
	ts.Close() // 立刻关掉：端口上没人听

	_, errOut, code := claimRun(t, claimPanicDeps(t), "-server", url, `D:\work`)
	if code != exitConfigFailed {
		t.Fatalf("退出码 = %d，期望 %d；stderr=%s", code, exitConfigFailed, errOut)
	}
	if !strings.Contains(errOut, "连不上") {
		t.Errorf("stderr 没有说清连不上:\n%s", errOut)
	}
}

// TestClaimServerFlagValidation 覆盖 -server 的用法错误：非回环地址、
// 与 -config 同时给、空值。
func TestClaimServerFlagValidation(t *testing.T) {
	whoTestEnv(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"非回环", []string{"-server", "http://10.1.2.3:8765", `D:\work`}, "回环"},
		{"缺主机", []string{"-server", "http://", `D:\work`}, "主机"},
		{"坏协议", []string{"-server", "ftp://127.0.0.1:1", `D:\work`}, "协议"},
		{"与 config 冲突", []string{"-server", "http://127.0.0.1:8765", "-config", "x.toml", `D:\work`}, "不能同时给"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errOut, code := claimRun(t, claimPanicDeps(t), tc.args...)
			if code != exitUsage {
				t.Fatalf("退出码 = %d，期望 2；stderr=%s", code, errOut)
			}
			if !strings.Contains(errOut, tc.want) {
				t.Errorf("stderr 没有提到 %q:\n%s", tc.want, errOut)
			}
		})
	}
}

// TestReleaseOverServer 覆盖交还：请求形状、成功、空结果与 404。
func TestReleaseOverServer(t *testing.T) {
	released := `{"granted":false,"released":[{"lease_id":"lease-1","resource_key":"c:\\work",
		"owner_kind":"dashboard","owner_id":"dashboard","mode":"write","generation":1,
		"granted_at":"2026-10-27T10:00:00Z","expires_at":"2026-10-27T10:02:00Z"}]}`

	t.Run("按 id 交还成功", func(t *testing.T) {
		whoTestEnv(t)
		fake, url := newLeaseFakeDashboard(t, http.StatusOK, released)
		out, errOut, code := releaseRun(t, claimPanicDeps(t), "-server", url, "-lease", "lease-1")
		if code != exitOK {
			t.Fatalf("退出码 = %d，期望 0；stderr=%s", code, errOut)
		}
		if fake.path != "/api/local-agents/lease/release" {
			t.Errorf("请求路径 = %q", fake.path)
		}
		if fake.reqBody["lease_id"] != "lease-1" {
			t.Errorf("请求体 = %v，期望带 lease_id", fake.reqBody)
		}
		if !strings.Contains(out, "已交还 1 条租约") || !strings.Contains(out, "lease-1") {
			t.Errorf("输出不对:\n%s", out)
		}
	})

	t.Run("按 id 空结果是 0", func(t *testing.T) {
		whoTestEnv(t)
		_, url := newLeaseFakeDashboard(t, http.StatusOK, `{"granted":false,"released":[]}`)
		_, errOut, code := releaseRun(t, claimPanicDeps(t), "-server", url, "-lease", "lease-1")
		if code != exitOK {
			t.Fatalf("退出码 = %d，期望 0（重复交还）；stderr=%s", code, errOut)
		}
	})

	t.Run("按资源空结果是 3", func(t *testing.T) {
		whoTestEnv(t)
		_, url := newLeaseFakeDashboard(t, http.StatusOK, `{"granted":false,"released":[]}`)
		_, errOut, code := releaseRun(t, claimPanicDeps(t), "-server", url,
			"-resource", `D:\work`, "-owner", "me")
		if code != exitLeaseDenied {
			t.Fatalf("退出码 = %d，期望 3（没匹配到）；stderr=%s", code, errOut)
		}
	})

	t.Run("404 是 3", func(t *testing.T) {
		whoTestEnv(t)
		_, url := newLeaseFakeDashboard(t, http.StatusNotFound, `{"error":"没有这条租约: x"}`)
		_, errOut, code := releaseRun(t, claimPanicDeps(t), "-server", url, "-lease", "x")
		if code != exitLeaseDenied {
			t.Fatalf("退出码 = %d，期望 3；stderr=%s", code, errOut)
		}
		if !strings.Contains(errOut, "没有这条租约") {
			t.Errorf("stderr 没有转述那句话:\n%s", errOut)
		}
	})
}

// TestLeaseServerRejectsRemoteHost 钉住「只连回环」这条安全边界（纯函数）。
func TestLeaseServerRejectsRemoteHost(t *testing.T) {
	ok := []string{
		"http://127.0.0.1:8765",
		"http://localhost:8765/",
		"HTTP://[::1]:8765",
	}
	for _, raw := range ok {
		if _, err := normalizeLeaseServer(raw); err != nil {
			t.Errorf("normalizeLeaseServer(%q) 应当通过: %v", raw, err)
		}
	}
	bad := []string{
		"http://192.168.1.10:8765",
		"https://example.com",
		"ftp://127.0.0.1:1",
		"127.0.0.1:8765", // 没有协议
		"",
	}
	for _, raw := range bad {
		if _, err := normalizeLeaseServer(raw); err == nil {
			t.Errorf("normalizeLeaseServer(%q) 应当被拒", raw)
		}
	}
	if got, err := normalizeLeaseServer("http://127.0.0.1:8765/"); err != nil || got != "http://127.0.0.1:8765" {
		t.Errorf("末尾斜杠没有去掉: %q, %v", got, err)
	}
}
