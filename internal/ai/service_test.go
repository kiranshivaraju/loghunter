package ai

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/loki"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// --- mocks ---

type mockStore struct {
	mu             sync.Mutex
	jobs           map[uuid.UUID]*models.Job
	results        []*models.AnalysisResult
	statusUpdates  []statusUpdate
	createJobErr   error
	updateStatusErr error
	createResultErr error
}

type statusUpdate struct {
	ID     uuid.UUID
	Status string
	ErrMsg string
}

func newMockStore() *mockStore {
	return &mockStore{jobs: make(map[uuid.UUID]*models.Job)}
}

func (s *mockStore) Ping(_ context.Context) error { return nil }
func (s *mockStore) GetDefaultTenant(_ context.Context) (*models.Tenant, error) { return nil, nil }
func (s *mockStore) GetAPIKeyByPrefix(_ context.Context, _ string) ([]*models.APIKey, error) { return nil, nil }
func (s *mockStore) UpdateAPIKeyLastUsed(_ context.Context, _ uuid.UUID) error { return nil }
func (s *mockStore) CreateAPIKey(_ context.Context, _ *models.APIKey) error { return nil }
func (s *mockStore) ListAPIKeys(_ context.Context, _ uuid.UUID) ([]*models.APIKey, error) { return nil, nil }
func (s *mockStore) RevokeAPIKey(_ context.Context, _ uuid.UUID, _ uuid.UUID) error { return nil }
func (s *mockStore) UpsertErrorCluster(_ context.Context, _ *models.ErrorCluster) (*models.ErrorCluster, error) { return nil, nil }
func (s *mockStore) ListErrorClusters(_ context.Context, _ store.ClusterFilter) ([]*models.ErrorCluster, int, error) { return nil, 0, nil }
func (s *mockStore) GetErrorCluster(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*models.ErrorCluster, error) { return nil, nil }
func (s *mockStore) GetClustersByFingerprints(_ context.Context, _ uuid.UUID, _ []string) ([]*models.ErrorCluster, error) { return nil, nil }
func (s *mockStore) GetAnalysisResultByJobID(_ context.Context, _ uuid.UUID) (*models.AnalysisResult, error) { return nil, nil }
func (s *mockStore) GetAnalysisResultByClusterID(_ context.Context, _ uuid.UUID) (*models.AnalysisResult, error) { return nil, nil }
func (s *mockStore) GetJob(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*models.Job, error) { return nil, nil }

func (s *mockStore) CreateJob(_ context.Context, job *models.Job) error {
	if s.createJobErr != nil {
		return s.createJobErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	return nil
}

func (s *mockStore) UpdateJobStatus(_ context.Context, id uuid.UUID, status string, opts ...store.JobUpdateOption) error {
	if s.updateStatusErr != nil {
		return s.updateStatusErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	upd := statusUpdate{ID: id, Status: status}
	// Apply opts to extract error message
	type optParams struct {
		ErrorMessage *string
		ClusterID    *uuid.UUID
	}
	for _, opt := range opts {
		// We can't directly call opt, but we track status
		_ = opt
	}
	s.statusUpdates = append(s.statusUpdates, upd)
	return nil
}

func (s *mockStore) CreateAnalysisResult(_ context.Context, result *models.AnalysisResult) error {
	if s.createResultErr != nil {
		return s.createResultErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results = append(s.results, result)
	return nil
}

type mockCache struct {
	mu       sync.Mutex
	statuses map[string]string
}

func newMockCache() *mockCache {
	return &mockCache{statuses: make(map[string]string)}
}

func (c *mockCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error { return nil }
func (c *mockCache) Get(_ context.Context, _ string) ([]byte, bool, error) { return nil, false, nil }
func (c *mockCache) Delete(_ context.Context, _ string) error { return nil }
func (c *mockCache) Ping(_ context.Context) error { return nil }
func (c *mockCache) IncrWithExpiry(_ context.Context, _ string, _ time.Duration) (int64, error) { return 0, nil }

func (c *mockCache) SetJobStatus(_ context.Context, jobID uuid.UUID, status string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.statuses[jobID.String()] = status
	return nil
}

func (c *mockCache) GetJobStatus(_ context.Context, jobID uuid.UUID) (string, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.statuses[jobID.String()]
	return s, ok, nil
}

type mockLoki struct {
	lines []models.LogLine
	err   error
}

func (l *mockLoki) QueryRange(_ context.Context, _ loki.QueryRangeRequest) ([]models.LogLine, error) {
	return l.lines, l.err
}
func (l *mockLoki) Labels(_ context.Context) ([]string, error)                { return nil, nil }
func (l *mockLoki) LabelValues(_ context.Context, _ string) ([]string, error) { return nil, nil }
func (l *mockLoki) Ready(_ context.Context) error                             { return nil }

type mockProvider struct {
	name        string
	analyzeFunc func(ctx context.Context, req models.AnalysisRequest) (models.AnalysisResult, error)
	summarizeFunc func(ctx context.Context, logs []models.LogLine) (string, error)
}

func (p *mockProvider) Name() string { return p.name }
func (p *mockProvider) Analyze(ctx context.Context, req models.AnalysisRequest) (models.AnalysisResult, error) {
	if p.analyzeFunc != nil {
		return p.analyzeFunc(ctx, req)
	}
	return models.AnalysisResult{}, nil
}
func (p *mockProvider) Summarize(ctx context.Context, logs []models.LogLine) (string, error) {
	if p.summarizeFunc != nil {
		return p.summarizeFunc(ctx, logs)
	}
	return "", nil
}

// --- helpers ---

func testCluster() *models.ErrorCluster {
	return &models.ErrorCluster{
		ID:          uuid.New(),
		TenantID:    uuid.New(),
		Service:     "payments-api",
		Namespace:   "production",
		Fingerprint: "abc123",
		Level:       "error",
		FirstSeenAt: time.Now().Add(-10 * time.Minute),
		LastSeenAt:  time.Now(),
		Count:       5,
	}
}

func waitForGoroutine(t *testing.T, s *mockStore, expectedUpdates int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		s.mu.Lock()
		count := len(s.statusUpdates)
		s.mu.Unlock()
		if count >= expectedUpdates {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d status updates, got %d", expectedUpdates, count)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// --- TriggerAnalysis tests ---

func TestTriggerAnalysis_ReturnsJobImmediately(t *testing.T) {
	st := newMockStore()
	ca := newMockCache()
	lokiClient := &mockLoki{
		lines: []models.LogLine{{Timestamp: time.Now(), Message: "error msg", Level: "error"}},
	}
	provider := &mockProvider{
		name: "mock",
		analyzeFunc: func(_ context.Context, _ models.AnalysisRequest) (models.AnalysisResult, error) {
			// Simulate slow AI
			time.Sleep(100 * time.Millisecond)
			return models.AnalysisResult{
				RootCause:  "test root cause",
				Confidence: 0.9,
				Summary:    "test summary",
				Provider:   "mock",
				Model:      "mock-v1",
			}, nil
		},
	}

	svc := NewAnalysisService(provider, lokiClient, st, ca, 30*time.Second)

	cluster := testCluster()
	start := time.Now()
	job, err := svc.TriggerAnalysis(context.Background(), cluster)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job == nil {
		t.Fatal("expected job, got nil")
	}
	if job.Status != models.JobStatusPending {
		t.Errorf("expected status pending, got %s", job.Status)
	}
	if job.TenantID != cluster.TenantID {
		t.Errorf("expected tenant %s, got %s", cluster.TenantID, job.TenantID)
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("TriggerAnalysis should return immediately, took %v", elapsed)
	}

	// Cache should have pending status
	status, ok, _ := ca.GetJobStatus(context.Background(), job.ID)
	if !ok || status != models.JobStatusPending {
		t.Errorf("expected cached status 'pending', got %q (found=%v)", status, ok)
	}
}

func TestTriggerAnalysis_InvalidCluster(t *testing.T) {
	svc := NewAnalysisService(
		&mockProvider{name: "mock"},
		&mockLoki{},
		newMockStore(),
		newMockCache(),
		30*time.Second,
	)

	// Zero-value UUID cluster
	cluster := &models.ErrorCluster{}
	_, err := svc.TriggerAnalysis(context.Background(), cluster)
	if err == nil {
		t.Fatal("expected error for invalid cluster")
	}
}

func TestRunAnalysis_StoresResultOnSuccess(t *testing.T) {
	st := newMockStore()
	ca := newMockCache()
	lokiClient := &mockLoki{
		lines: []models.LogLine{
			{Timestamp: time.Now(), Message: "error msg", Level: "error", Labels: map[string]string{}},
		},
	}
	provider := &mockProvider{
		name: "mock",
		analyzeFunc: func(_ context.Context, _ models.AnalysisRequest) (models.AnalysisResult, error) {
			return models.AnalysisResult{
				RootCause:  "database connection pool exhausted",
				Confidence: 0.95,
				Summary:    "DB connection issue",
				Provider:   "mock",
				Model:      "mock-v1",
			}, nil
		},
	}

	svc := NewAnalysisService(provider, lokiClient, st, ca, 30*time.Second)
	cluster := testCluster()

	job, err := svc.TriggerAnalysis(context.Background(), cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for goroutine to complete (running + completed = 2 updates)
	waitForGoroutine(t, st, 2)

	// Verify result was stored
	st.mu.Lock()
	defer st.mu.Unlock()

	if len(st.results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(st.results))
	}
	result := st.results[0]
	if result.JobID != job.ID {
		t.Errorf("expected job ID %s, got %s", job.ID, result.JobID)
	}
	if result.RootCause != "database connection pool exhausted" {
		t.Errorf("unexpected root cause: %s", result.RootCause)
	}

	// Verify status updates: running then completed
	if len(st.statusUpdates) < 2 {
		t.Fatalf("expected at least 2 status updates, got %d", len(st.statusUpdates))
	}
	if st.statusUpdates[0].Status != models.JobStatusRunning {
		t.Errorf("expected first update to 'running', got %s", st.statusUpdates[0].Status)
	}
	if st.statusUpdates[1].Status != models.JobStatusCompleted {
		t.Errorf("expected second update to 'completed', got %s", st.statusUpdates[1].Status)
	}

	// Verify cache updated
	status, _, _ := ca.GetJobStatus(context.Background(), job.ID)
	if status != models.JobStatusCompleted {
		t.Errorf("expected cached status 'completed', got %s", status)
	}
}

func TestRunAnalysis_MarksJobFailedOnProviderError(t *testing.T) {
	st := newMockStore()
	ca := newMockCache()
	lokiClient := &mockLoki{
		lines: []models.LogLine{
			{Timestamp: time.Now(), Message: "error msg", Level: "error", Labels: map[string]string{}},
		},
	}
	provider := &mockProvider{
		name: "mock",
		analyzeFunc: func(_ context.Context, _ models.AnalysisRequest) (models.AnalysisResult, error) {
			return models.AnalysisResult{}, ErrProviderUnavailable
		},
	}

	svc := NewAnalysisService(provider, lokiClient, st, ca, 30*time.Second)
	cluster := testCluster()

	job, err := svc.TriggerAnalysis(context.Background(), cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for goroutine: running + failed = 2 updates
	waitForGoroutine(t, st, 2)

	st.mu.Lock()
	defer st.mu.Unlock()

	// No results stored
	if len(st.results) != 0 {
		t.Errorf("expected 0 results on failure, got %d", len(st.results))
	}

	// Last status should be failed
	lastUpdate := st.statusUpdates[len(st.statusUpdates)-1]
	if lastUpdate.Status != models.JobStatusFailed {
		t.Errorf("expected status 'failed', got %s", lastUpdate.Status)
	}

	// Cache should show failed
	status, _, _ := ca.GetJobStatus(context.Background(), job.ID)
	if status != models.JobStatusFailed {
		t.Errorf("expected cached status 'failed', got %s", status)
	}
}

func TestRunAnalysis_ClampsConfidence(t *testing.T) {
	st := newMockStore()
	provider := &mockProvider{
		name: "mock",
		analyzeFunc: func(_ context.Context, _ models.AnalysisRequest) (models.AnalysisResult, error) {
			return models.AnalysisResult{
				Confidence: 1.5, // > 1.0, should be clamped
				RootCause:  "cause",
				Summary:    "summary",
			}, nil
		},
	}

	svc := NewAnalysisService(provider,
		&mockLoki{lines: []models.LogLine{{Timestamp: time.Now(), Message: "err", Level: "error", Labels: map[string]string{}}}},
		st, newMockCache(), 30*time.Second)

	cluster := testCluster()
	svc.TriggerAnalysis(context.Background(), cluster)
	waitForGoroutine(t, st, 2)

	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(st.results))
	}
	if st.results[0].Confidence != 1.0 {
		t.Errorf("expected confidence clamped to 1.0, got %f", st.results[0].Confidence)
	}
}

func TestRunAnalysis_DoesNotPanic(t *testing.T) {
	st := newMockStore()
	provider := &mockProvider{
		name: "mock",
		analyzeFunc: func(_ context.Context, _ models.AnalysisRequest) (models.AnalysisResult, error) {
			panic("simulated panic")
		},
	}

	svc := NewAnalysisService(provider,
		&mockLoki{lines: []models.LogLine{{Timestamp: time.Now(), Message: "err", Level: "error", Labels: map[string]string{}}}},
		st, newMockCache(), 30*time.Second)

	cluster := testCluster()
	// Should not panic — goroutine recovers
	job, err := svc.TriggerAnalysis(context.Background(), cluster)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for failure update
	waitForGoroutine(t, st, 2)

	st.mu.Lock()
	defer st.mu.Unlock()
	lastUpdate := st.statusUpdates[len(st.statusUpdates)-1]
	if lastUpdate.Status != models.JobStatusFailed {
		t.Errorf("expected failed after panic, got %s", lastUpdate.Status)
	}
	_ = job
}

// --- Summarize tests ---

func TestSummarize_Success(t *testing.T) {
	lokiClient := &mockLoki{
		lines: []models.LogLine{
			{Timestamp: time.Now().Add(-2 * time.Minute), Message: "log line 1", Level: "info"},
			{Timestamp: time.Now().Add(-1 * time.Minute), Message: "log line 2", Level: "error"},
			{Timestamp: time.Now(), Message: "log line 3", Level: "warn"},
		},
	}
	provider := &mockProvider{
		name: "mock",
		summarizeFunc: func(_ context.Context, logs []models.LogLine) (string, error) {
			return "Summary of 3 log lines", nil
		},
	}

	svc := NewAnalysisService(provider, lokiClient, newMockStore(), newMockCache(), 30*time.Second)

	now := time.Now()
	result, err := svc.Summarize(context.Background(), SummarizeParams{
		TenantID:  uuid.New(),
		Service:   "api",
		Namespace: "prod",
		Start:     now.Add(-1 * time.Hour),
		End:       now,
		MaxLines:  500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "Summary of 3 log lines" {
		t.Errorf("unexpected summary: %s", result.Summary)
	}
	if result.LinesAnalyzed != 3 {
		t.Errorf("expected 3 lines analyzed, got %d", result.LinesAnalyzed)
	}
	if result.Provider != "mock" {
		t.Errorf("expected provider 'mock', got %s", result.Provider)
	}
}

func TestSummarize_NoLogsFound(t *testing.T) {
	lokiClient := &mockLoki{
		lines: []models.LogLine{}, // empty
	}
	provider := &mockProvider{name: "mock"}

	svc := NewAnalysisService(provider, lokiClient, newMockStore(), newMockCache(), 30*time.Second)

	_, err := svc.Summarize(context.Background(), SummarizeParams{
		TenantID:  uuid.New(),
		Service:   "api",
		Namespace: "prod",
		Start:     time.Now().Add(-1 * time.Hour),
		End:       time.Now(),
		MaxLines:  500,
	})
	if err == nil {
		t.Fatal("expected error for empty logs")
	}
	if !errors.Is(err, ErrNoLogsFound) {
		t.Errorf("expected ErrNoLogsFound, got: %v", err)
	}
}

func TestSummarize_ProviderError(t *testing.T) {
	lokiClient := &mockLoki{
		lines: []models.LogLine{
			{Timestamp: time.Now(), Message: "log", Level: "error"},
		},
	}
	provider := &mockProvider{
		name: "mock",
		summarizeFunc: func(_ context.Context, _ []models.LogLine) (string, error) {
			return "", ErrProviderUnavailable
		},
	}

	svc := NewAnalysisService(provider, lokiClient, newMockStore(), newMockCache(), 30*time.Second)

	_, err := svc.Summarize(context.Background(), SummarizeParams{
		TenantID: uuid.New(),
		Service:  "api",
		Start:    time.Now().Add(-1 * time.Hour),
		End:      time.Now(),
		MaxLines: 500,
	})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Errorf("expected ErrProviderUnavailable, got: %v", err)
	}
}

func TestSummarize_TruncatesLongLines(t *testing.T) {
	longMsg := ""
	for i := 0; i < 1000; i++ {
		longMsg += "x"
	}
	var capturedLogs []models.LogLine
	lokiClient := &mockLoki{
		lines: []models.LogLine{
			{Timestamp: time.Now(), Message: longMsg, Level: "error"},
		},
	}
	provider := &mockProvider{
		name: "mock",
		summarizeFunc: func(_ context.Context, logs []models.LogLine) (string, error) {
			capturedLogs = logs
			return "summary", nil
		},
	}

	svc := NewAnalysisService(provider, lokiClient, newMockStore(), newMockCache(), 30*time.Second)

	svc.Summarize(context.Background(), SummarizeParams{
		TenantID: uuid.New(),
		Service:  "api",
		Start:    time.Now().Add(-1 * time.Hour),
		End:      time.Now(),
		MaxLines: 500,
	})

	if len(capturedLogs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(capturedLogs))
	}
	if len(capturedLogs[0].Message) > 500 {
		t.Errorf("expected message truncated to 500 chars, got %d", len(capturedLogs[0].Message))
	}
}
