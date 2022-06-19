package server

import (
	"context"
	"crypto/sha1"
	"fmt"
	"testing"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/internal/mock"
	"github.com/daichitakahashi/confort/proto/beacon"
	"github.com/docker/docker/api/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	health "google.golang.org/grpc/health/grpc_health_v1"
)

func defaultNamespaceFunc(_ context.Context, namespace string) (confort.Namespace, error) {
	nw := &types.NetworkResource{
		Name: namespace,
		ID:   fmt.Sprintf("%x", sha1.Sum([]byte(namespace))),
	}
	return &mock.NamespaceMock{
		NetworkFunc: func() *types.NetworkResource {
			return nw
		},
		NamespaceFunc: func() string {
			return namespace
		},
		ReleaseFunc: func(ctx context.Context) error {
			return nil
		},
	}, nil
}

func startServer(t *testing.T) (*mock.BackendMock, beacon.BeaconServiceClient, beacon.UniqueValueServiceClient, health.HealthClient) {
	ctx := context.Background()
	be := &mock.BackendMock{
		NamespaceFunc: defaultNamespaceFunc,
	}
	hc := HealthCheckFunc(func(ctx context.Context) error {
		return nil
	})

	srv := New(":0", be, hc) // use ephemeral port
	stop, err := srv.LaunchWorker(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stop(ctx)
	})

	conn, err := grpc.Dial(srv.addr, grpc.WithTransportCredentials(
		insecure.NewCredentials(),
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})
	return be, beacon.NewBeaconServiceClient(conn), beacon.NewUniqueValueServiceClient(conn), health.NewHealthClient(conn)
}
