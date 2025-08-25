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

	// CancelFn 任务 context「取消/超时」的处理方法
	CancelFn func(ctx context.Context, task *Task)

	// PanicFn 任务发生Panic的处理方法
	PanicFn func(ctx context.Context, task *Task, err any, stack []byte)
)

// TimeWheel 单层「秒级」时间轮
type TimeWheel interface {
	// Go 异步一个任务并返回任务ID；
	// 注意：任务是异步执行的，若 context 取消 / 超时，则任务也随之取消；
	// 如要保证任务不被取消，请使用`context.WithoutCancel`
	Go(ctx context.Context, taskFn TaskFn, execTime time.Time) *Task

	// Stop 终止时间轮
	Stop()
}

type timewheel struct {
	size int
	tick time.Duration

	duration time.Duration // 时间轮时长

	slot int // 当前槽位

	uniqID  atomic.Int64
	buckets []*Bucket

	cancelFn CancelFn // Ctx Done 处理函数
	panicFn  PanicFn  // Panic处理函数

	pool worker.Pool

	ctx    context.Context
	cancel context.CancelFunc
}

func (tw *timewheel) Go(ctx context.Context, taskFn TaskFn, execTime time.Time) *Task {
	task := &Task{
		id:        tw.uniqID.Add(1),
		callback:  taskFn,
		execTime:  execTime,
		execDelay: time.Until(execTime),
	}
	task.ctx, task.cancel = context.WithCancel(ctx)

	// 入时间轮
	tw.requeue(task)

	return task
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

func (tw *timewheel) requeue(task *Task) {
	select {
	case <-tw.ctx.Done(): // 时间轮已停止
		return
	default:
	}

	task.attempts++

	// 槽位 (task.execDelay+tw.tick-1 向上取整技巧，避免浮点数计算)
	slot := (int((task.execDelay+tw.tick-1)/tw.tick)%tw.size + tw.slot) % tw.size
	task.slot = slot

	if slot == tw.slot {
		if task.execDelay < tw.duration {
			tw.do(task)
		}
		return
	}

	// 存储任务
	tw.buckets[slot].Add(task)
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
			task := e.Value.(*Task)
			if time.Until(task.execTime) < tw.duration {
				tw.do(task)
			} else {
				tw.buckets[slot].Add(task)
			}
		}
	}()
}

func (tw *timewheel) do(task *Task) {
	select {
	case <-tw.ctx.Done(): // 时间轮停止
		return
	case <-task.ctx.Done(): // 任务被取消
		if tw.cancelFn != nil {
			tw.cancelFn(task.ctx, task)
		}
		return
	default:
	}

	_ = tw.pool.Go(task.ctx, func(ctx context.Context) {
		select {
		case <-tw.ctx.Done(): // 时间轮停止
			return
		case <-ctx.Done(): // 任务被取消
			if tw.cancelFn != nil {
				tw.cancelFn(ctx, task)
			}
			return
		default:
		}

		if tw.panicFn != nil {
			defer func() {
				if err := recover(); err != nil {
					tw.panicFn(task.ctx, task, err, debug.Stack())
				}
			}()
		}

		duration := task.callback(ctx, task)
		if duration > 0 {
			task.execTime = task.execTime.Add(duration)
			tw.requeue(task)
		}
	})
}

// New 返回一个「秒级」时间轮实例
func New(size int, opts ...Option) TimeWheel {
	ctx, cancel := context.WithCancel(context.TODO())
	tw := &timewheel{
		size: size,
		tick: time.Second,

		duration: time.Second * time.Duration(size),

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
