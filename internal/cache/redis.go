package cache

import (
	"context"
	"fmt"
	"harvest/internal/config"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// Cache defines the interface for a caching implementation
type Cache interface {
	// Get retrieves a value from the cache
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value in the cache with an optional TTL
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes a key from the cache
	Delete(ctx context.Context, key string) error

	// Ping tests the connection to the cache
	Ping(ctx context.Context) error

	// Close releases resources used by the cache
	Close() error
}

// ErrCacheMiss is returned when a key is not found in the cache
var ErrCacheMiss = fmt.Errorf("cache miss")

// RedisCache implements the Cache interface using Redis
type RedisCache struct {
	client *redis.Client
	prefix string
}

// NewRedisCache creates a new Redis cache instance
func NewRedisCache(config config.RedisConfig) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     config.Address,
		Password: config.Password,
		DB:       config.DB,
	})

	// Verify the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Error().Err(err).Msg("Failed to connect to Redis")
		return nil, err
	}

	log.Info().
		Str("address", config.Address).
		Str("prefix", config.Prefix).
		Int("db", config.DB).
		Msg("Redis cache initialized successfully")

	return &RedisCache{
		client: client,
		prefix: config.Prefix,
	}, nil
}

// formatKey adds the prefix to the key
func (c *RedisCache) formatKey(key string) string {
	return c.prefix + ":" + key
}

// Get retrieves a value from the cache
func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	formattedKey := c.formatKey(key)

	start := time.Now()
	result, err := c.client.Get(ctx, formattedKey).Bytes()
	duration := time.Since(start)

	if err == redis.Nil {
		log.Debug().
			Str("key", formattedKey).
			Dur("duration", duration).
			Msg("Cache miss")
		return nil, ErrCacheMiss
	} else if err != nil {
		log.Error().
			Err(err).
			Str("key", formattedKey).
			Dur("duration", duration).
			Msg("Error getting value from Redis")
		return nil, err
	}

	log.Debug().
		Str("key", formattedKey).
		Int("size", len(result)).
		Dur("duration", duration).
		Msg("Cache hit")

	return result, nil
}

// Set stores a value in the cache with an optional TTL
func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	formattedKey := c.formatKey(key)

	start := time.Now()
	err := c.client.Set(ctx, formattedKey, value, ttl).Err()
	duration := time.Since(start)

	if err != nil {
		log.Error().
			Err(err).
			Str("key", formattedKey).
			Int("size", len(value)).
			Dur("ttl", ttl).
			Dur("duration", duration).
			Msg("Error setting value in Redis")
		return err
	}

	log.Debug().
		Str("key", formattedKey).
		Int("size", len(value)).
		Dur("ttl", ttl).
		Dur("duration", duration).
		Msg("Successfully cached value")

	return nil
}

// Delete removes a key from the cache
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	formattedKey := c.formatKey(key)

	start := time.Now()
	err := c.client.Del(ctx, formattedKey).Err()
	duration := time.Since(start)

	if err != nil {
		log.Error().
			Err(err).
			Str("key", formattedKey).
			Dur("duration", duration).
			Msg("Error deleting key from Redis")
		return err
	}

	log.Debug().
		Str("key", formattedKey).
		Dur("duration", duration).
		Msg("Successfully deleted key from cache")

	return nil
}

// Ping tests the connection to the cache
func (c *RedisCache) Ping(ctx context.Context) error {
	start := time.Now()
	err := c.client.Ping(ctx).Err()
	duration := time.Since(start)

	if err != nil {
		log.Error().
			Err(err).
			Dur("duration", duration).
			Msg("Error pinging Redis")
		return err
	}

	log.Debug().
		Dur("duration", duration).
		Msg("Successfully pinged Redis")

	return nil
}

// Close releases resources used by the cache
func (c *RedisCache) Close() error {
	log.Info().Msg("Closing Redis cache connection")
	return c.client.Close()
}
