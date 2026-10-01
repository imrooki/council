package provider

import (
	"crypto/rand"
	"fmt"
)

// newV4UUID 铸一个 RFC 4122 version 4 UUID（8-4-4-4-12）。
//
// 不引入 github.com/google/uuid：那是 modernc.org/sqlite 的传递依赖，不是本包的 API
// （docs/09_STEP6_SPEC.md B 节）。
//
// 只有 grok 的 BuildTurn 在 ProviderSession 为空时调用。crypto/rand 让这一支
// 不是纯函数。冻结的 Provider 接口没有单独的铸 id 方法，规格 D 节把铸币放在
// BuildTurn 内部：失败就丢弃 PlannedSessionID，下一轮重铸，从不复用、从不派生。
// resume 那一支不走这里，同样的非空 ProviderSession 仍然给出同样的 argv。
func newV4UUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
