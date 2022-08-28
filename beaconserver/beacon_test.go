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

func lock(t *testing.T, stream beacon.BeaconService_LockForNamespaceClient, op beacon.LockOp) (*beacon.LockResponse, error) {
	t.Helper()

	err := stream.Send(&beacon.LockRequest{
		Operation: op,
	})
	if err != nil {
		t.Fatal(err)
	}
	return stream.Recv()
}

func TestBeaconServer_LockForNamespace_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("trying second lock", func(t *testing.T) {
		t.Parallel()

		connect := startServer(t, nil)
		cli := beacon.NewBeaconServiceClient(connect(t))

		stream, err := cli.LockForNamespace(ctx)
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

		connect := startServer(t, nil)
		cli := beacon.NewBeaconServiceClient(connect(t))

		stream, err := cli.LockForNamespace(ctx)
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

func TestBeaconServer_LockForBuild_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	connect := startServer(t, nil)
	cli := beacon.NewBeaconServiceClient(connect(t))

	t.Run("empty key", func(t *testing.T) {
		t.Parallel()

		stream, err := cli.LockForBuild(ctx)
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

		stream, err := cli.LockForBuild(ctx)
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

		stream, err := cli.LockForBuild(ctx)
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

func TestBeaconServer_LockForContainerSetup_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	connect := startServer(t, nil)
	cli := beacon.NewBeaconServiceClient(connect(t))

	t.Run("empty key", func(t *testing.T) {
		t.Parallel()

		stream, err := cli.LockForContainerSetup(ctx)
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

		stream, err := cli.LockForContainerSetup(ctx)
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

		stream, err := cli.LockForContainerSetup(ctx)
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

func TestBeaconServer_AcquireContainerLock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	connect := startServer(t, nil)
	cli := beacon.NewBeaconServiceClient(connect(t))

	newStream := func(t *testing.T) beacon.BeaconService_AcquireContainerLockClient {
		t.Helper()

		stream, err := cli.AcquireContainerLock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = stream.CloseSend()
		})
		return stream
	}

	lock := func(t *testing.T, stream beacon.BeaconService_AcquireContainerLockClient, name string, op beacon.AcquireOp) (*beacon.AcquireLockResponse, error) {
		t.Helper()
		err := stream.Send(&beacon.AcquireLockRequest{
			Key:       name,
			Operation: op,
		})
		if err != nil {
			t.Fatal(err)
		}
		return stream.Recv()
	}

	assertError := func(t *testing.T, resp *beacon.AcquireLockResponse, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("error expected but succeeded: %#v", resp)
		}
	}

	t.Run("empty key", func(t *testing.T) {
		t.Parallel()

		resp, err := lock(t, newStream(t), "", beacon.AcquireOp_ACQUIRE_OP_LOCK)
		assertError(t, resp, err)
	})

	t.Run("trying second lock", func(t *testing.T) {
		t.Parallel()

		operations := []beacon.AcquireOp{
			beacon.AcquireOp_ACQUIRE_OP_LOCK,
			beacon.AcquireOp_ACQUIRE_OP_SHARED_LOCK,
			beacon.AcquireOp_ACQUIRE_OP_INIT_LOCK,
			beacon.AcquireOp_ACQUIRE_OP_INIT_SHARED_LOCK,
		}
		for _, op := range operations {
			op := op
			t.Run(op.String(), func(t *testing.T) {
				t.Parallel()
				stream := newStream(t)
				name := unique.Must(t)
				_, err := lock(t, stream, name, op)
				if err != nil {
					t.Fatal(err)
				}

				resp, err := lock(t, stream, name, op)
				assertError(t, resp, err)
			})
		}
	})

	t.Run("unlock on unlocked key", func(t *testing.T) {
		t.Parallel()

		t.Run("unlocked", func(t *testing.T) {
			t.Parallel()
			resp, err := lock(t, newStream(t), unique.Must(t), beacon.AcquireOp_ACQUIRE_OP_UNLOCK)
			assertError(t, resp, err)
		})

		t.Run("different key", func(t *testing.T) {
			t.Parallel()
			stream := newStream(t)
			_, err := lock(t, stream, unique.Must(t), beacon.AcquireOp_ACQUIRE_OP_LOCK)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := lock(t, stream, unique.Must(t), beacon.AcquireOp_ACQUIRE_OP_UNLOCK)
			assertError(t, resp, err)
		})
	})

	t.Run("set init result on unlocked key", func(t *testing.T) {
		t.Parallel()

		operations := []beacon.AcquireOp{
			beacon.AcquireOp_ACQUIRE_OP_SET_INIT_DONE,
			beacon.AcquireOp_ACQUIRE_OP_SET_INIT_FAILED,
		}

		for _, op := range operations {
			op := op
			t.Run(op.String(), func(t *testing.T) {
				t.Parallel()

				t.Run("unlocked", func(t *testing.T) {
					t.Parallel()
					resp, err := lock(t, newStream(t), unique.Must(t), op)
					assertError(t, resp, err)
				})

				t.Run("different key", func(t *testing.T) {
					t.Parallel()
					stream := newStream(t)
					_, err := lock(t, stream, unique.Must(t), beacon.AcquireOp_ACQUIRE_OP_INIT_LOCK)
					if err != nil {
						t.Fatal(err)
					}
					resp, err := lock(t, stream, unique.Must(t), op)
					assertError(t, resp, err)
				})
			})
		}
	})

	t.Run("set init result on lock without init", func(t *testing.T) {
		t.Parallel()

		operations := []beacon.AcquireOp{
			beacon.AcquireOp_ACQUIRE_OP_SET_INIT_DONE,
			beacon.AcquireOp_ACQUIRE_OP_SET_INIT_FAILED,
		}

		for _, op := range operations {
			op := op
			t.Run(op.String(), func(t *testing.T) {
				t.Parallel()
				stream := newStream(t)
				name := unique.Must(t)
				_, err := lock(t, stream, name, beacon.AcquireOp_ACQUIRE_OP_LOCK)
				if err != nil {
					t.Fatal(err)
				}
				resp, err := lock(t, stream, name, op)
				assertError(t, resp, err)
			})
		}
	})
}
