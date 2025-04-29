package worker

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNormal(t *testing.T) {
	ctx := context.Background()
	p := New(2)
	defer p.Close()
	m := make(map[int]int)
	for i := range 4 {
		m[i] = i
	}
	var wg sync.WaitGroup
	wg.Add(1)
	_ = p.Go(ctx, func(context.Context) {
		m[1]++
		wg.Done()
	})
	wg.Add(1)
	_ = p.Go(ctx, func(context.Context) {
		m[2]++
		wg.Done()
	})
	wg.Wait()
	t.Log(m)
}

func sleep1s(context.Context) {
	time.Sleep(time.Second)
}

func TestLimit(t *testing.T) {
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
	p := New(2)
	defer p.Close()
	now = time.Now()
	for range 4 {
		wg.Add(1)
		_ = p.Go(ctx, func(ctx context.Context) {
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

func TestRecover(t *testing.T) {
	ch := make(chan struct{})
	defer close(ch)
	p := New(2, WithPanicHandler(func(ctx context.Context, err interface{}, stack []byte) {
		t.Log("[error] job panic:", err)
		t.Log("[stack]", string(stack))
		ch <- struct{}{}
	}))
	defer p.Close()
	_ = p.Go(context.Background(), func(ctx context.Context) {
		sleep1s(ctx)
		panic("oh my god!")
	})
	<-ch
}

func TestBlockTimeout(t *testing.T) {
	p := New(1, WithCacheSize(1), WithBlockTimeout(time.Second))
	defer p.Close()

	// 正常执行
	err := p.Go(context.Background(), func(ctx context.Context) {
		time.Sleep(2 * time.Second)
	})
	assert.Nil(t, err)

	// 等待队列
	err = p.Go(context.Background(), func(ctx context.Context) {
		time.Sleep(2 * time.Second)
	})
	assert.Nil(t, err)

	// 入缓存链表
	err = p.Go(context.Background(), func(ctx context.Context) {
		time.Sleep(2 * time.Second)
	})
	assert.Nil(t, err)

	// 阻塞超时
	err = p.Go(context.Background(), func(ctx context.Context) {
		time.Sleep(2 * time.Second)
	})
	assert.ErrorIs(t, ErrBlockTimeout, err)
}

func TestPoolClose(t *testing.T) {
	for range 100 {
		p := New(1)
		_ = p.Go(context.Background(), func(ctx context.Context) {
			t.Log("hello")
		})
		_ = p.Go(context.Background(), func(ctx context.Context) {
			t.Log("foo")
		})
		_ = p.Go(context.Background(), func(ctx context.Context) {
			t.Log("bar")
		})
		p.Close() // 关闭pool
		_ = p.Go(context.Background(), func(ctx context.Context) {
			t.Log("closed")
		})
		time.Sleep(100 * time.Millisecond)
	}
}
