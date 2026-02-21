package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/ai"
	mw "github.com/kiranshivaraju/loghunter/internal/api/middleware"
	"github.com/kiranshivaraju/loghunter/internal/api/response"
)

// ErrNoLogsFound is returned when no logs match the query parameters.
var ErrNoLogsFound = ai.ErrNoLogsFound

// SummarizeParams holds validated parameters for a summarization request.
type SummarizeParams struct {
	TenantID  uuid.UUID
	Service   string
	Namespace string
	Start     time.Time
	End       time.Time
	MaxLines  int
}

// SummarizeResult is the output of a summarization operation.
type SummarizeResult struct {
	Summary       string    `json:"summary"`
	LinesAnalyzed int       `json:"lines_analyzed"`
	From          time.Time `json:"from"`
	To            time.Time `json:"to"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
}

// Summarizer defines the interface the handler depends on.
type Summarizer interface {
	Summarize(params SummarizeParams) (*SummarizeResult, error)
}

// NewSummarizeHandler returns an http.HandlerFunc for POST /api/v1/summarize.
func NewSummarizeHandler(svc Summarizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := mw.GetTenantID(r)
		if !ok {
			response.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Missing tenant", nil)
			return
		}

		var req struct {
			Service   string `json:"service"`
			Namespace string `json:"namespace"`
			Start     string `json:"start"`
			End       string `json:"end"`
			MaxLines  int    `json:"max_lines"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", nil)
			return
		}

		if req.Service == "" {
			response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "service is required", nil)
			return
		}

		if req.Start == "" {
			response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "start is required", nil)
			return
		}
		startTime, err := time.Parse(time.RFC3339, req.Start)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "start must be a valid RFC3339 timestamp", nil)
			return
		}

		if req.End == "" {
			response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "end is required", nil)
			return
		}
		endTime, err := time.Parse(time.RFC3339, req.End)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "end must be a valid RFC3339 timestamp", nil)
			return
		}

		ns := req.Namespace
		if ns == "" {
			ns = "default"
		}

		maxLines := req.MaxLines
		if maxLines == 0 {
			maxLines = 500
		}
		if maxLines < 10 {
			maxLines = 10
		}
		if maxLines > 1000 {
			maxLines = 1000
		}

		result, err := svc.Summarize(SummarizeParams{
			TenantID:  tenantID,
			Service:   req.Service,
			Namespace: ns,
			Start:     startTime,
			End:       endTime,
			MaxLines:  maxLines,
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrNoLogsFound):
				response.Error(w, http.StatusNotFound, "NO_LOGS_FOUND",
					"No logs found for the given parameters", nil)
			case errors.Is(err, ai.ErrProviderUnavailable):
				response.Error(w, http.StatusBadGateway, "AI_PROVIDER_UNAVAILABLE",
					"The AI provider is not available", nil)
			case errors.Is(err, ai.ErrInferenceTimeout):
				response.Error(w, http.StatusGatewayTimeout, "AI_INFERENCE_TIMEOUT",
					"AI summarization took too long and was cancelled", nil)
			default:
				response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR",
					"An unexpected error occurred", nil)
			}
			return
		}

		response.JSON(w, summarizeResponse{
			Summary:       result.Summary,
			LinesAnalyzed: result.LinesAnalyzed,
			TimeRange: timeRange{
				From: result.From.UTC().Format(time.RFC3339),
				To:   result.To.UTC().Format(time.RFC3339),
			},
			Provider: result.Provider,
			Model:    result.Model,
		})
	}
}

type summarizeResponse struct {
	Summary       string    `json:"summary"`
	LinesAnalyzed int       `json:"lines_analyzed"`
	TimeRange     timeRange `json:"time_range"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
}

type timeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}
