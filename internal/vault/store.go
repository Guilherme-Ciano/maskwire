package vault

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type memEntry struct {
	val     []byte
	expires time.Time
}

type Memory struct {
	mu   sync.Mutex
	data map[string]memEntry
	now  func() time.Time
	max  int
}

func NewMemory(maxEntries int) *Memory {
	return &Memory{data: map[string]memEntry{}, now: time.Now, max: maxEntries}
}

func (m *Memory) MGet(_ context.Context, keys []string) ([][]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	out := make([][]byte, len(keys))
	for i, k := range keys {
		e, ok := m.data[k]
		if ok && now.Before(e.expires) {
			out[i] = e.val
		}
	}
	return out, nil
}

func (m *Memory) SetMany(_ context.Context, items map[string][]byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	if len(m.data)+len(items) > m.max {
		for k, e := range m.data {
			if !now.Before(e.expires) {
				delete(m.data, k)
			}
		}
	}
	if len(m.data)+len(items) > m.max {
		return ErrFull
	}
	for k, v := range items {
		m.data[k] = memEntry{val: v, expires: now.Add(ttl)}
	}
	return nil
}

var ErrFull = errors.New("in-memory vault is full")

type Redis struct {
	c *redis.Client
}

func NewRedis(addr, password string) *Redis {
	return &Redis{c: redis.NewClient(&redis.Options{Addr: addr, Password: password, Protocol: 2})}
}

func (r *Redis) MGet(ctx context.Context, keys []string) ([][]byte, error) {
	vals, err := r.c.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("redis mget: %w", err)
	}
	out := make([][]byte, len(vals))
	for i, v := range vals {
		if s, ok := v.(string); ok {
			out[i] = []byte(s)
		}
	}
	return out, nil
}

func (r *Redis) SetMany(ctx context.Context, items map[string][]byte, ttl time.Duration) error {
	pipe := r.c.Pipeline()
	for k, v := range items {
		pipe.Set(ctx, k, v, ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}

func (r *Redis) Ping(ctx context.Context) error {
	if err := r.c.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}

func (r *Redis) Close() error {
	if err := r.c.Close(); err != nil {
		return fmt.Errorf("redis close: %w", err)
	}
	return nil
}
