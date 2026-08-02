package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hibikipr/homeSIEM/siem-api/internal/rules"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

// newSchedulerTestServer gives the base test server (Task 23) a real,
// otherwise-idle Scheduler with no registered evaluators — Task 21 already
// covers scheduler evaluation behavior; here we only need CreateRule /
// UpdateRule / DeleteRule to be able to call StartRule / StopRule on
// something real without erroring.
func newSchedulerTestServer(t *testing.T) *Server {
	t.Helper()
	s, st := newTestServer(t)
	s.deps.Scheduler = rules.NewScheduler(st, map[string]rules.Evaluator{}, nil, apiTestLogger())
	s.deps.SchedulerCtx = context.Background()
	return s
}

func TestCreateRule_RequiresAnalyst(t *testing.T) {
	s := newSchedulerTestServer(t)
	token := authToken(t, s.deps.Store, "viewer", 100)

	body := `{"name":"wan-portscan","shape":"threshold","logql":"{job=\"siem\"}","window_sec":60,"threshold":5,"group_by":["src_ip"],"severity":"critical","destinations":["inapp"],"cooldown_sec":3600,"interval_sec":60,"enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/rules", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestCreateAndListRules(t *testing.T) {
	s := newSchedulerTestServer(t)
	token := authToken(t, s.deps.Store, "analyst", 50)

	body := `{"name":"wan-portscan","shape":"threshold","logql":"{job=\"siem\"}","window_sec":60,"threshold":5,"group_by":["src_ip"],"severity":"critical","destinations":["inapp"],"cooldown_sec":3600,"interval_sec":60,"enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/rules", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	var created ruleResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if created.ID == 0 || created.Name != "wan-portscan" {
		t.Fatalf("created = %+v", created)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/rules", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(listRec, listReq)

	var list []ruleResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %+v, want 1 rule", list)
	}

	entries, err := s.deps.Store.ListAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == "rule.create" {
			found = true
			if e.UserID == nil {
				t.Error("rule.create audit entry has nil UserID, want actor set")
			}
		}
	}
	if !found {
		t.Error("no rule.create audit entry found")
	}
}

func TestUpdateRule_DisablesStopsScheduler(t *testing.T) {
	s := newSchedulerTestServer(t)
	ctx := context.Background()
	token := authToken(t, s.deps.Store, "analyst", 50)

	created, err := s.deps.Store.CreateRule(ctx, store.Rule{
		Name: "r", Shape: "absence", Severity: "low", Destinations: []string{"inapp"},
		CooldownSec: 60, IntervalSec: 60, Enabled: true,
	}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	body := `{"name":"r","shape":"absence","severity":"low","destinations":["inapp"],"cooldown_sec":60,"interval_sec":60,"enabled":false}`
	req := httptest.NewRequest(http.MethodPut, "/rules/"+itoa(created.ID), bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	got, err := s.deps.Store.GetRule(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRule() error = %v", err)
	}
	if got.Enabled {
		t.Error("Enabled = true, want false after update")
	}

	entries, err := s.deps.Store.ListAudit(ctx, 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == "rule.update" {
			found = true
			if e.UserID == nil {
				t.Error("rule.update audit entry has nil UserID, want actor set")
			}
		}
	}
	if !found {
		t.Error("no rule.update audit entry found")
	}
}

func TestDeleteRule(t *testing.T) {
	s := newSchedulerTestServer(t)
	ctx := context.Background()
	token := authToken(t, s.deps.Store, "analyst", 50)

	created, err := s.deps.Store.CreateRule(ctx, store.Rule{
		Name: "r", Shape: "absence", Severity: "low", Destinations: []string{"inapp"},
		CooldownSec: 60, IntervalSec: 60, Enabled: true,
	}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/rules/"+itoa(created.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
	}
	if _, err := s.deps.Store.GetRule(ctx, created.ID); err == nil {
		t.Fatal("GetRule() after delete: error = nil, want not found")
	}

	entries, err := s.deps.Store.ListAudit(ctx, 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == "rule.delete" {
			found = true
			if e.UserID == nil {
				t.Error("rule.delete audit entry has nil UserID, want actor set")
			}
		}
	}
	if !found {
		t.Error("no rule.delete audit entry found")
	}
}
