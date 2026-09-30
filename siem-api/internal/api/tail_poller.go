package api

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"log/slog"
	"sort"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/sse"
)

type TailQuerier interface {
	QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (loki.QueryResult, error)
}

// tailLookback is how far back each poll re-reads. Loki entries carry the
// sender's own event timestamp, and they become queryable with real lag
// (Vector batching, network, a slower sender's queue) - so an entry can
// show up after a NEWER entry from a faster source has already been
// published. A single advancing high-water mark would skip it forever;
// re-reading a trailing window and de-duplicating against what's already
// been published catches anything that arrives within tailLookback of its
// own timestamp. Entries later than that are still missed by the live
// tail (they're in Loki; Search finds them).
var tailLookback = 30 * time.Second

// tailQueryLimit bounds one poll's read. Loki returns the newest entries
// first when a query hits the limit, so only a burst of more than this many
// lines inside one lookback window can leave a gap.
const tailQueryLimit = 5000

// tailKey identifies one log entry for de-duplication across overlapping
// polls: its timestamp plus a hash of its stream labels and line.
type tailKey struct {
	ts   int64
	hash uint64
}

func tailKeyOf(e loki.LogEntry) tailKey {
	h := fnv.New64a()
	names := make([]string, 0, len(e.Labels))
	for name := range e.Labels {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write([]byte(e.Labels[name]))
		h.Write([]byte{0})
	}
	h.Write([]byte(e.Line))
	return tailKey{ts: e.Timestamp.UnixNano(), hash: h.Sum64()}
}

// RunTailPoller publishes new Loki entries to the hub's "tail" topic, once
// each, in timestamp order. It only queries Loki while someone is
// subscribed - with no open Live tail/Ticker it would otherwise hit Loki
// every interval around the clock for output nobody receives.
func RunTailPoller(ctx context.Context, querier TailQuerier, jobLabel string, hub *sse.Hub, interval time.Duration, logger *slog.Logger) {
	logql := loki.BuildQuery(jobLabel, loki.Filters{})
	seen := map[tailKey]struct{}{}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if hub.SubscriberCount("tail") == 0 {
				// Forget what was published so memory doesn't linger; the
				// next subscriber starts with the last tailLookback of
				// context, which a freshly-opened viewer wants anyway.
				clear(seen)
				continue
			}

			end := time.Now().UTC()
			start := end.Add(-tailLookback)
			result, err := querier.QueryRange(ctx, logql, start, end, tailQueryLimit)
			if err != nil {
				logger.Error("tail poller: query failed", "error", err)
				continue
			}

			entries := result.Entries
			sort.SliceStable(entries, func(i, j int) bool {
				return entries[i].Timestamp.Before(entries[j].Timestamp)
			})
			for _, entry := range entries {
				key := tailKeyOf(entry)
				if _, ok := seen[key]; ok {
					continue
				}
				payload, err := json.Marshal(entry)
				if err != nil {
					continue
				}
				seen[key] = struct{}{}
				hub.Publish("tail", payload)
			}

			// Anything older than this poll's window can't be returned by a
			// later poll either, so it no longer needs de-duplicating.
			startNanos := start.UnixNano()
			for key := range seen {
				if key.ts <= startNanos {
					delete(seen, key)
				}
			}
		}
	}
}
