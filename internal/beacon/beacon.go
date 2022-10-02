package beacon

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/daichitakahashi/confort/internal/logging"
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
		if errors.Is(err, ErrIntegrationDisabled) {
			logging.Info(tb, err)
			return nil
		}
		logging.Fatal(tb, err)
	}
	if addr == "" {
		logging.Info(tb, "cannot get the address of beacon server")
		return nil
	}
	logging.Debugf(tb, "the address of beacon server: %s", addr)

	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(
		insecure.NewCredentials(),
	))
	if err != nil {
		logging.Fatal(tb, err)
	}
	tb.Cleanup(func() {
		err := conn.Close()
		if err != nil {
			logging.Error(tb, err)
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
		logging.Debugf(tb, "got health check status of beacon server: %s", status)
		if status == health.HealthCheckResponse_SERVING {
			break
		}
	}
	if err != nil {
		logging.Fatal(tb, err)
	} else if status != health.HealthCheckResponse_SERVING {
		logging.Fatalf(tb, "unexpected service status %s", status)
	}

	return &Connection{
		Conn: conn,
		Addr: addr,
	}
}
