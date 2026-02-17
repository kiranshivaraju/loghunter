package ollama

import (
	"context"

	"github.com/kiranshivaraju/loghunter/internal/config"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// Provider implements models.AIProvider using Ollama.
type Provider struct {
	cfg config.OllamaConfig
}

func NewProvider(cfg config.OllamaConfig) *Provider {
	return &Provider{cfg: cfg}
}

func (p *Provider) Name() string { return "ollama" }

func (p *Provider) Analyze(_ context.Context, _ models.AnalysisRequest) (models.AnalysisResult, error) {
	panic("ollama.Provider.Analyze not yet implemented")
}

func (p *Provider) Summarize(_ context.Context, _ []models.LogLine) (string, error) {
	panic("ollama.Provider.Summarize not yet implemented")
}

var _ models.AIProvider = (*Provider)(nil)
