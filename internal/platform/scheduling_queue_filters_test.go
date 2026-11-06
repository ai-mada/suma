package platform

import (
	"context"
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

func TestSchedulingQueueExactFrozenToolProfileFilter(t *testing.T) {
	f, _, profile := toolFixture(t)
	const sameCommandOtherProfile = "other-registered-shape"
	const foreignOnlyProfile = "foreign-tool-fixture"
	command := profile.Profile.CommandID
	bound := func(id, profileID, sha string) dag.Node {
		return dag.Node{ID: id, Type: "agent", Command: []string{command}, Config: map[string]any{
			"tool_profile_id": profileID, "tool_profile_sha256": sha,
		}}
	}
	unbound := func(id string) dag.Node {
		return dag.Node{ID: id, Type: "command", Command: []string{command}}
	}
	start := func(project, name string, nodes ...dag.Node) string {
		t.Helper()
		workflow := f.call("POST", "/workflows", map[string]any{"project_id": project, "name": name, "nodes": nodes, "idempotency_key": newID()}, 201)
		return f.call("POST", "/runs", map[string]any{"project_id": project, "workflow_id": workflow["id"], "workflow_version": 1, "idempotency_key": newID()}, 201)["id"].(string)
	}

	sha := profile.Profile.ProfileSHA256
	projectRun := start(f.project, "Mixed tool references",
		bound("first", profile.ID, sha),
		bound("other", sameCommandOtherProfile, sha),
		unbound("unbound"),
		bound("changed", profile.ID, strings.Repeat("c", 64)),
	)
	_, grant := dispatchAuthorizedPool(t, f)
	dispatchSetRoute(t, f, 0, &grant)
	poolRun := start(f.project, "Pool tool reference", bound("pool", profile.ID, sha))
	foreign := f.call("POST", "/projects", map[string]any{"title": "Other tool queue project", "idempotency_key": newID()}, 201)["id"].(string)
	start(foreign, "Other project references", bound("same", profile.ID, sha), bound("foreign", foreignOnlyProfile, sha))

	base := "/scheduling/queue?project_id=" + f.project
	read := func(suffix, tool string, total int, runIDs ...string) dag.ReadyPage {
		t.Helper()
		page := issueRepairRuntimeDecode[dag.ReadyPage](t, f.call("GET", base+suffix+"&tool_profile_id="+url.QueryEscape(tool), nil, 200))
		if page.Total != total || len(page.Items) != len(runIDs) {
			t.Fatalf("wrong filtered queue for %s, %s: %+v", suffix, tool, page)
		}
		for i, row := range page.Items {
			if row.RunID != runIDs[i] || row.Admission == nil || row.Admission.ToolBinding == nil ||
				row.Admission.ToolBinding.ProfileID != tool || row.CommandID != command {
				t.Fatalf("unrelated or unbound row in filtered queue: %+v", row)
			}
		}
		return page
	}
	readsBefore := historyDatabaseHash(t, f.s.DB)
	read("", profile.ID, 3, projectRun, projectRun, poolRun)
	read("", sameCommandOtherProfile, 1, projectRun)
	read("", foreignOnlyProfile, 0)
	read("", strings.ToUpper(sameCommandOtherProfile), 0)
	read("&route=pool", profile.ID, 1, poolRun)
	read("&route=pool&run_id="+projectRun, profile.ID, 0)

	scope := "&route=project&run_id=" + projectRun + "&limit=1"
	first := read(scope, profile.ID, 2, projectRun)
	if first.NextCursor == "" {
		t.Fatal("tool filter lost the next page")
	}
	cursor := "&cursor=" + url.QueryEscape(first.NextCursor)
	second := read(scope+cursor, profile.ID, 2, projectRun)
	if second.NextCursor != "" || second.Items[0].NodeID == first.Items[0].NodeID {
		t.Fatalf("cursor did not advance within exact tool scope: %+v", second)
	}
	for _, suffix := range []string{
		"&tool_profile_id=", "&tool_profile_id=%00", "&tool_profile_id=%2Foutside",
		"&tool_profile_id=" + strings.Repeat("a", 101),
		"&tool_profile_id=" + profile.ID + "&tool_profile_id=" + profile.ID,
		scope + cursor, scope + cursor + "&tool_profile_id=" + sameCommandOtherProfile,
		"&tool_profile_id=" + profile.ID + cursor,
	} {
		f.call("GET", base+suffix, nil, 400)
	}
	f.call("GET", "/scheduling/queue?project_id="+foreign+scope+cursor+"&tool_profile_id="+profile.ID, nil, 400)
	f.call("GET", "/scheduling/queue?project_id=foreign-project&tool_profile_id="+profile.ID, nil, 404)
	if after := historyDatabaseHash(t, f.s.DB); readsBefore != after {
		t.Fatal("tool filter changed durable state")
	}

	profile, err := f.s.CommandToolProfile(context.Background(), f.actor, f.project, profile.ID, ToolProfileCommandRequest{
		ProjectID: f.project, Action: "revoke", ExpectedVersion: profile.Version, IdempotencyKey: newID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	revoked := read("&run_id="+projectRun, profile.ID, 2, projectRun, projectRun)
	blocked := map[string]string{}
	for _, row := range revoked.Items {
		for _, reason := range row.Admission.Blockers {
			if reason.Source == "tool" {
				blocked[row.NodeID] = reason.Code
			}
		}
	}
	if blocked["first"] != "tool_profile_revoked" || blocked["changed"] != "tool_profile_changed" {
		t.Fatalf("frozen tool diagnoses were lost or substituted: %+v", blocked)
	}

	// Inject corrupt source independently of the product creation path. A
	// numeric value, incomplete digest, and invalid JSON are not frozen IDs.
	corruptRun := start(f.project, "Malformed binding storage fixture", unbound("number"), unbound("digest"), unbound("json"))
	for id, raw := range map[string]string{
		"number": `{"id":"number","type":"command","command":["native-agent"],"config":{"tool_profile_id":42,"tool_profile_sha256":"` + sha + `"}}`,
		"digest": `{"id":"digest","type":"command","command":["native-agent"],"config":{"tool_profile_id":"` + profile.ID + `","tool_profile_sha256":null}}`,
		"json":   `{`,
	} {
		if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET spec=? WHERE run_id=? AND id=?`, raw, corruptRun, id); err != nil {
			t.Fatal(err)
		}
	}
	read("&run_id="+corruptRun, "42", 0)
	read("&run_id="+corruptRun, profile.ID, 0)
}
