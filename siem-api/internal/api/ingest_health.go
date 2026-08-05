package api

import (
	"encoding/json"
	"net/http"
)

type ingestHealthResponse struct {
	ReceivedEventsPerSource map[string]float64 `json:"received_events_per_source"`
	LokiSentEventsTotal     float64            `json:"loki_sent_events_total"`
	Degraded                bool               `json:"degraded"`
}

const ingestHealthQuery = `{
	sources { nodes { componentId metrics { receivedEventsTotal { receivedEventsTotal } } } }
	sinks { nodes { componentId metrics { sentEventsTotal { sentEventsTotal } } } }
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

	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
