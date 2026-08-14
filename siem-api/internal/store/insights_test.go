package store

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

func fakeInsight(overrides func(*Insight)) Insight {
	in := Insight{
		Title:        "Bambuddy errors look mistagged",
		Detail:       "Several ERROR lines from Bambuddy are landing as info.",
		Severity:     "warning",
		Category:     "severity-misclassification",
		EvidenceJSON: `[{"program":"Bambuddy","sample_message":"ERROR ...","count":12}]`,
	}
	if overrides != nil {
		overrides(&in)
	}
	return in
}

func TestInsertInsight_RoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	inserted, err := s.InsertInsight(ctx, fakeInsight(nil))
	if err != nil {
		t.Fatalf("InsertInsight() error = %v", err)
	}
	if inserted.ID == 0 {
		t.Error("inserted.ID = 0, want a real row ID")
	}
	if inserted.CreatedAt.IsZero() {
		t.Error("inserted.CreatedAt is zero, want it set")
	}
	if inserted.Dismissed {
		t.Error("inserted.Dismissed = true, want false by default")
	}

	got, err := s.ListInsights(ctx, false, 10)
	if err != nil {
		t.Fatalf("ListInsights() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Title != inserted.Title || got[0].Detail != inserted.Detail ||
		got[0].Severity != inserted.Severity || got[0].Category != inserted.Category ||
		got[0].EvidenceJSON != inserted.EvidenceJSON {
		t.Errorf("got[0] = %+v, want fields matching %+v", got[0], inserted)
	}
}

func TestListInsights_ExcludesDismissedByDefault(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	kept, err := s.InsertInsight(ctx, fakeInsight(func(i *Insight) { i.Title = "kept" }))
	if err != nil {
		t.Fatalf("InsertInsight() error = %v", err)
	}
	dismissed, err := s.InsertInsight(ctx, fakeInsight(func(i *Insight) { i.Title = "dismissed" }))
	if err != nil {
		t.Fatalf("InsertInsight() error = %v", err)
	}
	if err := s.DismissInsight(ctx, dismissed.ID); err != nil {
		t.Fatalf("DismissInsight() error = %v", err)
	}

	nonDismissed, err := s.ListInsights(ctx, false, 10)
	if err != nil {
		t.Fatalf("ListInsights(false) error = %v", err)
	}
	if len(nonDismissed) != 1 || nonDismissed[0].ID != kept.ID {
		t.Errorf("ListInsights(false) = %+v, want only the non-dismissed insight", nonDismissed)
	}

	all, err := s.ListInsights(ctx, true, 10)
	if err != nil {
		t.Fatalf("ListInsights(true) error = %v", err)
	}
	if len(all) != 2 {
		t.Errorf("ListInsights(true) len = %d, want 2", len(all))
	}
	for _, in := range all {
		if in.ID == dismissed.ID && !in.Dismissed {
			t.Error("dismissed insight's Dismissed field = false, want true")
		}
	}
}

func TestListInsights_RespectsLimit(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	for i := 0; i < 5; i++ {
		if _, err := s.InsertInsight(ctx, fakeInsight(nil)); err != nil {
			t.Fatalf("InsertInsight() error = %v", err)
		}
	}

	got, err := s.ListInsights(ctx, false, 3)
	if err != nil {
		t.Fatalf("ListInsights() error = %v", err)
	}
	if len(got) != 3 {
		t.Errorf("len(got) = %d, want 3", len(got))
	}
}

func TestListInsights_OrdersNewestFirst(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	first, err := s.InsertInsight(ctx, fakeInsight(func(i *Insight) { i.Title = "first" }))
	if err != nil {
		t.Fatalf("InsertInsight() error = %v", err)
	}
	second, err := s.InsertInsight(ctx, fakeInsight(func(i *Insight) { i.Title = "second" }))
	if err != nil {
		t.Fatalf("InsertInsight() error = %v", err)
	}

	got, err := s.ListInsights(ctx, false, 10)
	if err != nil {
		t.Fatalf("ListInsights() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != second.ID || got[1].ID != first.ID {
		t.Errorf("got = %+v, want newest (%d) first, then %d", got, second.ID, first.ID)
	}
}

func TestDismissInsight_UnknownID_ReturnsErrNoRows(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	err := s.DismissInsight(ctx, 999)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("DismissInsight(unknown) error = %v, want sql.ErrNoRows", err)
	}
}

func TestComputeFingerprint_SameCategoryAndPrograms_SameFingerprint(t *testing.T) {
	a := ComputeFingerprint("operational", []string{"UI-poller"})
	b := ComputeFingerprint("operational", []string{"UI-poller"})
	if a != b {
		t.Errorf("ComputeFingerprint() = %q and %q, want identical for identical input", a, b)
	}
}

func TestComputeFingerprint_OrderIndependent(t *testing.T) {
	a := ComputeFingerprint("operational", []string{"tinyauth", "nginx-proxy-manager"})
	b := ComputeFingerprint("operational", []string{"nginx-proxy-manager", "tinyauth"})
	if a != b {
		t.Errorf("ComputeFingerprint() = %q and %q, want order-independent", a, b)
	}
}

func TestComputeFingerprint_DifferentCategoryOrPrograms_DifferentFingerprint(t *testing.T) {
	base := ComputeFingerprint("operational", []string{"UI-poller"})
	if got := ComputeFingerprint("security", []string{"UI-poller"}); got == base {
		t.Error("different category produced the same fingerprint")
	}
	if got := ComputeFingerprint("operational", []string{"tinyauth"}); got == base {
		t.Error("different program produced the same fingerprint")
	}
}

func TestBumpInsight_IncrementsCountAndUndismisses(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	in, err := s.InsertInsight(ctx, fakeInsight(func(i *Insight) { i.Fingerprint = "fp-1" }))
	if err != nil {
		t.Fatalf("InsertInsight() error = %v", err)
	}
	if in.OccurrenceCount != 1 {
		t.Fatalf("OccurrenceCount after insert = %d, want 1", in.OccurrenceCount)
	}
	if err := s.DismissInsight(ctx, in.ID); err != nil {
		t.Fatalf("DismissInsight() error = %v", err)
	}

	bumped, err := s.BumpInsight(ctx, in.ID, "new detail", "critical",
		`[{"program":"X","sample_message":"y","count":9}]`, "restart the X service")
	if err != nil {
		t.Fatalf("BumpInsight() error = %v", err)
	}
	if bumped.OccurrenceCount != 2 {
		t.Errorf("OccurrenceCount after bump = %d, want 2", bumped.OccurrenceCount)
	}
	if bumped.Detail != "new detail" || bumped.Severity != "critical" {
		t.Errorf("bumped = %+v, want refreshed detail/severity", bumped)
	}
	if bumped.RecommendedFix != "restart the X service" {
		t.Errorf("RecommendedFix after bump = %q, want %q", bumped.RecommendedFix, "restart the X service")
	}
	if bumped.Dismissed {
		t.Error("Dismissed = true after bump, want a recurrence to un-dismiss it")
	}
	if !bumped.LastSeenAt.After(in.CreatedAt.Add(-time.Second)) {
		t.Errorf("LastSeenAt = %v, want it refreshed to roughly now", bumped.LastSeenAt)
	}
}

func TestFindMostRecentInsightByFingerprint(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	if _, found, err := s.FindMostRecentInsightByFingerprint(ctx, "no-such-fp"); err != nil || found {
		t.Fatalf("FindMostRecentInsightByFingerprint(unknown) = found=%v, err=%v, want found=false, err=nil", found, err)
	}

	older, err := s.InsertInsight(ctx, fakeInsight(func(i *Insight) { i.Fingerprint = "fp-shared"; i.Title = "older" }))
	if err != nil {
		t.Fatalf("InsertInsight() error = %v", err)
	}
	newer, err := s.InsertInsight(ctx, fakeInsight(func(i *Insight) { i.Fingerprint = "fp-shared"; i.Title = "newer" }))
	if err != nil {
		t.Fatalf("InsertInsight() error = %v", err)
	}
	_ = older

	got, found, err := s.FindMostRecentInsightByFingerprint(ctx, "fp-shared")
	if err != nil {
		t.Fatalf("FindMostRecentInsightByFingerprint() error = %v", err)
	}
	if !found || got.ID != newer.ID {
		t.Errorf("got = %+v (found=%v), want the newest row (id=%d)", got, found, newer.ID)
	}
}

func TestMuteFingerprint_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	if muted, err := s.IsFingerprintMuted(ctx, "fp-x"); err != nil || muted {
		t.Fatalf("IsFingerprintMuted() before mute = %v, %v, want false, nil", muted, err)
	}

	if err := s.MuteFingerprint(ctx, "fp-x", "operational", "UI-poller", "UI-poller repeated errors"); err != nil {
		t.Fatalf("MuteFingerprint() error = %v", err)
	}
	if muted, err := s.IsFingerprintMuted(ctx, "fp-x"); err != nil || !muted {
		t.Fatalf("IsFingerprintMuted() after mute = %v, %v, want true, nil", muted, err)
	}

	list, err := s.ListMutedFingerprints(ctx)
	if err != nil {
		t.Fatalf("ListMutedFingerprints() error = %v", err)
	}
	if len(list) != 1 || list[0].Fingerprint != "fp-x" || list[0].Category != "operational" ||
		list[0].Programs != "UI-poller" || list[0].ExampleTitle != "UI-poller repeated errors" {
		t.Errorf("ListMutedFingerprints() = %+v, want one entry for fp-x", list)
	}

	// Muting again must not error (INSERT OR REPLACE), and must refresh
	// muted_at/example_title rather than leaving the first call's title stale.
	if err := s.MuteFingerprint(ctx, "fp-x", "operational", "UI-poller", "UI-poller error fetching DHCP clients"); err != nil {
		t.Fatalf("MuteFingerprint() [second call] error = %v", err)
	}
	list, err = s.ListMutedFingerprints(ctx)
	if err != nil {
		t.Fatalf("ListMutedFingerprints() [after second mute] error = %v", err)
	}
	if len(list) != 1 || list[0].ExampleTitle != "UI-poller error fetching DHCP clients" {
		t.Errorf("ListMutedFingerprints() [after second mute] = %+v, want ExampleTitle refreshed", list)
	}

	if err := s.UnmuteFingerprint(ctx, "fp-x"); err != nil {
		t.Fatalf("UnmuteFingerprint() error = %v", err)
	}
	if muted, err := s.IsFingerprintMuted(ctx, "fp-x"); err != nil || muted {
		t.Fatalf("IsFingerprintMuted() after unmute = %v, %v, want false, nil", muted, err)
	}
}

func TestUnmuteFingerprint_UnknownFingerprint_ReturnsErrNoRows(t *testing.T) {
	s := newTestStore(t)
	err := s.UnmuteFingerprint(t.Context(), "does-not-exist")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("UnmuteFingerprint(unknown) error = %v, want sql.ErrNoRows", err)
	}
}

// TestMigrate_BackfillsFingerprintForPreExistingRows simulates a row
// written before this feature existed (empty fingerprint/last_seen_at,
// exactly what a production database looks like pre-upgrade) by inserting
// directly with raw SQL rather than through InsertInsight, which always
// sets both now. A second Migrate() call (exactly what happens on every
// real startup) must backfill both from the row's own category/evidence/
// created_at without erroring - proving upgrade-in-place works, not just
// fresh installs.
func TestMigrate_BackfillsFingerprintForPreExistingRows(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO insights (created_at, title, detail, severity, category, evidence_json, dismissed, fingerprint, occurrence_count, last_seen_at)
		VALUES ('2026-01-01 00:00:00', 'pre-existing', 'd', 'warning', 'operational', '[{"program":"UI-poller","sample_message":"m","count":1}]', 0, '', 1, '')
	`)
	if err != nil {
		t.Fatalf("raw insert error = %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId() error = %v", err)
	}

	if err := Migrate(s.db); err != nil {
		t.Fatalf("Migrate() [backfill pass] error = %v", err)
	}

	got, err := s.GetInsight(ctx, id)
	if err != nil {
		t.Fatalf("GetInsight() error = %v", err)
	}
	want := ComputeFingerprint("operational", []string{"UI-poller"})
	if got.Fingerprint != want {
		t.Errorf("Fingerprint after backfill = %q, want %q", got.Fingerprint, want)
	}
	if got.LastSeenAt.IsZero() {
		t.Error("LastSeenAt after backfill is zero, want it backfilled from created_at")
	}
}
