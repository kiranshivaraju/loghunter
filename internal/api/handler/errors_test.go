package handler

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/kiranshivaraju/loghunter/internal/ai"
	"github.com/kiranshivaraju/loghunter/internal/loki"
	"github.com/kiranshivaraju/loghunter/internal/store"
)

func TestMapError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{
			name:       "loki unreachable",
			err:        loki.ErrLokiUnreachable,
			wantStatus: http.StatusBadGateway,
			wantCode:   "LOKI_UNREACHABLE",
			wantMsg:    "Loki is unreachable",
		},
		{
			name:       "loki query error",
			err:        loki.ErrLokiQueryError,
			wantStatus: http.StatusBadGateway,
			wantCode:   "LOKI_QUERY_ERROR",
			wantMsg:    "Loki query failed",
		},
		{
			name:       "ai provider unavailable",
			err:        ai.ErrProviderUnavailable,
			wantStatus: http.StatusBadGateway,
			wantCode:   "AI_PROVIDER_UNAVAILABLE",
			wantMsg:    "The AI provider is not available",
		},
		{
			name:       "ai inference timeout",
			err:        ai.ErrInferenceTimeout,
			wantStatus: http.StatusGatewayTimeout,
			wantCode:   "AI_INFERENCE_TIMEOUT",
			wantMsg:    "AI inference timed out",
		},
		{
			name:       "store not found",
			err:        store.ErrNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "RESOURCE_NOT_FOUND",
			wantMsg:    "Resource not found",
		},
		{
			name:       "no logs found",
			err:        ai.ErrNoLogsFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "NO_LOGS_FOUND",
			wantMsg:    "No logs found for the given parameters",
		},
		{
			name:       "unknown error",
			err:        errors.New("something unexpected"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
			wantMsg:    "An unexpected error occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code, msg := mapError(tt.err)
			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if code != tt.wantCode {
				t.Errorf("code = %q, want %q", code, tt.wantCode)
			}
			if msg != tt.wantMsg {
				t.Errorf("msg = %q, want %q", msg, tt.wantMsg)
			}
		})
	}
}

func TestMapError_WrappedErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "wrapped loki unreachable",
			err:        fmt.Errorf("query failed: %w", loki.ErrLokiUnreachable),
			wantStatus: http.StatusBadGateway,
			wantCode:   "LOKI_UNREACHABLE",
		},
		{
			name:       "wrapped ai provider unavailable",
			err:        fmt.Errorf("analysis failed: %w", ai.ErrProviderUnavailable),
			wantStatus: http.StatusBadGateway,
			wantCode:   "AI_PROVIDER_UNAVAILABLE",
		},
		{
			name:       "wrapped store not found",
			err:        fmt.Errorf("lookup: %w", store.ErrNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "RESOURCE_NOT_FOUND",
		},
		{
			name:       "double wrapped inference timeout",
			err:        fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", ai.ErrInferenceTimeout)),
			wantStatus: http.StatusGatewayTimeout,
			wantCode:   "AI_INFERENCE_TIMEOUT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code, _ := mapError(tt.err)
			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if code != tt.wantCode {
				t.Errorf("code = %q, want %q", code, tt.wantCode)
			}
		})
	}
}

func TestMapError_NilError(t *testing.T) {
	status, code, msg := mapError(nil)
	if status != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", status, http.StatusInternalServerError)
	}
	if code != "INTERNAL_ERROR" {
		t.Errorf("code = %q, want %q", code, "INTERNAL_ERROR")
	}
	if msg != "An unexpected error occurred" {
		t.Errorf("msg = %q, want %q", msg, "An unexpected error occurred")
	}
}
