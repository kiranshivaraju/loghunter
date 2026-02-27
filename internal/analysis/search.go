package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/api/handler"
	"github.com/kiranshivaraju/loghunter/internal/cache"
	"github.com/kiranshivaraju/loghunter/internal/loki"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"github.com/kiranshivaraju/loghunter/pkg/logql"
)

const searchCacheTTL = 60 * time.Second

// SearchService implements handler.Searcher with Redis caching.
type SearchService struct {
	loki  loki.Client
	store store.Store
	cache cache.Cache
	qb    logql.QueryBuilder
}

// NewSearchService creates a new SearchService.
func NewSearchService(lokiClient loki.Client, st store.Store, ca cache.Cache) *SearchService {
	return &SearchService{
		loki:  lokiClient,
		store: st,
		cache: ca,
	}
}

// Search queries Loki for log lines matching the given parameters, with Redis caching.
func (s *SearchService) Search(ctx context.Context, params handler.SearchParams) (*handler.SearchResult, error) {
	// Build cache key from tenant + filter hash
	filterHash := s.buildFilterHash(params)
	cacheKey := cache.SearchResultKey(params.TenantID, filterHash)

	// Check cache
	cached, found, err := s.cache.Get(ctx, cacheKey)
	if err == nil && found {
		var result handler.SearchResult
		if json.Unmarshal(cached, &result) == nil {
			result.CacheHit = true
			return &result, nil
		}
	}

	// Build LogQL query
	query := s.qb.BuildSearchQuery(logql.SearchParams{
		Service:   params.Service,
		Namespace: params.Namespace,
		Start:     params.Start,
		End:       params.End,
		Levels:    params.Levels,
		Keyword:   params.Keyword,
	})

	// Query Loki with limit+1 to detect has_next
	lines, err := s.loki.QueryRange(ctx, loki.QueryRangeRequest{
		Query:     query,
		Start:     params.Start,
		End:       params.End,
		Limit:     params.Limit + 1,
		Direction: "forward",
	})
	if err != nil {
		return nil, fmt.Errorf("querying loki: %w", err)
	}

	// Determine if there are more results
	hasMore := len(lines) > params.Limit
	if hasMore {
		lines = lines[:params.Limit]
	}

	// Build fingerprints for cluster lookup
	fingerprintMap := make(map[string]bool)
	for _, line := range lines {
		fp := Fingerprint(line.Message)
		fingerprintMap[fp] = true
	}

	fingerprints := make([]string, 0, len(fingerprintMap))
	for fp := range fingerprintMap {
		fingerprints = append(fingerprints, fp)
	}

	// Look up clusters by fingerprints
	clustersByFP := make(map[string]uuid.UUID)
	if len(fingerprints) > 0 {
		clusters, err := s.store.GetClustersByFingerprints(ctx, params.TenantID, fingerprints)
		if err == nil {
			for _, c := range clusters {
				clustersByFP[c.Fingerprint] = c.ID
			}
		}
	}

	// Convert to result lines
	results := make([]handler.SearchResultLine, len(lines))
	for i, line := range lines {
		results[i] = handler.SearchResultLine{
			Timestamp: line.Timestamp,
			Message:   line.Message,
			Level:     line.Level,
			Labels:    line.Labels,
		}
		fp := Fingerprint(line.Message)
		if id, ok := clustersByFP[fp]; ok {
			clusterID := id
			results[i].ClusterID = &clusterID
		}
	}

	result := &handler.SearchResult{
		Results:  results,
		Query:    query,
		CacheHit: false,
	}

	// Cache the result
	if data, err := json.Marshal(result); err == nil {
		_ = s.cache.Set(ctx, cacheKey, data, searchCacheTTL)
	}

	return result, nil
}

func (s *SearchService) buildFilterHash(params handler.SearchParams) string {
	raw := fmt.Sprintf("%s:%s:%s:%s:%v:%s:%d",
		params.TenantID,
		params.Service,
		params.Namespace,
		params.Start.Format(time.RFC3339),
		params.Levels,
		params.Keyword,
		params.Limit,
	)
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h[:8])
}

// Compile-time check that SearchService implements Searcher.
var _ handler.Searcher = (*SearchService)(nil)
