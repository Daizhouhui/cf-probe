package app

import (
	"context"
	"sync"
	"time"
)

// Event 是推送给界面的一条运行事件
type Event struct {
	Type       string   `json:"type"` // progress / log / done / error
	Stage      string   `json:"stage,omitempty"`
	Message    string   `json:"message,omitempty"`
	Current    int      `json:"current"`
	Total      int      `json:"total,omitempty"`
	Matched    int      `json:"matched"`
	MatchTotal int      `json:"match_total,omitempty"`
	Results    []Result `json:"results,omitempty"`
	Result     *Result  `json:"result,omitempty"`    // 下载测速逐条出结果时带上
	Cancelled  bool     `json:"cancelled,omitempty"` // 用户主动停止，结果只包含已测出的部分
	Finished   bool     `json:"finished"`
	At         int64    `json:"at"`
}

// Runner 串行调度测速任务，并把过程广播给所有订阅者
type Runner struct {
	mu       sync.RWMutex
	running  bool
	cancel   context.CancelFunc
	history  []Event
	results  []Result
	subs     map[chan Event]struct{}
	lastOpts Options
}

// NewRunner 创建调度器
func NewRunner() *Runner {
	return &Runner{subs: make(map[chan Event]struct{})}
}

// Subscribe 订阅事件流，返回通道与退订函数；已有历史会先回放
func (r *Runner) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	r.mu.Lock()
	// 只在任务进行中回放历史。任务结束后再回放旧的 progress 事件，
	// 会让刚刷新的页面误以为测速还在跑（停止按钮一直转圈）。
	// 已结束的结果由 /api/results 提供，界面不必依赖事件回放。
	if r.running {
		for _, e := range r.history {
			select {
			case ch <- e:
			default:
			}
		}
	}
	r.subs[ch] = struct{}{}
	r.mu.Unlock()
	return ch, func() {
		r.mu.Lock()
		if _, ok := r.subs[ch]; ok {
			delete(r.subs, ch)
			close(ch)
		}
		r.mu.Unlock()
	}
}

func (r *Runner) broadcast(e Event) {
	e.At = time.Now().UnixMilli()
	r.mu.Lock()
	r.history = append(r.history, e)
	if len(r.history) > 200 {
		r.history = r.history[len(r.history)-200:]
	}
	// 逐条结果同时累积到 results，这样中途刷新页面也能
	// 通过 /api/results 拿到已经测出来的部分
	if e.Type == "result" && e.Result != nil {
		r.results = append(r.results, *e.Result)
	}
	for ch := range r.subs {
		select {
		case ch <- e:
		default: // 订阅者太慢就丢事件，不阻塞测速
		}
	}
	r.mu.Unlock()
}

// Running 返回是否有任务在跑
func (r *Runner) Running() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running
}

// Results 返回最近一次测速结果
func (r *Runner) Results() []Result {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Result, len(r.results))
	copy(out, r.results)
	return out
}

// LastStage 返回最近一次进度事件的阶段名
func (r *Runner) LastStage() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := len(r.history) - 1; i >= 0; i-- {
		if r.history[i].Stage != "" {
			return r.history[i].Stage
		}
	}
	return ""
}

// LastOptions 返回最近一次使用的参数
func (r *Runner) LastOptions() Options {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastOpts
}

// Start 启动一次测速；已有任务在跑时返回 false
func (r *Runner) Start(o Options) bool {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.running = true
	r.cancel = cancel
	r.history = nil
	r.results = nil // 上一轮的结果别串到这一轮
	r.lastOpts = o
	r.mu.Unlock()

	go func() {
		defer func() {
			r.mu.Lock()
			r.running = false
			r.cancel = nil
			r.mu.Unlock()
			cancel()
		}()
		var finalProgress Progress
		rs, err := Run(ctx, o, func(p Progress) {
			if p.Stage == "done" {
				finalProgress = p
			}
			if p.Result != nil {
				// 单条结果单独发一种事件，前端可以立刻插进表格
				r.broadcast(Event{Type: "result", Stage: p.Stage, Result: p.Result})
				return
			}
			r.broadcast(Event{
				Type: "progress", Stage: p.Stage, Message: p.Message,
				Current: p.Current, Total: p.Total,
				Matched: p.Matched, MatchTotal: p.MatchTotal,
			})
		})
		if err != nil {
			if ctx.Err() != nil {
				// 主动停止不是失败：把已经测出的部分结果留下来并落盘，
				// 这样界面刷新、重开应用后还能继续下载这份结果
				partial := r.Results()
				if len(partial) > 0 {
					if werr := WriteCSV(ResultFile, partial); werr != nil {
						r.broadcast(Event{Type: "log", Message: "结果写入失败: " + werr.Error()})
					}
				}
				r.broadcast(Event{
					Type: "done", Stage: "done", Message: "已停止，已保留部分结果",
					Results: partial, Cancelled: true, Finished: true,
				})
				return
			}
			r.broadcast(Event{Type: "error", Message: err.Error(), Finished: true})
			return
		}
		r.mu.Lock()
		r.results = rs
		r.mu.Unlock()
		if err := WriteCSV(ResultFile, rs); err != nil {
			r.broadcast(Event{Type: "log", Message: "结果写入失败: " + err.Error()})
		}
		r.broadcast(Event{
			Type: "done", Stage: "done", Message: finalProgress.Message,
			Results: rs, Total: finalProgress.Total, Current: finalProgress.Current,
			Matched: finalProgress.Matched, MatchTotal: finalProgress.MatchTotal, Finished: true,
		})
	}()
	return true
}

// Cancel 取消当前任务
func (r *Runner) Cancel() {
	r.mu.RLock()
	c := r.cancel
	r.mu.RUnlock()
	if c != nil {
		c()
	}
}
