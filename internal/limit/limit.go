package limit

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Gate interface {
	Allow(ctx context.Context, key string, max int) bool
}

type Redis struct {
	client *redis.Client
}

func Open(url string) (*Redis, error) {
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
	return &Redis{client: client}, nil
}

func (r *Redis) Allow(ctx context.Context, key string, max int) bool {
	if r == nil || max <= 0 {
		return true
	}
	n, err := r.client.Incr(ctx, "gloaming:limit:"+key).Result()
	if err != nil {
		return true
	}
	if n == 1 {
		_ = r.client.Expire(ctx, "gloaming:limit:"+key, time.Minute).Err()
	}
	return int(n) <= max
}

type Memory struct {
	mu sync.Mutex
	n  map[string]int
}

func (m *Memory) Allow(_ context.Context, key string, max int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.n == nil {
		m.n = map[string]int{}
	}
	m.n[key]++
	return m.n[key] <= max
}
