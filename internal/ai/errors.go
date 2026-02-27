package ai

import "github.com/kiranshivaraju/loghunter/internal/ai/shared"

// Re-export error sentinels from shared package for backwards compatibility.
var (
	ErrProviderUnavailable = shared.ErrProviderUnavailable
	ErrInferenceTimeout    = shared.ErrInferenceTimeout
	ErrInvalidResponse     = shared.ErrInvalidResponse
	ErrNoLogsFound         = shared.ErrNoLogsFound
)
