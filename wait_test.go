package confort

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestCheckLogOccurrence(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	f := &FetcherMock{
		LogFunc: func(ctx context.Context) (io.ReadCloser, error) {
			var log string

			d, _ := ctx.Deadline()
			remain := d.Sub(time.Now())
			if remain < time.Second {
				log += "creation completed\n"
			}
			if remain < 500*time.Millisecond {
				log += "initialization completed\n"
			}

			return io.NopCloser(strings.NewReader(log)), nil
		},
	}

	checker := CheckLogOccurrence("completed", 2)

	time.Sleep(time.Second)
	ok, err := checker(ctx, f)
	if err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("unexpected complete")
	}

	time.Sleep(500 * time.Millisecond)
	ok, err = checker(ctx, f)
	if err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatal("expected to be completed")
	}
}
