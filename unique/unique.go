package unique

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/daichitakahashi/confort/internal/beacon"
	"github.com/daichitakahashi/confort/internal/beacon/proto"
	"github.com/lestrrat-go/option"
)

type Unique[T comparable] struct {
	g     uniqueValueGenerator[T]
	retry uint
}

type uniqueValueGenerator[T comparable] interface {
	generate(retry uint) (T, error)
}

type (
	Option interface {
		option.Interface
		unique() Option
	}
	identOptionRetry  struct{}
	identOptionBeacon struct{}
	uniqueOption      struct{ option.Interface }
)

func (o uniqueOption) unique() Option { return o }

// WithRetry configures the maximum number of retries for unique value generation.
func WithRetry(n uint) Option {
	if n == 0 {
		n--
	}
	return uniqueOption{
		Interface: option.New(identOptionRetry{}, n),
	}.unique()
}

// WithBeacon configures Unique to integrate with a starting beacon server.
// It enables us to generate unique values through all tests that reference
// the same beacon server and storeName.
//
// See confort.WithBeacon.
func WithBeacon(storeName string) Option {
	return uniqueOption{
		Interface: option.New(identOptionBeacon{}, storeName),
	}.unique()
}

// ErrRetryable indicates that the generation of a unique value has temporarily
// failed, but may succeed by retrying.
var ErrRetryable = errors.New("cannot create unique value but retryable")

// New creates unique value generator. Argument fn is an arbitrary generator function.
// When the generated value by fn is not unique or fn returns ErrRetryable, Unique retries.
// By default, Unique retries 10 times.
func New[T comparable](ctx context.Context, fn func() (T, error), opts ...Option) (*Unique[T], error) {
	u := &Unique[T]{
		g: &generator[T]{
			f: fn,
			m: make(map[T]struct{}),
		},
		retry: 10,
	}

	var storeName string
	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionRetry{}:
			u.retry = opt.Value().(uint)
		case identOptionBeacon{}:
			storeName = opt.Value().(string)
		}
	}

	if storeName != "" {
		conn, err := beacon.Connect(ctx)
		if err != nil {
			return nil, err
		}
		if conn.Enabled() {
			u.g = &globalGenerator[T]{
				f:     fn,
				cli:   proto.NewUniqueValueServiceClient(conn.Conn),
				store: storeName,
			}
		}
	}

	return u, nil
}

// Must is a helper that wraps a call to a function returning (*Unique[T], error)
// and panics if the error is non-nil. It is intended for use in variable initializations
// such as
//
//	var u = unique.Must(unique.String(context.Background(), 10))
func Must[T comparable](u *Unique[T], err error) *Unique[T] {
	if err != nil {
		panic(err)
	}
	return u
}

// New returns unique value.
func (u *Unique[T]) New() (T, error) {
	return u.g.generate(u.retry)
}

// Must returns unique value.
// If a unique value cannot be generated within the maximum number of retries,
// the test fails.
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
		if errors.Is(err, ErrRetryable) {
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
	cli   proto.UniqueValueServiceClient
	store string
}

func (g *globalGenerator[T]) generate(retry uint) (zero T, _ error) {
	ctx := context.Background()

	for i := uint(0); i < retry; i++ {
		v, err := g.f()
		if errors.Is(err, ErrRetryable) {
			continue
		} else if err != nil {
			return zero, err
		}
		resp, err := g.cli.StoreUniqueValue(ctx, &proto.StoreUniqueValueRequest{
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

// StringFunc is an n-digit random string generator.
// It uses upper/lower case alphanumeric characters.
func StringFunc(n int) func() (string, error) {
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

// String is a shorthand of New(StringFunc(n)).
func String(ctx context.Context, n int, opts ...Option) (*Unique[string], error) {
	return New(ctx, StringFunc(n), opts...)
}
