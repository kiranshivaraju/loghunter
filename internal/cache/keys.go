package cache

import (
	"fmt"

	"github.com/google/uuid"
)

func LokiQueryKey(tenantID uuid.UUID, queryHash string) string {
	return fmt.Sprintf("loki:query:%s:%s", tenantID, queryHash)
}

func JobStatusKey(jobID uuid.UUID) string {
	return fmt.Sprintf("job:%s", jobID)
}

func RateLimitKey(keyPrefix string) string {
	return fmt.Sprintf("ratelimit:%s", keyPrefix)
}

func SearchResultKey(tenantID uuid.UUID, filterHash string) string {
	return fmt.Sprintf("loki:search:%s:%s", tenantID, filterHash)
}
