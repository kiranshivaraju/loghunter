package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/api"
	mw "github.com/kiranshivaraju/loghunter/internal/api/middleware"
	"github.com/kiranshivaraju/loghunter/internal/cache"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"github.com/kiranshivaraju/loghunter/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- stub store that returns empty results (all auth fails) ---

type stubStore struct{}

func (s *stubStore) GetDefaultTenant(_ context.Context) (*models.Tenant, error) {
	return nil, store.ErrNotFound
}
func (s *stubStore) GetAPIKeyByPrefix(_ context.Context, _ string) ([]*models.APIKey, error) {
	return nil, nil
}
func (s *stubStore) UpdateAPIKeyLastUsed(_ context.Context, _ uuid.UUID) error       { return nil }
func (s *stubStore) CreateAPIKey(_ context.Context, _ *models.APIKey) error           { return nil }
func (s *stubStore) ListAPIKeys(_ context.Context, _ uuid.UUID) ([]*models.APIKey, error) {
	return nil, nil
}
func (s *stubStore) RevokeAPIKey(_ context.Context, _ uuid.UUID, _ uuid.UUID) error { return nil }
func (s *stubStore) UpsertErrorCluster(_ context.Context, c *models.ErrorCluster) (*models.ErrorCluster, error) {
	return c, nil
}
func (s *stubStore) ListErrorClusters(_ context.Context, _ store.ClusterFilter) ([]*models.ErrorCluster, int, error) {
	return nil, 0, nil
}
func (s *stubStore) GetErrorCluster(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*models.ErrorCluster, error) {
	return nil, store.ErrNotFound
}
func (s *stubStore) GetClustersByFingerprints(_ context.Context, _ uuid.UUID, _ []string) ([]*models.ErrorCluster, error) {
	return nil, nil
}
func (s *stubStore) CreateAnalysisResult(_ context.Context, _ *models.AnalysisResult) error {
	return nil
}
func (s *stubStore) GetAnalysisResultByJobID(_ context.Context, _ uuid.UUID) (*models.AnalysisResult, error) {
	return nil, store.ErrNotFound
}
func (s *stubStore) GetAnalysisResultByClusterID(_ context.Context, _ uuid.UUID) (*models.AnalysisResult, error) {
	return nil, store.ErrNotFound
}
func (s *stubStore) CreateJob(_ context.Context, _ *models.Job) error { return nil }
func (s *stubStore) GetJob(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*models.Job, error) {
	return nil, store.ErrNotFound
}
func (s *stubStore) UpdateJobStatus(_ context.Context, _ uuid.UUID, _ string, _ ...store.JobUpdateOption) error {
	return nil
}

// --- stub cache ---

type stubCache struct{}

func (c *stubCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error { return nil }
func (c *stubCache) Get(_ context.Context, _ string) ([]byte, bool, error)            { return nil, false, nil }
func (c *stubCache) Delete(_ context.Context, _ string) error                          { return nil }
func (c *stubCache) Ping(_ context.Context) error                                      { return nil }
func (c *stubCache) SetJobStatus(_ context.Context, _ uuid.UUID, _ string, _ time.Duration) error {
	return nil
}
func (c *stubCache) GetJobStatus(_ context.Context, _ uuid.UUID) (string, bool, error) {
	return "", false, nil
}
func (c *stubCache) IncrWithExpiry(_ context.Context, _ string, _ time.Duration) (int64, error) {
	return 1, nil
}

// --- router tests ---

func newTestRouter() http.Handler {
	return api.NewRouter(api.Dependencies{
		Auth:      mw.NewAuth(&stubStore{}),
		RateLimit: mw.NewRateLimit(&stubCache{}, 60),
		HealthHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		},
	})
}

func TestRouter_HealthEndpoint_Public(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_ProtectedEndpoints_RequireAuth(t *testing.T) {
	router := newTestRouter()

	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/analyze"},
		{"GET", "/api/v1/clusters"},
		{"POST", "/api/v1/summarize"},
		{"POST", "/api/v1/search"},
		{"POST", "/api/v1/admin/keys"},
		{"GET", "/api/v1/admin/keys"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)

			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			errObj := body["error"].(map[string]any)
			assert.Equal(t, "INVALID_TOKEN", errObj["code"])
		})
	}
}

func TestRouter_NotFound(t *testing.T) {
	router := newTestRouter()

	req := httptest.NewRequest("GET", "/api/v1/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Verify unused interfaces are satisfied
var _ store.Store = (*stubStore)(nil)
var _ cache.Cache = (*stubCache)(nil)
