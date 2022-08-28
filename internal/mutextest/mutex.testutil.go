package mutextest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/daichitakahashi/confort"
	"github.com/google/uuid"
)

var unique = confort.NewUnique(func() (string, error) {
	return uuid.New().String(), nil
})

func TestNamespaceLock(t *testing.T, ex confort.ExclusionControl) {
	ctx := context.Background()

	store := map[string]bool{} // race detector
	const key = "key"

	stop := make(chan bool)
	defer close(stop)
	go func() {
		for {
			unlock, err := ex.LockForNamespace(ctx)
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
			unlock, err := ex.LockForNamespace(ctx)
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

func TestBuildLock(t *testing.T, ex confort.ExclusionControl) {
	ctx := context.Background()

	store := map[string]bool{} // race detector
	const key = "key"

	image := unique.Must(t)

	stop := make(chan bool)
	defer close(stop)
	go func() {
		for {
			unlock, err := ex.LockForBuild(ctx, image)
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
			unlock, err := ex.LockForBuild(ctx, image)
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

func TestInitContainerLock(t *testing.T, ex confort.ExclusionControl) {
	ctx := context.Background()

	store := map[string]bool{} // race detector
	const key = "key"

	name := unique.Must(t)

	stop := make(chan bool)
	defer close(stop)
	go func() {
		for {
			unlock, err := ex.LockForContainerSetup(ctx, name)
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
			unlock, err := ex.LockForContainerSetup(ctx, name)
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

func TestAcquireContainerLock(t *testing.T, ex confort.ExclusionControl) {
	ctx := context.Background()

	store := map[string]bool{} // race detector
	const key = "key"

	var count int
	name := unique.Must(t)
	stop := make(chan bool)
	go func() {
		for {
			unlock, err := ex.LockForContainerUse(ctx, name, false)
			if err != nil {
				goto check
			}
			store[key] = false
			time.Sleep(100 * time.Microsecond)
			count++
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
	done := make(chan bool)
	go func() {
		for i := 0; i < 1000; i++ {
			time.Sleep(100 * time.Microsecond)
			unlock, err := ex.LockForContainerUse(ctx, name, true)
			if err != nil {
				panic(err)
			}
			store[key] = true
			unlock()
		}
		done <- true
	}()
	select {
	case <-done:
		close(stop)
		if count < 500 {
			t.Fatal("lack of shared lock")
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("can't acquire lock in 10 seconds")
	}
}

func TestTryAcquireContainerInitLock(t *testing.T, ex confort.ExclusionControl, acquireInitLock bool) {
	ctx := context.Background()

	name := unique.Must(t)

	var recordNum = 0
	if acquireInitLock {
		recordNum = 20
	} else {
		started := make(chan struct{})
		go func() {
			unlock, err := ex.LockForContainerUse(ctx, name, true)
			if err != nil {
				panic(err)
			}
			close(started)
			time.Sleep(time.Second * 2)
			unlock()
		}()
		<-started
	}

	record := map[int]bool{}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			downgrade, _, ok, err := ex.TryLockForContainerInitAndUse(ctx, name)
			if err != nil {
				panic(err)
			}
			if ok {
				for j := 0; j < 20; j++ {
					record[j] = true
				}
				time.Sleep(time.Millisecond * 10)
			}
			unlock, err := downgrade()
			if err != nil {
				panic(err)
			}
			if len(record) != recordNum {
				panic(fmt.Sprintf("len(record) != %d(%d)", recordNum, len(record)))
			}
			unlock()
		}()
	}
	wg.Wait()

}
