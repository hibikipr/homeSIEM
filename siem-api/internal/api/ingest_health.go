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

const ingestHealthQuery = `{
	sources { nodes { componentId metrics { receivedEventsTotal { receivedEventsTotal } } } }
	sinks { nodes { componentId metrics { sentEventsTotal { sentEventsTotal } } } }
	transforms { nodes { componentId metrics { receivedEventsTotal { receivedEventsTotal } sentEventsTotal { sentEventsTotal } } } }
}`

type ingestHealthGraphQLData struct {
	Sources struct {
		Nodes []struct {
			ComponentID string `json:"componentId"`
			Metrics     struct {
				ReceivedEventsTotal *struct {
					ReceivedEventsTotal float64 `json:"receivedEventsTotal"`
				} `json:"receivedEventsTotal"`
			} `json:"metrics"`
		} `json:"nodes"`
	} `json:"sources"`
	Sinks struct {
		Nodes []struct {
			ComponentID string `json:"componentId"`
			Metrics     struct {
				SentEventsTotal *struct {
					SentEventsTotal float64 `json:"sentEventsTotal"`
				} `json:"sentEventsTotal"`
			} `json:"metrics"`
		} `json:"nodes"`
	} `json:"sinks"`
	Transforms struct {
		Nodes []struct {
			ComponentID string `json:"componentId"`
			Metrics     struct {
				ReceivedEventsTotal *struct {
					ReceivedEventsTotal float64 `json:"receivedEventsTotal"`
				} `json:"receivedEventsTotal"`
				SentEventsTotal *struct {
					SentEventsTotal float64 `json:"sentEventsTotal"`
				} `json:"sentEventsTotal"`
			} `json:"metrics"`
		} `json:"nodes"`
	} `json:"transforms"`
}

func (s *Server) handleIngestHealth(w http.ResponseWriter, r *http.Request) {
	resp := ingestHealthResponse{ReceivedEventsPerSource: map[string]float64{}}

	if s.deps.Vector == nil {
		resp.Degraded = true
		writeJSON(w, resp)
		return
	}

	raw, err := s.deps.Vector.Query(r.Context(), ingestHealthQuery)
	if err != nil {
		s.deps.Logger.Error("ingest health: vector query failed", "error", err)
		resp.Degraded = true
		writeJSON(w, resp)
		return
	}

	var data ingestHealthGraphQLData
	if err := json.Unmarshal(raw, &data); err != nil {
		s.deps.Logger.Error("ingest health: decode vector response failed", "error", err)
		resp.Degraded = true
		writeJSON(w, resp)
		return
	}

	for _, node := range data.Sources.Nodes {
		if node.Metrics.ReceivedEventsTotal != nil {
			resp.ReceivedEventsPerSource[node.ComponentID] = node.Metrics.ReceivedEventsTotal.ReceivedEventsTotal
		}
	}
	for _, node := range data.Sinks.Nodes {
		if node.ComponentID == "loki" && node.Metrics.SentEventsTotal != nil {
			resp.LokiSentEventsTotal = node.Metrics.SentEventsTotal.SentEventsTotal
		}
	}
	for _, node := range data.Transforms.Nodes {
		if node.ComponentID != "drop_blank_messages" {
			continue
		}
		if node.Metrics.ReceivedEventsTotal != nil && node.Metrics.SentEventsTotal != nil {
			resp.BlankMessagesFilteredTotal = node.Metrics.ReceivedEventsTotal.ReceivedEventsTotal - node.Metrics.SentEventsTotal.SentEventsTotal
		}
	}

	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
