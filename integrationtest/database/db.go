//go:generate go run github.com/kyleconroy/sqlc/cmd/sqlc@v1.14.0 generate

package database

import (
	"context"
	"testing"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
)

func Connect(tb testing.TB, ctx context.Context, c pgconn.Config) *pgx.Conn {
	cc := pgx.ConnConfig{
		Config: c,
	}
	conn, err := pgx.Connect(ctx, cc.ConnString())
	if err != nil {
		tb.Fatal("database.Connect", err)
	}
	tb.Cleanup(func() {
		_ = conn.Close(context.Background())
	})
	return conn
}
