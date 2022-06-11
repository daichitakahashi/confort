package main

import (
	"context"

	"github.com/daichitakahashi/confort/beacon/app"
)

func main() {
	ctx := context.Background()
	const addr = ":8443"

	app.Run(ctx, addr)
}
