package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/daichitakahashi/confort/beacon/server"
	"github.com/daichitakahashi/workerctl"
)

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGTERM,
	)
	defer cancel()

	ctl, shutdown := workerctl.New()
	a := &workerctl.Aborter{}
	ctx = workerctl.WithAbort(ctx, a)

	err := ctl.Launch(ctx, server.New(":8443"))
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
