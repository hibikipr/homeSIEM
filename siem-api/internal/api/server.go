package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
	"github.com/hibikipr/homeSIEM/siem-api/internal/auth"
	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/rules"
	"github.com/hibikipr/homeSIEM/siem-api/internal/sse"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type Deps struct {
	Store           *store.Store
	Loki            *loki.Client
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
}

type Server struct {
	mux  *http.ServeMux
	deps Deps
}

func NewServer(deps Deps) *Server {
	s := &Server{mux: http.NewServeMux(), deps: deps}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return recoverMiddleware(s.deps.Logger)(logMiddleware(s.deps.Logger)(s.mux))
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("POST /ingest/fastpath", s.handleFastpath)
	s.mux.Handle("GET /events/search", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleEventsSearch)))
	s.mux.Handle("GET /events/tail", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleEventsTail)))
	s.mux.Handle("GET /events/stats", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleEventsStats)))
	s.mux.Handle("GET /alerts", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleListAlerts)))
	s.mux.Handle("POST /alerts/{id}/ack", protect(s.deps.Verifier, s.deps.Store, auth.RoleAnalyst, http.HandlerFunc(s.handleAckAlert)))
	s.mux.Handle("GET /alerts/stream", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleAlertsStream)))
	s.mux.Handle("GET /rules", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleListRules)))
	s.mux.Handle("POST /rules", protect(s.deps.Verifier, s.deps.Store, auth.RoleAnalyst, http.HandlerFunc(s.handleCreateRule)))
	s.mux.Handle("PUT /rules/{id}", protect(s.deps.Verifier, s.deps.Store, auth.RoleAnalyst, http.HandlerFunc(s.handleUpdateRule)))
	s.mux.Handle("DELETE /rules/{id}", protect(s.deps.Verifier, s.deps.Store, auth.RoleAnalyst, http.HandlerFunc(s.handleDeleteRule)))
	s.mux.Handle("GET /sources", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleListSources)))
	s.mux.Handle("POST /sources/{id}/claim", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleClaimSource)))
	s.mux.Handle("GET /settings/auth", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleGetAuthSettings)))
	s.mux.Handle("PUT /settings/auth", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleUpdateAuthSettings)))
	s.mux.HandleFunc("POST /auth/session", s.handleAuthSession)
	s.mux.HandleFunc("POST /auth/local", s.handleAuthLocal)
}
