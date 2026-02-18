package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/ai"
	mw "github.com/kiranshivaraju/loghunter/internal/api/middleware"
)

func setTenantCtx(ctx context.Context, id uuid.UUID) context.Context {
	return mw.SetTenantID(ctx, id)
}

// --- mock Summarizer ---

type mockSummarizer struct {
	fn func(params SummarizeParams) (*SummarizeResult, error)
}

func (m *mockSummarizer) Summarize(params SummarizeParams) (*SummarizeResult, error) {
	return m.fn(params)
}

func successSummarizer() *mockSummarizer {
	return &mockSummarizer{fn: func(params SummarizeParams) (*SummarizeResult, error) {
		return &SummarizeResult{
			Summary:       "Summary of log stream for testing",
			LinesAnalyzed: 42,
			From:          params.Start,
			To:            params.End,
			Provider:      "mock",
			Model:         "mock-v1",
		}, nil
	}}
}

// --- helpers ---

func summarizeReq(t *testing.T, body any, tenantID uuid.UUID) *http.Request {
	t.Helper()
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	return r.WithContext(setTenantCtx(r.Context(), tenantID))
}

func parseSummarizeOK(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return env.Data
}

func parseSummarizeErr(t *testing.T, rec *httptest.ResponseRecorder) (int, string) {
	t.Helper()
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return rec.Code, env.Error.Code
}

// --- tests ---

func TestSummarizeHandler_Success(t *testing.T) {
	h := NewSummarizeHandler(successSummarizer())
	rec := httptest.NewRecorder()
	tid := uuid.New()

	body := map[string]any{
		"service": "payments-api",
		"start":   "2024-02-17T00:00:00Z",
		"end":     "2024-02-17T01:00:00Z",
	}
	h.ServeHTTP(rec, summarizeReq(t, body, tid))

	data := parseSummarizeOK(t, rec)
	if data["summary"] != "Summary of log stream for testing" {
		t.Errorf("unexpected summary: %v", data["summary"])
	}
	if data["provider"] != "mock" {
		t.Errorf("unexpected provider: %v", data["provider"])
	}
}

func TestSummarizeHandler_DefaultNamespace(t *testing.T) {
	var captured SummarizeParams
	mock := &mockSummarizer{fn: func(params SummarizeParams) (*SummarizeResult, error) {
		captured = params
		return &SummarizeResult{Summary: "ok"}, nil
	}}

	h := NewSummarizeHandler(mock)
	rec := httptest.NewRecorder()

	body := map[string]any{
		"service": "svc",
		"start":   "2024-02-17T00:00:00Z",
		"end":     "2024-02-17T01:00:00Z",
	}
	h.ServeHTTP(rec, summarizeReq(t, body, uuid.New()))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if captured.Namespace != "default" {
		t.Errorf("expected namespace 'default', got %q", captured.Namespace)
	}
}

func TestSummarizeHandler_DefaultMaxLines(t *testing.T) {
	var captured SummarizeParams
	mock := &mockSummarizer{fn: func(params SummarizeParams) (*SummarizeResult, error) {
		captured = params
		return &SummarizeResult{Summary: "ok"}, nil
	}}

	h := NewSummarizeHandler(mock)
	rec := httptest.NewRecorder()

	body := map[string]any{
		"service": "svc",
		"start":   "2024-02-17T00:00:00Z",
		"end":     "2024-02-17T01:00:00Z",
	}
	h.ServeHTTP(rec, summarizeReq(t, body, uuid.New()))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if captured.MaxLines != 500 {
		t.Errorf("expected max_lines 500, got %d", captured.MaxLines)
	}
}

func TestSummarizeHandler_MaxLinesClamping(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{"below minimum", 5, 10},
		{"at minimum", 10, 10},
		{"normal", 200, 200},
		{"at maximum", 1000, 1000},
		{"above maximum", 2000, 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured SummarizeParams
			mock := &mockSummarizer{fn: func(params SummarizeParams) (*SummarizeResult, error) {
				captured = params
				return &SummarizeResult{Summary: "ok"}, nil
			}}

			h := NewSummarizeHandler(mock)
			rec := httptest.NewRecorder()

			body := map[string]any{
				"service":   "svc",
				"start":     "2024-02-17T00:00:00Z",
				"end":       "2024-02-17T01:00:00Z",
				"max_lines": tt.input,
			}
			h.ServeHTTP(rec, summarizeReq(t, body, uuid.New()))

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if captured.MaxLines != tt.expected {
				t.Errorf("expected max_lines %d, got %d", tt.expected, captured.MaxLines)
			}
		})
	}
}

func TestSummarizeHandler_MissingService(t *testing.T) {
	h := NewSummarizeHandler(successSummarizer())
	rec := httptest.NewRecorder()

	body := map[string]any{
		"start": "2024-02-17T00:00:00Z",
		"end":   "2024-02-17T01:00:00Z",
	}
	h.ServeHTTP(rec, summarizeReq(t, body, uuid.New()))

	status, code := parseSummarizeErr(t, rec)
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
	if code != "INVALID_REQUEST" {
		t.Errorf("expected INVALID_REQUEST, got %s", code)
	}
}

func TestSummarizeHandler_MissingStart(t *testing.T) {
	h := NewSummarizeHandler(successSummarizer())
	rec := httptest.NewRecorder()

	body := map[string]any{
		"service": "svc",
		"end":     "2024-02-17T01:00:00Z",
	}
	h.ServeHTTP(rec, summarizeReq(t, body, uuid.New()))

	status, code := parseSummarizeErr(t, rec)
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
	if code != "INVALID_REQUEST" {
		t.Errorf("expected INVALID_REQUEST, got %s", code)
	}
}

func TestSummarizeHandler_MissingEnd(t *testing.T) {
	h := NewSummarizeHandler(successSummarizer())
	rec := httptest.NewRecorder()

	body := map[string]any{
		"service": "svc",
		"start":   "2024-02-17T00:00:00Z",
	}
	h.ServeHTTP(rec, summarizeReq(t, body, uuid.New()))

	status, code := parseSummarizeErr(t, rec)
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
	if code != "INVALID_REQUEST" {
		t.Errorf("expected INVALID_REQUEST, got %s", code)
	}
}

func TestSummarizeHandler_InvalidJSON(t *testing.T) {
	h := NewSummarizeHandler(successSummarizer())
	rec := httptest.NewRecorder()

	r := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", bytes.NewReader([]byte("{invalid")))
	r = r.WithContext(setTenantCtx(r.Context(), uuid.New()))

	h.ServeHTTP(rec, r)

	status, code := parseSummarizeErr(t, rec)
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}
	if code != "INVALID_REQUEST" {
		t.Errorf("expected INVALID_REQUEST, got %s", code)
	}
}

func TestSummarizeHandler_NoTenant(t *testing.T) {
	h := NewSummarizeHandler(successSummarizer())
	rec := httptest.NewRecorder()

	body := map[string]any{
		"service": "svc",
		"start":   "2024-02-17T00:00:00Z",
		"end":     "2024-02-17T01:00:00Z",
	}
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/summarize", bytes.NewReader(b))
	// No tenant context set

	h.ServeHTTP(rec, r)

	status, code := parseSummarizeErr(t, rec)
	if status != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", status)
	}
	if code != "INVALID_TOKEN" {
		t.Errorf("expected INVALID_TOKEN, got %s", code)
	}
}

func TestSummarizeHandler_NoLogsFound(t *testing.T) {
	mock := &mockSummarizer{fn: func(_ SummarizeParams) (*SummarizeResult, error) {
		return nil, ErrNoLogsFound
	}}

	h := NewSummarizeHandler(mock)
	rec := httptest.NewRecorder()

	body := map[string]any{
		"service": "svc",
		"start":   "2024-02-17T00:00:00Z",
		"end":     "2024-02-17T01:00:00Z",
	}
	h.ServeHTTP(rec, summarizeReq(t, body, uuid.New()))

	status, code := parseSummarizeErr(t, rec)
	if status != http.StatusNotFound {
		t.Errorf("expected 404, got %d", status)
	}
	if code != "NO_LOGS_FOUND" {
		t.Errorf("expected NO_LOGS_FOUND, got %s", code)
	}
}

func TestSummarizeHandler_ProviderUnavailable(t *testing.T) {
	mock := &mockSummarizer{fn: func(_ SummarizeParams) (*SummarizeResult, error) {
		return nil, ai.ErrProviderUnavailable
	}}

	h := NewSummarizeHandler(mock)
	rec := httptest.NewRecorder()

	body := map[string]any{
		"service": "svc",
		"start":   "2024-02-17T00:00:00Z",
		"end":     "2024-02-17T01:00:00Z",
	}
	h.ServeHTTP(rec, summarizeReq(t, body, uuid.New()))

	status, code := parseSummarizeErr(t, rec)
	if status != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", status)
	}
	if code != "AI_PROVIDER_UNAVAILABLE" {
		t.Errorf("expected AI_PROVIDER_UNAVAILABLE, got %s", code)
	}
}

func TestSummarizeHandler_InferenceTimeout(t *testing.T) {
	mock := &mockSummarizer{fn: func(_ SummarizeParams) (*SummarizeResult, error) {
		return nil, ai.ErrInferenceTimeout
	}}

	h := NewSummarizeHandler(mock)
	rec := httptest.NewRecorder()

	body := map[string]any{
		"service": "svc",
		"start":   "2024-02-17T00:00:00Z",
		"end":     "2024-02-17T01:00:00Z",
	}
	h.ServeHTTP(rec, summarizeReq(t, body, uuid.New()))

	status, code := parseSummarizeErr(t, rec)
	if status != http.StatusGatewayTimeout {
		t.Errorf("expected 504, got %d", status)
	}
	if code != "AI_INFERENCE_TIMEOUT" {
		t.Errorf("expected AI_INFERENCE_TIMEOUT, got %s", code)
	}
}

func TestSummarizeHandler_UnexpectedError(t *testing.T) {
	mock := &mockSummarizer{fn: func(_ SummarizeParams) (*SummarizeResult, error) {
		return nil, errors.New("something went wrong")
	}}

	h := NewSummarizeHandler(mock)
	rec := httptest.NewRecorder()

	body := map[string]any{
		"service": "svc",
		"start":   "2024-02-17T00:00:00Z",
		"end":     "2024-02-17T01:00:00Z",
	}
	h.ServeHTTP(rec, summarizeReq(t, body, uuid.New()))

	status, code := parseSummarizeErr(t, rec)
	if status != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", status)
	}
	if code != "INTERNAL_ERROR" {
		t.Errorf("expected INTERNAL_ERROR, got %s", code)
	}
}

func TestSummarizeHandler_TenantIDPassedThrough(t *testing.T) {
	tid := uuid.New()
	var captured SummarizeParams
	mock := &mockSummarizer{fn: func(params SummarizeParams) (*SummarizeResult, error) {
		captured = params
		return &SummarizeResult{Summary: "ok"}, nil
	}}

	h := NewSummarizeHandler(mock)
	rec := httptest.NewRecorder()

	body := map[string]any{
		"service":   "payments-api",
		"namespace": "prod",
		"start":     "2024-02-17T00:00:00Z",
		"end":       "2024-02-17T01:00:00Z",
		"max_lines": 100,
	}
	h.ServeHTTP(rec, summarizeReq(t, body, tid))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if captured.TenantID != tid {
		t.Errorf("expected tenant %s, got %s", tid, captured.TenantID)
	}
	if captured.Service != "payments-api" {
		t.Errorf("expected service payments-api, got %s", captured.Service)
	}
	if captured.Namespace != "prod" {
		t.Errorf("expected namespace prod, got %s", captured.Namespace)
	}
	if captured.MaxLines != 100 {
		t.Errorf("expected max_lines 100, got %d", captured.MaxLines)
	}
}

func TestSummarizeHandler_ResponseShape(t *testing.T) {
	from := time.Date(2024, 2, 17, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 2, 17, 1, 0, 0, 0, time.UTC)
	mock := &mockSummarizer{fn: func(_ SummarizeParams) (*SummarizeResult, error) {
		return &SummarizeResult{
			Summary:       "Test summary",
			LinesAnalyzed: 150,
			From:          from,
			To:            to,
			Provider:      "ollama",
			Model:         "llama3",
		}, nil
	}}

	h := NewSummarizeHandler(mock)
	rec := httptest.NewRecorder()

	body := map[string]any{
		"service": "svc",
		"start":   "2024-02-17T00:00:00Z",
		"end":     "2024-02-17T01:00:00Z",
	}
	h.ServeHTTP(rec, summarizeReq(t, body, uuid.New()))

	data := parseSummarizeOK(t, rec)

	if data["summary"] != "Test summary" {
		t.Errorf("unexpected summary: %v", data["summary"])
	}
	if int(data["lines_analyzed"].(float64)) != 150 {
		t.Errorf("unexpected lines_analyzed: %v", data["lines_analyzed"])
	}
	if data["provider"] != "ollama" {
		t.Errorf("unexpected provider: %v", data["provider"])
	}
	if data["model"] != "llama3" {
		t.Errorf("unexpected model: %v", data["model"])
	}

	tr, ok := data["time_range"].(map[string]any)
	if !ok {
		t.Fatalf("time_range not a map: %v", data["time_range"])
	}
	if tr["from"] != "2024-02-17T00:00:00Z" {
		t.Errorf("unexpected from: %v", tr["from"])
	}
	if tr["to"] != "2024-02-17T01:00:00Z" {
		t.Errorf("unexpected to: %v", tr["to"])
	}
}
