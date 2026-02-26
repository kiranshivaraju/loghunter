package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// --- mock store for admin tests ---

type adminMockStore struct {
	keys         []*models.APIKey
	createErr    error
	listErr      error
	revokeErr    error
}

func (s *adminMockStore) CreateAPIKey(_ context.Context, key *models.APIKey) error {
	if s.createErr != nil {
		return s.createErr
	}
	for _, existing := range s.keys {
		if existing.Name == key.Name && existing.TenantID == key.TenantID {
			return store.ErrDuplicateKey
		}
	}
	s.keys = append(s.keys, key)
	return nil
}

func (s *adminMockStore) ListAPIKeys(_ context.Context, tenantID uuid.UUID) ([]*models.APIKey, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	var out []*models.APIKey
	for _, k := range s.keys {
		if k.TenantID == tenantID {
			out = append(out, k)
		}
	}
	return out, nil
}

func (s *adminMockStore) RevokeAPIKey(_ context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	if s.revokeErr != nil {
		return s.revokeErr
	}
	for _, k := range s.keys {
		if k.ID == id && k.TenantID == tenantID {
			return nil
		}
	}
	return store.ErrNotFound
}

// --- helpers ---

func jsonBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(v)
	return &buf
}

func parseJSON(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	return out
}

// --- CreateKeyHandler tests ---

func TestCreateKeyHandler_Success(t *testing.T) {
	tenantID := uuid.New()
	st := &adminMockStore{}

	handler := NewCreateKeyHandler(st)

	body := jsonBody(t, map[string]any{
		"name":   "my-key",
		"scopes": []string{"read", "write"},
	})
	req := httptest.NewRequest("POST", "/api/v1/admin/keys", body)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)

	if data["name"] != "my-key" {
		t.Errorf("expected name 'my-key', got %v", data["name"])
	}
	if data["key"] == nil || data["key"] == "" {
		t.Error("expected raw key in response")
	}
	if data["id"] == nil || data["id"] == "" {
		t.Error("expected id in response")
	}

	// Verify key was stored
	if len(st.keys) != 1 {
		t.Fatalf("expected 1 key stored, got %d", len(st.keys))
	}
	if st.keys[0].KeyHash == "" {
		t.Error("expected bcrypt hash to be stored")
	}
	if len(st.keys[0].KeyPrefix) != 8 {
		t.Errorf("expected key_prefix of length 8, got %d", len(st.keys[0].KeyPrefix))
	}
}

func TestCreateKeyHandler_DuplicateKey(t *testing.T) {
	tenantID := uuid.New()
	st := &adminMockStore{
		keys: []*models.APIKey{{
			ID:       uuid.New(),
			TenantID: tenantID,
			Name:     "existing-key",
		}},
	}

	handler := NewCreateKeyHandler(st)

	body := jsonBody(t, map[string]any{
		"name":   "existing-key",
		"scopes": []string{"read"},
	})
	req := httptest.NewRequest("POST", "/api/v1/admin/keys", body)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
	resp := parseJSON(t, rr)
	errObj := resp["error"].(map[string]any)
	if errObj["code"] != "DUPLICATE_KEY" {
		t.Errorf("expected code DUPLICATE_KEY, got %v", errObj["code"])
	}
}

func TestCreateKeyHandler_MissingName(t *testing.T) {
	handler := NewCreateKeyHandler(&adminMockStore{})

	body := jsonBody(t, map[string]any{
		"scopes": []string{"read"},
	})
	req := httptest.NewRequest("POST", "/api/v1/admin/keys", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestCreateKeyHandler_InvalidJSON(t *testing.T) {
	handler := NewCreateKeyHandler(&adminMockStore{})

	req := httptest.NewRequest("POST", "/api/v1/admin/keys", bytes.NewBufferString("{invalid"))
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestCreateKeyHandler_NoTenant(t *testing.T) {
	handler := NewCreateKeyHandler(&adminMockStore{})

	body := jsonBody(t, map[string]any{"name": "test", "scopes": []string{"read"}})
	req := httptest.NewRequest("POST", "/api/v1/admin/keys", body)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestCreateKeyHandler_RawKeyFormat(t *testing.T) {
	handler := NewCreateKeyHandler(&adminMockStore{})

	body := jsonBody(t, map[string]any{"name": "grafana", "scopes": []string{"read"}})
	req := httptest.NewRequest("POST", "/api/v1/admin/keys", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	rawKey := data["key"].(string)

	if len(rawKey) < 16 {
		t.Errorf("raw key too short: %s", rawKey)
	}
	if rawKey[:4] != "lhk_" {
		t.Errorf("raw key should start with lhk_, got %s", rawKey[:4])
	}
}

// --- ListKeysHandler tests ---

func TestListKeysHandler_Success(t *testing.T) {
	tenantID := uuid.New()
	st := &adminMockStore{
		keys: []*models.APIKey{
			{
				ID:        uuid.New(),
				TenantID:  tenantID,
				Name:      "key-1",
				KeyHash:   "$2a$10$hash1",
				KeyPrefix: "lhk_key1",
				Scopes:    []string{"read"},
				CreatedAt: time.Now(),
			},
			{
				ID:        uuid.New(),
				TenantID:  tenantID,
				Name:      "key-2",
				KeyHash:   "$2a$10$hash2",
				KeyPrefix: "lhk_key2",
				Scopes:    []string{"read", "write"},
				CreatedAt: time.Now(),
			},
		},
	}

	handler := NewListKeysHandler(st)

	req := httptest.NewRequest("GET", "/api/v1/admin/keys", nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	resp := parseJSON(t, rr)
	data := resp["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(data))
	}

	firstKey := data[0].(map[string]any)
	if firstKey["key_prefix"] == nil {
		t.Error("expected key_prefix in response")
	}
	if firstKey["key"] != nil {
		t.Error("raw key must NOT be in list response")
	}
	if firstKey["key_hash"] != nil {
		t.Error("key_hash must NOT be in list response")
	}
}

func TestListKeysHandler_NoTenant(t *testing.T) {
	handler := NewListKeysHandler(&adminMockStore{})

	req := httptest.NewRequest("GET", "/api/v1/admin/keys", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestListKeysHandler_FiltersByTenant(t *testing.T) {
	tenantA := uuid.New()
	tenantB := uuid.New()
	st := &adminMockStore{
		keys: []*models.APIKey{
			{ID: uuid.New(), TenantID: tenantA, Name: "key-a", KeyPrefix: "lhk_keya"},
			{ID: uuid.New(), TenantID: tenantB, Name: "key-b", KeyPrefix: "lhk_keyb"},
		},
	}

	handler := NewListKeysHandler(st)

	req := httptest.NewRequest("GET", "/api/v1/admin/keys", nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantA))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	resp := parseJSON(t, rr)
	data := resp["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("expected 1 key for tenant A, got %d", len(data))
	}
}

// --- RevokeKeyHandler tests ---

func TestRevokeKeyHandler_Success(t *testing.T) {
	tenantID := uuid.New()
	keyID := uuid.New()
	st := &adminMockStore{
		keys: []*models.APIKey{{ID: keyID, TenantID: tenantID, Name: "revoke-me"}},
	}

	handler := NewRevokeKeyHandler(st)

	req := httptest.NewRequest("DELETE", "/api/v1/admin/keys/"+keyID.String(), nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))

	// Set Chi URL param
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("keyID", keyID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestRevokeKeyHandler_NotFound(t *testing.T) {
	tenantID := uuid.New()
	st := &adminMockStore{}

	handler := NewRevokeKeyHandler(st)

	req := httptest.NewRequest("DELETE", "/api/v1/admin/keys/"+uuid.New().String(), nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("keyID", uuid.New().String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestRevokeKeyHandler_InvalidKeyID(t *testing.T) {
	handler := NewRevokeKeyHandler(&adminMockStore{})

	req := httptest.NewRequest("DELETE", "/api/v1/admin/keys/not-a-uuid", nil)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("keyID", "not-a-uuid")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestRevokeKeyHandler_WrongTenant(t *testing.T) {
	tenantA := uuid.New()
	tenantB := uuid.New()
	keyID := uuid.New()
	st := &adminMockStore{
		keys: []*models.APIKey{{ID: keyID, TenantID: tenantA, Name: "key-a"}},
	}

	handler := NewRevokeKeyHandler(st)

	req := httptest.NewRequest("DELETE", "/api/v1/admin/keys/"+keyID.String(), nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantB))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("keyID", keyID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong tenant, got %d", rr.Code)
	}
}

func TestRevokeKeyHandler_NoTenant(t *testing.T) {
	handler := NewRevokeKeyHandler(&adminMockStore{})

	req := httptest.NewRequest("DELETE", "/api/v1/admin/keys/"+uuid.New().String(), nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
