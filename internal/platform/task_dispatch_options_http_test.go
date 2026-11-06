package platform

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func TestTaskDispatchOptionsHTTPCurrentScopedReadAndExactQuery(t *testing.T) {
	f, _, preview, task := taskDispatchFixture(t)
	q := url.Values{"project_id": {f.project}, "expected_task_version": {"1"}, "workflow_id": {preview["workflow_id"].(string)}, "workflow_version": {"1"}, "agent_node_id": {"develop"}, "limit": {"1"}}
	clone := func() url.Values {
		out := url.Values{}
		for k, v := range q {
			out[k] = append([]string{}, v...)
		}
		return out
	}
	path := "/tasks/" + task["id"].(string) + "/dispatch-options"
	plans, runs := expertCount(t, f, "suma_task_dispatch_plans"), expertCount(t, f, "dag_runs")
	out := f.call("GET", path+"?"+q.Encode(), nil, 200)
	items := out["items"].([]any)
	if len(items) != 1 || out["task_id"] != task["id"] || out["recommended_tool_profile_id"] != preview["tool_profile_id"] || items[0].(map[string]any)["model_state"] != "fixed_by_tool" || items[0].(map[string]any)["cost_state"] != "unknown" {
		t.Fatal(out)
	}
	for _, tail := range []string{"&project_id=" + f.project, "&unknown=1", "&limit=2"} {
		f.call("GET", path+"?"+q.Encode()+tail, nil, 400)
	}
	for key, value := range map[string]string{"expected_task_version": "1e0", "workflow_version": "1.0", "limit": "201", "before": "forged-cursor"} {
		bad := clone()
		bad.Set(key, value)
		f.call("GET", path+"?"+bad.Encode(), nil, 400)
	}
	stale := clone()
	stale.Set("expected_task_version", "2")
	f.call("GET", path+"?"+stale.Encode(), nil, 409)
	other := clone()
	other.Set("project_id", "unowned")
	f.call("GET", path+"?"+other.Encode(), nil, 404)
	if expertCount(t, f, "suma_task_dispatch_plans") != plans || expertCount(t, f, "dag_runs") != runs {
		t.Fatal("read created execution state")
	}
}

func TestTaskDispatchOptionsHTTPExactProfileRead(t *testing.T) {
	f, _, preview, task := taskDispatchFixture(t)
	id := preview["tool_profile_id"].(string)
	version := preview["expected_tool_version"].(int)
	path := "/tasks/" + task["id"].(string) + "/dispatch-options"
	q := url.Values{"project_id": {f.project}, "expected_task_version": {"1"}, "workflow_id": {preview["workflow_id"].(string)}, "workflow_version": {"1"}, "agent_node_id": {"develop"}, "tool_profile_id": {id}}
	read := func(status int) map[string]any { return f.call("GET", path+"?"+q.Encode(), nil, status) }
	assertProfile := func(state string, eligible bool, reason, recommended string) {
		t.Helper()
		out := read(200)
		items := out["items"].([]any)
		if len(items) != 1 || out["task_id"] != task["id"] || out["total"] != float64(1) ||
			out["next_cursor"] != "" || out["recommended_tool_profile_id"] != recommended {
			t.Fatal(out)
		}
		option := items[0].(map[string]any)
		tool := option["tool"].(map[string]any)
		if tool["id"] != id || tool["state"] != state || option["eligible"] != eligible || option["reason_code"] != reason {
			t.Fatal(option)
		}
	}
	assertProfile("enabled", true, "eligible", id)
	for key, value := range map[string]string{"limit": "1", "before": "wrong-cursor", "query": "native"} {
		q.Set(key, value)
		read(400)
		q.Del(key)
	}
	f.call("GET", path+"?"+q.Encode()+"&tool_profile_id="+id, nil, 400)
	q.Set("tool_profile_id", "bad/profile")
	read(400)
	q.Set("tool_profile_id", newID())
	read(404)
	q.Set("tool_profile_id", id)
	for i, change := range []struct{ action, state string }{{"pause", "paused"}, {"revoke", "revoked"}} {
		f.call("POST", "/tool-profiles/"+id+"/commands", f.body(map[string]any{
			"action": change.action, "expected_version": version + i,
		}), 200)
		assertProfile(change.state, false, "tool_unavailable", "")
	}
}

func TestTaskDispatchOptionsHTTPFiltersBeforePagingAndBindsSearch(t *testing.T) {
	f, _, preview, task := taskDispatchFixture(t)
	profileID := preview["tool_profile_id"].(string)
	ctx := context.Background()
	profile, err := f.s.GetToolProfile(ctx, f.actor, f.project, profileID)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Team alpha", "Team beta", "Other machine"} {
		alternative := profile.Profile
		alternative.ProfileSHA256 = digest(name)
		_, err = f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{
			ProjectID: f.project, WorkerID: profile.WorkerID, Name: name, Profile: alternative, IdempotencyKey: newID(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	counts := map[string]int{}
	for _, table := range []string{"suma_task_dispatch_plans", "dag_runs", "suma_usage_attempts"} {
		counts[table] = dispatchOptionsCount(t, f, table)
	}

	path := "/tasks/" + task["id"].(string) + "/dispatch-options"
	q := url.Values{"project_id": {f.project}, "expected_task_version": {"1"}, "workflow_id": {preview["workflow_id"].(string)}, "workflow_version": {"1"}, "agent_node_id": {"develop"}, "limit": {"1"}}
	read := func(status int) map[string]any { return f.call("GET", path+"?"+q.Encode(), nil, status) }
	assertTeamPage := func(result map[string]any, query string) string {
		t.Helper()
		items := result["items"].([]any)
		if result["query"] != query || result["total"] != float64(2) || len(items) != 1 || result["recommended_tool_profile_id"] != "" {
			t.Fatalf("filtered page did not preserve its search scope: %#v", result)
		}
		item := items[0].(map[string]any)
		name := item["tool"].(map[string]any)["name"].(string)
		if !strings.HasPrefix(name, "Team ") || item["reason_code"] != "tool_unavailable" {
			t.Fatalf("search escaped the filter or hid the real compatibility result: %#v", item)
		}
		return name
	}

	q.Set("query", " TEAM ")
	first := read(200)
	name := assertTeamPage(first, " TEAM ")
	cursor := first["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("filtered page did not return a continuation")
	}
	q.Set("before", cursor)
	q.Set("query", "team")
	second := read(200)
	if assertTeamPage(second, "team") == name || second["next_cursor"] != "" {
		t.Fatal("the same canonical search did not advance exactly one row", second)
	}
	q.Set("query", "other")
	read(400)
	q.Del("query")
	read(400)
	q.Del("before")

	for _, invalid := range []string{"", " \u3000 ", "local\n", strings.Repeat("测", 41), string([]byte{0xff})} {
		q.Set("query", invalid)
		read(400)
	}
	q.Set("query", "LOCAL NATIVE")
	f.call("GET", path+"?"+q.Encode()+"&query=team", nil, 400)
	current := read(200)
	if current["query"] != "LOCAL NATIVE" || current["total"] != float64(1) || current["recommended_tool_profile_id"] != profileID {
		t.Fatal("search did not recommend the matching eligible profile", current)
	}
	q.Set("query", profile.Profile.ProfileSHA256)
	empty := read(200)
	if empty["total"] != float64(0) || len(empty["items"].([]any)) != 0 || empty["next_cursor"] != "" {
		t.Fatal("search exposed a match from an unsearched field", empty)
	}
	q.Del("query")
	if _, present := read(200)["query"]; present {
		t.Fatal("unfiltered reads should omit the query")
	}
	for table, before := range counts {
		if dispatchOptionsCount(t, f, table) != before {
			t.Fatal("search changed execution or accounting state", table)
		}
	}
}
