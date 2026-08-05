package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
)

type fastpathEvent struct {
	SrcIP       string  `json:"src_ip"`
	DstIP       string  `json:"dst_ip"`
	DstPort     *int    `json:"dst_port"`
	Action      string  `json:"action"`
	Message     string  `json:"message"`
	ThreatIntel *string `json:"threat_intel"`
}

func (s *Server) handleFastpath(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Fastpath-Token") != s.deps.FastpathToken || s.deps.FastpathToken == "" {
		http.Error(w, "invalid fastpath token", http.StatusUnauthorized)
		return
	}

	var ev fastpathEvent
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	now := time.Now().UTC()

	if ev.ThreatIntel != nil && *ev.ThreatIntel != "" {
		s.raiseFastpathCandidate(ctx, "threat-intel-hit", ev.SrcIP,
			fmt.Sprintf("Threat intel hit: %s", ev.SrcIP),
			fmt.Sprintf("%s tagged %q by threat intel feed", ev.SrcIP, *ev.ThreatIntel),
			ev.Message, now)
	}

	if ev.Action == "drop" && ev.DstPort != nil {
		s.raiseFastpathCandidate(ctx, "wan-drop", ev.SrcIP,
			fmt.Sprintf("Dropped connection from %s", ev.SrcIP),
			fmt.Sprintf("%s -> %s:%d dropped at the gateway", ev.SrcIP, ev.DstIP, *ev.DstPort),
			ev.Message, now)
	}

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) raiseFastpathCandidate(ctx context.Context, ruleName, groupKey, title, body, line string, now time.Time) {
	rule, err := s.deps.Store.GetRuleByName(ctx, ruleName)
	if err != nil {
		s.deps.Logger.Error("fastpath: lookup rule failed", "rule", ruleName, "error", err)
		return
	}
	if rule == nil || !rule.Enabled {
		return
	}

	if err := s.deps.Alerts.Raise(ctx, alerts.Candidate{
		RuleID: rule.ID, GroupKey: groupKey, Severity: rule.Severity, Title: title, Body: body,
		Samples: []alerts.Sample{{TS: now, Line: line}},
	}); err != nil {
		s.deps.Logger.Error("fastpath: raise failed", "rule", ruleName, "error", err)
	}
}
