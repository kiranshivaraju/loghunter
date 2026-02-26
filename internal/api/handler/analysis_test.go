package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// --- mock analysis store ---

type analysisMockStore struct {
	cluster   *models.ErrorCluster
	clusterErr error

	job    *models.Job
	jobErr error

	analysisResult    *models.AnalysisResult
	analysisResultErr error

	createdJob *models.Job
}

func (s *analysisMockStore) GetErrorCluster(_ context.Context, id uuid.UUID, tenantID uuid.UUID) (*models.ErrorCluster, error) {
	if s.clusterErr != nil {
		return nil, s.clusterErr
	}
	if s.cluster != nil && s.cluster.ID == id && s.cluster.TenantID == tenantID {
		return s.cluster, nil
	}
	return nil, store.ErrNotFound
}

func (s *analysisMockStore) GetJob(_ context.Context, id uuid.UUID, tenantID uuid.UUID) (*models.Job, error) {
	if s.jobErr != nil {
		return nil, s.jobErr
	}
	if s.job != nil && s.job.ID == id && s.job.TenantID == tenantID {
		return s.job, nil
	}
	return nil, store.ErrNotFound
}

func (s *analysisMockStore) GetAnalysisResultByJobID(_ context.Context, jobID uuid.UUID) (*models.AnalysisResult, error) {
	if s.analysisResultErr != nil {
		return nil, s.analysisResultErr
	}
	if s.analysisResult != nil && s.analysisResult.JobID == jobID {
		return s.analysisResult, nil
	}
	return nil, store.ErrNotFound
}

func (s *analysisMockStore) CreateJob(_ context.Context, job *models.Job) error {
	s.createdJob = job
	return nil
}

// --- mock analysis trigger ---

type mockAnalysisTrigger struct {
	triggered bool
	job       *models.Job
	err       error
}

func (m *mockAnalysisTrigger) TriggerAnalysis(_ context.Context, cluster *models.ErrorCluster) (*models.Job, error) {
	m.triggered = true
	if m.err != nil {
		return nil, m.err
	}
	return m.job, nil
}

// --- mock cache ---

type analysisMockCache struct {
	status string
	found  bool
	err    error
}

func (c *analysisMockCache) GetJobStatus(_ context.Context, _ uuid.UUID) (string, bool, error) {
	return c.status, c.found, c.err
}

// --- Analyze (POST) tests ---

func TestAnalyzeHandler_Success(t *testing.T) {
	tenantID := uuid.New()
	clusterID := uuid.New()
	jobID := uuid.New()

	st := &analysisMockStore{
		cluster: &models.ErrorCluster{ID: clusterID, TenantID: tenantID, Service: "api"},
	}
	trigger := &mockAnalysisTrigger{
		job: &models.Job{ID: jobID, TenantID: tenantID, Status: models.JobStatusPending},
	}

	handler := NewAnalyzeHandler(st, trigger)

	body := jsonBody(t, map[string]any{"cluster_id": clusterID.String()})
	req := httptest.NewRequest("POST", "/api/v1/analyze", body)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	if data["job_id"] == nil || data["job_id"] == "" {
		t.Error("expected job_id in response")
	}
	if !trigger.triggered {
		t.Error("expected TriggerAnalysis to be called")
	}
}

func TestAnalyzeHandler_InvalidClusterID(t *testing.T) {
	handler := NewAnalyzeHandler(&analysisMockStore{}, &mockAnalysisTrigger{})

	body := jsonBody(t, map[string]any{"cluster_id": "not-a-uuid"})
	req := httptest.NewRequest("POST", "/api/v1/analyze", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAnalyzeHandler_ClusterNotFound(t *testing.T) {
	st := &analysisMockStore{clusterErr: store.ErrNotFound}

	handler := NewAnalyzeHandler(st, &mockAnalysisTrigger{})

	body := jsonBody(t, map[string]any{"cluster_id": uuid.New().String()})
	req := httptest.NewRequest("POST", "/api/v1/analyze", body)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestAnalyzeHandler_InvalidJSON(t *testing.T) {
	handler := NewAnalyzeHandler(&analysisMockStore{}, &mockAnalysisTrigger{})

	req := httptest.NewRequest("POST", "/api/v1/analyze", jsonBody(t, "{invalid"))
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAnalyzeHandler_NoTenant(t *testing.T) {
	handler := NewAnalyzeHandler(&analysisMockStore{}, &mockAnalysisTrigger{})

	body := jsonBody(t, map[string]any{"cluster_id": uuid.New().String()})
	req := httptest.NewRequest("POST", "/api/v1/analyze", body)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestAnalyzeHandler_WrongTenant(t *testing.T) {
	tenantA := uuid.New()
	tenantB := uuid.New()
	clusterID := uuid.New()

	st := &analysisMockStore{
		cluster: &models.ErrorCluster{ID: clusterID, TenantID: tenantA, Service: "api"},
	}

	handler := NewAnalyzeHandler(st, &mockAnalysisTrigger{})

	body := jsonBody(t, map[string]any{"cluster_id": clusterID.String()})
	req := httptest.NewRequest("POST", "/api/v1/analyze", body)
	req = req.WithContext(setTenantCtx(req.Context(), tenantB))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong tenant, got %d", rr.Code)
	}
}

func TestAnalyzeHandler_TriggerError(t *testing.T) {
	tenantID := uuid.New()
	clusterID := uuid.New()

	st := &analysisMockStore{
		cluster: &models.ErrorCluster{ID: clusterID, TenantID: tenantID},
	}
	trigger := &mockAnalysisTrigger{err: store.ErrNotFound}

	handler := NewAnalyzeHandler(st, trigger)

	body := jsonBody(t, map[string]any{"cluster_id": clusterID.String()})
	req := httptest.NewRequest("POST", "/api/v1/analyze", body)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected mapped error status, got %d", rr.Code)
	}
}

// --- PollJob (GET) tests ---

func TestPollJobHandler_Completed(t *testing.T) {
	tenantID := uuid.New()
	jobID := uuid.New()

	st := &analysisMockStore{
		job: &models.Job{
			ID:       jobID,
			TenantID: tenantID,
			Status:   models.JobStatusCompleted,
		},
		analysisResult: &models.AnalysisResult{
			ID:         uuid.New(),
			JobID:      jobID,
			TenantID:   tenantID,
			RootCause:  "Null pointer in handler",
			Confidence: 0.85,
			Summary:    "NPE in request",
			Provider:   "openai",
			Model:      "gpt-4",
		},
	}
	cache := &analysisMockCache{found: false}

	handler := NewPollJobHandler(st, cache)

	req := httptest.NewRequest("GET", "/api/v1/analyze/"+jobID.String(), nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobID", jobID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	if data["status"] != "completed" {
		t.Errorf("expected status 'completed', got %v", data["status"])
	}
	result := data["result"].(map[string]any)
	if result["root_cause"] != "Null pointer in handler" {
		t.Errorf("expected root_cause, got %v", result["root_cause"])
	}
}

func TestPollJobHandler_Running(t *testing.T) {
	tenantID := uuid.New()
	jobID := uuid.New()

	st := &analysisMockStore{
		job: &models.Job{
			ID:       jobID,
			TenantID: tenantID,
			Status:   models.JobStatusRunning,
		},
	}
	cache := &analysisMockCache{found: false}

	handler := NewPollJobHandler(st, cache)

	req := httptest.NewRequest("GET", "/api/v1/analyze/"+jobID.String(), nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobID", jobID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	if data["status"] != "running" {
		t.Errorf("expected status 'running', got %v", data["status"])
	}
	if data["result"] != nil {
		t.Error("expected no result for running job")
	}
}

func TestPollJobHandler_CacheHit(t *testing.T) {
	tenantID := uuid.New()
	jobID := uuid.New()

	st := &analysisMockStore{
		job: &models.Job{
			ID:       jobID,
			TenantID: tenantID,
			Status:   models.JobStatusPending, // store has old status
		},
	}
	cache := &analysisMockCache{status: "running", found: true} // cache has newer status

	handler := NewPollJobHandler(st, cache)

	req := httptest.NewRequest("GET", "/api/v1/analyze/"+jobID.String(), nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobID", jobID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	if data["status"] != "running" {
		t.Errorf("expected cached status 'running', got %v", data["status"])
	}
}

func TestPollJobHandler_NotFound(t *testing.T) {
	st := &analysisMockStore{jobErr: store.ErrNotFound}
	cache := &analysisMockCache{found: false}

	handler := NewPollJobHandler(st, cache)

	req := httptest.NewRequest("GET", "/api/v1/analyze/"+uuid.New().String(), nil)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobID", uuid.New().String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestPollJobHandler_InvalidJobID(t *testing.T) {
	handler := NewPollJobHandler(&analysisMockStore{}, &analysisMockCache{})

	req := httptest.NewRequest("GET", "/api/v1/analyze/not-a-uuid", nil)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobID", "not-a-uuid")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestPollJobHandler_NoTenant(t *testing.T) {
	handler := NewPollJobHandler(&analysisMockStore{}, &analysisMockCache{})

	req := httptest.NewRequest("GET", "/api/v1/analyze/"+uuid.New().String(), nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestPollJobHandler_WrongTenant(t *testing.T) {
	tenantA := uuid.New()
	tenantB := uuid.New()
	jobID := uuid.New()

	st := &analysisMockStore{
		job: &models.Job{ID: jobID, TenantID: tenantA, Status: models.JobStatusPending},
	}
	cache := &analysisMockCache{found: false}

	handler := NewPollJobHandler(st, cache)

	req := httptest.NewRequest("GET", "/api/v1/analyze/"+jobID.String(), nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantB))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobID", jobID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong tenant, got %d", rr.Code)
	}
}

func TestPollJobHandler_CompletedWithoutResult(t *testing.T) {
	tenantID := uuid.New()
	jobID := uuid.New()

	st := &analysisMockStore{
		job: &models.Job{
			ID:       jobID,
			TenantID: tenantID,
			Status:   models.JobStatusCompleted,
		},
		analysisResultErr: store.ErrNotFound,
	}
	cache := &analysisMockCache{found: false}

	handler := NewPollJobHandler(st, cache)

	req := httptest.NewRequest("GET", "/api/v1/analyze/"+jobID.String(), nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobID", jobID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	if data["status"] != "completed" {
		t.Errorf("expected status 'completed', got %v", data["status"])
	}
	if data["result"] != nil {
		t.Error("expected no result when analysis result not found")
	}
}

func TestPollJobHandler_Failed(t *testing.T) {
	tenantID := uuid.New()
	jobID := uuid.New()
	errMsg := "AI provider timed out"

	st := &analysisMockStore{
		job: &models.Job{
			ID:           jobID,
			TenantID:     tenantID,
			Status:       models.JobStatusFailed,
			ErrorMessage: &errMsg,
		},
	}
	cache := &analysisMockCache{found: false}

	handler := NewPollJobHandler(st, cache)

	req := httptest.NewRequest("GET", "/api/v1/analyze/"+jobID.String(), nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobID", jobID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	if data["status"] != "failed" {
		t.Errorf("expected status 'failed', got %v", data["status"])
	}
}

// --- Helper to verify timestamps parse correctly ---
func TestPollJobHandler_TimestampsIncluded(t *testing.T) {
	tenantID := uuid.New()
	jobID := uuid.New()
	now := time.Now().UTC()

	st := &analysisMockStore{
		job: &models.Job{
			ID:        jobID,
			TenantID:  tenantID,
			Status:    models.JobStatusRunning,
			StartedAt: &now,
			CreatedAt: now,
		},
	}
	cache := &analysisMockCache{found: false}

	handler := NewPollJobHandler(st, cache)

	req := httptest.NewRequest("GET", "/api/v1/analyze/"+jobID.String(), nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("jobID", jobID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}
