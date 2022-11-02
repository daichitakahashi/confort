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
	acquirer       *Acquirer
	once           *oncewait.Factory
}

func NewLocker() *Locker {
	return &Locker{
		build:          NewKeyedLock(),
		containerSetup: NewKeyedLock(),
		containerUse:   NewKeyedLock(),
		acquirer:       NewAcquirer(),
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
	exclusive  bool
	downgraded int32
}

func (l *ContainerLock) InitAcquired() bool {
	return l.init
}

func (l *ContainerLock) SetInitResult(ok bool) {
	if l.init {
		if ok {
			if !l.exclusive && atomic.CompareAndSwapInt32(&l.downgraded, 0, 1) {
				l.l.Downgrade(l.name)
			}
		} else {
			l.once.Refresh(l.name)
		}
	}
}

func (l *ContainerLock) Release() {
	if l.exclusive || atomic.LoadInt32(&l.downgraded) == 0 { // exclusive
		l.l.Unlock(l.name)
	} else { // shared/downgraded
		l.l.RUnlock(l.name)
	}
}

type AcquireContainerLockEntry struct {
	Exclusive bool
	Init      bool

	l  *Locker
	cl *ContainerLock
	p  AcquireParam
}

func (p *AcquireContainerLockEntry) init(l *Locker, name string) {
	p.l = l

	p.p = AcquireParam{
		Lock: func(ctx context.Context, notifyLock func()) error {
			var err error
			if p.Init {
				var initAcquired bool
				l.once.Get(name).Do(func() {
					err = l.containerUse.Lock(ctx, name) // exclusive lock
					if err != nil {
						l.once.Refresh(name)
						return
					}
					initAcquired = true
				})
				if err != nil {
					return err
				}
				if initAcquired {
					notifyLock()
					p.cl = &ContainerLock{
						l:          l.containerUse,
						once:       l.once,
						name:       name,
						init:       true,
						exclusive:  p.Exclusive,
						downgraded: 0,
					}
					return nil
				}
			}

			// no init
			var downgraded int32
			if p.Exclusive {
				err = l.containerUse.Lock(ctx, name)
			} else {
				err = l.containerUse.RLock(ctx, name)
				downgraded = 1
			}
			if err != nil {
				return err
			}
			notifyLock()

			p.cl = &ContainerLock{
				l:          l.containerUse,
				once:       l.once,
				name:       name,
				init:       false,
				exclusive:  p.Exclusive,
				downgraded: downgraded,
			}
			return nil
		},
		Unlock: func() {
			if p.cl != nil {
				p.cl.Release()
			}
		},
	}
}

func (p *AcquireContainerLockEntry) ContainerLock() *ContainerLock {
	return p.cl
}

func (l *Locker) AcquireContainerLock(ctx context.Context, entries map[string]*AcquireContainerLockEntry) (func(), error) {
	p := map[string]AcquireParam{}
	for name, e := range entries {
		e.init(l, name)
		p[name] = e.p
	}
	err := l.acquirer.Acquire(ctx, p)
	if err != nil {
		return nil, err
	}
	return func() {
		l.acquirer.Release(p)
	}, nil
}
