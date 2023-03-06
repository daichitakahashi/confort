package unique

import (
	"context"
	"errors"
	"testing"

	"github.com/daichitakahashi/testingc"
)

func TestUnique_New(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var n int
	var genErr error
	unique, err := New(ctx, func() (int, error) {
		return n, genErr
	})

	v := unique.Must(t)
	if v != 0 {
		t.Fatalf("unexpected value: want %d, got %d", 0, v)
	}

	// cannot get unique value
	n = v
	_, err = unique.New()
	if err == nil {
		t.Fatal("error expected")
	}

	// force retry using ErrRetryable
	genErr = ErrRetryable
	_, err = unique.New()
	if err == nil {
		t.Fatal("error expected")
	}

	// error
	e := errors.New("ERROR")
	genErr = e
	_, err = unique.New()
	if err != e {
		t.Fatalf("unexpected error: want %q, got %q", e, err)
	}

	n = 1
	genErr = nil
	v = unique.Must(t)
	if v != 1 {
		t.Fatalf("unexpected value: want %d, got %d", 1, v)
	}
}

func TestUniqueString(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	unique, err := String(ctx, 1, WithRetry(999))
	if err != nil {
		t.Fatal(err)
	}

	m := map[string]bool{}
	for i := 0; i < len(letters); i++ {
		v := unique.Must(t)
		if m[v] {
			t.Fatalf("value %q already exists", v)
		}
		m[v] = true
	}

	result := testingc.Test(func(t *testingc.T) {
		unique.Must(t)
	})
	if !result.Failed() {
		t.Fatalf("unexpected success")
	}
}

func TestWithRetry_ZeroValue(t *testing.T) {
	t.Parallel()
	opt := WithRetry(0)

	var maxUint uint
	maxUint--
	v := opt.Value().(uint)
	if maxUint != v {
		t.Fatalf("expected: %d, actual: %d", maxUint, v)
	}
}
