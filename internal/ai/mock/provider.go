package mock

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/ai"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// MockProvider satisfies models.AIProvider for testing.
type MockProvider struct {
	Name_         string
	AnalyzeFunc   func(ctx context.Context, req models.AnalysisRequest) (models.AnalysisResult, error)
	SummarizeFunc func(ctx context.Context, logs []models.LogLine) (string, error)
}

func (m *MockProvider) Name() string { return m.Name_ }

func (m *MockProvider) Analyze(ctx context.Context, req models.AnalysisRequest) (models.AnalysisResult, error) {
	if m.AnalyzeFunc != nil {
		return m.AnalyzeFunc(ctx, req)
	}
	return models.AnalysisResult{}, nil
}

func (m *MockProvider) Summarize(ctx context.Context, logs []models.LogLine) (string, error) {
	if m.SummarizeFunc != nil {
		return m.SummarizeFunc(ctx, logs)
	}
	return "", nil
}

// NewMockProvider returns a MockProvider with sensible default responses.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		Name_: "mock",
		AnalyzeFunc: func(_ context.Context, req models.AnalysisRequest) (models.AnalysisResult, error) {
			action := "Check application logs for more context"
			return models.AnalysisResult{
				ID:              uuid.New(),
				ClusterID:       req.Cluster.ID,
				TenantID:        req.Cluster.TenantID,
				Provider:        "mock",
				Model:           "mock-v1",
				RootCause:       "Simulated root cause from mock provider",
				Confidence:      0.85,
				Summary:         "Mock analysis summary for testing",
				SuggestedAction: &action,
				CreatedAt:       time.Now().UTC(),
			}, nil
		},
		SummarizeFunc: func(_ context.Context, logs []models.LogLine) (string, error) {
			return "Mock summary: processed log entries for testing", nil
		},
	}
}

// NewFailingProvider returns a MockProvider that always returns the given error.
func NewFailingProvider(err error) *MockProvider {
	return &MockProvider{
		Name_: "mock-failing",
		AnalyzeFunc: func(_ context.Context, _ models.AnalysisRequest) (models.AnalysisResult, error) {
			return models.AnalysisResult{}, err
		},
		SummarizeFunc: func(_ context.Context, _ []models.LogLine) (string, error) {
			return "", err
		},
	}
}

// NewTimeoutProvider returns a MockProvider that blocks until context is cancelled.
func NewTimeoutProvider() *MockProvider {
	return &MockProvider{
		Name_: "mock-timeout",
		AnalyzeFunc: func(ctx context.Context, _ models.AnalysisRequest) (models.AnalysisResult, error) {
			<-ctx.Done()
			return models.AnalysisResult{}, ai.ErrInferenceTimeout
		},
		SummarizeFunc: func(ctx context.Context, _ []models.LogLine) (string, error) {
			<-ctx.Done()
			return "", ai.ErrInferenceTimeout
		},
	}
}

// Compile-time check that MockProvider implements AIProvider.
var _ models.AIProvider = (*MockProvider)(nil)
