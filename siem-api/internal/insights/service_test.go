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

	// bumped records the ids BumpInsight was called with, in call order -
	// tests assert against this to prove a recurrence updated an existing
	// row instead of InsertInsight creating a duplicate.
	bumped  []int64
	bumpErr error

	findErr error

	muted    map[string]bool
	mutedErr error
}

func (f *fakeInsightStore) InsertInsight(ctx context.Context, in store.Insight) (store.Insight, error) {
	if f.insertErr != nil {
		return store.Insight{}, f.insertErr
	}
	in.ID = int64(len(f.inserted) + 1)
	in.OccurrenceCount = 1
	f.inserted = append(f.inserted, in)
	return in, nil
}

// FindMostRecentInsightByFingerprint searches f.inserted for the newest
// (last-appended) entry with a matching fingerprint - close enough to the
// real store's "ORDER BY created_at DESC LIMIT 1" for these tests, since
// each test only ever has one matching lineage.
func (f *fakeInsightStore) FindMostRecentInsightByFingerprint(ctx context.Context, fingerprint string) (store.Insight, bool, error) {
	if f.findErr != nil {
		return store.Insight{}, false, f.findErr
	}
	for i := len(f.inserted) - 1; i >= 0; i-- {
		if f.inserted[i].Fingerprint == fingerprint {
			return f.inserted[i], true, nil
		}
	}
	return store.Insight{}, false, nil
}

func (f *fakeInsightStore) BumpInsight(ctx context.Context, id int64, detail, severity, evidenceJSON string) (store.Insight, error) {
	if f.bumpErr != nil {
		return store.Insight{}, f.bumpErr
	}
	for i := range f.inserted {
		if f.inserted[i].ID == id {
			f.inserted[i].OccurrenceCount++
			f.inserted[i].Detail = detail
			f.inserted[i].Severity = severity
			f.inserted[i].EvidenceJSON = evidenceJSON
			f.inserted[i].Dismissed = false
			f.bumped = append(f.bumped, id)
			return f.inserted[i], nil
		}
	}
	return store.Insight{}, errors.New("fakeInsightStore: BumpInsight: no such id")
}

func (f *fakeInsightStore) IsFingerprintMuted(ctx context.Context, fingerprint string) (bool, error) {
	if f.mutedErr != nil {
		return false, f.mutedErr
	}
	return f.muted[fingerprint], nil
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

	generated, err := svc.GenerateNow(context.Background())
	if err != nil {
		t.Fatalf("GenerateNow() error = %v", err)
	}
	if generated != 1 {
		t.Errorf("generated = %d, want 1", generated)
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

	if _, err := svc.GenerateNow(context.Background()); err != nil {
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

	generated, err := svc.GenerateNow(context.Background())
	if err != nil {
		t.Fatalf("GenerateNow() error = %v, want nil (a malformed response must not crash the scheduler loop)", err)
	}
	if generated != 0 {
		t.Errorf("generated = %d, want 0", generated)
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

	generated, err := svc.GenerateNow(context.Background())
	if err != nil {
		t.Fatalf("GenerateNow() error = %v", err)
	}
	if generated != 0 {
		t.Errorf("generated = %d, want 0 for a genuinely empty (nothing actionable) response", generated)
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

	if _, err := svc.GenerateNow(context.Background()); err != nil {
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

	if _, err := svc.GenerateNow(context.Background()); err == nil {
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

	if _, err := svc.GenerateNow(context.Background()); err == nil {
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

	if _, err := svc.GenerateNow(context.Background()); err != nil {
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

	if _, err := svc.GenerateNow(context.Background()); err != nil {
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

	if _, err := svc.GenerateNow(context.Background()); err != nil {
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

	if _, err := svc.GenerateNow(context.Background()); err == nil {
		t.Fatal("GenerateNow() error = nil, want the settings-fetch error propagated")
	}
	if len(ins.inserted) != 0 {
		t.Errorf("len(inserted) = %d, want 0", len(ins.inserted))
	}
}

func TestGenerateNow_RecurringFingerprint_BumpsInsteadOfDuplicating(t *testing.T) {
	chat := &fakeChatter{response: validResponse}
	ins := &fakeInsightStore{}
	svc := NewService(testPromptBuilder(), chat, ins, defaultTestSettings(), time.Hour, testLogger(&bytes.Buffer{}))

	// First pass: no prior occurrence, so this is a plain insert.
	if _, err := svc.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() [pass 1] error = %v", err)
	}
	if len(ins.inserted) != 1 {
		t.Fatalf("len(inserted) after pass 1 = %d, want 1", len(ins.inserted))
	}
	firstID := ins.inserted[0].ID

	// Second pass: same category+program (validResponse is identical), so
	// this must bump the existing row rather than inserting a second one.
	generated, err := svc.GenerateNow(context.Background())
	if err != nil {
		t.Fatalf("GenerateNow() [pass 2] error = %v", err)
	}
	if generated != 1 {
		t.Errorf("generated = %d, want 1 (the recurrence still counts as a live finding)", generated)
	}
	if len(ins.inserted) != 1 {
		t.Fatalf("len(inserted) after pass 2 = %d, want still 1 (no duplicate row)", len(ins.inserted))
	}
	if len(ins.bumped) != 1 || ins.bumped[0] != firstID {
		t.Fatalf("bumped = %v, want exactly [%d]", ins.bumped, firstID)
	}
	if ins.inserted[0].OccurrenceCount != 2 {
		t.Errorf("OccurrenceCount = %d, want 2 after one recurrence", ins.inserted[0].OccurrenceCount)
	}
}

func TestGenerateNow_RecurringFingerprint_UndismissesOnBump(t *testing.T) {
	chat := &fakeChatter{response: validResponse}
	ins := &fakeInsightStore{}
	svc := NewService(testPromptBuilder(), chat, ins, defaultTestSettings(), time.Hour, testLogger(&bytes.Buffer{}))

	if _, err := svc.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() [pass 1] error = %v", err)
	}
	ins.inserted[0].Dismissed = true

	if _, err := svc.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() [pass 2] error = %v", err)
	}
	if ins.inserted[0].Dismissed {
		t.Error("Dismissed = true after a recurrence, want a fresh occurrence to un-dismiss it")
	}
}

func TestGenerateNow_MutedFingerprint_SkippedEntirely(t *testing.T) {
	chat := &fakeChatter{response: validResponse}
	ins := &fakeInsightStore{muted: map[string]bool{
		store.ComputeFingerprint("severity-misclassification", []string{"Bambuddy"}): true,
	}}
	svc := NewService(testPromptBuilder(), chat, ins, defaultTestSettings(), time.Hour, testLogger(&bytes.Buffer{}))

	generated, err := svc.GenerateNow(context.Background())
	if err != nil {
		t.Fatalf("GenerateNow() error = %v", err)
	}
	if generated != 0 {
		t.Errorf("generated = %d, want 0 for a muted finding", generated)
	}
	if len(ins.inserted) != 0 {
		t.Errorf("len(inserted) = %d, want 0 - a muted fingerprint must never reach InsertInsight", len(ins.inserted))
	}
	if len(ins.bumped) != 0 {
		t.Errorf("len(bumped) = %d, want 0 - a muted fingerprint must never reach BumpInsight either", len(ins.bumped))
	}
}

func TestGenerateNow_DifferentPrograms_GetIndependentFingerprints(t *testing.T) {
	response := `[
		{"title": "t1", "detail": "d1", "severity": "warning", "category": "operational",
		 "evidence": [{"program": "UI-poller", "sample_message": "m1", "count": 1}]},
		{"title": "t2", "detail": "d2", "severity": "warning", "category": "operational",
		 "evidence": [{"program": "tinyauth", "sample_message": "m2", "count": 1}]}
	]`
	chat := &fakeChatter{response: response}
	ins := &fakeInsightStore{}
	svc := NewService(testPromptBuilder(), chat, ins, defaultTestSettings(), time.Hour, testLogger(&bytes.Buffer{}))

	if _, err := svc.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() error = %v", err)
	}
	if len(ins.inserted) != 2 {
		t.Fatalf("len(inserted) = %d, want 2 - same category but different programs must not collide", len(ins.inserted))
	}
	if ins.inserted[0].Fingerprint == ins.inserted[1].Fingerprint {
		t.Error("both insights got the same fingerprint despite different evidence programs")
	}
}

type fakeAlertListerErr struct{ err error }

func (f *fakeAlertListerErr) ListAlerts(ctx context.Context, state string) ([]store.Alert, error) {
	return nil, f.err
}
