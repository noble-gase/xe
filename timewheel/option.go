package timewheel

import "github.com/noble-gase/xe/worker"

// Option 时间轮选项
type Option func(tw *timewheel)

func WithCtxDoneFn(fn CtxDoneFn) Option {
	return func(tw *timewheel) {
		tw.ctxDoneFn = fn
	}
}

// WithPanicFn 指定任务执行Panic的处理方法
func WithPanicFn(fn PanicFn) Option {
	return func(tw *timewheel) {
		tw.panicFn = fn
	}
}

func WithWorkerPool(pool worker.Pool) Option {
	return func(tw *timewheel) {
		tw.pool = pool
	}
}
