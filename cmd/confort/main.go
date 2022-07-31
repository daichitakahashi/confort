package main

import (
	"context"
	"flag"
	"os"

	"github.com/daichitakahashi/confort/internal/cmd"
)

func main() {
	flag.Parse()
	ctx := context.Background()

	status := cmd.NewCommands(
		flag.CommandLine,
		os.Args[0],
		cmd.NewOperation(),
	).Execute(ctx)

	os.Exit(int(status))
}
