package container

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/integrationtest/database"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
)

type ConnectFunc func(tb testing.TB, ctx context.Context, exclusive bool) *pgx.Conn

const (
	dbUser     = "confort_test"
	dbPassword = "confort_pass"
)

func InitDatabase(tb testing.TB, ctx context.Context) ConnectFunc {
	tb.Helper()

	beacon := confort.ConnectBeacon(tb, ctx)
	cft, cleanup := confort.New(tb, ctx, confort.WithBeacon(beacon))
	tb.Cleanup(cleanup)

	cft.Run(tb, ctx, "db", &confort.Container{
		Image: "postgres:14.4-alpine3.16",
		Env: map[string]string{
			"POSTGRES_USER":     dbUser,
			"POSTGRES_PASSWORD": dbPassword,
		},
		ExposedPorts: []string{"5432/tcp"},
		Waiter:       confort.Healthy(),
	},
		confort.WithPullOptions(&types.ImagePullOptions{}, os.Stderr),
		confort.WithContainerConfig(func(config *container.Config) {
			config.Healthcheck = &container.HealthConfig{
				Test:     []string{"pg_isready"},
				Interval: 5 * time.Second,
				Timeout:  3 * time.Second,
			}
		}),
	)

	var release func()
	ports := cft.UseExclusive(tb, ctx, "db", confort.WithReleaseFunc(&release))
	defer release()

	port, ok := ports.Binding("5432/tcp")
	if !ok {
		tb.Fatal("port not found")
	}
	p, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		tb.Fatal(err)
	}

	connCfg := pgconn.Config{
		Host:     "127.0.0.1",
		Port:     uint16(p),
		User:     dbUser,
		Password: dbPassword,
		Database: dbUser,
	}

	conn, err := database.Connect(ctx, connCfg)
	if err != nil {
		tb.Fatal(err)
	}

	err = database.CreateTableIfNotExists(ctx, conn)
	if err != nil {
		tb.Fatal("LaunchDatabase:", err)
	}
	err = conn.Close(ctx)
	if err != nil {
		tb.Fatal("LaunchDatabase:", err)
	}

	return func(tb testing.TB, ctx context.Context, exclusive bool) *pgx.Conn {
		tb.Helper()

		if exclusive {
			cft.UseExclusive(tb, ctx, "db")
		} else {
			cft.UseShared(tb, ctx, "db")
		}

		conn, err := database.Connect(ctx, connCfg)
		if err != nil {
			tb.Fatal("ConnectFunc:", err)
		}
		return conn
	}
}
