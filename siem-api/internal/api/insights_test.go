package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	var got []insightResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got) != 1 || got[0].Title != "t" {
		t.Errorf("got = %+v, want one insight titled %q", got, "t")
	}
}

func TestListInsights_ExcludesDismissedUnlessAllTrue(t *testing.T) {
	s, st := newTestServer(t)
	pb := &insights.PromptBuilder{Loki: fakeLokiQuerierAPI{}, Alerts: st, JobLabel: "siem"}
	s.deps.Insights = insights.NewService(pb, &fakeChatterAPI{response: fakeInsightResponse}, st, st, time.Hour, s.deps.Logger)
	if err := s.deps.Insights.GenerateNow(context.Background()); err != nil {
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
	if err := s.deps.Insights.GenerateNow(context.Background()); err != nil {
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
