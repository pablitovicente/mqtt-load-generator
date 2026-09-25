package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/pablitovicente/mqtt-load-generator/internal/cli"
)

func main() {
	ctx, stopCatchingSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopCatchingSignals()

	// The first Ctrl-C cancels ctx and the command shuts down cleanly, which can take up to
	// --ack-timeout while in-flight publishes finish. After that, stop catching signals, so a
	// second Ctrl-C kills the process right away (Go's default behaviour).
	go func() {
		<-ctx.Done()
		stopCatchingSignals()
	}()

	rootCommand := cli.NewRootCommand()
	if err := rootCommand.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
