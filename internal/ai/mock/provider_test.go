package mock_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/ai"
	"github.com/kiranshivaraju/loghunter/internal/ai/mock"
	"github.com/kiranshivaraju/loghunter/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleRequest() models.AnalysisRequest {
	return models.AnalysisRequest{
		Cluster: models.ErrorCluster{
			ID:            uuid.New(),
			TenantID:      uuid.New(),
			Service:       "test-svc",
			Namespace:     "default",
			Fingerprint:   "fp-test",
			Level:         "ERROR",
			FirstSeenAt:   time.Now(),
			LastSeenAt:    time.Now(),
			Count:         5,
			SampleMessage: "NullPointerException",
		},
		ContextLogs: []models.LogLine{
			{Timestamp: time.Now(), Message: "error log", Level: "ERROR"},
		},
	}
}

func sampleLogs() []models.LogLine {
	return []models.LogLine{
		{Timestamp: time.Now(), Message: "log 1", Level: "INFO"},
		{Timestamp: time.Now(), Message: "log 2", Level: "ERROR"},
	}
}

// --- NewMockProvider ---

func TestNewMockProvider_Name(t *testing.T) {
	p := mock.NewMockProvider()
	assert.Equal(t, "mock", p.Name())
}

func TestNewMockProvider_Analyze(t *testing.T) {
	p := mock.NewMockProvider()
	result, err := p.Analyze(context.Background(), sampleRequest())

	require.NoError(t, err)
	assert.Equal(t, "mock", result.Provider)
	assert.Equal(t, "mock-v1", result.Model)
	assert.NotEmpty(t, result.RootCause)
	assert.InDelta(t, 0.85, result.Confidence, 0.001)
	assert.NotEmpty(t, result.Summary)
	assert.NotNil(t, result.SuggestedAction)
	assert.NotEqual(t, uuid.Nil, result.ID)
}

func TestNewMockProvider_Summarize(t *testing.T) {
	p := mock.NewMockProvider()
	summary, err := p.Summarize(context.Background(), sampleLogs())

	require.NoError(t, err)
	assert.NotEmpty(t, summary)
	assert.Contains(t, summary, "Mock summary")
}

// --- NewFailingProvider ---

func TestNewFailingProvider_Name(t *testing.T) {
	p := mock.NewFailingProvider(ai.ErrProviderUnavailable)
	assert.Equal(t, "mock-failing", p.Name())
}

func TestNewFailingProvider_Analyze(t *testing.T) {
	p := mock.NewFailingProvider(ai.ErrProviderUnavailable)
	_, err := p.Analyze(context.Background(), sampleRequest())

	assert.ErrorIs(t, err, ai.ErrProviderUnavailable)
}

func TestNewFailingProvider_Summarize(t *testing.T) {
	p := mock.NewFailingProvider(ai.ErrInvalidResponse)
	_, err := p.Summarize(context.Background(), sampleLogs())

	assert.ErrorIs(t, err, ai.ErrInvalidResponse)
}

func TestNewFailingProvider_CustomError(t *testing.T) {
	customErr := errors.New("custom AI error")
	p := mock.NewFailingProvider(customErr)

	_, err := p.Analyze(context.Background(), sampleRequest())
	assert.ErrorIs(t, err, customErr)

	_, err = p.Summarize(context.Background(), sampleLogs())
	assert.ErrorIs(t, err, customErr)
}

// --- NewTimeoutProvider ---

func TestNewTimeoutProvider_Name(t *testing.T) {
	p := mock.NewTimeoutProvider()
	assert.Equal(t, "mock-timeout", p.Name())
}

func TestNewTimeoutProvider_Analyze(t *testing.T) {
	p := mock.NewTimeoutProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := p.Analyze(ctx, sampleRequest())
	assert.ErrorIs(t, err, ai.ErrInferenceTimeout)
}

func TestNewTimeoutProvider_Summarize(t *testing.T) {
	p := mock.NewTimeoutProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := p.Summarize(ctx, sampleLogs())
	assert.ErrorIs(t, err, ai.ErrInferenceTimeout)
}

// --- Sentinel errors ---

func TestSentinelErrors(t *testing.T) {
	assert.NotNil(t, ai.ErrProviderUnavailable)
	assert.NotNil(t, ai.ErrInferenceTimeout)
	assert.NotNil(t, ai.ErrInvalidResponse)

	// Ensure they are distinct
	assert.NotEqual(t, ai.ErrProviderUnavailable, ai.ErrInferenceTimeout)
	assert.NotEqual(t, ai.ErrInferenceTimeout, ai.ErrInvalidResponse)
}

// --- Zero-value MockProvider ---

func TestMockProvider_NilFuncs(t *testing.T) {
	p := &mock.MockProvider{Name_: "bare"}

	result, err := p.Analyze(context.Background(), sampleRequest())
	assert.NoError(t, err)
	assert.Equal(t, models.AnalysisResult{}, result)

	summary, err := p.Summarize(context.Background(), sampleLogs())
	assert.NoError(t, err)
	assert.Equal(t, "", summary)
}

// --- Interface compliance ---

func TestMockProvider_ImplementsAIProvider(t *testing.T) {
	var _ models.AIProvider = mock.NewMockProvider()
	var _ models.AIProvider = mock.NewFailingProvider(nil)
	var _ models.AIProvider = mock.NewTimeoutProvider()
}
