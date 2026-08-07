package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/sse"
)

type TailQuerier interface {
	QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (loki.QueryResult, error)
}

func RunTailPoller(ctx context.Context, querier TailQuerier, jobLabel string, hub *sse.Hub, interval time.Duration, logger *slog.Logger) {
	watermark := time.Now().UTC()
	logql := loki.BuildQuery(jobLabel, loki.Filters{})

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			end := time.Now().UTC()
			result, err := querier.QueryRange(ctx, logql, watermark, end, 1000)
			if err != nil {
				logger.Error("tail poller: query failed", "error", err)
				continue
			}

			// Advance the watermark to the latest entry timestamp actually
			// observed this poll - never to wall-clock `end`. Loki ingestion
			// has real lag (batching, network, processing), so an event's
			// own timestamp can already be behind `end` by the time it
			// becomes queryable. Advancing to `end` unconditionally would
			// push the watermark past that event's timestamp before it's
			// ever seen, permanently failing `entry.Timestamp.After(watermark)`
			// on every future poll and silently dropping it forever. Leaving
			// the watermark unchanged when nothing new is found (rather than
			// creeping it forward to `end`) gives lagging entries as long as
			// they need to show up in a later poll.
			//
			// The publish check compares against `queryFloor`, the watermark as
			// it stood at the start of this poll - result.Entries isn't
			// guaranteed sorted, so comparing against `watermark` itself while
			// mutating it mid-loop could skip a later, out-of-order entry whose
			// timestamp falls between the original watermark and one an earlier
			// iteration already advanced past.
			queryFloor := watermark
			for _, entry := range result.Entries {
				if !entry.Timestamp.After(queryFloor) {
					continue
				}
				payload, err := json.Marshal(entry)
				if err != nil {
					continue
				}
				hub.Publish("tail", payload)
				if entry.Timestamp.After(watermark) {
					watermark = entry.Timestamp
				}
			}
		}
	}
}
