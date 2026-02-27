package shared

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ChatCompletionRequest is the OpenAI-compatible chat completions request.
type ChatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
}

// ChatMessage represents a single message in the OpenAI chat format.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse is the OpenAI-compatible chat completions response.
type ChatCompletionResponse struct {
	Choices []ChatChoice `json:"choices"`
}

// ChatChoice represents a single choice in the completion response.
type ChatChoice struct {
	Message ChatMessage `json:"message"`
}

// OpenAIChat sends an OpenAI-compatible chat completion request and returns the content.
// Used by both vLLM and OpenAI providers.
func OpenAIChat(ctx context.Context, client *http.Client, url, model, prompt string, headers map[string]string) (string, error) {
	body := ChatCompletionRequest{
		Model: model,
		Messages: []ChatMessage{
			{Role: "user", Content: prompt},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("%w: %v", ErrInferenceTimeout, ctx.Err())
		}
		return "", fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", fmt.Errorf("%w: rate limited (HTTP 429)", ErrProviderUnavailable)
	}
	if resp.StatusCode >= 500 {
		return "", fmt.Errorf("%w: HTTP %d", ErrProviderUnavailable, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("%w: HTTP %d: %s", ErrProviderUnavailable, resp.StatusCode, string(respBody))
	}

	var chatResp ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("%w: decoding response: %v", ErrInvalidResponse, err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("%w: no choices in response", ErrInvalidResponse)
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}
