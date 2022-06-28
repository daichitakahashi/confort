package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/daichitakahashi/confort/internal/integration"
)

func main() {
	flag.Parse()
	ctx := context.Background()

	op, err := integration.NewOperation()
	if err != nil {
		log.Fatal(err)
	}

	cmd := integration.NewCommands(flag.CommandLine, os.Args[0], op, os.Stdout, os.Stderr)
	os.Exit(int(cmd.Execute(ctx)))
}
