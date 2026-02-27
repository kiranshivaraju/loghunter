package ollama

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
	return NewProvider(config.OllamaConfig{
		BaseURL: baseURL,
		Model:   "llama3",
	})
}

func TestAnalyze_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected /api/chat, got %s", r.URL.Path)
		}

		var req ollamaChatRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "llama3" {
			t.Errorf("expected model llama3, got %s", req.Model)
		}

		resp := ollamaChatResponse{
			Message: ollamaMessage{
				Role: "assistant",
				Content: `{
					"root_cause": "Database connection pool exhausted due to connection leak",
					"confidence": 0.87,
					"summary": "The payments-api service experienced connection pool exhaustion.",
					"suggested_action": "Increase max_open_conns and fix connection leak"
				}`,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	p := newTestProvider(ts.URL)
	result, err := p.Analyze(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RootCause != "Database connection pool exhausted due to connection leak" {
		t.Errorf("unexpected root cause: %s", result.RootCause)
	}
	if result.Confidence < 0.86 || result.Confidence > 0.88 {
		t.Errorf("unexpected confidence: %f", result.Confidence)
	}
	if result.Provider != "ollama" {
		t.Errorf("expected provider ollama, got %s", result.Provider)
	}
	if result.Model != "llama3" {
		t.Errorf("expected model llama3, got %s", result.Model)
	}
	if result.SuggestedAction == nil {
		t.Error("expected suggested action to be set")
	}
}

func TestSummarize_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaChatResponse{
			Message: ollamaMessage{
				Role:    "assistant",
				Content: "The service experienced intermittent connection failures.",
			},
		}
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
		resp := ollamaChatResponse{
			Message: ollamaMessage{
				Role:    "assistant",
				Content: "This is not valid JSON at all",
			},
		}
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
		w.Write([]byte("internal server error"))
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
		w.WriteHeader(http.StatusOK)
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

func TestAnalyze_ConnectionRefused(t *testing.T) {
	p := newTestProvider("http://localhost:1") // nothing listening
	_, err := p.Analyze(context.Background(), sampleRequest())
	if !errors.Is(err, shared.ErrProviderUnavailable) {
		t.Errorf("expected ErrProviderUnavailable, got %v", err)
	}
}

func TestAnalyze_ConfidenceClamping(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaChatResponse{
			Message: ollamaMessage{
				Role: "assistant",
				Content: `{
					"root_cause": "test",
					"confidence": 1.5,
					"summary": "test summary"
				}`,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	p := newTestProvider(ts.URL)
	result, err := p.Analyze(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Confidence != 1.0 {
		t.Errorf("expected confidence clamped to 1.0, got %f", result.Confidence)
	}
}

func TestAnalyze_WhitespaceTrimming(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaChatResponse{
			Message: ollamaMessage{
				Role: "assistant",
				Content: `{
					"root_cause": "  root cause with spaces  ",
					"confidence": 0.5,
					"summary": "  summary with spaces  ",
					"suggested_action": "  action with spaces  "
				}`,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	p := newTestProvider(ts.URL)
	result, err := p.Analyze(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RootCause != "root cause with spaces" {
		t.Errorf("expected trimmed root cause, got %q", result.RootCause)
	}
	if result.Summary != "summary with spaces" {
		t.Errorf("expected trimmed summary, got %q", result.Summary)
	}
	if result.SuggestedAction == nil || *result.SuggestedAction != "action with spaces" {
		t.Errorf("expected trimmed suggested action")
	}
}

func TestName(t *testing.T) {
	p := NewProvider(config.OllamaConfig{})
	if p.Name() != "ollama" {
		t.Errorf("expected 'ollama', got %s", p.Name())
	}
}
