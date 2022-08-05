//go:generate go run github.com/kyleconroy/sqlc/cmd/sqlc@v1.14.0 generate

package database

import (
	"context"
	_ "embed"
	"regexp"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
)

func Connect(ctx context.Context, c pgconn.Config) (*pgx.Conn, error) {
	cc := pgx.ConnConfig{
		Config: c,
	}
	return pgx.Connect(ctx, cc.ConnString())
}

//go:embed schema.sql
var createTables string

var r = regexp.MustCompile(`(?sU)create table .+ \(.+\);`)

func CreateTableIfNotExists(ctx context.Context, conn *pgx.Conn) error {
	rr := r.FindAllString(createTables, -1)
	for _, sql := range rr {
		_, err := conn.Exec(ctx, sql)
		if err != nil {
			return err
		}
	}
	return nil
}
