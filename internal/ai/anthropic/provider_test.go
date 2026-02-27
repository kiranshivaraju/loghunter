package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/ai/shared"
	"github.com/kiranshivaraju/loghunter/internal/config"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

func sampleRequest() models.AnalysisRequest {
	return models.AnalysisRequest{
		Cluster: models.ErrorCluster{
			ID:            uuid.New(),
			TenantID:      uuid.New(),
			Service:       "payments-api",
			Namespace:     "default",
			Fingerprint:   "fp-test",
			Level:         "ERROR",
			FirstSeenAt:   time.Now(),
			LastSeenAt:    time.Now(),
			Count:         5,
			SampleMessage: "connection pool exhausted",
		},
		ContextLogs: []models.LogLine{
			{Timestamp: time.Now(), Message: "connection pool exhausted", Level: "ERROR"},
		},
	}
}

func sampleLogs() []models.LogLine {
	return []models.LogLine{
		{Timestamp: time.Now(), Message: "Starting server", Level: "INFO"},
		{Timestamp: time.Now(), Message: "connection refused", Level: "ERROR"},
	}
}

func newTestProvider(baseURL string) *Provider {
	p := NewProvider(config.AnthropicConfig{
		APIKey: "sk-ant-test-key",
		Model:  "claude-sonnet-4-5-20250929",
	})
	p.baseURL = baseURL
	return p
}

func anthropicResp(text string) anthropicResponse {
	return anthropicResponse{
		Content: []anthropicContent{
			{Type: "text", Text: text},
		},
	}
}

func TestAnalyze_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages, got %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "sk-ant-test-key" {
			t.Errorf("expected x-api-key header, got %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("expected anthropic-version header, got %s", r.Header.Get("anthropic-version"))
		}

		var req anthropicRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "claude-sonnet-4-5-20250929" {
			t.Errorf("unexpected model: %s", req.Model)
		}
		if req.MaxTokens != 1024 {
			t.Errorf("expected max_tokens 1024, got %d", req.MaxTokens)
		}

		resp := anthropicResp(`{
			"root_cause": "Database connection pool exhausted",
			"confidence": 0.91,
			"summary": "Connection pool exhaustion caused failures.",
			"suggested_action": "Increase pool size"
		}`)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	p := newTestProvider(ts.URL)
	result, err := p.Analyze(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RootCause != "Database connection pool exhausted" {
		t.Errorf("unexpected root cause: %s", result.RootCause)
	}
	if result.Provider != "anthropic" {
		t.Errorf("expected provider anthropic, got %s", result.Provider)
	}
	if result.Model != "claude-sonnet-4-5-20250929" {
		t.Errorf("unexpected model: %s", result.Model)
	}
}

func TestSummarize_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := anthropicResp("The service experienced intermittent connection failures.")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	p := newTestProvider(ts.URL)
	summary, err := p.Summarize(context.Background(), sampleLogs())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary != "The service experienced intermittent connection failures." {
		t.Errorf("unexpected summary: %s", summary)
	}
}

func TestAnalyze_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := anthropicResp("not valid JSON")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	p := newTestProvider(ts.URL)
	_, err := p.Analyze(context.Background(), sampleRequest())
	if !errors.Is(err, shared.ErrInvalidResponse) {
		t.Errorf("expected ErrInvalidResponse, got %v", err)
	}
}

func TestAnalyze_HTTP500(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	p := newTestProvider(ts.URL)
	_, err := p.Analyze(context.Background(), sampleRequest())
	if !errors.Is(err, shared.ErrProviderUnavailable) {
		t.Errorf("expected ErrProviderUnavailable, got %v", err)
	}
}

func TestAnalyze_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer ts.Close()

	p := newTestProvider(ts.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := p.Analyze(ctx, sampleRequest())
	if !errors.Is(err, shared.ErrInferenceTimeout) {
		t.Errorf("expected ErrInferenceTimeout, got %v", err)
	}
}

func TestName(t *testing.T) {
	p := NewProvider(config.AnthropicConfig{})
	if p.Name() != "anthropic" {
		t.Errorf("expected 'anthropic', got %s", p.Name())
	}
}

func TestAnalyze_EmptyContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := anthropicResponse{Content: []anthropicContent{}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	p := newTestProvider(ts.URL)
	_, err := p.Analyze(context.Background(), sampleRequest())
	if !errors.Is(err, shared.ErrInvalidResponse) {
		t.Errorf("expected ErrInvalidResponse for empty content, got %v", err)
	}
}
