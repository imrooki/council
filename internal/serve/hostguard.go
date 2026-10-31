package serve

// hostguard.go 是 S1 的落点：看板只绑回环，攻击面却不止「谁能连上」。
// DNS rebinding 让恶意网页用自己的域名访问 127.0.0.1，此时浏览器的
// Origin 与 Host 都是攻击者域名，checkWriteOrigin 的「Origin == Host」
// 天然成立。唯一的可靠判据是 Host 本身必须是本机回环地址与本次监听的端口。

import (
	"net"
	"net/http"
	"strconv"
	"strings"
)

// requireLoopbackHost 把 addr 解析出的「实际端口」固定下来，只放行
// host 为 localhost 或任意回环 IP、且端口正好等于该端口的请求。
//
// 为什么端口也要精确匹配：同一台机器上另一个本地服务不能借我们的
// 域名/端口组合绕过同源判断；而 addr 是 listener 的实际地址，
// -addr 127.0.0.1:0 时端口是内核分配的，不能拿 flag 里的 0 来比。
//
// 返回 421 而不是 403：请求确实到了错误的 Host，重定向/换地址才是
// 正确的语义；403 会被误读成「你有权限问题」。
func requireLoopbackHost(next http.Handler, addr net.Addr) http.Handler {
	port := listenPort(addr)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHostPort(r.Host, port) {
			writeError(w, http.StatusMisdirectedRequest,
				"拒绝请求：Host 不是本机回环地址（只接受 127.0.0.1、localhost 或 [::1] 加本次监听的端口）")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// listenPort 从 listener 的实际地址里取端口。拿不到时返回空串，
// isLoopbackHostPort 会因此拒绝所有请求 —— fail-closed，不猜。
func listenPort(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	if tcp, ok := addr.(*net.TCPAddr); ok {
		return strconv.Itoa(tcp.Port)
	}
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return ""
	}
	return port
}

// isLoopbackHostPort 只认字面量回环主机：localhost（大小写不敏感）或
// net.ParseIP 判定为回环的地址。不查 DNS：一个名字现在解析到 127.0.0.1
// 不代表它永远只解析到那里；查 DNS 反而给 rebinding 留了窗口。
func isLoopbackHostPort(hostport, wantPort string) bool {
	if wantPort == "" {
		return false
	}
	host, port, err := net.SplitHostPort(strings.TrimSpace(hostport))
	if err != nil || host == "" || port != wantPort {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
