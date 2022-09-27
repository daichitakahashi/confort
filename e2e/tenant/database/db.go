//go:generate go run github.com/kyleconroy/sqlc/cmd/sqlc@v1.14.0 generate

package database

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4/pgxpool"
)

func Connect(ctx context.Context, c pgconn.Config) (*pgxpool.Pool, error) {
	// "postgres://username:password@localhost:5432/database_name"
	s := fmt.Sprintf("postgres://%s:%s@%s:%d/%s", c.User, c.Password, c.Host, c.Port, c.Database)
	return pgxpool.Connect(ctx, s)
}

//go:embed schema.sql
var createTables string

var r = regexp.MustCompile(`(?sU)create table .+ \(.+\);`)

func CreateTableIfNotExists(ctx context.Context, conn *pgxpool.Pool) error {
	rr := r.FindAllString(createTables, -1)
	for _, sql := range rr {
		_, err := conn.Exec(ctx, sql)
		if err != nil {
			return err
		}
	}
	return nil
}
