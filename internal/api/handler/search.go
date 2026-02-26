package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
	"unicode"

	"github.com/google/uuid"
	mw "github.com/kiranshivaraju/loghunter/internal/api/middleware"
	"github.com/kiranshivaraju/loghunter/internal/api/response"
)

// SearchParams holds validated parameters for a search request.
type SearchParams struct {
	TenantID  uuid.UUID
	Service   string
	Namespace string
	Start     time.Time
	End       time.Time
	Levels    []string
	Keyword   string
	Limit     int
}

// SearchResult is the output of a search operation.
type SearchResult struct {
	Results  []SearchResultLine `json:"results"`
	Query    string             `json:"query"`
	CacheHit bool              `json:"cache_hit"`
}

// SearchResultLine represents a single log line in search results.
type SearchResultLine struct {
	Timestamp time.Time         `json:"timestamp"`
	Message   string            `json:"message"`
	Level     string            `json:"level"`
	Labels    map[string]string `json:"labels"`
	ClusterID *uuid.UUID        `json:"cluster_id,omitempty"`
}

// Searcher defines the interface the search handler depends on.
type Searcher interface {
	Search(ctx context.Context, params SearchParams) (*SearchResult, error)
}

// NewSearchHandler returns an http.HandlerFunc for POST /api/v1/search.
func NewSearchHandler(svc Searcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := mw.GetTenantID(r)
		if !ok {
			response.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Missing tenant", nil)
			return
		}

		var req struct {
			Service   string   `json:"service"`
			Namespace string   `json:"namespace"`
			Start     string   `json:"start"`
			End       string   `json:"end"`
			Levels    []string `json:"levels"`
			Keyword   string   `json:"keyword"`
			Limit     int      `json:"limit"`
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

		// Validate keyword
		if len(req.Keyword) > 200 {
			response.Error(w, http.StatusBadRequest, "INVALID_QUERY", "keyword must be 200 characters or fewer", nil)
			return
		}
		for _, ch := range req.Keyword {
			if !unicode.IsPrint(ch) {
				response.Error(w, http.StatusBadRequest, "INVALID_QUERY", "keyword contains non-printable characters", nil)
				return
			}
		}

		ns := req.Namespace
		if ns == "" {
			ns = "default"
		}

		limit := req.Limit
		if limit == 0 {
			limit = 100
		}
		if limit < 1 {
			limit = 1
		}
		if limit > 1000 {
			limit = 1000
		}

		result, err := svc.Search(r.Context(), SearchParams{
			TenantID:  tenantID,
			Service:   req.Service,
			Namespace: ns,
			Start:     startTime,
			End:       endTime,
			Levels:    req.Levels,
			Keyword:   req.Keyword,
			Limit:     limit,
		})
		if err != nil {
			status, code, msg := mapError(err)
			response.Error(w, status, code, msg, nil)
			return
		}

		response.JSON(w, result)
	}
}
