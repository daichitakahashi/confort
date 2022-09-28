package beacon

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	health "google.golang.org/grpc/health/grpc_health_v1"
)

var (
	connOnce  sync.Once
	connected = make(chan struct{})
	conn      *Connection
)

type Connection struct {
	Conn *grpc.ClientConn
	Addr string
}

func (c *Connection) Enabled() bool {
	return c != nil && c.Conn != nil
}

func Connect(tb testing.TB, ctx context.Context) *Connection {
	tb.Helper()

	connOnce.Do(func() {
		conn = connect(tb, ctx)
		close(connected)
	})
	<-connected
	return conn
}

func connect(tb testing.TB, ctx context.Context) *Connection {
	tb.Helper()

	addr, err := Address(ctx, LockFilePath())
	if err != nil {
		tb.Logf("beacon: %s", err)
	}
	if addr == "" {
		tb.Log("beacon: cannot get beacon address")
		return &Connection{}
	}

	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(
		insecure.NewCredentials(),
	))
	if err != nil {
		tb.Fatalf("beacon: %s", err)
	}
	tb.Cleanup(func() {
		err := conn.Close()
		if err != nil {
			tb.Logf("beacon: %s", err)
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
		tb.Fatalf("beacon: unexpected service status %s", status)
	}

	return &Connection{
		Conn: conn,
		Addr: addr,
	}
}
