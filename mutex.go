package confort

import (
	"context"
	"sync"

	"golang.org/x/sync/semaphore"
)

type keyedLock struct {
	m *sync.Map
}

func newKeyedLock() *keyedLock {
	return &keyedLock{
		m: new(sync.Map),
	}
}

const max = 1<<63 - 1

func (k *keyedLock) Lock(ctx context.Context, key string) error {
	v, _ := k.m.LoadOrStore(key, semaphore.NewWeighted(max))
	return v.(*semaphore.Weighted).Acquire(ctx, max)
}

func (k *keyedLock) Unlock(key string) {
	v, ok := k.m.Load(key)
	if !ok {
		panic("Unlock of unlocked mutex")
	}
	v.(*semaphore.Weighted).Release(max)
}

func (k *keyedLock) TryLock(key string) bool {
	v, _ := k.m.LoadOrStore(key, semaphore.NewWeighted(max))
	return v.(*semaphore.Weighted).TryAcquire(max)
}

func (k *keyedLock) Downgrade(key string) {
	v, ok := k.m.Load(key)
	if !ok {
		panic("Downgrade of unlocked mutex")
	}
	v.(*semaphore.Weighted).Release(max - 1)
}

func (k *keyedLock) RLock(ctx context.Context, key string) error {
	v, _ := k.m.LoadOrStore(key, semaphore.NewWeighted(max))
	return v.(*semaphore.Weighted).Acquire(ctx, 1)
}

func (k *keyedLock) RUnlock(key string) {
	v, ok := k.m.Load(key)
	if !ok {
		panic("RUnlock of unlocked mutex")
	}
	v.(*semaphore.Weighted).Release(1)
}

type ExclusionControl interface {
	NamespaceLock(ctx context.Context) (func(), error)
	BuildLock(ctx context.Context, image string) (func(), error)
	InitContainerLock(ctx context.Context, name string) (func(), error)
	AcquireContainerLock(ctx context.Context, name string, exclusive bool, initFunc InitFunc) (func(), error)
}

type InitFunc func(ctx context.Context) error

type exclusionControl struct {
	namespace sync.Mutex
	build     *keyedLock
	init      *keyedLock
	container *keyedLock
}

func NewExclusionControl() ExclusionControl {
	return &exclusionControl{
		build:     newKeyedLock(),
		init:      newKeyedLock(),
		container: newKeyedLock(),
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

func (c *exclusionControl) AcquireContainerLock(ctx context.Context, name string, exclusive bool, initFunc InitFunc) (func(), error) {
	if exclusive {
		err := c.container.Lock(ctx, name)
		if err != nil {
			return nil, err
		}
		return func() {
			c.container.Unlock(name)
		}, nil
	}
	if initFunc != nil {
		if c.container.TryLock(name) {
			err := initFunc(ctx)
			if err != nil {
				c.container.Unlock(name)
				return nil, err
			}
			c.container.Downgrade(name)
		}
	} else {
		err := c.container.RLock(ctx, name)
		if err != nil {
			return nil, err
		}
	}
	return func() {
		c.container.RUnlock(name)
	}, nil
}

var _ ExclusionControl = (*exclusionControl)(nil)
