package watcher

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/analysis"
	"github.com/kiranshivaraju/loghunter/internal/cache"
	"github.com/kiranshivaraju/loghunter/internal/config"
	"github.com/kiranshivaraju/loghunter/internal/loki"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"github.com/kiranshivaraju/loghunter/pkg/logql"
	"github.com/kiranshivaraju/loghunter/pkg/models"
)

const analysisGuardTTL = 6 * time.Hour

// WatcherStore is the store subset required by the Watcher.
type WatcherStore interface {
	UpsertErrorClusterFull(ctx context.Context, cluster *models.ErrorCluster) (store.UpsertResult, error)
	CreateWatcherFinding(ctx context.Context, finding *models.WatcherFinding) error
	ListWatcherFindings(ctx context.Context, tenantID uuid.UUID, limit int) ([]*models.WatcherFinding, error)
}

// Analyzer triggers AI analysis on an error cluster.
type Analyzer interface {
	TriggerAnalysis(ctx context.Context, cluster *models.ErrorCluster) (*models.Job, error)
}

// LokiDiscoverer is the Loki client subset required by the Watcher.
type LokiDiscoverer interface {
	LabelValues(ctx context.Context, label string) ([]string, error)
	QueryRange(ctx context.Context, req loki.QueryRangeRequest) ([]models.LogLine, error)
}

// AnalysisGuardCache is the cache subset for the analysis dedup guard.
type AnalysisGuardCache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
}

// Notifier is called after a finding is detected.
// Implementations must be safe for concurrent use and must not block.
type Notifier interface {
	Notify(ctx context.Context, finding Finding) error
}

// NoopNotifier is the default notifier that does nothing.
type NoopNotifier struct{}

func (NoopNotifier) Notify(_ context.Context, _ Finding) error { return nil }

// Finding is the payload passed to notifiers.
type Finding struct {
	Cluster   *models.ErrorCluster
	Kind      string
	PrevCount int
	JobID     *uuid.UUID
	DetectedAt time.Time
}

// WatcherStatus is returned by the Status method for the API.
type WatcherStatus struct {
	Enabled         bool               `json:"enabled"`
	Running         bool               `json:"running"`
	LastPollAt      *time.Time         `json:"last_poll_at,omitempty"`
	NextPollAt      *time.Time         `json:"next_poll_at,omitempty"`
	ServicesWatched []string           `json:"services_watched"`
	RecentFindings  []*models.WatcherFinding `json:"recent_findings"`
}

// Option configures a Watcher.
type Option func(*Watcher)

// WithNotifier sets a custom notifier.
func WithNotifier(n Notifier) Option {
	return func(w *Watcher) { w.notifier = n }
}

// Watcher continuously monitors Loki for error/warning logs and auto-triggers analysis.
type Watcher struct {
	cfg      config.WatcherConfig
	loki     LokiDiscoverer
	store    WatcherStore
	analyzer Analyzer
	cache    AnalysisGuardCache
	notifier Notifier
	tenant   *models.Tenant

	mu              sync.RWMutex
	running         bool
	lastPollAt      *time.Time
	nextPollAt      *time.Time
	servicesWatched []string
}

// New creates a Watcher with the given dependencies.
func New(cfg config.WatcherConfig, loki LokiDiscoverer, st WatcherStore, analyzer Analyzer, ca AnalysisGuardCache, tenant *models.Tenant, opts ...Option) *Watcher {
	w := &Watcher{
		cfg:      cfg,
		loki:     loki,
		store:    st,
		analyzer: analyzer,
		cache:    ca,
		notifier: NoopNotifier{},
		tenant:   tenant,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Run starts the watcher loop. It blocks until ctx is cancelled.
// If the watcher is disabled via config, it returns immediately.
func (w *Watcher) Run(ctx context.Context) {
	if !w.cfg.Enabled {
		slog.Info("watcher disabled, skipping")
		return
	}

	w.mu.Lock()
	w.running = true
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
	}()

	slog.Info("watcher started", "interval", w.cfg.Interval)

	// Run immediately on start
	w.poll(ctx)

	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()

	for {
		next := time.Now().Add(w.cfg.Interval)
		w.mu.Lock()
		w.nextPollAt = &next
		w.mu.Unlock()

		select {
		case <-ctx.Done():
			slog.Info("watcher shutting down")
			return
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

// Status returns the current watcher state for the API.
func (w *Watcher) Status(ctx context.Context) (WatcherStatus, error) {
	w.mu.RLock()
	status := WatcherStatus{
		Enabled:         w.cfg.Enabled,
		Running:         w.running,
		LastPollAt:      w.lastPollAt,
		NextPollAt:      w.nextPollAt,
		ServicesWatched: w.servicesWatched,
	}
	w.mu.RUnlock()

	if status.ServicesWatched == nil {
		status.ServicesWatched = []string{}
	}

	findings, err := w.store.ListWatcherFindings(ctx, w.tenant.ID, 20)
	if err != nil {
		return status, err
	}
	status.RecentFindings = findings
	if status.RecentFindings == nil {
		status.RecentFindings = []*models.WatcherFinding{}
	}

	return status, nil
}

func (w *Watcher) poll(ctx context.Context) {
	now := time.Now().UTC()
	w.mu.Lock()
	w.lastPollAt = &now
	w.mu.Unlock()

	services, err := w.resolveServices(ctx)
	if err != nil {
		slog.Error("watcher: service discovery failed", "error", err)
		return
	}
	w.mu.Lock()
	w.servicesWatched = services
	w.mu.Unlock()

	end := now
	start := now.Add(-w.cfg.LookbackWindow)

	for _, svc := range services {
		if ctx.Err() != nil {
			return
		}
		w.pollService(ctx, svc, start, end)
	}
}

func (w *Watcher) resolveServices(ctx context.Context) ([]string, error) {
	if len(w.cfg.Services) > 0 {
		return w.cfg.Services, nil
	}

	values, err := w.loki.LabelValues(ctx, "service")
	if err != nil {
		return nil, err
	}

	if len(values) > w.cfg.MaxServices {
		slog.Warn("watcher: discovered more services than MaxServices cap, truncating",
			"discovered", len(values), "cap", w.cfg.MaxServices)
		values = values[:w.cfg.MaxServices]
	}

	return values, nil
}

func (w *Watcher) pollService(ctx context.Context, service string, start, end time.Time) {
	qb := logql.QueryBuilder{}
	query := qb.BuildDetectionQuery(logql.DetectionParams{
		Service:   service,
		Namespace: w.cfg.Namespace,
	})

	lines, err := w.loki.QueryRange(ctx, loki.QueryRangeRequest{
		Query: query,
		Start: start,
		End:   end,
		Limit: w.cfg.LogsLimit,
	})
	if err != nil {
		slog.Warn("watcher: loki query failed", "service", service, "error", err)
		return
	}
	if len(lines) == 0 {
		return
	}

	clusters := analysis.Cluster(lines, service, w.cfg.Namespace)

	for i := range clusters {
		clusters[i].TenantID = w.tenant.ID
		w.handleCluster(ctx, &clusters[i])
	}
}

func (w *Watcher) handleCluster(ctx context.Context, c *models.ErrorCluster) {
	result, err := w.store.UpsertErrorClusterFull(ctx, c)
	if err != nil {
		slog.Error("watcher: upsert failed", "fingerprint", c.Fingerprint, "error", err)
		return
	}

	kind := w.classifyFinding(result)
	if kind == "" {
		return
	}

	var jobID *uuid.UUID
	triggered := false
	if w.cfg.AutoAnalyze && !w.isAnalysisGuarded(ctx, result.Cluster.ID) {
		job, err := w.analyzer.TriggerAnalysis(ctx, result.Cluster)
		if err != nil {
			slog.Warn("watcher: auto-analysis trigger failed", "cluster_id", result.Cluster.ID, "error", err)
		} else {
			triggered = true
			jobID = &job.ID
			w.setAnalysisGuard(ctx, result.Cluster.ID)
		}
	}

	slog.Info("watcher: finding detected",
		"kind", kind,
		"service", c.Service,
		"cluster_id", result.Cluster.ID,
		"count", result.Cluster.Count,
		"prev_count", result.PrevCount,
		"analysis_triggered", triggered,
	)

	w.recordFinding(ctx, result.Cluster, kind, result.PrevCount, triggered, jobID)

	_ = w.notifier.Notify(ctx, Finding{
		Cluster:    result.Cluster,
		Kind:       kind,
		PrevCount:  result.PrevCount,
		JobID:      jobID,
		DetectedAt: time.Now().UTC(),
	})
}

func (w *Watcher) classifyFinding(r store.UpsertResult) string {
	if r.IsNew {
		return "new"
	}
	if r.PrevCount > 0 {
		ratio := float64(r.Cluster.Count) / float64(r.PrevCount)
		if ratio >= w.cfg.SpikeThreshold {
			return "spike"
		}
	}
	return ""
}

func (w *Watcher) isAnalysisGuarded(ctx context.Context, clusterID uuid.UUID) bool {
	_, found, _ := w.cache.Get(ctx, cache.WatcherAnalysisGuardKey(w.tenant.ID, clusterID))
	return found
}

func (w *Watcher) setAnalysisGuard(ctx context.Context, clusterID uuid.UUID) {
	_ = w.cache.Set(ctx, cache.WatcherAnalysisGuardKey(w.tenant.ID, clusterID), []byte("1"), analysisGuardTTL)
}

func (w *Watcher) recordFinding(ctx context.Context, cluster *models.ErrorCluster, kind string, prevCount int, triggered bool, jobID *uuid.UUID) {
	finding := &models.WatcherFinding{
		ID:                uuid.New(),
		TenantID:          w.tenant.ID,
		ClusterID:         cluster.ID,
		Service:           cluster.Service,
		Namespace:         cluster.Namespace,
		Kind:              kind,
		CurrentCount:      cluster.Count,
		PrevCount:         prevCount,
		AnalysisTriggered: triggered,
		JobID:             jobID,
		DetectedAt:        time.Now().UTC(),
	}
	if err := w.store.CreateWatcherFinding(ctx, finding); err != nil {
		slog.Error("watcher: failed to record finding", "error", err)
	}
}
