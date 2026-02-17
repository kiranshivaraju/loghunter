package ai

import "errors"

var (
	ErrProviderUnavailable = errors.New("ai provider unavailable")
	ErrInferenceTimeout    = errors.New("ai inference timeout")
	ErrInvalidResponse     = errors.New("ai provider returned invalid response")
)
