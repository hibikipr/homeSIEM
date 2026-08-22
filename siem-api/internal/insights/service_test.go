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

func (f *fakeInsightStore) BumpInsight(ctx context.Context, id int64, detail, severity, evidenceJSON, recommendedFix string) (store.Insight, error) {
	if f.bumpErr != nil {
		return store.Insight{}, f.bumpErr
	}
	for i := range f.inserted {
		if f.inserted[i].ID == id {
			f.inserted[i].OccurrenceCount++
			f.inserted[i].Detail = detail
			f.inserted[i].Severity = severity
			f.inserted[i].EvidenceJSON = evidenceJSON
			f.inserted[i].RecommendedFix = recommendedFix
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
	if got.RecommendedFix != "" {
		t.Errorf("RecommendedFix = %q, want empty - validResponse omits the field entirely", got.RecommendedFix)
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

func TestGenerateNow_TruncatedResponse_SalvagesCompleteInsightsAndWarns(t *testing.T) {
	// Simulates the real production failure this was written against:
	// Ollama hitting num_predict mid-generation, cutting the JSON array
	// off partway through its last element - here, mid-way through the
	// third object's "category" value, with no closing braces/brackets at
	// all after that point.
	truncated := `[
		{"title":"a","detail":"d1","severity":"warning","category":"operational","evidence":[{"program":"UI-poller","sample_message":"x","count":1}]},
		{"title":"b","detail":"d2","severity":"warning","category":"operational","evidence":[{"program":"tinyauth","sample_message":"y","count":1}]},
		{"title":"c","detail":"d3","severity":"warning","cate`
	var buf bytes.Buffer
	chat := &fakeChatter{response: truncated}
	ins := &fakeInsightStore{}
	svc := NewService(testPromptBuilder(), chat, ins, defaultTestSettings(), time.Hour, testLogger(&buf))

	generated, err := svc.GenerateNow(context.Background())
	if err != nil {
		t.Fatalf("GenerateNow() error = %v, want nil - a truncated tail must not fail the whole pass", err)
	}
	if generated != 2 {
		t.Errorf("generated = %d, want 2 (the two complete insights, not the truncated third)", generated)
	}
	if len(ins.inserted) != 2 || ins.inserted[0].Title != "a" || ins.inserted[1].Title != "b" {
		t.Fatalf("inserted = %+v, want insights titled a and b", ins.inserted)
	}
	if !bytes.Contains(buf.Bytes(), []byte("WARN")) {
		t.Error("expected a warning to be logged even though the pass partially succeeded, for operational visibility into the truncation")
	}
}

func TestGenerateNow_TruncatedResponse_EndsRightAfterLastCompleteElement_StillSalvages(t *testing.T) {
	// Edge case for the truncation detector itself: the stream ends exactly
	// after a complete element's closing '}' - no trailing comma, no
	// partial next object, and no closing ']' either. dec.More() alone
	// can't tell this apart from a clean close (both look like "nothing
	// more to read"), which is why parseModelResponse explicitly consumes
	// the closing token afterward rather than trusting More() alone.
	truncated := `[{"title":"a","detail":"d1","severity":"warning","category":"operational","evidence":[]}`
	chat := &fakeChatter{response: truncated}
	ins := &fakeInsightStore{}
	svc := NewService(testPromptBuilder(), chat, ins, defaultTestSettings(), time.Hour, testLogger(&bytes.Buffer{}))

	generated, err := svc.GenerateNow(context.Background())
	if err != nil {
		t.Fatalf("GenerateNow() error = %v, want nil", err)
	}
	if generated != 1 || len(ins.inserted) != 1 || ins.inserted[0].Title != "a" {
		t.Fatalf("generated=%d inserted=%+v, want the one complete insight salvaged", generated, ins.inserted)
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

const responseWithRecommendedFix = `[
  {
    "title": "UI-poller repeated errors",
    "detail": "The list/alarm endpoint is returning 400 on every poll.",
    "severity": "warning",
    "category": "operational",
    "evidence": [{"program": "UI-poller", "sample_message": "400 invalid status code from server", "count": 40}],
    "recommended_fix": "Set POLLER_SAVE_ALARMS=false - the endpoint no longer works on this controller version."
  }
]`

func TestGenerateNow_RecommendedFixPresent_IsStoredOnInsert(t *testing.T) {
	chat := &fakeChatter{response: responseWithRecommendedFix}
	ins := &fakeInsightStore{}
	svc := NewService(testPromptBuilder(), chat, ins, defaultTestSettings(), time.Hour, testLogger(&bytes.Buffer{}))

	if _, err := svc.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() error = %v", err)
	}
	if len(ins.inserted) != 1 {
		t.Fatalf("len(inserted) = %d, want 1", len(ins.inserted))
	}
	want := "Set POLLER_SAVE_ALARMS=false - the endpoint no longer works on this controller version."
	if ins.inserted[0].RecommendedFix != want {
		t.Errorf("RecommendedFix = %q, want %q", ins.inserted[0].RecommendedFix, want)
	}
}

func TestGenerateNow_RecommendedFixPresent_IsRefreshedOnBump(t *testing.T) {
	chat := &fakeChatter{response: validResponse} // no recommended_fix
	ins := &fakeInsightStore{}
	svc := NewService(testPromptBuilder(), chat, ins, defaultTestSettings(), time.Hour, testLogger(&bytes.Buffer{}))

	if _, err := svc.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() [pass 1] error = %v", err)
	}
	if ins.inserted[0].RecommendedFix != "" {
		t.Fatalf("RecommendedFix after pass 1 = %q, want empty", ins.inserted[0].RecommendedFix)
	}

	// Pass 2 reports the same fingerprint but this time with a fix - the
	// bumped row must pick it up, not keep the first pass's empty value.
	fixedResponse := `[
	  {
	    "title": "Bambuddy errors look mistagged",
	    "detail": "Several ERROR lines are actually routine.",
	    "severity": "warning",
	    "category": "severity-misclassification",
	    "evidence": [{"program": "Bambuddy", "sample_message": "ERROR ...", "count": 12}],
	    "recommended_fix": "Add a severity-override rule for Bambuddy's ERROR-prefixed routine lines."
	  }
	]`
	chat.response = fixedResponse

	if _, err := svc.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() [pass 2] error = %v", err)
	}
	if len(ins.inserted) != 1 {
		t.Fatalf("len(inserted) after pass 2 = %d, want still 1 (bumped, not duplicated)", len(ins.inserted))
	}
	want := "Add a severity-override rule for Bambuddy's ERROR-prefixed routine lines."
	if ins.inserted[0].RecommendedFix != want {
		t.Errorf("RecommendedFix after bump = %q, want %q", ins.inserted[0].RecommendedFix, want)
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

// TestGenerateNow_SameProgramsDifferentCategory_StillBumps is the
// regression test for the actual production bug: a UniFi "Blocked by
// Firewall" condition got fingerprinted differently every time Ollama
// picked a different (individually valid) category for it across passes -
// security, then operational, then severity-misclassification - so the
// same real condition kept re-inserting as a "new" finding, and muting one
// phrasing did nothing for the next. category is no longer part of
// ComputeFingerprint at all (see its doc) specifically so this converges.
func TestGenerateNow_SameProgramsDifferentCategory_StillBumps(t *testing.T) {
	securityResponse := `[{"title": "Blocked by Firewall alert", "detail": "d1", "severity": "warning", "category": "security",
		"evidence": [{"program": "Blocked by Firewall", "sample_message": "m1", "count": 1}]}]`
	operationalResponse := `[{"title": "Blocked by Firewall warning", "detail": "d2", "severity": "warning", "category": "operational",
		"evidence": [{"program": "Blocked by Firewall", "sample_message": "m2", "count": 42}]}]`

	chat := &fakeChatter{response: securityResponse}
	ins := &fakeInsightStore{}
	svc := NewService(testPromptBuilder(), chat, ins, defaultTestSettings(), time.Hour, testLogger(&bytes.Buffer{}))

	if _, err := svc.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() [pass 1, category=security] error = %v", err)
	}
	if len(ins.inserted) != 1 {
		t.Fatalf("len(inserted) after pass 1 = %d, want 1", len(ins.inserted))
	}
	firstID := ins.inserted[0].ID

	chat.response = operationalResponse
	if _, err := svc.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() [pass 2, category=operational] error = %v", err)
	}
	if len(ins.inserted) != 1 {
		t.Fatalf("len(inserted) after pass 2 = %d, want still 1 - a category change alone must not create a second row for the same programs", len(ins.inserted))
	}
	if len(ins.bumped) != 1 || ins.bumped[0] != firstID {
		t.Fatalf("bumped = %v, want exactly [%d]", ins.bumped, firstID)
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
		store.ComputeFingerprint([]string{"Bambuddy"}): true,
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
