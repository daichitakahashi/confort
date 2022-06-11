package app

import (
	"context"
	"log"
	"os/signal"
	"syscall"

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

	err := ctl.Launch(ctx, server.New(addr))
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
