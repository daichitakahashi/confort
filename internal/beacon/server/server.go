package server

import (
	"context"

	"github.com/daichitakahashi/confort/internal/beacon/proto"
	"github.com/daichitakahashi/confort/internal/exclusion"
	"google.golang.org/grpc"
	health "google.golang.org/grpc/health/grpc_health_v1"
)

func Register(serv *grpc.Server, interrupt func() error) {
	proto.RegisterBeaconServiceServer(serv, &beaconServer{
		l:         exclusion.NewLocker(),
		interrupt: interrupt,
	})
	proto.RegisterUniqueValueServiceServer(serv, &uniqueValueServer{})
	health.RegisterHealthServer(serv, &healthServer{
		checker: HealthCheckFunc(func(ctx context.Context) error {
			return nil
		}),
	})
}
