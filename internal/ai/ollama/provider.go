package ollama

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

// ollamaChatRequest is the request body for Ollama's /api/chat endpoint.
type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

// ollamaMessage represents a single message in the Ollama chat format.
type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaChatResponse is the response from Ollama's /api/chat endpoint.
type ollamaChatResponse struct {
	Message ollamaMessage `json:"message"`
}

// Provider implements models.AIProvider using Ollama's REST API.
type Provider struct {
	cfg    config.OllamaConfig
	client *http.Client
}

// NewProvider creates a new Ollama AI provider.
func NewProvider(cfg config.OllamaConfig) *Provider {
	return &Provider{
		cfg:    cfg,
		client: &http.Client{},
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "ollama" }

// Analyze performs root cause analysis on an error cluster via Ollama.
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

	return parsed.ToResult("ollama", p.cfg.Model), nil
}

// Summarize condenses log lines into a plain-language summary via Ollama.
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

// chat sends a chat request to Ollama and returns the assistant's response content.
func (p *Provider) chat(ctx context.Context, prompt string) (string, error) {
	body := ollamaChatRequest{
		Model: p.cfg.Model,
		Messages: []ollamaMessage{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

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

	var chatResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("%w: decoding response: %v", shared.ErrInvalidResponse, err)
	}

	return chatResp.Message.Content, nil
}

var _ models.AIProvider = (*Provider)(nil)
