package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// --- mock cluster store ---

type clusterMockStore struct {
	clusters []*models.ErrorCluster
	total    int
	listErr  error

	cluster    *models.ErrorCluster
	getErr     error

	analysis   *models.AnalysisResult
	analysisErr error

	capturedFilter *store.ClusterFilter
}

func (s *clusterMockStore) ListErrorClusters(_ context.Context, filter store.ClusterFilter) ([]*models.ErrorCluster, int, error) {
	s.capturedFilter = &filter
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.clusters, s.total, nil
}

func (s *clusterMockStore) GetErrorCluster(_ context.Context, id uuid.UUID, tenantID uuid.UUID) (*models.ErrorCluster, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.cluster != nil && s.cluster.ID == id && s.cluster.TenantID == tenantID {
		return s.cluster, nil
	}
	return nil, store.ErrNotFound
}

func (s *clusterMockStore) GetAnalysisResultByClusterID(_ context.Context, clusterID uuid.UUID) (*models.AnalysisResult, error) {
	if s.analysisErr != nil {
		return nil, s.analysisErr
	}
	if s.analysis != nil && s.analysis.ClusterID == clusterID {
		return s.analysis, nil
	}
	return nil, store.ErrNotFound
}

// --- ListClusters tests ---

func TestListClustersHandler_Success(t *testing.T) {
	tenantID := uuid.New()
	now := time.Now()
	st := &clusterMockStore{
		clusters: []*models.ErrorCluster{
			{
				ID:            uuid.New(),
				TenantID:      tenantID,
				Service:       "api",
				Namespace:     "default",
				Fingerprint:   "abc123",
				Level:         "error",
				FirstSeenAt:   now.Add(-1 * time.Hour),
				LastSeenAt:    now,
				Count:         42,
				SampleMessage: "connection refused",
			},
		},
		total: 1,
	}

	handler := NewListClustersHandler(st)

	req := httptest.NewRequest("GET", "/api/v1/clusters", nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	resp := parseJSON(t, rr)
	data := resp["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(data))
	}
	meta := resp["meta"].(map[string]any)
	if meta["total"] != float64(1) {
		t.Errorf("expected total 1, got %v", meta["total"])
	}
	if meta["page"] != float64(1) {
		t.Errorf("expected page 1, got %v", meta["page"])
	}
	if meta["limit"] != float64(20) {
		t.Errorf("expected limit 20, got %v", meta["limit"])
	}
	if meta["has_next"] != false {
		t.Errorf("expected has_next false, got %v", meta["has_next"])
	}
}

func TestListClustersHandler_Pagination(t *testing.T) {
	tenantID := uuid.New()
	st := &clusterMockStore{
		clusters: []*models.ErrorCluster{
			{ID: uuid.New(), TenantID: tenantID, Service: "api"},
		},
		total: 50, // total > page*limit so has_next should be true
	}

	handler := NewListClustersHandler(st)

	req := httptest.NewRequest("GET", "/api/v1/clusters?page=2&limit=10", nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	resp := parseJSON(t, rr)
	meta := resp["meta"].(map[string]any)
	if meta["page"] != float64(2) {
		t.Errorf("expected page 2, got %v", meta["page"])
	}
	if meta["limit"] != float64(10) {
		t.Errorf("expected limit 10, got %v", meta["limit"])
	}
	if meta["has_next"] != true {
		t.Errorf("expected has_next true")
	}

	if st.capturedFilter.Page != 2 {
		t.Errorf("expected filter page 2, got %d", st.capturedFilter.Page)
	}
	if st.capturedFilter.Limit != 10 {
		t.Errorf("expected filter limit 10, got %d", st.capturedFilter.Limit)
	}
}

func TestListClustersHandler_Filters(t *testing.T) {
	tenantID := uuid.New()
	st := &clusterMockStore{clusters: []*models.ErrorCluster{}, total: 0}

	handler := NewListClustersHandler(st)

	req := httptest.NewRequest("GET", "/api/v1/clusters?service=api&namespace=production&level=error&since=2h", nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if st.capturedFilter.Service != "api" {
		t.Errorf("expected service 'api', got %q", st.capturedFilter.Service)
	}
	if st.capturedFilter.Namespace != "production" {
		t.Errorf("expected namespace 'production', got %q", st.capturedFilter.Namespace)
	}
	if st.capturedFilter.Level != "error" {
		t.Errorf("expected level 'error', got %q", st.capturedFilter.Level)
	}
	if st.capturedFilter.Since.IsZero() {
		t.Error("expected since to be set")
	}
}

func TestListClustersHandler_InvalidSince(t *testing.T) {
	handler := NewListClustersHandler(&clusterMockStore{})

	req := httptest.NewRequest("GET", "/api/v1/clusters?since=invalid", nil)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestListClustersHandler_LimitClamping(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantLimit int
	}{
		{"default", "", 20},
		{"custom", "?limit=50", 50},
		{"over max clamped", "?limit=200", 100},
		{"zero defaults", "?limit=0", 20},
		{"negative defaults", "?limit=-5", 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &clusterMockStore{clusters: []*models.ErrorCluster{}, total: 0}
			handler := NewListClustersHandler(st)

			req := httptest.NewRequest("GET", "/api/v1/clusters"+tt.query, nil)
			req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
			}
			if st.capturedFilter.Limit != tt.wantLimit {
				t.Errorf("expected limit %d, got %d", tt.wantLimit, st.capturedFilter.Limit)
			}
		})
	}
}

func TestListClustersHandler_NoTenant(t *testing.T) {
	handler := NewListClustersHandler(&clusterMockStore{})

	req := httptest.NewRequest("GET", "/api/v1/clusters", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestListClustersHandler_StoreError(t *testing.T) {
	st := &clusterMockStore{listErr: errors.New("db failure")}

	handler := NewListClustersHandler(st)

	req := httptest.NewRequest("GET", "/api/v1/clusters", nil)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestListClustersHandler_TenantIDPassedToFilter(t *testing.T) {
	tenantID := uuid.New()
	st := &clusterMockStore{clusters: []*models.ErrorCluster{}, total: 0}

	handler := NewListClustersHandler(st)

	req := httptest.NewRequest("GET", "/api/v1/clusters", nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if st.capturedFilter.TenantID != tenantID {
		t.Errorf("expected tenant %s, got %s", tenantID, st.capturedFilter.TenantID)
	}
}

// --- GetCluster tests ---

func TestGetClusterHandler_Success(t *testing.T) {
	tenantID := uuid.New()
	clusterID := uuid.New()
	st := &clusterMockStore{
		cluster: &models.ErrorCluster{
			ID:            clusterID,
			TenantID:      tenantID,
			Service:       "api",
			SampleMessage: "timeout error",
			Count:         10,
		},
	}

	handler := NewGetClusterHandler(st)

	req := httptest.NewRequest("GET", "/api/v1/clusters/"+clusterID.String(), nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("clusterID", clusterID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	cluster := data["cluster"].(map[string]any)
	if cluster["service"] != "api" {
		t.Errorf("expected service 'api', got %v", cluster["service"])
	}
}

func TestGetClusterHandler_WithAnalysis(t *testing.T) {
	tenantID := uuid.New()
	clusterID := uuid.New()
	st := &clusterMockStore{
		cluster: &models.ErrorCluster{
			ID:       clusterID,
			TenantID: tenantID,
			Service:  "api",
		},
		analysis: &models.AnalysisResult{
			ID:         uuid.New(),
			ClusterID:  clusterID,
			TenantID:   tenantID,
			RootCause:  "Null pointer in handler",
			Confidence: 0.85,
			Summary:    "NPE in request handling",
		},
	}

	handler := NewGetClusterHandler(st)

	req := httptest.NewRequest("GET", "/api/v1/clusters/"+clusterID.String(), nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("clusterID", clusterID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	if data["analysis"] == nil {
		t.Error("expected analysis in response")
	}
	analysis := data["analysis"].(map[string]any)
	if analysis["root_cause"] != "Null pointer in handler" {
		t.Errorf("expected root_cause 'Null pointer in handler', got %v", analysis["root_cause"])
	}
}

func TestGetClusterHandler_WithoutAnalysis(t *testing.T) {
	tenantID := uuid.New()
	clusterID := uuid.New()
	st := &clusterMockStore{
		cluster: &models.ErrorCluster{
			ID:       clusterID,
			TenantID: tenantID,
			Service:  "api",
		},
		analysisErr: store.ErrNotFound,
	}

	handler := NewGetClusterHandler(st)

	req := httptest.NewRequest("GET", "/api/v1/clusters/"+clusterID.String(), nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantID))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("clusterID", clusterID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	resp := parseJSON(t, rr)
	data := resp["data"].(map[string]any)
	if data["analysis"] != nil {
		t.Error("expected no analysis in response when not available")
	}
}

func TestGetClusterHandler_NotFound(t *testing.T) {
	st := &clusterMockStore{getErr: store.ErrNotFound}

	handler := NewGetClusterHandler(st)

	req := httptest.NewRequest("GET", "/api/v1/clusters/"+uuid.New().String(), nil)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("clusterID", uuid.New().String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestGetClusterHandler_InvalidClusterID(t *testing.T) {
	handler := NewGetClusterHandler(&clusterMockStore{})

	req := httptest.NewRequest("GET", "/api/v1/clusters/not-a-uuid", nil)
	req = req.WithContext(setTenantCtx(req.Context(), uuid.New()))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("clusterID", "not-a-uuid")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestGetClusterHandler_NoTenant(t *testing.T) {
	handler := NewGetClusterHandler(&clusterMockStore{})

	req := httptest.NewRequest("GET", "/api/v1/clusters/"+uuid.New().String(), nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestGetClusterHandler_WrongTenant(t *testing.T) {
	tenantA := uuid.New()
	tenantB := uuid.New()
	clusterID := uuid.New()
	st := &clusterMockStore{
		cluster: &models.ErrorCluster{
			ID:       clusterID,
			TenantID: tenantA,
			Service:  "api",
		},
	}

	handler := NewGetClusterHandler(st)

	req := httptest.NewRequest("GET", "/api/v1/clusters/"+clusterID.String(), nil)
	req = req.WithContext(setTenantCtx(req.Context(), tenantB))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("clusterID", clusterID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong tenant, got %d", rr.Code)
	}
}
