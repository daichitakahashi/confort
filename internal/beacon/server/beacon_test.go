package server

import (
	"context"
	"testing"

	"github.com/daichitakahashi/confort/internal/beacon/proto"
	"github.com/daichitakahashi/confort/unique"
	"github.com/google/uuid"
	"google.golang.org/grpc"
)

var uniq = unique.Must(unique.New(context.Background(), func() (string, error) {
	return uuid.New().String(), nil
}))

func lock(t *testing.T, stream proto.BeaconService_LockForNamespaceClient, op proto.LockOp) (*proto.LockResponse, error) {
	t.Helper()

	err := stream.Send(&proto.LockRequest{
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
		cli := proto.NewBeaconServiceClient(connect(t))

		stream, err := cli.LockForNamespace(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = stream.CloseSend()
		})

		resp, err := lock(t, stream, proto.LockOp_LOCK_OP_LOCK)
		if err != nil {
			t.Fatal(err)
		}
		if resp.GetState() != proto.LockState_LOCK_STATE_LOCKED {
			t.Fatalf("not locked: %s", resp.GetState())
		}

		resp, err = lock(t, stream, proto.LockOp_LOCK_OP_LOCK)
		if err == nil {
			t.Fatal("error expected but succeeded:", resp)
		}
	})
	t.Run("unlock of unlocked", func(t *testing.T) {
		t.Parallel()

		connect := startServer(t, nil)
		cli := proto.NewBeaconServiceClient(connect(t))

		stream, err := cli.LockForNamespace(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = stream.CloseSend()
		})

		resp, err := lock(t, stream, proto.LockOp_LOCK_OP_UNLOCK)
		if err == nil {
			t.Fatal("error expected but succeeded:", resp)
		}
	})
}

type keyedLockStream interface {
	Send(*proto.KeyedLockRequest) error
	Recv() (*proto.LockResponse, error)
	grpc.ClientStream
}

func keyedLock(t *testing.T, stream keyedLockStream, key string, op proto.LockOp) (*proto.LockResponse, error) {
	if t != nil {
		t.Helper()
	}

	err := stream.Send(&proto.KeyedLockRequest{
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
	cli := proto.NewBeaconServiceClient(connect(t))

	t.Run("empty key", func(t *testing.T) {
		t.Parallel()

		stream, err := cli.LockForBuild(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = stream.CloseSend()
		})

		resp, err := keyedLock(t, stream, "", proto.LockOp_LOCK_OP_LOCK)
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

		image := uniq.Must(t)
		resp, err := keyedLock(t, stream, image, proto.LockOp_LOCK_OP_LOCK)
		if err != nil {
			t.Fatal(err)
		}
		if resp.GetState() != proto.LockState_LOCK_STATE_LOCKED {
			t.Fatalf("not locked: %s", resp.GetState())
		}

		resp, err = keyedLock(t, stream, image, proto.LockOp_LOCK_OP_LOCK)
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

		image := uniq.Must(t)
		resp, err := keyedLock(t, stream, image, proto.LockOp_LOCK_OP_UNLOCK)
		if err == nil {
			t.Fatal("error expected but succeeded:", resp)
		}
	})
}

func TestBeaconServer_LockForContainerSetup_Error(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	connect := startServer(t, nil)
	cli := proto.NewBeaconServiceClient(connect(t))

	t.Run("empty key", func(t *testing.T) {
		t.Parallel()

		stream, err := cli.LockForContainerSetup(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = stream.CloseSend()
		})

		resp, err := keyedLock(t, stream, "", proto.LockOp_LOCK_OP_LOCK)
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

		name := uniq.Must(t)
		resp, err := keyedLock(t, stream, name, proto.LockOp_LOCK_OP_LOCK)
		if err != nil {
			t.Fatal(err)
		}
		if resp.GetState() != proto.LockState_LOCK_STATE_LOCKED {
			t.Fatalf("not locked: %s", resp.GetState())
		}

		resp, err = keyedLock(t, stream, name, proto.LockOp_LOCK_OP_LOCK)
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

		name := uniq.Must(t)
		resp, err := keyedLock(t, stream, name, proto.LockOp_LOCK_OP_UNLOCK)
		if err == nil {
			t.Fatal("error expected but succeeded:", resp)
		}
	})
}

func TestBeaconServer_AcquireContainerLock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	connect := startServer(t, nil)
	cli := proto.NewBeaconServiceClient(connect(t))

	newStream := func(t *testing.T) proto.BeaconService_AcquireContainerLockClient {
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

	lock := func(t *testing.T, stream proto.BeaconService_AcquireContainerLockClient, name string, op proto.AcquireOp) (*proto.AcquireLockResponse, error) {
		t.Helper()
		err := stream.Send(&proto.AcquireLockRequest{
			Param: &proto.AcquireLockRequest_Acquire{
				Acquire: &proto.AcquireLockAcquireParam{
					Targets: map[string]*proto.AcquireLockParam{
						name: {
							Operation: op,
						},
					},
				},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return stream.Recv()
	}

	assertError := func(t *testing.T, resp *proto.AcquireLockResponse, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("error expected but succeeded: %#v", resp)
		}
	}

	t.Run("empty key", func(t *testing.T) {
		t.Parallel()

		resp, err := lock(t, newStream(t), "", proto.AcquireOp_ACQUIRE_OP_LOCK)
		assertError(t, resp, err)
	})

	t.Run("trying second lock", func(t *testing.T) {
		t.Parallel()

		operations := []proto.AcquireOp{
			proto.AcquireOp_ACQUIRE_OP_LOCK,
			proto.AcquireOp_ACQUIRE_OP_SHARED_LOCK,
			proto.AcquireOp_ACQUIRE_OP_INIT_LOCK,
			proto.AcquireOp_ACQUIRE_OP_INIT_SHARED_LOCK,
		}
		for _, op := range operations {
			op := op
			t.Run(op.String(), func(t *testing.T) {
				t.Parallel()
				stream := newStream(t)
				name := uniq.Must(t)
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
			resp, err := lock(t, newStream(t), uniq.Must(t), proto.AcquireOp_ACQUIRE_OP_UNLOCK)
			assertError(t, resp, err)
		})

		t.Run("different key", func(t *testing.T) {
			t.Parallel()
			stream := newStream(t)
			_, err := lock(t, stream, uniq.Must(t), proto.AcquireOp_ACQUIRE_OP_LOCK)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := lock(t, stream, uniq.Must(t), proto.AcquireOp_ACQUIRE_OP_UNLOCK)
			assertError(t, resp, err)
		})
	})

	t.Run("set init result on unlocked key", func(t *testing.T) {
		t.Parallel()

		operations := []proto.AcquireOp{
			proto.AcquireOp_ACQUIRE_OP_SET_INIT_DONE,
			proto.AcquireOp_ACQUIRE_OP_SET_INIT_FAILED,
		}

		for _, op := range operations {
			op := op
			t.Run(op.String(), func(t *testing.T) {
				t.Parallel()

				t.Run("unlocked", func(t *testing.T) {
					t.Parallel()
					resp, err := lock(t, newStream(t), uniq.Must(t), op)
					assertError(t, resp, err)
				})

				t.Run("different key", func(t *testing.T) {
					t.Parallel()
					stream := newStream(t)
					_, err := lock(t, stream, uniq.Must(t), proto.AcquireOp_ACQUIRE_OP_INIT_LOCK)
					if err != nil {
						t.Fatal(err)
					}
					resp, err := lock(t, stream, uniq.Must(t), op)
					assertError(t, resp, err)
				})
			})
		}
	})

	t.Run("set init result on lock without init", func(t *testing.T) {
		t.Parallel()

		operations := []proto.AcquireOp{
			proto.AcquireOp_ACQUIRE_OP_SET_INIT_DONE,
			proto.AcquireOp_ACQUIRE_OP_SET_INIT_FAILED,
		}

		for _, op := range operations {
			op := op
			t.Run(op.String(), func(t *testing.T) {
				t.Parallel()
				stream := newStream(t)
				name := uniq.Must(t)
				_, err := lock(t, stream, name, proto.AcquireOp_ACQUIRE_OP_LOCK)
				if err != nil {
					t.Fatal(err)
				}
				resp, err := lock(t, stream, name, op)
				assertError(t, resp, err)
			})
		}
	})
}
