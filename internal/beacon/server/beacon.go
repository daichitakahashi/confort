package server

import (
	"context"
	"io"

	"github.com/daichitakahashi/confort/internal/beacon/proto"
	"github.com/daichitakahashi/confort/internal/exclusion"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type beaconServer struct {
	proto.UnimplementedBeaconServiceServer
	l         *exclusion.Locker
	interrupt func() error
}

func (b *beaconServer) LockForNamespace(stream proto.BeaconService_LockForNamespaceServer) error {
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
		case proto.LockOp_LOCK_OP_LOCK:
			if unlock != nil {
				return status.Error(codes.InvalidArgument, "trying second lock")
			}
			unlock = b.l.LockForNamespace()
			err = stream.Send(&proto.LockResponse{
				State: proto.LockState_LOCK_STATE_LOCKED,
			})
			if err != nil {
				return err
			}
		case proto.LockOp_LOCK_OP_UNLOCK:
			if unlock == nil {
				return status.Error(codes.InvalidArgument, "unlock on unlocked")
			}
			unlock()
			unlock = nil
			err = stream.Send(&proto.LockResponse{
				State: proto.LockState_LOCK_STATE_UNLOCKED,
			})
			if err != nil {
				return err
			}
		}
	}
}

func (b *beaconServer) LockForBuild(stream proto.BeaconService_LockForBuildServer) error {
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
		case proto.LockOp_LOCK_OP_LOCK:
			if unlock != nil {
				return status.Error(codes.InvalidArgument, "trying second lock")
			}
			key = k
			unlock, err = b.l.LockForBuild(ctx, key)
			if err != nil {
				return err
			}
			err = stream.Send(&proto.LockResponse{
				State: proto.LockState_LOCK_STATE_LOCKED,
			})
			if err != nil {
				return err
			}
		case proto.LockOp_LOCK_OP_UNLOCK:
			if unlock == nil || k != key {
				return status.Error(codes.InvalidArgument, "unlock on unlocked key")
			}
			unlock()
			key = ""
			unlock = nil
			err = stream.Send(&proto.LockResponse{
				State: proto.LockState_LOCK_STATE_UNLOCKED,
			})
			if err != nil {
				return err
			}
		}
	}
}

func (b *beaconServer) LockForContainerSetup(stream proto.BeaconService_LockForContainerSetupServer) error {
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
		case proto.LockOp_LOCK_OP_LOCK:
			if unlock != nil {
				return status.Error(codes.InvalidArgument, "trying second lock")
			}
			key = k
			unlock, err = b.l.LockForContainerSetup(ctx, key)
			if err != nil {
				return err
			}
			err = stream.Send(&proto.LockResponse{
				State: proto.LockState_LOCK_STATE_LOCKED,
			})
			if err != nil {
				return err
			}
		case proto.LockOp_LOCK_OP_UNLOCK:
			if unlock == nil || k != key {
				return status.Error(codes.InvalidArgument, "unlock on unlocked key")
			}
			unlock()
			key = ""
			unlock = nil
			err = stream.Send(&proto.LockResponse{
				State: proto.LockState_LOCK_STATE_UNLOCKED,
			})
			if err != nil {
				return err
			}
		}
	}
}

func (b *beaconServer) AcquireContainerLock(stream proto.BeaconService_AcquireContainerLockServer) error {
	ctx := stream.Context()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		acquireParam, ok := req.GetParam().(*proto.AcquireLockRequest_Acquire)
		if !ok {
			return status.Error(codes.InvalidArgument, "invalid operation")
		}

		entries := map[string]*exclusion.AcquireContainerLockEntry{}
		for key, target := range acquireParam.Acquire.GetTargets() {
			if key == "" {
				return status.Error(codes.InvalidArgument, "empty key")
			}
			var exclusive, init bool
			switch target.GetOperation() {
			case proto.AcquireOp_ACQUIRE_OP_LOCK:
				exclusive = true
			case proto.AcquireOp_ACQUIRE_OP_INIT_LOCK:
				exclusive = true
				init = true
			case proto.AcquireOp_ACQUIRE_OP_SHARED_LOCK:
				exclusive = false
			case proto.AcquireOp_ACQUIRE_OP_INIT_SHARED_LOCK:
				exclusive = false
				init = true
			default:
				return status.Error(codes.InvalidArgument, "invalid operation")
			}
			entries[key] = &exclusion.AcquireContainerLockEntry{
				Exclusive: exclusive,
				Init:      init,
			}
		}
		release, err := b.l.AcquireContainerLock(ctx, entries)
		if err != nil {
			return err
		}
		initTargets := map[string]struct{}{}
		results := map[string]*proto.AcquireLockResult{}
		for key, entry := range entries {
			initAcquired := entry.ContainerLock().InitAcquired()
			var state proto.LockState
			if entry.Exclusive {
				state = proto.LockState_LOCK_STATE_LOCKED
			} else if initAcquired {
				state = proto.LockState_LOCK_STATE_LOCKED
			} else {
				state = proto.LockState_LOCK_STATE_SHARED_LOCKED
			}
			if initAcquired {
				initTargets[key] = struct{}{}
			}

			results[key] = &proto.AcquireLockResult{
				State:       state,
				AcquireInit: initAcquired,
			}
		}
		err = stream.Send(&proto.AcquireLockResponse{
			Results: results,
		})
		if err != nil {
			release()
			return err
		}

		var e error
	InitLoop:
		for i := 0; i < len(initTargets); i++ {
			req, err := stream.Recv()
			if err != nil {
				release()
				return err
			}

			switch param := req.GetParam().(type) {
			case *proto.AcquireLockRequest_Init:
				key := param.Init.GetKey()
				succeeded := param.Init.GetInitSucceeded()

				entries[key].ContainerLock().SetInitResult(succeeded)
				delete(initTargets, key)
				if !succeeded {
					break InitLoop
				}
			case *proto.AcquireLockRequest_Release:
				break InitLoop
			default:
				e = status.Error(codes.InvalidArgument, "invalid operation")
				break InitLoop
			}
		}
		var failed bool
		for key := range initTargets {
			entries[key].ContainerLock().SetInitResult(false)
			failed = true
		}
		if e != nil {
			release()
			return e
		} else if failed {
			release()
			continue
		}

		req, err = stream.Recv()
		if err != nil {
			release()
			return err
		}
		_, ok = req.GetParam().(*proto.AcquireLockRequest_Release)
		if !ok {
			return status.Error(codes.InvalidArgument, "invalid operation")
		}
		release()
	}
}

func (b *beaconServer) Interrupt(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	err := b.interrupt()
	return &emptypb.Empty{}, err
}

var _ proto.BeaconServiceServer = (*beaconServer)(nil)
