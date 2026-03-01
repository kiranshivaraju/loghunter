package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/kiranshivaraju/loghunter/internal/api/response"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// WatcherStatus holds the watcher state for the API response.
type WatcherStatus struct {
	Enabled         bool                     `json:"enabled"`
	Running         bool                     `json:"running"`
	LastPollAt      *time.Time               `json:"last_poll_at,omitempty"`
	NextPollAt      *time.Time               `json:"next_poll_at,omitempty"`
	ServicesWatched []string                 `json:"services_watched"`
	RecentFindings  []*models.WatcherFinding `json:"recent_findings"`
}

// WatcherStatusProvider returns the current watcher status.
type WatcherStatusProvider interface {
	WatcherStatus(ctx context.Context) (WatcherStatus, error)
}

// NewWatcherStatusHandler returns an http.HandlerFunc for GET /api/v1/watcher/status.
func NewWatcherStatusHandler(provider WatcherStatusProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := provider.WatcherStatus(r.Context())
		if err != nil {
			code, errCode, msg := mapError(err)
			response.Error(w, code, errCode, msg, nil)
			return
		}
		response.JSON(w, status)
	}
}
