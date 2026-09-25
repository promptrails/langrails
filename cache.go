package langrails

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sync"
	"time"
)

// Cache stores completion responses by request key. Implementations must be
// safe for concurrent use. Errors are treated as cache misses by
// CacheProvider, never as request failures, so a flaky remote cache cannot
// take completions down with it.
type Cache interface {
	// Get returns the response stored under key, and whether there was one.
	Get(ctx context.Context, key string) (*CompletionResponse, bool, error)

	// Set stores resp under key.
	Set(ctx context.Context, key string, resp *CompletionResponse) error
}

// CacheKey returns the key CacheProvider uses for req: a SHA-256 of the
// request's JSON form. Every field takes part, so any change to model,
// messages, tools or parameters is a different key.
func CacheKey(req *CompletionRequest) (string, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// CacheProvider wraps a Provider and answers repeated identical Complete
// requests from a Cache. Stream is passed through uncached.
type CacheProvider struct {
	inner Provider
	cache Cache
}

// WithCache wraps a provider with a response cache.
//
// Example:
//
//	cache := langrails.NewMemoryCache(1000, time.Hour)
//	provider := langrails.WithCache(openai.New("sk-..."), cache)
//
// Only successful responses are stored. A cached response is returned as a
// copy, so callers may modify it freely. Note that sampling makes most
// requests non-deterministic; a cache returns the first answer for every
// identical request, which is usually what you want for tests, evals and
// idempotent pipelines, and not what you want for chat.
func WithCache(provider Provider, cache Cache) *CacheProvider {
	return &CacheProvider{inner: provider, cache: cache}
}

// Complete returns a cached response for an identical earlier request, or
// calls the provider and caches its response.
func (c *CacheProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	key, keyErr := CacheKey(req)
	if keyErr == nil {
		if resp, ok, err := c.cache.Get(ctx, key); err == nil && ok && resp != nil {
			return cloneResponse(resp), nil
		}
	}

	resp, err := c.inner.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	if keyErr == nil {
		_ = c.cache.Set(ctx, key, cloneResponse(resp))
	}
	return resp, nil
}

// Stream is not cached; it calls the wrapped provider directly.
func (c *CacheProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error) {
	return c.inner.Stream(ctx, req)
}

func cloneResponse(r *CompletionResponse) *CompletionResponse {
	cp := *r
	cp.ToolCalls = slices.Clone(r.ToolCalls)
	cp.Citations = slices.Clone(r.Citations)
	return &cp
}

// MemoryCache is an in-process Cache with least-recently-used eviction and
// an optional time-to-live.
type MemoryCache struct {
	mu         sync.Mutex
	maxEntries int
	ttl        time.Duration
	ll         *list.List
	items      map[string]*list.Element
	now        func() time.Time
}

type memoryCacheEntry struct {
	key     string
	resp    *CompletionResponse
	expires time.Time
}

// NewMemoryCache creates a cache holding at most maxEntries responses
// (0 = unbounded), each kept for ttl (0 = no expiry).
func NewMemoryCache(maxEntries int, ttl time.Duration) *MemoryCache {
	return &MemoryCache{
		maxEntries: maxEntries,
		ttl:        ttl,
		ll:         list.New(),
		items:      map[string]*list.Element{},
		now:        time.Now,
	}
}

// Get implements Cache.
func (m *MemoryCache) Get(_ context.Context, key string) (*CompletionResponse, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	el, ok := m.items[key]
	if !ok {
		return nil, false, nil
	}
	e := el.Value.(*memoryCacheEntry)
	if !e.expires.IsZero() && m.now().After(e.expires) {
		m.ll.Remove(el)
		delete(m.items, key)
		return nil, false, nil
	}
	m.ll.MoveToFront(el)
	return e.resp, true, nil
}

// Set implements Cache.
func (m *MemoryCache) Set(_ context.Context, key string, resp *CompletionResponse) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var expires time.Time
	if m.ttl > 0 {
		expires = m.now().Add(m.ttl)
	}
	if el, ok := m.items[key]; ok {
		e := el.Value.(*memoryCacheEntry)
		e.resp, e.expires = resp, expires
		m.ll.MoveToFront(el)
		return nil
	}
	m.items[key] = m.ll.PushFront(&memoryCacheEntry{key: key, resp: resp, expires: expires})
	if m.maxEntries > 0 && m.ll.Len() > m.maxEntries {
		oldest := m.ll.Back()
		m.ll.Remove(oldest)
		delete(m.items, oldest.Value.(*memoryCacheEntry).key)
	}
	return nil
}

// Len returns the number of cached entries, including expired ones not yet
// evicted.
func (m *MemoryCache) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ll.Len()
}
