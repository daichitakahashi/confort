package server

import (
	"context"
	"log"
	"net"

	"github.com/daichitakahashi/confort/proto/beacon"
	"github.com/daichitakahashi/workerctl"
	"google.golang.org/grpc"
)

type Server struct {
	addr string
}

func New(addr string) *Server {
	return &Server{
		addr: addr,
	}
}

func (s *Server) LaunchWorker(ctx context.Context) (stop func(ctx context.Context), err error) {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return nil, err
	}

	serv := grpc.NewServer()
	beacon.RegisterBeaconServiceServer(serv, &beaconServer{})
	beacon.RegisterUniqueValueServiceServer(serv, &uniqueValueServer{})

	go func() {
		err := serv.Serve(ln)
		if err != nil {
			log.Println(err)
			workerctl.Abort(ctx)
		}
	}()
	return func(_ context.Context) {
		serv.GracefulStop()
	}, nil
}

var _ workerctl.WorkerLauncher = (*Server)(nil)
