package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kiranshivaraju/loghunter/internal/ai/shared"
	"github.com/kiranshivaraju/loghunter/internal/config"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

const defaultBaseURL = "https://api.anthropic.com"

// anthropicRequest is the request body for the Anthropic Messages API.
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

// anthropicMessage represents a single message in the Anthropic format.
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicResponse is the response from the Anthropic Messages API.
type anthropicResponse struct {
	Content []anthropicContent `json:"content"`
}

// anthropicContent represents a content block in the Anthropic response.
type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Provider implements models.AIProvider using the Anthropic Messages API.
type Provider struct {
	cfg     config.AnthropicConfig
	client  *http.Client
	baseURL string
}

// NewProvider creates a new Anthropic AI provider.
// API key is sourced from config (environment variable) — never hardcoded.
func NewProvider(cfg config.AnthropicConfig) *Provider {
	return &Provider{
		cfg:     cfg,
		client:  &http.Client{},
		baseURL: defaultBaseURL,
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "anthropic" }

// Analyze performs root cause analysis on an error cluster via Anthropic.
func (p *Provider) Analyze(ctx context.Context, req models.AnalysisRequest) (models.AnalysisResult, error) {
	prompt, err := shared.BuildAnalyzePrompt(req)
	if err != nil {
		return models.AnalysisResult{}, fmt.Errorf("building prompt: %w", err)
	}

	content, err := p.chat(ctx, prompt)
	if err != nil {
		return models.AnalysisResult{}, err
	}

	var parsed shared.AnalysisJSON
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return models.AnalysisResult{}, fmt.Errorf("%w: %v", shared.ErrInvalidResponse, err)
	}

	return parsed.ToResult("anthropic", p.cfg.Model), nil
}

// Summarize condenses log lines into a plain-language summary via Anthropic.
func (p *Provider) Summarize(ctx context.Context, logs []models.LogLine) (string, error) {
	prompt, err := shared.BuildSummarizePrompt(logs)
	if err != nil {
		return "", fmt.Errorf("building prompt: %w", err)
	}

	content, err := p.chat(ctx, prompt)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(content), nil
}

// chat sends a message to the Anthropic Messages API and returns the response text.
func (p *Provider) chat(ctx context.Context, prompt string) (string, error) {
	body := anthropicRequest{
		Model:     p.cfg.Model,
		MaxTokens: 1024,
		Messages: []anthropicMessage{
			{Role: "user", Content: prompt},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	url := p.baseURL + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("%w: %v", shared.ErrInferenceTimeout, ctx.Err())
		}
		return "", fmt.Errorf("%w: %v", shared.ErrProviderUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return "", fmt.Errorf("%w: HTTP %d", shared.ErrProviderUnavailable, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("%w: HTTP %d: %s", shared.ErrProviderUnavailable, resp.StatusCode, string(respBody))
	}

	var anthropicResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
		return "", fmt.Errorf("%w: decoding response: %v", shared.ErrInvalidResponse, err)
	}

	if len(anthropicResp.Content) == 0 {
		return "", fmt.Errorf("%w: no content in response", shared.ErrInvalidResponse)
	}

	return strings.TrimSpace(anthropicResp.Content[0].Text), nil
}

var _ models.AIProvider = (*Provider)(nil)
