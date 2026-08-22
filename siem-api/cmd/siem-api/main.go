package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
	"github.com/hibikipr/homeSIEM/siem-api/internal/api"
	"github.com/hibikipr/homeSIEM/siem-api/internal/auth"
	"github.com/hibikipr/homeSIEM/siem-api/internal/config"
	"github.com/hibikipr/homeSIEM/siem-api/internal/insights"
	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/ntfy"
	"github.com/hibikipr/homeSIEM/siem-api/internal/ollama"
	"github.com/hibikipr/homeSIEM/siem-api/internal/rules"
	"github.com/hibikipr/homeSIEM/siem-api/internal/sse"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
	"github.com/hibikipr/homeSIEM/siem-api/internal/vector"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}

	db, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		logger.Error("db open failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		logger.Error("db migrate failed", "error", err)
		os.Exit(1)
	}
	st := store.New(db)

	if cfg.LocalAdminUsername != "" && cfg.LocalAdminPasswordHash != "" {
		if _, err := st.EnsureLocalAdmin(context.Background(), cfg.LocalAdminUsername, cfg.LocalAdminPasswordHash); err != nil {
			logger.Error("ensure local admin failed", "error", err)
			os.Exit(1)
		}
	}

	lokiClient := loki.New(cfg.LokiURL, &http.Client{Timeout: 30 * time.Second})
	vectorClient := vector.New(cfg.VectorGraphQLURL, &http.Client{Timeout: 10 * time.Second})
	ntfyClient := ntfy.New(cfg.NtfyURL, cfg.NtfyTopic, cfg.NtfyToken, &http.Client{Timeout: 10 * time.Second})
	hub := sse.NewHub()
	alertsSvc := alerts.NewService(st, hub, ntfyClient, cfg.AppURL, logger)

	evaluators := map[string]rules.Evaluator{
		"threshold":  &rules.ThresholdEvaluator{Querier: lokiClient},
		"first_seen": &rules.FirstSeenEvaluator{Querier: lokiClient, Seen: st},
		"absence":    &rules.AbsenceEvaluator{Sources: st},
		"insight":    &rules.InsightEvaluator{Store: st},
	}
	scheduler := rules.NewScheduler(st, evaluators, alertsSvc, logger)

	// insightsSvc stays nil when OLLAMA_URL is unset - Deps.Insights being
	// nil is exactly what handleGenerateInsights checks to 400 rather than
	// attempt a Chat() call against an empty base URL. Matches ntfy's own
	// degrade-gracefully-when-unconfigured posture.
	var insightsSvc *insights.Service
	if cfg.OllamaURL != "" {
		ollamaClient := ollama.New(cfg.OllamaURL, cfg.OllamaModel, &http.Client{Timeout: time.Duration(cfg.OllamaTimeoutSec) * time.Second})
		promptBuilder := &insights.PromptBuilder{Loki: lokiClient, Alerts: st, JobLabel: cfg.LokiJobLabel}
		insightsSvc = insights.NewService(promptBuilder, ollamaClient, st, st,
			time.Duration(cfg.InsightsLookbackMin)*time.Minute, logger)
	}

	verifier := auth.NewTokenVerifier(cfg.SessionSecret)
	sessionEst := auth.NewSessionEstablisher(st, st)
	localAuth := auth.NewLocalAuthenticator(st)

	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := scheduler.Start(appCtx); err != nil {
		logger.Error("scheduler start failed", "error", err)
		os.Exit(1)
	}
	defer scheduler.Stop()

	go api.RunTailPoller(appCtx, lokiClient, cfg.LokiJobLabel, hub, time.Second, logger)

	if insightsSvc != nil {
		insightsScheduler := insights.NewScheduler(insightsSvc, cfg.InsightsIntervalSec, logger)
		go insightsScheduler.Start(appCtx)
	}

	server := api.NewServer(api.Deps{
		Store: st, Loki: lokiClient, Vector: vectorClient, JobLabel: cfg.LokiJobLabel, Hub: hub,
		Alerts: alertsSvc, Scheduler: scheduler, SchedulerCtx: appCtx,
		Verifier: verifier, SessionEst: sessionEst, LocalAuth: localAuth,
		FastpathToken: cfg.FastpathToken,
		OIDCIssuer:    cfg.OIDCIssuer, OIDCClientID: cfg.OIDCClientID, OIDCGroupsScope: cfg.OIDCGroupsScope,
		NtfyURL: cfg.NtfyURL, NtfyTopic: cfg.NtfyTopic, Ntfy: ntfyClient,
		Insights:  insightsSvc,
		OllamaURL: cfg.OllamaURL, OllamaModel: cfg.OllamaModel, OllamaTimeoutSec: cfg.OllamaTimeoutSec,
		InsightsIntervalSec: cfg.InsightsIntervalSec, InsightsLookbackMin: cfg.InsightsLookbackMin,
		Logger: logger,
	})

	httpServer := &http.Server{Addr: cfg.Addr, Handler: server.Handler()}

	go func() {
		logger.Info("siem-api listening", "addr", cfg.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown error", "error", err)
	}
}
