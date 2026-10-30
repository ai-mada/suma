package platform

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// Exercise the domain router using Actors from actual authenticated fixtures.
// The shared Server entry/CSRF hook is a separate integration requirement.
func callCloudHTTP(t *testing.T, h *cloudFixture, actor Actor, scope, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "/api/v1/"+path, bytes.NewReader(body))
	w := httptest.NewRecorder()
	s := &Server{Service: h.f.s, WorkBuddy: h.w}
	parts := strings.Split(strings.SplitN(path, "?", 2)[0], "/")
	if !s.workBuddyCloudHTTP(w, r, actor, scope, h.f.project, parts, h.w) {
		t.Fatal("cloud path was not handled")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("protected response is cacheable")
	}
	return w
}

func TestWorkBuddyCloudHTTPRoutesExactMachineTaskAndOwnerObservation(t *testing.T) {
	h := newCloudFixture(t, nil)
	raw, _ := json.Marshal(h.request)
	w := callCloudHTTP(t, h, h.actor, "runner", "POST", "runner/workbuddy-cloud-submissions", raw)
	if w.Code != 201 {
		t.Fatal("create route", w.Code, w.Body.String())
	}
	var submission wc.CloudSubmission
	if wc.Decode(w.Body.Bytes(), &submission) != nil || submission.Validate() != nil || submission.Task == nil || h.creates.Load() != 1 {
		t.Fatal("invalid safe submission")
	}
	path := "workbuddy-cloud-nodes?project_id=" + url.QueryEscape(h.f.project) + "&run_id=" + url.QueryEscape(h.request.Assignment.RunID) + "&node_id=" + url.QueryEscape(h.request.Assignment.NodeID)
	w = callCloudHTTP(t, h, h.f.actor, "", "GET", path, nil)
	var view wc.CloudNodeView
	if w.Code != 200 || wc.Decode(w.Body.Bytes(), &view) != nil || view.Validate() != nil || view.Submission == nil || view.Submission.ID != submission.ID || h.queries.Load() != 0 {
		t.Fatal("read contacted vendor or lost exact source", w.Code, w.Body.String())
	}
	observe := wc.CloudObserveRequest{ProjectID: h.f.project, ExpectedVersion: submission.Version, IdempotencyKey: "http-observe-152"}
	raw, _ = json.Marshal(observe)
	w = callCloudHTTP(t, h, h.f.actor, "", "POST", "workbuddy-cloud-submissions/"+submission.ID+"/observe", raw)
	if w.Code != 200 || h.queries.Load() != 1 || h.creates.Load() != 1 {
		t.Fatal("observation did not read original task", w.Code, w.Body.String())
	}
	for _, secret := range []string{"private-access", "private-refresh", "private-app-secret", "access_token", "sandboxLink"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("private material in public response")
		}
	}
}

func TestWorkBuddyCloudHTTPRejectsIdentityMixingAndUnboundInput(t *testing.T) {
	h := newCloudFixture(t, nil)
	raw, _ := json.Marshal(h.request)
	if w := callCloudHTTP(t, h, h.f.actor, "", "POST", "runner/workbuddy-cloud-submissions", raw); w.Code != 403 {
		t.Fatal("browser used machine route")
	}
	if w := callCloudHTTP(t, h, h.actor, "runner", "POST", "workbuddy-cloud-delegations", raw); w.Code != 403 {
		t.Fatal("machine used human route")
	}
	for _, invalid := range [][]byte{
		[]byte(strings.TrimSuffix(string(raw), "}") + `,"prompt":"replacement"}`),
		[]byte(strings.TrimSuffix(string(raw), "}") + `,"project_id":"other"}`),
		[]byte(strings.Replace(string(raw), `"expected_version":1`, `"expected_version":null`, 1)),
		[]byte(strings.Repeat(" ", 64<<10) + "{}"),
	} {
		if w := callCloudHTTP(t, h, h.actor, "runner", "POST", "runner/workbuddy-cloud-submissions", invalid); w.Code != 400 {
			t.Fatal("invalid machine envelope accepted", w.Code)
		}
	}
	if w := callCloudHTTP(t, h, h.actor, "runner", "POST", "runner/workbuddy-cloud-submissions?project_id="+h.f.project, raw); w.Code != 400 {
		t.Fatal("POST query accepted")
	}
	for _, query := range []string{"project_id=" + h.f.project + "&project_id=" + h.f.project, "project_id=" + h.f.project + "&token=private", "project_id=" + h.f.project} {
		if w := callCloudHTTP(t, h, h.f.actor, "", "GET", "workbuddy-cloud-nodes?"+query, nil); w.Code != 400 {
			t.Fatal("invalid exact source query accepted", w.Code)
		}
	}
	if h.creates.Load() != 0 || h.queries.Load() != 0 {
		t.Fatal("rejected request reached vendor")
	}
}

func TestWorkBuddyCloudHTTPPoolKeepsRealMachineProof(t *testing.T) {
	h := newCloudFixtureMode(t, nil, true)
	raw, _ := json.Marshal(h.request)
	s := &Server{Service: h.f.s, WorkBuddy: h.w}
	call := func(token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/v1/pool-runner/workbuddy-cloud-submissions", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if w := call(""); w.Code != 401 {
		t.Fatal("missing machine token accepted")
	}
	if w := call(h.poolToken); w.Code != 201 || h.creates.Load() != 1 {
		t.Fatal("real pool proof failed", w.Code, w.Body.String())
	}
	if w := call("unrelated-token"); w.Code < 400 || h.creates.Load() != 1 {
		t.Fatal("unknown machine inherited original receipt")
	}
	// Ordinary body readers remain bounded even on an unreadable request.
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Body = io.NopCloser(strings.NewReader(`null`))
	var req wc.CloudCreateRequest
	if cloudHTTPBody(r, &req) == nil {
		t.Fatal("null envelope accepted")
	}
}

func TestWorkBuddyCloudHTTPPublicEntryRequiresOriginalViewAndCurrentCredentials(t *testing.T) {
	h := newCloudFixture(t, nil)
	h.f.handler = (&Server{Service: h.f.s, Engine: h.f.e, WorkBuddy: h.w, AllowedOrigins: []string{"http://suma.test"}}).Handler()
	// This token is issued through the actual authenticated public endpoint.
	machine := h.f.call("POST", "/auth/tokens", h.f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	request := func(method, path string, body any, token string, originalView bool) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "http://suma.test/api/v1/"+path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://suma.test")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		} else {
			r.AddCookie(h.f.cookie)
		}
		if originalView {
			r.Header.Set("X-Suma-Expected-User", h.f.view.ID)
			r.Header.Set("X-Suma-Expected-Organization", h.f.view.TenantID)
		}
		w := httptest.NewRecorder()
		h.f.handler.ServeHTTP(w, r)
		return w
	}
	revoke := wc.CloudRevokeRequest{ProjectID: h.f.project, ExpectedVersion: h.delegation.Version, IdempotencyKey: "public-revoke-152"}
	if w := request("POST", "workbuddy-cloud-delegations/"+h.delegation.ID+"/revoke", revoke, "", false); w.Code != 428 {
		t.Fatal("browser command bypassed original view", w.Code)
	}
	if w := request("POST", "runner/workbuddy-cloud-submissions", h.request, "", true); w.Code != 403 {
		t.Fatal("browser used machine authority", w.Code)
	}
	w := request("POST", "runner/workbuddy-cloud-submissions", h.request, machine, false)
	var submission wc.CloudSubmission
	if w.Code != 201 || wc.Decode(w.Body.Bytes(), &submission) != nil || submission.Task == nil || h.creates.Load() != 1 {
		t.Fatal("public machine create failed", w.Code, w.Body.String())
	}
	observe := wc.CloudObserveRequest{ProjectID: h.f.project, ExpectedVersion: submission.Version, IdempotencyKey: "public-observe-152"}
	path := "workbuddy-cloud-submissions/" + submission.ID + "/observe"
	if w := request("POST", path, observe, "", false); w.Code != 428 || h.queries.Load() != 0 {
		t.Fatal("observation bypassed original view")
	}
	if w := request("POST", path, observe, "", true); w.Code != 200 || h.queries.Load() != 1 {
		t.Fatal("public owner observation failed", w.Code, w.Body.String())
	}
	if _, err := h.f.s.DB.Exec(`DELETE FROM suma_org_members WHERE tenant_id=? AND user_id=?`, h.f.user.TenantID, h.f.user.ID); err != nil {
		t.Fatal(err)
	}
	w = request("POST", path, observe, "", true)
	if w.Code < 400 || h.queries.Load() != 1 || strings.Contains(w.Body.String(), "cloud-task") {
		t.Fatal("revoked caller read original receipt", w.Code)
	}
}
