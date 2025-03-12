package errgroup

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/noble-gase/xe/worker"
)

// A group is a collection of goroutines working on subtasks that are part of
// the same overall task.
//
// A zero group is valid, has no limit on the number of active goroutines,
// and does not cancel on error. use WithContext instead.
type Group interface {
	// Go calls the given function in a new goroutine.
	//
	// The first call to return a non-nil error cancels the group; its error will be
	// returned by Wait.
	Go(fn func(ctx context.Context) error)

	// Wait blocks until all function calls from the Go method have returned, then
	// returns the first non-nil error (if any) from them.
	Wait() error
}

type group struct {
	wg   sync.WaitGroup
	pool worker.Pool

	err  error
	once sync.Once

	ctx    context.Context
	cancel context.CancelCauseFunc
}

// WithContext returns a new group with a canceled Context derived from ctx.
//
// The derived Context is canceled the first time a function passed to Go
// returns a non-nil error or the first time Wait returns, whichever occurs first.
func WithContext(ctx context.Context, pool worker.Pool) Group {
	ctx, cancel := context.WithCancelCause(ctx)

	return &group{
		pool:   pool,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (g *group) Go(fn func(ctx context.Context) error) {
	g.wg.Add(1)
	g.do(fn)
}

func (g *group) Wait() error {
	defer g.cancel(g.err)
	g.wg.Wait()

	return g.err
}

func (g *group) do(fn func(ctx context.Context) error) {
	g.pool.Sync(g.ctx, func(ctx context.Context) {
		var err error
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("errgroup panic recovered: %+v\n%s", r, string(debug.Stack()))
			}
			if err != nil {
				g.once.Do(func() {
					g.err = err
					g.cancel(err)
				})
			}
			g.wg.Done()
		}()
		err = fn(ctx)
	})
}
