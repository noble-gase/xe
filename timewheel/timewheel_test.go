package timewheel

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestTimeWheel 测试时间轮
func TestTimeWheel(t *testing.T) {
	ctx := context.Background()

	ch := make(chan string)
	defer close(ch)

	tw := New()
	defer tw.Stop()

	addedAt := time.Now()

	fmt.Println("===========", "[now]", addedAt.Format(time.DateTime), "===========")

	// 立即执行
	tw.Go(ctx, func(ctx context.Context, task *Task) time.Duration {
		ch <- fmt.Sprintf("task-%d [%d] run at %s, duration %s", task.ID(), task.Attempts(), time.Now().Format(time.DateTime), time.Since(addedAt).String())
		return 0
	}, time.Now())

	// 精度 < 1s，延迟到 1s 执行
	tw.Go(ctx, func(ctx context.Context, task *Task) time.Duration {
		ch <- fmt.Sprintf("task-%d [%d] run at %s, duration %s", task.ID(), task.Attempts(), time.Now().Format(time.DateTime), time.Since(addedAt).String())
		return 0
	}, time.Now().Add(200*time.Millisecond))

	tw.Go(ctx, func(ctx context.Context, task *Task) time.Duration {
		ch <- fmt.Sprintf("task-%d [%d] run at %s, duration %s", task.ID(), task.Attempts(), time.Now().Format(time.DateTime), time.Since(addedAt).String())
		if task.Attempts() >= 9 {
			return 0
		}
		if (task.Attempts()+1)%2 == 0 {
			return time.Second * 2
		}
		return time.Second
	}, time.Now().Add(time.Second))

	for range 11 {
		fmt.Println(<-ch)
	}
}

func TestTaskCancel(t *testing.T) {
	ctx := context.Background()

	ch := make(chan string)
	defer close(ch)

	tw := New(WithCancelFn(func(ctx context.Context, task *Task) {
		ch <- fmt.Sprintf("task-%d canceled", task.ID())
	}))
	defer tw.Stop()

	addedAt := time.Now()

	fmt.Println("=======", "[now]", addedAt.Format(time.DateTime), "=======")

	task := tw.Go(ctx, func(ctx context.Context, task *Task) time.Duration {
		ch <- fmt.Sprintf("task-%d done", task.ID())
		return 0
	}, time.Now().Add(2*time.Second))
	task.Cancel()

	_ = tw.Go(ctx, func(ctx context.Context, task *Task) time.Duration {
		ch <- fmt.Sprintf("task-%d run at %s, duration %s", task.ID(), time.Now().Format(time.DateTime), time.Since(addedAt).String())
		return 0
	}, time.Now().Add(6*time.Second))

	_ = tw.Go(ctx, func(ctx context.Context, task *Task) time.Duration {
		ch <- fmt.Sprintf("task-%d run at %s, duration %s", task.ID(), time.Now().Format(time.DateTime), time.Since(addedAt).String())
		return 0
	}, time.Now().Add(7*time.Second))

	_ = tw.Go(ctx, func(ctx context.Context, task *Task) time.Duration {
		ch <- fmt.Sprintf("task-%d run at %s, duration %s", task.ID(), time.Now().Format(time.DateTime), time.Since(addedAt).String())
		return 0
	}, time.Now().Add(8*time.Second))

	for range 4 {
		fmt.Println(<-ch)
	}
}

// TestCtxDone 测试任务context done
func TestCtxDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	tw := New(WithCancelFn(func(ctx context.Context, task *Task) {
		fmt.Println("[task]", task.ID())
		fmt.Println("[error]", ctx.Err())
		cancel()
	}))
	defer tw.Stop()

	addedAt := time.Now()

	taskCtx, taskCancel := context.WithTimeout(context.Background(), time.Millisecond*200)
	defer taskCancel()
	tw.Go(taskCtx, func(ctx context.Context, task *Task) time.Duration {
		fmt.Println("task run after", time.Since(addedAt).String())
		return 0
	}, time.Now().Add(time.Second))

	<-ctx.Done()
}

// TestPanic 测试Panic
func TestPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	tw := New(WithPanicFn(func(ctx context.Context, task *Task, err any, stack []byte) {
		fmt.Println("[task]", task.ID())
		fmt.Println("[error]", err)
		fmt.Println("[stack]", string(stack))
		cancel()
	}))
	defer tw.Stop()

	addedAt := time.Now()

	tw.Go(ctx, func(ctx context.Context, task *Task) time.Duration {
		fmt.Println("task run after", time.Since(addedAt).String())
		panic("oh no!")
	}, time.Now().Add(time.Second))

	<-ctx.Done()
}
