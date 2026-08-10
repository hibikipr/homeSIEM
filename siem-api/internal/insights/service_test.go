package insights

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

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
}

func (f *fakeChatter) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return f.response, f.err
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
	svc := NewService(testPromptBuilder(), chat, ins, time.Hour, testLogger(&bytes.Buffer{}))

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
	svc := NewService(testPromptBuilder(), chat, ins, time.Hour, testLogger(&bytes.Buffer{}))

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
	svc := NewService(testPromptBuilder(), chat, ins, time.Hour, testLogger(&buf))

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
	svc := NewService(testPromptBuilder(), chat, ins, time.Hour, testLogger(&bytes.Buffer{}))

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
	svc := NewService(testPromptBuilder(), chat, ins, time.Hour, testLogger(&bytes.Buffer{}))

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
	svc := NewService(testPromptBuilder(), chat, ins, time.Hour, testLogger(&bytes.Buffer{}))

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
	svc := NewService(pb, chat, ins, time.Hour, testLogger(&bytes.Buffer{}))

	if err := svc.GenerateNow(context.Background()); err == nil {
		t.Fatal("GenerateNow() error = nil, want the prompt-build error propagated")
	}
	if len(ins.inserted) != 0 {
		t.Errorf("len(inserted) = %d, want 0", len(ins.inserted))
	}
}

type fakeAlertListerErr struct{ err error }

func (f *fakeAlertListerErr) ListAlerts(ctx context.Context, state string) ([]store.Alert, error) {
	return nil, f.err
}
