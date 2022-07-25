package beaconserver

import (
	"context"
	"io"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/proto/beacon"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type beaconServer struct {
	beacon.UnimplementedBeaconServiceServer
	ex        confort.ExclusionControl
	interrupt func() error
}

func (b *beaconServer) NamespaceLock(stream beacon.BeaconService_NamespaceLockServer) error {
	ctx := stream.Context()
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
			unlock, err = b.ex.NamespaceLock(ctx)
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

func (b *beaconServer) BuildLock(stream beacon.BeaconService_BuildLockServer) error {
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
			unlock, err = b.ex.BuildLock(ctx, key)
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

func (b *beaconServer) InitContainerLock(stream beacon.BeaconService_InitContainerLockServer) error {
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
			unlock, err = b.ex.InitContainerLock(ctx, key)
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
	var unlock func()
	var downgrade func() (func(), error)
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
		case beacon.AcquireOp_ACQUIRE_OP_LOCK:
			if unlock != nil {
				return status.Error(codes.InvalidArgument, "trying second lock")
			}
			key = k
			unlock, err = b.ex.AcquireContainerLock(ctx, key, true)
			if err != nil {
				return err
			}
			err = stream.Send(&beacon.LockResponse{
				State: beacon.LockState_LOCK_STATE_LOCKED,
			})
			if err != nil {
				return err
			}
		case beacon.AcquireOp_ACQUIRE_OP_UNLOCK:
			if unlock == nil || key != k {
				return status.Error(codes.InvalidArgument, "unlock on unlocked key")
			}
			unlock()
			key = ""
			unlock = nil
			downgrade = nil
			err = stream.Send(&beacon.LockResponse{
				State: beacon.LockState_LOCK_STATE_UNLOCKED,
			})
		case beacon.AcquireOp_ACQUIRE_OP_SHARED_LOCK:
			if unlock != nil {
				return status.Error(codes.InvalidArgument, "trying second lock")
			}
			key = k
			unlock, err = b.ex.AcquireContainerLock(ctx, key, false)
			if err != nil {
				return err
			}
			err = stream.Send(&beacon.LockResponse{
				State: beacon.LockState_LOCK_STATE_SHARED_LOCKED,
			})
			if err != nil {
				return err
			}
		case beacon.AcquireOp_ACQUIRE_OP_INIT_SHARED_LOCK:
			if unlock != nil {
				return status.Error(codes.InvalidArgument, "trying second lock")
			}
			key = k
			down, cancel, ok, err := b.ex.TryAcquireContainerInitLock(ctx, key)
			if err != nil {
				return err
			}
			if ok {
				downgrade = down
				unlock = cancel
				err = stream.Send(&beacon.LockResponse{
					State: beacon.LockState_LOCK_STATE_LOCKED,
				})
				if err != nil {
					return err
				}
			} else {
				unlock, err = down()
				if err != nil {
					cancel()
					return err
				}
				err = stream.Send(&beacon.LockResponse{
					State: beacon.LockState_LOCK_STATE_SHARED_LOCKED,
				})
				if err != nil {
					return err
				}
			}
		case beacon.AcquireOp_ACQUIRE_OP_DOWNGRADE:
			if downgrade == nil {
				return status.Error(codes.InvalidArgument, "downgrade on unlocked key")
			}
			_unlock, err := downgrade()
			if err != nil {
				return err
			}
			unlock = _unlock
			downgrade = nil
			err = stream.Send(&beacon.LockResponse{
				State: beacon.LockState_LOCK_STATE_SHARED_LOCKED,
			})
			if err != nil {
				return err
			}
		}
	}
}

func (b *beaconServer) Interrupt(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	err := b.interrupt()
	return &emptypb.Empty{}, err
}

var _ beacon.BeaconServiceServer = (*beaconServer)(nil)
