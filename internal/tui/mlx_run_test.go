package tui

// 本文件覆盖冻结契约 X44 v1.2（E:\multi_model_cowork\x44_contract.md）§2.4 的 R1：
// 真 tui.Run，os.Pipe 做 in 与 stdout，括号粘贴字节走真输入解码。
//
// 背对背纪律：本文件由测试腿在看不见实现腿产出的情况下写成，依据只有契约文本与 HEAD 80a9385 的源码。
// 只用 HEAD 已有的标识符，在 HEAD 上能编译（契约 §2.0 末条）。
//
// 装置形状照 rba_wiring_test.go（读 goroutine 在 Run 之前起；收尾 cancel、等 Run、关 inW、inR、outW、
// 等读 goroutine、关 outR，每步 5 秒超时，幂等，挂进 t.Cleanup）。
//
// 【为什么是子串】终端字节流，同 RB-A W1：剥 ANSI、删 ASCII 空格之后找子串。

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// mlxHF 是版本为 test 的提示表头删掉空格之后的样子（契约 §2.4 的 HF）。
var mlxHF = strings.ReplaceAll("council test   "+mlxH, " ", "")

// mlxRunRig 是一台跑着的真 Run。
type mlxRunRig struct {
	g   *rbaTuiRig
	buf *tui1dBuf
	inW *os.File
}

func (r *mlxRunRig) flatHas(d time.Duration, want string) bool {
	return tui1dWaitFor(r.buf, d, func(raw string) bool { return strings.Contains(rbaFlat(raw), want) })
}

func (r *mlxRunRig) write(t *testing.T, what, s string) {
	t.Helper()
	if _, err := r.inW.Write([]byte(s)); err != nil {
		t.Fatalf("前提不成立：往输入管道写 %s 失败：%v", what, err)
	}
}

// mlxStartRun 起真 Run 并走完契约 §2.4 的同步点，最后写入括号粘贴与回车。任何一步等不到都是前提不成立。
func mlxStartRun(t *testing.T, paste string) *mlxRunRig {
	t.Helper()
	g := rbaNewTuiRig(t)

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("前提不成立：os.Pipe() 造输入管道失败：%v", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		_ = inW.Close()
		_ = inR.Close()
		t.Fatalf("前提不成立：os.Pipe() 造输出管道失败：%v", err)
	}

	buf := &tui1dBuf{}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		_, _ = io.Copy(buf, outR)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- Run(ctx, g.r, g.lw, "test", inR, outW) }()

	tornDown := false
	teardown := func() {
		if tornDown {
			return
		}
		tornDown = true
		cancel()
		select {
		case <-runDone:
		case <-time.After(5 * time.Second):
			t.Errorf("收尾：cancel() 之后 5 秒 Run 仍没有返回")
		}
		_ = inW.Close()
		_ = inR.Close()
		_ = outW.Close()
		select {
		case <-readDone:
		case <-time.After(5 * time.Second):
			t.Errorf("收尾：关掉 outW 之后 5 秒读 goroutine 仍没有结束")
		}
		_ = outR.Close()
	}
	t.Cleanup(teardown)

	r := &mlxRunRig{g: g, buf: buf, inW: inW}
	if !tui1dWaitFor(buf, 5*time.Second, func(raw string) bool { return strings.Contains(raw, tui1dESC+"[J") }) {
		t.Fatalf("前提不成立：5 秒内 stdout 管道里没有出现渲染器的 ESC[J；内容 %q", buf.String())
	}
	r.write(t, "窗口尺寸报告", tui1dESC+"[8;24;100t")
	if !r.flatHas(5*time.Second, "counciltest空闲") {
		t.Fatalf("前提不成立：写入窗口尺寸报告后 5 秒内没有看到空闲表头；内容 %q", rbaFlat(buf.String()))
	}
	r.write(t, "/use", "/use "+g.convID+"\r")
	if !r.flatHas(5*time.Second, "当前会话:") {
		t.Fatalf("前提不成立：写入 /use 后 5 秒内没有看到「当前会话:」；内容 %q", rbaFlat(buf.String()))
	}
	r.write(t, "括号粘贴", tui1dESC+"[200~"+paste+tui1dESC+"[201~")
	r.write(t, "回车", "\r")
	return r
}

// sayEscape 写「出路按键」：Ctrl-Home、四个字、LF 字节解成 Ctrl-J、CR 解成回车（契约 §2.4）。
func (r *mlxRunRig) sayEscape(t *testing.T) {
	t.Helper()
	r.write(t, "出路按键", tui1dESC+"[1;5H"+"/say"+"\n"+"\r")
}

// checkHistory 断言会话里恰好 2 条消息、seq 2 的正文是 want。
func (r *mlxRunRig) checkHistory(t *testing.T, want string) {
	t.Helper()
	msgs, err := r.g.svc.History(context.Background(), r.g.convID, 0, 100)
	if err != nil {
		t.Fatalf("读 History 失败：%v", err)
	}
	if len(msgs) != 2 {
		texts := make([]string, 0, len(msgs))
		for _, m := range msgs {
			texts = append(texts, m.Text)
		}
		t.Errorf("会话里有 %d 条消息 %q，契约期望恰好 2 条", len(msgs), texts)
		return
	}
	if msgs[1].Seq != 2 || msgs[1].Text != want {
		t.Errorf("第 2 条消息 seq=%d text=%q，契约期望 seq 2 %q", msgs[1].Seq, msgs[1].Text, want)
	}
}

// checkConvs 断言库里恰好 n 个会话，返回它们的标题。
func (r *mlxRunRig) checkConvs(t *testing.T, n int) []string {
	t.Helper()
	convs, err := r.g.svc.Conversations(context.Background())
	if err != nil {
		t.Fatalf("读 Conversations 失败：%v", err)
	}
	titles := make([]string, 0, len(convs))
	for _, c := range convs {
		titles = append(titles, c.Title)
	}
	if len(convs) != n {
		t.Errorf("库里有 %d 个会话（标题 %q），契约期望恰好 %d 个", len(convs), titles, n)
	}
	return titles
}

// TestMlxRunPipeBlocksLaterCommand：真 Run 上挡下、走出路（契约 §2.4 R1），四个子测试各一套装置。
//
// 【判别力】走的是真输入解码：括号粘贴 → PasteMsg → sanitizer 把 CR 变换行 → 回车 → 门。
// 门被删（Y1）或措辞改了（Y10）→ reject_* 等不到 HF（主断言）。reject_new 的「会话仍是 1 个」是缺席断言，
// 对照是 control_new（只差一个 /，同一装置里 /new 真的建出了第 2 个会话）；control_* 的「不含 HF」是缺席断言，
// 对照是同表的 reject_*。reject_* 发出去的正文与粘贴内容（换行归一之后）逐字节相同，证明挡下时一个字节没丢。
func TestMlxRunPipeBlocksLaterCommand(t *testing.T) {
	t.Run("reject_cr", func(t *testing.T) {
		r := mlxStartRun(t, "一句话\r/round rba_a rba_b --go")
		if !r.flatHas(5*time.Second, mlxHF) {
			t.Fatalf("主断言：回车之后 5 秒内表头没有出现提示 %q；内容 %q", mlxHF, rbaFlat(r.buf.String()))
		}
		r.sayEscape(t)
		if !r.flatHas(5*time.Second, "已记下seq2") {
			t.Fatalf("前提不成立：出路按键之后 5 秒内没有看到「已记下 seq 2」；内容 %q", rbaFlat(r.buf.String()))
		}
		time.Sleep(200 * time.Millisecond)
		r.checkHistory(t, "一句话\n/round rba_a rba_b --go")
		r.checkConvs(t, 1)
	})

	t.Run("control_cr", func(t *testing.T) {
		r := mlxStartRun(t, "一句话\rround rba_a rba_b --go")
		if !r.flatHas(5*time.Second, "已记下seq2") {
			t.Fatalf("前提不成立：回车之后 5 秒内没有看到「已记下 seq 2」；内容 %q", rbaFlat(r.buf.String()))
		}
		time.Sleep(200 * time.Millisecond)
		r.checkHistory(t, "一句话\nround rba_a rba_b --go")
		r.checkConvs(t, 1)
		if strings.Contains(rbaFlat(r.buf.String()), mlxHF) {
			t.Errorf("对照：输出里出现了提示 %q，契约期望不出现", mlxHF)
		}
	})

	t.Run("reject_new", func(t *testing.T) {
		r := mlxStartRun(t, "/new 真机复验\n/add rba_a")
		if !r.flatHas(5*time.Second, mlxHF) {
			t.Fatalf("主断言：回车之后 5 秒内表头没有出现提示 %q；内容 %q", mlxHF, rbaFlat(r.buf.String()))
		}
		r.sayEscape(t)
		if !r.flatHas(5*time.Second, "已记下seq2") {
			t.Fatalf("前提不成立：出路按键之后 5 秒内没有看到「已记下 seq 2」；内容 %q", rbaFlat(r.buf.String()))
		}
		time.Sleep(200 * time.Millisecond)
		r.checkHistory(t, "/new 真机复验\n/add rba_a")
		r.checkConvs(t, 1)
	})

	t.Run("control_new", func(t *testing.T) {
		r := mlxStartRun(t, "/new 真机复验\nadd rba_a")
		if !r.flatHas(5*time.Second, "已建会话") {
			t.Fatalf("前提不成立：回车之后 5 秒内没有看到「已建会话」；内容 %q", rbaFlat(r.buf.String()))
		}
		time.Sleep(200 * time.Millisecond)
		titles := r.checkConvs(t, 2)
		found := false
		for _, s := range titles {
			if s == "真机复验\nadd rba_a" {
				found = true
			}
		}
		if !found {
			t.Errorf("没有标题为 %q 的会话；实有标题 %q", "真机复验\nadd rba_a", titles)
		}
		msgs, err := r.g.svc.History(context.Background(), r.g.convID, 0, 100)
		if err != nil {
			t.Fatalf("读 History 失败：%v", err)
		}
		if len(msgs) != 1 {
			t.Errorf("原会话里有 %d 条消息，契约期望只有种子 1 条", len(msgs))
		}
		if strings.Contains(rbaFlat(r.buf.String()), mlxHF) {
			t.Errorf("对照：输出里出现了提示 %q，契约期望不出现", mlxHF)
		}
	})
}
