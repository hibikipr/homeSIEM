package api

import (
	"context"
	"sync"
	"time"
)

// Response-cache TTLs for the console endpoints whose Loki queries are
// expensive (2.5-5 s) and whose data is fine to show up to this stale.
const (
	statsCacheTTL   = 30 * time.Second
	searchCacheTTL  = 30 * time.Second
	sourcesCacheTTL = 30 * time.Second
)

// defaultCacheMaxEntries bounds memory: search keys include arbitrary user
// filters, so the key space is unbounded without a cap.
const defaultCacheMaxEntries = 256

// ttlCache is a small in-memory TTL cache with singleflight-style
// de-duplication: concurrent misses for the same key run the loader once and
// share its result. Errors are never cached. Safe for concurrent use.
//
// Values are shared between callers, so they must be treated as read-only
// (the handlers cache immutable []byte JSON bodies or never-mutated maps).
type ttlCache[V any] struct {
	mu         sync.Mutex
	now        func() time.Time
	maxEntries int
	entries    map[string]cacheEntry[V]
	inflight   map[string]*cacheCall[V]
}

type cacheEntry[V any] struct {
	val     V
	expires time.Time
}

type cacheCall[V any] struct {
	done  chan struct{}
	val   V
	err   error
	ready bool // set once the loader returned normally (not panicked)
}

// newTTLCache returns a cache holding at most maxEntries live entries
// (<=0 selects defaultCacheMaxEntries). now may be nil (time.Now); tests
// inject a fake clock.
func newTTLCache[V any](maxEntries int, now func() time.Time) *ttlCache[V] {
	if maxEntries <= 0 {
		maxEntries = defaultCacheMaxEntries
	}
	if now == nil {
		now = time.Now
	}
	return &ttlCache[V]{
		now:        now,
		maxEntries: maxEntries,
		entries:    map[string]cacheEntry[V]{},
		inflight:   map[string]*cacheCall[V]{},
	}
}

// Do returns the cached value for key, or runs load (once across concurrent
// callers) and caches its result for ttl. load returns (value, store, err):
// an error is never cached, and store=false serves the value to the callers
// currently waiting but does not keep it (used for degraded/partial results).
//
// load runs under a context detached from any single caller's cancellation,
// so one client hanging up does not fail the other waiters; the Loki HTTP
// client's own timeout bounds it. A waiter whose own ctx ends stops waiting
// and returns ctx.Err() while the load continues for the others.
func (c *ttlCache[V]) Do(ctx context.Context, key string, ttl time.Duration, load func(ctx context.Context) (V, bool, error)) (V, error) {
	c.mu.Lock()
	if e, ok := c.entries[key]; ok {
		if c.now().Before(e.expires) {
			c.mu.Unlock()
			return e.val, nil
		}
		delete(c.entries, key)
	}
	if call, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		select {
		case <-call.done:
			if !call.ready {
				var zero V
				return zero, errLoaderPanicked
			}
			return call.val, call.err
		case <-ctx.Done():
			var zero V
			return zero, ctx.Err()
		}
	}
	call := &cacheCall[V]{done: make(chan struct{})}
	c.inflight[key] = call
	c.mu.Unlock()

	store := false
	defer func() {
		// Runs on panic too, so waiters are released instead of hanging.
		c.mu.Lock()
		delete(c.inflight, key)
		if call.ready && call.err == nil && store {
			c.putLocked(key, call.val, ttl)
		}
		c.mu.Unlock()
		close(call.done)
	}()
	call.val, store, call.err = load(context.WithoutCancel(ctx))
	call.ready = true
	return call.val, call.err
}

// putLocked stores an entry, sweeping expired ones and then evicting the
// soonest-to-expire entries if still over the cap. Caller holds c.mu.
func (c *ttlCache[V]) putLocked(key string, val V, ttl time.Duration) {
	now := c.now()
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.maxEntries {
		for k, e := range c.entries {
			if !now.Before(e.expires) {
				delete(c.entries, k)
			}
		}
		for len(c.entries) >= c.maxEntries {
			var oldestKey string
			var oldest time.Time
			first := true
			for k, e := range c.entries {
				if first || e.expires.Before(oldest) {
					oldestKey, oldest, first = k, e.expires, false
				}
			}
			delete(c.entries, oldestKey)
		}
	}
	c.entries[key] = cacheEntry[V]{val: val, expires: now.Add(ttl)}
}

// Len reports the number of stored entries (including not-yet-swept expired).
func (c *ttlCache[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

type cacheError string

func (e cacheError) Error() string { return string(e) }

const errLoaderPanicked = cacheError("cache loader panicked")
