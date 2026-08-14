package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/insights"
	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/ollama"
)

type fakeChatterAPI struct{ response string }

func (f *fakeChatterAPI) Chat(ctx context.Context, systemPrompt, userPrompt string, opts ollama.ChatOptions) (string, error) {
	return f.response, nil
}

type fakeChatterUnreachableAPI struct{}

func (f *fakeChatterUnreachableAPI) Chat(ctx context.Context, systemPrompt, userPrompt string, opts ollama.ChatOptions) (string, error) {
	return "", fmt.Errorf("ollama: chat request to http://192.168.1.135:11434: %w: dial tcp 192.168.1.135:11434: connect: no route to host", ollama.ErrUnreachable)
}

type fakeLokiQuerierAPI struct{}

func (fakeLokiQuerierAPI) QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (loki.QueryResult, error) {
	return loki.QueryResult{}, nil
}

func (fakeLokiQuerierAPI) QueryInstant(ctx context.Context, logql string, at time.Time) (loki.MatrixResult, error) {
	return loki.MatrixResult{}, nil
}

const fakeInsightResponse = `[{"title":"t","detail":"d","severity":"warning","category":"other","evidence":[]}]`

func TestListInsights_ReturnsEmptyArrayWhenNoneExist(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "viewer", 5)

	req := httptest.NewRequest(http.MethodGet, "/insights", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got []insightResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got == nil {
		t.Error("response = nil, want an empty (but non-null) array")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

func TestGenerateInsights_ViewerForbidden(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "viewer", 5)

	req := httptest.NewRequest(http.MethodPost, "/insights/generate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestGenerateInsights_NotConfigured_Returns400(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "analyst", 50)

	req := httptest.NewRequest(http.MethodPost, "/insights/generate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGenerateInsights_OllamaUnreachable_Returns502WithClearMessage(t *testing.T) {
	s, st := newTestServer(t)
	pb := &insights.PromptBuilder{Loki: fakeLokiQuerierAPI{}, Alerts: st, JobLabel: "siem"}
	s.deps.Insights = insights.NewService(pb, &fakeChatterUnreachableAPI{}, st, st, time.Hour, s.deps.Logger)
	token := authToken(t, st, "analyst", 50)

	req := httptest.NewRequest(http.MethodPost, "/insights/generate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Ollama host not reachable") {
		t.Errorf("body = %q, want it to mention the host being unreachable rather than a generic failure", rec.Body.String())
	}
}

func TestGenerateInsights_Success_InsertsAndReturnsInsights(t *testing.T) {
	s, st := newTestServer(t)
	pb := &insights.PromptBuilder{Loki: fakeLokiQuerierAPI{}, Alerts: st, JobLabel: "siem"}
	s.deps.Insights = insights.NewService(pb, &fakeChatterAPI{response: fakeInsightResponse}, st, st, time.Hour, s.deps.Logger)
	token := authToken(t, st, "analyst", 50)

	req := httptest.NewRequest(http.MethodPost, "/insights/generate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got generateInsightsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.Generated != 1 {
		t.Errorf("Generated = %d, want 1", got.Generated)
	}
	if len(got.Insights) != 1 || got.Insights[0].Title != "t" {
		t.Errorf("Insights = %+v, want one insight titled %q", got.Insights, "t")
	}
}

func TestGenerateInsights_NoInsightsProduced_ReturnsZeroGenerated(t *testing.T) {
	s, st := newTestServer(t)
	pb := &insights.PromptBuilder{Loki: fakeLokiQuerierAPI{}, Alerts: st, JobLabel: "siem"}
	s.deps.Insights = insights.NewService(pb, &fakeChatterAPI{response: "[]"}, st, st, time.Hour, s.deps.Logger)
	token := authToken(t, st, "analyst", 50)

	req := httptest.NewRequest(http.MethodPost, "/insights/generate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got generateInsightsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.Generated != 0 {
		t.Errorf("Generated = %d, want 0 when the model returned nothing actionable", got.Generated)
	}
	if len(got.Insights) != 0 {
		t.Errorf("Insights = %+v, want empty", got.Insights)
	}
}

func TestListInsights_ExcludesDismissedUnlessAllTrue(t *testing.T) {
	s, st := newTestServer(t)
	pb := &insights.PromptBuilder{Loki: fakeLokiQuerierAPI{}, Alerts: st, JobLabel: "siem"}
	s.deps.Insights = insights.NewService(pb, &fakeChatterAPI{response: fakeInsightResponse}, st, st, time.Hour, s.deps.Logger)
	if _, err := s.deps.Insights.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() error = %v", err)
	}
	list, err := st.ListInsights(context.Background(), false, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("setup: ListInsights() = %v, %v", list, err)
	}
	if err := st.DismissInsight(context.Background(), list[0].ID); err != nil {
		t.Fatalf("setup: DismissInsight() error = %v", err)
	}

	token := authToken(t, st, "viewer", 5)

	req := httptest.NewRequest(http.MethodGet, "/insights", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var got []insightResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 0 {
		t.Errorf("GET /insights (default) len = %d, want 0 (dismissed excluded)", len(got))
	}

	reqAll := httptest.NewRequest(http.MethodGet, "/insights?all=true", nil)
	reqAll.Header.Set("Authorization", "Bearer "+token)
	recAll := httptest.NewRecorder()
	s.Handler().ServeHTTP(recAll, reqAll)
	var gotAll []insightResponse
	json.Unmarshal(recAll.Body.Bytes(), &gotAll)
	if len(gotAll) != 1 || !gotAll[0].Dismissed {
		t.Errorf("GET /insights?all=true = %+v, want 1 dismissed insight", gotAll)
	}
}

func TestDismissInsight_Success(t *testing.T) {
	s, st := newTestServer(t)
	pb := &insights.PromptBuilder{Loki: fakeLokiQuerierAPI{}, Alerts: st, JobLabel: "siem"}
	s.deps.Insights = insights.NewService(pb, &fakeChatterAPI{response: fakeInsightResponse}, st, st, time.Hour, s.deps.Logger)
	if _, err := s.deps.Insights.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() error = %v", err)
	}
	list, _ := st.ListInsights(context.Background(), false, 10)

	token := authToken(t, st, "analyst", 50)
	req := httptest.NewRequest(http.MethodPut, "/insights/"+strconv.FormatInt(list[0].ID, 10)+"/dismiss", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
	}
}

func TestDismissInsight_UnknownID_Returns404(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "analyst", 50)

	req := httptest.NewRequest(http.MethodPut, "/insights/999/dismiss", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGenerateInsights_RecurringFinding_BumpsOccurrenceCountInstead(t *testing.T) {
	s, st := newTestServer(t)
	pb := &insights.PromptBuilder{Loki: fakeLokiQuerierAPI{}, Alerts: st, JobLabel: "siem"}
	s.deps.Insights = insights.NewService(pb, &fakeChatterAPI{response: fakeInsightResponse}, st, st, time.Hour, s.deps.Logger)

	if _, err := s.deps.Insights.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() [pass 1] error = %v", err)
	}
	if _, err := s.deps.Insights.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() [pass 2] error = %v", err)
	}

	token := authToken(t, st, "viewer", 5)
	req := httptest.NewRequest(http.MethodGet, "/insights", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	var got []insightResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 - the second pass must bump, not duplicate", len(got))
	}
	if got[0].OccurrenceCount != 2 {
		t.Errorf("OccurrenceCount = %d, want 2", got[0].OccurrenceCount)
	}
	if got[0].Fingerprint == "" {
		t.Error("Fingerprint is empty, want a non-empty computed fingerprint")
	}
}

func TestMuteInsight_Success_DismissesAndSuppressesFutureRecurrences(t *testing.T) {
	s, st := newTestServer(t)
	pb := &insights.PromptBuilder{Loki: fakeLokiQuerierAPI{}, Alerts: st, JobLabel: "siem"}
	s.deps.Insights = insights.NewService(pb, &fakeChatterAPI{response: fakeInsightResponse}, st, st, time.Hour, s.deps.Logger)
	if _, err := s.deps.Insights.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() error = %v", err)
	}
	list, _ := st.ListInsights(context.Background(), false, 10)
	id := list[0].ID

	token := authToken(t, st, "analyst", 50)
	req := httptest.NewRequest(http.MethodPut, "/insights/"+strconv.FormatInt(id, 10)+"/mute", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("mute status = %d, want 204, body=%s", rec.Code, rec.Body.String())
	}

	// Muted AND dismissed immediately.
	active, err := st.ListInsights(context.Background(), false, 10)
	if err != nil || len(active) != 0 {
		t.Fatalf("ListInsights(active) after mute = %v, %v, want empty", active, err)
	}

	// A later pass reporting the exact same finding must not resurrect it.
	if _, err := s.deps.Insights.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() after mute error = %v", err)
	}
	activeAfter, err := st.ListInsights(context.Background(), false, 10)
	if err != nil || len(activeAfter) != 0 {
		t.Fatalf("ListInsights(active) after a muted recurrence = %v, %v, want still empty", activeAfter, err)
	}
}

func TestMuteInsight_ViewerForbidden(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "viewer", 5)

	req := httptest.NewRequest(http.MethodPut, "/insights/1/mute", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestMuteInsight_UnknownID_Returns404(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "analyst", 50)

	req := httptest.NewRequest(http.MethodPut, "/insights/999/mute", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestListAndUnmuteInsights_RoundTrip(t *testing.T) {
	s, st := newTestServer(t)
	pb := &insights.PromptBuilder{Loki: fakeLokiQuerierAPI{}, Alerts: st, JobLabel: "siem"}
	s.deps.Insights = insights.NewService(pb, &fakeChatterAPI{response: fakeInsightResponse}, st, st, time.Hour, s.deps.Logger)
	if _, err := s.deps.Insights.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() error = %v", err)
	}
	list, _ := st.ListInsights(context.Background(), false, 10)
	id := list[0].ID
	fingerprint := list[0].Fingerprint

	analystToken := authToken(t, st, "analyst", 50)
	muteReq := httptest.NewRequest(http.MethodPut, "/insights/"+strconv.FormatInt(id, 10)+"/mute", nil)
	muteReq.Header.Set("Authorization", "Bearer "+analystToken)
	s.Handler().ServeHTTP(httptest.NewRecorder(), muteReq)

	viewerToken := authToken(t, st, "viewer", 5)
	listReq := httptest.NewRequest(http.MethodGet, "/insights/muted", nil)
	listReq.Header.Set("Authorization", "Bearer "+viewerToken)
	listRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET /insights/muted status = %d, want 200, body=%s", listRec.Code, listRec.Body.String())
	}
	var muted []mutedFingerprintResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &muted); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(muted) != 1 || muted[0].Fingerprint != fingerprint {
		t.Fatalf("muted = %+v, want exactly one entry for fingerprint %q", muted, fingerprint)
	}

	unmuteReq := httptest.NewRequest(http.MethodDelete, "/insights/muted/"+fingerprint, nil)
	unmuteReq.Header.Set("Authorization", "Bearer "+analystToken)
	unmuteRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(unmuteRec, unmuteReq)
	if unmuteRec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /insights/muted/{fingerprint} status = %d, want 204, body=%s", unmuteRec.Code, unmuteRec.Body.String())
	}

	// Unmuting doesn't resurrect the old row, but a fresh pass now inserts again.
	if _, err := s.deps.Insights.GenerateNow(context.Background()); err != nil {
		t.Fatalf("GenerateNow() after unmute error = %v", err)
	}
	activeAfter, err := st.ListInsights(context.Background(), false, 10)
	if err != nil || len(activeAfter) != 1 {
		t.Fatalf("ListInsights(active) after unmute+recur = %v, %v, want exactly 1", activeAfter, err)
	}
}

func TestUnmuteInsight_UnknownFingerprint_Returns404(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "analyst", 50)

	req := httptest.NewRequest(http.MethodDelete, "/insights/muted/does-not-exist", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}
