package main

import (
	"context"
	"flag"
	"os"

	"github.com/daichitakahashi/confort/internal/integration"
)

func main() {
	flag.Parse()
	ctx := context.Background()

	cmd := integration.NewCommands(flag.CommandLine, os.Args[0], integration.NewOperation())
	os.Exit(int(cmd.Execute(ctx)))
}
