package tests

import (
	"context"
	"testing"

	"github.com/daichitakahashi/confort/integrationtest/database"
	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v4"
)

func TestTenants(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// get connection pool (EXCLUSIVE)
	pool := connect(t, ctx, true)
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// remove all data
	q := database.New(conn)
	err = q.ClearEmployees(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = q.ClearTenants(ctx)
	if err != nil {
		t.Fatal(err)
	}

	tenants := make([]database.Tenant, 0, 5)
	for i := 0; i < 5; i++ {
		created, err := q.CreateTenant(ctx, uniqueTenantName.Must(t))
		if err != nil {
			t.Fatal(err)
		}
		tenants = append(tenants, created)
	}
	conn.Release()

	t.Run("GetTenant", func(t *testing.T) {
		t.Parallel()

		t.Run("[1]", func(t *testing.T) {
			t.Parallel()

			q := database.New(pool)

			_, err := q.GetTenant(ctx, tenants[1].ID)
			if err != nil {
				t.Fatalf("tenant %q not found: %s", tenants[1].Name, err)
			}
		})

		t.Run("notfound", func(t *testing.T) {
			t.Parallel()

			q := database.New(pool)

			_, err := q.GetTenant(ctx, tenants[4].ID+1)
			if err == pgx.ErrNoRows {
				return // ok
			} else if err != nil {
				t.Fatal(err)
			} else {
				t.Fatal("error expected but succeeded")
			}
		})
	})

	t.Run("ListTenants", func(t *testing.T) {
		t.Parallel()

		q := database.New(pool)

		actualTenants, err := q.ListTenants(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(tenants, actualTenants); diff != "" {
			t.Fatal(diff)
		}
	})
}
