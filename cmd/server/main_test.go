package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/cache"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"github.com/kiranshivaraju/loghunter/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── mock store ──────────────────────────────────────────────────────────────

type testStore struct {
	pingErr error
}

func (s *testStore) Ping(_ context.Context) error                                { return s.pingErr }
func (s *testStore) GetDefaultTenant(_ context.Context) (*models.Tenant, error)  { return nil, nil }
func (s *testStore) GetAPIKeyByPrefix(_ context.Context, _ string) ([]*models.APIKey, error) {
	return nil, nil
}
func (s *testStore) UpdateAPIKeyLastUsed(_ context.Context, _ uuid.UUID) error   { return nil }
func (s *testStore) CreateAPIKey(_ context.Context, _ *models.APIKey) error      { return nil }
func (s *testStore) ListAPIKeys(_ context.Context, _ uuid.UUID) ([]*models.APIKey, error) {
	return nil, nil
}
func (s *testStore) RevokeAPIKey(_ context.Context, _ uuid.UUID, _ uuid.UUID) error { return nil }
func (s *testStore) UpsertErrorCluster(_ context.Context, c *models.ErrorCluster) (*models.ErrorCluster, error) {
	return c, nil
}
func (s *testStore) ListErrorClusters(_ context.Context, _ store.ClusterFilter) ([]*models.ErrorCluster, int, error) {
	return nil, 0, nil
}
func (s *testStore) GetErrorCluster(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*models.ErrorCluster, error) {
	return nil, store.ErrNotFound
}
func (s *testStore) GetClustersByFingerprints(_ context.Context, _ uuid.UUID, _ []string) ([]*models.ErrorCluster, error) {
	return nil, nil
}
func (s *testStore) CreateAnalysisResult(_ context.Context, _ *models.AnalysisResult) error {
	return nil
}
func (s *testStore) GetAnalysisResultByJobID(_ context.Context, _ uuid.UUID) (*models.AnalysisResult, error) {
	return nil, store.ErrNotFound
}
func (s *testStore) GetAnalysisResultByClusterID(_ context.Context, _ uuid.UUID) (*models.AnalysisResult, error) {
	return nil, store.ErrNotFound
}
func (s *testStore) CreateJob(_ context.Context, _ *models.Job) error { return nil }
func (s *testStore) GetJob(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*models.Job, error) {
	return nil, store.ErrNotFound
}
func (s *testStore) UpdateJobStatus(_ context.Context, _ uuid.UUID, _ string, _ ...store.JobUpdateOption) error {
	return nil
}

var _ store.Store = (*testStore)(nil)

// ─── mock cache ──────────────────────────────────────────────────────────────

type testCache struct {
	pingErr error
}

func (c *testCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error { return nil }
func (c *testCache) Get(_ context.Context, _ string) ([]byte, bool, error)            { return nil, false, nil }
func (c *testCache) Delete(_ context.Context, _ string) error                          { return nil }
func (c *testCache) Ping(_ context.Context) error                                      { return c.pingErr }
func (c *testCache) SetJobStatus(_ context.Context, _ uuid.UUID, _ string, _ time.Duration) error {
	return nil
}
func (c *testCache) GetJobStatus(_ context.Context, _ uuid.UUID) (string, bool, error) {
	return "", false, nil
}
func (c *testCache) IncrWithExpiry(_ context.Context, _ string, _ time.Duration) (int64, error) {
	return 1, nil
}

var _ cache.Cache = (*testCache)(nil)

// ─── health handler tests ───────────────────────────────────────────────────

func TestHealthHandler_AllOK(t *testing.T) {
	h := healthHandler(&testStore{}, &testCache{})

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	h(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	data := body["data"].(map[string]any)
	assert.Equal(t, "ok", data["status"])
	services := data["services"].(map[string]any)
	assert.Equal(t, "ok", services["database"])
	assert.Equal(t, "ok", services["cache"])
}

func TestHealthHandler_DatabaseDegraded(t *testing.T) {
	h := healthHandler(&testStore{pingErr: errors.New("connection refused")}, &testCache{})

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	h(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "DEGRADED", errObj["code"])
}

func TestHealthHandler_CacheDegraded(t *testing.T) {
	h := healthHandler(&testStore{}, &testCache{pingErr: errors.New("redis down")})

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	h(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHealthHandler_BothDegraded(t *testing.T) {
	h := healthHandler(
		&testStore{pingErr: errors.New("db down")},
		&testCache{pingErr: errors.New("redis down")},
	)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	h(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// ─── run() config validation tests ──────────────────────────────────────────

func TestRun_FailsOnMissingConfig(t *testing.T) {
	// Clear all env vars that config.Load() requires
	for _, key := range []string{
		"DATABASE_URL", "REDIS_URL", "LOKI_BASE_URL", "AI_PROVIDER",
	} {
		t.Setenv(key, "")
	}

	err := run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load config")
}

func TestRun_FailsOnInvalidDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "not-a-valid-url")
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("LOKI_BASE_URL", "http://localhost:3100")
	t.Setenv("AI_PROVIDER", "ollama")

	err := run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connect database")
}

func TestRun_FailsOnInvalidRedisURL(t *testing.T) {
	// Use a valid but unreachable database URL and invalid Redis URL
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:15432/testdb")
	t.Setenv("REDIS_URL", "redis://localhost:6379")
	t.Setenv("LOKI_BASE_URL", "http://localhost:3100")
	t.Setenv("AI_PROVIDER", "ollama")

	// This will fail on DB connect before reaching Redis
	err := run()
	require.Error(t, err)
}

// ─── shutdown timeout constant test ─────────────────────────────────────────

func TestShutdownTimeout(t *testing.T) {
	assert.Equal(t, 30*time.Second, shutdownTimeout)
}

// ─── helper: clear env ──────────────────────────────────────────────────────

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"DATABASE_URL", "REDIS_URL", "LOKI_BASE_URL", "AI_PROVIDER",
		"OPENAI_API_KEY", "ANTHROPIC_API_KEY",
	} {
		os.Unsetenv(key)
	}
}
