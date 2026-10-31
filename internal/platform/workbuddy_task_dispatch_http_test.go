package platform

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func workBuddyTaskHTTP(t *testing.T, h workBuddyTaskDispatchFixture, method, path, raw, token string, want int) map[string]any {
	t.Helper()
	r := httptest.NewRequest(method, "http://suma.test/api/v1"+path, strings.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	if token == "" {
		r.AddCookie(h.f.cookie)
		r.Header.Set("X-Suma-Expected-User", h.f.view.ID)
		r.Header.Set("X-Suma-Expected-Organization", h.f.view.TenantID)
	} else {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.f.handler.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s =>%d want%d: %s", method, path, w.Code, want, w.Body.String())
	}
	var out map[string]any
	if json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatal("non-JSON dispatch response")
	}
	return out
}

func TestWorkBuddyTaskDispatchHTTPClosedRequestsAndPrivateScope(t *testing.T) {
	h := newWorkBuddyTaskDispatchFixture(t)
	f := h.f
	f.handler = (&Server{Service: f.s, Engine: f.e, WorkBuddy: h.w}).Handler()
	path := "/workbuddy-task-dispatch-plans"
	raw := stringify(h.req)
	for _, body := range []string{strings.TrimSuffix(raw, "}") + `,"prompt":"replacement"}`, strings.TrimSuffix(raw, "}") + `,"grant_generation":2}`, strings.Replace(raw, `"expected_task_version":1`, `"expected_task_version":null`, 1), strings.Replace(raw, `"expected_task_version":1`, `"expected_task_version":1.0`, 1), strings.Replace(raw, `"grant_generation":1,`, "", 1), strings.TrimSuffix(raw, "}") + `,"knowledge":null}`} {
		workBuddyTaskHTTP(t, h, "POST", path, body, "", 400)
	}
	workBuddyTaskHTTP(t, h, "POST", path+"?project_id="+f.project, raw, "", 400)
	issued, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	workBuddyTaskHTTP(t, h, "POST", path, raw, issued.Token, 403)
	first := workBuddyTaskHTTP(t, h, "POST", path, raw, "", 201)
	id := first["id"].(string)
	replay := workBuddyTaskHTTP(t, h, "POST", path, raw, "", 201)
	if replay["id"] != id {
		t.Fatal("preview retry duplicated plan")
	}
	workBuddyTaskHTTP(t, h, "GET", path+"/"+id+"?project_id="+f.project+"&project_id="+f.project, "", "", 400)
	workBuddyTaskHTTP(t, h, "GET", path+"/"+id+"?project_id="+f.project+"&owner_id=other", "", "", 400)
	workBuddyTaskHTTP(t, h, "GET", path+"/"+id+"?project_id="+f.project, "", "", 200)
	start := WorkBuddyTaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()}
	workBuddyTaskHTTP(t, h, "POST", path+"/"+id+"/start", strings.TrimSuffix(stringify(start), "}")+`,"run_id":"other"}`, "", 400)
	workBuddyTaskHTTP(t, h, "POST", path+"/"+id+"/start", stringify(start), "", 201)
	if domainACount(t, f.s, "dag_runs") != 1 || h.creates.Load() != 0 {
		t.Fatal("browser directly called supplier or duplicated run")
	}
}
