package ai_test

import (
	"testing"

	"github.com/kiranshivaraju/loghunter/internal/ai"
	"github.com/kiranshivaraju/loghunter/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider_Ollama(t *testing.T) {
	cfg := config.AIConfig{
		Provider: "ollama",
		Ollama:   config.OllamaConfig{BaseURL: "http://localhost:11434", Model: "llama3"},
	}
	p, err := ai.NewProvider(cfg)
	require.NoError(t, err)
	assert.Equal(t, "ollama", p.Name())
}

func TestNewProvider_VLLM(t *testing.T) {
	cfg := config.AIConfig{
		Provider: "vllm",
		VLLM:     config.VLLMConfig{BaseURL: "http://localhost:8000", Model: "mistral-7b"},
	}
	p, err := ai.NewProvider(cfg)
	require.NoError(t, err)
	assert.Equal(t, "vllm", p.Name())
}

func TestNewProvider_OpenAI(t *testing.T) {
	cfg := config.AIConfig{
		Provider: "openai",
		OpenAI:   config.OpenAIConfig{APIKey: "sk-test", Model: "gpt-4"},
	}
	p, err := ai.NewProvider(cfg)
	require.NoError(t, err)
	assert.Equal(t, "openai", p.Name())
}

func TestNewProvider_Anthropic(t *testing.T) {
	cfg := config.AIConfig{
		Provider:  "anthropic",
		Anthropic: config.AnthropicConfig{APIKey: "sk-ant-test", Model: "claude-sonnet-4-5-20250929"},
	}
	p, err := ai.NewProvider(cfg)
	require.NoError(t, err)
	assert.Equal(t, "anthropic", p.Name())
}

func TestNewProvider_Unknown(t *testing.T) {
	cfg := config.AIConfig{Provider: "unknown-provider"}
	_, err := ai.NewProvider(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown AI provider")
	assert.Contains(t, err.Error(), "unknown-provider")
}

func TestNewProvider_Empty(t *testing.T) {
	cfg := config.AIConfig{Provider: ""}
	_, err := ai.NewProvider(cfg)
	require.Error(t, err)
}
