package app

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/beacon/server"
	"github.com/daichitakahashi/workerctl"
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

	hc := server.HealthCheckFunc(func(ctx context.Context) error {
		return nil
	})
	svr := server.New(addr, confort.NewExclusionControl(), hc)
	err := ctl.Launch(ctx, svr)
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
