package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/kiranshivaraju/loghunter/internal/api/response"
	"github.com/kiranshivaraju/loghunter/internal/cache"
)

const defaultRequestsPerMinute = 60

// RateLimit provides sliding-window rate limiting via Redis.
type RateLimit struct {
	cache            cache.Cache
	requestsPerMin   int
}

// NewRateLimit creates a new RateLimit middleware.
func NewRateLimit(c cache.Cache, requestsPerMin int) *RateLimit {
	if requestsPerMin <= 0 {
		requestsPerMin = defaultRequestsPerMinute
	}
	return &RateLimit{cache: c, requestsPerMin: requestsPerMin}
}

// Limit applies rate limiting based on the key_prefix set by auth middleware.
func (rl *RateLimit) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix, ok := getKeyPrefix(r)
		if !ok {
			// No key prefix means auth middleware didn't run; pass through
			next.ServeHTTP(w, r)
			return
		}

		key := cache.RateLimitKey(prefix)
		count, err := rl.cache.IncrWithExpiry(r.Context(), key, 60*time.Second)
		if err != nil {
			// On Redis error, allow the request (fail open)
			next.ServeHTTP(w, r)
			return
		}

		remaining := rl.requestsPerMin - int(count)
		if remaining < 0 {
			remaining = 0
		}
		resetTime := time.Now().Add(60 * time.Second).Unix()

		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.requestsPerMin))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime))

		if count > int64(rl.requestsPerMin) {
			w.Header().Set("Retry-After", "60")
			response.Error(w, http.StatusTooManyRequests,
				"RATE_LIMIT_EXCEEDED", "Too many requests", nil)
			return
		}

		next.ServeHTTP(w, r)
	})
}
