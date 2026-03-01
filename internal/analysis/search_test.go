package analysis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/api/handler"
	"github.com/kiranshivaraju/loghunter/internal/loki"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// --- mock loki client ---

type mockLokiClient struct {
	lines []models.LogLine
	err   error
}

func (m *mockLokiClient) QueryRange(_ context.Context, _ loki.QueryRangeRequest) ([]models.LogLine, error) {
	return m.lines, m.err
}
func (m *mockLokiClient) Labels(_ context.Context) ([]string, error)              { return nil, nil }
func (m *mockLokiClient) LabelValues(_ context.Context, _ string) ([]string, error) { return nil, nil }
func (m *mockLokiClient) Ready(_ context.Context) error                            { return nil }

// --- mock store ---

type mockSearchStore struct {
	clusters []*models.ErrorCluster
}

func (m *mockSearchStore) Ping(_ context.Context) error { return nil }
func (m *mockSearchStore) GetDefaultTenant(_ context.Context) (*models.Tenant, error) {
	return nil, nil
}
func (m *mockSearchStore) GetErrorCluster(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*models.ErrorCluster, error) {
	return nil, nil
}
func (m *mockSearchStore) GetClustersByFingerprints(_ context.Context, _ uuid.UUID, _ []string) ([]*models.ErrorCluster, error) {
	return m.clusters, nil
}
func (m *mockSearchStore) ListErrorClusters(_ context.Context, _ store.ClusterFilter) ([]*models.ErrorCluster, int, error) {
	return nil, 0, nil
}
func (m *mockSearchStore) UpsertErrorCluster(_ context.Context, _ *models.ErrorCluster) (*models.ErrorCluster, error) {
	return nil, nil
}
func (m *mockSearchStore) CreateJob(_ context.Context, _ *models.Job) error { return nil }
func (m *mockSearchStore) GetJob(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*models.Job, error) {
	return nil, nil
}
func (m *mockSearchStore) UpdateJobStatus(_ context.Context, _ uuid.UUID, _ string, _ ...store.JobUpdateOption) error {
	return nil
}
func (m *mockSearchStore) CreateAnalysisResult(_ context.Context, _ *models.AnalysisResult) error {
	return nil
}
func (m *mockSearchStore) GetAnalysisResultByJobID(_ context.Context, _ uuid.UUID) (*models.AnalysisResult, error) {
	return nil, nil
}
func (m *mockSearchStore) GetAnalysisResultByClusterID(_ context.Context, _ uuid.UUID) (*models.AnalysisResult, error) {
	return nil, nil
}
func (m *mockSearchStore) GetAPIKeyByPrefix(_ context.Context, _ string) ([]*models.APIKey, error) {
	return nil, nil
}
func (m *mockSearchStore) UpdateAPIKeyLastUsed(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockSearchStore) CreateAPIKey(_ context.Context, _ *models.APIKey) error    { return nil }
func (m *mockSearchStore) ListAPIKeys(_ context.Context, _ uuid.UUID) ([]*models.APIKey, error) {
	return nil, nil
}
func (m *mockSearchStore) RevokeAPIKey(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return nil
}
func (m *mockSearchStore) UpsertErrorClusterFull(_ context.Context, _ *models.ErrorCluster) (store.UpsertResult, error) {
	return store.UpsertResult{}, nil
}
func (m *mockSearchStore) CreateWatcherFinding(_ context.Context, _ *models.WatcherFinding) error {
	return nil
}
func (m *mockSearchStore) ListWatcherFindings(_ context.Context, _ uuid.UUID, _ int) ([]*models.WatcherFinding, error) {
	return nil, nil
}

// --- mock cache ---

type mockSearchCache struct {
	data  map[string][]byte
	setCalled bool
}

func newMockCache() *mockSearchCache {
	return &mockSearchCache{data: make(map[string][]byte)}
}

func (m *mockSearchCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	m.data[key] = value
	m.setCalled = true
	return nil
}
func (m *mockSearchCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	v, ok := m.data[key]
	return v, ok, nil
}
func (m *mockSearchCache) Delete(_ context.Context, _ string) error { return nil }
func (m *mockSearchCache) Ping(_ context.Context) error             { return nil }
func (m *mockSearchCache) SetJobStatus(_ context.Context, _ uuid.UUID, _ string, _ time.Duration) error {
	return nil
}
func (m *mockSearchCache) GetJobStatus(_ context.Context, _ uuid.UUID) (string, bool, error) {
	return "", false, nil
}
func (m *mockSearchCache) IncrWithExpiry(_ context.Context, _ string, _ time.Duration) (int64, error) {
	return 0, nil
}

// --- tests ---

func searchParams() handler.SearchParams {
	return handler.SearchParams{
		TenantID:  uuid.New(),
		Service:   "payments-api",
		Namespace: "default",
		Start:     time.Now().Add(-1 * time.Hour),
		End:       time.Now(),
		Keyword:   "timeout",
		Limit:     100,
	}
}

func TestSearch_CacheHit(t *testing.T) {
	mc := newMockCache()
	lokiClient := &mockLokiClient{} // should NOT be called
	st := &mockSearchStore{}

	svc := NewSearchService(lokiClient, st, mc)
	params := searchParams()

	// Pre-populate cache
	cached := handler.SearchResult{
		Results: []handler.SearchResultLine{
			{Message: "cached result", Level: "ERROR"},
		},
		Query:    `{service="payments-api"}`,
		CacheHit: false,
	}
	data, _ := json.Marshal(cached)
	filterHash := svc.buildFilterHash(params)
	key := "loki:search:" + params.TenantID.String() + ":" + filterHash
	mc.data[key] = data

	result, err := svc.Search(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.CacheHit {
		t.Error("expected CacheHit to be true")
	}
	if len(result.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].Message != "cached result" {
		t.Errorf("unexpected message: %s", result.Results[0].Message)
	}
}

func TestSearch_CacheMiss_CallsLoki(t *testing.T) {
	mc := newMockCache()
	lines := []models.LogLine{
		{Timestamp: time.Now(), Message: "connection timeout", Level: "ERROR", Labels: map[string]string{"service": "api"}},
		{Timestamp: time.Now(), Message: "retrying request", Level: "WARN", Labels: map[string]string{"service": "api"}},
	}
	lokiClient := &mockLokiClient{lines: lines}
	st := &mockSearchStore{}

	svc := NewSearchService(lokiClient, st, mc)
	params := searchParams()

	result, err := svc.Search(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CacheHit {
		t.Error("expected CacheHit to be false")
	}
	if len(result.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(result.Results))
	}
	if result.Results[0].Message != "connection timeout" {
		t.Errorf("unexpected first message: %s", result.Results[0].Message)
	}
	if !mc.setCalled {
		t.Error("expected cache Set to be called")
	}
}

func TestSearch_HasNext_LimitPlusOne(t *testing.T) {
	// When Loki returns limit+1 lines, we should only return limit lines
	lines := make([]models.LogLine, 4) // limit is 3, so 4 means has_next
	for i := range lines {
		lines[i] = models.LogLine{
			Timestamp: time.Now(),
			Message:   "log line",
			Level:     "INFO",
			Labels:    map[string]string{},
		}
	}
	lokiClient := &mockLokiClient{lines: lines}
	mc := newMockCache()
	st := &mockSearchStore{}

	svc := NewSearchService(lokiClient, st, mc)
	params := searchParams()
	params.Limit = 3

	result, err := svc.Search(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Results) != 3 {
		t.Errorf("expected 3 results (limit), got %d", len(result.Results))
	}
}

func TestSearch_ClusterIDAttached(t *testing.T) {
	clusterID := uuid.New()
	lines := []models.LogLine{
		{Timestamp: time.Now(), Message: "connection timeout", Level: "ERROR", Labels: map[string]string{}},
	}
	lokiClient := &mockLokiClient{lines: lines}
	mc := newMockCache()

	fp := Fingerprint("connection timeout")
	st := &mockSearchStore{
		clusters: []*models.ErrorCluster{
			{ID: clusterID, Fingerprint: fp},
		},
	}

	svc := NewSearchService(lokiClient, st, mc)
	params := searchParams()

	result, err := svc.Search(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Results[0].ClusterID == nil {
		t.Fatal("expected ClusterID to be set")
	}
	if *result.Results[0].ClusterID != clusterID {
		t.Errorf("unexpected cluster ID: %v", *result.Results[0].ClusterID)
	}
}

func TestSearch_LokiError(t *testing.T) {
	lokiClient := &mockLokiClient{err: loki.ErrLokiUnreachable}
	mc := newMockCache()
	st := &mockSearchStore{}

	svc := NewSearchService(lokiClient, st, mc)
	params := searchParams()

	_, err := svc.Search(context.Background(), params)
	if err == nil {
		t.Fatal("expected error")
	}
}
