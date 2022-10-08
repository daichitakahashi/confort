package tests

import (
	"context"
	"os"
	"testing"

	"github.com/daichitakahashi/confort/e2e/tenant/database"
	"github.com/daichitakahashi/confort/internal/beacon"
	"github.com/daichitakahashi/confort/unique"
	"github.com/daichitakahashi/testingc"
)

var (
	connect                database.ConnectFunc
	uniqueTenantName       *unique.Unique[string]
	uniqueEmployeeUserName *unique.Unique[string]
)

func TestMain(m *testing.M) { testingc.M(m, testMain) }

func testMain(m *testingc.MC) int {
	ctx := context.Background()
	err := os.Setenv(beacon.LogLevelEnv, "0")
	if err != nil {
		m.Fatal(err)
	}

	connect = database.InitDatabase(m, ctx)
	uniqueTenantName, err = unique.String(ctx, 10,
		unique.WithBeacon("tenant_name"),
	)
	if err != nil {
		m.Fatal(err)
	}
	uniqueEmployeeUserName, err = unique.String(ctx, 10,
		unique.WithBeacon("employee_username"),
	)
	if err != nil {
		m.Fatal(err)
	}

	return m.Run()
}
