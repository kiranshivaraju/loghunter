package store_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kiranshivaraju/loghunter/internal/store"
	"github.com/kiranshivaraju/loghunter/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// migrationsDir returns the absolute path to the migrations directory.
func migrationsDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "migrations")
}

// setupTestDB spins up a Postgres container, runs migrations, and returns a pool + cleanup.
func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("loghunter_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second)),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, pgContainer.Terminate(ctx))
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Run migrations
	err = store.RunMigrations(connStr, migrationsDir())
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	return pool
}

// defaultTenantID returns the UUID of the seeded default tenant.
func defaultTenantID(t *testing.T, s store.Store) uuid.UUID {
	t.Helper()
	tenant, err := s.GetDefaultTenant(context.Background())
	require.NoError(t, err)
	return tenant.ID
}

// --- Tenant Tests ---

func TestGetDefaultTenant(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)

	tenant, err := s.GetDefaultTenant(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "default", tenant.Name)
	assert.Equal(t, "default", tenant.LokiOrgID)
	assert.NotEqual(t, uuid.Nil, tenant.ID)
}

// --- API Key Tests ---

func TestAPIKey_CreateAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)

	now := time.Now().UTC().Truncate(time.Microsecond)
	key := &models.APIKey{
		ID:        uuid.New(),
		TenantID:  tenantID,
		Name:      "test-key",
		KeyHash:   "bcrypt-hash-here",
		KeyPrefix: "lh_abcd",
		Scopes:    []string{"ingest", "read"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	err := s.CreateAPIKey(ctx, key)
	require.NoError(t, err)

	// Get by prefix
	keys, err := s.GetAPIKeyByPrefix(ctx, "lh_abcd")
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Equal(t, key.ID, keys[0].ID)
	assert.Equal(t, "test-key", keys[0].Name)
}

func TestAPIKey_List(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	for i := 0; i < 3; i++ {
		err := s.CreateAPIKey(ctx, &models.APIKey{
			ID:        uuid.New(),
			TenantID:  tenantID,
			Name:      "key-" + uuid.NewString()[:4],
			KeyHash:   "hash-" + uuid.NewString()[:4],
			KeyPrefix: "lh_" + uuid.NewString()[:4],
			Scopes:    []string{"read"},
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.NoError(t, err)
	}

	keys, err := s.ListAPIKeys(ctx, tenantID)
	require.NoError(t, err)
	assert.Len(t, keys, 3)
}

func TestAPIKey_Revoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	key := &models.APIKey{
		ID:        uuid.New(),
		TenantID:  tenantID,
		Name:      "revoke-me",
		KeyHash:   "hash",
		KeyPrefix: "lh_revk",
		Scopes:    []string{"read"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, s.CreateAPIKey(ctx, key))

	// Revoke
	err := s.RevokeAPIKey(ctx, key.ID, tenantID)
	require.NoError(t, err)

	// Should not appear in list or prefix lookup
	keys, err := s.ListAPIKeys(ctx, tenantID)
	require.NoError(t, err)
	assert.Empty(t, keys)

	keys, err = s.GetAPIKeyByPrefix(ctx, "lh_revk")
	require.NoError(t, err)
	assert.Empty(t, keys)
}

func TestAPIKey_RevokeNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)

	err := s.RevokeAPIKey(context.Background(), uuid.New(), uuid.New())
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestAPIKey_UpdateLastUsed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	key := &models.APIKey{
		ID:        uuid.New(),
		TenantID:  tenantID,
		Name:      "usage-key",
		KeyHash:   "hash",
		KeyPrefix: "lh_used",
		Scopes:    []string{"read"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, s.CreateAPIKey(ctx, key))

	err := s.UpdateAPIKeyLastUsed(ctx, key.ID)
	require.NoError(t, err)

	keys, err := s.GetAPIKeyByPrefix(ctx, "lh_used")
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.NotNil(t, keys[0].LastUsedAt)
}

func TestAPIKey_DuplicateID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	id := uuid.New()
	key := &models.APIKey{
		ID: id, TenantID: tenantID, Name: "dup1", KeyHash: "h1", KeyPrefix: "lh_dup1",
		Scopes: []string{"read"}, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, s.CreateAPIKey(ctx, key))

	key2 := &models.APIKey{
		ID: id, TenantID: tenantID, Name: "dup2", KeyHash: "h2", KeyPrefix: "lh_dup2",
		Scopes: []string{"read"}, CreatedAt: now, UpdatedAt: now,
	}
	err := s.CreateAPIKey(ctx, key2)
	assert.ErrorIs(t, err, store.ErrDuplicateKey)
}

// --- Error Cluster Tests ---

func TestErrorCluster_UpsertInsert(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	cluster := &models.ErrorCluster{
		ID: uuid.New(), TenantID: tenantID, Service: "api-server",
		Namespace: "default", Fingerprint: "fp-abc123", Level: "ERROR",
		FirstSeenAt: now, LastSeenAt: now, Count: 5,
		SampleMessage: "NullPointerException at line 42",
		CreatedAt: now, UpdatedAt: now,
	}

	result, err := s.UpsertErrorCluster(ctx, cluster)
	require.NoError(t, err)
	assert.Equal(t, cluster.ID, result.ID)
	assert.Equal(t, 5, result.Count)
}

func TestErrorCluster_UpsertMerge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	cluster := &models.ErrorCluster{
		ID: uuid.New(), TenantID: tenantID, Service: "api-server",
		Namespace: "default", Fingerprint: "fp-merge", Level: "ERROR",
		FirstSeenAt: now, LastSeenAt: now, Count: 3,
		SampleMessage: "first error", CreatedAt: now, UpdatedAt: now,
	}
	_, err := s.UpsertErrorCluster(ctx, cluster)
	require.NoError(t, err)

	// Upsert again with same fingerprint — count should add
	later := now.Add(5 * time.Minute)
	cluster2 := &models.ErrorCluster{
		ID: uuid.New(), TenantID: tenantID, Service: "api-server",
		Namespace: "default", Fingerprint: "fp-merge", Level: "ERROR",
		FirstSeenAt: later, LastSeenAt: later, Count: 7,
		SampleMessage: "second error", CreatedAt: later, UpdatedAt: later,
	}
	result, err := s.UpsertErrorCluster(ctx, cluster2)
	require.NoError(t, err)
	assert.Equal(t, cluster.ID, result.ID) // original ID preserved
	assert.Equal(t, 10, result.Count)      // 3 + 7
	assert.Equal(t, later, result.LastSeenAt.UTC().Truncate(time.Microsecond))
}

func TestErrorCluster_GetByID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	cluster := &models.ErrorCluster{
		ID: uuid.New(), TenantID: tenantID, Service: "svc1", Namespace: "ns1",
		Fingerprint: "fp-get", Level: "WARN", FirstSeenAt: now, LastSeenAt: now,
		Count: 1, SampleMessage: "warn msg", CreatedAt: now, UpdatedAt: now,
	}
	_, err := s.UpsertErrorCluster(ctx, cluster)
	require.NoError(t, err)

	got, err := s.GetErrorCluster(ctx, cluster.ID, tenantID)
	require.NoError(t, err)
	assert.Equal(t, cluster.ID, got.ID)
	assert.Equal(t, "svc1", got.Service)
}

func TestErrorCluster_GetNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)

	_, err := s.GetErrorCluster(context.Background(), uuid.New(), uuid.New())
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestErrorCluster_List(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	for i := 0; i < 5; i++ {
		_, err := s.UpsertErrorCluster(ctx, &models.ErrorCluster{
			ID: uuid.New(), TenantID: tenantID, Service: "svc",
			Namespace: "default", Fingerprint: uuid.NewString()[:8], Level: "ERROR",
			FirstSeenAt: now, LastSeenAt: now, Count: 1,
			SampleMessage: "err", CreatedAt: now, UpdatedAt: now,
		})
		require.NoError(t, err)
	}

	clusters, total, err := s.ListErrorClusters(ctx, store.ClusterFilter{
		TenantID: tenantID, Service: "svc", Page: 1, Limit: 3,
	})
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, clusters, 3)
}

func TestErrorCluster_ListWithFilters(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Create clusters with different levels
	for _, level := range []string{"ERROR", "WARN"} {
		_, err := s.UpsertErrorCluster(ctx, &models.ErrorCluster{
			ID: uuid.New(), TenantID: tenantID, Service: "filtered-svc",
			Namespace: "prod", Fingerprint: "fp-" + level, Level: level,
			FirstSeenAt: now, LastSeenAt: now, Count: 1,
			SampleMessage: level + " msg", CreatedAt: now, UpdatedAt: now,
		})
		require.NoError(t, err)
	}

	clusters, total, err := s.ListErrorClusters(ctx, store.ClusterFilter{
		TenantID: tenantID, Level: "ERROR", Page: 1, Limit: 20,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, clusters, 1)
	assert.Equal(t, "ERROR", clusters[0].Level)
}

func TestErrorCluster_GetByFingerprints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	fps := []string{"fp-a", "fp-b", "fp-c"}
	for _, fp := range fps {
		_, err := s.UpsertErrorCluster(ctx, &models.ErrorCluster{
			ID: uuid.New(), TenantID: tenantID, Service: "svc",
			Namespace: "default", Fingerprint: fp, Level: "ERROR",
			FirstSeenAt: now, LastSeenAt: now, Count: 1,
			SampleMessage: "msg", CreatedAt: now, UpdatedAt: now,
		})
		require.NoError(t, err)
	}

	clusters, err := s.GetClustersByFingerprints(ctx, tenantID, []string{"fp-a", "fp-c"})
	require.NoError(t, err)
	assert.Len(t, clusters, 2)
}

func TestErrorCluster_GetByFingerprintsEmpty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)

	clusters, err := s.GetClustersByFingerprints(context.Background(), uuid.New(), []string{})
	require.NoError(t, err)
	assert.Empty(t, clusters)
}

// --- Analysis Result Tests ---

func TestAnalysisResult_CreateAndGetByJob(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Create cluster and job first
	clusterID := uuid.New()
	_, err := s.UpsertErrorCluster(ctx, &models.ErrorCluster{
		ID: clusterID, TenantID: tenantID, Service: "svc", Namespace: "default",
		Fingerprint: "fp-analysis", Level: "ERROR", FirstSeenAt: now, LastSeenAt: now,
		Count: 1, SampleMessage: "error", CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)

	jobID := uuid.New()
	require.NoError(t, s.CreateJob(ctx, &models.Job{
		ID: jobID, TenantID: tenantID, Type: "analysis", Status: "pending",
		ClusterID: &clusterID, CreatedAt: now, UpdatedAt: now,
	}))

	action := "restart the pod"
	result := &models.AnalysisResult{
		ID: uuid.New(), ClusterID: clusterID, TenantID: tenantID, JobID: jobID,
		Provider: "ollama", Model: "llama3", RootCause: "OOM",
		Confidence: 0.85, Summary: "Out of memory error",
		SuggestedAction: &action, CreatedAt: now,
	}
	err = s.CreateAnalysisResult(ctx, result)
	require.NoError(t, err)

	got, err := s.GetAnalysisResultByJobID(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, result.ID, got.ID)
	assert.Equal(t, "OOM", got.RootCause)
	assert.InDelta(t, 0.85, got.Confidence, 0.001)
}

func TestAnalysisResult_GetByCluster(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	clusterID := uuid.New()
	_, err := s.UpsertErrorCluster(ctx, &models.ErrorCluster{
		ID: clusterID, TenantID: tenantID, Service: "svc", Namespace: "default",
		Fingerprint: "fp-cluster-analysis", Level: "ERROR", FirstSeenAt: now, LastSeenAt: now,
		Count: 1, SampleMessage: "error", CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)

	jobID := uuid.New()
	require.NoError(t, s.CreateJob(ctx, &models.Job{
		ID: jobID, TenantID: tenantID, Type: "analysis", Status: "pending",
		ClusterID: &clusterID, CreatedAt: now, UpdatedAt: now,
	}))

	require.NoError(t, s.CreateAnalysisResult(ctx, &models.AnalysisResult{
		ID: uuid.New(), ClusterID: clusterID, TenantID: tenantID, JobID: jobID,
		Provider: "ollama", Model: "llama3", RootCause: "disk full",
		Confidence: 0.9, Summary: "Disk is full", CreatedAt: now,
	}))

	got, err := s.GetAnalysisResultByClusterID(ctx, clusterID)
	require.NoError(t, err)
	assert.Equal(t, "disk full", got.RootCause)
}

func TestAnalysisResult_GetByJobNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)

	_, err := s.GetAnalysisResultByJobID(context.Background(), uuid.New())
	assert.ErrorIs(t, err, store.ErrNotFound)
}

// --- Job Tests ---

func TestJob_CreateAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	job := &models.Job{
		ID: uuid.New(), TenantID: tenantID, Type: "analysis",
		Status: "pending", CreatedAt: now, UpdatedAt: now,
	}
	err := s.CreateJob(ctx, job)
	require.NoError(t, err)

	got, err := s.GetJob(ctx, job.ID, tenantID)
	require.NoError(t, err)
	assert.Equal(t, "pending", got.Status)
	assert.Nil(t, got.StartedAt)
}

func TestJob_GetNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)

	_, err := s.GetJob(context.Background(), uuid.New(), uuid.New())
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestJob_UpdateStatusPendingToRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	job := &models.Job{
		ID: uuid.New(), TenantID: tenantID, Type: "analysis",
		Status: "pending", CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, s.CreateJob(ctx, job))

	err := s.UpdateJobStatus(ctx, job.ID, "running")
	require.NoError(t, err)

	got, err := s.GetJob(ctx, job.ID, tenantID)
	require.NoError(t, err)
	assert.Equal(t, "running", got.Status)
	assert.NotNil(t, got.StartedAt)
}

func TestJob_UpdateStatusRunningToCompleted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	job := &models.Job{
		ID: uuid.New(), TenantID: tenantID, Type: "analysis",
		Status: "pending", CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, s.CreateJob(ctx, job))
	require.NoError(t, s.UpdateJobStatus(ctx, job.ID, "running"))

	err := s.UpdateJobStatus(ctx, job.ID, "completed")
	require.NoError(t, err)

	got, err := s.GetJob(ctx, job.ID, tenantID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
	assert.NotNil(t, got.CompletedAt)
}

func TestJob_UpdateStatusRunningToFailed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	job := &models.Job{
		ID: uuid.New(), TenantID: tenantID, Type: "analysis",
		Status: "pending", CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, s.CreateJob(ctx, job))
	require.NoError(t, s.UpdateJobStatus(ctx, job.ID, "running"))

	err := s.UpdateJobStatus(ctx, job.ID, "failed", store.WithErrorMessage("timeout"))
	require.NoError(t, err)

	got, err := s.GetJob(ctx, job.ID, tenantID)
	require.NoError(t, err)
	assert.Equal(t, "failed", got.Status)
	assert.NotNil(t, got.CompletedAt)
	require.NotNil(t, got.ErrorMessage)
	assert.Equal(t, "timeout", *got.ErrorMessage)
}

func TestJob_UpdateStatusInvalidTransition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	job := &models.Job{
		ID: uuid.New(), TenantID: tenantID, Type: "analysis",
		Status: "pending", CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, s.CreateJob(ctx, job))

	err := s.UpdateJobStatus(ctx, job.ID, "completed") // pending -> completed is invalid
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid job status transition")
}

func TestJob_UpdateStatusNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)

	err := s.UpdateJobStatus(context.Background(), uuid.New(), "running")
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestJob_UpdateStatusWithClusterID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)
	ctx := context.Background()
	tenantID := defaultTenantID(t, s)
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Create cluster
	clusterID := uuid.New()
	_, err := s.UpsertErrorCluster(ctx, &models.ErrorCluster{
		ID: clusterID, TenantID: tenantID, Service: "svc", Namespace: "default",
		Fingerprint: "fp-jobcluster", Level: "ERROR", FirstSeenAt: now, LastSeenAt: now,
		Count: 1, SampleMessage: "err", CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)

	job := &models.Job{
		ID: uuid.New(), TenantID: tenantID, Type: "analysis",
		Status: "pending", CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, s.CreateJob(ctx, job))
	require.NoError(t, s.UpdateJobStatus(ctx, job.ID, "running"))

	err = s.UpdateJobStatus(ctx, job.ID, "completed", store.WithClusterID(clusterID))
	require.NoError(t, err)

	got, err := s.GetJob(ctx, job.ID, tenantID)
	require.NoError(t, err)
	require.NotNil(t, got.ClusterID)
	assert.Equal(t, clusterID, *got.ClusterID)
}

// --- Ping Test ---

func TestPing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	pool := setupTestDB(t)
	s := store.NewPostgresStore(pool)

	err := s.Ping(context.Background())
	assert.NoError(t, err)
}
