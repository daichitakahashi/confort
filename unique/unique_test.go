package unique

import (
	"errors"
	"testing"

	"github.com/daichitakahashi/testingc"
)

func TestUnique_New(t *testing.T) {
	t.Parallel()

	var n int
	var err error
	unique := New(func() (int, error) {
		return n, err
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
	err = ErrRetryable
	_, err = unique.New()
	if err == nil {
		t.Fatal("error expected")
	}

	// error
	e := errors.New("ERROR")
	err = e
	_, err = unique.New()
	if err != e {
		t.Fatalf("unexpected error: want %q, got %q", e, err)
	}

	n = 1
	err = nil
	v = unique.Must(t)
	if v != 1 {
		t.Fatalf("unexpected value: want %d, got %d", 1, v)
	}
}

func TestUniqueString(t *testing.T) {
	t.Parallel()

	unique := String(1, WithRetry(999))

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

	// only coverage matters
	opt.unique()

	var maxUint uint
	maxUint--
	v := opt.Value().(uint)
	if maxUint != v {
		t.Fatalf("expected: %d, actual: %d", maxUint, v)
	}
}
