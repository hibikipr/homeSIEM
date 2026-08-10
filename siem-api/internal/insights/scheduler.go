package insights

import (
	"context"
	"log/slog"
	"time"
)

// Scheduler runs Service.GenerateNow on a fixed interval. One job, not the
// per-rule concurrency machinery rules.Scheduler needs - a plain ticker
// loop is enough here.
type Scheduler struct {
	svc         *Service
	intervalSec int
	logger      *slog.Logger
}

func NewScheduler(svc *Service, intervalSec int, logger *slog.Logger) *Scheduler {
	return &Scheduler{svc: svc, intervalSec: intervalSec, logger: logger}
}

// Start blocks until ctx is done, running GenerateNow every intervalSec
// seconds. A failed pass is logged and never stops future ticks - one bad
// pass (e.g. Ollama temporarily unreachable) must not take the whole
// scheduler down.
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(s.intervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.svc.GenerateNow(ctx); err != nil {
				s.logger.Warn("insights: scheduled pass failed", "error", err)
			}
		}
	}
}
