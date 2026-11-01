package task

import (
	"context"
	"sync"
)

// 测速内核以包级变量传参，取消信号沿用同一风格。
// 延迟与下载测速的派发循环会检查它，让用户点「停止」后能立即收手，
// 而不必等几千个 IP 全部探测完。
var (
	runCtxMu sync.RWMutex
	runCtx   context.Context
)

// SetContext 设置本次测速的取消信号，传 nil 表示不可取消
func SetContext(ctx context.Context) {
	runCtxMu.Lock()
	runCtx = ctx
	runCtxMu.Unlock()
}

// Context 返回本次测速的上下文；未设置时返回不可取消的后台上下文
func Context() context.Context {
	runCtxMu.RLock()
	ctx := runCtx
	runCtxMu.RUnlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// Canceled 报告本次测速是否已被取消
func Canceled() bool {
	return Context().Err() != nil
}

func canceled() bool { return Canceled() }
