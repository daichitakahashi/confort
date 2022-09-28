package server

import (
	"context"
	"testing"
	"time"

	health "google.golang.org/grpc/health/grpc_health_v1"
)

func TestHealthServer_Check(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	connect := startServer(t, nil)
	cli := health.NewHealthClient(connect(t))

	for i := 0; i < 10; i++ {
		resp, err := cli.Check(ctx, &health.HealthCheckRequest{
			Service: "test",
		})
		if err != nil {
			t.Fatal(err)
		}
		status := resp.GetStatus()
		if status != health.HealthCheckResponse_SERVING {
			t.Log(status)
		} else {
			time.Sleep(100 * time.Millisecond)
			break
		}
	}
}
