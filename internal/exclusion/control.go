package exclusion

import (
	"context"
	"fmt"
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
		err = func() (err error) {
			defer func() {
				r := recover()
				if r != nil {
					err = fmt.Errorf("%v", r)
				}
			}()
			return init()
		}()
		lock.SetInitResult(err == nil)
		if err != nil {
			lock.Release()
			return nil, err
		}
	}
	return lock.Release, nil
}

var _ Control = (*control)(nil)
