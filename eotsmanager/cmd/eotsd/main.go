package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/babylonlabs-io/finality-provider/eotsmanager/cmd/eotsd/daemon"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := daemon.NewRootCmd().ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error while executing eotsd CLI: %s", err.Error())
		os.Exit(1) //nolint:gocritic
	}
}
