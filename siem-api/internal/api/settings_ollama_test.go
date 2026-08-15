package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hibikipr/homeSIEM/siem-api/internal/insights"
)

func TestGetOllamaSettings_RequiresAdmin(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "analyst", 50)

	req := httptest.NewRequest(http.MethodGet, "/settings/ollama", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestGetOllamaSettings_DefaultsAndNotConfigured(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 5)

	req := httptest.NewRequest(http.MethodGet, "/settings/ollama", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp ollamaSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Configured {
		t.Error("Configured = true, want false when OllamaURL is unset")
	}
	if resp.SystemPrompt != "" {
		t.Errorf("SystemPrompt = %q, want empty (no override stored)", resp.SystemPrompt)
	}
	if resp.DefaultSystemPrompt != insights.DefaultSystemPrompt {
		t.Error("DefaultSystemPrompt does not match insights.DefaultSystemPrompt")
	}
	if resp.Temperature != 0.2 || resp.TopP != 0.9 || resp.NumPredict != 2048 || resp.NumCtx != 8192 {
		t.Errorf("generation options = %+v, want the migration's seeded defaults", resp)
	}
}

func TestGetOllamaSettings_ConfiguredWhenUrlSet(t *testing.T) {
	s, st := newTestServer(t)
	s.deps.OllamaURL = "http://ollama.local:11434"
	s.deps.OllamaModel = "qwen3.6:27b"
	token := authToken(t, st, "admin", 5)

	req := httptest.NewRequest(http.MethodGet, "/settings/ollama", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	var resp ollamaSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.Configured {
		t.Error("Configured = false, want true when OllamaURL is set")
	}
	if resp.Model != "qwen3.6:27b" {
		t.Errorf("Model = %q, want qwen3.6:27b", resp.Model)
	}
}

func TestUpdateOllamaSettings_PersistsValidSettings(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 5)

	body := `{"system_prompt":"custom prompt","temperature":0.5,"top_p":0.8,"num_predict":2048,"num_ctx":16384}`
	req := httptest.NewRequest(http.MethodPut, "/settings/ollama", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
	}

	got, err := st.GetOllamaSettings(req.Context())
	if err != nil {
		t.Fatalf("GetOllamaSettings() error = %v", err)
	}
	if got.SystemPrompt != "custom prompt" || got.Temperature != 0.5 || got.TopP != 0.8 ||
		got.NumPredict != 2048 || got.NumCtx != 16384 {
		t.Errorf("GetOllamaSettings() = %+v, want the values just PUT", got)
	}
}

func TestUpdateOllamaSettings_EmptyPromptResetsToDefault(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 5)

	// Seed a non-default override first.
	body := `{"system_prompt":"custom prompt","temperature":0.5,"top_p":0.8,"num_predict":2048,"num_ctx":16384}`
	r1 := httptest.NewRequest(http.MethodPut, "/settings/ollama", bytes.NewReader([]byte(body)))
	r1.Header.Set("Authorization", "Bearer "+token)
	rec1 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec1, r1)
	if rec1.Code != http.StatusNoContent {
		t.Fatalf("setup PUT status = %d, want 204", rec1.Code)
	}

	// Now PUT with an empty system_prompt - should reset to "use default".
	resetBody := `{"system_prompt":"","temperature":0.5,"top_p":0.8,"num_predict":2048,"num_ctx":16384}`
	r2 := httptest.NewRequest(http.MethodPut, "/settings/ollama", bytes.NewReader([]byte(resetBody)))
	r2.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, r2)
	if rec2.Code != http.StatusNoContent {
		t.Fatalf("reset PUT status = %d, want 204", rec2.Code)
	}

	got, err := st.GetOllamaSettings(r2.Context())
	if err != nil {
		t.Fatalf("GetOllamaSettings() error = %v", err)
	}
	if got.SystemPrompt != "" {
		t.Errorf("SystemPrompt = %q, want empty after resetting", got.SystemPrompt)
	}
}

func TestUpdateOllamaSettings_RejectsOutOfRangeValues(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 5)

	cases := []string{
		`{"temperature":5,"top_p":0.9,"num_predict":1024,"num_ctx":8192}`,
		`{"temperature":0.2,"top_p":0,"num_predict":1024,"num_ctx":8192}`,
		`{"temperature":0.2,"top_p":1.5,"num_predict":1024,"num_ctx":8192}`,
		`{"temperature":0.2,"top_p":0.9,"num_predict":0,"num_ctx":8192}`,
		`{"temperature":0.2,"top_p":0.9,"num_predict":1024,"num_ctx":10}`,
	}
	for _, body := range cases {
		req := httptest.NewRequest(http.MethodPut, "/settings/ollama", bytes.NewReader([]byte(body)))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("body=%s: status = %d, want 400", body, rec.Code)
		}
	}
}
