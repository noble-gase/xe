package worker

import (
	"context"
	"runtime/debug"
	"sync"
	"time"

	"github.com/noble-gase/xe/internal/linklist"
)

type Mode int

const (
	Sync  Mode = 1
	Async Mode = 2
)

const (
	defaultPoolCap     = 10000
	defaultIdleTimeout = 60 * time.Second
)

// Pool 协程并发复用，降低CPU和内存负载
type Pool interface {
	// Sync 同步模式：没有闲置协程时会等待
	//
	// 通常需要 context.WithoutCancel(ctx)
	Sync(ctx context.Context, fn func(ctx context.Context))

	// Async 异步模式：没有闲置协程时会放人全局链表缓存
	//
	// 通常需要 context.WithoutCancel(ctx)
	Async(ctx context.Context, fn func(ctx context.Context))

	// Close 关闭资源
	Close()
}

// PanicFn 处理Panic方法
type PanicFn func(ctx context.Context, err any, stack []byte)

type worker struct {
	timeUsed time.Time
	cancel   context.CancelFunc
}

type task struct {
	ctx  context.Context
	fn   func(ctx context.Context)
	mode Mode
}

type pool struct {
	input chan *task
	queue chan *task
	cache *linklist.DoublyLinkList[*task]

	capacity int
	workers  *linklist.DoublyLinkList[*worker]

	prefill     int
	queueCap    int
	idleTimeout time.Duration
	panicFn     PanicFn

	ctx    context.Context
	cancel context.CancelFunc
}

// NewPool 生成一个新的Pool
func NewPool(cap int, opts ...Option) Pool {
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
	p.queue = make(chan *task, p.queueCap)
	// 缓存链表
	p.cache = linklist.New[*task]()
	// 预填充
	if p.prefill > 0 {
		count := p.prefill
		if p.prefill > p.capacity {
			count = p.capacity
		}
		for i := 0; i < count; i++ {
			p.spawn()
		}
	}

	go p.run()
	go p.idle()

	return p
}

func (p *pool) Sync(ctx context.Context, fn func(ctx context.Context)) {
	p.input <- &task{ctx: ctx, fn: fn, mode: Sync}
}

func (p *pool) Async(ctx context.Context, fn func(ctx context.Context)) {
	p.input <- &task{ctx: ctx, fn: fn, mode: Async}
}

func (p *pool) Close() {
	select {
	case <-p.ctx.Done(): // 已关闭
		return
	default:
	}

	// 销毁协程
	p.cancel()

	// 关闭通道
	for {
		select {
		case <-p.input:
		case <-p.queue:
		default:
			close(p.input)
			close(p.queue)
			return
		}
	}
}

func (p *pool) run() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case t := <-p.input:
			select {
			case p.queue <- t:
			default:
				// 未达上限，新开一个协程
				if p.workers.Size() < p.capacity {
					p.spawn()
					p.queue <- t
					break
				}
				// 异步模式，放入本地缓存
				if t.mode == Async {
					p.cache.Append(t)
					break
				}
				// 同步模式，等待闲置协程
				select {
				case <-p.ctx.Done():
					return
				case p.queue <- t:
				}
			}
		}
	}
}

func (p *pool) idle() {
	ticker := time.NewTicker(p.idleTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			idles := p.workers.Filter(func(index int, value *worker) bool {
				return time.Since(value.timeUsed) > p.idleTimeout
			})
			for _, wk := range idles {
				wk.cancel()
			}
		}
	}
}

func (p *pool) spawn() {
	ctx, cancel := context.WithCancel(context.TODO())
	wk := &worker{
		timeUsed: time.Now(),
		cancel:   cancel,
	}
	// 存储协程信息
	p.workers.Append(wk)

	go func(ctx context.Context, wk *worker) {
		var taskCtx context.Context
		defer func() {
			if e := recover(); e != nil {
				if p.panicFn != nil {
					p.panicFn(taskCtx, e, debug.Stack())
				}
			}
		}()
		for {
			var t *task
			// 获取任务
			select {
			case <-p.ctx.Done(): // Pool关闭，销毁
				return
			case <-ctx.Done(): // 闲置超时，销毁
				return
			case t = <-p.queue: // 尝试从队列获取任务
			default:
				// 队列无任务，去取缓存的任务执行
				if v, ok := p.cache.Remove(0); ok && v != nil {
					t = v
					break
				}
				// 缓存未取到任务，则等待新任务
				select {
				case <-p.ctx.Done():
					return
				case <-ctx.Done():
					return
				case t = <-p.queue:
				}
			}
			// 执行任务
			wk.timeUsed = time.Now()
			taskCtx = t.ctx
			t.fn(t.ctx)
		}
	}(ctx, wk)
}

var (
	pp   Pool
	once sync.Once
)

// Init 初始化默认的全局Pool
func Init(cap int, opts ...Option) {
	pp = NewPool(cap, opts...)
}

// P 返回默认的全局Pool
func P() Pool {
	if pp == nil {
		once.Do(func() {
			pp = NewPool(defaultPoolCap)
		})
	}
	return pp
}
