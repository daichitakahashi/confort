package app

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/beacon/server"
	"github.com/daichitakahashi/workerctl"
	"github.com/docker/docker/client"
)

func Run(ctx context.Context, addr string) {
	ctx, stop := signal.NotifyContext(
		ctx,
		syscall.SIGTERM,
	)
	defer stop()

	ctl, shutdown := workerctl.New()
	a := &workerctl.Aborter{}
	ctx = workerctl.WithAbort(ctx, a)

	// init docker client
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Fatal(err)
	}

	be := confort.NewDockerBackend(cli, confort.ResourcePolicyReuse) // TODO: policy
	hc := server.HealthCheckFunc(func(ctx context.Context) error {
		_, err := cli.Ping(ctx)
		return err
	})
	svr := server.New(addr, be, hc)
	err = ctl.Launch(ctx, svr)
	if err != nil {
		log.Fatal(err)
	}

	shutdownCtx := context.Background()
	select {
	case <-a.Aborted():
		log.Println("aborted")
	case <-ctx.Done():
		log.Println("start shutdown")
	}
	err = shutdown(shutdownCtx)
	if err != nil {
		log.Fatal("error on shutdown")
	}
}
