package exclusion

import (
	"container/list"
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
)

type AcquireParam struct {
	Lock   func(ctx context.Context, notifyLock func()) error
	Unlock func()
}

type Acquirer struct {
	c *acquireController
}

func NewAcquirer() *Acquirer {
	return &Acquirer{
		c: &acquireController{
			m:     new(sync.Mutex),
			queue: list.New(),
		},
	}
}

func (a *Acquirer) Acquire(ctx context.Context, params map[string]AcquireParam) error {
	set := map[string]struct{}{}
	for key := range params {
		set[key] = struct{}{}
	}
	e, proceed := a.c.accept(set)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-proceed:
	}

	eg, ctx := errgroup.WithContext(ctx)
	locked := make([]*AcquireParam, len(params))
	var i int
	for key, param := range params {
		idx := i
		key := key
		param := param
		eg.Go(func() error {
			err := param.Lock(ctx, func() {
				a.c.removeLockedKey(e, key)
			})
			if err != nil {
				return err
			}
			locked[idx] = &param
			return nil
		})
		i++
	}
	err := eg.Wait()
	if err != nil {
		a.c.remove(e)

		var wg sync.WaitGroup
		for _, lockedParam := range locked {
			if lockedParam == nil {
				continue
			}
			p := lockedParam
			wg.Add(1)
			go func() {
				defer wg.Done()
				p.Unlock()
			}()
		}
		wg.Wait()
		return err
	}
	return nil
}

func (a *Acquirer) Release(params map[string]AcquireParam) {
	for _, p := range params {
		p.Unlock()
	}
}

type acquireSet struct {
	set     map[string]struct{}
	proceed chan struct{}
}

type acquireController struct {
	m     *sync.Mutex
	queue *list.List
}

type entry = list.Element

func (q *acquireController) accept(set map[string]struct{}) (*entry, chan struct{}) {
	q.m.Lock()
	defer q.m.Unlock()

	s := &acquireSet{
		set:     set,
		proceed: make(chan struct{}),
	}
	e := q.queue.PushBack(s)
	q.proceed()
	return e, s.proceed
}

func (q *acquireController) removeLockedKey(e *entry, key string) {
	q.m.Lock()
	defer q.m.Unlock()

	s := e.Value.(*acquireSet)
	if len(s.set) > 1 {
		delete(s.set, key)
	} else {
		// if all lock has acquired, remove entry from queue
		q.queue.Remove(e)
	}

	q.proceed()
}

func (q *acquireController) remove(e *entry) {
	q.m.Lock()
	defer q.m.Unlock()
	q.queue.Remove(e)

	q.proceed()
}

func (q *acquireController) proceed() {
	if q.queue.Len() == 0 {
		return
	}

	// check queue
	waitSet := map[string]struct{}{}
	for e := q.queue.Front(); e != nil; e = e.Next() {
		s := e.Value.(*acquireSet)
		select {
		case <-s.proceed:
			// lock already started
			for key := range s.set {
				waitSet[key] = struct{}{}
			}
		default:
			var pending bool
			for key := range s.set {
				_, found := waitSet[key]
				if found {
					pending = true
				} else {
					waitSet[key] = struct{}{}
				}
			}
			if !pending {
				close(s.proceed)
			}
		}
	}
}
