package api

import (
	"encoding/json"
	"net/http"
)

type ingestHealthResponse struct {
	ReceivedEventsPerSource map[string]float64 `json:"received_events_per_source"`
	LokiSentEventsTotal     float64            `json:"loki_sent_events_total"`
	// BlankMessagesFilteredTotal is drop_blank_messages' own
	// received-minus-sent (see vector.toml: it's the sole input to
	// sinks.loki, and drops every event whose message is empty) - the
	// precise, named explanation for the gap between total received
	// (summed across every source) and loki_sent_events_total, rather than
	// leaving that gap as two bare unexplained numbers. Not the *only*
	// possible source of a gap (a Vector restart can lose a small amount
	// of in-flight/buffered data too), but it's the one deliberate,
	// by-design filter in the path and accounts for the overwhelming
	// majority of it in practice.
	BlankMessagesFilteredTotal float64 `json:"blank_messages_filtered_total"`
	Degraded                   bool    `json:"degraded"`
}

const (
	metricReceivedEvents = "vector_component_received_events_total"
	metricSentEvents     = "vector_component_sent_events_total"
)

func (s *Server) handleIngestHealth(w http.ResponseWriter, r *http.Request) {
	resp := ingestHealthResponse{ReceivedEventsPerSource: map[string]float64{}}

	if s.deps.Vector == nil {
		resp.Degraded = true
		writeJSON(w, resp)
		return
	}

	samples, err := s.deps.Vector.Metrics(r.Context(), metricReceivedEvents, metricSentEvents)
	if err != nil {
		s.deps.Logger.Error("ingest health: vector metrics failed", "error", err)
		resp.Degraded = true
		writeJSON(w, resp)
		return
	}

	var blankReceived, blankSent float64
	for _, m := range samples {
		id, kind := m.Labels["component_id"], m.Labels["component_kind"]
		switch {
		// Vector's own internal_metrics source (feeding the exporter this
		// reads) is plumbing, not a log feed - leave it out of the per-source
		// counts the Sources page shows.
		case kind == "source" && m.Name == metricReceivedEvents && m.Labels["component_type"] != "internal_metrics":
			resp.ReceivedEventsPerSource[id] += m.Value
		case kind == "sink" && id == "loki" && m.Name == metricSentEvents:
			resp.LokiSentEventsTotal += m.Value
		// sent is reported per output; summing covers transforms with more
		// than the one _default output.
		case kind == "transform" && id == "drop_blank_messages" && m.Name == metricReceivedEvents:
			blankReceived += m.Value
		case kind == "transform" && id == "drop_blank_messages" && m.Name == metricSentEvents:
			blankSent += m.Value
		}
	}
	resp.BlankMessagesFilteredTotal = blankReceived - blankSent

	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
