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

	mutex sync.RWMutex
}

func (lru *WorkerLRU) Upsert(w *worker) {
	lru.mutex.Lock()
	defer lru.mutex.Unlock()

	w.keepalive.Store(time.Now().UnixNano())

	if e, ok := lru.wkMap[w.id]; ok {
		lru.wkList.MoveToFront(e)
		return
	}
	lru.wkMap[w.id] = lru.wkList.PushFront(w)
}

func (lru *WorkerLRU) IdleCheck(timeout time.Duration) {
	lru.mutex.Lock()
	defer lru.mutex.Unlock()

	for e := lru.wkList.Back(); e != nil; e = e.Prev() {
		w := e.Value.(*worker)
		// 未超时，直接结束
		if time.Now().UnixNano()-w.keepalive.Load() <= timeout.Nanoseconds() {
			return
		}
		// 超时，移除并关闭协程
		lru.wkList.Remove(e)
		delete(lru.wkMap, w.id)
		w.cancel()
	}
}

func (lru *WorkerLRU) Size() int {
	lru.mutex.RLock()
	defer lru.mutex.RUnlock()

	return lru.wkList.Len()
}

func NewWorkerLRU(initCap int) *WorkerLRU {
	return &WorkerLRU{
		wkMap:  make(map[int64]*list.Element, initCap),
		wkList: list.New(),
	}
}
