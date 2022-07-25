package beaconserver

import (
	"context"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/proto/beacon"
	"google.golang.org/grpc"
	health "google.golang.org/grpc/health/grpc_health_v1"
)

func Register(serv *grpc.Server, interrupt func() error) {
	beacon.RegisterBeaconServiceServer(serv, &beaconServer{
		ex:        confort.NewExclusionControl(),
		interrupt: interrupt,
	})
	beacon.RegisterUniqueValueServiceServer(serv, &uniqueValueServer{})
	health.RegisterHealthServer(serv, &healthServer{
		checker: HealthCheckFunc(func(ctx context.Context) error {
			return nil
		}),
	})
}
