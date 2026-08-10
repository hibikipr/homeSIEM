package store

import (
	"database/sql"
	"errors"
	"testing"
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
