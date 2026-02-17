package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/api"
	mw "github.com/kiranshivaraju/loghunter/internal/api/middleware"
	"github.com/kiranshivaraju/loghunter/internal/api/response"
	"github.com/kiranshivaraju/loghunter/internal/cache"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"github.com/kiranshivaraju/loghunter/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// ─── test fixtures ───────────────────────────────────────────────────────────

var (
	testTenantID = uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	testRawKey   = "lhk_test_contract_key_1234567890"
	testPrefix   = testRawKey[:8]
	testCluster  = &models.ErrorCluster{
		ID:            uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc"),
		TenantID:      testTenantID,
		Service:       "api-gateway",
		Namespace:     "production",
		Fingerprint:   "fp-abc123",
		Level:         "error",
		FirstSeenAt:   time.Now().Add(-1 * time.Hour),
		LastSeenAt:    time.Now(),
		Count:         42,
		SampleMessage: "NullPointerException in handleRequest",
	}
	testJobID = uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd")
)

func testKeyHash() string {
	h, _ := bcrypt.GenerateFromPassword([]byte(testRawKey), bcrypt.MinCost)
	return string(h)
}

// ─── mock store ──────────────────────────────────────────────────────────────

type mockStore struct {
	keys     []*models.APIKey
	clusters []*models.ErrorCluster
	jobs     map[uuid.UUID]*models.Job
	results  map[uuid.UUID]*models.AnalysisResult
}

func newMockStore() *mockStore {
	return &mockStore{
		keys: []*models.APIKey{{
			ID:        uuid.New(),
			TenantID:  testTenantID,
			Name:      "test-key",
			KeyHash:   testKeyHash(),
			KeyPrefix: testPrefix,
			Scopes:    []string{"read", "write", "admin"},
		}},
		clusters: []*models.ErrorCluster{testCluster},
		jobs:     make(map[uuid.UUID]*models.Job),
		results:  make(map[uuid.UUID]*models.AnalysisResult),
	}
}

func (s *mockStore) Ping(_ context.Context) error { return nil }

func (s *mockStore) GetDefaultTenant(_ context.Context) (*models.Tenant, error) {
	return &models.Tenant{ID: testTenantID, Name: "test-tenant"}, nil
}

func (s *mockStore) GetAPIKeyByPrefix(_ context.Context, prefix string) ([]*models.APIKey, error) {
	var out []*models.APIKey
	for _, k := range s.keys {
		if k.KeyPrefix == prefix {
			out = append(out, k)
		}
	}
	return out, nil
}

func (s *mockStore) UpdateAPIKeyLastUsed(_ context.Context, _ uuid.UUID) error { return nil }

func (s *mockStore) CreateAPIKey(_ context.Context, key *models.APIKey) error {
	for _, existing := range s.keys {
		if existing.Name == key.Name && existing.TenantID == key.TenantID {
			return store.ErrDuplicateKey
		}
	}
	s.keys = append(s.keys, key)
	return nil
}

func (s *mockStore) ListAPIKeys(_ context.Context, tenantID uuid.UUID) ([]*models.APIKey, error) {
	var out []*models.APIKey
	for _, k := range s.keys {
		if k.TenantID == tenantID {
			out = append(out, k)
		}
	}
	return out, nil
}

func (s *mockStore) RevokeAPIKey(_ context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	for _, k := range s.keys {
		if k.ID == id && k.TenantID == tenantID {
			return nil
		}
	}
	return store.ErrNotFound
}

func (s *mockStore) UpsertErrorCluster(_ context.Context, c *models.ErrorCluster) (*models.ErrorCluster, error) {
	return c, nil
}

func (s *mockStore) ListErrorClusters(_ context.Context, f store.ClusterFilter) ([]*models.ErrorCluster, int, error) {
	var out []*models.ErrorCluster
	for _, c := range s.clusters {
		if c.TenantID != f.TenantID {
			continue
		}
		if f.Service != "" && c.Service != f.Service {
			continue
		}
		if f.Namespace != "" && c.Namespace != f.Namespace {
			continue
		}
		out = append(out, c)
	}
	return out, len(out), nil
}

func (s *mockStore) GetErrorCluster(_ context.Context, id uuid.UUID, tenantID uuid.UUID) (*models.ErrorCluster, error) {
	for _, c := range s.clusters {
		if c.ID == id && c.TenantID == tenantID {
			return c, nil
		}
	}
	return nil, store.ErrNotFound
}

func (s *mockStore) GetClustersByFingerprints(_ context.Context, _ uuid.UUID, _ []string) ([]*models.ErrorCluster, error) {
	return nil, nil
}

func (s *mockStore) CreateAnalysisResult(_ context.Context, r *models.AnalysisResult) error {
	s.results[r.JobID] = r
	return nil
}

func (s *mockStore) GetAnalysisResultByJobID(_ context.Context, jobID uuid.UUID) (*models.AnalysisResult, error) {
	if r, ok := s.results[jobID]; ok {
		return r, nil
	}
	return nil, store.ErrNotFound
}

func (s *mockStore) GetAnalysisResultByClusterID(_ context.Context, clusterID uuid.UUID) (*models.AnalysisResult, error) {
	for _, r := range s.results {
		if r.ClusterID == clusterID {
			return r, nil
		}
	}
	return nil, store.ErrNotFound
}

func (s *mockStore) CreateJob(_ context.Context, job *models.Job) error {
	s.jobs[job.ID] = job
	return nil
}

func (s *mockStore) GetJob(_ context.Context, id uuid.UUID, tenantID uuid.UUID) (*models.Job, error) {
	if j, ok := s.jobs[id]; ok && j.TenantID == tenantID {
		return j, nil
	}
	return nil, store.ErrNotFound
}

func (s *mockStore) UpdateJobStatus(_ context.Context, id uuid.UUID, status string, _ ...store.JobUpdateOption) error {
	if j, ok := s.jobs[id]; ok {
		j.Status = status
		return nil
	}
	return store.ErrNotFound
}

var _ store.Store = (*mockStore)(nil)

// ─── mock cache ──────────────────────────────────────────────────────────────

type mockCache struct {
	counters map[string]int64
}

func newMockCache() *mockCache {
	return &mockCache{counters: make(map[string]int64)}
}

func (c *mockCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error { return nil }
func (c *mockCache) Get(_ context.Context, _ string) ([]byte, bool, error)            { return nil, false, nil }
func (c *mockCache) Delete(_ context.Context, _ string) error                          { return nil }
func (c *mockCache) Ping(_ context.Context) error                                      { return nil }
func (c *mockCache) SetJobStatus(_ context.Context, _ uuid.UUID, _ string, _ time.Duration) error {
	return nil
}
func (c *mockCache) GetJobStatus(_ context.Context, _ uuid.UUID) (string, bool, error) {
	return "", false, nil
}
func (c *mockCache) IncrWithExpiry(_ context.Context, key string, _ time.Duration) (int64, error) {
	c.counters[key]++
	return c.counters[key], nil
}

var _ cache.Cache = (*mockCache)(nil)

// ─── test harness ────────────────────────────────────────────────────────────

type testServer struct {
	server *httptest.Server
	store  *mockStore
	cache  *mockCache
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()

	ms := newMockStore()
	mc := newMockCache()

	// Pre-populate a completed job with analysis result for poll tests
	completedJob := &models.Job{
		ID:       testJobID,
		TenantID: testTenantID,
		Type:     "analyze",
		Status:   models.JobStatusCompleted,
	}
	ms.jobs[testJobID] = completedJob
	ms.results[testJobID] = &models.AnalysisResult{
		ID:         uuid.New(),
		ClusterID:  testCluster.ID,
		TenantID:   testTenantID,
		JobID:      testJobID,
		Provider:   "mock",
		Model:      "test",
		RootCause:  "Null pointer in request handler",
		Confidence: 0.85,
		Summary:    "NPE caused by uninitialized field",
	}

	deps := api.Dependencies{
		Auth:      mw.NewAuth(ms),
		RateLimit: mw.NewRateLimit(mc, 10), // low limit for rate-limit tests

		HealthHandler:    healthHandler(ms, mc),
		AnalyzeHandler:   analyzeHandler(ms),
		PollJobHandler:   pollJobHandler(ms),
		ListClusters:     listClustersHandler(ms),
		GetCluster:       getClusterHandler(ms),
		SummarizeHandler: summarizeHandler(),
		SearchHandler:    searchHandler(),
		CreateKeyHandler: createKeyHandler(ms),
		ListKeysHandler:  listKeysHandler(ms),
		RevokeKeyHandler: revokeKeyHandler(ms),
	}

	router := api.NewRouter(deps)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	return &testServer{server: srv, store: ms, cache: mc}
}

func (ts *testServer) authRequest(method, path string, body any) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, ts.server.URL+path, &buf)
	req.Header.Set("Authorization", "Bearer "+testRawKey)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func (ts *testServer) unauthRequest(method, path string) *http.Request {
	req, _ := http.NewRequest(method, ts.server.URL+path, nil)
	return req
}

func parseBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return body
}

// ─── mock handlers ───────────────────────────────────────────────────────────
// These simulate the real handler contracts without real infrastructure.

func healthHandler(s *mockStore, c *mockCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbErr := false
		cacheErr := c.Ping(r.Context()) != nil

		status := "ok"
		if dbErr || cacheErr {
			status = "degraded"
			response.Error(w, http.StatusServiceUnavailable, "DEGRADED", "One or more services degraded", map[string]any{
				"database": "ok",
				"cache":    "degraded",
			})
			return
		}
		response.JSON(w, map[string]string{"status": status})
	}
}

func analyzeHandler(s *mockStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := mw.GetTenantID(r)
		if !ok {
			response.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Missing tenant", nil)
			return
		}

		var req struct {
			ClusterID string `json:"cluster_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", nil)
			return
		}

		clusterID, err := uuid.Parse(req.ClusterID)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_CLUSTER_ID", "Invalid cluster_id format", nil)
			return
		}

		if _, err := s.GetErrorCluster(r.Context(), clusterID, tenantID); err != nil {
			response.Error(w, http.StatusNotFound, "CLUSTER_NOT_FOUND", "Cluster not found", nil)
			return
		}

		jobID := uuid.New()
		job := &models.Job{
			ID:       jobID,
			TenantID: tenantID,
			Type:     "analyze",
			Status:   models.JobStatusPending,
		}
		s.CreateJob(r.Context(), job)

		response.Accepted(w, map[string]string{"job_id": jobID.String()})
	}
}

func pollJobHandler(s *mockStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := mw.GetTenantID(r)
		if !ok {
			response.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Missing tenant", nil)
			return
		}

		// Extract jobID from URL using chi
		jobIDStr := r.PathValue("jobID")
		if jobIDStr == "" {
			// Fallback: parse from URL path
			parts := splitPath(r.URL.Path)
			if len(parts) >= 4 {
				jobIDStr = parts[3] // /api/v1/analyze/{jobID}
			}
		}

		jobID, err := uuid.Parse(jobIDStr)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_JOB_ID", "Invalid job ID format", nil)
			return
		}

		job, err := s.GetJob(r.Context(), jobID, tenantID)
		if err != nil {
			response.Error(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", nil)
			return
		}

		result := map[string]any{
			"job_id": job.ID.String(),
			"status": job.Status,
		}

		if job.Status == models.JobStatusCompleted {
			if ar, err := s.GetAnalysisResultByJobID(r.Context(), jobID); err == nil {
				result["result"] = map[string]any{
					"root_cause":  ar.RootCause,
					"confidence":  ar.Confidence,
					"summary":     ar.Summary,
					"provider":    ar.Provider,
					"model":       ar.Model,
				}
			}
		}

		response.JSON(w, result)
	}
}

func listClustersHandler(s *mockStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := mw.GetTenantID(r)
		if !ok {
			response.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Missing tenant", nil)
			return
		}

		filter := store.ClusterFilter{
			TenantID:  tenantID,
			Service:   r.URL.Query().Get("service"),
			Namespace: r.URL.Query().Get("namespace"),
			Page:      1,
			Limit:     20,
		}

		clusters, total, err := s.ListErrorClusters(r.Context(), filter)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list clusters", nil)
			return
		}

		response.Collection(w, clusters, response.PaginationMeta{
			Page:    filter.Page,
			Limit:   filter.Limit,
			Total:   total,
			HasNext: total > filter.Page*filter.Limit,
		})
	}
}

func getClusterHandler(s *mockStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := mw.GetTenantID(r)
		if !ok {
			response.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Missing tenant", nil)
			return
		}

		clusterIDStr := r.PathValue("clusterID")
		if clusterIDStr == "" {
			parts := splitPath(r.URL.Path)
			if len(parts) >= 4 {
				clusterIDStr = parts[3]
			}
		}

		clusterID, err := uuid.Parse(clusterIDStr)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_CLUSTER_ID", "Invalid cluster ID", nil)
			return
		}

		cluster, err := s.GetErrorCluster(r.Context(), clusterID, tenantID)
		if err != nil {
			response.Error(w, http.StatusNotFound, "CLUSTER_NOT_FOUND", "Cluster not found", nil)
			return
		}

		result := map[string]any{
			"cluster": cluster,
		}

		if ar, err := s.GetAnalysisResultByClusterID(r.Context(), clusterID); err == nil {
			result["analysis"] = ar
		}

		response.JSON(w, result)
	}
}

func summarizeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := mw.GetTenantID(r)
		if !ok {
			response.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Missing tenant", nil)
			return
		}

		var req struct {
			Logs []models.LogLine `json:"logs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", nil)
			return
		}

		if len(req.Logs) == 0 {
			response.Error(w, http.StatusNotFound, "NO_LOGS", "No log lines provided", nil)
			return
		}

		response.JSON(w, map[string]string{
			"summary": fmt.Sprintf("Summary of %d log lines: multiple errors detected in service", len(req.Logs)),
		})
	}
}

func searchHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := mw.GetTenantID(r)
		if !ok {
			response.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Missing tenant", nil)
			return
		}

		var req struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", nil)
			return
		}

		if req.Query == "" {
			response.Error(w, http.StatusBadRequest, "INVALID_QUERY", "Search query is required", nil)
			return
		}

		response.JSON(w, map[string]any{
			"results":   []any{},
			"query":     req.Query,
			"cache_hit": false,
		})
	}
}

func createKeyHandler(s *mockStore) http.HandlerFunc {
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

		rawKey := fmt.Sprintf("lhk_%s_%s", req.Name, uuid.New().String()[:8])
		hash, _ := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)

		key := &models.APIKey{
			ID:        uuid.New(),
			TenantID:  tenantID,
			Name:      req.Name,
			KeyHash:   string(hash),
			KeyPrefix: rawKey[:8],
			Scopes:    req.Scopes,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := s.CreateAPIKey(r.Context(), key); err != nil {
			if err == store.ErrDuplicateKey {
				response.Error(w, http.StatusConflict, "DUPLICATE_KEY", "API key with this name already exists", nil)
				return
			}
			response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create key", nil)
			return
		}

		response.Created(w, map[string]any{
			"id":        key.ID.String(),
			"name":      key.Name,
			"key":       rawKey, // Only shown once at creation
			"scopes":    key.Scopes,
			"created_at": key.CreatedAt,
		})
	}
}

func listKeysHandler(s *mockStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := mw.GetTenantID(r)
		if !ok {
			response.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Missing tenant", nil)
			return
		}

		keys, err := s.ListAPIKeys(r.Context(), tenantID)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list keys", nil)
			return
		}

		// Strip sensitive fields — raw key must never be exposed in list
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

func revokeKeyHandler(s *mockStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := mw.GetTenantID(r)
		if !ok {
			response.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Missing tenant", nil)
			return
		}

		keyIDStr := r.PathValue("keyID")
		if keyIDStr == "" {
			parts := splitPath(r.URL.Path)
			if len(parts) >= 5 {
				keyIDStr = parts[4]
			}
		}

		keyID, err := uuid.Parse(keyIDStr)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_KEY_ID", "Invalid key ID", nil)
			return
		}

		if err := s.RevokeAPIKey(r.Context(), keyID, tenantID); err != nil {
			response.Error(w, http.StatusNotFound, "KEY_NOT_FOUND", "API key not found", nil)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func splitPath(path string) []string {
	var parts []string
	for _, p := range bytes.Split([]byte(path), []byte("/")) {
		if len(p) > 0 {
			parts = append(parts, string(p))
		}
	}
	return parts
}

// ═══════════════════════════════════════════════════════════════════════════════
// CONTRACT TESTS
// ═══════════════════════════════════════════════════════════════════════════════

// ─── GET /api/v1/health ──────────────────────────────────────────────────────

func TestHealth_200_AllOK(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.unauthRequest("GET", "/api/v1/health"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body := parseBody(t, resp)
	data := body["data"].(map[string]any)
	assert.Equal(t, "ok", data["status"])
}

func TestHealth_Unauthenticated(t *testing.T) {
	ts := newTestServer(t)

	// Health endpoint must be accessible without auth
	resp, err := http.DefaultClient.Do(ts.unauthRequest("GET", "/api/v1/health"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ─── POST /api/v1/analyze ────────────────────────────────────────────────────

func TestAnalyze_202_WithJobID(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.authRequest("POST", "/api/v1/analyze", map[string]string{
		"cluster_id": testCluster.ID.String(),
	}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	body := parseBody(t, resp)
	data := body["data"].(map[string]any)
	assert.NotEmpty(t, data["job_id"])

	// Verify job_id is valid UUID
	_, err = uuid.Parse(data["job_id"].(string))
	assert.NoError(t, err)
}

func TestAnalyze_400_InvalidClusterID(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.authRequest("POST", "/api/v1/analyze", map[string]string{
		"cluster_id": "not-a-uuid",
	}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body := parseBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "INVALID_CLUSTER_ID", errObj["code"])
}

func TestAnalyze_401_MissingToken(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.unauthRequest("POST", "/api/v1/analyze"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	body := parseBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "INVALID_TOKEN", errObj["code"])
}

func TestAnalyze_404_ClusterNotFound(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.authRequest("POST", "/api/v1/analyze", map[string]string{
		"cluster_id": uuid.New().String(), // random UUID, doesn't exist
	}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ─── GET /api/v1/analyze/{jobID} ────────────────────────────────────────────

func TestPollJob_200_WithResult(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.authRequest("GET", "/api/v1/analyze/"+testJobID.String(), nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body := parseBody(t, resp)
	data := body["data"].(map[string]any)
	assert.Equal(t, "completed", data["status"])
	assert.NotNil(t, data["result"])

	result := data["result"].(map[string]any)
	assert.NotEmpty(t, result["root_cause"])
	assert.NotEmpty(t, result["summary"])
	assert.NotZero(t, result["confidence"])
}

func TestPollJob_200_StatusRunning(t *testing.T) {
	ts := newTestServer(t)

	// Create a running job
	runningJobID := uuid.New()
	ts.store.jobs[runningJobID] = &models.Job{
		ID:       runningJobID,
		TenantID: testTenantID,
		Type:     "analyze",
		Status:   models.JobStatusRunning,
	}

	resp, err := http.DefaultClient.Do(ts.authRequest("GET", "/api/v1/analyze/"+runningJobID.String(), nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body := parseBody(t, resp)
	data := body["data"].(map[string]any)
	assert.Equal(t, "running", data["status"])
	assert.Nil(t, data["result"]) // no result yet
}

func TestPollJob_404_WrongTenant(t *testing.T) {
	ts := newTestServer(t)

	// Create a job for a different tenant
	otherJobID := uuid.New()
	otherTenantID := uuid.New()
	ts.store.jobs[otherJobID] = &models.Job{
		ID:       otherJobID,
		TenantID: otherTenantID, // different tenant
		Type:     "analyze",
		Status:   models.JobStatusPending,
	}

	resp, err := http.DefaultClient.Do(ts.authRequest("GET", "/api/v1/analyze/"+otherJobID.String(), nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ─── GET /api/v1/clusters ───────────────────────────────────────────────────

func TestListClusters_200_Paginated(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.authRequest("GET", "/api/v1/clusters", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body := parseBody(t, resp)

	// Verify collection envelope with meta
	assert.NotNil(t, body["data"])
	meta := body["meta"].(map[string]any)
	assert.NotNil(t, meta["page"])
	assert.NotNil(t, meta["limit"])
	assert.NotNil(t, meta["total"])
}

func TestListClusters_200_FiltersApplied(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.authRequest("GET", "/api/v1/clusters?service=api-gateway&namespace=production", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body := parseBody(t, resp)
	meta := body["meta"].(map[string]any)
	assert.Equal(t, float64(1), meta["total"]) // our test cluster matches
}

func TestListClusters_401_NoToken(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.unauthRequest("GET", "/api/v1/clusters"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// ─── GET /api/v1/clusters/{id} ──────────────────────────────────────────────

func TestGetCluster_200_WithAnalysis(t *testing.T) {
	ts := newTestServer(t)

	// Add analysis result for the test cluster
	ts.store.results[uuid.New()] = &models.AnalysisResult{
		ID:        uuid.New(),
		ClusterID: testCluster.ID,
		TenantID:  testTenantID,
		JobID:     testJobID,
		RootCause: "NPE in handler",
		Summary:   "Null pointer exception",
	}

	resp, err := http.DefaultClient.Do(ts.authRequest("GET", "/api/v1/clusters/"+testCluster.ID.String(), nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body := parseBody(t, resp)
	data := body["data"].(map[string]any)
	assert.NotNil(t, data["cluster"])
	assert.NotNil(t, data["analysis"])
}

func TestGetCluster_404_Missing(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.authRequest("GET", "/api/v1/clusters/"+uuid.New().String(), nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	body := parseBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "CLUSTER_NOT_FOUND", errObj["code"])
}

// ─── POST /api/v1/summarize ─────────────────────────────────────────────────

func TestSummarize_200_Summary(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.authRequest("POST", "/api/v1/summarize", map[string]any{
		"logs": []map[string]any{
			{"timestamp": time.Now().Format(time.RFC3339), "message": "error: connection refused", "level": "error"},
			{"timestamp": time.Now().Format(time.RFC3339), "message": "retry attempt 1", "level": "warn"},
		},
	}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body := parseBody(t, resp)
	data := body["data"].(map[string]any)
	assert.NotEmpty(t, data["summary"])
}

func TestSummarize_404_NoLogs(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.authRequest("POST", "/api/v1/summarize", map[string]any{
		"logs": []any{},
	}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	body := parseBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "NO_LOGS", errObj["code"])
}

// ─── POST /api/v1/search ────────────────────────────────────────────────────

func TestSearch_200_Results(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.authRequest("POST", "/api/v1/search", map[string]string{
		"query": "connection timeout",
	}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body := parseBody(t, resp)
	data := body["data"].(map[string]any)
	assert.Equal(t, "connection timeout", data["query"])
	assert.NotNil(t, data["results"])
}

func TestSearch_400_EmptyQuery(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.authRequest("POST", "/api/v1/search", map[string]string{
		"query": "",
	}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body := parseBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "INVALID_QUERY", errObj["code"])
}

// ─── POST /api/v1/admin/keys ────────────────────────────────────────────────

func TestCreateKey_201_WithRawKey(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.authRequest("POST", "/api/v1/admin/keys", map[string]any{
		"name":   "my-new-key",
		"scopes": []string{"read", "write"},
	}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	body := parseBody(t, resp)
	data := body["data"].(map[string]any)
	assert.NotEmpty(t, data["id"])
	assert.NotEmpty(t, data["key"]) // raw key shown at creation
	assert.Equal(t, "my-new-key", data["name"])
}

func TestCreateKey_409_Duplicate(t *testing.T) {
	ts := newTestServer(t)

	// The mock store already has a key named "test-key" for testTenantID
	resp, err := http.DefaultClient.Do(ts.authRequest("POST", "/api/v1/admin/keys", map[string]any{
		"name":   "test-key",
		"scopes": []string{"read"},
	}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	body := parseBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "DUPLICATE_KEY", errObj["code"])
}

func TestListKeys_DoesNotExposeRawKey(t *testing.T) {
	ts := newTestServer(t)

	// GET /api/v1/admin/keys — requires admin scope
	resp, err := http.DefaultClient.Do(ts.authRequest("GET", "/api/v1/admin/keys", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body := parseBody(t, resp)
	data := body["data"].([]any)
	require.NotEmpty(t, data)

	firstKey := data[0].(map[string]any)
	assert.NotEmpty(t, firstKey["key_prefix"])
	assert.Nil(t, firstKey["key"])      // raw key NOT exposed
	assert.Nil(t, firstKey["key_hash"]) // hash NOT exposed
}

// ─── Auth middleware contract ────────────────────────────────────────────────

func TestAuth_AllProtectedEndpoints_Reject401(t *testing.T) {
	ts := newTestServer(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/analyze"},
		{"GET", "/api/v1/analyze/" + testJobID.String()},
		{"GET", "/api/v1/clusters"},
		{"GET", "/api/v1/clusters/" + testCluster.ID.String()},
		{"POST", "/api/v1/summarize"},
		{"POST", "/api/v1/search"},
		{"POST", "/api/v1/admin/keys"},
		{"GET", "/api/v1/admin/keys"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			resp, err := http.DefaultClient.Do(ts.unauthRequest(ep.method, ep.path))
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
			body := parseBody(t, resp)
			errObj := body["error"].(map[string]any)
			assert.Equal(t, "INVALID_TOKEN", errObj["code"])
		})
	}
}

func TestAuth_InvalidBearerToken(t *testing.T) {
	ts := newTestServer(t)

	req, _ := http.NewRequest("GET", ts.server.URL+"/api/v1/clusters", nil)
	req.Header.Set("Authorization", "Bearer wrong_key_that_does_not_match")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// ─── Rate limiting contract ─────────────────────────────────────────────────

func TestRateLimit_Headers_Present(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.authRequest("GET", "/api/v1/clusters", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Limit"))
	assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Reset"))
}

func TestRateLimit_429_Exceeded(t *testing.T) {
	ts := newTestServer(t)

	// The rate limit is set to 10 in newTestServer
	// Send 11 requests to trigger rate limiting
	var lastResp *http.Response
	for i := 0; i < 11; i++ {
		resp, err := http.DefaultClient.Do(ts.authRequest("GET", "/api/v1/clusters", nil))
		require.NoError(t, err)
		if i < 10 {
			resp.Body.Close()
		} else {
			lastResp = resp
		}
	}
	defer lastResp.Body.Close()

	assert.Equal(t, http.StatusTooManyRequests, lastResp.StatusCode)
	assert.NotEmpty(t, lastResp.Header.Get("Retry-After"))

	body := parseBody(t, lastResp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", errObj["code"])
}

// ─── Admin scope contract ───────────────────────────────────────────────────

func TestAdminEndpoints_403_WithoutAdminScope(t *testing.T) {
	ts := newTestServer(t)

	// Create a key without admin scope
	noAdminKey := "lhk_noadmin_1234567890abcdef"
	noAdminHash, _ := bcrypt.GenerateFromPassword([]byte(noAdminKey), bcrypt.MinCost)
	ts.store.keys = append(ts.store.keys, &models.APIKey{
		ID:        uuid.New(),
		TenantID:  testTenantID,
		Name:      "no-admin-key",
		KeyHash:   string(noAdminHash),
		KeyPrefix: noAdminKey[:8],
		Scopes:    []string{"read", "write"}, // no "admin"
	})

	adminEndpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/admin/keys"},
		{"GET", "/api/v1/admin/keys"},
	}

	for _, ep := range adminEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ts.server.URL+ep.path, bytes.NewBuffer([]byte(`{"name":"x","scopes":["read"]}`)))
			req.Header.Set("Authorization", "Bearer "+noAdminKey)
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusForbidden, resp.StatusCode)
			body := parseBody(t, resp)
			errObj := body["error"].(map[string]any)
			assert.Equal(t, "FORBIDDEN", errObj["code"])
		})
	}
}

// ─── Response format contract ───────────────────────────────────────────────

func TestResponseFormat_SuccessEnvelope(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.unauthRequest("GET", "/api/v1/health"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	body := parseBody(t, resp)
	assert.Contains(t, body, "data")
}

func TestResponseFormat_ErrorEnvelope(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.DefaultClient.Do(ts.unauthRequest("POST", "/api/v1/analyze"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	body := parseBody(t, resp)
	assert.Contains(t, body, "error")
	errObj := body["error"].(map[string]any)
	assert.NotEmpty(t, errObj["code"])
	assert.NotEmpty(t, errObj["message"])
}
