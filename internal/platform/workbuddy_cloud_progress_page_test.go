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
	page, err := wc.DecodeCloudNodeProgressPage(response.Body.Bytes())
	if response.Code != http.StatusOK || err != nil {
		t.Fatalf("invalid progress page: %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("progress page lacks privacy headers")
	}
	expectedFields := 2
	if page.NextCursor != "" {
		expectedFields++
	}
	if page.Filter != "" {
		expectedFields++
	}
	if len(page.TaskLinks) > 0 {
		expectedFields++
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil || len(fields) != expectedFields ||
		fields["items"] == nil || page.Items == nil || fields["project_id"] == nil || (fields["next_cursor"] != nil) != (page.NextCursor != "") ||
		(fields["filter"] != nil) != (page.Filter != "") || (fields["task_links"] != nil) != (len(page.TaskLinks) > 0) {
		t.Fatal("progress page contains fields outside the public contract")
	}
	return page
}

func TestWorkBuddyCloudProgressPageLocatesFormalTaskDelivery(t *testing.T) {
	h := newWorkBuddyTaskDispatchFixtureWithCreateStatus(t, "running")
	f := h.f
	f.handler = (&Server{Service: f.s, Engine: f.e, WorkBuddy: h.w}).Handler()
	assignment, _ := workBuddyTaskParkAtProviderWait(t, h)
	cloud := &cloudFixture{f: f, w: h.w}
	reader := cloudProgressToken(t, cloud, "api")
	path := cloudProgressListPath(f.project)
	read := func(path string) *httptest.ResponseRecorder {
		return cloudProgressRequest(cloud, http.MethodGet, path, reader)
	}
	creates, queries := h.creates.Load(), h.queries.Load()
	assertLink := func(relation, title string, version int) {
		t.Helper()
		page := readCloudProgressPage(t, read(path+"&filter=attention"))
		want := wc.CloudProgressTaskLink{RunID: assignment.RunID, NodeID: assignment.NodeID, TaskID: h.task.ID,
			TaskVersion: version, RunRelation: relation, TaskTitle: title}
		if len(page.Items) != 1 || page.Items[0].RunID != assignment.RunID || page.Items[0].NodeID != assignment.NodeID ||
			len(page.TaskLinks) != 1 || page.TaskLinks[0] != want {
			t.Fatalf("formal task locator differs from current task and saved progress: %+v", page)
		}
		exact := read("/runs/" + assignment.RunID + "/nodes/" + assignment.NodeID + "/workbuddy-status?project_id=" + f.project)
		progress, err := wc.DecodeCloudNodeProgress(exact.Body.Bytes())
		if exact.Code != http.StatusOK || err != nil || progress != page.Items[0] {
			t.Fatalf("exact node progress changed shape or facts: %d %v", exact.Code, err)
		}
	}
	assertLink("current", h.task.Title, h.task.Version+1)
	const renamed = "团队交付任务"
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET run_id=?,title=?,version=version+1 WHERE tenant_id=? AND project_id=? AND id=?`, newID(), renamed, f.user.TenantID, f.project, h.task.ID); err != nil {
		t.Fatal(err)
	}
	assertLink("changed", renamed, h.task.Version+2)
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET run_id='',title=?,version=version+1 WHERE tenant_id=? AND project_id=? AND id=?`, "unsafe\x01title", f.user.TenantID, f.project, h.task.ID); err != nil {
		t.Fatal(err)
	}
	assertLink("unbound", "", h.task.Version+3)

	if _, err := f.s.DB.Exec(`DELETE FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`, f.user.TenantID, f.project, h.task.ID); err != nil {
		t.Fatal(err)
	}
	if page := readCloudProgressPage(t, read(path)); len(page.Items) != 1 || len(page.TaskLinks) != 0 {
		t.Fatalf("deleted task changed saved progress or retained a link: %+v", page)
	}

	// A valid request can still be attached to a node specification that differs
	// from the formal dispatch. An attention page only resolves its returned rows.
	var spec string
	if err := f.s.DB.QueryRow(`SELECT spec FROM dag_nodes WHERE run_id=? AND id=?`, assignment.RunID, assignment.NodeID).Scan(&spec); err != nil {
		t.Fatal(err)
	}
	var changedSpec map[string]any
	if err := json.Unmarshal([]byte(spec), &changedSpec); err != nil {
		t.Fatal(err)
	}
	changedSpec["name"] = "Changed after dispatch"
	if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET spec=?,reason='' WHERE run_id=? AND id=?`, stringify(changedSpec), assignment.RunID, assignment.NodeID); err != nil {
		t.Fatal(err)
	}
	if page := readCloudProgressPage(t, read(path+"&filter=attention")); len(page.Items) != 0 || len(page.TaskLinks) != 0 {
		t.Fatalf("filtered row unexpectedly contributed task facts: %+v", page)
	}
	if response := read(path); response.Code != http.StatusConflict {
		t.Fatalf("corrupted formal binding returned %d: %s", response.Code, response.Body.String())
	}
	if h.creates.Load() != creates || h.queries.Load() != queries {
		t.Fatal("progress reads contacted WorkBuddy")
	}
}

func cloudProgressNextSubmission(t *testing.T, h *cloudFixture) wc.CloudSubmission {
	t.Helper()
	return cloudProgressCreateFollowingSubmission(t, h, false)
}

func cloudProgressParkSubmission(t *testing.T, h *cloudFixture, submission wc.CloudSubmission) {
	t.Helper()
	a, r := cloudResultFixture(t, h)
	r.WorkBuddyCloudWait.SubmissionID = submission.ID
	r.WorkBuddyCloudWait.SubmissionVersion = submission.Version
	r.WorkBuddyCloudWait.Cause = "observation_unconfirmed"
	if err := completeCloudFixture(t, h, a, r); err != nil {
		t.Fatal(err)
	}
}

func cloudProgressCreateFollowingSubmission(t *testing.T, h *cloudFixture, park bool) wc.CloudSubmission {
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
	request := wc.CloudCreateRequest{
		ProjectID: f.project, DelegationID: delegation.ID, ExpectedVersion: delegation.Version,
		Assignment: repositoryTestRef(*a), IdempotencyKey: newID(),
	}
	sub, err := h.w.CreateCloudTask(ctx, h.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	if park {
		original := h.request
		h.request = request
		defer func() { h.request = original }()
		cloudProgressParkSubmission(t, h, sub)
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
	cloudProgressParkSubmission(t, h, first)
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
	if len(firstPage.TaskLinks)+len(secondPage.TaskLinks) != 0 {
		t.Fatal("manual workflow submissions acquired formal task locators")
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

func TestWorkBuddyCloudProgressAttentionContinuesAcrossScanBoundary(t *testing.T) {
	h := newCloudFixture(t, nil)
	f := h.f
	f.handler = (&Server{Service: f.s, Engine: f.e, WorkBuddy: h.w}).Handler()
	token := cloudProgressToken(t, h, "api")
	base := cloudProgressListPath(f.project)
	read := func(path string) *httptest.ResponseRecorder {
		return cloudProgressRequest(h, http.MethodGet, path, token)
	}

	filtered := base + "&filter=attention&limit=1"
	empty := readCloudProgressPage(t, read(filtered))
	if empty.Filter != "attention" || len(empty.Items) != 0 || empty.NextCursor != "" {
		t.Fatalf("empty project unexpectedly contains saved progress: %+v", empty)
	}

	const candidates = 43
	subs := make([]wc.CloudSubmission, 0, candidates)
	firstSubmission := createCloudFixture(t, h)
	subs = append(subs, firstSubmission)
	cloudProgressParkSubmission(t, h, firstSubmission)
	for len(subs) < candidates {
		subs = append(subs, cloudProgressCreateFollowingSubmission(t, h, true))
	}
	if _, err := f.s.DB.Exec("UPDATE dag_nodes SET reason='' WHERE run_id IN (SELECT run_id FROM suma_workbuddy_cloud_submissions WHERE tenant_id=? AND project_id=?)", f.user.TenantID, f.project); err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(subs, func(a, b wc.CloudSubmission) int { return strings.Compare(a.ID, b.ID) })
	subs[40].Task.Status = "waiting_input"
	subs[41].LastError = "workbuddy_cloud_upstream"
	for _, i := range []int{40, 41} {
		if _, err := f.s.DB.Exec("UPDATE suma_workbuddy_cloud_submissions SET payload=? WHERE id=?", stringify(subs[i]), subs[i].ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.s.DB.Exec("UPDATE dag_nodes SET reason=? WHERE run_id=? AND id=?", "cloud.delivery_required", subs[42].RunID, subs[42].NodeID); err != nil {
		t.Fatal(err)
	}

	first := readCloudProgressPage(t, read(filtered))
	if len(first.Items) != 0 || first.NextCursor == "" {
		t.Fatalf("first forty inspected records should yield an empty page and continuation: %+v", first)
	}
	all := readCloudProgressPage(t, read(base+"&limit=1"))
	if all.Filter != "" || all.NextCursor == "" || len(all.Items) != 1 {
		t.Fatal("unfiltered page lost its first record or continuation")
	}
	for _, path := range []string{
		base + "&filter=",
		base + "&filter=all",
		base + "&filter=attention&filter=attention",
		base + "&before=" + url.QueryEscape(first.NextCursor),
		filtered + "&before=" + url.QueryEscape(all.NextCursor),
	} {
		if response := read(path); response.Code != http.StatusBadRequest {
			t.Fatalf("malformed filter or cross-filter cursor accepted: %d", response.Code)
		}
	}

	cursor := first.NextCursor
	for i, sub := range subs[40:] {
		page := readCloudProgressPage(t, read(filtered+"&before="+url.QueryEscape(cursor)))
		if page.Filter != "attention" {
			t.Fatal("page did not confirm its exact filter")
		}
		if len(page.Items) != 1 || page.Items[0].RunID != sub.RunID || page.Items[0].NodeID != sub.NodeID {
			t.Fatalf("expected matching projected record at offset %d, got %+v", i, page)
		}
		item := page.Items[0]
		switch i {
		case 0:
			if item.ProviderStatus != "waiting_input" || item.NodeReason != "" {
				t.Fatal("supplier observation alone should yield a match", item)
			}
		case 1:
			if !item.ObservationFailed {
				t.Fatal("latest failed observation should yield a match", item)
			}
		case 2:
			if item.NodeReason != "cloud.delivery_required" {
				t.Fatal("platform delivery reason should yield a match", item)
			}
		}
		if page.NextCursor == cursor || (page.NextCursor != "") != (i < 2) {
			t.Fatalf("continuation must advance only when additional records remain: %+v", page)
		}
		cursor = page.NextCursor
	}
	if h.creates.Load() != candidates || h.queries.Load() != 0 {
		t.Fatalf("filtered reads contacted provider: %d creates, %d queries", h.creates.Load(), h.queries.Load())
	}
}
