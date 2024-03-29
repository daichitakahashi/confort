package wait_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/daichitakahashi/confort/internal/mock"
	"github.com/daichitakahashi/confort/wait"
	"github.com/docker/docker/api/types"
)

func TestCheckLogOccurrence(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	f := &mock.Fetcher{
		LogFunc: func(ctx context.Context) (io.ReadCloser, error) {
			var log string

			d, _ := ctx.Deadline()
			remain := d.Sub(time.Now())
			if remain < time.Second {
				log += "creation completed\n"
			}
			if remain < 500*time.Millisecond {
				log += "initialization completed\n"
			}

			return io.NopCloser(strings.NewReader(log)), nil
		},
	}

	checker := wait.CheckLogOccurrence("completed", 2)

	time.Sleep(time.Second)
	ok, err := checker(ctx, f)
	if err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("unexpected complete")
	}

	time.Sleep(500 * time.Millisecond)
	ok, err = checker(ctx, f)
	if err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatal("expected to be completed")
	}
}

func TestCheckHealthy(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	f := &mock.Fetcher{
		StatusFunc: func(ctx context.Context) (*types.ContainerState, error) {
			status := "unhealthy"

			d, _ := ctx.Deadline()
			remain := d.Sub(time.Now())
			if remain < 500*time.Millisecond {
				status = "healthy"
			}

			return &types.ContainerState{
				Health: &types.Health{
					Status: status,
				},
			}, nil
		},
	}

	time.Sleep(300 * time.Millisecond)
	ok, err := wait.CheckHealthy(ctx, f)
	if err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("unexpected complete")
	}

	time.Sleep(300 * time.Millisecond)
	ok, err = wait.CheckHealthy(ctx, f)
	if err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatal("expected to be completed")
	}
}

func TestCheckCommandSucceeds(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	f := &mock.Fetcher{
		ExecFunc: func(ctx context.Context, cmd ...string) ([]byte, error) {
			d, _ := ctx.Deadline()
			remain := d.Sub(time.Now())
			if remain < 500*time.Millisecond {
				return nil, nil
			}
			return nil, errors.New("temporary error")
		},
	}

	check := wait.CheckCommandSucceeds([]string{"curl", "--fail", "http://localhost"})

	time.Sleep(300 * time.Millisecond)
	ok, err := check(ctx, f)
	if err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("unexpected complete")
	}

	time.Sleep(300 * time.Millisecond)
	ok, err = check(ctx, f)
	if err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatal("expected to be completed")
	}
}

func TestWaiter_Wait(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	w := wait.New(func(ctx context.Context, f wait.Fetcher) (bool, error) {
		status, err := f.Status(ctx)
		if err != nil {
			return false, err
		}
		return status.Status == "running", nil
	}, wait.WithInterval(100*time.Millisecond), wait.WithTimeout(700*time.Millisecond))

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		var count int
		f := &mock.Fetcher{
			StatusFunc: func(ctx context.Context) (*types.ContainerState, error) {
				status := "created"
				count++

				d, _ := ctx.Deadline()
				remain := d.Sub(time.Now())
				if remain < 200*time.Millisecond {
					status = "running"
				}

				return &types.ContainerState{
					Status: status,
				}, nil
			},
		}
		err := w.Wait(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		if count < 4 {
			t.Fatal("unexpected count of try to check status")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		t.Parallel()

		var count int
		f := &mock.Fetcher{
			StatusFunc: func(ctx context.Context) (*types.ContainerState, error) {
				count++
				return &types.ContainerState{
					Status: "created",
				}, nil
			},
		}
		err := w.Wait(ctx, f)
		if err == nil {
			t.Fatal("unexpected success")
		}
		if count < 6 {
			t.Fatal("unexpected count of try to check status")
		}
	})
}
