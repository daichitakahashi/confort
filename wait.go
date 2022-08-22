package confort

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/lestrrat-go/option"
)

type Waiter struct {
	interval time.Duration
	timeout  time.Duration
	checker  func(ctx context.Context, fetcher Fetcher) (bool, error)
}

type Fetcher interface {
	Status(ctx context.Context) (*types.ContainerState, error)
	Ports() Ports
	Log(ctx context.Context) (io.ReadCloser, error)
}

type fetcher struct {
	cli         *client.Client
	containerID string
	ports       Ports
}

func (f *fetcher) Status(ctx context.Context) (*types.ContainerState, error) {
	i, err := f.cli.ContainerInspect(ctx, f.containerID)
	return i.State, err
}

func (f *fetcher) Ports() Ports {
	return f.ports
}

func (f *fetcher) Log(ctx context.Context) (io.ReadCloser, error) {
	return f.cli.ContainerLogs(ctx, f.containerID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
}

var _ Fetcher = (*fetcher)(nil)

type (
	WaitOption interface {
		option.Interface
		wait() WaitOption
	}
	identOptionInterval struct{}
	identOptionTimeout  struct{}
	waitOption          struct{ option.Interface }
)

func (o waitOption) wait() WaitOption { return o }

func WithInterval(d time.Duration) WaitOption {
	return waitOption{
		Interface: option.New(identOptionInterval{}, d),
	}.wait()
}

func WithTimeout(d time.Duration) WaitOption {
	return waitOption{
		Interface: option.New(identOptionTimeout{}, d),
	}.wait()
}

const (
	defaultInterval = 500 * time.Millisecond
	defaultTimeout  = 30 * time.Second
)

func NewWaiter(f func(ctx context.Context, f Fetcher) (bool, error), opts ...WaitOption) *Waiter {
	w := &Waiter{
		interval: defaultInterval,
		timeout:  defaultTimeout,
		checker:  f,
	}

	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionInterval{}:
			w.interval = opt.Value().(time.Duration)
		case identOptionTimeout{}:
			w.timeout = opt.Value().(time.Duration)
		}
	}

	return w
}

func LogContains(message string, occurrence int, opts ...WaitOption) *Waiter {
	return NewWaiter(CheckLogOccurrence(message, occurrence), opts...)
}

func CheckLogOccurrence(message string, occurrence int) func(ctx context.Context, f Fetcher) (bool, error) {
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

func Healthy(opts ...WaitOption) *Waiter {
	return NewWaiter(CheckHealthy, opts...)
}

func CheckHealthy(ctx context.Context, f Fetcher) (bool, error) {
	status, err := f.Status(ctx)
	if err != nil {
		return false, err
	}
	return status.Health != nil && status.Health.Status == "healthy", nil
}

func (w *Waiter) Wait(ctx context.Context, f Fetcher) error {
	ctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()

	for {
		ok, err := w.checker(ctx, f)
		if err != nil {
			return err
		} else if ok {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(w.interval):
		}
	}
}
