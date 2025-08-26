package timewheel

import (
	"github.com/noble-gase/xe/worker"
)

// Option 时间轮选项
type Option func(tw *timewheel)

// WithCancelFn 指定任务 context「取消/超时」的处理方法
func WithCancelFn(fn CancelFn) Option {
	return func(tw *timewheel) {
		tw.cancelFn = fn
	}
}

// WithPanicFn 指定任务执行Panic的处理方法
func WithPanicFn(fn PanicFn) Option {
	return func(tw *timewheel) {
		tw.panicFn = fn
	}
}

// WithWorkerPool 指定协程池；默认：worker.New(2000, worker.WithCacheSize(100))
func WithWorkerPool(pool worker.Pool) Option {
	return func(tw *timewheel) {
		tw.pool = pool
	}
}

// WithTimeLevel 指定时间轮层级；默认：3层，每层精度依次为：小时、分钟、秒
func WithTimeLevel(levels ...*TimeLevel) Option {
	return func(tw *timewheel) {
		tw.levels = append(tw.levels, levels...)
	}
}
