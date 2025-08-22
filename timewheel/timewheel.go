package timewheel

import (
	"context"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/noble-gase/xe/worker"
)

type (
	// TaskFn 任务方法，返回下一次执行的延迟时间；若返回0，则表示不再执行
	TaskFn func(ctx context.Context, task *Task) time.Duration

	// CtxDoneFn 任务 context「取消/超时」的处理方法
	CtxDoneFn func(ctx context.Context, task *Task)

	// PanicFn 任务发生Panic的处理方法
	PanicFn func(ctx context.Context, task *Task, err any, stack []byte)
)

// TimeWheel 单层时间轮
type TimeWheel interface {
	// Go 异步一个任务并返回任务ID；
	// 注意：任务是异步执行的，`ctx`一旦被取消/超时，则任务也随之取消；
	// 如要保证任务不被取消，请使用`context.WithoutCancel`
	Go(ctx context.Context, taskFn TaskFn, delay time.Duration) *Task

	// Stop 终止时间轮
	Stop()
}

type timewheel struct {
	slot int
	size int
	tick time.Duration

	uniqID  atomic.Int64
	buckets []*Bucket

	ctxDoneFn CtxDoneFn // Ctx Done 处理函数
	panicFn   PanicFn   // Panic处理函数

	pool worker.Pool

	ctx    context.Context
	cancel context.CancelFunc
}

func (tw *timewheel) Go(ctx context.Context, taskFn TaskFn, delay time.Duration) *Task {
	t := &Task{
		id:       tw.uniqID.Add(1),
		callback: taskFn,
	}
	t.ctx, t.cancel = context.WithCancel(ctx)

	tw.requeue(t, delay)

	return t
}

func (tw *timewheel) Stop() {
	select {
	case <-tw.ctx.Done(): // 时间轮已停止
		return
	default:
	}

	tw.cancel()
	tw.pool.Close()
}

func (tw *timewheel) requeue(t *Task, duration time.Duration) {
	select {
	case <-tw.ctx.Done(): // 时间轮已停止
		return
	default:
	}

	t.attempts++

	tick := tw.tick.Nanoseconds()
	nanosec := duration.Nanoseconds()
	// 圈数
	t.round = int(nanosec / (tick * int64(tw.size)))
	// 槽位
	slot := (int(nanosec/tick)%tw.size + tw.slot) % tw.size
	if slot == tw.slot {
		if t.round == 0 {
			t.delay = duration
			tw.do(t)
			return
		}
		t.round--
	}
	t.slot = slot
	// 剩余延迟
	t.delay = time.Duration(nanosec % tick)
	// 存储任务
	tw.buckets[slot].Add(t)
}

func (tw *timewheel) scheduler() {
	ticker := time.NewTicker(tw.tick)
	defer ticker.Stop()

	for {
		select {
		case <-tw.ctx.Done(): // 时间轮已停止
			return
		case <-ticker.C:
			tw.slot = (tw.slot + 1) % tw.size
			tw.process(tw.slot)
		}
	}
}

func (tw *timewheel) process(slot int) {
	taskList := tw.buckets[slot].Reset()

	go func() {
		defer func() {
			taskList.Init()
			listPool.Put(taskList)
		}()

		for e := taskList.Front(); e != nil; e = e.Next() {
			t := e.Value.(*Task)
			if t.round <= 0 {
				tw.do(t)
			} else {
				t.round--
				tw.buckets[slot].Add(t)
			}
		}
	}()
}

func (tw *timewheel) do(t *Task) {
	select {
	case <-tw.ctx.Done(): // 时间轮停止
		return
	case <-t.ctx.Done(): // 任务被取消
		if tw.ctxDoneFn != nil {
			tw.ctxDoneFn(t.ctx, t)
		}
		return
	default:
	}

	_ = tw.pool.Go(t.ctx, func(ctx context.Context) {
		select {
		case <-tw.ctx.Done(): // 时间轮停止
			return
		case <-ctx.Done(): // 任务被取消
			if tw.ctxDoneFn != nil {
				tw.ctxDoneFn(ctx, t)
			}
			return
		default:
		}

		if t.delay > 0 {
			time.Sleep(t.delay)

			select {
			case <-tw.ctx.Done(): // 时间轮停止
				return
			case <-ctx.Done(): // 任务被取消
				if tw.ctxDoneFn != nil {
					tw.ctxDoneFn(ctx, t)
				}
				return
			default:
			}
		}

		if tw.panicFn != nil {
			defer func() {
				if err := recover(); err != nil {
					tw.panicFn(t.ctx, t, err, debug.Stack())
				}
			}()
		}

		duration := t.callback(ctx, t)
		if duration > 0 {
			tw.requeue(t, duration)
		}
	})
}

// New 返回一个时间轮实例
func New(size int, tick time.Duration, opts ...Option) TimeWheel {
	ctx, cancel := context.WithCancel(context.TODO())
	tw := &timewheel{
		size: size,
		tick: tick,

		buckets: make([]*Bucket, size),

		ctx:    ctx,
		cancel: cancel,
	}
	for _, fn := range opts {
		fn(tw)
	}
	for i := range size {
		tw.buckets[i] = NewBucket()
	}
	if tw.pool == nil {
		tw.pool = worker.New(2000, worker.WithCacheSize(100))
	}

	go tw.scheduler()

	return tw
}
