package tests

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/daichitakahashi/confort/e2e/tenant/database"
	"github.com/daichitakahashi/confort/internal/beacon"
	"github.com/daichitakahashi/confort/unique"
)

var (
	connect                database.ConnectFunc
	uniqueTenantName       *unique.Unique[string]
	uniqueEmployeeUserName *unique.Unique[string]
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	err := os.Setenv(beacon.LogLevelEnv, "0")
	if err != nil {
		log.Panic(err)
	}

	var term func()
	connect, term, err = database.InitDatabase(ctx)
	if err != nil {
		log.Panic(err)
	}
	defer term()
	uniqueTenantName, err = unique.String(ctx, 10,
		unique.WithBeacon("tenant_name"),
	)
	if err != nil {
		log.Panic(err)
	}
	uniqueEmployeeUserName, err = unique.String(ctx, 10,
		unique.WithBeacon("employee_username"),
	)
	if err != nil {
		log.Panic(err)
	}

	m.Run()
}
