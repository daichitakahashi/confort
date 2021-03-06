package confort

import (
	"context"
	"fmt"
	"sync"

	"github.com/daichitakahashi/confort/internal/keyedlock"
	"github.com/daichitakahashi/confort/proto/beacon"
)

type ExclusionControl interface {
	NamespaceLock(ctx context.Context) (func(), error)
	BuildLock(ctx context.Context, image string) (func(), error)
	InitContainerLock(ctx context.Context, name string) (func(), error)
	AcquireContainerLock(ctx context.Context, name string, exclusive bool) (func(), error)
	TryAcquireContainerInitLock(ctx context.Context, name string) (downgrade func() (func(), error), cancel func(), ok bool, _ error)
}

type exclusionControl struct {
	namespace sync.Mutex
	build     *keyedlock.KeyedLock
	init      *keyedlock.KeyedLock
	container *keyedlock.KeyedLock
}

func NewExclusionControl() *exclusionControl {
	return &exclusionControl{
		build:     keyedlock.New(),
		init:      keyedlock.New(),
		container: keyedlock.New(),
	}
}

func (c *exclusionControl) NamespaceLock(_ context.Context) (func(), error) {
	c.namespace.Lock()
	return c.namespace.Unlock, nil
}

func (c *exclusionControl) BuildLock(ctx context.Context, image string) (func(), error) {
	err := c.build.Lock(ctx, image)
	if err != nil {
		return nil, err
	}
	return func() {
		c.build.Unlock(image)
	}, nil
}

func (c *exclusionControl) InitContainerLock(ctx context.Context, name string) (func(), error) {
	err := c.init.Lock(ctx, name)
	if err != nil {
		return nil, err
	}
	return func() {
		c.init.Unlock(name)
	}, nil
}

func (c *exclusionControl) AcquireContainerLock(ctx context.Context, name string, exclusive bool) (func(), error) {
	if exclusive {
		err := c.container.Lock(ctx, name)
		if err != nil {
			return nil, err
		}
		return func() {
			c.container.Unlock(name)
		}, nil
	}

	err := c.container.RLock(ctx, name)
	if err != nil {
		return nil, err
	}
	return func() {
		c.container.RUnlock(name)
	}, nil
}

func (c *exclusionControl) TryAcquireContainerInitLock(ctx context.Context, name string) (downgrade func() (func(), error), cancel func(), ok bool, _ error) {
	ok = c.container.TryLock(name)
	if ok {
		return func() (func(), error) {
				c.container.Downgrade(name)
				return func() {
					c.container.RUnlock(name)
				}, nil
			}, func() {
				c.container.Unlock(name)
			}, ok, nil
	}
	return func() (func(), error) {
		err := c.container.RLock(ctx, name)
		if err != nil {
			return nil, err
		}
		return func() {
			c.container.RUnlock(name)
		}, nil
	}, func() {}, ok, nil
}

var _ ExclusionControl = (*exclusionControl)(nil)

type beaconControl struct {
	cli beacon.BeaconServiceClient
}

func NewBeaconControl(cli beacon.BeaconServiceClient) *beaconControl {
	return &beaconControl{
		cli: cli,
	}
}

func (b *beaconControl) NamespaceLock(ctx context.Context) (func(), error) {
	stream, err := b.cli.NamespaceLock(ctx)
	if err != nil {
		return nil, err
	}

	err = stream.Send(&beacon.LockRequest{
		Operation: beacon.LockOp_LOCK_OP_LOCK,
	})
	if err != nil {
		return nil, err
	}

	_, err = stream.Recv()
	if err != nil {
		return nil, err
	}
	return func() {
		err := stream.Send(&beacon.LockRequest{
			Operation: beacon.LockOp_LOCK_OP_UNLOCK,
		})
		_ = err // TODO: error handling
		_ = stream.CloseSend()
	}, nil
}

func (b *beaconControl) BuildLock(ctx context.Context, image string) (func(), error) {
	stream, err := b.cli.BuildLock(ctx)
	if err != nil {
		return nil, err
	}

	err = stream.Send(&beacon.KeyedLockRequest{
		Key:       image,
		Operation: beacon.LockOp_LOCK_OP_LOCK,
	})
	if err != nil {
		return nil, err
	}

	_, err = stream.Recv()
	if err != nil {
		return nil, err
	}
	return func() {
		err := stream.Send(&beacon.KeyedLockRequest{
			Key:       image,
			Operation: beacon.LockOp_LOCK_OP_UNLOCK,
		})
		_ = err // TODO: error handling
		_ = stream.CloseSend()
	}, nil
}

func (b *beaconControl) InitContainerLock(ctx context.Context, name string) (func(), error) {
	stream, err := b.cli.InitContainerLock(ctx)
	if err != nil {
		return nil, err
	}

	err = stream.Send(&beacon.KeyedLockRequest{
		Key:       name,
		Operation: beacon.LockOp_LOCK_OP_LOCK,
	})
	if err != nil {
		return nil, err
	}

	_, err = stream.Recv()
	if err != nil {
		return nil, err
	}
	return func() {
		err := stream.Send(&beacon.KeyedLockRequest{
			Key:       name,
			Operation: beacon.LockOp_LOCK_OP_UNLOCK,
		})
		_ = err // TODO: error handling
		_ = stream.CloseSend()
	}, nil
}

func (b *beaconControl) AcquireContainerLock(ctx context.Context, name string, exclusive bool) (func(), error) {
	stream, err := b.cli.AcquireContainerLock(ctx)
	if err != nil {
		return nil, err
	}

	operation := beacon.AcquireOp_ACQUIRE_OP_LOCK
	if !exclusive {
		operation = beacon.AcquireOp_ACQUIRE_OP_SHARED_LOCK
	}
	err = stream.Send(&beacon.AcquireLockRequest{
		Key:       name,
		Operation: operation,
	})
	if err != nil {
		return nil, err
	}

	_, err = stream.Recv()
	if err != nil {
		return nil, err
	}
	return func() {
		err := stream.Send(&beacon.AcquireLockRequest{
			Key:       name,
			Operation: beacon.AcquireOp_ACQUIRE_OP_UNLOCK,
		})
		_ = err // TODO: error handling
		_ = stream.CloseSend()
	}, nil
}

func (b *beaconControl) TryAcquireContainerInitLock(ctx context.Context, name string) (downgrade func() (func(), error), cancel func(), ok bool, _ error) {
	stream, err := b.cli.AcquireContainerLock(ctx)
	if err != nil {
		return nil, nil, false, err
	}

	err = stream.Send(&beacon.AcquireLockRequest{
		Key:       name,
		Operation: beacon.AcquireOp_ACQUIRE_OP_INIT_SHARED_LOCK,
	})
	if err != nil {
		return nil, nil, false, err
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, nil, false, err
	}

	var init bool
	unlock := func() {
		_ = stream.Send(&beacon.AcquireLockRequest{
			Key:       name,
			Operation: beacon.AcquireOp_ACQUIRE_OP_UNLOCK,
		})
	}

	switch resp.GetState() {
	case beacon.LockState_LOCK_STATE_LOCKED:
		downgrade = func() (func(), error) {
			err := stream.Send(&beacon.AcquireLockRequest{
				Key:       name,
				Operation: beacon.AcquireOp_ACQUIRE_OP_DOWNGRADE,
			})
			if err != nil {
				return nil, err
			}
			resp, err = stream.Recv()
			if err != nil {
				return nil, err
			}
			if resp.GetState() != beacon.LockState_LOCK_STATE_SHARED_LOCKED {
				return nil, fmt.Errorf("beaconControl: unexpected state(%s)", resp.GetState())
			}
			return unlock, nil
		}
		init = true
	case beacon.LockState_LOCK_STATE_SHARED_LOCKED:
		downgrade = func() (func(), error) {
			return unlock, nil
		}
	}
	return downgrade, unlock, init, nil
}

var _ ExclusionControl = (*beaconControl)(nil)
