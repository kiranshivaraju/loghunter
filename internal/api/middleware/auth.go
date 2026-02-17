package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/kiranshivaraju/loghunter/internal/api/response"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const keyPrefixLen = 8

// Auth provides authentication and scope-checking middleware.
type Auth struct {
	store store.Store
}

// NewAuth creates a new Auth middleware.
func NewAuth(s store.Store) *Auth {
	return &Auth{store: s}
}

// Authenticate validates the Bearer token, looks up the API key, and sets
// tenant_id, key_prefix, and scopes in the request context.
func (a *Auth) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawKey := extractBearerToken(r)
		if rawKey == "" {
			response.Error(w, http.StatusUnauthorized,
				"INVALID_TOKEN", "Missing or invalid Authorization header", nil)
			return
		}

		if len(rawKey) < keyPrefixLen {
			response.Error(w, http.StatusUnauthorized,
				"INVALID_TOKEN", "Invalid API key format", nil)
			return
		}

		prefix := rawKey[:keyPrefixLen]

		keys, err := a.store.GetAPIKeyByPrefix(r.Context(), prefix)
		if err != nil {
			response.Error(w, http.StatusInternalServerError,
				"INTERNAL_ERROR", "Failed to validate API key", nil)
			return
		}

		// Find matching key by bcrypt comparison
		var matched bool
		for _, key := range keys {
			if bcrypt.CompareHashAndPassword([]byte(key.KeyHash), []byte(rawKey)) == nil {
				ctx := r.Context()
				ctx = SetTenantID(ctx, key.TenantID)
				ctx = setKeyPrefix(ctx, prefix)
				ctx = setScopes(ctx, key.Scopes)
				r = r.WithContext(ctx)
				matched = true

				// Update last_used_at async
				go a.store.UpdateAPIKeyLastUsed(context.Background(), key.ID)
				break
			}
		}

		if !matched {
			response.Error(w, http.StatusUnauthorized,
				"INVALID_TOKEN", "Invalid API key", nil)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequireScope returns middleware that checks whether the authenticated
// API key has the specified scope.
func (a *Auth) RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scopes := getScopes(r)
			for _, s := range scopes {
				if s == scope {
					next.ServeHTTP(w, r)
					return
				}
			}
			response.Error(w, http.StatusForbidden,
				"FORBIDDEN", "Insufficient permissions", nil)
		})
	}
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
