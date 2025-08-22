package timewheel

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"
)

// TestTimeWheel 测试时间轮
func TestTimeWheel(t *testing.T) {
	ctx := context.Background()

	ch := make(chan string)
	defer close(ch)

	tw := New(7, time.Second)
	defer tw.Stop()

	addedAt := time.Now()

	tw.Go(ctx, func(ctx context.Context, task *Task) time.Duration {
		ch <- fmt.Sprintf("task[%d][%d] run after %ds", task.ID(), task.Attempts(), int64(math.Round(time.Since(addedAt).Seconds())))
		if task.Attempts() >= 10 {
			return 0
		}
		if task.Attempts()%2 == 0 {
			return time.Second * 2
		}
		return time.Second
	}, time.Second)

	tw.Go(ctx, func(ctx context.Context, task *Task) time.Duration {
		ch <- fmt.Sprintf("task[%d][%d] run after %ds", task.ID(), task.Attempts(), int64(math.Round(time.Since(addedAt).Seconds())))
		if task.Attempts() >= 5 {
			return 0
		}
		return time.Second * 2
	}, time.Second*2)

	for range 15 {
		t.Log(<-ch)
	}
}

func TestTaskCancel(t *testing.T) {
	ctx := context.Background()

	tw := New(5, time.Second, WithCtxDoneFn(func(ctx context.Context, task *Task) {
		fmt.Println("task", task.ID(), "canceled")
	}))
	defer tw.Stop()

	addedAt := time.Now()

	task := tw.Go(ctx, func(ctx context.Context, task *Task) time.Duration {
		fmt.Println("task", task.ID(), "done")
		return 0
	}, 2*time.Second)
	task.Cancel()

	_ = tw.Go(ctx, func(ctx context.Context, task *Task) time.Duration {
		fmt.Println("task", task.ID(), "done after", time.Since(addedAt).String())
		return 0
	}, 6*time.Second)

	_ = tw.Go(ctx, func(ctx context.Context, task *Task) time.Duration {
		fmt.Println("task", task.ID(), "done after", time.Since(addedAt).String())
		return 0
	}, 7*time.Second)

	_ = tw.Go(ctx, func(ctx context.Context, task *Task) time.Duration {
		fmt.Println("task", task.ID(), "done after", time.Since(addedAt).String())
		return 0
	}, 8*time.Second)

	time.Sleep(10 * time.Second)
}

// TestCtxDone 测试任务context done
func TestCtxDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	tw := New(7, time.Second, WithCtxDoneFn(func(ctx context.Context, task *Task) {
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
	}, time.Second)

	<-ctx.Done()
}

// TestPanic 测试Panic
func TestPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	tw := New(7, time.Second, WithPanicFn(func(ctx context.Context, task *Task, err any, stack []byte) {
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
	}, time.Second)

	<-ctx.Done()
}
