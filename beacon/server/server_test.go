package server

import (
	"context"
	"testing"

	"github.com/daichitakahashi/confort"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func startServer(t *testing.T, ex confort.ExclusionControl, hc HealthChecker) func(t *testing.T) *grpc.ClientConn {
	ctx := context.Background()
	exclusionCtl := ex
	if exclusionCtl == nil {
		exclusionCtl = confort.NewExclusionControl()
	}
	healthChecker := hc
	if healthChecker == nil {
		healthChecker = HealthCheckFunc(func(ctx context.Context) error {
			return nil
		})
	}

	srv := New(":0", exclusionCtl, healthChecker) // use ephemeral port
	stop, err := srv.LaunchWorker(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stop(ctx)
	})

	return func(t *testing.T) *grpc.ClientConn {
		t.Helper()

		conn, err := grpc.Dial(srv.addr, grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = conn.Close()
		})
		return conn
	}
}
