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

			for _, entry := range result.Entries {
				if !entry.Timestamp.After(watermark) {
					continue
				}
				payload, err := json.Marshal(entry)
				if err != nil {
					continue
				}
				hub.Publish("tail", payload)
			}

			if end.After(watermark) {
				watermark = end
			}
		}
	}
}
