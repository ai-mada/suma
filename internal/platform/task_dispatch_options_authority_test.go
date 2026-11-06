package platform

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTaskDispatchOptionsAndTestOptionProjectAPIUseCurrentAuthority(t *testing.T) {
	f, preview, tester := dispatchTestBindingsFixture(t)
	owner := f.cookie
	q := taskDispatchTestOptionQuery(preview)
	testPath := "/tasks/" + preview.TaskID + "/dispatch-test-option"
	optionsPath := "/tasks/" + preview.TaskID + "/dispatch-options"
	readQuery := taskDispatchTestOptionQuery(preview)
	for _, field := range []string{"development_tool_profile_id", "development_tool_profile_version", "test_node_id", "test_tool_profile_id", "test_tool_profile_version"} {
		readQuery.Del(field)
	}
	other := f.call(http.MethodPost, "/projects", map[string]any{"title": "Different dispatch project", "idempotency_key": newID()}, http.StatusCreated)["id"].(string)
	otherOrganization := f.call(http.MethodPost, "/organizations", map[string]any{"name": "Different dispatch organization", "idempotency_key": newID()}, http.StatusCreated)["id"].(string)
	f.call(http.MethodPost, "/auth/switch", map[string]any{"organization_id": otherOrganization, "idempotency_key": newID()}, http.StatusOK)
	foreign := f.call(http.MethodPost, "/projects", map[string]any{"title": "Foreign dispatch project", "idempotency_key": newID()}, http.StatusCreated)["id"].(string)
	f.cookie = owner
	f.call(http.MethodGet, "/auth/session", nil, http.StatusOK)
	invitation := governanceInvite(f, "cli-dispatch-admin@example.test", "admin", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	member := governanceAccept(f, invitation, "cli-dispatch-admin@example.test")["user"].(map[string]any)["id"].(string)
	issue := func(kind string) string {
		return f.call(http.MethodPost, "/auth/tokens", f.body(map[string]any{"scope": kind}), http.StatusCreated)["token"].(string)
	}
	api, runner, agent := issue("api"), issue("runner"), issue("agent")
	before := make(map[string]int)
	for _, table := range []string{"suma_task_dispatch_plans", "dag_runs", "suma_usage_attempts"} {
		before[table] = dispatchOptionsCount(t, f, table)
	}
	read := func(t *testing.T, path string) map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "http://suma.test/api/v1"+path, nil)
		req.Header.Set("Authorization", "Bearer "+api)
		res := httptest.NewRecorder()
		f.handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK || res.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s got %d with cache policy %q: %s", path, res.Code, res.Header().Get("Cache-Control"), res.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	for _, path := range []string{optionsPath + "?" + readQuery.Encode(), testPath + "?" + q.Encode()} {
		f.call(http.MethodGet, path, nil, http.StatusOK)
	}
	t.Run("recommendations", func(t *testing.T) {
		got := read(t, optionsPath+"?"+readQuery.Encode())
		if got["task_id"] != preview.TaskID || got["recommended_tool_profile_id"] != preview.ToolProfileID {
			t.Fatal("API read resolved the wrong task or developer profile", got)
		}
	})
	t.Run("independent test", func(t *testing.T) {
		got := read(t, testPath+"?"+q.Encode())
		binding := got["binding"].(map[string]any)
		if got["task_id"] != preview.TaskID || binding["tool_profile_id"] != tester.ID || binding["worker_id"] != tester.WorkerID {
			t.Fatal("API preflight resolved the wrong task or independent test tool", got)
		}
	})

	t.Run("cursor belongs to current actor", func(t *testing.T) {
		memberCookie := f.cookie
		defer func() {
			f.cookie = memberCookie
			readQuery.Del("limit")
			readQuery.Del("before")
		}()
		readQuery.Set("limit", "1")
		first := read(t, optionsPath+"?"+readQuery.Encode())
		cursor, ok := first["next_cursor"].(string)
		if !ok || cursor == "" {
			t.Fatal("expected a continuation from the member's first page", first)
		}
		readQuery.Set("before", cursor)
		second := read(t, optionsPath+"?"+readQuery.Encode())
		if len(second["items"].([]any)) != 1 || second["next_cursor"] != "" {
			t.Fatal("member could not continue the original page", second)
		}
		f.cookie = owner
		f.call(http.MethodGet, optionsPath+"?"+readQuery.Encode(), nil, http.StatusBadRequest)
	})
	readQuery.Set("tool_profile_id", preview.ToolProfileID)
	exact := read(t, optionsPath+"?"+readQuery.Encode())
	if exact["total"] != float64(1) || exact["next_cursor"] != "" || exact["recommended_tool_profile_id"] != preview.ToolProfileID {
		t.Fatal("exact API preflight resolved the wrong profile", exact)
	}
	for _, blocked := range []string{runner, agent} {
		f.bearer(http.MethodGet, optionsPath+"?"+readQuery.Encode(), blocked, nil, http.StatusForbidden)
		f.bearer(http.MethodGet, testPath+"?"+q.Encode(), blocked, nil, http.StatusForbidden)
	}
	for _, target := range []string{other, foreign} {
		original := readQuery.Get("project_id")
		readQuery.Set("project_id", target)
		q.Set("project_id", target)
		f.bearer(http.MethodGet, optionsPath+"?"+readQuery.Encode(), api, nil, http.StatusForbidden)
		f.bearer(http.MethodGet, testPath+"?"+q.Encode(), api, nil, http.StatusForbidden)
		readQuery.Set("project_id", original)
		q.Set("project_id", original)
	}

	f.cookie = owner
	f.call(http.MethodGet, "/auth/session", nil, http.StatusOK)
	f.call(http.MethodPatch, "/organization/members/"+member, map[string]any{"role": "member", "expected_version": 1, "idempotency_key": newID()}, http.StatusOK)
	read(t, optionsPath+"?"+readQuery.Encode())
	f.bearer(http.MethodGet, testPath+"?"+q.Encode(), api, nil, http.StatusForbidden)
	f.call(http.MethodPost, "/project-access", f.body(map[string]any{"user_id": member, "action": "remove", "expected_version": 1}), http.StatusOK)
	for _, path := range []string{optionsPath + "?" + readQuery.Encode(), testPath + "?" + q.Encode()} {
		f.bearer(http.MethodGet, path, api, nil, http.StatusUnauthorized)
	}
	for table, count := range before {
		if dispatchOptionsCount(t, f, table) != count {
			t.Fatal("dispatch discovery wrote execution or usage state", table)
		}
	}
}
