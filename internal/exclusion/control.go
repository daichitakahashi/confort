package exclusion

import (
	"context"
	"fmt"

	"github.com/daichitakahashi/confort/internal/beacon/proto"
	"go.uber.org/multierr"
)

type Control interface {
	LockForNamespace(ctx context.Context) (func(), error)
	LockForBuild(ctx context.Context, image string) (func(), error)
	LockForContainerSetup(ctx context.Context, name string) (func(), error)
	LockForContainerUse(ctx context.Context, name string, exclusive bool, init func() error) (func(), error)
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

func (c *control) LockForContainerUse(ctx context.Context, name string, exclusive bool, init func() error) (unlock func(), err error) {
	lock, err := c.l.AcquireContainerLock(ctx, name, exclusive, init != nil)
	if err != nil {
		return nil, err
	}
	if lock.InitAcquired() {
		err = initSafe(init)
		lock.SetInitResult(err == nil)
		if err != nil {
			lock.Release()
			return nil, err
		}
	}
	return lock.Release, nil
}

func initSafe(init func() error) (err error) {
	defer func() {
		r := recover()
		if r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return init()
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

func (b *beaconControl) LockForContainerUse(ctx context.Context, name string, exclusive bool, init func() error) (func(), error) {
	var op proto.AcquireOp
	if exclusive {
		if init == nil {
			op = proto.AcquireOp_ACQUIRE_OP_LOCK
		} else {
			op = proto.AcquireOp_ACQUIRE_OP_INIT_LOCK
		}
	} else {
		if init == nil {
			op = proto.AcquireOp_ACQUIRE_OP_SHARED_LOCK
		} else {
			op = proto.AcquireOp_ACQUIRE_OP_INIT_SHARED_LOCK
		}
	}

	stream, err := b.cli.AcquireContainerLock(ctx)
	if err != nil {
		return nil, err
	}
	err = stream.Send(&proto.AcquireLockRequest{
		Key:       name,
		Operation: op,
	})
	if err != nil {
		return nil, err
	}
	resp, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	if resp.GetAcquireInit() {
		initErr := initSafe(init)
		op := proto.AcquireOp_ACQUIRE_OP_SET_INIT_DONE
		if initErr != nil {
			op = proto.AcquireOp_ACQUIRE_OP_SET_INIT_FAILED
		}
		err = stream.Send(&proto.AcquireLockRequest{
			Key:       name,
			Operation: op,
		})
		if err != nil {
			return nil, multierr.Append(initErr, err)
		}
		_, err = stream.Recv()
		if err != nil {
			return nil, multierr.Append(initErr, err)
		}
		if initErr != nil {
			return nil, initErr
		}
	}
	return func() {
		err := stream.Send(&proto.AcquireLockRequest{
			Key:       name,
			Operation: proto.AcquireOp_ACQUIRE_OP_UNLOCK,
		})
		_ = err // TODO: error handling
		_ = stream.CloseSend()
	}, nil
}

var _ Control = (*beaconControl)(nil)
