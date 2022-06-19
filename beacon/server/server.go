package server

import (
	"context"
	"log"
	"net"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/proto/beacon"
	"github.com/daichitakahashi/workerctl"
	"google.golang.org/grpc"
	health "google.golang.org/grpc/health/grpc_health_v1"
)

type Server struct {
	addr   string
	be     confort.Backend
	health HealthChecker
}

func New(addr string, be confort.Backend, h HealthChecker) *Server {
	return &Server{
		addr:   addr,
		be:     be,
		health: h,
	}
}

func (s *Server) LaunchWorker(ctx context.Context) (stop func(ctx context.Context), err error) {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return nil, err
	}
	s.addr = ln.Addr().String() // set actual address

	serv := grpc.NewServer()
	beaconSvr := &beaconServer{
		be:               s.be,
		namespaces:       map[string]*namespace{},
		clientsNamespace: map[string]*namespace{},
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
		err := beaconSvr.Shutdown(ctx)
		if err != nil {
			log.Printf("error on shutdown beacon server: %s", err)
		}
	}, nil
}

var _ workerctl.WorkerLauncher = (*Server)(nil)
