package insights

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/ollama"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func testLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, nil))
}

func testPromptBuilder() *PromptBuilder {
	return &PromptBuilder{
		Loki:     &fakeLokiQuerier{},
		Alerts:   &fakeAlertLister{},
		JobLabel: "siem",
	}
}

type fakeChatter struct {
	response string
	err      error

	// gotSystemPrompt/gotOpts capture the last call's arguments, so tests can
	// assert the effective (override-or-default) prompt and the options
	// actually reached Chat, not just that GenerateNow returned no error.
	gotSystemPrompt string
	gotOpts         ollama.ChatOptions
}

func (f *fakeChatter) Chat(ctx context.Context, systemPrompt, userPrompt string, opts ollama.ChatOptions) (string, error) {
	f.gotSystemPrompt = systemPrompt
	f.gotOpts = opts
	return f.response, f.err
}

type fakeSettingsStore struct {
	settings store.OllamaSettings
	err      error
}

func (f *fakeSettingsStore) GetOllamaSettings(ctx context.Context) (store.OllamaSettings, error) {
	return f.settings, f.err
}

func defaultTestSettings() *fakeSettingsStore {
	return &fakeSettingsStore{settings: store.OllamaSettings{
		Temperature: 0.2, TopP: 0.9, NumPredict: 1024, NumCtx: 8192,
	}}
}

type fakeInsightStore struct {
	inserted  []store.Insight
	insertErr error
}

func (f *fakeInsightStore) InsertInsight(ctx context.Context, in store.Insight) (store.Insight, error) {
	if f.insertErr != nil {
		return store.Insight{}, f.insertErr
	}
	f.inserted = append(f.inserted, in)
	return in, nil
}

const validResponse = `[
  {
    "title": "Bambuddy errors look mistagged",
    "detail": "Several ERROR lines are actually routine.",
    "severity": "warning",
    "category": "severity-misclassification",
    "evidence": [{"program": "Bambuddy", "sample_message": "ERROR ...", "count": 12}]
  }
]`

func TestGenerateNow_HappyPath_InsertsOneInsightPerElement(t *testing.T) {
	chat := &fakeChatter{response: validResponse}
	ins := &fakeInsightStore{}
	svc := NewService(testPromptBuilder(), chat, ins, defaultTestSettings(), time.Hour, testLogger(&bytes.Buffer{}))

	if err := svc.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() error = %v", err)
	}

	if len(ins.inserted) != 1 {
		t.Fatalf("len(inserted) = %d, want 1", len(ins.inserted))
	}
	got := ins.inserted[0]
	if got.Title != "Bambuddy errors look mistagged" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Severity != "warning" {
		t.Errorf("Severity = %q, want warning", got.Severity)
	}
	if got.Category != "severity-misclassification" {
		t.Errorf("Category = %q", got.Category)
	}
	if got.EvidenceJSON == "" {
		t.Error("EvidenceJSON is empty, want the encoded evidence array")
	}
}

func TestGenerateNow_ResponseWrappedInProseAndCodeFence_StillParses(t *testing.T) {
	wrapped := "Here is my analysis:\n```json\n" + validResponse + "\n```\nLet me know if you need more."
	chat := &fakeChatter{response: wrapped}
	ins := &fakeInsightStore{}
	svc := NewService(testPromptBuilder(), chat, ins, defaultTestSettings(), time.Hour, testLogger(&bytes.Buffer{}))

	if err := svc.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() error = %v", err)
	}
	if len(ins.inserted) != 1 {
		t.Fatalf("len(inserted) = %d, want 1 (extracted from surrounding prose/fence)", len(ins.inserted))
	}
}

func TestGenerateNow_MalformedResponse_InsertsNothingLogsWarningNoError(t *testing.T) {
	var buf bytes.Buffer
	chat := &fakeChatter{response: "not json at all"}
	ins := &fakeInsightStore{}
	svc := NewService(testPromptBuilder(), chat, ins, defaultTestSettings(), time.Hour, testLogger(&buf))

	err := svc.GenerateNow(context.Background())
	if err != nil {
		t.Fatalf("GenerateNow() error = %v, want nil (a malformed response must not crash the scheduler loop)", err)
	}
	if len(ins.inserted) != 0 {
		t.Errorf("len(inserted) = %d, want 0", len(ins.inserted))
	}
	if !bytes.Contains(buf.Bytes(), []byte("WARN")) {
		t.Error("expected a warning to be logged for the malformed response")
	}
}

func TestGenerateNow_EmptyArrayResponse_InsertsNothingNoError(t *testing.T) {
	chat := &fakeChatter{response: "[]"}
	ins := &fakeInsightStore{}
	svc := NewService(testPromptBuilder(), chat, ins, defaultTestSettings(), time.Hour, testLogger(&bytes.Buffer{}))

	if err := svc.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() error = %v", err)
	}
	if len(ins.inserted) != 0 {
		t.Errorf("len(inserted) = %d, want 0 for a genuinely empty (nothing actionable) response", len(ins.inserted))
	}
}

func TestGenerateNow_InvalidSeverity_CoercedToInfo(t *testing.T) {
	response := `[{"title":"t","detail":"d","severity":"bogus","category":"other","evidence":[]}]`
	chat := &fakeChatter{response: response}
	ins := &fakeInsightStore{}
	svc := NewService(testPromptBuilder(), chat, ins, defaultTestSettings(), time.Hour, testLogger(&bytes.Buffer{}))

	if err := svc.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() error = %v", err)
	}
	if len(ins.inserted) != 1 {
		t.Fatalf("len(inserted) = %d, want 1", len(ins.inserted))
	}
	if ins.inserted[0].Severity != "info" {
		t.Errorf("Severity = %q, want info (unrecognized falls to the lowest tier)", ins.inserted[0].Severity)
	}
}

func TestGenerateNow_ChatError_PropagatesNoInserts(t *testing.T) {
	chat := &fakeChatter{err: errors.New("ollama unreachable")}
	ins := &fakeInsightStore{}
	svc := NewService(testPromptBuilder(), chat, ins, defaultTestSettings(), time.Hour, testLogger(&bytes.Buffer{}))

	if err := svc.GenerateNow(context.Background()); err == nil {
		t.Fatal("GenerateNow() error = nil, want the Chat error propagated")
	}
	if len(ins.inserted) != 0 {
		t.Errorf("len(inserted) = %d, want 0", len(ins.inserted))
	}
}

func TestGenerateNow_PromptBuildError_PropagatesNoInserts(t *testing.T) {
	failingAlerts := &fakeAlertListerErr{err: errors.New("db unavailable")}
	pb := &PromptBuilder{Loki: &fakeLokiQuerier{}, Alerts: failingAlerts, JobLabel: "siem"}
	chat := &fakeChatter{response: validResponse}
	ins := &fakeInsightStore{}
	svc := NewService(pb, chat, ins, defaultTestSettings(), time.Hour, testLogger(&bytes.Buffer{}))

	if err := svc.GenerateNow(context.Background()); err == nil {
		t.Fatal("GenerateNow() error = nil, want the prompt-build error propagated")
	}
	if len(ins.inserted) != 0 {
		t.Errorf("len(inserted) = %d, want 0", len(ins.inserted))
	}
}

func TestGenerateNow_NoPromptOverride_UsesDefaultSystemPrompt(t *testing.T) {
	chat := &fakeChatter{response: "[]"}
	ins := &fakeInsightStore{}
	svc := NewService(testPromptBuilder(), chat, ins, defaultTestSettings(), time.Hour, testLogger(&bytes.Buffer{}))

	if err := svc.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() error = %v", err)
	}
	if chat.gotSystemPrompt != DefaultSystemPrompt {
		t.Error("Chat was not called with DefaultSystemPrompt when no override is stored")
	}
}

func TestGenerateNow_PromptOverrideSet_UsesOverride(t *testing.T) {
	chat := &fakeChatter{response: "[]"}
	ins := &fakeInsightStore{}
	settings := &fakeSettingsStore{settings: store.OllamaSettings{
		SystemPrompt: "custom system prompt", Temperature: 0.2, TopP: 0.9, NumPredict: 1024, NumCtx: 8192,
	}}
	svc := NewService(testPromptBuilder(), chat, ins, settings, time.Hour, testLogger(&bytes.Buffer{}))

	if err := svc.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() error = %v", err)
	}
	if chat.gotSystemPrompt != "custom system prompt" {
		t.Errorf("gotSystemPrompt = %q, want the stored override", chat.gotSystemPrompt)
	}
}

func TestGenerateNow_PassesGenerationOptionsThrough(t *testing.T) {
	chat := &fakeChatter{response: "[]"}
	ins := &fakeInsightStore{}
	settings := &fakeSettingsStore{settings: store.OllamaSettings{
		Temperature: 0.7, TopP: 0.5, NumPredict: 2048, NumCtx: 16384,
	}}
	svc := NewService(testPromptBuilder(), chat, ins, settings, time.Hour, testLogger(&bytes.Buffer{}))

	if err := svc.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() error = %v", err)
	}
	want := ollama.ChatOptions{Temperature: 0.7, TopP: 0.5, NumPredict: 2048, NumCtx: 16384}
	if chat.gotOpts != want {
		t.Errorf("gotOpts = %+v, want %+v", chat.gotOpts, want)
	}
}

func TestGenerateNow_SettingsError_PropagatesNoInserts(t *testing.T) {
	chat := &fakeChatter{response: validResponse}
	ins := &fakeInsightStore{}
	settings := &fakeSettingsStore{err: errors.New("db unavailable")}
	svc := NewService(testPromptBuilder(), chat, ins, settings, time.Hour, testLogger(&bytes.Buffer{}))

	if err := svc.GenerateNow(context.Background()); err == nil {
		t.Fatal("GenerateNow() error = nil, want the settings-fetch error propagated")
	}
	if len(ins.inserted) != 0 {
		t.Errorf("len(inserted) = %d, want 0", len(ins.inserted))
	}
}

type fakeAlertListerErr struct{ err error }

func (f *fakeAlertListerErr) ListAlerts(ctx context.Context, state string) ([]store.Alert, error) {
	return nil, f.err
}
