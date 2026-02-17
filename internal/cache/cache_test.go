package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kiranshivaraju/loghunter/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupRedis spins up a Redis container and returns a connected RedisCache + cleanup.
func setupRedis(t *testing.T) *cache.RedisCache {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "redis:7-alpine",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(30 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(ctx)) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "6379")
	require.NoError(t, err)

	redisURL := "redis://" + host + ":" + port.Port()
	rc, err := cache.NewRedisCache(redisURL)
	require.NoError(t, err)

	return rc
}

// --- Ping ---

func TestPing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	rc := setupRedis(t)
	err := rc.Ping(context.Background())
	assert.NoError(t, err)
}

// --- Set / Get roundtrip ---

func TestSetGet_Roundtrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	rc := setupRedis(t)
	ctx := context.Background()

	err := rc.Set(ctx, "test:key", []byte("hello"), 10*time.Second)
	require.NoError(t, err)

	val, found, err := rc.Get(ctx, "test:key")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []byte("hello"), val)
}

func TestGet_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	rc := setupRedis(t)

	val, found, err := rc.Get(context.Background(), "nonexistent:key")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, val)
}

func TestSet_TTLExpiry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	rc := setupRedis(t)
	ctx := context.Background()

	err := rc.Set(ctx, "expiry:key", []byte("temp"), 1*time.Second)
	require.NoError(t, err)

	// Immediately should exist
	_, found, err := rc.Get(ctx, "expiry:key")
	require.NoError(t, err)
	assert.True(t, found)

	// Wait for TTL to expire
	time.Sleep(1500 * time.Millisecond)

	_, found, err = rc.Get(ctx, "expiry:key")
	require.NoError(t, err)
	assert.False(t, found)
}

// --- Delete ---

func TestDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	rc := setupRedis(t)
	ctx := context.Background()

	require.NoError(t, rc.Set(ctx, "del:key", []byte("bye"), 10*time.Second))

	err := rc.Delete(ctx, "del:key")
	require.NoError(t, err)

	_, found, err := rc.Get(ctx, "del:key")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestDelete_NonExistent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	rc := setupRedis(t)

	err := rc.Delete(context.Background(), "does:not:exist")
	assert.NoError(t, err)
}

// --- Job Status ---

func TestSetGetJobStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	rc := setupRedis(t)
	ctx := context.Background()
	jobID := uuid.New()

	err := rc.SetJobStatus(ctx, jobID, "running", 10*time.Second)
	require.NoError(t, err)

	status, found, err := rc.GetJobStatus(ctx, jobID)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "running", status)
}

func TestGetJobStatus_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	rc := setupRedis(t)

	status, found, err := rc.GetJobStatus(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, "", status)
}

// --- IncrWithExpiry ---

func TestIncrWithExpiry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	rc := setupRedis(t)
	ctx := context.Background()
	key := "ratelimit:test:" + uuid.NewString()[:8]

	val, err := rc.IncrWithExpiry(ctx, key, 10*time.Second)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val)

	val, err = rc.IncrWithExpiry(ctx, key, 10*time.Second)
	require.NoError(t, err)
	assert.Equal(t, int64(2), val)

	val, err = rc.IncrWithExpiry(ctx, key, 10*time.Second)
	require.NoError(t, err)
	assert.Equal(t, int64(3), val)
}

func TestIncrWithExpiry_Expires(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	rc := setupRedis(t)
	ctx := context.Background()
	key := "ratelimit:expiry:" + uuid.NewString()[:8]

	_, err := rc.IncrWithExpiry(ctx, key, 1*time.Second)
	require.NoError(t, err)

	time.Sleep(1500 * time.Millisecond)

	// After expiry, should start from 1 again
	val, err := rc.IncrWithExpiry(ctx, key, 10*time.Second)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val)
}

// --- Cache Key Builders ---

func TestLokiQueryKey(t *testing.T) {
	tenantID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	key := cache.LokiQueryKey(tenantID, "abc123hash")
	assert.Equal(t, "loki:query:11111111-1111-1111-1111-111111111111:abc123hash", key)
}

func TestJobStatusKey(t *testing.T) {
	jobID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	key := cache.JobStatusKey(jobID)
	assert.Equal(t, "job:22222222-2222-2222-2222-222222222222", key)
}

func TestRateLimitKey(t *testing.T) {
	key := cache.RateLimitKey("lh_abcd1234")
	assert.Equal(t, "ratelimit:lh_abcd1234", key)
}

func TestSearchResultKey(t *testing.T) {
	tenantID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	key := cache.SearchResultKey(tenantID, "filterhash456")
	assert.Equal(t, "loki:search:33333333-3333-3333-3333-333333333333:filterhash456", key)
}

func TestKeyBuilders_NonColliding(t *testing.T) {
	tenantID := uuid.New()
	jobID := uuid.New()

	keys := map[string]bool{
		cache.LokiQueryKey(tenantID, "hash1"):     true,
		cache.JobStatusKey(jobID):                  true,
		cache.RateLimitKey("lh_prefix"):            true,
		cache.SearchResultKey(tenantID, "filter1"): true,
	}
	assert.Len(t, keys, 4, "all keys should be unique")
}
