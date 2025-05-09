package worker

import "time"

// 协程池选项
type Option func(*pool)

// WithPrefill 预填充协程数量
func WithPrefill(n int) Option {
	return func(p *pool) {
		if n > 0 {
			p.prefill = n
		}
	}
}

// WithCacheSize 任务缓存容量，默认：0=不缓存
func WithCacheSize(n int) Option {
	return func(p *pool) {
		if n > 0 {
			p.cacheSize = n
		}
	}
}

// WithBlockTimeout 任务阻塞超时时长，默认：0=不限制
func WithBlockTimeout(duration time.Duration) Option {
	return func(p *pool) {
		if duration > 0 {
			p.blockTimeout = duration
		}
	}
}

// WithIdleTimeout 协程闲置超时时长，默认：10min
func WithIdleTimeout(duration time.Duration) Option {
	return func(p *pool) {
		if duration > 0 {
			p.idleTimeout = duration
		}
	}
}

// WithPanicHandler 任务Panic处理方法
func WithPanicHandler(fn PanicFn) Option {
	return func(p *pool) {
		p.panicFn = fn
	}
}
