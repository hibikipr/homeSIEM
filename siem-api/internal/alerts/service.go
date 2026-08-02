package alerts

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/sse"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type Sample struct {
	TS   time.Time
	Line string
}

type Candidate struct {
	RuleID   int64
	GroupKey string
	Severity string
	Title    string
	Body     string
	Samples  []Sample
	Context  map[string]any
}

type AlertStore interface {
	GetRule(ctx context.Context, id int64) (store.Rule, error)
	FindLatestAlert(ctx context.Context, ruleID int64, groupKey string) (*store.Alert, error)
	InsertAlert(ctx context.Context, a store.Alert) (store.Alert, error)
	TouchAlert(ctx context.Context, id int64, at time.Time) error
	ReopenAlert(ctx context.Context, id int64, at time.Time) error
	AddAlertSample(ctx context.Context, alertID int64, ts time.Time, line string) error
}

type Notifier interface {
	Publish(ctx context.Context, title, message, priority string) error
}

type Service struct {
	store    AlertStore
	hub      *sse.Hub
	notifier Notifier
	logger   *slog.Logger
}

func NewService(s AlertStore, hub *sse.Hub, notifier Notifier, logger *slog.Logger) *Service {
	return &Service{store: s, hub: hub, notifier: notifier, logger: logger}
}

func (s *Service) Raise(ctx context.Context, c Candidate) error {
	rule, err := s.store.GetRule(ctx, c.RuleID)
	if err != nil {
		return err
	}

	contextJSON, err := json.Marshal(c.Context)
	if err != nil {
		return err
	}

	existing, err := s.store.FindLatestAlert(ctx, c.RuleID, c.GroupKey)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	var alertID int64
	notify := false

	switch {
	case existing != nil && existing.State == "open" && now.Sub(existing.LastSeenAt) < time.Duration(rule.CooldownSec)*time.Second:
		if err := s.store.TouchAlert(ctx, existing.ID, now); err != nil {
			return err
		}
		alertID = existing.ID

	case existing != nil:
		// Either still open but cooldown lapsed, or previously acked/muted/closed
		// and firing again — either way, reuse the same row (never insert a
		// second row for this rule_id+group_key, which would violate the
		// schema's UNIQUE(rule_id, group_key, state) once it's later acked).
		if err := s.store.ReopenAlert(ctx, existing.ID, now); err != nil {
			return err
		}
		alertID = existing.ID
		notify = true

	default:
		inserted, err := s.store.InsertAlert(ctx, store.Alert{
			RuleID: c.RuleID, GroupKey: c.GroupKey, Severity: c.Severity,
			Title: c.Title, Body: c.Body, EventCount: 1, Context: string(contextJSON),
			State: "open", FirstSeenAt: now, LastSeenAt: now,
		})
		if err != nil {
			return err
		}
		alertID = inserted.ID
		notify = true
	}

	for _, sample := range c.Samples {
		if err := s.store.AddAlertSample(ctx, alertID, sample.TS, sample.Line); err != nil {
			return err
		}
	}

	if !notify {
		return nil
	}

	payload, _ := json.Marshal(struct {
		ID       int64  `json:"id"`
		RuleID   int64  `json:"rule_id"`
		Severity string `json:"severity"`
		Title    string `json:"title"`
		Body     string `json:"body"`
	}{alertID, c.RuleID, c.Severity, c.Title, c.Body})
	s.hub.Publish("alerts", payload)

	if s.notifier != nil {
		if err := s.notifier.Publish(ctx, c.Title, c.Body, severityToPriority(c.Severity)); err != nil {
			s.logger.Error("ntfy publish failed", "error", err, "alert_id", alertID)
		}
	}
	return nil
}

func severityToPriority(severity string) string {
	switch severity {
	case "critical":
		return "urgent"
	case "high":
		return "high"
	case "medium":
		return "default"
	default: // "low", or anything unrecognized
		return "low"
	}
}
