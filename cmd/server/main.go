// Package main is the entrypoint for the LogHunter API server.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("loghunter server starting")

	// TODO: load config, wire dependencies, start HTTP server
	<-ctx.Done()
	slog.Info("loghunter server shutting down")
}
