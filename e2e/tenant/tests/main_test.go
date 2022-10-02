package tests

import (
	"context"
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
	m.Setenv(beacon.LogLevelEnv, "0")

	connect = database.InitDatabase(m, ctx)
	uniqueTenantName = unique.String(10, unique.WithBeacon(m, ctx, "tenant_name"))
	uniqueEmployeeUserName = unique.String(10, unique.WithBeacon(m, ctx, "employee_username"))
	return m.Run()
}
