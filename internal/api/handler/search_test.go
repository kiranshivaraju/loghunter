package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// --- mock searcher ---

type mockSearcher struct {
	result   *SearchResult
	err      error
	captured *SearchParams
}

func (s *mockSearcher) Search(_ context.Context, params SearchParams) (*SearchResult, error) {
	s.captured = &params
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

// --- helpers ---

func searchBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(v)
	return &buf
}

func parseSearchResp(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	return out
}

// --- tests ---

func TestSearchHandler_Success(t *testing.T) {
	now := time.Now()
	svc := &mockSearcher{
		result: &SearchResult{
			Results: []SearchResultLine{
				{Timestamp: now, Message: "connection timeout", Level: "error", Labels: map[string]string{"service": "api"}},
			},
			Query:    "timeout",
			CacheHit: false,
		},
	}

	handler := NewSearchHandler(svc)

	body := searchBody(t, map[string]any{
		"service": "api",
		"start":   now.Add(-1 * time.Hour).Format(time.RFC3339),
		"end":     now.Format(time.RFC3339),
		"keyword": "timeout",
	})
	req := httptest.NewRequest("POST", "/api/v1/search", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	resp := parseSearchResp(t, rr)
	data := resp["data"].(map[string]any)
	if data["query"] != "timeout" {
		t.Errorf("expected query 'timeout', got %v", data["query"])
	}
	results := data["results"].([]any)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestSearchHandler_CacheHit(t *testing.T) {
	svc := &mockSearcher{
		result: &SearchResult{
			Results:  []SearchResultLine{},
			Query:    "timeout",
			CacheHit: true,
		},
	}

	handler := NewSearchHandler(svc)

	body := searchBody(t, map[string]any{
		"service": "api",
		"start":   time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"end":     time.Now().Format(time.RFC3339),
		"keyword": "timeout",
	})
	req := httptest.NewRequest("POST", "/api/v1/search", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	resp := parseSearchResp(t, rr)
	data := resp["data"].(map[string]any)
	if data["cache_hit"] != true {
		t.Error("expected cache_hit to be true")
	}
}

func TestSearchHandler_MissingService(t *testing.T) {
	handler := NewSearchHandler(&mockSearcher{result: &SearchResult{}})

	body := searchBody(t, map[string]any{
		"keyword": "timeout",
		"start":   time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"end":     time.Now().Format(time.RFC3339),
	})
	req := httptest.NewRequest("POST", "/api/v1/search", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestSearchHandler_MissingStart(t *testing.T) {
	handler := NewSearchHandler(&mockSearcher{result: &SearchResult{}})

	body := searchBody(t, map[string]any{
		"service": "api",
		"end":     time.Now().Format(time.RFC3339),
		"keyword": "timeout",
	})
	req := httptest.NewRequest("POST", "/api/v1/search", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestSearchHandler_MissingEnd(t *testing.T) {
	handler := NewSearchHandler(&mockSearcher{result: &SearchResult{}})

	body := searchBody(t, map[string]any{
		"service": "api",
		"start":   time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"keyword": "timeout",
	})
	req := httptest.NewRequest("POST", "/api/v1/search", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestSearchHandler_InvalidJSON(t *testing.T) {
	handler := NewSearchHandler(&mockSearcher{result: &SearchResult{}})

	req := httptest.NewRequest("POST", "/api/v1/search", bytes.NewBufferString("{invalid"))
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestSearchHandler_NoTenant(t *testing.T) {
	handler := NewSearchHandler(&mockSearcher{result: &SearchResult{}})

	body := searchBody(t, map[string]any{"service": "api", "keyword": "timeout",
		"start": time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"end":   time.Now().Format(time.RFC3339),
	})
	req := httptest.NewRequest("POST", "/api/v1/search", body)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestSearchHandler_KeywordNonPrintable(t *testing.T) {
	handler := NewSearchHandler(&mockSearcher{result: &SearchResult{}})

	body := searchBody(t, map[string]any{
		"service": "api",
		"keyword": "hello\x00world",
		"start":   time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"end":     time.Now().Format(time.RFC3339),
	})
	req := httptest.NewRequest("POST", "/api/v1/search", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-printable keyword, got %d", rr.Code)
	}
	resp := parseSearchResp(t, rr)
	errObj := resp["error"].(map[string]any)
	if errObj["code"] != "INVALID_QUERY" {
		t.Errorf("expected INVALID_QUERY, got %v", errObj["code"])
	}
}

func TestSearchHandler_KeywordTooLong(t *testing.T) {
	handler := NewSearchHandler(&mockSearcher{result: &SearchResult{}})

	body := searchBody(t, map[string]any{
		"service": "api",
		"keyword": strings.Repeat("a", 201),
		"start":   time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"end":     time.Now().Format(time.RFC3339),
	})
	req := httptest.NewRequest("POST", "/api/v1/search", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for keyword > 200 chars, got %d", rr.Code)
	}
}

func TestSearchHandler_LimitClamping(t *testing.T) {
	tests := []struct {
		name      string
		limit     int
		wantLimit int
	}{
		{"zero defaults to 100", 0, 100},
		{"below minimum clamped to 1", -5, 1},
		{"above maximum clamped to 1000", 2000, 1000},
		{"normal passes through", 50, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockSearcher{result: &SearchResult{Results: []SearchResultLine{}, Query: "test"}}

			handler := NewSearchHandler(svc)

			body := searchBody(t, map[string]any{
				"service": "api",
				"keyword": "test",
				"start":   time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				"end":     time.Now().Format(time.RFC3339),
				"limit":   tt.limit,
			})
			req := httptest.NewRequest("POST", "/api/v1/search", body)
			req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
			}
			if svc.captured == nil {
				t.Fatal("expected service to be called")
			}
			if svc.captured.Limit != tt.wantLimit {
				t.Errorf("expected limit %d, got %d", tt.wantLimit, svc.captured.Limit)
			}
		})
	}
}

func TestSearchHandler_DefaultNamespace(t *testing.T) {
	svc := &mockSearcher{result: &SearchResult{Results: []SearchResultLine{}, Query: "test"}}

	handler := NewSearchHandler(svc)

	body := searchBody(t, map[string]any{
		"service": "api",
		"keyword": "test",
		"start":   time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"end":     time.Now().Format(time.RFC3339),
	})
	req := httptest.NewRequest("POST", "/api/v1/search", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if svc.captured.Namespace != "default" {
		t.Errorf("expected default namespace, got %q", svc.captured.Namespace)
	}
}

func TestSearchHandler_ServiceError(t *testing.T) {
	svc := &mockSearcher{err: errors.New("loki connection failed")}

	handler := NewSearchHandler(svc)

	body := searchBody(t, map[string]any{
		"service": "api",
		"keyword": "test",
		"start":   time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"end":     time.Now().Format(time.RFC3339),
	})
	req := httptest.NewRequest("POST", "/api/v1/search", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestSearchHandler_ResultWithClusterID(t *testing.T) {
	clusterID := uuid.New()
	svc := &mockSearcher{
		result: &SearchResult{
			Results: []SearchResultLine{
				{
					Timestamp: time.Now(),
					Message:   "error",
					Level:     "error",
					Labels:    map[string]string{},
					ClusterID: &clusterID,
				},
			},
			Query: "error",
		},
	}

	handler := NewSearchHandler(svc)

	body := searchBody(t, map[string]any{
		"service": "api",
		"keyword": "error",
		"start":   time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"end":     time.Now().Format(time.RFC3339),
	})
	req := httptest.NewRequest("POST", "/api/v1/search", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	resp := parseSearchResp(t, rr)
	data := resp["data"].(map[string]any)
	results := data["results"].([]any)
	first := results[0].(map[string]any)
	if first["cluster_id"] == nil {
		t.Error("expected cluster_id in result")
	}
}

func TestSearchHandler_LevelsPassedThrough(t *testing.T) {
	svc := &mockSearcher{result: &SearchResult{Results: []SearchResultLine{}, Query: "test"}}

	handler := NewSearchHandler(svc)

	body := searchBody(t, map[string]any{
		"service": "api",
		"keyword": "test",
		"levels":  []string{"ERROR", "WARN"},
		"start":   time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"end":     time.Now().Format(time.RFC3339),
	})
	req := httptest.NewRequest("POST", "/api/v1/search", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if len(svc.captured.Levels) != 2 {
		t.Errorf("expected 2 levels, got %d", len(svc.captured.Levels))
	}
}

func TestSearchHandler_KeywordValidPrintable(t *testing.T) {
	// 200 chars exactly should pass
	svc := &mockSearcher{result: &SearchResult{Results: []SearchResultLine{}, Query: strings.Repeat("a", 200)}}
	handler := NewSearchHandler(svc)

	body := searchBody(t, map[string]any{
		"service": "api",
		"keyword": strings.Repeat("a", 200),
		"start":   time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"end":     time.Now().Format(time.RFC3339),
	})
	req := httptest.NewRequest("POST", "/api/v1/search", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for 200-char keyword, got %d", rr.Code)
	}
}

func TestSearchHandler_EmptyKeywordAllowed(t *testing.T) {
	svc := &mockSearcher{result: &SearchResult{Results: []SearchResultLine{}, Query: ""}}
	handler := NewSearchHandler(svc)

	body := searchBody(t, map[string]any{
		"service": "api",
		"start":   time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"end":     time.Now().Format(time.RFC3339),
	})
	req := httptest.NewRequest("POST", "/api/v1/search", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty keyword (browse mode), got %d: %s", rr.Code, rr.Body.String())
	}
}
