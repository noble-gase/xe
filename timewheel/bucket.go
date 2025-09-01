package timewheel

import (
	"container/list"
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Task 任务
type Task struct {
	id int64 // 任务ID

	execFunc TaskFn    // 任务执行函数
	execTime time.Time // 任务执行时间

	attempts atomic.Int32 // 当前任务执行的次数

	ctx    context.Context
	cancel context.CancelFunc
}

// ID 返回任务ID
func (t *Task) ID() int64 {
	return t.id
}

// Attempts 返回任务执行的次数
func (t *Task) Attempts() int {
	return int(t.attempts.Load())
}

// Cancel 取消任务
func (t *Task) Cancel() {
	t.cancel()
}

type Bucket struct {
	list  *list.List
	mutex sync.Mutex
}

func (b *Bucket) Add(t *Task) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.list == nil {
		b.list = list.New()
	}
	b.list.PushBack(t)
}

func (b *Bucket) Reset() *list.List {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	old := b.list
	b.list = list.New()
	return old
}

func NewBucket() *Bucket {
	return &Bucket{
		list: list.New(),
	}
}
