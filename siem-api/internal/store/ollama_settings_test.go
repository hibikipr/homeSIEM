package store

import (
	"context"
	"testing"
)

func TestGetOllamaSettings_DefaultsAppliedByMigration(t *testing.T) {
	s := newTestStoreForNotifications(t)

	got, err := s.GetOllamaSettings(context.Background())
	if err != nil {
		t.Fatalf("GetOllamaSettings() error = %v", err)
	}
	if got.SystemPrompt != "" {
		t.Errorf("SystemPrompt = %q, want empty (means use the built-in default)", got.SystemPrompt)
	}
	if got.Temperature != 0.2 {
		t.Errorf("Temperature = %v, want 0.2", got.Temperature)
	}
	if got.TopP != 0.9 {
		t.Errorf("TopP = %v, want 0.9", got.TopP)
	}
	if got.NumPredict != 1024 {
		t.Errorf("NumPredict = %d, want 1024", got.NumPredict)
	}
	if got.NumCtx != 8192 {
		t.Errorf("NumCtx = %d, want 8192", got.NumCtx)
	}
}

func TestSetOllamaSettings_RoundTrips(t *testing.T) {
	s := newTestStoreForNotifications(t)
	ctx := context.Background()

	want := OllamaSettings{
		SystemPrompt: "custom prompt text",
		Temperature:  0.5,
		TopP:         0.8,
		NumPredict:   2048,
		NumCtx:       16384,
	}
	if err := s.SetOllamaSettings(ctx, want); err != nil {
		t.Fatalf("SetOllamaSettings() error = %v", err)
	}

	got, err := s.GetOllamaSettings(ctx)
	if err != nil {
		t.Fatalf("GetOllamaSettings() error = %v", err)
	}
	if got != want {
		t.Errorf("GetOllamaSettings() = %+v, want %+v", got, want)
	}
}

func TestSetOllamaSettings_RecreatesRowIfMissing(t *testing.T) {
	s := newTestStoreForNotifications(t)
	ctx := context.Background()

	if _, err := s.db.ExecContext(ctx, `DELETE FROM ollama_settings WHERE id = 1`); err != nil {
		t.Fatalf("delete row: %v", err)
	}

	want := OllamaSettings{SystemPrompt: "", Temperature: 0.1, TopP: 0.7, NumPredict: 512, NumCtx: 4096}
	if err := s.SetOllamaSettings(ctx, want); err != nil {
		t.Fatalf("SetOllamaSettings() error = %v", err)
	}

	got, err := s.GetOllamaSettings(ctx)
	if err != nil {
		t.Fatalf("GetOllamaSettings() error = %v", err)
	}
	if got != want {
		t.Errorf("GetOllamaSettings() = %+v, want %+v (SetOllamaSettings must recreate the row if it's missing, not silently no-op)", got, want)
	}
}
