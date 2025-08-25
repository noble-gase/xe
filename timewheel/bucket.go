package timewheel

import (
	"container/list"
	"context"
	"sync"
	"time"
)

var listPool = sync.Pool{
	New: func() any {
		return list.New()
	},
}

type Task struct {
	id int64 // 任务ID

	callback TaskFn // 任务执行函数
	attempts int    // 当前任务执行的次数

	slot int // 时间轮槽位

	execTime  time.Time // 任务执行时间
	execDelay time.Duration

	ctx    context.Context
	cancel context.CancelFunc
}

func (t *Task) ID() int64 {
	return t.id
}

func (t *Task) Attempts() int {
	return t.attempts
}

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
		l := listPool.Get().(*list.List)
		l.Init()
		b.list = l
	}
	b.list.PushBack(t)
}

func (b *Bucket) Reset() *list.List {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	old := b.list

	new := listPool.Get().(*list.List)
	new.Init()
	b.list = new

	return old
}

func NewBucket() *Bucket {
	return &Bucket{
		list: list.New(),
	}
}
