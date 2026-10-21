// Command worker relays committed Outbox Events to the fixed dev Stream.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/monkeylabx/threadline/services/worker/internal/process"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "threadline-worker:", err)
		os.Exit(1)
	}
}

func run() error {
	config, err := process.LoadConfig(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return process.Run(ctx, config)
}
