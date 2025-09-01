package worker

import (
	"container/list"
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type worker struct {
	id        int64
	keepalive atomic.Int64

	ctx    context.Context
	cancel context.CancelFunc
}

type WorkerLRU struct {
	wkMap  map[int64]*list.Element
	wkList *list.List
	size   atomic.Int32
	mutex  sync.Mutex
}

func (lru *WorkerLRU) Upsert(w *worker) {
	lru.mutex.Lock()
	defer lru.mutex.Unlock()

	w.keepalive.Store(time.Now().UnixNano())

	// 存在，移动到头部
	if e, ok := lru.wkMap[w.id]; ok {
		lru.wkList.MoveToFront(e)
		return
	}

	lru.wkMap[w.id] = lru.wkList.PushFront(w)
	lru.size.Store(int32(lru.wkList.Len()))
}

func (lru *WorkerLRU) IdleCheck(timeout time.Duration) {
	lru.mutex.Lock()
	defer lru.mutex.Unlock()

	now := time.Now().UnixNano()
	for e := lru.wkList.Back(); e != nil; e = e.Prev() {
		w := e.Value.(*worker)

		// 未超时，直接结束
		if now-w.keepalive.Load() <= timeout.Nanoseconds() {
			return
		}

		// 超时，移除并关闭协程
		lru.wkList.Remove(e)
		delete(lru.wkMap, w.id)
		w.cancel()
	}
	lru.size.Store(int32(lru.wkList.Len()))
}

func (lru *WorkerLRU) Size() int {
	return int(lru.size.Load())
}

func NewWorkerLRU(initCap int) *WorkerLRU {
	return &WorkerLRU{
		wkMap:  make(map[int64]*list.Element, initCap),
		wkList: list.New(),
	}
}
