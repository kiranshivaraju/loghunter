package handler

import (
	"errors"
	"net/http"

	"github.com/kiranshivaraju/loghunter/internal/ai"
	"github.com/kiranshivaraju/loghunter/internal/loki"
	"github.com/kiranshivaraju/loghunter/internal/store"
)

// mapError maps a service-layer error to an HTTP status code, error code, and message.
// Uses errors.Is for all checks to correctly handle wrapped errors.
func mapError(err error) (httpStatus int, code string, message string) {
	switch {
	case errors.Is(err, loki.ErrLokiUnreachable):
		return http.StatusBadGateway, "LOKI_UNREACHABLE", "Loki is unreachable"
	case errors.Is(err, loki.ErrLokiQueryError):
		return http.StatusBadGateway, "LOKI_QUERY_ERROR", "Loki query failed"
	case errors.Is(err, ai.ErrProviderUnavailable):
		return http.StatusBadGateway, "AI_PROVIDER_UNAVAILABLE", "The AI provider is not available"
	case errors.Is(err, ai.ErrInferenceTimeout):
		return http.StatusGatewayTimeout, "AI_INFERENCE_TIMEOUT", "AI inference timed out"
	case errors.Is(err, ai.ErrNoLogsFound):
		return http.StatusNotFound, "NO_LOGS_FOUND", "No logs found for the given parameters"
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound, "RESOURCE_NOT_FOUND", "Resource not found"
	default:
		return http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred"
	}
}
