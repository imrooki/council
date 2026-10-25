// gui5b_export_pagination_test.go 覆盖 B3「导出分页」的 serve 侧契约。
//
// 导出拼装本身在浏览器里（index.html 的 collectExportMessages），Go 测试
// 不执行 JS；这里钉的是它依赖的那条数据通路与同一套停止条件：
//
//   - 只读 server 上 after/limit 分页能连续取完 2500 条，一页不缺、不重；
//   - 50000 上限下不截断；
//   - 把上限压到 1200 时返回 truncated=true，且停在恰好 1200 条；
//   - 空页 / 不足一页 / lastSeq 不前进 三种坏响应都会停，不会死循环；
//   - 上限恰好等于总数时，探测页为空 => 不误报截断。
//
// 真正的浏览器路径（点导出、下载 .md、分页后文件里 2500 条齐全）由 CDP
// 验收覆盖，见交付报告。两边用同一组常量：EXPORT_LIMIT=1000、
// EXPORT_TOTAL_LIMIT=50000，锚点由 gui5b_export_pagination_ui_test.go 钉住。
package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"council/internal/conversation"
)

// gui5bPageSize 与 index.html 的 EXPORT_LIMIT / handler.go 的
// maxMessagesLimit 对齐。
const gui5bPageSize = 1000

// gui5bFetcher 取一页消息。after / limit 的语义与 GET /messages 相同。
type gui5bFetcher func(after int64, limit int) (messagesResponse, error)

// gui5bHTTPFetcher 用真实 HTTP 分页；断言 200 与 JSON 形状。
func gui5bHTTPFetcher(t *testing.T, baseURL, convID string) gui5bFetcher {
	t.Helper()
	return func(after int64, limit int) (messagesResponse, error) {
		url := fmt.Sprintf("%s/api/conversations/%s/messages?after=%d&limit=%d",
			baseURL, convID, after, limit)
		res, err := http.Get(url)
		if err != nil {
			return messagesResponse{}, err
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			return messagesResponse{}, err
		}
		if res.StatusCode != http.StatusOK {
			return messagesResponse{}, fmt.Errorf("GET %s = %d: %s", url, res.StatusCode, raw)
		}
		var page messagesResponse
		if err := json.Unmarshal(raw, &page); err != nil {
			return messagesResponse{}, fmt.Errorf("GET %s 不是合法 JSON: %w", url, err)
		}
		return page, nil
	}
}

// gui5bCollect 是 index.html collectExportMessages 的逐行镜像。
//
// 为什么在 Go 里再写一遍：这一层要验证的是接口能否支撑这套循环（分页、
// 上限、探测），浏览器里的同一套循环由 CDP 在真 Edge 里跑。两边都保留
// 一份判断的代价是改停止条件时要改两处；收益是任何一边单独漂移都会在
// 测试里露出来，而不是等用户导出时才发现少了几百条。
func gui5bCollect(t *testing.T, fetch gui5bFetcher, maxTotal int) (messages []messageJSON, truncated bool, requests int) {
	t.Helper()
	after := int64(0)
	for len(messages) < maxTotal {
		page, err := fetch(after, gui5bPageSize)
		if err != nil {
			t.Fatalf("第 %d 页失败: %v", requests+1, err)
		}
		requests++
		if len(page.Messages) == 0 {
			break
		}
		last := page.Messages[len(page.Messages)-1]
		// 防御：last_seq 不前进说明响应坏了（或代理重放了上一页），
		// 继续循环只会把同一批消息无限追加。
		if last.Seq <= after {
			break
		}
		room := maxTotal - len(messages)
		if len(page.Messages) > room {
			messages = append(messages, page.Messages[:room]...)
			truncated = true
			break
		}
		messages = append(messages, page.Messages...)
		after = last.Seq
		if len(messages) >= maxTotal {
			probe, err := fetch(after, 1)
			if err != nil {
				t.Fatalf("上限探测页失败: %v", err)
			}
			requests++
			truncated = len(probe.Messages) > 0
			break
		}
		if len(page.Messages) < gui5bPageSize {
			break
		}
	}
	return messages, truncated, requests
}

// gui5bSeedMessages 在一个写事务里追加 n 条消息（2500 条逐条 WriteTx 太慢，
// 而 AppendMessage 本身可以在同一个事务里连续调用）。每 25 条插一条
// system，给「仅对话」过滤留出现成的样本；正文带序号，方便断言不重不漏。
func gui5bSeedMessages(t *testing.T, env *serveTestEnv, convID string, n int) []conversation.Message {
	t.Helper()
	ctx := env.ctx(t)
	created := make([]conversation.Message, 0, n)
	err := env.store.WriteTx(ctx, func(ctx context.Context, tx conversation.Tx) error {
		for i := 1; i <= n; i++ {
			sender := "alpha"
			kind := conversation.KindAgent
			if i%25 == 0 {
				sender = conversation.ReservedSenderSystem
				kind = conversation.KindSystem
			}
			msg, err := tx.AppendMessage(ctx, conversation.Message{
				ConvID:   convID,
				SenderID: sender,
				Kind:     kind,
				Text:     fmt.Sprintf("分页消息 %05d | pagination-%05d", i, i),
			})
			if err != nil {
				return err
			}
			created = append(created, msg)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("批量追加 %d 条消息失败: %v", n, err)
	}
	return created
}

// TestGui5ExportPaginationCollects2500Messages 是 B3 的主验收：2500 条消息
// 在 50000 上限下由分页循环全部取回，页数 3（1000 + 1000 + 500），
// 不截断、不重复、序号连续。
func TestGui5ExportPaginationCollects2500Messages(t *testing.T) {
	env := newServeTestEnv(t, nil)
	conv := env.createConversation(t, "导出分页：2500 条")
	seed := gui5bSeedMessages(t, env, conv.ID, 2500)

	got, truncated, requests := gui5bCollect(t,
		gui5bHTTPFetcher(t, env.http.URL, conv.ID), 50000)

	if truncated {
		t.Errorf("2500 条在 50000 上限下被标成截断，期望不截断")
	}
	if len(got) != 2500 {
		t.Fatalf("分页取回 %d 条，期望 2500 条", len(got))
	}
	if requests != 3 {
		t.Errorf("分页请求 %d 次，期望 3 次（1000 + 1000 + 500）", requests)
	}

	// 导出件按 seq 升序逐条写；这里检查导出源数据的完整性。
	for i, msg := range got {
		if msg.Seq != int64(i+1) {
			t.Fatalf("第 %d 条的 seq = %d，期望 %d（分页有洞或重复）", i+1, msg.Seq, i+1)
		}
		if msg.Text != seed[i].Text {
			t.Fatalf("第 %d 条的正文 = %q，期望 %q", i+1, msg.Text, seed[i].Text)
		}
		if msg.Kind != string(seed[i].Kind) {
			t.Fatalf("第 %d 条的 kind = %q，期望 %q", i+1, msg.Kind, seed[i].Kind)
		}
	}
	if first, last := got[0], got[len(got)-1]; first.Seq != 1 || last.Seq != 2500 {
		t.Errorf("分页区间 = [%d, %d]，期望 [1, 2500]", first.Seq, last.Seq)
	}

	// 2500 条里 100 条 system（每 25 条一条），仅对话导出应留下 2400 条。
	system := 0
	for _, msg := range got {
		if msg.Kind == string(conversation.KindSystem) {
			system++
		}
	}
	if system != 100 {
		t.Errorf("分页结果里 system 消息 %d 条，期望 100 条", system)
	}

	// 用导出件同样的消息行拼一份文档：断言「文件里」每条正文都在，
	// 而不是只相信内存里那个切片。
	var doc strings.Builder
	for _, msg := range got {
		fmt.Fprintf(&doc, "**%s** · %s\n%s\n\n", msg.SenderID, msg.CreatedAt.Format("2006-01-02 15:04"), msg.Text)
	}
	if !strings.Contains(doc.String(), "pagination-00001") ||
		!strings.Contains(doc.String(), "pagination-01250") ||
		!strings.Contains(doc.String(), "pagination-02500") {
		t.Errorf("导出文档缺少首 / 中 / 尾消息样本")
	}
	if n := strings.Count(doc.String(), "pagination-"); n != 2500 {
		t.Errorf("导出文档里消息行有 %d 条，期望 2500 条", n)
	}
}

// gui5bFakeFetcher 返回固定的一页；同一页会被循环反复请求，用来验证
// 「lastSeq 不前进就停」。
func gui5bFakeFetcher(page messagesResponse) gui5bFetcher {
	return func(after int64, limit int) (messagesResponse, error) {
		return page, nil
	}
}

// gui5bMessages 造 seq 从 from 到 to 的消息页（闭区间）。
func gui5bMessages(from, to int64) []messageJSON {
	out := make([]messageJSON, 0, to-from+1)
	for seq := from; seq <= to; seq++ {
		out = append(out, messageJSON{
			Seq:       seq,
			SenderID:  "alpha",
			Kind:      string(conversation.KindAgent),
			Text:      fmt.Sprintf("pagination-%05d", seq),
			CreatedAt: time.Unix(seq, 0).UTC(),
		})
	}
	return out
}

// TestGui5ExportPaginationTruncationAndStops 覆盖「把上限调低」与三种停止
// 条件。上限压到 1200 时第二页跨过上限：只带走 200 条、标 truncated，
// 不再请求第三页。
func TestGui5ExportPaginationTruncationAndStops(t *testing.T) {
	t.Run("上限压到 1200 时截断", func(t *testing.T) {
		env := newServeTestEnv(t, nil)
		conv := env.createConversation(t, "导出分页：压低上限")
		gui5bSeedMessages(t, env, conv.ID, 2500)

		got, truncated, requests := gui5bCollect(t,
			gui5bHTTPFetcher(t, env.http.URL, conv.ID), 1200)
		if !truncated {
			t.Errorf("上限 1200 时 truncated = false，期望 true")
		}
		if len(got) != 1200 {
			t.Fatalf("上限 1200 时取回 %d 条，期望恰好 1200 条", len(got))
		}
		if requests != 2 {
			t.Errorf("截断后请求 %d 次，期望 2 次（达到上限就停，不再拉后面的页）", requests)
		}
		if got[len(got)-1].Seq != 1200 {
			t.Errorf("最后一条 seq = %d，期望 1200", got[len(got)-1].Seq)
		}
		// 截断必须带着提示一起交出去：导出件顶部与 toast 各一处。
		// 文案在 index.html 里，单独由结构锚点测试钉住；这里再确认
		// 「truncated=true 时前端确实有可写的提示」，避免只回一个布尔。
		if !strings.Contains(string(indexHTML), "> **已截断**：") ||
			!strings.Contains(string(indexHTML), "条上限，已截断") {
			t.Errorf("index.html 缺少截断提示文案（导出件顶部 / 界面 toast）")
		}
	})

	t.Run("空页立即停止", func(t *testing.T) {
		got, truncated, requests := gui5bCollect(t,
			gui5bFakeFetcher(messagesResponse{Messages: []messageJSON{}}), 50000)
		if len(got) != 0 || truncated || requests != 1 {
			t.Errorf("空页的结果 = %d 条 / truncated=%v / %d 次请求，期望 0 / false / 1",
				len(got), truncated, requests)
		}
	})

	t.Run("不足一页立即停止", func(t *testing.T) {
		page := messagesResponse{Messages: gui5bMessages(1, 7), LastSeq: 7}
		got, truncated, requests := gui5bCollect(t, gui5bFakeFetcher(page), 50000)
		if len(got) != 7 || truncated || requests != 1 {
			t.Errorf("不足一页的结果 = %d 条 / truncated=%v / %d 次请求，期望 7 / false / 1",
				len(got), truncated, requests)
		}
	})

	t.Run("lastSeq 不前进立即停止", func(t *testing.T) {
		// 同一页 1000 条（满页）被反复返回：第二次请求 lastSeq 仍停在 1000，
		// 必须停下而不是无限追加同一批消息。
		page := messagesResponse{Messages: gui5bMessages(1, 1000), LastSeq: 1000}
		got, truncated, requests := gui5bCollect(t, gui5bFakeFetcher(page), 5000)
		if len(got) != 1000 || truncated {
			t.Errorf("lastSeq 卡住的结果 = %d 条 / truncated=%v，期望 1000 / false", len(got), truncated)
		}
		if requests < 2 || requests > 3 {
			t.Errorf("lastSeq 卡住时请求 %d 次，期望 2 次左右（不能无限循环）", requests)
		}
	})

	t.Run("恰好到上限且没有更多时不误报截断", func(t *testing.T) {
		calls := 0
		fetch := func(after int64, limit int) (messagesResponse, error) {
			calls++
			if calls == 1 {
				return messagesResponse{Messages: gui5bMessages(1, 1000), LastSeq: 1000}, nil
			}
			// 探测页：后面没有了。
			return messagesResponse{Messages: []messageJSON{}, LastSeq: after}, nil
		}
		got, truncated, requests := gui5bCollect(t, fetch, 1000)
		if len(got) != 1000 || truncated {
			t.Errorf("恰好 1000 条的结果 = %d 条 / truncated=%v，期望 1000 / false", len(got), truncated)
		}
		if requests != 2 {
			t.Errorf("恰好到上限时请求 %d 次，期望 2 次（含一次探测）", requests)
		}
	})

	t.Run("恰好到上限且还有更多时截断", func(t *testing.T) {
		calls := 0
		fetch := func(after int64, limit int) (messagesResponse, error) {
			calls++
			if calls == 1 {
				return messagesResponse{Messages: gui5bMessages(1, 1000), LastSeq: 1000}, nil
			}
			return messagesResponse{Messages: gui5bMessages(1001, 1001), LastSeq: 1001}, nil
		}
		got, truncated, requests := gui5bCollect(t, fetch, 1000)
		if len(got) != 1000 || !truncated {
			t.Errorf("上限外还有消息的结果 = %d 条 / truncated=%v，期望 1000 / true", len(got), truncated)
		}
		if requests != 2 {
			t.Errorf("探测后请求 %d 次，期望 2 次", requests)
		}
	})
}
