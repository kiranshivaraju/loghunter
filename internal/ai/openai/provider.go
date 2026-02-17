package openai

import (
	"context"

	"github.com/kiranshivaraju/loghunter/internal/config"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// Provider implements models.AIProvider using OpenAI.
type Provider struct {
	cfg config.OpenAIConfig
}

func NewProvider(cfg config.OpenAIConfig) *Provider {
	return &Provider{cfg: cfg}
}

func (p *Provider) Name() string { return "openai" }

func (p *Provider) Analyze(_ context.Context, _ models.AnalysisRequest) (models.AnalysisResult, error) {
	panic("openai.Provider.Analyze not yet implemented")
}

func (p *Provider) Summarize(_ context.Context, _ []models.LogLine) (string, error) {
	panic("openai.Provider.Summarize not yet implemented")
}

var _ models.AIProvider = (*Provider)(nil)
