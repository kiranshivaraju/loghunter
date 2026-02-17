package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type contextKey string

const (
	tenantIDKey contextKey = "tenant_id"
	keyPrefixKey contextKey = "key_prefix"
	apiKeyScopesKey contextKey = "api_key_scopes"
)

func SetTenantID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, tenantIDKey, id)
}

func GetTenantID(r *http.Request) (uuid.UUID, bool) {
	id, ok := r.Context().Value(tenantIDKey).(uuid.UUID)
	return id, ok
}

func setKeyPrefix(ctx context.Context, prefix string) context.Context {
	return context.WithValue(ctx, keyPrefixKey, prefix)
}

func getKeyPrefix(r *http.Request) (string, bool) {
	prefix, ok := r.Context().Value(keyPrefixKey).(string)
	return prefix, ok
}

func setScopes(ctx context.Context, scopes []string) context.Context {
	return context.WithValue(ctx, apiKeyScopesKey, scopes)
}

func getScopes(r *http.Request) []string {
	scopes, _ := r.Context().Value(apiKeyScopesKey).([]string)
	return scopes
}

// ExportedKeyPrefixKey returns the context key for key_prefix (for testing).
func ExportedKeyPrefixKey() contextKey {
	return keyPrefixKey
}
