package ai

import (
	"context"
	"fmt"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/cache"
	"github.com/kiranshivaraju/loghunter/internal/loki"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"github.com/kiranshivaraju/loghunter/pkg/logql"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// SummarizeParams holds validated parameters for a summarization request.
type SummarizeParams struct {
	TenantID  uuid.UUID
	Service   string
	Namespace string
	Start     time.Time
	End       time.Time
	MaxLines  int
}

// SummarizeResult is the output of a summarization operation.
type SummarizeResult struct {
	Summary       string
	LinesAnalyzed int
	From          time.Time
	To            time.Time
	Provider      string
	Model         string
}

// AnalysisService orchestrates AI analysis and summarization.
type AnalysisService struct {
	provider models.AIProvider
	loki     loki.Client
	store    store.Store
	cache    cache.Cache
	timeout  time.Duration
}

// NewAnalysisService creates a new AnalysisService.
func NewAnalysisService(provider models.AIProvider, lokiClient loki.Client, st store.Store, ca cache.Cache, timeout time.Duration) *AnalysisService {
	return &AnalysisService{
		provider: provider,
		loki:     lokiClient,
		store:    st,
		cache:    ca,
		timeout:  timeout,
	}
}

// TriggerAnalysis creates a pending job and dispatches analysis in a background goroutine.
// Returns the job immediately without waiting for analysis to complete.
func (s *AnalysisService) TriggerAnalysis(ctx context.Context, cluster *models.ErrorCluster) (*models.Job, error) {
	if cluster.ID == uuid.Nil {
		return nil, fmt.Errorf("invalid cluster: ID is required")
	}

	job := &models.Job{
		ID:        uuid.New(),
		TenantID:  cluster.TenantID,
		Type:      "analysis",
		Status:    models.JobStatusPending,
		ClusterID: &cluster.ID,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.store.CreateJob(ctx, job); err != nil {
		return nil, fmt.Errorf("creating job: %w", err)
	}

	_ = s.cache.SetJobStatus(ctx, job.ID, models.JobStatusPending, 30*time.Minute)

	go s.runAnalysis(cluster, job.ID, cluster.TenantID)

	return job, nil
}

// runAnalysis performs the actual AI analysis in a goroutine.
// It recovers from panics and always marks the job as completed or failed.
func (s *AnalysisService) runAnalysis(cluster *models.ErrorCluster, jobID uuid.UUID, tenantID uuid.UUID) {
	ctx := context.Background()

	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in runAnalysis", "error", r, "job_id", jobID)
			_ = s.store.UpdateJobStatus(ctx, jobID, models.JobStatusFailed,
				store.WithErrorMessage(fmt.Sprintf("panic: %v", r)))
			_ = s.cache.SetJobStatus(ctx, jobID, models.JobStatusFailed, 30*time.Minute)
		}
	}()

	// Mark as running
	_ = s.store.UpdateJobStatus(ctx, jobID, models.JobStatusRunning)
	_ = s.cache.SetJobStatus(ctx, jobID, models.JobStatusRunning, 30*time.Minute)

	// Fetch context logs from Loki (±5 min around cluster window)
	qb := logql.QueryBuilder{}
	query := qb.BuildDetectionQuery(logql.DetectionParams{
		Service:   cluster.Service,
		Namespace: cluster.Namespace,
	})

	logs, err := s.loki.QueryRange(ctx, loki.QueryRangeRequest{
		Query: query,
		Start: cluster.FirstSeenAt.Add(-5 * time.Minute),
		End:   cluster.LastSeenAt.Add(5 * time.Minute),
		Limit: 1000,
	})
	if err != nil {
		_ = s.store.UpdateJobStatus(ctx, jobID, models.JobStatusFailed,
			store.WithErrorMessage(fmt.Sprintf("fetching logs: %v", err)))
		_ = s.cache.SetJobStatus(ctx, jobID, models.JobStatusFailed, 30*time.Minute)
		return
	}

	// Call AI provider with timeout
	analysisCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result, err := s.provider.Analyze(analysisCtx, models.AnalysisRequest{
		Cluster:     *cluster,
		ContextLogs: logs,
	})
	if err != nil {
		_ = s.store.UpdateJobStatus(ctx, jobID, models.JobStatusFailed,
			store.WithErrorMessage(err.Error()))
		_ = s.cache.SetJobStatus(ctx, jobID, models.JobStatusFailed, 30*time.Minute)
		return
	}

	// Clamp confidence to [0, 1]
	if result.Confidence < 0 {
		result.Confidence = 0
	}
	if result.Confidence > 1.0 {
		result.Confidence = 1.0
	}

	// Truncate fields
	result.RootCause = truncateString(result.RootCause, 4000)
	result.Summary = truncateString(result.Summary, 2000)

	// Store result
	result.ID = uuid.New()
	result.JobID = jobID
	result.ClusterID = cluster.ID
	result.TenantID = tenantID
	result.Provider = s.provider.Name()
	result.CreatedAt = time.Now().UTC()

	if err := s.store.CreateAnalysisResult(ctx, &result); err != nil {
		_ = s.store.UpdateJobStatus(ctx, jobID, models.JobStatusFailed,
			store.WithErrorMessage(fmt.Sprintf("storing result: %v", err)))
		_ = s.cache.SetJobStatus(ctx, jobID, models.JobStatusFailed, 30*time.Minute)
		return
	}

	// Mark completed
	_ = s.store.UpdateJobStatus(ctx, jobID, models.JobStatusCompleted,
		store.WithClusterID(cluster.ID))
	_ = s.cache.SetJobStatus(ctx, jobID, models.JobStatusCompleted, 30*time.Minute)
}

// Summarize fetches logs from Loki and sends them to the AI provider for summarization.
func (s *AnalysisService) Summarize(ctx context.Context, params SummarizeParams) (*SummarizeResult, error) {
	qb := logql.QueryBuilder{}
	query := qb.BuildSearchQuery(logql.SearchParams{
		Service:   params.Service,
		Namespace: params.Namespace,
	})

	logs, err := s.loki.QueryRange(ctx, loki.QueryRangeRequest{
		Query: query,
		Start: params.Start,
		End:   params.End,
		Limit: params.MaxLines,
	})
	if err != nil {
		return nil, fmt.Errorf("querying logs: %w", err)
	}

	if len(logs) == 0 {
		return nil, ErrNoLogsFound
	}

	// Truncate long messages before sending to AI
	for i := range logs {
		logs[i].Message = truncateString(logs[i].Message, 500)
	}

	summarizeCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	summary, err := s.provider.Summarize(summarizeCtx, logs)
	if err != nil {
		return nil, err
	}

	return &SummarizeResult{
		Summary:       summary,
		LinesAnalyzed: len(logs),
		From:          params.Start,
		To:            params.End,
		Provider:      s.provider.Name(),
	}, nil
}

// truncateString truncates s to maxBytes without splitting UTF-8 runes.
func truncateString(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}
