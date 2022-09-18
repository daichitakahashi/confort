package tests

import (
	"context"
	"testing"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/integrationtest/container"
	"github.com/daichitakahashi/testingc"
)

var (
	connect                container.ConnectFunc
	uniqueTenantName       *confort.Unique[string]
	uniqueEmployeeUserName *confort.Unique[string]
)

func TestMain(m *testing.M) { testingc.M(m, testMain) }

func testMain(c *testingc.C) int {
	ctx := context.Background()
	beacon := confort.ConnectBeacon(c, ctx)
	connect = container.InitDatabase(c, ctx, beacon)
	uniqueTenantName = confort.UniqueString(10, confort.WithGlobalUniqueness(beacon, "tenant_name"))
	uniqueEmployeeUserName = confort.UniqueString(10, confort.WithGlobalUniqueness(beacon, "employee_username"))
	return c.Run()
}
