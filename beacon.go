package confort

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	health "google.golang.org/grpc/health/grpc_health_v1"
)

type Connection struct {
	conn *grpc.ClientConn
}

func (c *Connection) Enabled() bool {
	return c.conn != nil
}

func ConnectBeacon(tb testing.TB, ctx context.Context) *Connection {
	var conn *grpc.ClientConn
	beaconEndpoint := os.Getenv("CFT_BEACON_ENDPOINT")
	if beaconEndpoint != "" {
		var err error
		conn, err = grpc.DialContext(ctx, beaconEndpoint, grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		))
		if err != nil {
			tb.Fatalf("confort: %s", err)
		}
		tb.Cleanup(func() {
			err := conn.Close()
			if err != nil {
				tb.Logf("confort: %s", err)
			}
		})

		// health check
		hc := health.NewHealthClient(conn)
		var status health.HealthCheckResponse_ServingStatus
		ctl := backoff.Constant(
			backoff.WithInterval(time.Millisecond*100),
			backoff.WithMaxRetries(20),
		).Start(ctx)
		for backoff.Continue(ctl) {
			var resp *health.HealthCheckResponse
			resp, err = hc.Check(ctx, &health.HealthCheckRequest{
				Service: "beacon",
			})
			status = resp.GetStatus()
			if status == health.HealthCheckResponse_SERVING {
				break
			}
		}
		if err != nil {
			tb.Fatal(err)
		} else if status != health.HealthCheckResponse_SERVING {
			tb.Fatalf("unexpected service status %s", status)
		}
	}
	return &Connection{
		conn: conn,
	}
}
