package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	mw "github.com/kiranshivaraju/loghunter/internal/api/middleware"
	"github.com/kiranshivaraju/loghunter/internal/api/response"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"github.com/kiranshivaraju/loghunter/pkg/models"
	"golang.org/x/crypto/bcrypt"
)

// KeyCreator is the store interface needed by NewCreateKeyHandler.
type KeyCreator interface {
	CreateAPIKey(ctx context.Context, key *models.APIKey) error
}

// KeyLister is the store interface needed by NewListKeysHandler.
type KeyLister interface {
	ListAPIKeys(ctx context.Context, tenantID uuid.UUID) ([]*models.APIKey, error)
}

// KeyRevoker is the store interface needed by NewRevokeKeyHandler.
type KeyRevoker interface {
	RevokeAPIKey(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error
}

// NewCreateKeyHandler returns an http.HandlerFunc for POST /api/v1/admin/keys.
func NewCreateKeyHandler(st KeyCreator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := mw.GetTenantID(r)
		if !ok {
			response.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Missing tenant", nil)
			return
		}

		var req struct {
			Name   string   `json:"name"`
			Scopes []string `json:"scopes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", nil)
			return
		}

		if req.Name == "" {
			response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "name is required", nil)
			return
		}

		// Generate raw key: lhk_<name>_<random hex>
		randomBytes := make([]byte, 16)
		rand.Read(randomBytes)
		rawKey := fmt.Sprintf("lhk_%s_%s", req.Name, hex.EncodeToString(randomBytes))

		hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to hash key", nil)
			return
		}

		now := time.Now().UTC()
		key := &models.APIKey{
			ID:        uuid.New(),
			TenantID:  tenantID,
			Name:      req.Name,
			KeyHash:   string(hash),
			KeyPrefix: rawKey[:8],
			Scopes:    req.Scopes,
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := st.CreateAPIKey(r.Context(), key); err != nil {
			if err == store.ErrDuplicateKey {
				response.Error(w, http.StatusConflict, "DUPLICATE_KEY", "API key with this name already exists", nil)
				return
			}
			response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create key", nil)
			return
		}

		response.Created(w, map[string]any{
			"id":         key.ID.String(),
			"name":       key.Name,
			"key":        rawKey,
			"scopes":     key.Scopes,
			"created_at": key.CreatedAt,
		})
	}
}

// NewListKeysHandler returns an http.HandlerFunc for GET /api/v1/admin/keys.
func NewListKeysHandler(st KeyLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := mw.GetTenantID(r)
		if !ok {
			response.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Missing tenant", nil)
			return
		}

		keys, err := st.ListAPIKeys(r.Context(), tenantID)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list keys", nil)
			return
		}

		safeKeys := make([]map[string]any, len(keys))
		for i, k := range keys {
			safeKeys[i] = map[string]any{
				"id":         k.ID.String(),
				"name":       k.Name,
				"key_prefix": k.KeyPrefix,
				"scopes":     k.Scopes,
				"created_at": k.CreatedAt,
			}
		}

		response.JSON(w, safeKeys)
	}
}

// NewRevokeKeyHandler returns an http.HandlerFunc for DELETE /api/v1/admin/keys/{keyID}.
func NewRevokeKeyHandler(st KeyRevoker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := mw.GetTenantID(r)
		if !ok {
			response.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Missing tenant", nil)
			return
		}

		keyIDStr := chi.URLParam(r, "keyID")
		keyID, err := uuid.Parse(keyIDStr)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_KEY_ID", "Invalid key ID", nil)
			return
		}

		if err := st.RevokeAPIKey(r.Context(), keyID, tenantID); err != nil {
			response.Error(w, http.StatusNotFound, "KEY_NOT_FOUND", "API key not found", nil)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
