package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- mock health checkers ---

type healthMockDB struct{ err error }

func (m *healthMockDB) Ping(_ context.Context) error { return m.err }

type healthMockCache struct{ err error }

func (m *healthMockCache) Ping(_ context.Context) error { return m.err }

type healthMockLoki struct{ err error }

func (m *healthMockLoki) Ready(_ context.Context) error { return m.err }

type healthMockAI struct{ name string }

func (m *healthMockAI) Name() string { return m.name }

// --- tests ---

func TestHealthHandler_AllHealthy(t *testing.T) {
	handler := NewHealthHandler(
		&healthMockDB{},
		&healthMockCache{},
		&healthMockLoki{},
		&healthMockAI{name: "openai"},
	)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	if data["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", data["status"])
	}

	checks := data["checks"].(map[string]any)
	for _, dep := range []string{"database", "redis", "loki", "ai_provider"} {
		if checks[dep] != "ok" {
			t.Errorf("expected %s 'ok', got %v", dep, checks[dep])
		}
	}
}

func TestHealthHandler_DatabaseDown(t *testing.T) {
	handler := NewHealthHandler(
		&healthMockDB{err: errors.New("connection refused")},
		&healthMockCache{},
		&healthMockLoki{},
		&healthMockAI{name: "openai"},
	)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	if data["status"] != "degraded" {
		t.Errorf("expected status 'degraded', got %v", data["status"])
	}
	checks := data["checks"].(map[string]any)
	if checks["database"] != "error" {
		t.Errorf("expected database 'error', got %v", checks["database"])
	}
	if checks["redis"] != "ok" {
		t.Errorf("expected redis 'ok', got %v", checks["redis"])
	}
}

func TestHealthHandler_RedisDown(t *testing.T) {
	handler := NewHealthHandler(
		&healthMockDB{},
		&healthMockCache{err: errors.New("redis timeout")},
		&healthMockLoki{},
		&healthMockAI{name: "openai"},
	)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	checks := data["checks"].(map[string]any)
	if checks["redis"] != "error" {
		t.Errorf("expected redis 'error', got %v", checks["redis"])
	}
}

func TestHealthHandler_LokiDown(t *testing.T) {
	handler := NewHealthHandler(
		&healthMockDB{},
		&healthMockCache{},
		&healthMockLoki{err: errors.New("loki unreachable")},
		&healthMockAI{name: "openai"},
	)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	checks := data["checks"].(map[string]any)
	if checks["loki"] != "error" {
		t.Errorf("expected loki 'error', got %v", checks["loki"])
	}
}

func TestHealthHandler_AIProviderUnconfigured(t *testing.T) {
	handler := NewHealthHandler(
		&healthMockDB{},
		&healthMockCache{},
		&healthMockLoki{},
		nil, // no AI provider
	)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	checks := data["checks"].(map[string]any)
	if checks["ai_provider"] != "error" {
		t.Errorf("expected ai_provider 'error', got %v", checks["ai_provider"])
	}
}

func TestHealthHandler_MultipleDepsDown(t *testing.T) {
	handler := NewHealthHandler(
		&healthMockDB{err: errors.New("db down")},
		&healthMockCache{err: errors.New("cache down")},
		&healthMockLoki{},
		&healthMockAI{name: "openai"},
	)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	checks := data["checks"].(map[string]any)
	if checks["database"] != "error" {
		t.Errorf("expected database 'error', got %v", checks["database"])
	}
	if checks["redis"] != "error" {
		t.Errorf("expected redis 'error', got %v", checks["redis"])
	}
	if checks["loki"] != "ok" {
		t.Errorf("expected loki 'ok', got %v", checks["loki"])
	}
}

func TestHealthHandler_NoAuthRequired(t *testing.T) {
	// Health handler should work without any tenant context
	handler := NewHealthHandler(
		&healthMockDB{},
		&healthMockCache{},
		&healthMockLoki{},
		&healthMockAI{name: "openai"},
	)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	// No tenant context set — should still work
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 without auth, got %d", rr.Code)
	}
}

func TestHealthHandler_AllDepsDown(t *testing.T) {
	handler := NewHealthHandler(
		&healthMockDB{err: errors.New("db")},
		&healthMockCache{err: errors.New("cache")},
		&healthMockLoki{err: errors.New("loki")},
		nil,
	)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	if data["status"] != "degraded" {
		t.Errorf("expected status 'degraded', got %v", data["status"])
	}
	checks := data["checks"].(map[string]any)
	for _, dep := range []string{"database", "redis", "loki", "ai_provider"} {
		if checks[dep] != "error" {
			t.Errorf("expected %s 'error', got %v", dep, checks[dep])
		}
	}
}
