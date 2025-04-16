package worker

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"
)

func TestSyncNormal(t *testing.T) {
	ctx := context.Background()
	p := NewPool(2)
	defer p.Close()
	m := make(map[int]int)
	for i := range 4 {
		m[i] = i
	}
	var wg sync.WaitGroup
	wg.Add(1)
	p.Sync(ctx, func(context.Context) {
		m[1]++
		wg.Done()
	})
	wg.Add(1)
	p.Sync(ctx, func(context.Context) {
		m[2]++
		wg.Done()
	})
	wg.Wait()
	t.Log(m)
}

func sleep1s(context.Context) {
	time.Sleep(time.Second)
}

func TestSyncLimit(t *testing.T) {
	ctx := context.Background()
	var wg sync.WaitGroup
	// 没有并发数限制
	now := time.Now()
	for range 4 {
		wg.Add(1)
		go func() {
			sleep1s(ctx)
			wg.Done()
		}()
	}
	wg.Wait()
	sec := math.Round(time.Since(now).Seconds())
	if sec != 1 {
		t.FailNow()
	}
	// 限制并发数
	p := NewPool(2)
	defer p.Close()
	now = time.Now()
	for range 4 {
		wg.Add(1)
		p.Sync(ctx, func(ctx context.Context) {
			sleep1s(ctx)
			wg.Done()
		})
	}
	wg.Wait()
	sec = math.Round(time.Since(now).Seconds())
	if sec != 2 {
		t.FailNow()
	}
}

func TestSyncRecover(t *testing.T) {
	ch := make(chan struct{})
	defer close(ch)
	p := NewPool(2, WithPanicHandler(func(ctx context.Context, err interface{}, stack []byte) {
		t.Log("[error] job panic:", err)
		t.Log("[stack]", string(stack))
		ch <- struct{}{}
	}))
	defer p.Close()
	p.Sync(context.Background(), func(ctx context.Context) {
		sleep1s(ctx)
		panic("oh my god!")
	})
	<-ch
}

func TestAsyncNormal(t *testing.T) {
	ctx := context.Background()
	p := NewPool(2)
	defer p.Close()
	m := make(map[int]int)
	for i := range 4 {
		m[i] = i
	}
	var wg sync.WaitGroup
	wg.Add(1)
	p.Async(ctx, func(context.Context) {
		m[1]++
		wg.Done()
	})
	wg.Add(1)
	p.Async(ctx, func(context.Context) {
		m[2]++
		wg.Done()
	})
	wg.Wait()
	t.Log(m)
}

func TestAsyncLimit(t *testing.T) {
	ctx := context.Background()
	var wg sync.WaitGroup
	// 没有并发数限制
	now := time.Now()
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			sleep1s(ctx)
			wg.Done()
		}()
	}
	wg.Wait()
	sec := math.Round(time.Since(now).Seconds())
	if sec != 1 {
		t.FailNow()
	}
	// 限制并发数
	p := NewPool(2)
	defer p.Close()
	now = time.Now()
	for range 4 {
		wg.Add(1)
		p.Async(ctx, func(ctx context.Context) {
			sleep1s(ctx)
			wg.Done()
		})
	}
	wg.Wait()
	sec = math.Round(time.Since(now).Seconds())
	if sec != 2 {
		t.FailNow()
	}
}

func TestAsyncRecover(t *testing.T) {
	ch := make(chan struct{})
	defer close(ch)
	p := NewPool(2, WithPanicHandler(func(ctx context.Context, err interface{}, stack []byte) {
		t.Log("[error] panic recovered:", err)
		t.Log("[stack]", string(stack))
		ch <- struct{}{}
	}))
	defer p.Close()
	p.Async(context.Background(), func(ctx context.Context) {
		sleep1s(ctx)
		panic("oh my god!")
	})
	<-ch
}

func TestPoolClose(t *testing.T) {
	for range 100 {
		p := NewPool(1)
		p.Sync(context.Background(), func(ctx context.Context) {
			t.Log("hello")
		})
		p.Sync(context.Background(), func(ctx context.Context) {
			t.Log("foo")
		})
		p.Sync(context.Background(), func(ctx context.Context) {
			t.Log("bar")
		})
		p.Close() // 关闭pool
		p.Sync(context.Background(), func(ctx context.Context) {
			t.Log("closed")
		})
		time.Sleep(100 * time.Millisecond)
	}
}
