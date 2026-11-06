package platform

import (
	"net/url"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func TestSchedulingQueueFrozenRouteAndExactRunFilters(t *testing.T) {
	f := newFixture(t)
	start := func(project, name string, nodes int) string {
		t.Helper()
		var definition []dag.Node
		for _, node := range []string{"first", "second"}[:nodes] {
			definition = append(definition, dag.Node{ID: node, Type: "command", Command: []string{"inspect"}})
		}
		workflow := f.call("POST", "/workflows", map[string]any{"project_id": project, "name": name, "nodes": definition, "idempotency_key": newID()}, 201)
		run := f.call("POST", "/runs", map[string]any{"project_id": project, "workflow_id": workflow["id"], "workflow_version": 1, "idempotency_key": newID()}, 201)
		return run["id"].(string)
	}

	projectRun := start(f.project, "Original project route", 2)
	pool, grant := dispatchAuthorizedPool(t, f)
	dispatchSetRoute(t, f, 0, &grant)
	poolRun := start(f.project, "Original shared route", 2)
	dispatchSetRoute(t, f, 1, nil)
	secondProjectRun := start(f.project, "Restored project route", 1)
	foreignProject := f.call("POST", "/projects", map[string]any{"title": "Foreign queue filter fixture", "idempotency_key": newID()}, 201)["id"].(string)
	foreignRun := start(foreignProject, "Other project run", 1)
	f.call("POST", "/execution-pools/"+pool.ID+"/commands", map[string]any{"action": "pause", "expected_version": pool.Version, "idempotency_key": newID()}, 200)

	counts := map[string]int{}
	for _, table := range []string{"suma_operations", "dag_runs", "dag_events", "dag_ready", "dag_attempts", "dag_resource_ledger"} {
		counts[table] = expertCount(t, f, table)
	}
	base := "/scheduling/queue?project_id=" + f.project
	read := func(suffix string, total int, runIDs ...string) map[string]any {
		t.Helper()
		page := f.call("GET", base+suffix, nil, 200)
		items := page["items"].([]any)
		if page["total"] != float64(total) || len(items) != len(runIDs) {
			t.Fatalf("wrong filtered scope for %s: %#v", suffix, page)
		}
		for i, item := range items {
			if row := item.(map[string]any); row["run_id"] != runIDs[i] {
				t.Fatalf("unexpected row at %d for %s: %#v", i, suffix, row)
			}
		}
		return page
	}

	read("&route=project", 3, projectRun, projectRun, secondProjectRun)
	first := read("&route=pool&limit=1", 2, poolRun)
	next, ok := first["next_cursor"].(string)
	if !ok || next == "" {
		t.Fatal("filtered first page lost its cursor")
	}
	firstItem := first["items"].([]any)[0].(map[string]any)
	admission := firstItem["admission"].(map[string]any)
	if admission["execution_pool_id"] != pool.ID || admission["status"] != "blocked" {
		t.Fatal("pool filtering altered current admission diagnostics")
	}
	second := read("&route=pool&limit=1&cursor="+url.QueryEscape(next), 2, poolRun)
	if second["next_cursor"] != nil || second["items"].([]any)[0].(map[string]any)["node_id"] == firstItem["node_id"] {
		t.Fatal("filtered cursor did not advance to the remaining ready node")
	}
	combined := read("&route=project&run_id="+projectRun+"&limit=1", 2, projectRun)
	combinedCursor := url.QueryEscape(combined["next_cursor"].(string))
	read("&route=project&run_id="+projectRun+"&limit=1&cursor="+combinedCursor, 2, projectRun)
	read("&route=project&run_id="+poolRun, 0)
	read("&run_id="+poolRun, 2, poolRun, poolRun)
	read("&run_id="+foreignRun, 0)
	read("&run_id=missing-run", 0)
	if upper := strings.ToUpper(poolRun); upper != poolRun {
		read("&run_id="+upper, 0)
	}

	all := read("&limit=1", 5, projectRun)
	allCursor := url.QueryEscape(all["next_cursor"].(string))
	read("&limit=1&cursor="+allCursor, 5, projectRun)
	for _, suffix := range []string{
		"&route=all", "&route=", "&route=POOL", "&route=project&route=pool", "&route=%zz",
		"&run_id=", "&run_id=/outside", "&run_id=%00", "&run_id=" + strings.Repeat("a", 101),
		"&run_id=" + poolRun + "&run_id=" + poolRun,
		"&route=pool&cursor=" + allCursor,
		"&route=project&cursor=" + url.QueryEscape(next),
		"&cursor=" + url.QueryEscape(next),
		"&route=project&run_id=" + secondProjectRun + "&cursor=" + combinedCursor,
		"&run_id=" + projectRun + "&cursor=" + combinedCursor,
	} {
		f.call("GET", base+suffix, nil, 400)
	}
	f.call("GET", "/scheduling/queue?project_id=foreign-project&route=pool&run_id="+poolRun, nil, 404)
	f.call("GET", "/scheduling/queue?project_id="+foreignProject+"&route=project&run_id="+projectRun+"&cursor="+combinedCursor, nil, 400)
	for table, before := range counts {
		if after := expertCount(t, f, table); before != after {
			t.Fatalf("queue query mutated %s from %d to %d", table, before, after)
		}
	}
}
