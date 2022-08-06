package tests

import (
	"context"
	"fmt"
	"testing"

	"github.com/daichitakahashi/confort/integrationtest/database"
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
		created, err := q.CreateTenant(ctx, fmt.Sprintf("tenant%02d", i))
		if err != nil {
			t.Fatal(err)
		}
		tenants = append(tenants, created)
	}
	conn.Release()

	t.Run("GetTenant", func(t *testing.T) {
		t.Parallel()

		t.Run("tenant01", func(t *testing.T) {
			t.Parallel()

			q := database.New(pool)

			_, err = q.GetTenant(ctx, tenants[1].ID)
			if err != nil {
				t.Fatalf("tenant %q not found: %s", tenants[1].Name, err)
			}
		})

		t.Run("tenant_notfound", func(t *testing.T) {
			t.Parallel()

			q := database.New(pool)

			_, err = q.GetTenant(ctx, -1)
			if err == pgx.ErrNoRows {
				return // ok
			} else if err != nil {
				t.Fatal(err)
			} else {
				t.Fatal("error expected but test passed")
			}
		})
	})

	t.Run("ListTenants", func(t *testing.T) {
		t.Parallel()

		q := database.New(pool)

		expectedTenants := map[string]bool{
			"tenant00": true,
			"tenant01": true,
			"tenant02": true,
			"tenant03": true,
			"tenant04": true,
		}
		tenants, err := q.ListTenants(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, tenant := range tenants {
			_, ok := expectedTenants[tenant.Name]
			if !ok {
				t.Errorf("unexpected tenant found: %#v", tenant)
			} else {
				delete(expectedTenants, tenant.Name)
			}
		}
		for name := range expectedTenants {
			t.Errorf("tenant %q not found", name)
		}
	})
}
