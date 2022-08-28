package exclusion

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func testLockForNamespace(t *testing.T, c Control) {
	ctx := context.Background()

	store := map[string]bool{} // race detector
	const key = "key"

	stop := make(chan bool)
	defer close(stop)
	go func() {
		for {
			unlock, err := c.LockForNamespace(ctx)
			if err != nil {
				goto check
			}
			store[key] = true
			time.Sleep(100 * time.Microsecond)
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
		for i := 0; i < 1000; i++ {
			time.Sleep(100 * time.Microsecond)
			unlock, err := c.LockForNamespace(ctx)
			if err != nil {
				panic(err)
			}
			store[key] = false
			unlock()
		}
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("can't acquire lock in 10 seconds")
	}
}

func TestControl_LockForNamespace(t *testing.T) {
	t.Parallel()

	testLockForNamespace(t, NewControl())
}

func lockForBuild(c Control, image string) error {
	ctx := context.Background()

	store := map[string]bool{} // race detector
	const key = "key"

	stop := make(chan bool)
	defer close(stop)
	go func() {
		for {
			unlock, err := c.LockForBuild(ctx, image)
			if err != nil {
				goto check
			}
			store[key] = true
			time.Sleep(100 * time.Microsecond)
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
		for i := 0; i < 1000; i++ {
			time.Sleep(100 * time.Microsecond)
			unlock, err := c.LockForBuild(ctx, image)
			if err != nil {
				panic(err)
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

func testLockForBuild(t *testing.T, c Control) {
	var eg errgroup.Group
	for i := 0; i < 10; i++ {
		eg.Go(func() error {
			return lockForBuild(c, uuid.NewString())
		})
	}
	err := eg.Wait()
	if err != nil {
		t.Fatal(err)
	}
}

func TestControl_LockForBuild(t *testing.T) {
	t.Parallel()

	testLockForBuild(t, NewControl())
}

func lockForContainerSetup(c Control, name string) error {
	ctx := context.Background()

	store := map[string]bool{} // race detector
	const key = "key"

	stop := make(chan bool)
	defer close(stop)
	go func() {
		for {
			unlock, err := c.LockForContainerSetup(ctx, name)
			if err != nil {
				goto check
			}
			store[key] = true
			time.Sleep(100 * time.Microsecond)
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
		for i := 0; i < 1000; i++ {
			time.Sleep(100 * time.Microsecond)
			unlock, err := c.LockForContainerSetup(ctx, name)
			if err != nil {
				panic(err)
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

func testLockForContainerSetup(t *testing.T, c Control) {
	var eg errgroup.Group
	for i := 0; i < 10; i++ {
		eg.Go(func() error {
			return lockForContainerSetup(c, uuid.NewString())
		})
	}
	err := eg.Wait()
	if err != nil {
		t.Fatal(err)
	}
}

func TestControl_LockForContainerSetup(t *testing.T) {
	t.Parallel()

	testLockForContainerSetup(t, NewControl())
}

func lockForContainerUse(c Control, name string) error {
	ctx := context.Background()

	store := map[string]bool{} // race detector
	const key = "key"

	var count int32
	stop := make(chan bool)
	go func() {
		errSink := make(chan error)
		for {
			// exclusive lock continues 100 microseconds.
			// during that, more than two shared locks will be acquired.
			time.Sleep(40 * time.Microsecond)
			go func() {
				unlock, err := c.LockForContainerUse(ctx, name, false, nil)
				if err != nil {
					errSink <- err
					return
				}

				_ = store[key]
				atomic.AddInt32(&count, 1)
				time.AfterFunc(50*time.Microsecond, unlock)
			}()
			select {
			case <-stop:
				return
			case err := <-errSink:
				if err != nil {
					panic(err)
				}
			default:
			}
		}
	}()
	done := make(chan bool)
	go func() {
		for i := 0; i < 1000; i++ {
			unlock, err := c.LockForContainerUse(ctx, name, true, nil)
			if err != nil {
				panic(err)
			}
			time.Sleep(100 * time.Microsecond)
			store[key] = true
			unlock()
		}
		done <- true
	}()
	select {
	case <-done:
		close(stop)
		cnt := atomic.LoadInt32(&count)
		if cnt < 2000 {
			return fmt.Errorf("lack of shared lock: %d < 2000", cnt)
		}
	case <-time.After(10 * time.Second):
		return errors.New("can't acquire lock in 10 seconds")
	}
	return nil
}

func testLockForContainerUse(t *testing.T, c Control) {
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

	testLockForContainerUse(t, NewControl())
}

func lockForContainerUseWithInit(c Control, name string, exclusive bool) error {
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

		time.AfterFunc(30*time.Microsecond, unlock)
	}
	if exclusive && len(store[key]) != 9 {
		return fmt.Errorf("unexpected number of acquisition of exclusive lock: %d", len(store[key]))
	}
	if count != 2 {
		return fmt.Errorf("unexpected number of call initFunc: %d", count)
	}
	return nil
}

func testLockForContainerUseWithInit(t *testing.T, c Control, exclusive bool) {
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
	c := NewControl()

	t.Run("exclusive", func(t *testing.T) {
		t.Parallel()
		testLockForContainerUseWithInit(t, c, true)
	})

	t.Run("shared", func(t *testing.T) {
		t.Parallel()
		testLockForContainerUseWithInit(t, c, false)
	})
}
