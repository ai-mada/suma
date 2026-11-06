package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func cloudProgressRequest(h *cloudFixture, method, path, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://suma.test/api/v1"+path, nil)
	if token == "" {
		r.AddCookie(h.f.cookie)
	} else {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.f.handler.ServeHTTP(w, r)
	return w
}

func cloudProgressPath(h *cloudFixture) string {
	return "/runs/" + url.PathEscape(h.delegation.RunID) + "/nodes/" + url.PathEscape(h.delegation.NodeID) + "/workbuddy-status"
}

func cloudProgressToken(t *testing.T, h *cloudFixture, scope string) string {
	t.Helper()
	issued, err := h.f.s.IssueProjectCredential(context.Background(), h.f.actor, ProjectCredentialIssueRequest{
		ProjectID: h.f.project, Scope: scope, IdempotencyKey: newID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return issued.Token
}

func cloudProgressExpect(t *testing.T, w *httptest.ResponseRecorder, want map[string]any) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status read failed: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("protected response lacks privacy headers")
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projection changed:\n got %#v\nwant %#v", got, want)
	}
}

func cloudProgressBase(t *testing.T, h *cloudFixture, state string) map[string]any {
	t.Helper()
	run, err := h.f.e.GetRun(context.Background(), h.f.user.TenantID, h.f.project, h.delegation.RunID)
	if err != nil || len(run.Nodes) != 1 {
		t.Fatalf("read fixture run: %v", err)
	}
	node := run.Nodes[0]
	return map[string]any{
		"project_id": h.f.project, "run_id": run.ID, "node_id": node.ID,
		"run_version": float64(run.Version), "run_state": run.State, "node_state": node.State,
		"attempt": float64(node.Attempt), "submission_state": state, "observation_failed": false,
	}
}

func TestWorkBuddyCloudProgressExposesOnlyStoredExactFacts(t *testing.T) {
	h := newCloudFixture(t, nil)
	h.f.handler = (&Server{Service: h.f.s, Engine: h.f.e, WorkBuddy: h.w}).Handler()
	path := cloudProgressPath(h) + "?project_id=" + url.QueryEscape(h.f.project)
	token := cloudProgressToken(t, h, "api")
	read := func() *httptest.ResponseRecorder { return cloudProgressRequest(h, http.MethodGet, path, token) }
	cloudProgressExpect(t, read(), cloudProgressBase(t, h, "not_recorded"))

	created, err := h.w.CreateCloudTask(context.Background(), h.actor, h.f.project, h.request)
	if err != nil || created.Task == nil || created.Task.Status != "running" {
		t.Fatalf("establish known submission: %v", err)
	}
	want := cloudProgressBase(t, h, "task_known")
	want["provider_status"], want["observed_at"] = "running", created.ObservedAt
	cloudProgressExpect(t, read(), want)
	if _, err = h.f.s.DB.Exec(`UPDATE dag_nodes SET reason=? WHERE run_id=? AND id=?`, "cloud.waiting_input", h.delegation.RunID, h.delegation.NodeID); err != nil {
		t.Fatal(err)
	}
	want["node_reason"] = "cloud.waiting_input"
	cloudProgressExpect(t, read(), want)

	// Preserve a stale observation while testing that free-form provider and
	// executor text does not cross this API, even with an authorized caller.
	created.Task.Status, created.LastError = "future status with private text", "workbuddy_cloud_upstream"
	raw := stringify(created)
	if _, err = h.f.s.DB.Exec(`UPDATE suma_workbuddy_cloud_submissions SET payload=? WHERE id=?`, raw, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.f.s.DB.Exec(`UPDATE dag_nodes SET reason=? WHERE run_id=? AND id=?`, "executor private text", h.delegation.RunID, h.delegation.NodeID); err != nil {
		t.Fatal(err)
	}
	delete(want, "node_reason")
	want["provider_status"], want["observation_failed"] = "unknown", true
	cloudProgressExpect(t, read(), want)
	cloudProgressExpect(t, cloudProgressRequest(h, http.MethodGet, path, ""), want)

	var after string
	if err = h.f.s.DB.QueryRow(`SELECT payload FROM suma_workbuddy_cloud_submissions WHERE id=?`, created.ID).Scan(&after); err != nil || after != raw {
		t.Fatalf("read changed saved submission: %v", err)
	}
	if h.creates.Load() != 1 || h.queries.Load() != 0 {
		t.Fatalf("progress read contacted vendor: %d creates, %d queries", h.creates.Load(), h.queries.Load())
	}
	created.RequestSHA256 = strings.Repeat("0", 64)
	if _, err = h.f.s.DB.Exec(`UPDATE suma_workbuddy_cloud_submissions SET payload=? WHERE id=?`, stringify(created), created.ID); err != nil {
		t.Fatal(err)
	}
	if w := read(); w.Code != http.StatusConflict || strings.Contains(w.Body.String(), "provider_status") {
		t.Fatalf("submission with changed source was projected: %d %s", w.Code, w.Body.String())
	}
}

func TestWorkBuddyCloudProgressPreservesPreparedAndUnknown(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	h := newCloudFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusBadGateway)
	})
	h.f.handler = (&Server{Service: h.f.s, Engine: h.f.e, WorkBuddy: h.w}).Handler()
	path := cloudProgressPath(h) + "?project_id=" + url.QueryEscape(h.f.project)
	token := cloudProgressToken(t, h, "api")
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	done := make(chan error, 1)
	go func() {
		_, err := h.w.CreateCloudTask(context.Background(), h.actor, h.f.project, h.request)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("provider attempt never started")
	}
	expectProgress := func(state string) {
		t.Helper()
		want := cloudProgressBase(t, h, state)
		cloudProgressExpect(t, cloudProgressRequest(h, http.MethodGet, path, token), want)
		cloudProgressExpect(t, cloudProgressRequest(h, http.MethodGet, cloudProgressListPath(h.f.project), token),
			map[string]any{"project_id": h.f.project, "items": []any{want}})
	}
	expectProgress("prepared")
	close(release)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("unverified provider attempt reported success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("provider attempt never finished")
	}
	expectProgress("unknown")
	if h.creates.Load() != 1 || h.queries.Load() != 0 {
		t.Fatal("reading uncertainty created or queried a supplier task")
	}
}

func TestWorkBuddyCloudProgressChecksCurrentExactProjectAuthority(t *testing.T) {
	h := newCloudFixture(t, nil)
	h.f.handler = (&Server{Service: h.f.s, Engine: h.f.e, WorkBuddy: h.w}).Handler()
	token := cloudProgressToken(t, h, "api")
	agentToken := cloudProgressToken(t, h, "agent")
	base := cloudProgressPath(h)
	query := "?project_id=" + url.QueryEscape(h.f.project)
	path := base + query
	cloudProgressExpect(t, cloudProgressRequest(h, http.MethodGet, path, token), cloudProgressBase(t, h, "not_recorded"))
	for _, test := range []struct {
		name, method, path, token string
		status                    int
	}{
		{"missing project", http.MethodGet, base, token, http.StatusBadRequest},
		{"duplicate project", http.MethodGet, path + "&project_id=" + h.f.project, token, http.StatusBadRequest},
		{"extra query", http.MethodGet, path + "&unused=x", token, http.StatusBadRequest},
		{"wrong project", http.MethodGet, base + "?project_id=another-project", token, http.StatusForbidden},
		{"wrong node", http.MethodGet, strings.Replace(base, "/nodes/cloud/", "/nodes/missing/", 1) + query, token, http.StatusNotFound},
		{"runner denied", http.MethodGet, path, h.claimToken, http.StatusForbidden},
		{"agent denied", http.MethodGet, path, agentToken, http.StatusForbidden},
		{"method denied", http.MethodPost, path, token, http.StatusMethodNotAllowed},
		{"private route", http.MethodGet, "/workbuddy-cloud-nodes" + query + "&run_id=" + h.delegation.RunID + "&node_id=cloud", token, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := cloudProgressRequest(h, test.method, test.path, test.token)
			if w.Code != test.status || strings.Contains(w.Body.String(), "fixed cloud prompt") {
				t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
			}
			if test.status == http.StatusMethodNotAllowed && w.Header().Get("Allow") != http.MethodGet {
				t.Fatal("method denial omitted the read-only method")
			}
		})
	}
	api := domainCActor(t, h.f.s, token)
	if _, err := h.f.s.DB.Exec(`UPDATE suma_credential_grants SET revoked=1 WHERE id=?`, api.credential.grantID); err != nil {
		t.Fatal(err)
	}
	if w := cloudProgressRequest(h, http.MethodGet, path, token); w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked credential read progress: %d %s", w.Code, w.Body.String())
	}
	if h.creates.Load() != 0 || h.queries.Load() != 0 {
		t.Fatal("authorization probes contacted vendor")
	}
}
