package server

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/proto/beacon"
	"github.com/daichitakahashi/workerctl"
	"google.golang.org/grpc"
	health "google.golang.org/grpc/health/grpc_health_v1"
)

type Server struct {
	addr   string
	ex     confort.ExclusionControl
	health HealthChecker
}

func New(addr string, ex confort.ExclusionControl, h HealthChecker) *Server {
	return &Server{
		addr:   addr,
		ex:     ex,
		health: h,
	}
}

func (s *Server) LaunchWorker(ctx context.Context) (stop func(ctx context.Context), err error) {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return nil, err
	}
	s.addr = ln.Addr().String() // set actual address

	serv := grpc.NewServer(
		grpc.ConnectionTimeout(time.Minute * 5), // TODO: configure
	)
	beaconSvr := &beaconServer{
		ex: s.ex,
	}
	beacon.RegisterBeaconServiceServer(serv, beaconSvr)
	beacon.RegisterUniqueValueServiceServer(serv, &uniqueValueServer{})
	health.RegisterHealthServer(serv, &healthServer{
		checker: s.health,
	})

	go func() {
		err := serv.Serve(ln)
		if err != nil {
			log.Println(err)
			workerctl.Abort(ctx)
		}
	}()
	return func(ctx context.Context) {
		serv.GracefulStop()
	}, nil
}

func (s *Server) Addr() string {
	return s.addr
}

var _ workerctl.WorkerLauncher = (*Server)(nil)
