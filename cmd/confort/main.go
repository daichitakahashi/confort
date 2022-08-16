package main

import (
	"context"
	"flag"
	"os"

	"github.com/daichitakahashi/confort/internal/cmd"
)

func main() {
	ctx := context.Background()

	command := cmd.NewCommands(
		flag.CommandLine,
		cmd.NewOperation(),
	)
	flag.Parse()

	os.Exit(
		int(command.Execute(ctx)),
	)
}
