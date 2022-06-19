package beaconutil

import (
	"context"
	"io"
)

type StreamClient[T any] interface {
	Recv() (*T, error)
}

func ReceiveStream[T any](ctx context.Context, stream StreamClient[T], fn func(resp *T) (bool, error)) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		completed, err := fn(resp)
		if err != nil {
			return err
		}
		if completed {
			return nil
		}
	}
}
