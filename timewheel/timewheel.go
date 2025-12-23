package timewheel

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/noble-gase/xe/worker"
)

type (
	// TaskFn 任务方法，返回下一次执行的延迟时间 (<=0 表示不再执行)
	TaskFn func(ctx context.Context, task *Task) time.Duration

	// CancelFn 任务 context「超时｜取消」的处理方法
	CancelFn func(ctx context.Context, task *Task)
)

// TimeWheel 时间轮
type TimeWheel interface {
	// Go 任务入时间轮
	//
	// 注意：任务是异步执行的，若 context 超时｜取消，则任务也随之取消
	//
	// 通常需要 context.WithoutCancel(ctx)
	Go(ctx context.Context, taskId string, taskFn TaskFn, execTime time.Time) *Task

	// Stop 终止时间轮
	Stop()
}

// TimeLevel 层级
type TimeLevel struct {
	size  int
	prec  time.Duration
	round time.Duration // 一圈时长

	slot atomic.Int32 // 当前槽位

	isMin bool // 是否是最小精度

	buckets []*Bucket
}

// Level 返回一个层级
func Level(size int, prec time.Duration) *TimeLevel {
	return &TimeLevel{
		size:    size,
		prec:    prec,
		round:   prec * time.Duration(size),
		buckets: make([]*Bucket, size),
	}
}

// Hour 24小时
func Hour() *TimeLevel {
	return Level(24, time.Hour)
}

// Minute 60分钟
func Minute() *TimeLevel {
	return Level(60, time.Minute)
}

// Second 60秒
func Second() *TimeLevel {
	return Level(60, time.Second)
}

type timewheel struct {
	levels []*TimeLevel

	pool worker.Pool

	cancelFn CancelFn

	ctx    context.Context
	cancel context.CancelFunc
}

func (tw *timewheel) Go(ctx context.Context, taskId string, taskFn TaskFn, execTime time.Time) *Task {
	ctx, cancel := context.WithCancelCause(ctx)

	task := &Task{
		id: taskId,

		execFunc: taskFn,
		execTime: execTime,

		ctx:    ctx,
		cancel: cancel,
	}

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
	case <-task.ctx.Done(): // 任务被取消
		if tw.cancelFn != nil {
			tw.cancelFn(task.ctx, task)
		}
		return
	default:
	}

	delay := time.Until(task.execTime)
	if delay <= 0 {
		tw.do(task)
		return
	}

	var tl *TimeLevel
	for _, tl = range tw.levels {
		if delay > tl.prec {
			break
		}
	}

	var mod int
	if tl.isMin {
		mod = int((delay + tl.prec - 1) / tl.prec)
	} else {
		mod = int(delay / tl.prec)
	}
	slot := (mod%tl.size + int(tl.slot.Load())) % tl.size

	tl.buckets[slot].Add(task)
}

func (tw *timewheel) scheduler() {
	for _, v := range tw.levels {
		go func(tl *TimeLevel) {
			ticker := time.NewTicker(tl.prec)
			defer ticker.Stop()

			for {
				select {
				case <-tw.ctx.Done(): // 时间轮已停止
					return
				case <-ticker.C:
					slot := (int(tl.slot.Load()) + 1) % tl.size
					tl.slot.Store(int32(slot))
					tw.process(tl, slot)
				}
			}
		}(v)
	}
}

func (tw *timewheel) process(tl *TimeLevel, slot int) {
	taskList := tl.buckets[slot].Reset()

	go func() {
		for e := taskList.Front(); e != nil; e = e.Next() {
			task := e.Value.(*Task)
			if delay := time.Until(task.execTime); delay < tl.round {
				if tl.isMin {
					tw.do(task)
				} else {
					tw.requeue(task) // 放入更小精度的时间轮
				}
			} else {
				tl.buckets[slot].Add(task) // 放回原槽位，等待下一轮
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
		if d := time.Until(task.execTime); d > 0 {
			time.Sleep(d)
		}

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

		task.attempts.Add(1)

		if d := task.execFunc(ctx, task); d > 0 {
			task.execTime = task.execTime.Add(d)
			tw.requeue(task)
		}
	})
}

// New 返回一个时间轮
func New(opts ...Option) TimeWheel {
	ctx, cancel := context.WithCancel(context.TODO())

	tw := &timewheel{
		ctx:    ctx,
		cancel: cancel,
	}
	for _, fn := range opts {
		fn(tw)
	}

	if tw.pool == nil {
		tw.pool = worker.New(10000, worker.WithCacheSize(1000))
	}

	if tw.cancelFn == nil {
		tw.cancelFn = func(ctx context.Context, task *Task) {
			err := context.Cause(ctx)
			slog.LogAttrs(ctx, slog.LevelWarn, "task canceled", slog.String("task_id", task.ID()), slog.String("error", err.Error()))
		}
	}

	// 层级
	if len(tw.levels) == 0 {
		tw.levels = []*TimeLevel{Hour(), Minute(), Second()}
	}
	sort.SliceStable(tw.levels, func(i, j int) bool {
		return tw.levels[i].prec > tw.levels[j].prec
	})
	tw.levels[len(tw.levels)-1].isMin = true

	// 初始化槽位
	for _, v := range tw.levels {
		for i := range v.size {
			v.buckets[i] = NewBucket()
		}
	}

	go tw.scheduler()

	return tw
}

var (
	tw   TimeWheel
	once sync.Once
)

// Init 初始化默认的全局时间轮
func Init(opts ...Option) {
	tw = New(opts...)
}

// Go 使用默认的全局时间轮
func Go(ctx context.Context, taskId string, taskFn TaskFn, execTime time.Time) *Task {
	if tw == nil {
		once.Do(func() {
			tw = New()
		})
	}
	return tw.Go(ctx, taskId, taskFn, execTime)
}

// Close 关闭默认的全局时间轮
func Stop() {
	if tw != nil {
		tw.Stop()
	}
}
