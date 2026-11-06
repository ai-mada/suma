package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func cloudProgressListPath(project string) string {
	return "/workbuddy-cloud-progress?project_id=" + url.QueryEscape(project)
}

func readCloudProgressPage(t *testing.T, response *httptest.ResponseRecorder) wc.CloudNodeProgressPage {
	t.Helper()
	var page wc.CloudNodeProgressPage
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil {
		t.Fatalf("invalid progress page: %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("progress page lacks privacy headers")
	}
	expectedFields := 2
	if page.NextCursor != "" {
		expectedFields++
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil || len(fields) != expectedFields ||
		fields["items"] == nil || page.Items == nil || fields["project_id"] == nil || (fields["next_cursor"] != nil) != (page.NextCursor != "") {
		t.Fatal("progress page contains fields outside the public contract")
	}
	return page
}

func cloudProgressNextSubmission(t *testing.T, h *cloudFixture) wc.CloudSubmission {
	t.Helper()
	ctx, f := context.Background(), h.f
	first, err := f.e.GetRun(ctx, f.user.TenantID, f.project, h.delegation.RunID)
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, first.WorkflowID, newID())
	if err != nil {
		t.Fatal(err)
	}
	previous := h.delegation
	delegation, err := h.w.DelegateCloud(ctx, f.actor, f.project, wc.CloudDelegateRequest{
		ProjectID: f.project, ConnectionID: previous.ConnectionID,
		ExpectedConnectionVersion: previous.ConnectionVersionAtIssue, GrantGeneration: previous.GrantGeneration,
		RunID: run.ID, NodeID: previous.NodeID, ExpectedRunVersion: run.Version,
		NodeSpecSHA256: previous.NodeSpecSHA256, RequestSHA256: previous.RequestSHA256,
		ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339), MaxCreates: 1, IdempotencyKey: newID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	a := cloudHTTPClaim(t, h, false, newID(), http.StatusOK)
	if a == nil || a.RunID != run.ID {
		t.Fatal("second saved progress must come from a separate authoritative run")
	}
	sub, err := h.w.CreateCloudTask(ctx, h.actor, f.project, wc.CloudCreateRequest{
		ProjectID: f.project, DelegationID: delegation.ID, ExpectedVersion: delegation.Version,
		Assignment: repositoryTestRef(*a), IdempotencyKey: newID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

func TestWorkBuddyCloudProgressPageUsesSavedScopedFacts(t *testing.T) {
	h := newCloudFixture(t, nil)
	f := h.f
	f.handler = (&Server{Service: f.s, Engine: f.e, WorkBuddy: h.w}).Handler()
	path := cloudProgressListPath(f.project)
	read := func(path, token string) *httptest.ResponseRecorder {
		return cloudProgressRequest(h, http.MethodGet, path, token)
	}
	if page := readCloudProgressPage(t, read(path, "")); page.ProjectID != f.project || len(page.Items) != 0 || page.NextCursor != "" {
		t.Fatalf("unsubmitted workflow appeared in the progress list: %+v", page)
	}

	first := createCloudFixture(t, h)
	a, r := cloudResultFixture(t, h)
	r.WorkBuddyCloudWait.SubmissionID = first.ID
	r.WorkBuddyCloudWait.SubmissionVersion = first.Version
	r.WorkBuddyCloudWait.Cause = "observation_unconfirmed"
	if err := completeCloudFixture(t, h, a, r); err != nil {
		t.Fatal(err)
	}
	second := cloudProgressNextSubmission(t, h)
	subs := []wc.CloudSubmission{first, second}
	slices.SortFunc(subs, func(a, b wc.CloudSubmission) int { return strings.Compare(a.ID, b.ID) })
	token, agent := cloudProgressToken(t, h, "api"), cloudProgressToken(t, h, "agent")
	firstPage := readCloudProgressPage(t, read(path+"&limit=1", token))
	if firstPage.ProjectID != f.project || len(firstPage.Items) != 1 || firstPage.NextCursor == "" {
		t.Fatalf("invalid first progress page: %+v", firstPage)
	}
	continuation := "&limit=1&before=" + url.QueryEscape(firstPage.NextCursor)
	secondPage := readCloudProgressPage(t, read(path+continuation, token))
	if len(secondPage.Items) != 1 || secondPage.NextCursor != "" {
		t.Fatalf("invalid final progress page: %+v", secondPage)
	}
	for i, item := range append(firstPage.Items, secondPage.Items...) {
		sub := subs[i]
		exact := read("/runs/"+sub.RunID+"/nodes/"+sub.NodeID+"/workbuddy-status?project_id="+f.project, token)
		got, err := wc.DecodeCloudNodeProgress(exact.Body.Bytes())
		if exact.Code != http.StatusOK || err != nil || item != got ||
			item.RunID != sub.RunID || item.SubmissionState != "task_known" || item.ProviderStatus != "running" {
			t.Fatalf("page differs from exact stored progress: %+v, %d, %+v, %v", item, exact.Code, got, err)
		}
	}
	all := read(path, token)
	for _, private := range []string{"cloud-task", "fixed cloud prompt", "private-refresh", h.delegation.ConnectionID, "delegation_id", "assignment", "submission_id"} {
		if strings.Contains(all.Body.String(), private) {
			t.Fatalf("progress page exposed private %q", private)
		}
	}
	other := f.call("POST", "/projects", map[string]any{"title": "Other progress project", "idempotency_key": newID()}, http.StatusCreated)["id"].(string)
	if page := readCloudProgressPage(t, read(cloudProgressListPath(other), "")); page.ProjectID != other || len(page.Items) != 0 {
		t.Fatalf("scope disclosed another project's submissions: %+v", page)
	}
	for _, tc := range []struct {
		method, path, token string
		status              int
	}{
		{http.MethodGet, "/workbuddy-cloud-progress", token, http.StatusBadRequest},
		{http.MethodGet, path + "&project_id=" + f.project, token, http.StatusBadRequest},
		{http.MethodGet, path + "&limit=0", token, http.StatusBadRequest},
		{http.MethodGet, path + "&limit=21", token, http.StatusBadRequest},
		{http.MethodGet, path + "&limit=1&limit=2", token, http.StatusBadRequest},
		{http.MethodGet, path + "&before=", token, http.StatusBadRequest},
		{http.MethodGet, path + "&before=bad", token, http.StatusBadRequest},
		{http.MethodGet, path + "&unused=x", token, http.StatusBadRequest},
		{http.MethodGet, cloudProgressListPath(other) + continuation, "", http.StatusBadRequest},
		{http.MethodGet, cloudProgressListPath(other), token, http.StatusForbidden},
		{http.MethodGet, path, agent, http.StatusForbidden},
		{http.MethodGet, path, h.claimToken, http.StatusForbidden},
		{http.MethodPost, path, token, http.StatusMethodNotAllowed},
	} {
		if w := cloudProgressRequest(h, tc.method, tc.path, tc.token); w.Code != tc.status {
			t.Fatalf("%s %s: got %d want %d: %s", tc.method, tc.path, w.Code, tc.status, w.Body.String())
		}
	}

	owner := f.cookie
	const email = "workbuddy-progress-viewer@example.test"
	member := governanceAccept(f, governanceInvite(f, email, "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}}), email)
	viewer := f.cookie
	page := readCloudProgressPage(t, read(path, ""))
	if len(page.Items) != 2 {
		t.Fatal("current viewer could not read the shared saved progress")
	}
	if response := read(path+continuation, ""); response.Code != http.StatusBadRequest {
		t.Fatalf("another reader reused the original cursor: %d", response.Code)
	}
	f.cookie = owner
	if _, err := f.s.SaveProjectAccess(context.Background(), f.actor, map[string]any{
		"project_id": f.project, "user_id": member["user"].(map[string]any)["id"], "action": "remove",
		"expected_version": float64(1), "idempotency_key": newID(),
	}); err != nil {
		t.Fatal(err)
	}
	f.cookie = viewer
	if response := read(path, ""); response.Code != http.StatusForbidden {
		t.Fatalf("removed viewer kept reading saved progress: %d", response.Code)
	}
	f.cookie = owner

	second.RequestSHA256 = strings.Repeat("0", 64)
	if _, err := f.s.DB.Exec("UPDATE suma_workbuddy_cloud_submissions SET payload=? WHERE id=?", stringify(second), second.ID); err != nil {
		t.Fatal(err)
	}
	if response := read(path, token); response.Code != http.StatusConflict || strings.Contains(response.Body.String(), "items") {
		t.Fatalf("corrupt source produced a partial list: %d %s", response.Code, response.Body.String())
	}
	if h.creates.Load() != 2 || h.queries.Load() != 0 {
		t.Fatalf("progress reads contacted provider: %d creates, %d queries", h.creates.Load(), h.queries.Load())
	}
}
