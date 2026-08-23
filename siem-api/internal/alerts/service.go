package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/ntfy"
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
	TouchAlert(ctx context.Context, id int64, at time.Time, title, body, contextJSON string) error
	ReopenAlert(ctx context.Context, id int64, at time.Time, title, body, contextJSON string) error
	AddAlertSample(ctx context.Context, alertID int64, ts time.Time, line string) error
	GetMinNotifySeverity(ctx context.Context) (string, error)
}

type Notifier interface {
	Publish(ctx context.Context, msg ntfy.Message) error
}

type Service struct {
	store    AlertStore
	hub      *sse.Hub
	notifier Notifier
	appURL   string
	logger   *slog.Logger
}

// appURL is the public URL siem-web is reachable at (e.g.
// "https://siem.example.com"). It's used to build the ntfy notification's
// click-through link, action button, and icon — all optional, so an empty
// appURL still sends notifications, just without those three fields.
func NewService(s AlertStore, hub *sse.Hub, notifier Notifier, appURL string, logger *slog.Logger) *Service {
	return &Service{store: s, hub: hub, notifier: notifier, appURL: strings.TrimRight(appURL, "/"), logger: logger}
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
		if err := s.store.TouchAlert(ctx, existing.ID, now, c.Title, c.Body, string(contextJSON)); err != nil {
			return err
		}
		alertID = existing.ID

	case existing != nil && existing.State == "muted" && existing.MutedUntil != nil && now.Before(*existing.MutedUntil):
		if err := s.store.TouchAlert(ctx, existing.ID, now, c.Title, c.Body, string(contextJSON)); err != nil {
			return err
		}
		alertID = existing.ID

	case existing != nil:
		// Either still open but cooldown lapsed, previously acked/closed and
		// firing again, or muted with an expired MutedUntil — either way,
		// reuse the same row (never insert a second row for this
		// rule_id+group_key, which would violate the schema's
		// UNIQUE(rule_id, group_key, state) once it's later acked).
		if err := s.store.ReopenAlert(ctx, existing.ID, now, c.Title, c.Body, string(contextJSON)); err != nil {
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
		minSeverity, err := s.store.GetMinNotifySeverity(ctx)
		if err != nil {
			// Fail open: a broken threshold read must never silently drop a
			// real notification. severityRank("") is 0 (the lowest tier),
			// so this always passes the threshold below.
			s.logger.Error("get min notify severity failed, notifying anyway", "error", err, "alert_id", alertID)
			minSeverity = ""
		}
		if severityRank(c.Severity) >= severityRank(minSeverity) {
			if err := s.notifier.Publish(ctx, s.buildNotifyMessage(alertID, c, rule)); err != nil {
				s.logger.Error("ntfy publish failed", "error", err, "alert_id", alertID)
			}
		}
	}
	return nil
}

// buildNotifyMessage turns a raised alert into a fully-dressed ntfy
// notification: a severity emoji tag, a numeric priority, a Markdown body
// with the rule/group and (if available) a sample log line in a code
// fence, and — when appURL is configured — a click-through link, a matching
// "view" action button, and the app icon.
func (s *Service) buildNotifyMessage(alertID int64, c Candidate, rule store.Rule) ntfy.Message {
	body := c.Body
	if rule.Name != "" {
		body += fmt.Sprintf("\n\n**Rule:** `%s`  **Group:** `%s`", rule.Name, c.GroupKey)
	} else {
		body += fmt.Sprintf("\n\n**Group:** `%s`", c.GroupKey)
	}
	if len(c.Samples) > 0 {
		sample := c.Samples[len(c.Samples)-1].Line
		const maxSampleLen = 300
		if len(sample) > maxSampleLen {
			sample = sample[:maxSampleLen] + "…"
		}
		body += fmt.Sprintf("\n\n**Sample:**\n```\n%s\n```", sample)
	}

	msg := ntfy.Message{
		Title:    c.Title,
		Body:     body,
		Priority: severityToPriority(c.Severity),
		Tags:     severityToTags(c.Severity),
		Markdown: true,
	}
	if s.appURL != "" {
		click := fmt.Sprintf("%s/alerts?id=%d", s.appURL, alertID)
		msg.Click = click
		msg.Icon = s.appURL + "/icons/homesiem-192.png"
		msg.Actions = []ntfy.Action{{Label: "Open in homeSIEM", URL: click}}
	}
	return msg
}

// severityToPriority maps to ntfy's 1 (min) .. 5 (urgent) priority scale.
func severityToPriority(severity string) int {
	switch severity {
	case "critical":
		return 5 // urgent
	case "warning":
		return 3 // default
	default: // "info", or anything unrecognized
		return 2 // low
	}
}

// severityToTags maps to ntfy emoji short-codes (https://docs.ntfy.sh/emojis/).
func severityToTags(severity string) []string {
	switch severity {
	case "critical":
		return []string{"rotating_light"}
	case "warning":
		return []string{"warning"}
	default: // "info", or anything unrecognized
		return []string{"information_source"}
	}
}

func severityRank(severity string) int {
	switch severity {
	case "critical":
		return 2
	case "warning":
		return 1
	default: // "info", or anything unrecognized
		return 0
	}
}
