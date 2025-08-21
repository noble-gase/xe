package worker

import (
	"context"
	"errors"
	"runtime/debug"
	"sync"
	"time"

	"github.com/noble-gase/xe/internal/linklist"
)

const (
	defaultPoolCap     = 10000
	defaultIdleTimeout = 10 * time.Minute
)

var ErrPoolClosed = errors.New("pool closed")

// Pool 协程并发复用，降低CPU和内存负载
type Pool interface {
	// Go 执行任务，没有闲置协程时入缓存队列，缓存达到上限会阻塞等待
	//
	// 通常需要 context.WithoutCancel(ctx)
	Go(ctx context.Context, fn func(ctx context.Context)) error

	// Close 关闭资源
	Close()
}

// PanicFn 处理Panic方法
type PanicFn func(ctx context.Context, err any, stack []byte)

type worker struct {
	keepalive time.Time

	ctx    context.Context
	cancel context.CancelFunc
}

type task struct {
	ctx context.Context
	fn  func(ctx context.Context)
}

type pool struct {
	input chan *task
	queue chan *task

	cache     chan *task
	cacheSize int

	capacity int
	prefill  int
	workers  *linklist.DoublyLinkList[*worker]

	idleTimeout time.Duration

	panicFn PanicFn

	ctx    context.Context
	cancel context.CancelFunc
}

// New 生成一个新的Pool
func New(cap int, opts ...Option) Pool {
	if cap <= 0 {
		cap = defaultPoolCap
	}

	ctx, cancel := context.WithCancel(context.TODO())

	p := &pool{
		input: make(chan *task),

		capacity: cap,
		workers:  linklist.New[*worker](),

		idleTimeout: defaultIdleTimeout,

		ctx:    ctx,
		cancel: cancel,
	}

	for _, fn := range opts {
		fn(p)
	}
	p.queue = make(chan *task)
	p.cache = make(chan *task, p.cacheSize)

	// 预填充
	if p.prefill > 0 {
		count := min(p.prefill, p.capacity)
		for range count {
			p.spawn()
		}
	}

	go p.run()
	go p.idle()

	return p
}

func (p *pool) Go(ctx context.Context, fn func(ctx context.Context)) error {
	select {
	case <-p.ctx.Done(): // Pool关闭
		return ErrPoolClosed
	case <-ctx.Done():
		return ctx.Err()
	case p.input <- &task{ctx: ctx, fn: fn}:
		return nil
	}
}

func (p *pool) Close() {
	select {
	case <-p.ctx.Done(): // Pool已关闭
		return
	default:
	}

	// 销毁协程
	p.cancel()

	// 处理剩余的任务
	for v := range p.cache {
		if v != nil {
			p.do(v)
		}
	}
}

func (p *pool) run() {
	for {
		select {
		case <-p.ctx.Done(): // Pool关闭
			close(p.cache)
			return
		case v, ok := <-p.input:
			if !ok || v == nil {
				break
			}

			select {
			case p.queue <- v:
			default:
				// 未达上限，新开一个协程
				if p.workers.Size() < p.capacity {
					p.spawn()
				}

				// 等待闲置协程
				select {
				case p.queue <- v:
				case p.cache <- v:
				}
			}
		}
	}
}

func (p *pool) idle() {
	ticker := time.NewTicker(p.idleTimeout / 10)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done(): // Pool关闭
			return
		case <-ticker.C:
			idles := p.workers.Filter(func(index int, value *worker) bool {
				return time.Since(value.keepalive) > p.idleTimeout
			})
			for _, wk := range idles {
				wk.cancel()
			}
		}
	}
}

func (p *pool) spawn() {
	wk := &worker{
		keepalive: time.Now(),
	}
	wk.ctx, wk.cancel = context.WithCancel(context.TODO())
	// 存储协程信息
	p.workers.Append(wk)

	go func() {
		for {
			// 获取任务
			select {
			case <-p.ctx.Done(): // Pool关闭，销毁
				return
			case <-wk.ctx.Done(): // 闲置超时，销毁
				return
			case v, ok := <-p.queue: // 从队列获取任务
				if ok && v != nil {
					p.do(v)
				}
			case v, ok := <-p.cache: // 从缓存获取任务
				if ok && v != nil {
					p.do(v)
				}
			}
			wk.keepalive = time.Now()
		}
	}()
}

func (p *pool) do(task *task) {
	if task == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			if p.panicFn != nil {
				p.panicFn(task.ctx, r, debug.Stack())
			}
		}
	}()
	task.fn(task.ctx)
}

var (
	pp   Pool
	once sync.Once
)

// Init 初始化默认的全局Pool
func Init(cap int, opts ...Option) {
	pp = New(cap, opts...)
}

// Go 使用默认的全局Pool
func Go(ctx context.Context, fn func(ctx context.Context)) error {
	if pp == nil {
		once.Do(func() {
			pp = New(defaultPoolCap)
		})
	}
	return pp.Go(ctx, fn)
}

// Close 关闭默认的全局Pool
func Close() {
	if pp != nil {
		pp.Close()
	}
}
