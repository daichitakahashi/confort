package server

import (
	"context"
	"log"

	health "google.golang.org/grpc/health/grpc_health_v1"
)

type HealthChecker interface {
	HealthCheck(ctx context.Context) error
}

type HealthCheckFunc func(ctx context.Context) error

func (f HealthCheckFunc) HealthCheck(ctx context.Context) error {
	return f(ctx)
}

var _ HealthChecker = (HealthCheckFunc)(nil)

type healthServer struct {
	health.UnimplementedHealthServer
	checker HealthChecker
}

func (h *healthServer) Check(ctx context.Context, _ *health.HealthCheckRequest) (*health.HealthCheckResponse, error) {
	err := h.checker.HealthCheck(ctx)
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
