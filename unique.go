package confort

import (
	"errors"
	"math/rand"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/lestrrat-go/option"
)

type Unique[T comparable] struct {
	f     func() (T, error)
	mu    sync.Mutex
	m     map[T]struct{}
	retry uint
}

type (
	UniqueOption interface {
		option.Interface
		unique()
	}
	identOptionRetry struct{}
	uniqueOption     struct{ option.Interface }
)

func (uniqueOption) unique() {}

func WithRetry(n uint) UniqueOption {
	if n == 0 {
		n--
	}
	return uniqueOption{
		Interface: option.New(identOptionRetry{}, n),
	}
}

func NewUnique[T comparable](f func() (T, error), opts ...UniqueOption) *Unique[T] {
	u := &Unique[T]{
		f:     f,
		m:     map[T]struct{}{},
		retry: 10,
	}

	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionRetry{}:
			u.retry = opt.Value().(uint)
		}
	}

	return u
}

var ErrRetryable = errors.New("cannot create unique value but retryable")

func (u *Unique[T]) New() (T, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	var zero T
	for i := uint(0); i < u.retry; i++ {
		v, err := u.f()
		if err == ErrRetryable {
			continue
		} else if err != nil {
			return zero, err
		}
		if _, ok := u.m[v]; ok {
			continue
		}
		u.m[v] = struct{}{}
		return v, nil
	}
	return zero, errors.New("cannot create new unique value")
}

func (u *Unique[T]) Must(tb testing.TB) T {
	tb.Helper()

	v, err := u.New()
	if err != nil {
		tb.Fatal(err)
	}
	return v
}

const (
	letters       = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	letterIdxBits = 6
	letterIdxMask = 1<<letterIdxBits - 1
	letterIdxMax  = 63 / letterIdxBits
)

func UniqueStringFunc(n int) func() (string, error) {
	randSrc := rand.NewSource(time.Now().UnixNano())
	return func() (string, error) {
		b := make([]byte, n)
		cache, remain := randSrc.Int63(), letterIdxMax
		for i := n - 1; i >= 0; {
			if remain == 0 {
				cache, remain = randSrc.Int63(), letterIdxMax
			}
			idx := int(cache & letterIdxMask)
			if idx < len(letters) {
				b[i] = letters[idx]
				i--
			}
			cache >>= letterIdxBits
			remain--
		}
		return *(*string)(unsafe.Pointer(&b)), nil
	}
}

func UniqueString(n int, opts ...UniqueOption) *Unique[string] {
	return NewUnique(UniqueStringFunc(n), opts...)
}
