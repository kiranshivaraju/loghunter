package openai

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
	p := NewProvider(config.OpenAIConfig{
		APIKey: "sk-test-key",
		Model:  "gpt-4",
	})
	p.baseURL = baseURL
	return p
}

func chatResponse(content string) shared.ChatCompletionResponse {
	return shared.ChatCompletionResponse{
		Choices: []shared.ChatChoice{
			{Message: shared.ChatMessage{Role: "assistant", Content: content}},
		},
	}
}

func TestAnalyze_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected /v1/chat/completions, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test-key" {
			t.Errorf("expected Bearer auth header, got %s", r.Header.Get("Authorization"))
		}

		resp := chatResponse(`{
			"root_cause": "Database connection pool exhausted",
			"confidence": 0.92,
			"summary": "Connection pool exhaustion caused service degradation.",
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
	if result.Provider != "openai" {
		t.Errorf("expected provider openai, got %s", result.Provider)
	}
	if result.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", result.Model)
	}
}

func TestSummarize_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse("The service experienced intermittent connection failures.")
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
		resp := chatResponse("not valid JSON")
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

func TestAnalyze_RateLimit429(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	p := newTestProvider(ts.URL)
	_, err := p.Analyze(context.Background(), sampleRequest())
	if !errors.Is(err, shared.ErrProviderUnavailable) {
		t.Errorf("expected ErrProviderUnavailable on 429, got %v", err)
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
	p := NewProvider(config.OpenAIConfig{})
	if p.Name() != "openai" {
		t.Errorf("expected 'openai', got %s", p.Name())
	}
}

func TestAPIKeyNeverHardcoded(t *testing.T) {
	// Verify the provider gets API key from config, not hardcoded
	p := NewProvider(config.OpenAIConfig{APIKey: "my-secret-key", Model: "gpt-4"})
	headers := p.authHeaders()
	if headers["Authorization"] != "Bearer my-secret-key" {
		t.Errorf("API key not sourced from config")
	}
}
