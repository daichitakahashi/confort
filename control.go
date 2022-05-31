package confort

import (
	"fmt"
	"log"
	"testing"
)

type C struct {
	testing.TB
	cleanup []func()
}

// NewControl creates new instance of C, intended to use in TestMain.
// Instead of testing.T or testing.B, C satisfies testing.TB.
// It enables us to create container shared by package's unit tests.
//
// C.Fatal panics, so failure on creating container makes
// TestMain to stop before run any tests.
//
// Implementation of testing.TB is not perfect.
// If you want to use an unimplemented method, inject implementation
// by yourself.
func NewControl() (*C, func()) {
	c := &C{}
	term := func() {
		last := len(c.cleanup) - 1
		for i := range c.cleanup {
			c.cleanup[last-i]()
		}
	}
	return c, term
}

func (c *C) Cleanup(f func()) {
	c.cleanup = append(c.cleanup, f)
}

// func (c C) Error(args ...any) {}

// func (c C) Errorf(format string, args ...any) {}

// func (c C) Fail() {}

// func (c C) FailNow() {}

// func (c C) Failed() bool { return false }

func (c C) Fatal(args ...any) {
	panic(fmt.Sprint(args...))
}

func (c C) Fatalf(format string, args ...any) {
	panic(fmt.Sprintf(format, args...))
}

func (c C) Helper() {}

func (c C) Log(args ...any) {
	log.Println(args...)
}

func (c C) Logf(format string, args ...any) {
	log.Printf(format, args...)
}

// func (c C) Name() string {}

// func (c C) Setenv(key, value string) {}

// func (c C) Skip(args ...any) {}

// func (c C) SkipNow() {}

// func (c C) Skipf(format string, args ...any) {}

// func (c C) Skipped() bool {}

// func (c C) TempDir() string {}

var _ testing.TB = (*C)(nil)
