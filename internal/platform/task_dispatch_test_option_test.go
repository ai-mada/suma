package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func taskDispatchTestOptionQuery(req TaskDispatchPreviewRequest) url.Values {
	test := req.TestBindings[0]
	return url.Values{
		"project_id": {req.ProjectID}, "expected_task_version": {strconv.Itoa(req.ExpectedTaskVersion)},
		"workflow_id": {req.WorkflowID}, "workflow_version": {strconv.Itoa(req.WorkflowVersion)}, "agent_node_id": {req.AgentNodeID},
		"development_tool_profile_id": {req.ToolProfileID}, "development_tool_profile_version": {strconv.Itoa(req.ExpectedToolVersion)},
		"test_node_id": {test.NodeID}, "test_tool_profile_id": {test.ToolProfileID}, "test_tool_profile_version": {strconv.Itoa(test.ExpectedToolVersion)},
	}
}

func TestTaskDispatchTestOptionReturnsExactBindingWithoutWriting(t *testing.T) {
	f, request, profile := dispatchTestBindingsFixture(t)
	var original string
	if err := f.s.DB.QueryRow(`SELECT spec FROM dag_workflows WHERE id=? AND version=?`, request.WorkflowID, request.WorkflowVersion).Scan(&original); err != nil {
		t.Fatal(err)
	}
	before := make(map[string]int)
	for _, table := range []string{"suma_task_dispatch_plans", "dag_runs", "suma_usage_attempts"} {
		before[table] = dispatchOptionsCount(t, f, table)
	}
	q := taskDispatchTestOptionQuery(request)
	path := "/tasks/" + request.TaskID + "/dispatch-test-option?" + q.Encode()
	got := f.call(http.MethodGet, path, nil, http.StatusOK)
	want := TaskDispatchTestBinding{NodeID: "test", ToolProfileID: profile.ID, ToolProfileVersion: profile.Version, ToolProfileSHA256: profile.Profile.ProfileSHA256, WorkerID: profile.WorkerID, CommandID: profile.Profile.CommandID}
	if got["task_id"] != request.TaskID || got["task_version"] != float64(request.ExpectedTaskVersion) ||
		got["project_id"] != f.project || got["workflow_id"] != request.WorkflowID || got["workflow_version"] != float64(request.WorkflowVersion) ||
		got["agent_node_id"] != request.AgentNodeID || got["development_tool_profile_id"] != request.ToolProfileID ||
		got["development_tool_profile_version"] != float64(request.ExpectedToolVersion) ||
		stringify(got["binding"]) != stringify(normalizeTestOptionBinding(t, want)) {
		t.Fatalf("incorrect dispatch test option: %v", got)
	}
	if checkedAt, ok := got["checked_at"].(string); !ok || checkedAt == "" {
		t.Fatal("missing server check time", got)
	} else if _, err := time.Parse(time.RFC3339Nano, checkedAt); err != nil {
		t.Fatal(err)
	}
	if route := got["execution_route"].(map[string]any); route["project_id"] != f.project || route["tenant_id"] != f.user.TenantID || route["pool_id"] != "" {
		t.Fatal("incorrect read scope", route)
	}
	cacheRequest := httptest.NewRequest(http.MethodGet, "http://suma.test/api/v1"+path, nil)
	cacheRequest.AddCookie(f.cookie)
	cacheResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(cacheResponse, cacheRequest)
	if cacheResponse.Code != http.StatusOK || cacheResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("preflight read can be cached", cacheResponse.Code, cacheResponse.Header())
	}
	for table, count := range before {
		if dispatchOptionsCount(t, f, table) != count {
			t.Fatal("test preflight created execution or accounting records", table)
		}
	}
	var after string
	if err := f.s.DB.QueryRow(`SELECT spec FROM dag_workflows WHERE id=? AND version=?`, request.WorkflowID, request.WorkflowVersion).Scan(&after); err != nil || after != original {
		t.Fatal("test preflight changed the published workflow", err)
	}
	plan, err := f.s.CreateTaskDispatchPlan(context.Background(), f.e, f.actor, f.project, request)
	if err != nil || len(plan.TestBindings) != 1 || plan.TestBindings[0] != want ||
		got["development_tool_profile_sha256"] != plan.ToolProfileSHA256 {
		t.Fatal("preview and preflight resolved different execution inputs", plan.TestBindings, err)
	}
}

func normalizeTestOptionBinding(t *testing.T, binding TaskDispatchTestBinding) map[string]any {
	t.Helper()
	return map[string]any{
		"node_id": binding.NodeID, "tool_profile_id": binding.ToolProfileID, "tool_profile_version": float64(binding.ToolProfileVersion),
		"tool_profile_sha256": binding.ToolProfileSHA256, "worker_id": binding.WorkerID, "command_id": binding.CommandID,
	}
}

func TestTaskDispatchTestOptionRejectsBadQueryDriftAndUnauthorizedReaders(t *testing.T) {
	f, request, profile := dispatchTestBindingsFixture(t)
	base := "/tasks/" + request.TaskID + "/dispatch-test-option?"
	for _, scenario := range []struct {
		name, key, value string
		status           int
	}{
		{"unknown parameter", "scope", "other", http.StatusBadRequest},
		{"noncanonical version", "workflow_version", "01", http.StatusBadRequest},
		{"out of range version", "development_tool_profile_version", "2147483647", http.StatusBadRequest},
		{"stale task", "expected_task_version", strconv.Itoa(request.ExpectedTaskVersion + 1), http.StatusConflict},
		{"stale development configuration", "development_tool_profile_version", strconv.Itoa(request.ExpectedToolVersion + 1), http.StatusConflict},
		{"stale test configuration", "test_tool_profile_version", strconv.Itoa(profile.Version + 1), http.StatusConflict},
		{"unpublished version", "workflow_version", strconv.Itoa(request.WorkflowVersion + 1), http.StatusNotFound},
		{"wrong test target", "test_node_id", request.AgentNodeID, http.StatusBadRequest},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			q := taskDispatchTestOptionQuery(request)
			q.Set(scenario.key, scenario.value)
			f.call(http.MethodGet, base+q.Encode(), nil, scenario.status)
		})
	}
	q := taskDispatchTestOptionQuery(request)
	q.Add("test_node_id", "test")
	f.call(http.MethodGet, base+q.Encode(), nil, http.StatusBadRequest)
	q.Del("test_node_id")
	f.call(http.MethodGet, base+q.Encode(), nil, http.StatusBadRequest)
	q.Set("test_node_id", "")
	f.call(http.MethodGet, base+q.Encode(), nil, http.StatusBadRequest)

	path := base + taskDispatchTestOptionQuery(request).Encode()
	token := f.call(http.MethodPost, "/auth/tokens", f.body(map[string]any{"scope": "api"}), http.StatusCreated)["token"].(string)
	f.bearer(http.MethodGet, path, token, nil, http.StatusForbidden)

	invitation := governanceInvite(f, "test-preflight-viewer@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	governanceAccept(f, invitation, "test-preflight-viewer@example.test")
	f.call(http.MethodGet, "/tasks/"+request.TaskID+"?project_id="+f.project, nil, http.StatusOK)
	f.call(http.MethodGet, path, nil, http.StatusForbidden)
}

func TestTaskDispatchTestOptionRequiresAnAvailableIndependentTestOnTheCurrentRoute(t *testing.T) {
	f, request, profile := dispatchTestBindingsFixture(t)
	base := "/tasks/" + request.TaskID + "/dispatch-test-option?"
	path := base + taskDispatchTestOptionQuery(request).Encode()
	dev, err := f.s.GetToolProfile(context.Background(), f.actor, f.project, request.ToolProfileID)
	if err != nil {
		t.Fatal(err)
	}

	// Preserve the test protocol while putting the test on the same Worker.
	// A different tool ID is not sufficient to achieve independence.
	if _, err := f.s.DB.Exec(`UPDATE suma_tool_profiles SET worker_id=? WHERE id=?`, dev.WorkerID, profile.ID); err != nil {
		t.Fatal(err)
	}
	f.call(http.MethodGet, path, nil, http.StatusBadRequest)
	if _, err := f.s.DB.Exec(`UPDATE suma_tool_profiles SET worker_id=? WHERE id=?`, profile.WorkerID, profile.ID); err != nil {
		t.Fatal(err)
	}

	noReport := dispatchTestBindingsProfile(t, f, "preflight-no-report", "no-junit", "")
	q := taskDispatchTestOptionQuery(request)
	q.Set("test_tool_profile_id", noReport.ID)
	q.Set("test_tool_profile_version", strconv.Itoa(noReport.Version))
	f.call(http.MethodGet, base+q.Encode(), nil, http.StatusBadRequest)

	// Route only the development tool into the currently approved pool. Both
	// observations are individually available; the local tester is unreachable.
	pool, grant := dispatchAuthorizedPool(t, f)
	poolDev := dispatchPoolTool(t, f, pool)
	dispatchSetRoute(t, f, 0, &grant)
	q = taskDispatchTestOptionQuery(request)
	q.Set("development_tool_profile_id", poolDev.ID)
	q.Set("development_tool_profile_version", strconv.Itoa(poolDev.Version))
	reply := f.call(http.MethodGet, base+q.Encode(), nil, http.StatusConflict)
	if issue := reply["error"].(map[string]any); issue["message"] != "所选工具的执行节点不属于当前目标资源池，请重新选择工具或执行路由" {
		t.Fatalf("expected the independent tester route conflict, got %v", reply)
	}
	dispatchSetRoute(t, f, 1, nil)

	if _, err := f.s.DB.Exec(`UPDATE suma_tool_inventories SET observed_at='2000-01-01T00:00:00Z' WHERE worker_id=?`, profile.WorkerID); err != nil {
		t.Fatal(err)
	}
	f.call(http.MethodGet, path, nil, http.StatusConflict)
	if dispatchOptionsCount(t, f, "suma_task_dispatch_plans") != 0 || dispatchOptionsCount(t, f, "dag_runs") != 0 {
		t.Fatal("rejected test preflight created execution state")
	}
}
