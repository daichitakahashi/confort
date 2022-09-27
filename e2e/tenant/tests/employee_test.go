package tests

import (
	"context"
	"testing"

	database2 "github.com/daichitakahashi/confort/e2e/tenant/database"
	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v4"
)

func TestEmployees(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// get connection pool (SHARED)
	pool := connect(t, ctx, false)
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// create test tenant
	q := database2.New(conn)
	tenant, err := q.CreateTenant(ctx, uniqueTenantName.Must(t))
	if err != nil {
		t.Fatal(err)
	}

	var employees []database2.Employee
	for i := 0; i < 50; i++ {
		userName := uniqueEmployeeUserName.Must(t)
		employee, err := q.CreateEmployee(ctx, database2.CreateEmployeeParams{
			Username: userName,
			Name:     userName,
			TenantID: tenant.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		employees = append(employees, employee)
	}
	conn.Release()

	t.Run("GetEmployee", func(t *testing.T) {
		t.Parallel()

		t.Run("[20]", func(t *testing.T) {
			t.Parallel()

			q := database2.New(pool)

			_, err := q.GetEmployees(ctx, database2.GetEmployeesParams{
				TenantID: tenant.ID,
				ID:       employees[20].ID,
			})
			if err != nil {
				t.Fatalf("employee %q not found: %s", employees[20].Username, err)
			}
		})

		t.Run("notfound", func(t *testing.T) {
			t.Parallel()

			q := database2.New(pool)

			_, err := q.GetEmployees(ctx, database2.GetEmployeesParams{
				TenantID: tenant.ID,
				ID:       employees[49].ID + 1,
			})
			if err == pgx.ErrNoRows {
				return // ok
			} else if err != nil {
				t.Fatal(err)
			} else {
				t.Fatal("error expected but succeeded")
			}
		})
	})

	t.Run("ListEmployees", func(t *testing.T) {
		t.Parallel()

		q := database2.New(pool)

		actualEmployees, err := q.ListEmployees(ctx, tenant.ID)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(employees, actualEmployees); diff != "" {
			t.Fatal(diff)
		}
	})
}
