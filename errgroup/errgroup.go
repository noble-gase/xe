package errgroup

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
)

// A ErrGroup is a collection of goroutines working on subtasks that are part of
// the same overall task. A ErrGroup should not be reused for different tasks.
//
// A zero Group is valid, has no limit on the number of active goroutines,
// and does not cancel on error. use WithContext instead.
type ErrGroup interface {
	// Go calls the given function in a goroutine.
	//
	// The first call to return a non-nil error cancels the group; its error will be
	// returned by Wait.
	Go(fn func(ctx context.Context) error)

	// Wait blocks until all function calls from the Go method have returned, then
	// returns the first non-nil error (if any) from them.
	Wait() error
}

type group struct {
	wg sync.WaitGroup

	err  error
	once sync.Once

	remain int

	ch    chan func(ctx context.Context) error
	cache []func(ctx context.Context) error

	ctx    context.Context
	cancel context.CancelCauseFunc
}

// WithContext returns a new ErrGroup that is associated with a derived Context.
//
// The returned group's Context is canceled in the following cases:
//   - The first time a goroutine started with Go returns a non-nil error.
//   - Or when Wait is called and returns.
//
// If limit > 0, the group restricts the number of active goroutines
// to at most 'limit'. Additional functions passed to Go will be queued
// and executed only when running goroutines complete.
//
// The derived Context is created with context.WithCancelCause, so the
// cancellation reason is preserved and can be retrieved via context.Cause.
func WithContext(ctx context.Context, limit int) ErrGroup {
	ctx, cancel := context.WithCancelCause(ctx)

	g := &group{
		remain: limit,

		ctx:    ctx,
		cancel: cancel,
	}
	if limit > 0 {
		g.remain = limit
		g.ch = make(chan func(context.Context) error)
	}
	return g
}

func (g *group) Go(fn func(ctx context.Context) error) {
	g.wg.Add(1)

	if g.ch == nil {
		go g.do(fn)
		return
	}

	select {
	case g.ch <- fn:
	default:
		if g.remain > 0 {
			g.spawn()
			g.remain--
		}
		select {
		case g.ch <- fn:
		default:
			g.cache = append(g.cache, fn)
		}
	}
}

func (g *group) Wait() error {
	defer func() {
		select {
		case <-g.ctx.Done():
		default:
			g.cancel(nil)
		}
		if g.ch != nil {
			close(g.ch) // let all receiver exit
		}
	}()

	if g.ch != nil {
		for _, fn := range g.cache {
			g.ch <- fn
		}
		g.cache = nil
	}
	g.wg.Wait()

	return g.err
}

func (g *group) spawn() {
	go func() {
		for fn := range g.ch {
			g.do(fn)
		}
	}()
}

func (g *group) do(fn func(ctx context.Context) error) {
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

	select {
	case <-g.ctx.Done():
		err = g.ctx.Err()
	default:
		err = fn(g.ctx)
	}
}
