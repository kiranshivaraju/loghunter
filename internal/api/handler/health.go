package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
)

// DBPinger checks database connectivity.
type DBPinger interface {
	Ping(ctx context.Context) error
}

// CachePinger checks cache connectivity.
type CachePinger interface {
	Ping(ctx context.Context) error
}

// LokiReadyChecker checks Loki availability.
type LokiReadyChecker interface {
	Ready(ctx context.Context) error
}

// AIProviderNamer checks AI provider configuration.
type AIProviderNamer interface {
	Name() string
}

var errNotConfigured = errors.New("not configured")

// NewHealthHandler returns an http.HandlerFunc for GET /api/v1/health.
// All dependency checks run concurrently.
func NewHealthHandler(db DBPinger, cache CachePinger, loki LokiReadyChecker, ai AIProviderNamer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		type result struct {
			name   string
			status string
		}

		ch := make(chan result, 4)

		var wg sync.WaitGroup
		wg.Add(4)

		go func() { defer wg.Done(); s := "ok"; if db.Ping(ctx) != nil { s = "error" }; ch <- result{"database", s} }()
		go func() { defer wg.Done(); s := "ok"; if cache.Ping(ctx) != nil { s = "error" }; ch <- result{"redis", s} }()
		go func() { defer wg.Done(); s := "ok"; if loki.Ready(ctx) != nil { s = "error" }; ch <- result{"loki", s} }()
		go func() { defer wg.Done(); s := "ok"; if ai == nil { s = "error" }; ch <- result{"ai_provider", s} }()

		wg.Wait()
		close(ch)

		checks := make(map[string]string, 4)
		degraded := false
		for res := range ch {
			checks[res.name] = res.status
			if res.status != "ok" {
				degraded = true
			}
		}

		status := "ok"
		httpStatus := http.StatusOK
		if degraded {
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"status": status,
				"checks": checks,
			},
		})
	}
}
