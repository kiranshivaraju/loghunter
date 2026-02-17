package vllm

import (
	"context"

	"github.com/kiranshivaraju/loghunter/internal/config"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// Provider implements models.AIProvider using vLLM.
type Provider struct {
	cfg config.VLLMConfig
}

func NewProvider(cfg config.VLLMConfig) *Provider {
	return &Provider{cfg: cfg}
}

func (p *Provider) Name() string { return "vllm" }

func (p *Provider) Analyze(_ context.Context, _ models.AnalysisRequest) (models.AnalysisResult, error) {
	panic("vllm.Provider.Analyze not yet implemented")
}

func (p *Provider) Summarize(_ context.Context, _ []models.LogLine) (string, error) {
	panic("vllm.Provider.Summarize not yet implemented")
}

var _ models.AIProvider = (*Provider)(nil)
