package platform

import (
	"net/url"
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
