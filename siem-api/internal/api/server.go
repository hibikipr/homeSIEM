package api

import (
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
	Store         *store.Store
	Loki          *loki.Client
	JobLabel      string
	Hub           *sse.Hub
	Alerts        *alerts.Service
	Scheduler     *rules.Scheduler
	Verifier      *auth.TokenVerifier
	SessionEst    *auth.SessionEstablisher
	LocalAuth     *auth.LocalAuthenticator
	FastpathToken string
	Logger        *slog.Logger
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
}
