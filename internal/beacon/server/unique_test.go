package server

import (
	"context"
	"testing"

	"github.com/daichitakahashi/confort/internal/beacon/proto"
)

func TestUniqueValueServer_StoreUniqueValue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	connect := startServer(t, nil)
	cli := proto.NewUniqueValueServiceClient(connect(t))

	testCases := []struct {
		store   string
		value   string
		success bool
	}{
		{"uuid", "cac66b57-3f31-4f44-915e-1efe083147f1", true},
		{"uuid", "fc72ad70-9190-41c3-b327-65afbe751d01", true},
		{"uuid", "fc72ad70-9190-41c3-b327-65afbe751d01", false},
		{"uuid", "cac66b57-3f31-4f44-915e-1efe083147f1", false},
		{"uuid", "b2119847-a173-460e-8b4e-0218e084fa58", true},
		{"another-uuid", "cac66b57-3f31-4f44-915e-1efe083147f1", true},
		{"another-uuid", "cac66b57-3f31-4f44-915e-1efe083147f1", false},
		{"another-uuid", "fc72ad70-9190-41c3-b327-65afbe751d01", true},
	}

	for _, tc := range testCases {
		resp, err := cli.StoreUniqueValue(ctx, &proto.StoreUniqueValueRequest{
			Store: tc.store,
			Value: tc.value,
		})
		if err != nil {
			t.Fatalf("store: %s, value: %s, err: %s", tc.store, tc.value, err)
		}
		want := resp.GetSucceeded()
		got := tc.success
		if got != want {
			t.Errorf("store: %s, value: %s, want: %s, got: %s",
				tc.store, tc.value, result(want), result(got))
		}
	}
}

func result(succeeded bool) string {
	if succeeded {
		return "succeeded"
	}
	return "failed"
}
