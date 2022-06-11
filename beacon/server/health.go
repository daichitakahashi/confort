package server

import (
	"context"
	"log"

	"github.com/docker/docker/client"
	health "google.golang.org/grpc/health/grpc_health_v1"
)

type healthServer struct {
	health.UnimplementedHealthServer
	cli *client.Client
}

func (h *healthServer) Check(ctx context.Context, _ *health.HealthCheckRequest) (*health.HealthCheckResponse, error) {
	_, err := h.cli.Ping(ctx)
	if err != nil {
		log.Println("health check failed:", err)
		return &health.HealthCheckResponse{
			Status: health.HealthCheckResponse_UNKNOWN,
		}, nil
	}
	return &health.HealthCheckResponse{
		Status: health.HealthCheckResponse_SERVING,
	}, nil
}

var _ health.HealthServer = (*healthServer)(nil)
