package api

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Unix(1_700_000_000, 0)} }

func TestTTLCache_HitExpiryAndErrors(t *testing.T) {
	ctx := context.Background()
	errBoom := errors.New("boom")

	tests := []struct {
		name string
		run  func(t *testing.T, c *ttlCache[int], clk *fakeClock)
	}{
		{"second call within ttl is a hit", func(t *testing.T, c *ttlCache[int], clk *fakeClock) {
			var calls int
			load := func(context.Context) (int, bool, error) { calls++; return calls, true, nil }
			a, _ := c.Do(ctx, "k", time.Minute, load)
			clk.advance(59 * time.Second)
			b, _ := c.Do(ctx, "k", time.Minute, load)
			if a != 1 || b != 1 || calls != 1 {
				t.Fatalf("a=%d b=%d calls=%d, want 1 1 1", a, b, calls)
			}
		}},
		{"expired entry reloads", func(t *testing.T, c *ttlCache[int], clk *fakeClock) {
			var calls int
			load := func(context.Context) (int, bool, error) { calls++; return calls, true, nil }
			c.Do(ctx, "k", time.Minute, load)
			clk.advance(time.Minute)
			v, _ := c.Do(ctx, "k", time.Minute, load)
			if v != 2 || calls != 2 {
				t.Fatalf("v=%d calls=%d, want 2 2", v, calls)
			}
		}},
		{"different keys are independent", func(t *testing.T, c *ttlCache[int], clk *fakeClock) {
			var calls int
			load := func(context.Context) (int, bool, error) { calls++; return calls, true, nil }
			c.Do(ctx, "a", time.Minute, load)
			c.Do(ctx, "b", time.Minute, load)
			if calls != 2 {
				t.Fatalf("calls=%d, want 2", calls)
			}
		}},
		{"error is not cached", func(t *testing.T, c *ttlCache[int], clk *fakeClock) {
			var calls int
			_, err := c.Do(ctx, "k", time.Minute, func(context.Context) (int, bool, error) { calls++; return 0, true, errBoom })
			if !errors.Is(err, errBoom) {
				t.Fatalf("err = %v, want boom", err)
			}
			v, err := c.Do(ctx, "k", time.Minute, func(context.Context) (int, bool, error) { calls++; return 7, true, nil })
			if err != nil || v != 7 || calls != 2 {
				t.Fatalf("v=%d err=%v calls=%d, want 7 nil 2", v, err, calls)
			}
		}},
		{"store=false is not kept", func(t *testing.T, c *ttlCache[int], clk *fakeClock) {
			var calls int
			load := func(context.Context) (int, bool, error) { calls++; return calls, false, nil }
			c.Do(ctx, "k", time.Minute, load)
			c.Do(ctx, "k", time.Minute, load)
			if calls != 2 {
				t.Fatalf("calls=%d, want 2", calls)
			}
		}},
		{"max entries bounds memory, expired swept first", func(t *testing.T, c *ttlCache[int], clk *fakeClock) {
			load := func(context.Context) (int, bool, error) { return 1, true, nil }
			c.Do(ctx, "old", time.Second, load)
			c.Do(ctx, "b", time.Minute, load)
			c.Do(ctx, "c", time.Minute, load)
			clk.advance(2 * time.Second) // "old" expired
			c.Do(ctx, "d", time.Minute, load)
			if n := c.Len(); n != 3 {
				t.Fatalf("Len = %d, want 3 (cap)", n)
			}
			c.Do(ctx, "e", time.Minute, load) // none expired: evicts soonest-expiring
			if n := c.Len(); n != 3 {
				t.Fatalf("Len = %d after overflow, want 3", n)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clk := newFakeClock()
			tc.run(t, newTTLCache[int](3, clk.now), clk)
		})
	}
}

func TestTTLCache_ConcurrentMissRunsLoaderOnce(t *testing.T) {
	c := newTTLCache[int](0, nil)
	var calls atomic.Int32
	release := make(chan struct{})
	load := func(context.Context) (int, bool, error) {
		calls.Add(1)
		<-release
		return 42, true, nil
	}

	const n = 20
	var wg sync.WaitGroup
	results := make([]int, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = c.Do(context.Background(), "k", time.Minute, load)
		}()
	}
	// Wait until the leader is inside the loader, then let everyone pile up.
	for calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("loader ran %d times, want 1", got)
	}
	for i, v := range results {
		if v != 42 {
			t.Errorf("results[%d] = %d, want 42", i, v)
		}
	}
}

func TestTTLCache_WaiterCancelDoesNotFailLoader(t *testing.T) {
	c := newTTLCache[int](0, nil)
	release := make(chan struct{})
	started := make(chan struct{})
	go c.Do(context.Background(), "k", time.Minute, func(ctx context.Context) (int, bool, error) {
		close(started)
		<-release
		return 1, true, nil
	})
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Do(ctx, "k", time.Minute, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	close(release)
}

func TestTTLCache_LoaderPanicReleasesWaiters(t *testing.T) {
	c := newTTLCache[int](0, nil)
	func() {
		defer func() { _ = recover() }()
		c.Do(context.Background(), "k", time.Minute, func(context.Context) (int, bool, error) { panic("x") })
	}()
	v, err := c.Do(context.Background(), "k", time.Minute, func(context.Context) (int, bool, error) { return 5, true, nil })
	if err != nil || v != 5 {
		t.Fatalf("v=%d err=%v, want 5 nil", v, err)
	}
}
