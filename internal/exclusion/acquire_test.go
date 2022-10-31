package exclusion

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

func permutation(targets []string) [][]string {
	ret := make([][]string, 0, factorial(len(targets)))
	ret = append(ret, copySlice(targets))

	n := len(targets)
	p := make([]int, n+1)
	for i := 0; i < n+1; i++ {
		p[i] = i
	}
	for i := 1; i < n; {
		p[i]--
		j := 0
		if i%2 == 1 {
			j = p[i]
		}

		targets[i], targets[j] = targets[j], targets[i]
		ret = append(ret, copySlice(targets))
		for i = 1; p[i] == 0; i++ {
			p[i] = i
		}
	}
	return ret
}

func factorial(n int) int {
	ret := 1
	for i := 2; i <= n; i++ {
		ret *= i
	}
	return ret
}

func copySlice(nums []string) []string {
	return append([]string{}, nums...)
}

func randomTargets(t map[int][]string) []string {
	var targets []string
	for _, tt := range t {
		targets = tt
		break
	}

	n := rand.Intn(4)
	return targets[:n+1]
}

type randomBool struct {
	src       rand.Source
	cache     int64
	remaining int
}

func (b *randomBool) Bool() bool {
	if b.remaining == 0 {
		b.cache, b.remaining = b.src.Int63(), 63
	}

	result := b.cache&0x01 == 1
	b.cache >>= 1
	b.remaining--

	return result
}

func TestAcquirer(t *testing.T) {
	t.Parallel()

	list := permutation([]string{"a", "b", "c", "d", "e", "f"})
	targets := make(map[int][]string, len(list))
	for i, key := range list {
		targets[i] = key
	}
	r := &randomBool{
		src: rand.NewSource(time.Now().UnixNano()),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	a := NewAcquirer()
	eg, ctx := errgroup.WithContext(ctx)

	locker := NewKeyedLock()
	for i := 0; i < 1000; i++ {
		params := map[string]AcquireParam{}
		tt := randomTargets(targets)
		for _, target := range tt {
			target := target
			exclusive := r.Bool()

			params[target] = AcquireParam{
				Lock: func(ctx context.Context, notifyLock func()) (err error) {
					defer func() {
						if err != nil {
							return
						}
						notifyLock()
					}()
					if exclusive {
						return locker.Lock(ctx, target)
					}
					return locker.RLock(ctx, target)
				},
				Unlock: func() {
					if exclusive {
						locker.Unlock(target)
					} else {
						locker.RUnlock(target)
					}
				},
			}
		}

		eg.Go(func() error {
			err := a.Acquire(ctx, params)
			if err != nil {
				return err
			}
			time.Sleep(time.Millisecond)
			a.Release(params)
			return nil
		})
	}

	err := eg.Wait()
	if err != nil {
		t.Fatal(err)
	}
}
