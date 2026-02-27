package vllm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/kiranshivaraju/loghunter/internal/ai/shared"
	"github.com/kiranshivaraju/loghunter/internal/config"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// Provider implements models.AIProvider using vLLM's OpenAI-compatible endpoint.
type Provider struct {
	cfg    config.VLLMConfig
	client *http.Client
}

// NewProvider creates a new vLLM AI provider.
func NewProvider(cfg config.VLLMConfig) *Provider {
	return &Provider{
		cfg:    cfg,
		client: &http.Client{},
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "vllm" }

// Analyze performs root cause analysis on an error cluster via vLLM.
func (p *Provider) Analyze(ctx context.Context, req models.AnalysisRequest) (models.AnalysisResult, error) {
	prompt, err := shared.BuildAnalyzePrompt(req)
	if err != nil {
		return models.AnalysisResult{}, fmt.Errorf("building prompt: %w", err)
	}

	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/v1/chat/completions"
	content, err := shared.OpenAIChat(ctx, p.client, url, p.cfg.Model, prompt, nil)
	if err != nil {
		return models.AnalysisResult{}, err
	}

	var parsed shared.AnalysisJSON
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return models.AnalysisResult{}, fmt.Errorf("%w: %v", shared.ErrInvalidResponse, err)
	}

	return parsed.ToResult("vllm", p.cfg.Model), nil
}

// Summarize condenses log lines into a plain-language summary via vLLM.
func (p *Provider) Summarize(ctx context.Context, logs []models.LogLine) (string, error) {
	prompt, err := shared.BuildSummarizePrompt(logs)
	if err != nil {
		return "", fmt.Errorf("building prompt: %w", err)
	}

	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/v1/chat/completions"
	content, err := shared.OpenAIChat(ctx, p.client, url, p.cfg.Model, prompt, nil)
	if err != nil {
		return "", err
	}

	return content, nil
}

var _ models.AIProvider = (*Provider)(nil)
