package tests

import (
	"context"
	"testing"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/e2e/tenant/database"
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
	beacon := confort.ConnectBeacon(m, ctx)
	connect = database.InitDatabase(m, ctx, beacon)
	uniqueTenantName = unique.String(10, unique.WithBeacon(beacon, "tenant_name"))
	uniqueEmployeeUserName = unique.String(10, unique.WithBeacon(beacon, "employee_username"))
	return m.Run()
}