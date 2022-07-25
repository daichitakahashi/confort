package beaconserver

import (
	"context"
	"testing"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/proto/beacon"
	"github.com/google/uuid"
	"google.golang.org/grpc"
)

var unique = confort.NewUnique(func() (string, error) {
	return uuid.New().String(), nil
})

/*func TestBeaconServer_NamespaceLock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	connect := startServer(t, nil, nil)
	cli1 := beacon.NewBeaconServiceClient(connect(t))
	cli2 := beacon.NewBeaconServiceClient(connect(t))

	stream1, err := cli1.NamespaceLock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream2, err := cli2.NamespaceLock(ctx)
	if err != nil {
		t.Fatal(err)
	}

	store := map[string]bool{} // race detector
	const key = "key"

	lock := func(stream beacon.BeaconService_NamespaceLockClient) error {
		err := stream.Send(&beacon.LockRequest{
			Operation: beacon.LockOp_LOCK_OP_LOCK,
		})
		if err != nil {
			return err
		}
		resp, err := stream.Recv()
		if err != nil {
			return err
		}
		if resp.GetState() != beacon.LockState_LOCK_STATE_LOCKED {
			return fmt.Errorf("not locked: %s", resp.GetState())
		}
		return nil
	}

	unlock := func(stream beacon.BeaconService_NamespaceLockClient) error {
		err := stream.Send(&beacon.LockRequest{
			Operation: beacon.LockOp_LOCK_OP_UNLOCK,
		})
		if err != nil {
			return err
		}
		resp, err := stream.Recv()
		if err != nil {
			return err
		}
		if resp.GetState() != beacon.LockState_LOCK_STATE_UNLOCKED {
			return fmt.Errorf("not unlocked: %s", resp.GetState())
		}
		return nil
	}

	stop := make(chan bool)
	defer close(stop)
	go func() {
		for {
			err := lock(stream1)
			if err != nil {
				goto check
			}
			store[key] = true
			time.Sleep(100 * time.Microsecond)
			err = unlock(stream1)
			if err != nil {
				goto check
			}
		check:
			select {
			case <-stop:
				_ = stream1.CloseSend()
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
			err := lock(stream2)
			if err != nil {
				panic(err)
			}
			store[key] = false
			err = unlock(stream2)
			if err != nil {
				panic(err)
			}
		}
		_ = stream2.CloseSend()
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("can't acquire lock in 10 seconds")
	}
}*/

func lock(t *testing.T, stream beacon.BeaconService_NamespaceLockClient, op beacon.LockOp) (*beacon.LockResponse, error) {
	t.Helper()

	err := stream.Send(&beacon.LockRequest{
		Operation: op,
	})
	if err != nil {
		t.Fatal(err)
	}
	return stream.Recv()
}

func TestBeaconServer_NamespaceLock_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("trying second lock", func(t *testing.T) {
		t.Parallel()

		connect := startServer(t, nil, nil)
		cli := beacon.NewBeaconServiceClient(connect(t))

		stream, err := cli.NamespaceLock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = stream.CloseSend()
		})

		resp, err := lock(t, stream, beacon.LockOp_LOCK_OP_LOCK)
		if err != nil {
			t.Fatal(err)
		}
		if resp.GetState() != beacon.LockState_LOCK_STATE_LOCKED {
			t.Fatalf("not locked: %s", resp.GetState())
		}

		resp, err = lock(t, stream, beacon.LockOp_LOCK_OP_LOCK)
		if err == nil {
			t.Fatal("error expected but succeeded:", resp)
		}
	})
	t.Run("unlock of unlocked", func(t *testing.T) {
		t.Parallel()

		connect := startServer(t, nil, nil)
		cli := beacon.NewBeaconServiceClient(connect(t))

		stream, err := cli.NamespaceLock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = stream.CloseSend()
		})

		resp, err := lock(t, stream, beacon.LockOp_LOCK_OP_UNLOCK)
		if err == nil {
			t.Fatal("error expected but succeeded:", resp)
		}
	})
}

type keyedLockStream interface {
	Send(*beacon.KeyedLockRequest) error
	Recv() (*beacon.LockResponse, error)
	grpc.ClientStream
}

func keyedLock(t *testing.T, stream keyedLockStream, key string, op beacon.LockOp) (*beacon.LockResponse, error) {
	if t != nil {
		t.Helper()
	}

	err := stream.Send(&beacon.KeyedLockRequest{
		Key:       key,
		Operation: op,
	})
	if err != nil {
		if t == nil {
			return nil, err
		}
		t.Fatal(err)
	}
	return stream.Recv()
}

/*func TestBeaconServer_BuildLock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	connect := startServer(t, nil, nil)
	cli1 := beacon.NewBeaconServiceClient(connect(t))
	cli2 := beacon.NewBeaconServiceClient(connect(t))

	stream1, err := cli1.BuildLock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream2, err := cli2.BuildLock(ctx)
	if err != nil {
		t.Fatal(err)
	}

	store := map[string]bool{} // race detector
	const key = "key"

	image := unique.Must(t)
	lock := func(t *testing.T, stream beacon.BeaconService_BuildLockClient) error {
		resp, err := keyedLock(t, stream, image, beacon.LockOp_LOCK_OP_LOCK)
		if err != nil {
			return err
		}
		if resp.GetState() != beacon.LockState_LOCK_STATE_LOCKED {
			return fmt.Errorf("not locked: %s", resp.GetState())
		}
		return nil
	}

	unlock := func(t *testing.T, stream beacon.BeaconService_BuildLockClient) error {
		resp, err := keyedLock(t, stream, image, beacon.LockOp_LOCK_OP_UNLOCK)
		if err != nil {
			return err
		}
		if resp.GetState() != beacon.LockState_LOCK_STATE_UNLOCKED {
			return fmt.Errorf("not unlocked: %s", resp.GetState())
		}
		return nil
	}

	stop := make(chan bool)
	defer close(stop)
	go func() {
		for {
			err := lock(nil, stream1)
			if err != nil {
				goto check
			}
			store[key] = true
			time.Sleep(100 * time.Microsecond)
			err = unlock(nil, stream1)
			if err != nil {
				goto check
			}
		check:
			select {
			case <-stop:
				_ = stream1.CloseSend()
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
			err := lock(t, stream2)
			if err != nil {
				panic(err)
			}
			store[key] = false
			err = unlock(t, stream2)
			if err != nil {
				panic(err)
			}
		}
		_ = stream2.CloseSend()
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("can't acquire lock in 10 seconds")
	}
}*/

func TestBeaconServer_BuildLock_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	connect := startServer(t, nil, nil)
	cli := beacon.NewBeaconServiceClient(connect(t))

	t.Run("empty key", func(t *testing.T) {
		t.Parallel()

		stream, err := cli.BuildLock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = stream.CloseSend()
		})

		resp, err := keyedLock(t, stream, "", beacon.LockOp_LOCK_OP_LOCK)
		if err == nil {
			t.Fatal("error expected but succeeded:", resp)
		}
	})
	t.Run("trying second lock", func(t *testing.T) {
		t.Parallel()

		stream, err := cli.BuildLock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = stream.CloseSend()
		})

		image := unique.Must(t)
		resp, err := keyedLock(t, stream, image, beacon.LockOp_LOCK_OP_LOCK)
		if err != nil {
			t.Fatal(err)
		}
		if resp.GetState() != beacon.LockState_LOCK_STATE_LOCKED {
			t.Fatalf("not locked: %s", resp.GetState())
		}

		resp, err = keyedLock(t, stream, image, beacon.LockOp_LOCK_OP_LOCK)
		if err == nil {
			t.Fatal("error expected but succeeded:", resp)
		}
	})
	t.Run("unlock of unlocked", func(t *testing.T) {
		t.Parallel()

		stream, err := cli.BuildLock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = stream.CloseSend()
		})

		image := unique.Must(t)
		resp, err := keyedLock(t, stream, image, beacon.LockOp_LOCK_OP_UNLOCK)
		if err == nil {
			t.Fatal("error expected but succeeded:", resp)
		}
	})
}

/*func TestBeaconServer_InitContainerLock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	connect := startServer(t, nil, nil)
	cli1 := beacon.NewBeaconServiceClient(connect(t))
	cli2 := beacon.NewBeaconServiceClient(connect(t))

	stream1, err := cli1.InitContainerLock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream2, err := cli2.InitContainerLock(ctx)
	if err != nil {
		t.Fatal(err)
	}

	store := map[string]bool{} // race detector
	const key = "key"

	name := unique.Must(t)
	lock := func(t *testing.T, stream beacon.BeaconService_InitContainerLockClient) error {
		resp, err := keyedLock(t, stream, name, beacon.LockOp_LOCK_OP_LOCK)
		if err != nil {
			return err
		}
		if resp.GetState() != beacon.LockState_LOCK_STATE_LOCKED {
			return fmt.Errorf("not locked: %s", resp.GetState())
		}
		return nil
	}

	unlock := func(t *testing.T, stream beacon.BeaconService_InitContainerLockClient) error {
		resp, err := keyedLock(t, stream, name, beacon.LockOp_LOCK_OP_UNLOCK)
		if err != nil {
			return err
		}
		if resp.GetState() != beacon.LockState_LOCK_STATE_UNLOCKED {
			return fmt.Errorf("not unlocked: %s", resp.GetState())
		}
		return nil
	}

	stop := make(chan bool)
	defer close(stop)
	go func() {
		for {
			err := lock(nil, stream1)
			if err != nil {
				goto check
			}
			store[key] = true
			time.Sleep(100 * time.Microsecond)
			err = unlock(nil, stream1)
			if err != nil {
				goto check
			}
		check:
			select {
			case <-stop:
				_ = stream1.CloseSend()
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
			err := lock(t, stream2)
			if err != nil {
				panic(err)
			}
			store[key] = false
			err = unlock(t, stream2)
			if err != nil {
				panic(err)
			}
		}
		_ = stream2.CloseSend()
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("can't acquire lock in 10 seconds")
	}
}*/

func TestBeaconServer_InitContainerLock_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	connect := startServer(t, nil, nil)
	cli := beacon.NewBeaconServiceClient(connect(t))

	t.Run("empty key", func(t *testing.T) {
		t.Parallel()

		stream, err := cli.InitContainerLock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = stream.CloseSend()
		})

		resp, err := keyedLock(t, stream, "", beacon.LockOp_LOCK_OP_LOCK)
		if err == nil {
			t.Fatal("error expected but succeeded:", resp)
		}
	})
	t.Run("trying second lock", func(t *testing.T) {
		t.Parallel()

		stream, err := cli.InitContainerLock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = stream.CloseSend()
		})

		name := unique.Must(t)
		resp, err := keyedLock(t, stream, name, beacon.LockOp_LOCK_OP_LOCK)
		if err != nil {
			t.Fatal(err)
		}
		if resp.GetState() != beacon.LockState_LOCK_STATE_LOCKED {
			t.Fatalf("not locked: %s", resp.GetState())
		}

		resp, err = keyedLock(t, stream, name, beacon.LockOp_LOCK_OP_LOCK)
		if err == nil {
			t.Fatal("error expected but succeeded:", resp)
		}
	})
	t.Run("unlock of unlocked", func(t *testing.T) {
		t.Parallel()

		stream, err := cli.InitContainerLock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = stream.CloseSend()
		})

		name := unique.Must(t)
		resp, err := keyedLock(t, stream, name, beacon.LockOp_LOCK_OP_UNLOCK)
		if err == nil {
			t.Fatal("error expected but succeeded:", resp)
		}
	})
}

/*func TestBeaconServer_AcquireContainerLock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	connect := startServer(t, nil, nil)
	cli := beacon.NewBeaconServiceClient(connect(t))

	anyLock := func(t *testing.T, stream beacon.BeaconService_AcquireContainerLockClient, name string, op beacon.AcquireOp) *beacon.LockResponse {
		t.Helper()
		err := stream.Send(&beacon.AcquireLockRequest{
			Key:       name,
			Operation: op,
		})
		if err != nil {
			t.Fatal(err)
		}
		resp, err := stream.Recv()
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	lock := func(t *testing.T, name string) (time.Time, func()) {
		t.Helper()

		stream, err := cli.AcquireContainerLock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		resp := anyLock(t, stream, name, beacon.AcquireOp_ACQUIRE_OP_LOCK)
		if resp.State != beacon.LockState_LOCK_STATE_LOCKED {
			t.Fatalf("not locked: %s", resp.GetState())
		}
		return time.Now(), func() {
			t.Helper()
			resp := anyLock(t, stream, name, beacon.AcquireOp_ACQUIRE_OP_UNLOCK)
			if resp.State != beacon.LockState_LOCK_STATE_UNLOCKED {
				t.Fatalf("not unlocked: %s", resp.GetState())
			}
			err = stream.CloseSend()
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	rLock := func(t *testing.T, name string) (time.Time, func()) {
		t.Helper()

		stream, err := cli.AcquireContainerLock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		resp := anyLock(t, stream, name, beacon.AcquireOp_ACQUIRE_OP_SHARED_LOCK)
		if resp.State != beacon.LockState_LOCK_STATE_SHARED_LOCKED {
			t.Fatalf("not locked: %s", resp.GetState())
		}
		return time.Now(), func() {
			t.Helper()
			resp := anyLock(t, stream, name, beacon.AcquireOp_ACQUIRE_OP_UNLOCK)
			if resp.State != beacon.LockState_LOCK_STATE_UNLOCKED {
				t.Fatalf("not unlocked: %s", resp.GetState())
			}
			err = stream.CloseSend()
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	t.Run("double exclusive lock", func(t *testing.T) {
		t.Parallel()

		name := unique.Must(t)
		order := make(chan struct{})

		var first, second time.Time
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			var unlock func()
			first, unlock = lock(t, name)
			close(order)
			time.Sleep(time.Millisecond * 500)
			unlock()
		}()
		go func() {
			defer wg.Done()
			var unlock func()
			<-order
			second, unlock = lock(t, name)
			unlock()
		}()
		wg.Wait()

		if !first.Before(second) {
			t.Fatalf("unexpected lock order: first=%s second=%s", first, second)
		}
		if second.Sub(first) < time.Millisecond*500 {
			t.Fatal("too early")
		}
	})

	t.Run("double shared lock", func(t *testing.T) {
		t.Parallel()

		name := unique.Must(t)
		start := time.Now()

		var first, second time.Time
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			var unlock func()
			first, unlock = rLock(t, name)
			time.Sleep(time.Millisecond * 500)
			unlock()
		}()
		go func() {
			defer wg.Done()
			var unlock func()
			second, unlock = rLock(t, name)
			time.Sleep(time.Millisecond * 500)
			unlock()
		}()
		wg.Wait()

		firstDur := first.Sub(start)
		secondDur := second.Sub(start)
		if firstDur > time.Millisecond*500 || secondDur > time.Millisecond*500 {
			t.Fatalf("it takes too long: first=%s, second=%s", firstDur, secondDur)
		}
	})

	t.Run("shared lock during exclusive lock", func(t *testing.T) {
		t.Parallel()

		name := unique.Must(t)
		order := make(chan struct{})

		var first, second time.Time
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			var unlock func()
			first, unlock = lock(t, name)
			close(order)
			time.Sleep(time.Millisecond * 500)
			unlock()
		}()
		go func() {
			defer wg.Done()
			var unlock func()
			<-order
			second, unlock = rLock(t, name)
			unlock()
		}()
		wg.Wait()

		if !first.Before(second) {
			t.Fatalf("unexpected lock order: first=%s second=%s", first, second)
		}
		if second.Sub(first) < time.Millisecond*500 {
			t.Fatal("too early")
		}
	})

	t.Run("exclusive lock during shared lock", func(t *testing.T) {
		t.Parallel()

		name := unique.Must(t)
		order := make(chan struct{})

		var first, second time.Time
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			var unlock func()
			first, unlock = rLock(t, name)
			close(order)
			time.Sleep(time.Millisecond * 500)
			unlock()
		}()
		go func() {
			defer wg.Done()
			var unlock func()
			<-order
			second, unlock = lock(t, name)
			unlock()
		}()
		wg.Wait()

		if !first.Before(second) {
			t.Fatalf("unexpected lock order: first=%s second=%s", first, second)
		}
		if second.Sub(first) < time.Millisecond*500 {
			t.Fatal("too early")
		}
	})

}*/
