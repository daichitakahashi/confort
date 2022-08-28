package exclusion

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/daichitakahashi/oncewait"
)

type Locker struct {
	namespace      sync.Mutex
	build          *KeyedLock
	containerSetup *KeyedLock
	containerUse   *KeyedLock
	once           *oncewait.Factory
}

func NewLocker() *Locker {
	return &Locker{
		build:          NewKeyedLock(),
		containerSetup: NewKeyedLock(),
		containerUse:   NewKeyedLock(),
		once:           &oncewait.Factory{},
	}
}

func (l *Locker) LockForNamespace() func() {
	l.namespace.Lock()
	return l.namespace.Unlock
}

func (l *Locker) LockForBuild(ctx context.Context, image string) (func(), error) {
	err := l.build.Lock(ctx, image)
	if err != nil {
		return nil, err
	}
	return func() {
		l.build.Unlock(image)
	}, nil
}

func (l *Locker) LockForContainerSetup(ctx context.Context, name string) (func(), error) {
	err := l.containerSetup.Lock(ctx, name)
	if err != nil {
		return nil, err
	}
	return func() {
		l.containerSetup.Unlock(name)
	}, nil
}

type ContainerLock struct {
	l          *KeyedLock
	once       *oncewait.Factory
	name       string
	init       bool
	downgraded int32
}

func (l *ContainerLock) InitAcquired() bool {
	return l.init
}

func (l *ContainerLock) SetInitResult(ok bool) {
	if l.init {
		if ok && atomic.CompareAndSwapInt32(&l.downgraded, 0, 1) {
			l.l.Downgrade(l.name)
		} else if !ok {
			l.once.Refresh(l.name)
		}
	}
}

func (l *ContainerLock) Release() {
	if atomic.LoadInt32(&l.downgraded) == 0 { // exclusive
		l.l.Unlock(l.name)
	} else { // shared/downgraded
		l.l.RUnlock(l.name)
	}
}

func (l *Locker) AcquireContainerLock(ctx context.Context, name string, exclusive, init bool) (*ContainerLock, error) {
	var err error
	if init {
		var acquireInit bool
		l.once.Get(name).Do(func() {
			err = l.containerUse.Lock(ctx, name) // exclusive lock
			if err != nil {
				l.once.Refresh(name)
				return
			}
			acquireInit = true
		})
		if err != nil {
			return nil, err
		}
		if acquireInit {
			return &ContainerLock{
				l:          l.containerUse,
				once:       l.once,
				name:       name,
				init:       true,
				downgraded: 0,
			}, nil
		}
	}

	// no init
	var downgraded int32
	if exclusive {
		err = l.containerUse.Lock(ctx, name)
	} else {
		err = l.containerUse.RLock(ctx, name)
		downgraded = 1
	}
	if err != nil {
		return nil, err
	}

	return &ContainerLock{
		l:          l.containerUse,
		once:       l.once,
		name:       name,
		init:       false,
		downgraded: downgraded,
	}, nil
}
