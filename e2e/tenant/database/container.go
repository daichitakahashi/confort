package database

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/wait"
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

	cft, err := confort.New(ctx,
		confort.WithBeacon(),
		confort.WithNamespace("integrationtest", false),
	)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		_ = cft.Close()
	})

	db, err := cft.Run(ctx, &confort.ContainerParams{
		Name:  "db",
		Image: "postgres:14.4-alpine3.16",
		Env: map[string]string{
			"POSTGRES_USER":     dbUser,
			"POSTGRES_PASSWORD": dbPassword,
		},
		ExposedPorts: []string{"5432/tcp"},
		Waiter:       wait.Healthy(),
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
	if err != nil {
		tb.Fatal(err)
	}

	return func(tb testing.TB, ctx context.Context, exclusive bool) *pgxpool.Pool {
		tb.Helper()

		ports, release, err := db.Use(ctx, exclusive, confort.WithInitFunc(func(ctx context.Context, ports confort.Ports) error {
			cfg, err := configFromPorts(ports)
			if err != nil {
				return err
			}
			pool, err := Connect(ctx, cfg)
			if err != nil {
				return err
			}
			defer pool.Close()
			return CreateTableIfNotExists(ctx, pool)
		}))
		if err != nil {
			tb.Fatal(err)
		}
		tb.Cleanup(release)

		cfg, err := configFromPorts(ports)
		if err != nil {
			tb.Fatal(err)
		}
		pool, err := Connect(ctx, cfg)
		if err != nil {
			tb.Fatal("ConnectFunc:", err)
		}
		tb.Cleanup(func() {
			pool.Close()
		})
		return pool
	}
}

func configFromPorts(ports confort.Ports) (cfg pgconn.Config, err error) {
	binding := ports.Binding("5432/tcp")
	if binding.HostIP == "" {
		return cfg, errors.New("port not found")
	}
	p, err := strconv.ParseUint(binding.HostPort, 10, 16)
	if err != nil {
		return cfg, err
	}

	return pgconn.Config{
		Host:     "127.0.0.1",
		Port:     uint16(p),
		User:     dbUser,
		Password: dbPassword,
		Database: Database,
	}, nil
}
