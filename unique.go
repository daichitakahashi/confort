package confort

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/daichitakahashi/confort/proto/beacon"
	"github.com/lestrrat-go/option"
	"google.golang.org/grpc"
)

type Unique[T comparable] struct {
	g     uniqueValueGenerator[T]
	retry uint
}

type uniqueValueGenerator[T comparable] interface {
	generate(retry uint) (T, error)
}

type (
	UniqueOption interface {
		option.Interface
		unique()
	}
	identOptionRetry            struct{}
	identOptionGlobalUniqueness struct{}
	globalUniquenessOptions     struct {
		store string
		conn  *grpc.ClientConn
	}
	uniqueOption struct{ option.Interface }
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

func WithGlobalUniqueness(conn *grpc.ClientConn, beaconStore string) UniqueOption {
	return uniqueOption{
		Interface: option.New(identOptionGlobalUniqueness{}, globalUniquenessOptions{
			store: beaconStore,
			conn:  conn,
		}),
	}
}

func NewUnique[T comparable](f func() (T, error), opts ...UniqueOption) *Unique[T] {
	u := &Unique[T]{
		g: &generator[T]{
			f: f,
			m: make(map[T]struct{}),
		},
		retry: 10,
	}
	var options globalUniquenessOptions

	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionRetry{}:
			u.retry = opt.Value().(uint)
		case identOptionGlobalUniqueness{}:
			options = opt.Value().(globalUniquenessOptions)
		}
	}

	if options.store != "" && options.conn != nil {
		u.g = &globalGenerator[T]{
			f:     f,
			cli:   beacon.NewUniqueValueServiceClient(options.conn),
			store: options.store,
		}
	}

	return u
}

var ErrRetryable = errors.New("cannot create unique value but retryable")

func (u *Unique[T]) New() (T, error) {
	return u.g.generate(u.retry)
}

func (u *Unique[T]) Must(tb testing.TB) T {
	tb.Helper()

	v, err := u.g.generate(u.retry)
	if err != nil {
		tb.Fatal(err)
	}
	return v
}

var errFailedToGenerate = errors.New("cannot create new unique value")

type generator[T comparable] struct {
	f  func() (T, error)
	mu sync.Mutex
	m  map[T]struct{}
}

func (g *generator[T]) generate(retry uint) (zero T, _ error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := uint(0); i < retry; i++ {
		v, err := g.f()
		if err == ErrRetryable {
			continue
		} else if err != nil {
			return zero, err
		}
		if _, ok := g.m[v]; ok { // not unique, retry
			continue
		}
		g.m[v] = struct{}{}
		return v, nil
	}
	return zero, errFailedToGenerate
}

var _ uniqueValueGenerator[int] = (*generator[int])(nil)

type globalGenerator[T comparable] struct {
	f     func() (T, error)
	cli   beacon.UniqueValueServiceClient
	store string
}

func (g *globalGenerator[T]) generate(retry uint) (zero T, _ error) {
	ctx := context.Background()

	for i := uint(0); i < retry; i++ {
		v, err := g.f()
		if err == ErrRetryable {
			continue
		} else if err != nil {
			return zero, err
		}
		resp, err := g.cli.StoreUniqueValue(ctx, &beacon.StoreUniqueValueRequest{
			Store: g.store,
			Value: fmt.Sprint(v),
		})
		if err != nil {
			return zero, err
		} else if !resp.GetSucceeded() { // not unique, retry
			continue
		}
		return v, nil
	}
	return zero, errFailedToGenerate
}

var _ uniqueValueGenerator[int] = (*globalGenerator[int])(nil)

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
