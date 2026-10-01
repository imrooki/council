package provider

// fu1：MD1 独立审查余项在 provider 包的补测。只加用例，不动生产代码。
// 只调用生产标识符（isClaudeDiscoveryResponse、DiscoveryCommandFor、ParseDiscovery），
// 不借用本包其它测试文件的助手。

import (
	"errors"
	"testing"
)

// TestFuxClaudeDiscoveryNilOuterResponse 钉外层 response 缺席：{"type":"control_response"} 一行。
// 握手谓词（经 DiscoveryCommandFor 取到的 CloseStdinAfter，与 isClaudeDiscoveryResponse 本身）
// 都返回 false，ParseDiscovery("claude") 返回 ErrDiscoveryUnparseable。
//
// 【判别力】变异：discovery.go 里 claudeDiscoveryResponse 的 v.Response == nil 判据删掉。
// 这时 v.Response.RequestID 解引用 nil，panic，用例红。
func TestFuxClaudeDiscoveryNilOuterResponse(t *testing.T) {
	line := []byte(`{"type":"control_response"}`)
	cmd, err := DiscoveryCommandFor("claude", "claude.exe")
	if err != nil || cmd.CloseStdinAfter == nil {
		t.Fatalf("前提不成立：claude 的识别命令应当带 CloseStdinAfter，实际 err=%v", err)
	}
	if cmd.CloseStdinAfter(line) {
		t.Fatalf("CloseStdinAfter 对外层 response 缺席的一行应当返回 false")
	}
	if isClaudeDiscoveryResponse(line) {
		t.Fatalf("isClaudeDiscoveryResponse 对外层 response 缺席的一行应当返回 false")
	}
	models, err := ParseDiscovery("claude", append(line, '\n'))
	if !errors.Is(err, ErrDiscoveryUnparseable) || models != nil {
		t.Fatalf("ParseDiscovery 应当返回 ErrDiscoveryUnparseable 且没有模型，实际 models=%v err=%v", models, err)
	}
}

// TestFuxClaudeDiscoveryNilInnerResponse 钉内层 response 缺席：subtype success、request_id 匹配，
// 但没有 response.response。握手谓词认它（这一行就是本次 initialize 的回应），
// ParseDiscovery("claude") 返回 ErrDiscoveryUnparseable，错误文本逐字等于实跑值。
//
// 【判别力】变异：discovery_parse.go 里 v.Response.Response == nil 判据删掉。
// 这时 v.Response.Response.Models 解引用 nil，panic，用例红。
func TestFuxClaudeDiscoveryNilInnerResponse(t *testing.T) {
	line := []byte(`{"type":"control_response","response":{"subtype":"success","request_id":"council-models"}}`)
	if !isClaudeDiscoveryResponse(line) {
		t.Fatalf("前提不成立：request_id 匹配的 control_response 应当被握手谓词认出")
	}
	models, err := ParseDiscovery("claude", append(line, '\n'))
	if !errors.Is(err, ErrDiscoveryUnparseable) || models != nil {
		t.Fatalf("ParseDiscovery 应当返回 ErrDiscoveryUnparseable 且没有模型，实际 models=%v err=%v", models, err)
	}
	const want = `claude: control_response subtype "success": model discovery output is not a known shape`
	if err.Error() != want {
		t.Fatalf("错误文本不对\n实际: %q\n期望: %q", err.Error(), want)
	}
}
