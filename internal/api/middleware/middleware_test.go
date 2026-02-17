package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	mw "github.com/kiranshivaraju/loghunter/internal/api/middleware"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"github.com/kiranshivaraju/loghunter/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// --- Mock Store ---

type mockStore struct {
	keys []*models.APIKey
	err  error
}

func (m *mockStore) Ping(_ context.Context) error { return nil }
func (m *mockStore) GetAPIKeyByPrefix(_ context.Context, _ string) ([]*models.APIKey, error) {
	return m.keys, m.err
}
func (m *mockStore) UpdateAPIKeyLastUsed(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockStore) GetDefaultTenant(_ context.Context) (*models.Tenant, error) {
	return nil, store.ErrNotFound
}
func (m *mockStore) CreateAPIKey(_ context.Context, _ *models.APIKey) error  { return nil }
func (m *mockStore) ListAPIKeys(_ context.Context, _ uuid.UUID) ([]*models.APIKey, error) {
	return nil, nil
}
func (m *mockStore) RevokeAPIKey(_ context.Context, _ uuid.UUID, _ uuid.UUID) error { return nil }
func (m *mockStore) UpsertErrorCluster(_ context.Context, c *models.ErrorCluster) (*models.ErrorCluster, error) {
	return c, nil
}
func (m *mockStore) ListErrorClusters(_ context.Context, _ store.ClusterFilter) ([]*models.ErrorCluster, int, error) {
	return nil, 0, nil
}
func (m *mockStore) GetErrorCluster(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*models.ErrorCluster, error) {
	return nil, store.ErrNotFound
}
func (m *mockStore) GetClustersByFingerprints(_ context.Context, _ uuid.UUID, _ []string) ([]*models.ErrorCluster, error) {
	return nil, nil
}
func (m *mockStore) CreateAnalysisResult(_ context.Context, _ *models.AnalysisResult) error {
	return nil
}
func (m *mockStore) GetAnalysisResultByJobID(_ context.Context, _ uuid.UUID) (*models.AnalysisResult, error) {
	return nil, store.ErrNotFound
}
func (m *mockStore) GetAnalysisResultByClusterID(_ context.Context, _ uuid.UUID) (*models.AnalysisResult, error) {
	return nil, store.ErrNotFound
}
func (m *mockStore) CreateJob(_ context.Context, _ *models.Job) error { return nil }
func (m *mockStore) GetJob(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*models.Job, error) {
	return nil, store.ErrNotFound
}
func (m *mockStore) UpdateJobStatus(_ context.Context, _ uuid.UUID, _ string, _ ...store.JobUpdateOption) error {
	return nil
}

// --- Mock Cache ---

type mockCache struct {
	counter int64
	err     error
}

func (m *mockCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error { return nil }
func (m *mockCache) Get(_ context.Context, _ string) ([]byte, bool, error)            { return nil, false, nil }
func (m *mockCache) Delete(_ context.Context, _ string) error                          { return nil }
func (m *mockCache) Ping(_ context.Context) error                                      { return nil }
func (m *mockCache) SetJobStatus(_ context.Context, _ uuid.UUID, _ string, _ time.Duration) error {
	return nil
}
func (m *mockCache) GetJobStatus(_ context.Context, _ uuid.UUID) (string, bool, error) {
	return "", false, nil
}
func (m *mockCache) IncrWithExpiry(_ context.Context, _ string, _ time.Duration) (int64, error) {
	m.counter++
	return m.counter, m.err
}

// --- helpers ---

func okHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}
}

func hashKey(t *testing.T, rawKey string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.MinCost)
	require.NoError(t, err)
	return string(h)
}

func errBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body["error"].(map[string]any)
}

// ========================================
// Auth Middleware Tests
// ========================================

func TestAuth_MissingAuthHeader(t *testing.T) {
	auth := mw.NewAuth(&mockStore{})
	handler := auth.Authenticate(okHandler())

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "INVALID_TOKEN", errBody(t, w)["code"])
}

func TestAuth_InvalidBearerFormat(t *testing.T) {
	auth := mw.NewAuth(&mockStore{})
	handler := auth.Authenticate(okHandler())

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic abc123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_KeyTooShort(t *testing.T) {
	auth := mw.NewAuth(&mockStore{})
	handler := auth.Authenticate(okHandler())

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer short")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_KeyNotFound(t *testing.T) {
	auth := mw.NewAuth(&mockStore{keys: []*models.APIKey{}})
	handler := auth.Authenticate(okHandler())

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer lh_test1234567890")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_WrongPassword(t *testing.T) {
	rawKey := "lh_test1234567890abcdef"
	ms := &mockStore{keys: []*models.APIKey{{
		ID:        uuid.New(),
		TenantID:  uuid.New(),
		KeyHash:   hashKey(t, "different_key_entirely"),
		KeyPrefix: rawKey[:8],
		Scopes:    []string{"read"},
	}}}
	auth := mw.NewAuth(ms)
	handler := auth.Authenticate(okHandler())

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_ValidKey(t *testing.T) {
	rawKey := "lh_test1234567890abcdef"
	tenantID := uuid.New()
	ms := &mockStore{keys: []*models.APIKey{{
		ID:        uuid.New(),
		TenantID:  tenantID,
		KeyHash:   hashKey(t, rawKey),
		KeyPrefix: rawKey[:8],
		Scopes:    []string{"read", "admin"},
	}}}
	auth := mw.NewAuth(ms)

	var gotTenantID uuid.UUID
	var gotOK bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenantID, gotOK = mw.GetTenantID(r)
		w.WriteHeader(http.StatusOK)
	})
	handler := auth.Authenticate(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, gotOK)
	assert.Equal(t, tenantID, gotTenantID)
}

func TestAuth_RequireScope_Allowed(t *testing.T) {
	rawKey := "lh_admin_1234567890abcdef"
	ms := &mockStore{keys: []*models.APIKey{{
		ID:        uuid.New(),
		TenantID:  uuid.New(),
		KeyHash:   hashKey(t, rawKey),
		KeyPrefix: rawKey[:8],
		Scopes:    []string{"read", "admin"},
	}}}
	auth := mw.NewAuth(ms)

	handler := auth.Authenticate(auth.RequireScope("admin")(okHandler()))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuth_RequireScope_Denied(t *testing.T) {
	rawKey := "lh_read__1234567890abcdef"
	ms := &mockStore{keys: []*models.APIKey{{
		ID:        uuid.New(),
		TenantID:  uuid.New(),
		KeyHash:   hashKey(t, rawKey),
		KeyPrefix: rawKey[:8],
		Scopes:    []string{"read"},
	}}}
	auth := mw.NewAuth(ms)

	handler := auth.Authenticate(auth.RequireScope("admin")(okHandler()))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "FORBIDDEN", errBody(t, w)["code"])
}

// ========================================
// Rate Limit Middleware Tests
// ========================================

func TestRateLimit_AllowsUnderLimit(t *testing.T) {
	mc := &mockCache{counter: 0}
	rl := mw.NewRateLimit(mc, 60)

	// Simulate auth middleware by setting context
	inner := okHandler()
	handler := rl.Limit(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	ctx := context.WithValue(req.Context(), mw.ExportedKeyPrefixKey(), "lh_test1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "60", w.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "59", w.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))
}

func TestRateLimit_RejectsOverLimit(t *testing.T) {
	mc := &mockCache{counter: 60} // next IncrWithExpiry will return 61
	rl := mw.NewRateLimit(mc, 60)

	handler := rl.Limit(okHandler())

	req := httptest.NewRequest("GET", "/test", nil)
	ctx := context.WithValue(req.Context(), mw.ExportedKeyPrefixKey(), "lh_over1")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "60", w.Header().Get("Retry-After"))
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", errBody(t, w)["code"])
}

func TestRateLimit_NoKeyPrefix_PassThrough(t *testing.T) {
	mc := &mockCache{}
	rl := mw.NewRateLimit(mc, 60)

	handler := rl.Limit(okHandler())

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ========================================
// Recovery Middleware Tests
// ========================================

func TestRecovery_CatchesPanic(t *testing.T) {
	panicking := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("something went wrong")
	})

	handler := mw.Recovery(panicking)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "INTERNAL_ERROR", errBody(t, w)["code"])
}

func TestRecovery_NoPanic(t *testing.T) {
	handler := mw.Recovery(okHandler())

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ========================================
// Logging Middleware Tests
// ========================================

func TestLogger_SetsStatus(t *testing.T) {
	handler := mw.Logger(okHandler())

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
