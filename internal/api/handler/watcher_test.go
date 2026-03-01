package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

type mockWatcherStatusProvider struct {
	fn func(ctx context.Context) (WatcherStatus, error)
}

func (m *mockWatcherStatusProvider) WatcherStatus(ctx context.Context) (WatcherStatus, error) {
	return m.fn(ctx)
}

func TestWatcherStatusHandler_Success(t *testing.T) {
	now := time.Now().UTC()
	next := now.Add(60 * time.Second)

	provider := &mockWatcherStatusProvider{
		fn: func(_ context.Context) (WatcherStatus, error) {
			return WatcherStatus{
				Enabled:         true,
				Running:         true,
				LastPollAt:      &now,
				NextPollAt:      &next,
				ServicesWatched: []string{"auth-service", "payment-service"},
				RecentFindings: []*models.WatcherFinding{
					{
						ID:                uuid.New(),
						ClusterID:         uuid.New(),
						Service:           "auth-service",
						Kind:              "new",
						CurrentCount:      5,
						PrevCount:         0,
						AnalysisTriggered: true,
						DetectedAt:        now,
					},
				},
			}, nil
		},
	}

	handler := NewWatcherStatusHandler(provider)
	req := httptest.NewRequest("GET", "/api/v1/watcher/status", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data envelope")
	}

	if data["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", data["enabled"])
	}
	if data["running"] != true {
		t.Errorf("expected running=true, got %v", data["running"])
	}

	services, ok := data["services_watched"].([]any)
	if !ok || len(services) != 2 {
		t.Errorf("expected 2 services, got %v", data["services_watched"])
	}

	findings, ok := data["recent_findings"].([]any)
	if !ok || len(findings) != 1 {
		t.Errorf("expected 1 finding, got %v", data["recent_findings"])
	}
}

func TestWatcherStatusHandler_Disabled(t *testing.T) {
	provider := &mockWatcherStatusProvider{
		fn: func(_ context.Context) (WatcherStatus, error) {
			return WatcherStatus{
				Enabled:         false,
				Running:         false,
				ServicesWatched: []string{},
				RecentFindings:  []*models.WatcherFinding{},
			}, nil
		},
	}

	handler := NewWatcherStatusHandler(provider)
	req := httptest.NewRequest("GET", "/api/v1/watcher/status", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp["data"].(map[string]any)

	if data["enabled"] != false {
		t.Errorf("expected enabled=false")
	}
}

func TestWatcherStatusHandler_Error(t *testing.T) {
	provider := &mockWatcherStatusProvider{
		fn: func(_ context.Context) (WatcherStatus, error) {
			return WatcherStatus{}, errors.New("db connection failed")
		},
	}

	handler := NewWatcherStatusHandler(provider)
	req := httptest.NewRequest("GET", "/api/v1/watcher/status", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}
