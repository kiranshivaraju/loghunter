package ai

import (
	"fmt"

	"github.com/kiranshivaraju/loghunter/internal/ai/anthropic"
	"github.com/kiranshivaraju/loghunter/internal/ai/ollama"
	"github.com/kiranshivaraju/loghunter/internal/ai/openai"
	"github.com/kiranshivaraju/loghunter/internal/ai/vllm"
	"github.com/kiranshivaraju/loghunter/internal/config"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// NewProvider constructs the appropriate AI provider based on config.
// Called once at server startup.
func NewProvider(cfg config.AIConfig) (models.AIProvider, error) {
	switch cfg.Provider {
	case "ollama":
		return ollama.NewProvider(cfg.Ollama), nil
	case "vllm":
		return vllm.NewProvider(cfg.VLLM), nil
	case "openai":
		return openai.NewProvider(cfg.OpenAI), nil
	case "anthropic":
		return anthropic.NewProvider(cfg.Anthropic), nil
	default:
		return nil, fmt.Errorf("unknown AI provider %q: must be one of ollama, vllm, openai, anthropic", cfg.Provider)
	}
}
