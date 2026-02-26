package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	mw "github.com/kiranshivaraju/loghunter/internal/api/middleware"
	"github.com/kiranshivaraju/loghunter/internal/api/response"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// ClusterLister is the store interface needed by NewListClustersHandler.
type ClusterLister interface {
	ListErrorClusters(ctx context.Context, filter store.ClusterFilter) ([]*models.ErrorCluster, int, error)
}

// ClusterGetter is the store interface needed by NewGetClusterHandler.
type ClusterGetter interface {
	GetErrorCluster(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*models.ErrorCluster, error)
	GetAnalysisResultByClusterID(ctx context.Context, clusterID uuid.UUID) (*models.AnalysisResult, error)
}

// NewListClustersHandler returns an http.HandlerFunc for GET /api/v1/clusters.
func NewListClustersHandler(st ClusterLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := mw.GetTenantID(r)
		if !ok {
			response.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Missing tenant", nil)
			return
		}

		q := r.URL.Query()

		page, _ := strconv.Atoi(q.Get("page"))
		if page < 1 {
			page = 1
		}

		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit < 1 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}

		filter := store.ClusterFilter{
			TenantID:  tenantID,
			Service:   q.Get("service"),
			Namespace: q.Get("namespace"),
			Level:     q.Get("level"),
			Page:      page,
			Limit:     limit,
		}

		if since := q.Get("since"); since != "" {
			dur, err := time.ParseDuration(since)
			if err != nil {
				response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "since must be a valid Go duration (e.g. 1h, 30m)", nil)
				return
			}
			filter.Since = time.Now().Add(-dur)
		}

		clusters, total, err := st.ListErrorClusters(r.Context(), filter)
		if err != nil {
			status, code, msg := mapError(err)
			response.Error(w, status, code, msg, nil)
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

// NewGetClusterHandler returns an http.HandlerFunc for GET /api/v1/clusters/{clusterID}.
func NewGetClusterHandler(st ClusterGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := mw.GetTenantID(r)
		if !ok {
			response.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Missing tenant", nil)
			return
		}

		clusterIDStr := chi.URLParam(r, "clusterID")
		clusterID, err := uuid.Parse(clusterIDStr)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_CLUSTER_ID", "Invalid cluster ID", nil)
			return
		}

		cluster, err := st.GetErrorCluster(r.Context(), clusterID, tenantID)
		if err != nil {
			response.Error(w, http.StatusNotFound, "CLUSTER_NOT_FOUND", "Cluster not found", nil)
			return
		}

		result := map[string]any{
			"cluster": cluster,
		}

		if ar, err := st.GetAnalysisResultByClusterID(r.Context(), clusterID); err == nil {
			result["analysis"] = ar
		}

		response.JSON(w, result)
	}
}
