package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/henriquemarlon/mate/cmd/mate/root"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := root.Cmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
