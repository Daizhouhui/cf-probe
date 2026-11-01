package utils

import (
	"fmt"
	"io"
	"sync"

	"github.com/cheggaaa/pb/v3"
)

// Quiet 为真时不输出进度条，供图形界面等场景静默运行
var Quiet bool

// OnProgress 在进度推进时被调用，供图形界面把进度转成事件推给前端。
// 内核所有进度都经由 Bar.Grow，挂在这里就能覆盖延迟与下载两个阶段。
var OnProgress func(current, total int)

// OnProgressDetail 在需要附带额外计数时报告进度。
var OnProgressDetail func(current, total, matched int)

type Bar struct {
	pb    *pb.ProgressBar
	total int
	cur   int
	mu    sync.Mutex
}

func NewBar(count int, MyStrStart, MyStrEnd string) *Bar {
	tmpl := fmt.Sprintf(`{{counters . }} {{ bar . "[" "-" (cycle . "↖" "↗" "↘" "↙" ) "_" "]"}} %s {{string . "MyStr" | green}} %s `, MyStrStart, MyStrEnd)
	bar := pb.ProgressBarTemplate(tmpl).New(count)
	if Quiet {
		bar.SetWriter(io.Discard)
	}
	bar.Start()
	return &Bar{pb: bar, total: count}
}

func (b *Bar) Grow(num int, MyStrVal string) {
	b.grow(num, MyStrVal, -1)
}

func (b *Bar) GrowWithMatch(num int, MyStrVal string, matched int) {
	b.grow(num, MyStrVal, matched)
}

func (b *Bar) grow(num int, MyStrVal string, matched int) {
	b.pb.Set("MyStr", MyStrVal).Add(num)
	b.mu.Lock()
	b.cur += num
	current := b.cur
	b.mu.Unlock()
	if OnProgress != nil {
		OnProgress(current, b.total)
	}
	if matched >= 0 && OnProgressDetail != nil {
		OnProgressDetail(current, b.total, matched)
	}
}

func (b *Bar) Done() {
	b.pb.Finish()
}
