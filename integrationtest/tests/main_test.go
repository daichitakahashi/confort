package tests

import (
	"context"
	"os"
	"testing"

	"github.com/daichitakahashi/confort/integrationtest/container"
	"github.com/daichitakahashi/testingc"
)

var connect container.ConnectFunc

func TestMain(m *testing.M) {
	os.Exit(
		testingc.M(m, testMain),
	)
}
func testMain(c *testingc.C) int {
	ctx := context.Background()
	connect = container.InitDatabase(c, ctx)
	return c.Run()
}
