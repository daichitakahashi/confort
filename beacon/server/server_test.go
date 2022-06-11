package server

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var conn *grpc.ClientConn

func TestMain(m *testing.M) {
	ctx := context.Background()

	srv := New(":0") // use ephemeral port
	stop, err := srv.LaunchWorker(ctx)
	if err != nil {
		panic(err)
	}
	defer stop(ctx)

	conn, err = grpc.Dial(srv.addr, grpc.WithTransportCredentials(
		insecure.NewCredentials(),
	))
	if err != nil {
		panic(err)
	}

	m.Run()
}
