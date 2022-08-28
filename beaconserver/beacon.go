package beaconserver

import (
	"context"
	"io"

	"github.com/daichitakahashi/confort/internal/exclusion"
	"github.com/daichitakahashi/confort/proto/beacon"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type beaconServer struct {
	beacon.UnimplementedBeaconServiceServer
	l         *exclusion.Locker
	interrupt func() error
}

func (b *beaconServer) LockForNamespace(stream beacon.BeaconService_LockForNamespaceServer) error {
	var unlock func()
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		switch req.GetOperation() {
		case beacon.LockOp_LOCK_OP_LOCK:
			if unlock != nil {
				return status.Error(codes.InvalidArgument, "trying second lock")
			}
			unlock = b.l.LockForNamespace()
			err = stream.Send(&beacon.LockResponse{
				State: beacon.LockState_LOCK_STATE_LOCKED,
			})
			if err != nil {
				return err
			}
		case beacon.LockOp_LOCK_OP_UNLOCK:
			if unlock == nil {
				return status.Error(codes.InvalidArgument, "unlock on unlocked")
			}
			unlock()
			unlock = nil
			err = stream.Send(&beacon.LockResponse{
				State: beacon.LockState_LOCK_STATE_UNLOCKED,
			})
			if err != nil {
				return err
			}
		}
	}
}

func (b *beaconServer) LockForBuild(stream beacon.BeaconService_LockForBuildServer) error {
	ctx := stream.Context()
	var key string
	var unlock func()
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		k := req.GetKey()
		if k == "" {
			return status.Error(codes.InvalidArgument, "empty key")
		}

		switch req.GetOperation() {
		case beacon.LockOp_LOCK_OP_LOCK:
			if unlock != nil {
				return status.Error(codes.InvalidArgument, "trying second lock")
			}
			key = k
			unlock, err = b.l.LockForBuild(ctx, key)
			if err != nil {
				return err
			}
			err = stream.Send(&beacon.LockResponse{
				State: beacon.LockState_LOCK_STATE_LOCKED,
			})
			if err != nil {
				return err
			}
		case beacon.LockOp_LOCK_OP_UNLOCK:
			if unlock == nil || k != key {
				return status.Error(codes.InvalidArgument, "unlock on unlocked key")
			}
			unlock()
			key = ""
			unlock = nil
			err = stream.Send(&beacon.LockResponse{
				State: beacon.LockState_LOCK_STATE_UNLOCKED,
			})
			if err != nil {
				return err
			}
		}
	}
}

func (b *beaconServer) LockForContainerSetup(stream beacon.BeaconService_LockForContainerSetupServer) error {
	ctx := stream.Context()
	var key string
	var unlock func()
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		k := req.GetKey()
		if k == "" {
			return status.Error(codes.InvalidArgument, "empty key")
		}

		switch req.GetOperation() {
		case beacon.LockOp_LOCK_OP_LOCK:
			if unlock != nil {
				return status.Error(codes.InvalidArgument, "trying second lock")
			}
			key = k
			unlock, err = b.l.LockForContainerSetup(ctx, key)
			if err != nil {
				return err
			}
			err = stream.Send(&beacon.LockResponse{
				State: beacon.LockState_LOCK_STATE_LOCKED,
			})
			if err != nil {
				return err
			}
		case beacon.LockOp_LOCK_OP_UNLOCK:
			if unlock == nil || k != key {
				return status.Error(codes.InvalidArgument, "unlock on unlocked key")
			}
			unlock()
			key = ""
			unlock = nil
			err = stream.Send(&beacon.LockResponse{
				State: beacon.LockState_LOCK_STATE_UNLOCKED,
			})
			if err != nil {
				return err
			}
		}
	}
}

func (b *beaconServer) AcquireContainerLock(stream beacon.BeaconService_AcquireContainerLockServer) error {
	ctx := stream.Context()
	var key string
	var op beacon.AcquireOp = -1
	var lock *exclusion.ContainerLock
	defer func() {
		if lock != nil {
			lock.Release()
		}
	}()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		k := req.GetKey()
		if k == "" {
			return status.Error(codes.InvalidArgument, "empty key")
		}

		switch req.GetOperation() {
		case beacon.AcquireOp_ACQUIRE_OP_LOCK:
			if lock != nil {
				return status.Error(codes.InvalidArgument, "trying second lock")
			}
			key = k
			lock, err = b.l.AcquireContainerLock(ctx, key, true, false)
			if err != nil {
				return err
			}
			err = stream.Send(&beacon.AcquireLockResponse{
				State:       beacon.LockState_LOCK_STATE_LOCKED,
				AcquireInit: false,
			})
		case beacon.AcquireOp_ACQUIRE_OP_INIT_LOCK:
			if lock != nil {
				return status.Error(codes.InvalidArgument, "trying second lock")
			}
			key = k
			op = beacon.AcquireOp_ACQUIRE_OP_INIT_LOCK
			lock, err = b.l.AcquireContainerLock(ctx, key, true, true)
			if err != nil {
				return err
			}
			err = stream.Send(&beacon.AcquireLockResponse{
				State:       beacon.LockState_LOCK_STATE_LOCKED,
				AcquireInit: lock.InitAcquired(),
			})
		case beacon.AcquireOp_ACQUIRE_OP_SHARED_LOCK:
			if lock != nil {
				return status.Error(codes.InvalidArgument, "trying second lock")
			}
			key = k
			lock, err = b.l.AcquireContainerLock(ctx, key, false, false)
			if err != nil {
				return err
			}
			err = stream.Send(&beacon.AcquireLockResponse{
				State:       beacon.LockState_LOCK_STATE_SHARED_LOCKED,
				AcquireInit: false,
			})
		case beacon.AcquireOp_ACQUIRE_OP_INIT_SHARED_LOCK:
			if lock != nil {
				return status.Error(codes.InvalidArgument, "trying second lock")
			}
			key = k
			op = beacon.AcquireOp_ACQUIRE_OP_INIT_SHARED_LOCK
			lock, err = b.l.AcquireContainerLock(ctx, key, false, true)
			if err != nil {
				return err
			}
			err = stream.Send(&beacon.AcquireLockResponse{
				State:       beacon.LockState_LOCK_STATE_LOCKED,
				AcquireInit: lock.InitAcquired(),
			})
		case beacon.AcquireOp_ACQUIRE_OP_UNLOCK:
			if lock == nil || key != k {
				return status.Error(codes.InvalidArgument, "unlock on unlocked key")
			}
			lock.Release()
			key = ""
			op = -1
			lock = nil
			err = stream.Send(&beacon.AcquireLockResponse{
				State:       beacon.LockState_LOCK_STATE_UNLOCKED,
				AcquireInit: false,
			})
		case beacon.AcquireOp_ACQUIRE_OP_SET_INIT_DONE:
			if lock == nil || key != k {
				return status.Error(codes.InvalidArgument, "set init result on unlocked key")
			}
			if !lock.InitAcquired() {
				return status.Error(codes.InvalidArgument, "set init result on lock without init")
			}
			lock.SetInitResult(true)
			state := beacon.LockState_LOCK_STATE_LOCKED
			if op == beacon.AcquireOp_ACQUIRE_OP_INIT_SHARED_LOCK {
				state = beacon.LockState_LOCK_STATE_SHARED_LOCKED
			}
			err = stream.Send(&beacon.AcquireLockResponse{
				State:       state,
				AcquireInit: false,
			})
		case beacon.AcquireOp_ACQUIRE_OP_SET_INIT_FAILED:
			if lock == nil || key != k {
				return status.Error(codes.InvalidArgument, "set init result on unlocked key")
			}
			if !lock.InitAcquired() {
				return status.Error(codes.InvalidArgument, "set init result on lock without init")
			}
			lock.SetInitResult(false)
			lock.Release()
			key = ""
			op = -1
			lock = nil
			err = stream.Send(&beacon.AcquireLockResponse{
				State:       beacon.LockState_LOCK_STATE_UNLOCKED,
				AcquireInit: false,
			})
		}
		if err != nil {
			return err
		}
	}
}

func (b *beaconServer) Interrupt(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	err := b.interrupt()
	return &emptypb.Empty{}, err
}

var _ beacon.BeaconServiceServer = (*beaconServer)(nil)
