package beacon

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/daichitakahashi/confort/internal/logging"
	"github.com/lestrrat-go/backoff/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	health "google.golang.org/grpc/health/grpc_health_v1"
)

var (
	connMu sync.Mutex
	conn   *Connection
)

type Connection struct {
	Conn *grpc.ClientConn
	Addr string
}

func (c *Connection) Enabled() bool {
	return c != nil && c.Conn != nil
}

func (c *Connection) Close() error {
	return c.Conn.Close()
}

func Connect(ctx context.Context) (*Connection, error) {
	connMu.Lock()
	defer connMu.Unlock()
	if conn != nil {
		return conn, nil
	}
	var err error
	conn, err = connect(ctx)
	return conn, err
}

func connect(ctx context.Context) (*Connection, error) {

	addr, err := Address(ctx, LockFilePath())
	if err != nil {
		if errors.Is(err, ErrIntegrationDisabled) {
			logging.Info(err)
			return &Connection{}, nil
		}
		return nil, err
	}
	if addr == "" {
		logging.Info("cannot get the address of beacon server")
		return &Connection{}, nil
	}
	logging.Debugf("the address of beacon server: %s", addr)

	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(
		insecure.NewCredentials(),
	))
	if err != nil {
		return nil, err
	}

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
		logging.Debugf("got health check status of beacon server: %s", status)
		if status == health.HealthCheckResponse_SERVING {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	if status != health.HealthCheckResponse_SERVING {
		return nil, fmt.Errorf("unexpected service status %s", status)
	}

	return &Connection{
		Conn: conn,
		Addr: addr,
	}, nil
}
