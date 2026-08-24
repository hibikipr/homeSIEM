package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
	"github.com/hibikipr/homeSIEM/siem-api/internal/auth"
	"github.com/hibikipr/homeSIEM/siem-api/internal/insights"
	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/ntfy"
	"github.com/hibikipr/homeSIEM/siem-api/internal/rules"
	"github.com/hibikipr/homeSIEM/siem-api/internal/sse"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
	"github.com/hibikipr/homeSIEM/siem-api/internal/vector"
)

type Deps struct {
	Store           *store.Store
	Loki            *loki.Client
	Vector          *vector.Client
	JobLabel        string
	Hub             *sse.Hub
	Alerts          *alerts.Service
	Scheduler       *rules.Scheduler
	SchedulerCtx    context.Context
	Verifier        *auth.TokenVerifier
	SessionEst      *auth.SessionEstablisher
	LocalAuth       *auth.LocalAuthenticator
	FastpathToken   string
	Logger          *slog.Logger
	OIDCIssuer      string
	OIDCClientID    string
	OIDCGroupsScope string
	NtfyURL         string
	NtfyTopic       string
	Ntfy            *ntfy.Client
	// Insights is nil when OLLAMA_URL is unset - handleGenerateInsights
	// checks for that and 400s, matching handleTestNotification's existing
	// not-configured shape. GET/dismiss still work with Insights nil, since
	// those only touch Store.
	Insights *insights.Service
	// OllamaURL/OllamaModel/OllamaTimeoutSec/InsightsIntervalSec/
	// InsightsLookbackMin are read-only in Settings → Ollama (handleGetOllamaSettings) -
	// deployment topology, set via env vars only, same non-editable-here
	// posture as NtfyURL/NtfyTopic above.
	OllamaURL           string
	OllamaModel         string
	OllamaTimeoutSec    int
	InsightsIntervalSec int
	InsightsLookbackMin int
}

// MaxConcurrentLokiQueries bounds how many Loki requests this server fires
// at once across all in-flight requests, not just within a single
// queryHourlyBySource call - see stats.go's handleEventsStats, which can
// have three queryHourlyBySource calls in flight at the same time, each
// wanting to fire 25 of its own. Firing all of those unbounded risks
// overwhelming a modest homelab Loki instance (trading "slow" for "flaky")
// in exchange for marginal extra speedup once concurrency is already this
// high.
//
// Exported so cmd/siem-api can size the Loki HTTP client's connection pool
// (MaxIdleConnsPerHost) to match - otherwise the default of 2 idle
// connections per host means most of these concurrent requests tear down
// and re-establish a fresh connection (and DNS lookup) on every Wall page
// load instead of reusing a warm one from the last.
const MaxConcurrentLokiQueries = 12

type Server struct {
	mux     *http.ServeMux
	deps    Deps
	lokiSem chan struct{}
}

func NewServer(deps Deps) *Server {
	s := &Server{mux: http.NewServeMux(), deps: deps, lokiSem: make(chan struct{}, MaxConcurrentLokiQueries)}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return recoverMiddleware(s.deps.Logger)(logMiddleware(s.deps.Logger)(s.mux))
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("POST /ingest/fastpath", s.handleFastpath)
	s.mux.HandleFunc("POST /sources/heartbeat", s.handleSourceHeartbeat)
	s.mux.Handle("GET /events/search", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleEventsSearch)))
	s.mux.Handle("GET /events/tail", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleEventsTail)))
	s.mux.Handle("GET /events/stats", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleEventsStats)))
	s.mux.Handle("GET /nav/summary", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleNavSummary)))
	s.mux.Handle("GET /alerts", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleListAlerts)))
	s.mux.Handle("POST /alerts/{id}/ack", protect(s.deps.Verifier, s.deps.Store, auth.RoleAnalyst, http.HandlerFunc(s.handleAckAlert)))
	s.mux.Handle("POST /alerts/{id}/mute", protect(s.deps.Verifier, s.deps.Store, auth.RoleAnalyst, http.HandlerFunc(s.handleMuteAlert)))
	s.mux.Handle("GET /alerts/{id}/samples", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleListAlertSamples)))
	s.mux.Handle("GET /alerts/stream", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleAlertsStream)))
	s.mux.Handle("GET /rules", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleListRules)))
	s.mux.Handle("POST /rules", protect(s.deps.Verifier, s.deps.Store, auth.RoleAnalyst, http.HandlerFunc(s.handleCreateRule)))
	s.mux.Handle("PUT /rules/{id}", protect(s.deps.Verifier, s.deps.Store, auth.RoleAnalyst, http.HandlerFunc(s.handleUpdateRule)))
	s.mux.Handle("DELETE /rules/{id}", protect(s.deps.Verifier, s.deps.Store, auth.RoleAnalyst, http.HandlerFunc(s.handleDeleteRule)))
	s.mux.Handle("GET /sources", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleListSources)))
	s.mux.Handle("GET /sources/ingest-health", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleIngestHealth)))
	s.mux.Handle("POST /sources/{id}/claim", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleClaimSource)))
	s.mux.Handle("PUT /sources/{id}/rename", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleRenameSource)))
	s.mux.Handle("PUT /sources/{id}/heartbeat", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleSetHeartbeat)))
	s.mux.Handle("DELETE /sources/{id}", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleDeleteSource)))
	s.mux.Handle("GET /settings/auth", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleGetAuthSettings)))
	s.mux.Handle("PUT /settings/auth", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleUpdateAuthSettings)))
	s.mux.Handle("GET /settings/notifications", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleGetNotificationSettings)))
	s.mux.Handle("PUT /settings/notifications", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleUpdateNotificationSettings)))
	s.mux.Handle("POST /settings/notifications/test", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleTestNotification)))
	s.mux.Handle("GET /settings/ollama", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleGetOllamaSettings)))
	s.mux.Handle("PUT /settings/ollama", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleUpdateOllamaSettings)))
	s.mux.Handle("GET /insights", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleListInsights)))
	s.mux.Handle("POST /insights/generate", protect(s.deps.Verifier, s.deps.Store, auth.RoleAnalyst, http.HandlerFunc(s.handleGenerateInsights)))
	s.mux.Handle("PUT /insights/{id}/dismiss", protect(s.deps.Verifier, s.deps.Store, auth.RoleAnalyst, http.HandlerFunc(s.handleDismissInsight)))
	s.mux.Handle("PUT /insights/{id}/mute", protect(s.deps.Verifier, s.deps.Store, auth.RoleAnalyst, http.HandlerFunc(s.handleMuteInsight)))
	s.mux.Handle("GET /insights/muted", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleListMutedInsights)))
	s.mux.Handle("DELETE /insights/muted/{fingerprint}", protect(s.deps.Verifier, s.deps.Store, auth.RoleAnalyst, http.HandlerFunc(s.handleUnmuteInsight)))
	s.mux.HandleFunc("POST /auth/session", s.handleAuthSession)
	s.mux.HandleFunc("POST /auth/local", s.handleAuthLocal)
}
