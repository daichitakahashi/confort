package keyedlock

import (
	"context"
	"sync"

	"golang.org/x/sync/semaphore"
)

type KeyedLock struct {
	m *sync.Map
}

func New() *KeyedLock {
	return &KeyedLock{
		m: new(sync.Map),
	}
}

const max = 1<<63 - 1

func (k *KeyedLock) Lock(ctx context.Context, key string) error {
	v, _ := k.m.LoadOrStore(key, semaphore.NewWeighted(max))
	return v.(*semaphore.Weighted).Acquire(ctx, max)
}

func (k *KeyedLock) Unlock(key string) {
	v, ok := k.m.Load(key)
	if !ok {
		panic("Unlock of unlocked mutex")
	}
	v.(*semaphore.Weighted).Release(max)
}

func (k *KeyedLock) TryLock(key string) bool {
	v, _ := k.m.LoadOrStore(key, semaphore.NewWeighted(max))
	return v.(*semaphore.Weighted).TryAcquire(max)
}

func (k *KeyedLock) Downgrade(key string) {
	v, ok := k.m.Load(key)
	if !ok {
		panic("Downgrade of unlocked mutex")
	}
	v.(*semaphore.Weighted).Release(max - 1)
}

func (k *KeyedLock) RLock(ctx context.Context, key string) error {
	v, _ := k.m.LoadOrStore(key, semaphore.NewWeighted(max))
	return v.(*semaphore.Weighted).Acquire(ctx, 1)
}

func (k *KeyedLock) RUnlock(key string) {
	v, ok := k.m.Load(key)
	if !ok {
		panic("RUnlock of unlocked mutex")
	}
	v.(*semaphore.Weighted).Release(1)
}
