package localagent

// lease.go 是资源键与租约参数的规范化，对应 Plan §8.1 的 v1 范围
// （只有「项目目录 + 端口」两类）与 §8.3 的租约语义。
//
// 它**不碰数据库**：SQL 只在 internal/store/localagent.go。这一层刻意做成
// 纯函数，好处是两个调用方（council claim / release，以及将来 serve -rw 上
// 的 HTTP 端点）对「这两个写法是不是同一个资源」不可能给出两种答案，而这份
// 判断能被表驱动测试逐条钉住（不必为每个用例开一次库）。
//
// 本模块不做强制锁（Plan §8.2）：租约是「我先说我占了，你愿意就让一让」。
// 因此这里的规范化宁可保守 —— 两个其实不同的路径被判成同一个资源，代价是
// 多等一次；判成两个资源，代价是两边同时写坏一个目录。

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// AccessMode 是租约的读写模式。读共享、写独占（Plan §8.3）。
//
// 取值与 internal/store 的 LeaseAccess 逐字一致；分成两个类型是因为这一层
// 不认识数据库，而 store 那一层只认识 SQL 参数。转换点在 cmd/council。
type AccessMode string

const (
	AccessRead  AccessMode = "read"
	AccessWrite AccessMode = "write"
)

// 租约的默认与边界 TTL。
//
// 为什么有上限：租约只是提醒，越长越像「永久占用」；而崩溃之后没人来交还的
// 租约要等到期才能被后来者接管，24 小时已经是「一个人一天不开机」的量级。
// 为什么不给 0 = 永久：那会让一次手滑永久占住一个目录，且没有任何界面能让
// 用户发现它（L2 没有租约列表页）。
const (
	DefaultLeaseTTL = 120 * time.Second
	MinLeaseTTL     = 5 * time.Second
	MaxLeaseTTL     = 24 * time.Hour
)

// tcpResourcePrefix 是端口类资源的键前缀，与 Plan §8.1 的示例
// （tcp:127.0.0.1:8765）一致。
const tcpResourcePrefix = "tcp:"

// ErrInvalidResource 是资源描述不合法时的哨兵。调用方（CLI）据此退 2
// （用法错误），而不是 1（环境/库的问题）。
var ErrInvalidResource = errors.New("invalid resource")

// NormalizeResource 把用户给的资源描述折成 resource_key。
//
// 两类，与 Plan §8.1 的 v1 范围一致：
//   - tcp:<port> / tcp:<host>:<port> → "tcp:<host>:<port>"，host 缺省
//     127.0.0.1（本仓库的看板只绑回环，丢一个裸端口不可能是别的地址）；
//   - 其它一律当路径：相对路径按当前目录展开成绝对路径，然后 Clean、
//     折小写、去尾部分隔符。
//
// 为什么折小写：Windows 的路径不分大小写，`D:\Work` 与 `d:\work` 是同一个
// 目录。在区分大小写的文件系统上这会把两个目录判成同一个资源 —— 本模块
// 接受这个偏差，因为「多等一次」比「两边同时写」便宜（文件头那段）。
//
// 为什么不做 filepath.EvalSymlinks：符号链接与目录联接的判定要真去碰文件
// 系统，Plan §8.1 把它留到 P5 之后。这里只做纯字符串规范化，不读盘。
func NormalizeResource(raw string) (string, error) {
	spec := strings.TrimSpace(raw)
	if spec == "" {
		return "", fmt.Errorf("%w: 资源不能为空（写目录路径，或 tcp:<host>:<port>）", ErrInvalidResource)
	}
	if len(spec) >= len(tcpResourcePrefix) && strings.EqualFold(spec[:len(tcpResourcePrefix)], tcpResourcePrefix) {
		return normalizePortResource(spec[len(tcpResourcePrefix):])
	}
	return normalizePathResource(spec)
}

// normalizePortResource 处理 tcp:<port> 与 tcp:<host>:<port>。
//
// 端口必须是 1-65535 的整数：0 在 TCP 里是「随便挑一个」，拿它当资源键
// 只会把互不相干的两个进程绑在一起。
func normalizePortResource(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", fmt.Errorf("%w: tcp 资源要写成 tcp:<host>:<port> 或 tcp:<port>", ErrInvalidResource)
	}
	host := "127.0.0.1"
	portPart := spec
	// 从右往左找最后一个冒号，这样 [::1]:8765 这种 IPv6 写法也能拆对。
	if i := strings.LastIndex(spec, ":"); i >= 0 {
		hostPart := strings.TrimSpace(spec[:i])
		portPart = strings.TrimSpace(spec[i+1:])
		if hostPart != "" {
			host = hostPart
		}
	}
	port, err := strconv.Atoi(portPart)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("%w: 端口 %q 不合法（1-65535 的整数）", ErrInvalidResource, portPart)
	}
	return tcpResourcePrefix + strings.ToLower(host) + ":" + strconv.Itoa(port), nil
}

// normalizePathResource 把路径折成绝对的、小写的、无尾部分隔符的键。
//
// 用 filepath.Abs 而不是自己拼 cwd：它还顺手 Clean（去掉 . / .. 与重复
// 分隔符），而 Windows 上的 / 与 \ 也会被统一成 \。
func normalizePathResource(spec string) (string, error) {
	abs, err := filepath.Abs(spec)
	if err != nil {
		return "", fmt.Errorf("%w: 无法把 %q 解析成绝对路径: %v", ErrInvalidResource, spec, err)
	}
	abs = filepath.Clean(abs)
	// Clean 只对根目录保留末尾分隔符（D:\ 与 \\server\share\），其余都已去掉；
	// 这里再折一次大小写，让 "D:\Work\" 与 "d:\work" 落到同一个键上。
	return strings.ToLower(abs), nil
}

// ParseAccessMode 解析 -mode 的取值。空串按 write：申请租约的默认意图是
// 「我要动它」，把空串当 read 会让一次手滑变成无效保护。
//
// 接受 r / w / read / write 四种写法（大小写不敏感）：命令行上敲一个 -mode w
// 比 -mode write 常见。
func ParseAccessMode(raw string) (AccessMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "w", "write":
		return AccessWrite, nil
	case "r", "read":
		return AccessRead, nil
	default:
		return "", fmt.Errorf("%w: mode %q 不合法（可选 read / write）", ErrInvalidResource, raw)
	}
}

// ResourceOverlaps 回答「两个 resource_key 是不是同一个地方或父子关系」。
//
// 用途只有一个：看板要提醒「你正站在的这个目录已经有人占了」（Plan §11 P5）。
// 判据刻意保守：
//   - 相等 → 重叠；
//   - 一个是另一个的父目录 → 重叠（在子目录里改文件，父目录的租约同样被踩）；
//   - tcp: 资源只按相等判（端口没有父子关系）；
//   - 其它情况一律不重叠 —— 不许把两个不相干的目录说成冲突。
//
// 两个入参都应当是已经规范化过的键（NormalizeResource 的输出），这样
// 大小写与分隔符的差异不会漏判。分隔符在这里统一折成 "/" 再比边界：
// Windows 上 NormalizeResource 产出 "\"，而手工写进库的旧键可能是 "/"。
func ResourceOverlaps(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	if strings.HasPrefix(a, tcpResourcePrefix) || strings.HasPrefix(b, tcpResourcePrefix) {
		return false
	}
	x := strings.ReplaceAll(a, `\`, "/")
	y := strings.ReplaceAll(b, `\`, "/")
	return isPathPrefix(x, y) || isPathPrefix(y, x)
}

// isPathPrefix 判断 child 是否在 parent 里面（含分隔符边界）。
// "/a/bc" 不算 "/a/b" 的子路径 —— 那是两个不同的目录。
func isPathPrefix(parent, child string) bool {
	if !strings.HasPrefix(child, parent) || len(child) <= len(parent) {
		return false
	}
	if !strings.HasSuffix(parent, "/") {
		return child[len(parent)] == '/'
	}
	return true
}

// NormalizeTTL 把 -ttl 折成合法区间。0 表示没给，取 DefaultLeaseTTL；
// 超出 [MinLeaseTTL, MaxLeaseTTL] 直接报错而不是悄悄夹到边界 ——
// `-ttl 1s` 是打错了，不是「想要 5 秒」。
func NormalizeTTL(ttl time.Duration) (time.Duration, error) {
	if ttl == 0 {
		return DefaultLeaseTTL, nil
	}
	if ttl < MinLeaseTTL {
		return 0, fmt.Errorf("ttl 太短：%s（最短 %s）", ttl, MinLeaseTTL)
	}
	if ttl > MaxLeaseTTL {
		return 0, fmt.Errorf("ttl 太长：%s（最长 %s；租约只是提醒，不是永久锁）", ttl, MaxLeaseTTL)
	}
	return ttl, nil
}
