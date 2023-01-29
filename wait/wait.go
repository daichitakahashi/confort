package wait

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/go-connections/nat"
	"github.com/lestrrat-go/option"
	"golang.org/x/sync/errgroup"
)

type Waiter struct {
	interval   time.Duration
	timeout    time.Duration
	minSuccess int
	check      Checker
}

// Fetcher provides several ways to access the state of the container.
type Fetcher interface {
	ContainerID() string
	Status(ctx context.Context) (*types.ContainerState, error)
	Ports() nat.PortMap
	Log(ctx context.Context) (io.ReadCloser, error)
	Exec(ctx context.Context, cmd ...string) ([]byte, error)
}

type (
	Option interface {
		option.Interface
		wait() Option
	}
	identOptionInterval   struct{}
	identOptionTimeout    struct{}
	identOptionMinSuccess struct{}
	waitOption            struct{ option.Interface }
)

func (o waitOption) wait() Option { return o }

// WithInterval sets the interval between container readiness checks.
func WithInterval(d time.Duration) Option {
	return waitOption{
		Interface: option.New(identOptionInterval{}, d),
	}.wait()
}

// WithTimeout sets the timeout for waiting for the container to be ready.
func WithTimeout(d time.Duration) Option {
	return waitOption{
		Interface: option.New(identOptionTimeout{}, d),
	}.wait()
}

func WithMinSuccess(n int) Option {
	return waitOption{
		Interface: option.New(identOptionMinSuccess{}, n),
	}.wait()
}

const (
	defaultInterval = 500 * time.Millisecond
	defaultTimeout  = 30 * time.Second
)

type (
	Checker interface {
		Check(ctx context.Context, f Fetcher) (bool, error)
	}

	Cloner interface {
		Clone() Checker
	}

	CheckFunc func(ctx context.Context, f Fetcher) (bool, error)
)

func (fn CheckFunc) Check(ctx context.Context, f Fetcher) (bool, error) {
	return fn(ctx, f)
}

// New creates a Waiter that waits for the container to be ready.
// CheckFunc is the criteria for evaluating readiness. Use Fetcher to obtain
// the container status.
//
// Waiter repeatedly checks the readiness until first success. We can set
// interval and timeout by WithInterval and WithTimeout. The default value for
// the interval is 500ms and for the timeout is 30sec.
func New(check CheckFunc, opts ...Option) *Waiter {
	return NewWaiter(check, opts...)
}

func NewWaiter(c Checker, opts ...Option) *Waiter {
	w := &Waiter{
		interval: defaultInterval,
		timeout:  defaultTimeout,
		check:    c,
	}
	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionInterval{}:
			w.interval = opt.Value().(time.Duration)
		case identOptionTimeout{}:
			w.timeout = opt.Value().(time.Duration)
		case identOptionMinSuccess{}:
			w.minSuccess = opt.Value().(int)
		}
	}
	return w
}

// LogContains waits for the given number of occurrences of the given message
// in the container log.
func LogContains(message string, occurrence int, opts ...Option) *Waiter {
	return New(CheckLogOccurrence(message, occurrence), opts...)
}

// CheckLogOccurrence creates CheckFunc. See LogContains.
func CheckLogOccurrence(message string, occurrence int) CheckFunc {
	msg := []byte(message)
	return func(ctx context.Context, f Fetcher) (bool, error) {
		rc, err := f.Log(ctx)
		if err != nil {
			return false, err
		}
		defer func() {
			_ = rc.Close()
		}()

		data, err := io.ReadAll(rc)
		if err != nil {
			return false, err
		}

		return bytes.Count(data, msg) >= occurrence, nil
	}
}

// Healthy waits for the container's health status to be healthy.
func Healthy(opts ...Option) *Waiter {
	return New(CheckHealthy, opts...)
}

// CheckHealthy is a CheckFunc. See Healthy.
func CheckHealthy(ctx context.Context, f Fetcher) (bool, error) {
	status, err := f.Status(ctx)
	if err != nil {
		return false, err
	}
	return status.Health != nil && status.Health.Status == "healthy", nil
}

// CommandSucceeds waits for the success of given command.
func CommandSucceeds(cmd []string, opts ...Option) *Waiter {
	return New(CheckCommandSucceeds(cmd), opts...)
}

// CheckCommandSucceeds creates CheckFunc. See CommandSucceeds.
func CheckCommandSucceeds(cmd []string) CheckFunc {
	return func(ctx context.Context, f Fetcher) (bool, error) {
		_, err := f.Exec(ctx, cmd...)
		return err == nil, nil
	}
}

// Wait calls CheckFunc with given Fetcher repeatedly until the first success.
// Deprecated: Wait calls CheckFunc with given Fetcher repeatedly until the first success.
func (w *Waiter) Wait(ctx context.Context, f Fetcher) error {
	return w.WaitForReady(ctx, []Fetcher{f})
}

type ready struct {
	sem   chan struct{}
	ready chan struct{}
}

func (w *Waiter) ready(ctx context.Context, n int) *ready {
	if w.minSuccess <= 0 || n <= 1 {
		return &ready{}
	}

	r := &ready{
		sem:   make(chan struct{}, n),
		ready: make(chan struct{}, 1),
	}
	go func() {
		var c int
		for {
			select {
			case <-ctx.Done():
				return
			case <-r.sem:
				c++
				if w.minSuccess <= c {
					close(r.ready)
					return
				}
			}
		}
	}()
	return r
}

func (r *ready) add(ctx context.Context) {
	go func() {
		select {
		case <-ctx.Done():
		case r.sem <- struct{}{}:
		default:
		}
	}()
}

func (w *Waiter) WaitForReady(ctx context.Context, f []Fetcher) error {
	ctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()

	r := w.ready(ctx, len(f))
	cc, _ := w.check.(Cloner)
	eg, ctx := errgroup.WithContext(ctx)

	for i := range f {
		c := w.check
		if i > 0 && cc != nil {
			c = cc.Clone()
		}
		eg.Go(func() error {
			for {
				ok, err := c.Check(ctx, f[i])
				if err != nil {
					return err
				} else if ok {
					r.add(ctx)
					return nil
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-r.ready:
					return nil
				case <-time.After(w.interval):
				}
			}
		})
	}
	return eg.Wait()
}
