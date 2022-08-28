package beaconserver

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func startServer(t *testing.T, hc HealthChecker) func(t *testing.T) *grpc.ClientConn {
	healthChecker := hc
	if healthChecker == nil {
		healthChecker = HealthCheckFunc(func(ctx context.Context) error {
			return nil
		})
	}

	srv := grpc.NewServer()
	Register(srv, func() error {
		return nil
	})
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = srv.Serve(ln)
		_ = ln.Close()
	}()
	t.Cleanup(func() {
		srv.Stop()
	})

	return func(t *testing.T) *grpc.ClientConn {
		t.Helper()

		conn, err := grpc.Dial(ln.Addr().String(), grpc.WithTransportCredentials(
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
