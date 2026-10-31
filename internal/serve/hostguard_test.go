package serve

// hostguard_test.go 钉住 S1：看板的每一个请求（含 GET 与 `/`）都只接受
// 指向本机回环地址的 Host。DNS rebinding 下浏览器的 Host 是攻击者域名，
// 只比 Origin 与 Host 是否相等是挡不住的。

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRequireLoopbackHost(t *testing.T) {
	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8765}
	called := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	})
	guarded := requireLoopbackHost(next, addr)

	cases := []struct {
		name   string
		method string
		host   string
		want   int
	}{
		{"IPv4 回环", http.MethodGet, "127.0.0.1:8765", http.StatusNoContent},
		{"localhost", http.MethodGet, "localhost:8765", http.StatusNoContent},
		{"IPv6 回环", http.MethodGet, "[::1]:8765", http.StatusNoContent},
		{"127/8 内其它回环", http.MethodGet, "127.0.0.2:8765", http.StatusNoContent},
		{"伪造域名 GET", http.MethodGet, "evil.example:8765", http.StatusMisdirectedRequest},
		{"伪造域名 POST", http.MethodPost, "evil.example:8765", http.StatusMisdirectedRequest},
		{"端口不符", http.MethodGet, "127.0.0.1:8766", http.StatusMisdirectedRequest},
		{"没有端口", http.MethodGet, "localhost", http.StatusMisdirectedRequest},
		{"空 Host", http.MethodGet, "", http.StatusMisdirectedRequest},
		{"通配地址", http.MethodGet, "0.0.0.0:8765", http.StatusMisdirectedRequest},
		{"域名前缀", http.MethodGet, "localhost.evil.example:8765", http.StatusMisdirectedRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called = 0
			req := httptest.NewRequest(tc.method, "http://example/", nil)
			req.Host = tc.host
			rec := httptest.NewRecorder()
			guarded.ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("Host %q 的 %s 请求状态 = %d，期望 %d；body=%s",
					tc.host, tc.method, rec.Code, tc.want, rec.Body.String())
			}
			if tc.want == http.StatusNoContent && called != 1 {
				t.Errorf("Host %q 应放行到下一个 handler，实际 called=%d", tc.host, called)
			}
			if tc.want == http.StatusMisdirectedRequest && called != 0 {
				t.Errorf("Host %q 应被挡在 handler 之前，实际 called=%d", tc.host, called)
			}
			if tc.want == http.StatusMisdirectedRequest && !strings.Contains(rec.Body.String(), "Host") {
				t.Errorf("拒绝响应应说明 Host，实际 body=%q", rec.Body.String())
			}
		})
	}
}

// TestListenAndServeWrapsHostGuard 证明 guard 真的装在了生产路径上，而不是
// 只有测试直接调用的那个函数。用 127.0.0.1:0 起一个真监听，再伪造 Host。
func TestListenAndServeWrapsHostGuard(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := newSyncBuffer()
	done := make(chan error, 1)
	go func() {
		done <- listenAndServe(ctx, "127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "ok")
		}), browserOptions{}, out, io.Discard, nil)
	}()

	var url string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		line := strings.TrimSpace(out.String())
		if strings.HasPrefix(line, "council serve: http://") {
			url = strings.TrimPrefix(line, "council serve: ")
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if url == "" {
		cancel()
		t.Fatalf("5 秒内没有打印启动行，实际 %q", out.String())
	}

	req, err := http.NewRequest(http.MethodGet, url+"/", nil)
	if err != nil {
		t.Fatalf("造请求失败: %v", err)
	}
	req.Host = "evil.example"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("伪造 Host 请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("伪造 Host 的 GET 状态 = %d，期望 421", resp.StatusCode)
	}

	resp2, err := http.Get(url + "/")
	if err != nil {
		t.Fatalf("正常 Host 请求失败: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("正常 Host 的 GET 状态 = %d，期望 200", resp2.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("取消 ctx 之后 listenAndServe 返回错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("取消 ctx 之后 listenAndServe 5 秒没有返回")
	}
}
