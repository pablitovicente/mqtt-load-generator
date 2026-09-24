package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/pablitovicente/mqtt-load-generator/internal/cli"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	rootCommand := cli.NewRootCommand()
	if err := rootCommand.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
