package confort

import (
	"context"
	"testing"
	"time"

	"github.com/daichitakahashi/confort/internal/beaconutil"
	"github.com/lestrrat-go/backoff/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	health "google.golang.org/grpc/health/grpc_health_v1"
)

type Connection struct {
	Conn *grpc.ClientConn
	addr string
}

func (c *Connection) Enabled() bool {
	return c.Conn != nil
}

// ConnectBeacon tries to connect beacon server and returns its result.
// The address of server will be read from CFT_BEACON_ADDR or lock file specified as CFT_LOCKFILE.
//
// # With `confort test` command
//
// This command starts beacon server and sets the address as CFT_BEACON_ADDR automatically.
//
// # With `confort start` command
//
// This command starts beacon server and creates a lock file that contains the address.
// The default filename is ".confort.lock" and you don't need to set the file name as CFT_LOCKFILE.
// If you set a custom filename with "-lock-file" option, also you have to set the file name as CFT_LOCKFILE,
// or you can set address that read from lock file as CFT_BEACON_ADDR.
func ConnectBeacon(tb testing.TB, ctx context.Context) *Connection {
	tb.Helper()

	addr, err := beaconutil.Address(ctx, beaconutil.LockFilePath())
	if err != nil {
		tb.Logf("confort: %s", err)
	}
	if addr == "" {
		tb.Log("cannot get beacon address")
		return &Connection{}
	}

	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(
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

	return &Connection{
		Conn: conn,
		addr: addr,
	}
}
