package exclusion

import (
	"context"
	"fmt"

	"github.com/daichitakahashi/confort/internal/beacon/proto"
	"go.uber.org/multierr"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Control interface {
	LockForNamespace(ctx context.Context) (func(), error)
	LockForBuild(ctx context.Context, image string) (func(), error)
	LockForContainerSetup(ctx context.Context, name string) (func(), error)
	LockForContainerUse(ctx context.Context, params map[string]ContainerUseParam) (unlock func(), err error)
}

type control struct {
	l *Locker
}

func NewControl() Control {
	return &control{
		l: NewLocker(),
	}
}

func (c *control) LockForNamespace(_ context.Context) (func(), error) {
	unlock := c.l.LockForNamespace()
	return unlock, nil
}

func (c *control) LockForBuild(ctx context.Context, image string) (func(), error) {
	return c.l.LockForBuild(ctx, image)
}

func (c *control) LockForContainerSetup(ctx context.Context, name string) (func(), error) {
	return c.l.LockForContainerSetup(ctx, name)
}

type ContainerUseParam struct {
	Exclusive bool
	Init      func(ctx context.Context) error
}

func (c *control) LockForContainerUse(ctx context.Context, params map[string]ContainerUseParam) (unlock func(), err error) {
	entries := map[string]*AcquireContainerLockEntry{}
	for name, param := range params {
		entries[name] = &AcquireContainerLockEntry{
			Exclusive: param.Exclusive,
			Init:      param.Init != nil,
		}
	}
	unlock, err = c.l.AcquireContainerLock(ctx, entries)
	if err != nil {
		return nil, err
	}
	var initErr error
	for name, entry := range entries {
		lock := entry.ContainerLock()
		if lock.InitAcquired() {
			if initErr == nil {
				initErr = initSafe(ctx, params[name].Init)
			}
			lock.SetInitResult(initErr == nil)
		}
	}
	if initErr != nil {
		unlock()
		return nil, initErr
	}
	return unlock, nil
}

func initSafe(ctx context.Context, init func(ctx context.Context) error) (err error) {
	defer func() {
		r := recover()
		if r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return init(ctx)
}

var _ Control = (*control)(nil)

type beaconControl struct {
	cli proto.BeaconServiceClient
}

func NewBeaconControl(cli proto.BeaconServiceClient) Control {
	return &beaconControl{
		cli: cli,
	}
}

func (b *beaconControl) LockForNamespace(ctx context.Context) (func(), error) {
	stream, err := b.cli.LockForNamespace(ctx)
	if err != nil {
		return nil, err
	}

	err = stream.Send(&proto.LockRequest{
		Operation: proto.LockOp_LOCK_OP_LOCK,
	})
	if err != nil {
		return nil, err
	}
	_, err = stream.Recv()
	if err != nil {
		return nil, err
	}
	return func() {
		err := stream.Send(&proto.LockRequest{
			Operation: proto.LockOp_LOCK_OP_UNLOCK,
		})
		_ = err // TODO: error handling
		_ = stream.CloseSend()
	}, nil
}

func (b *beaconControl) LockForBuild(ctx context.Context, image string) (func(), error) {
	stream, err := b.cli.LockForBuild(ctx)
	if err != nil {
		return nil, err
	}

	err = stream.Send(&proto.KeyedLockRequest{
		Key:       image,
		Operation: proto.LockOp_LOCK_OP_LOCK,
	})
	if err != nil {
		return nil, err
	}

	_, err = stream.Recv()
	if err != nil {
		return nil, err
	}
	return func() {
		err := stream.Send(&proto.KeyedLockRequest{
			Key:       image,
			Operation: proto.LockOp_LOCK_OP_UNLOCK,
		})
		_ = err // TODO: error handling
		_ = stream.CloseSend()
	}, nil
}

func (b *beaconControl) LockForContainerSetup(ctx context.Context, name string) (func(), error) {
	stream, err := b.cli.LockForContainerSetup(ctx)
	if err != nil {
		return nil, err
	}

	err = stream.Send(&proto.KeyedLockRequest{
		Key:       name,
		Operation: proto.LockOp_LOCK_OP_LOCK,
	})
	if err != nil {
		return nil, err
	}

	_, err = stream.Recv()
	if err != nil {
		return nil, err
	}
	return func() {
		err := stream.Send(&proto.KeyedLockRequest{
			Key:       name,
			Operation: proto.LockOp_LOCK_OP_UNLOCK,
		})
		_ = err // TODO: error handling
		_ = stream.CloseSend()
	}, nil
}

func (b *beaconControl) LockForContainerUse(ctx context.Context, params map[string]ContainerUseParam) (unlock func(), err error) {
	targets := map[string]*proto.AcquireLockParam{}
	for name, param := range params {
		var op proto.AcquireOp
		if param.Exclusive {
			if param.Init == nil {
				op = proto.AcquireOp_ACQUIRE_OP_LOCK
			} else {
				op = proto.AcquireOp_ACQUIRE_OP_INIT_LOCK
			}
		} else {
			if param.Init == nil {
				op = proto.AcquireOp_ACQUIRE_OP_SHARED_LOCK
			} else {
				op = proto.AcquireOp_ACQUIRE_OP_INIT_SHARED_LOCK
			}
		}
		targets[name] = &proto.AcquireLockParam{
			Operation: op,
		}
	}

	stream, err := b.cli.AcquireContainerLock(ctx)
	if err != nil {
		return nil, err
	}
	err = stream.Send(&proto.AcquireLockRequest{
		Param: &proto.AcquireLockRequest_Acquire{
			Acquire: &proto.AcquireLockAcquireParam{
				Targets: targets,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	for name, result := range resp.GetResults() {
		if result.GetAcquireInit() {
			initErr := initSafe(ctx, params[name].Init)
			err = stream.Send(&proto.AcquireLockRequest{
				Param: &proto.AcquireLockRequest_Init{
					Init: &proto.AcquireLockInitParam{
						Key:           name,
						InitSucceeded: initErr == nil,
					},
				},
			})
			if err != nil {
				return nil, multierr.Append(initErr, err)
			}
			if initErr != nil {
				return nil, multierr.Append(initErr, stream.CloseSend())
			}
		}
	}

	return func() {
		err := stream.Send(&proto.AcquireLockRequest{
			Param: &proto.AcquireLockRequest_Release{
				Release: &emptypb.Empty{},
			},
		})
		_ = err // TODO: error handling
		_ = stream.CloseSend()
	}, nil
}

var _ Control = (*beaconControl)(nil)
