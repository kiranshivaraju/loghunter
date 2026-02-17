// Package main is the entrypoint for the LogHunter API server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kiranshivaraju/loghunter/internal/ai"
	"github.com/kiranshivaraju/loghunter/internal/api"
	mw "github.com/kiranshivaraju/loghunter/internal/api/middleware"
	"github.com/kiranshivaraju/loghunter/internal/api/response"
	"github.com/kiranshivaraju/loghunter/internal/cache"
	"github.com/kiranshivaraju/loghunter/internal/config"
	"github.com/kiranshivaraju/loghunter/internal/store"
)

const shutdownTimeout = 30 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// 1. Load config — fail fast on invalid config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	slog.Info("config loaded", "ai_provider", cfg.AI.Provider, "env", cfg.Server.Env)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 2. Connect to database
	pool, err := store.Connect(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()
	slog.Info("database connected")

	// 3. Run migrations
	if err := store.RunMigrations(cfg.Database.URL, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	slog.Info("database migrations applied")

	// 4. Create Redis cache
	redisCache, err := cache.NewRedisCache(cfg.Redis.URL)
	if err != nil {
		return fmt.Errorf("create redis cache: %w", err)
	}
	defer redisCache.Close()

	if err := redisCache.Ping(ctx); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}
	slog.Info("redis connected")

	// 5. Create AI provider
	aiProvider, err := ai.NewProvider(cfg.AI)
	if err != nil {
		return fmt.Errorf("create AI provider: %w", err)
	}
	slog.Info("AI provider initialized", "provider", aiProvider.Name())

	// 6. Create store
	pgStore := store.NewPostgresStore(pool)

	// 7. Build router with dependencies
	auth := mw.NewAuth(pgStore)
	rateLimit := mw.NewRateLimit(redisCache, 60)

	deps := api.Dependencies{
		Auth:      auth,
		RateLimit: rateLimit,

		HealthHandler: healthHandler(pgStore, redisCache),
		// Remaining handlers will be wired as they are implemented
	}

	router := api.NewRouter(deps)

	// 8. Start HTTP server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		slog.Info("server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		slog.Info("shutdown signal received, draining connections...")
	}

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	slog.Info("server stopped gracefully")
	return nil
}

// healthHandler checks database and cache connectivity.
func healthHandler(s store.Store, c cache.Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]string{
			"database": "ok",
			"cache":    "ok",
		}

		if err := s.Ping(r.Context()); err != nil {
			checks["database"] = "degraded"
		}
		if err := c.Ping(r.Context()); err != nil {
			checks["cache"] = "degraded"
		}

		degraded := checks["database"] != "ok" || checks["cache"] != "ok"
		if degraded {
			response.Error(w, http.StatusServiceUnavailable, "DEGRADED",
				"One or more services degraded", checks)
			return
		}

		response.JSON(w, map[string]any{
			"status":   "ok",
			"services": checks,
		})
	}
}
