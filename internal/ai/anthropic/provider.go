package anthropic

import (
	"context"

	"github.com/kiranshivaraju/loghunter/internal/config"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// Provider implements models.AIProvider using Anthropic.
type Provider struct {
	cfg config.AnthropicConfig
}

func NewProvider(cfg config.AnthropicConfig) *Provider {
	return &Provider{cfg: cfg}
}

func (p *Provider) Name() string { return "anthropic" }

func (p *Provider) Analyze(_ context.Context, _ models.AnalysisRequest) (models.AnalysisResult, error) {
	panic("anthropic.Provider.Analyze not yet implemented")
}

func (p *Provider) Summarize(_ context.Context, _ []models.LogLine) (string, error) {
	panic("anthropic.Provider.Summarize not yet implemented")
}

var _ models.AIProvider = (*Provider)(nil)
