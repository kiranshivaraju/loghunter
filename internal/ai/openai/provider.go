package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kiranshivaraju/loghunter/internal/ai/shared"
	"github.com/kiranshivaraju/loghunter/internal/config"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

const defaultBaseURL = "https://api.openai.com"

// Provider implements models.AIProvider using the OpenAI API.
type Provider struct {
	cfg     config.OpenAIConfig
	client  *http.Client
	baseURL string
}

// NewProvider creates a new OpenAI AI provider.
// API key is sourced from config (environment variable) — never hardcoded.
func NewProvider(cfg config.OpenAIConfig) *Provider {
	return &Provider{
		cfg:     cfg,
		client:  &http.Client{},
		baseURL: defaultBaseURL,
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "openai" }

// Analyze performs root cause analysis on an error cluster via OpenAI.
func (p *Provider) Analyze(ctx context.Context, req models.AnalysisRequest) (models.AnalysisResult, error) {
	prompt, err := shared.BuildAnalyzePrompt(req)
	if err != nil {
		return models.AnalysisResult{}, fmt.Errorf("building prompt: %w", err)
	}

	url := p.baseURL + "/v1/chat/completions"
	content, err := shared.OpenAIChat(ctx, p.client, url, p.cfg.Model, prompt, p.authHeaders())
	if err != nil {
		return models.AnalysisResult{}, err
	}

	var parsed shared.AnalysisJSON
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return models.AnalysisResult{}, fmt.Errorf("%w: %v", shared.ErrInvalidResponse, err)
	}

	return parsed.ToResult("openai", p.cfg.Model), nil
}

// Summarize condenses log lines into a plain-language summary via OpenAI.
func (p *Provider) Summarize(ctx context.Context, logs []models.LogLine) (string, error) {
	prompt, err := shared.BuildSummarizePrompt(logs)
	if err != nil {
		return "", fmt.Errorf("building prompt: %w", err)
	}

	url := p.baseURL + "/v1/chat/completions"
	content, err := shared.OpenAIChat(ctx, p.client, url, p.cfg.Model, prompt, p.authHeaders())
	if err != nil {
		return "", err
	}

	return content, nil
}

func (p *Provider) authHeaders() map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + p.cfg.APIKey,
	}
}

var _ models.AIProvider = (*Provider)(nil)
