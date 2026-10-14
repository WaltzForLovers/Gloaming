package catalog

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const catalogCacheTTL = 12 * time.Hour

type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, value []byte)
}

type memoryCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func newMemoryCache() *memoryCache {
	return &memoryCache{m: map[string][]byte{}}
}

func (c *memoryCache) Get(_ context.Context, key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	body, ok := c.m[key]
	return body, ok
}

func (c *memoryCache) Set(_ context.Context, key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = append([]byte(nil), value...)
}

type redisCache struct {
	client *redis.Client
}

func OpenRedis(url string) (Cache, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, err
	}
	return &redisCache{client: client}, nil
}

func (c *redisCache) Get(ctx context.Context, key string) ([]byte, bool) {
	body, err := c.client.Get(ctx, "gloaming:catalog:"+key).Bytes()
	if err != nil {
		return nil, false
	}
	return body, true
}

func (c *redisCache) Set(ctx context.Context, key string, value []byte) {
	_ = c.client.Set(ctx, "gloaming:catalog:"+key, value, catalogCacheTTL).Err()
}

func loadBody(ctx context.Context, client *http.Client, cache Cache, userAgent, endpoint string) ([]byte, int, error) {
	if cache != nil {
		if body, ok := cache.Get(ctx, endpoint); ok && len(body) > 0 {
			return body, http.StatusOK, nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode == http.StatusOK && cache != nil {
		cache.Set(context.WithoutCancel(ctx), endpoint, body)
	}
	return body, resp.StatusCode, nil
}
