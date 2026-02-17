package cache

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Cache is the caching interface. All cache operations go through here.
// Implementations must be safe for concurrent use.
type Cache interface {
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Delete(ctx context.Context, key string) error
	Ping(ctx context.Context) error
	SetJobStatus(ctx context.Context, jobID uuid.UUID, status string, ttl time.Duration) error
	GetJobStatus(ctx context.Context, jobID uuid.UUID) (string, bool, error)
	IncrWithExpiry(ctx context.Context, key string, expiry time.Duration) (int64, error)
}

// RedisCache implements the Cache interface using go-redis/v9.
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new RedisCache from a Redis URL.
func NewRedisCache(redisURL string) (*RedisCache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return &RedisCache{client: redis.NewClient(opts)}, nil
}

func (c *RedisCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	val, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return val, true, nil
}

func (c *RedisCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

func (c *RedisCache) SetJobStatus(ctx context.Context, jobID uuid.UUID, status string, ttl time.Duration) error {
	return c.client.Set(ctx, JobStatusKey(jobID), status, ttl).Err()
}

func (c *RedisCache) GetJobStatus(ctx context.Context, jobID uuid.UUID) (string, bool, error) {
	val, err := c.client.Get(ctx, JobStatusKey(jobID)).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

func (c *RedisCache) IncrWithExpiry(ctx context.Context, key string, expiry time.Duration) (int64, error) {
	pipe := c.client.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, expiry)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}
