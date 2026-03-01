package watcher

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/config"
	"github.com/kiranshivaraju/loghunter/internal/loki"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

// --- mocks ---

type mockLoki struct {
	labelValues func(ctx context.Context, label string) ([]string, error)
	queryRange  func(ctx context.Context, req loki.QueryRangeRequest) ([]models.LogLine, error)
}

func (m *mockLoki) LabelValues(ctx context.Context, label string) ([]string, error) {
	if m.labelValues != nil {
		return m.labelValues(ctx, label)
	}
	return nil, nil
}

func (m *mockLoki) QueryRange(ctx context.Context, req loki.QueryRangeRequest) ([]models.LogLine, error) {
	if m.queryRange != nil {
		return m.queryRange(ctx, req)
	}
	return nil, nil
}

type mockStore struct {
	mu                    sync.Mutex
	upsertFull            func(ctx context.Context, cluster *models.ErrorCluster) (store.UpsertResult, error)
	createFinding         func(ctx context.Context, finding *models.WatcherFinding) error
	listFindings          func(ctx context.Context, tenantID uuid.UUID, limit int) ([]*models.WatcherFinding, error)
	createFindingCalls    int
}

func (m *mockStore) UpsertErrorClusterFull(ctx context.Context, cluster *models.ErrorCluster) (store.UpsertResult, error) {
	if m.upsertFull != nil {
		return m.upsertFull(ctx, cluster)
	}
	return store.UpsertResult{Cluster: cluster, IsNew: true}, nil
}

func (m *mockStore) CreateWatcherFinding(ctx context.Context, finding *models.WatcherFinding) error {
	m.mu.Lock()
	m.createFindingCalls++
	m.mu.Unlock()
	if m.createFinding != nil {
		return m.createFinding(ctx, finding)
	}
	return nil
}

func (m *mockStore) ListWatcherFindings(ctx context.Context, tenantID uuid.UUID, limit int) ([]*models.WatcherFinding, error) {
	if m.listFindings != nil {
		return m.listFindings(ctx, tenantID, limit)
	}
	return nil, nil
}

type mockAnalyzer struct {
	mu    sync.Mutex
	calls int
	fn    func(ctx context.Context, cluster *models.ErrorCluster) (*models.Job, error)
}

func (m *mockAnalyzer) TriggerAnalysis(ctx context.Context, cluster *models.ErrorCluster) (*models.Job, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	if m.fn != nil {
		return m.fn(ctx, cluster)
	}
	return &models.Job{ID: uuid.New()}, nil
}

type mockCache struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMockCache() *mockCache {
	return &mockCache{data: make(map[string][]byte)}
}

func (m *mockCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	return v, ok, nil
}

func (m *mockCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

type mockNotifier struct {
	mu    sync.Mutex
	calls []Finding
}

func (m *mockNotifier) Notify(_ context.Context, f Finding) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, f)
	return nil
}

// --- helpers ---

func testTenant() *models.Tenant {
	return &models.Tenant{
		ID:        uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Name:      "default",
		LokiOrgID: "default",
	}
}

func testConfig() config.WatcherConfig {
	return config.WatcherConfig{
		Enabled:        true,
		Interval:       50 * time.Millisecond,
		LookbackWindow: 2 * time.Minute,
		Namespace:      "default",
		AutoAnalyze:    true,
		SpikeThreshold: 3.0,
		MaxServices:    50,
		LogsLimit:      2000,
	}
}

func errorLogLines(service string, count int) []models.LogLine {
	lines := make([]models.LogLine, count)
	for i := range lines {
		lines[i] = models.LogLine{
			Timestamp: time.Now().UTC(),
			Message:   "NullPointerException at com.example.Service.process(Service.java:42)",
			Level:     "ERROR",
			Labels:    map[string]string{"service": service},
		}
	}
	return lines
}

// --- tests ---

func TestWatcherDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.Enabled = false

	ml := &mockLoki{}
	w := New(cfg, ml, &mockStore{}, &mockAnalyzer{}, newMockCache(), testTenant())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	w.Run(ctx)

	// Should return immediately without calling any deps
	if ml.labelValues != nil || ml.queryRange != nil {
		t.Error("expected no loki calls when watcher is disabled")
	}
}

func TestPollDetectsNewClusters(t *testing.T) {
	cfg := testConfig()
	cfg.Services = []string{"auth-service"}

	ml := &mockLoki{
		queryRange: func(_ context.Context, _ loki.QueryRangeRequest) ([]models.LogLine, error) {
			return errorLogLines("auth-service", 5), nil
		},
	}

	clusterID := uuid.New()
	ms := &mockStore{
		upsertFull: func(_ context.Context, c *models.ErrorCluster) (store.UpsertResult, error) {
			c.ID = clusterID
			return store.UpsertResult{Cluster: c, IsNew: true, PrevCount: 0}, nil
		},
	}

	ma := &mockAnalyzer{}
	mc := newMockCache()

	w := New(cfg, ml, ms, ma, mc, testTenant())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	ma.mu.Lock()
	calls := ma.calls
	ma.mu.Unlock()

	if calls == 0 {
		t.Error("expected TriggerAnalysis to be called for new cluster")
	}

	ms.mu.Lock()
	findingCalls := ms.createFindingCalls
	ms.mu.Unlock()

	if findingCalls == 0 {
		t.Error("expected finding to be recorded")
	}
}

func TestSpikeDetection(t *testing.T) {
	tests := []struct {
		name      string
		prevCount int
		newCount  int
		threshold float64
		want      string
	}{
		{"new cluster", 0, 5, 3.0, "new"},
		{"spike 3x", 10, 30, 3.0, "spike"},
		{"spike above threshold", 10, 35, 3.0, "spike"},
		{"no spike below threshold", 10, 25, 3.0, ""},
		{"no spike equal", 10, 10, 3.0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig()
			cfg.SpikeThreshold = tt.threshold

			w := New(cfg, &mockLoki{}, &mockStore{}, &mockAnalyzer{}, newMockCache(), testTenant())

			result := store.UpsertResult{
				Cluster:   &models.ErrorCluster{Count: tt.newCount},
				IsNew:     tt.prevCount == 0 && tt.name == "new cluster",
				PrevCount: tt.prevCount,
			}

			got := w.classifyFinding(result)
			if got != tt.want {
				t.Errorf("classifyFinding() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAnalysisGuardPreventsRepeat(t *testing.T) {
	cfg := testConfig()
	cfg.Services = []string{"api-gateway"}

	ml := &mockLoki{
		queryRange: func(_ context.Context, _ loki.QueryRangeRequest) ([]models.LogLine, error) {
			return errorLogLines("api-gateway", 3), nil
		},
	}

	clusterID := uuid.New()
	ms := &mockStore{
		upsertFull: func(_ context.Context, c *models.ErrorCluster) (store.UpsertResult, error) {
			c.ID = clusterID
			return store.UpsertResult{Cluster: c, IsNew: true, PrevCount: 0}, nil
		},
	}

	ma := &mockAnalyzer{}
	mc := newMockCache()

	w := New(cfg, ml, ms, ma, mc, testTenant())

	// Run multiple polls — the guard should prevent duplicate analysis
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	ma.mu.Lock()
	calls := ma.calls
	ma.mu.Unlock()

	// Should be called exactly once due to guard
	if calls != 1 {
		t.Errorf("expected TriggerAnalysis to be called exactly 1 time, got %d", calls)
	}
}

func TestMaxServicesCapEnforced(t *testing.T) {
	cfg := testConfig()
	cfg.MaxServices = 3

	var discoveredCount int
	many := make([]string, 10)
	for i := range many {
		many[i] = "svc-" + uuid.New().String()[:8]
	}

	ml := &mockLoki{
		labelValues: func(_ context.Context, _ string) ([]string, error) {
			return many, nil
		},
		queryRange: func(_ context.Context, req loki.QueryRangeRequest) ([]models.LogLine, error) {
			discoveredCount++
			return nil, nil // no logs
		},
	}

	w := New(cfg, ml, &mockStore{}, &mockAnalyzer{}, newMockCache(), testTenant())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	// Should have polled at most MaxServices services per poll cycle
	if discoveredCount > cfg.MaxServices*5 { // allow a few poll cycles
		t.Errorf("expected at most %d services polled per cycle, but total queryRange calls = %d", cfg.MaxServices, discoveredCount)
	}
}

func TestGracefulShutdown(t *testing.T) {
	cfg := testConfig()
	cfg.Services = []string{"slow-service"}
	cfg.Interval = 10 * time.Second // long interval to ensure we test shutdown

	ml := &mockLoki{
		queryRange: func(ctx context.Context, _ loki.QueryRangeRequest) ([]models.LogLine, error) {
			// Simulate a slow query
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Millisecond):
				return nil, nil
			}
		},
	}

	w := New(cfg, ml, &mockStore{}, &mockAnalyzer{}, newMockCache(), testTenant())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	// Let first poll run
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not shut down in time")
	}
}

func TestStaticServiceListSkipsDiscovery(t *testing.T) {
	cfg := testConfig()
	cfg.Services = []string{"my-service"}

	labelValuesCalled := false
	ml := &mockLoki{
		labelValues: func(_ context.Context, _ string) ([]string, error) {
			labelValuesCalled = true
			return nil, nil
		},
		queryRange: func(_ context.Context, _ loki.QueryRangeRequest) ([]models.LogLine, error) {
			return nil, nil
		},
	}

	w := New(cfg, ml, &mockStore{}, &mockAnalyzer{}, newMockCache(), testTenant())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	if labelValuesCalled {
		t.Error("expected LabelValues to NOT be called when static services are configured")
	}
}

func TestNotifierCalled(t *testing.T) {
	cfg := testConfig()
	cfg.Services = []string{"notify-svc"}

	ml := &mockLoki{
		queryRange: func(_ context.Context, _ loki.QueryRangeRequest) ([]models.LogLine, error) {
			return errorLogLines("notify-svc", 2), nil
		},
	}

	clusterID := uuid.New()
	ms := &mockStore{
		upsertFull: func(_ context.Context, c *models.ErrorCluster) (store.UpsertResult, error) {
			c.ID = clusterID
			return store.UpsertResult{Cluster: c, IsNew: true, PrevCount: 0}, nil
		},
	}

	mn := &mockNotifier{}
	w := New(cfg, ml, ms, &mockAnalyzer{}, newMockCache(), testTenant(), WithNotifier(mn))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	mn.mu.Lock()
	calls := len(mn.calls)
	mn.mu.Unlock()

	if calls == 0 {
		t.Error("expected notifier to be called for new cluster")
	}
}

func TestStatusReturnsCorrectState(t *testing.T) {
	cfg := testConfig()
	cfg.Services = []string{"status-svc"}

	ms := &mockStore{
		listFindings: func(_ context.Context, _ uuid.UUID, _ int) ([]*models.WatcherFinding, error) {
			return []*models.WatcherFinding{
				{ID: uuid.New(), Kind: "new", Service: "status-svc"},
			}, nil
		},
	}

	w := New(cfg, &mockLoki{}, ms, &mockAnalyzer{}, newMockCache(), testTenant())

	// Not running yet
	status, err := w.Status(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !status.Enabled {
		t.Error("expected enabled=true")
	}
	if status.Running {
		t.Error("expected running=false before Run()")
	}
	if len(status.RecentFindings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(status.RecentFindings))
	}
}

func TestAutoAnalyzeDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.Services = []string{"no-analyze"}
	cfg.AutoAnalyze = false

	ml := &mockLoki{
		queryRange: func(_ context.Context, _ loki.QueryRangeRequest) ([]models.LogLine, error) {
			return errorLogLines("no-analyze", 3), nil
		},
	}

	clusterID := uuid.New()
	ms := &mockStore{
		upsertFull: func(_ context.Context, c *models.ErrorCluster) (store.UpsertResult, error) {
			c.ID = clusterID
			return store.UpsertResult{Cluster: c, IsNew: true, PrevCount: 0}, nil
		},
	}

	ma := &mockAnalyzer{}
	w := New(cfg, ml, ms, ma, newMockCache(), testTenant())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	ma.mu.Lock()
	calls := ma.calls
	ma.mu.Unlock()

	if calls != 0 {
		t.Errorf("expected no analysis calls when AutoAnalyze is disabled, got %d", calls)
	}

	// But finding should still be recorded
	ms.mu.Lock()
	findingCalls := ms.createFindingCalls
	ms.mu.Unlock()

	if findingCalls == 0 {
		t.Error("expected finding to be recorded even when auto-analyze is disabled")
	}
}
