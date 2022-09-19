package exclusion_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/daichitakahashi/confort/beaconserver"
	"github.com/daichitakahashi/confort/internal/exclusion"
	"github.com/daichitakahashi/confort/proto/beacon"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func newBeaconControl(t *testing.T) exclusion.Control {
	t.Helper()

	srv := grpc.NewServer()
	beaconserver.Register(srv, func() error {
		return nil
	})

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = srv.Serve(ln)
		_ = ln.Close()
	}()
	t.Cleanup(func() {
		srv.Stop()
	})

	conn, err := grpc.Dial(ln.Addr().String(), grpc.WithTransportCredentials(
		insecure.NewCredentials(),
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	return exclusion.NewBeaconControl(beacon.NewBeaconServiceClient(conn))
}

type control struct {
	name    string
	control exclusion.Control
}

func controls(t *testing.T) [2]control {
	return [2]control{
		{
			name:    "control",
			control: exclusion.NewControl(),
		}, {
			name:    "beaconControl",
			control: newBeaconControl(t),
		},
	}
}

func testLock(acquireLock func(ctx context.Context) (func(), error)) error {
	ctx := context.Background()

	store := map[string]bool{} // race detector
	const key = "key"

	stop := make(chan bool)
	defer close(stop)
	go func() {
		for {
			time.Sleep(time.Millisecond)
			unlock, err := acquireLock(ctx)
			if err != nil {
				goto check
			}
			store[key] = true
			unlock()
		check:
			select {
			case <-stop:
				return
			default:
				if err != nil {
					panic(err)
				}
			}
		}
	}()
	done := make(chan bool, 1)
	go func() {
		for i := 0; i < 200; i++ {
			unlock, err := acquireLock(ctx)
			if err != nil {
				log.Printf("%d/200 %s", i+1, err)
				return
			}
			store[key] = false
			unlock()
		}
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		return errors.New("can't acquire lock in 10 seconds")
	}
	return nil
}

func TestControl_LockForNamespace(t *testing.T) {
	t.Parallel()

	for _, c := range controls(t) {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := testLock(func(ctx context.Context) (func(), error) {
				return c.control.LockForNamespace(ctx)
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func testLockForBuild(t *testing.T, c exclusion.Control) {
	var eg errgroup.Group
	for i := 0; i < 10; i++ {
		eg.Go(func() error {
			name := uuid.NewString()
			return testLock(func(ctx context.Context) (func(), error) {
				return c.LockForBuild(ctx, name)
			})
		})
	}
	err := eg.Wait()
	if err != nil {
		t.Fatal(err)
	}
}

func TestControl_LockForBuild(t *testing.T) {
	t.Parallel()

	for _, c := range controls(t) {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			testLockForBuild(t, c.control)
		})
	}
}

func testLockForContainerSetup(t *testing.T, c exclusion.Control) {
	var eg errgroup.Group
	for i := 0; i < 40; i++ {
		eg.Go(func() error {
			name := uuid.NewString()
			return testLock(func(ctx context.Context) (func(), error) {
				return c.LockForContainerSetup(ctx, name)
			})
		})
	}
	err := eg.Wait()
	if err != nil {
		t.Fatal(err)
	}
}

func TestControl_LockForContainerSetup(t *testing.T) {
	t.Parallel()

	for _, c := range controls(t) {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			testLockForContainerSetup(t, c.control)
		})
	}
}

func lockForContainerUse(c exclusion.Control, name string) error {
	ctx := context.Background()

	store := map[string]bool{} // race detector
	const key = "key"

	var count int32
	stop := make(chan bool)
	go func() {
		for {
			// exclusive lock continues a millisecond.
			// during that, more than two shared locks will be acquired.
			time.Sleep(time.Millisecond / 2)

			unlock, err := c.LockForContainerUse(ctx, name, false, nil)
			if err != nil {
				if status.Code(err) != codes.Canceled {
					panic(err)
				}
				return
			}

			_ = store[key]
			atomic.AddInt32(&count, 1)
			unlock()

			select {
			case <-stop:
				return
			default:
			}
		}
	}()
	done := make(chan bool)
	go func() {
		for i := 0; i < 200; i++ {
			unlock, err := c.LockForContainerUse(ctx, name, true, nil)
			if err != nil {
				log.Printf("%d/200 %s", i+1, err)
				return
			}
			store[key] = true
			unlock()
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()
	select {
	case <-done:
		close(stop)
		cnt := atomic.LoadInt32(&count)
		if cnt < 100 {
			return fmt.Errorf("lack of shared lock: %d < 100", cnt)
		}
	case <-time.After(10 * time.Second):
		close(stop)
		return errors.New("can't acquire lock in 10 seconds")
	}
	return nil
}

func testLockForContainerUse(t *testing.T, c exclusion.Control) {
	var eg errgroup.Group
	for i := 0; i < 10; i++ {
		eg.Go(func() error {
			return lockForContainerUse(c, uuid.NewString())
		})
	}
	err := eg.Wait()
	if err != nil {
		t.Fatal(err)
	}
}

func TestControl_LockForContainerUse(t *testing.T) {
	t.Parallel()

	for _, c := range controls(t) {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			testLockForContainerUse(t, c.control)
		})
	}
}

func lockForContainerUseWithInit(c exclusion.Control, name string, exclusive bool) error {
	ctx := context.Background()

	store := map[string][]int{} // race detector
	const key = "key"

	var count int32
	var sentinel = errors.New("sentinel error")

	for i := 0; i < 10; i++ {
		unlock, err := c.LockForContainerUse(ctx, name, exclusive, func() error {
			count++
			if count == 1 {
				return sentinel
			}
			return nil
		})
		if err != nil {
			if errors.Is(err, sentinel) {
				continue
			}
			return err
		}
		if exclusive {
			store[key] = append(store[key], 0)
		} else {
			_ = store[key]
		}

		time.AfterFunc(time.Millisecond, unlock)
	}
	if exclusive && len(store[key]) != 9 {
		return fmt.Errorf("unexpected number of acquisition of exclusive lock: %d", len(store[key]))
	}
	if count != 2 {
		return fmt.Errorf("unexpected number of call initFunc: %d", count)
	}
	return nil
}

func testLockForContainerUseWithInit(t *testing.T, c exclusion.Control, exclusive bool) {
	var eg errgroup.Group
	for i := 0; i < 10; i++ {
		eg.Go(func() error {
			return lockForContainerUseWithInit(c, uuid.NewString(), exclusive)
		})
	}
	err := eg.Wait()
	if err != nil {
		t.Fatal(err)
	}
}

func TestControl_LockForContainerUse_WithInit(t *testing.T) {
	t.Parallel()

	for _, c := range controls(t) {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			t.Run("exclusive", func(t *testing.T) {
				t.Parallel()
				testLockForContainerUseWithInit(t, c.control, true)
			})

			t.Run("shared", func(t *testing.T) {
				t.Parallel()
				testLockForContainerUseWithInit(t, c.control, false)
			})
		})
	}
}

func testLockForContainerUseDowngrade(t *testing.T, c exclusion.Control) {
	ctx := context.Background()

	// exclusive
	t.Run("shared lock after exclusive lock", func(t *testing.T) {
		t.Parallel()

		name := uuid.NewString()
		unlock, err := c.LockForContainerUse(ctx, name, true, func() error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()

		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_, err = c.LockForContainerUse(ctx, name, false, nil)
		if err == nil {
			t.Fatalf("unexpected lock acquisition")
		} else if !errors.Is(err, context.DeadlineExceeded) && status.Code(err) != codes.DeadlineExceeded {
			t.Fatalf("unexpected error: %#v", err)
		}
	})

	// shared
	t.Run("consecutive shared lock", func(t *testing.T) {
		t.Parallel()

		name := uuid.NewString()
		unlock, err := c.LockForContainerUse(ctx, name, false, func() error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()

		unlock, err = c.LockForContainerUse(ctx, name, false, func() error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()
	})
}

func TestControl_LockForContainerUse_Downgrade(t *testing.T) {
	t.Parallel()

	for _, c := range controls(t) {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			testLockForContainerUseDowngrade(t, c.control)
		})
	}
}
