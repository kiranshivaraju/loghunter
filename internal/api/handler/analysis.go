package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	mw "github.com/kiranshivaraju/loghunter/internal/api/middleware"
	"github.com/kiranshivaraju/loghunter/internal/api/response"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// AnalysisClusterGetter retrieves a cluster for validation before triggering analysis.
type AnalysisClusterGetter interface {
	GetErrorCluster(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*models.ErrorCluster, error)
}

// AnalysisTrigger starts an async analysis job for a cluster.
type AnalysisTrigger interface {
	TriggerAnalysis(ctx context.Context, cluster *models.ErrorCluster) (*models.Job, error)
}

// JobPoller is the store interface needed by NewPollJobHandler.
type JobPoller interface {
	GetJob(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*models.Job, error)
	GetAnalysisResultByJobID(ctx context.Context, jobID uuid.UUID) (*models.AnalysisResult, error)
}

// JobStatusCache provides a fast path for checking job status.
type JobStatusCache interface {
	GetJobStatus(ctx context.Context, jobID uuid.UUID) (string, bool, error)
}

// NewAnalyzeHandler returns an http.HandlerFunc for POST /api/v1/analyze.
func NewAnalyzeHandler(st AnalysisClusterGetter, trigger AnalysisTrigger) http.HandlerFunc {
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

		cluster, err := st.GetErrorCluster(r.Context(), clusterID, tenantID)
		if err != nil {
			response.Error(w, http.StatusNotFound, "CLUSTER_NOT_FOUND", "Cluster not found", nil)
			return
		}

		job, err := trigger.TriggerAnalysis(r.Context(), cluster)
		if err != nil {
			status, code, msg := mapError(err)
			response.Error(w, status, code, msg, nil)
			return
		}

		response.Accepted(w, map[string]string{"job_id": job.ID.String()})
	}
}

// NewPollJobHandler returns an http.HandlerFunc for GET /api/v1/analyze/{jobID}.
func NewPollJobHandler(st JobPoller, cache JobStatusCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := mw.GetTenantID(r)
		if !ok {
			response.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "Missing tenant", nil)
			return
		}

		jobIDStr := chi.URLParam(r, "jobID")
		jobID, err := uuid.Parse(jobIDStr)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_JOB_ID", "Invalid job ID format", nil)
			return
		}

		job, err := st.GetJob(r.Context(), jobID, tenantID)
		if err != nil {
			response.Error(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", nil)
			return
		}

		// Check cache for a more recent status
		status := job.Status
		if cachedStatus, found, err := cache.GetJobStatus(r.Context(), jobID); err == nil && found {
			status = cachedStatus
		}

		result := map[string]any{
			"job_id": job.ID.String(),
			"status": status,
		}

		if status == models.JobStatusCompleted {
			if ar, err := st.GetAnalysisResultByJobID(r.Context(), jobID); err == nil {
				result["result"] = map[string]any{
					"root_cause": ar.RootCause,
					"confidence": ar.Confidence,
					"summary":    ar.Summary,
					"provider":   ar.Provider,
					"model":      ar.Model,
				}
			}
		}

		response.JSON(w, result)
	}
}
