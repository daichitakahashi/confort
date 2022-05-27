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
