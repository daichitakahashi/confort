package container

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/integrationtest/database"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4/pgxpool"
)

type ConnectFunc func(tb testing.TB, ctx context.Context, exclusive bool) *pgxpool.Pool

const (
	dbUser     = "confort_test"
	dbPassword = "confort_pass"
	Database   = dbUser
)

func InitDatabase(tb testing.TB, ctx context.Context) ConnectFunc {
	tb.Helper()

	beacon := confort.ConnectBeacon(tb, ctx)
	cft, cleanup := confort.New(tb, ctx,
		confort.WithBeacon(beacon),
		confort.WithNamespace("integrationtest", false),
	)
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
				Test:     []string{"CMD-SHELL", "pg_isready"},
				Interval: 5 * time.Second,
				Timeout:  3 * time.Second,
			}
		}),
	)

	var release func()
	ports := cft.UseExclusive(tb, ctx, "db", confort.WithReleaseFunc(&release))
	defer release()

	endpoint, ok := ports.Binding("5432/tcp")
	if !ok {
		tb.Fatal("port not found")
	}
	_, port, _ := strings.Cut(endpoint, ":")
	p, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		tb.Fatal(err)
	}

	connCfg := pgconn.Config{
		Host:     "127.0.0.1",
		Port:     uint16(p),
		User:     dbUser,
		Password: dbPassword,
		Database: Database,
	}

	pool, err := database.Connect(ctx, connCfg)
	if err != nil {
		tb.Fatal(err)
	}
	err = database.CreateTableIfNotExists(ctx, pool)
	if err != nil {
		tb.Fatal("LaunchDatabase:", err)
	}
	pool.Close()

	return func(tb testing.TB, ctx context.Context, exclusive bool) *pgxpool.Pool {
		tb.Helper()

		if exclusive {
			cft.UseExclusive(tb, ctx, "db")
		} else {
			cft.UseShared(tb, ctx, "db")
		}

		pool, err := database.Connect(ctx, connCfg)
		if err != nil {
			tb.Fatal("ConnectFunc:", err)
		}
		tb.Cleanup(func() {
			pool.Close()
		})
		return pool
	}
}
