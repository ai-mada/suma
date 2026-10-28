package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// The business dependency graph is a complete, versioned read contract. It must
// never be inferred from a paginated task list or confused with an execution DAG.
func TestTaskDependenciesHTTPCompleteVersionedGraph(t *testing.T) {
	f := newFixture(t)
	first := f.call("POST", "/tasks", f.body(map[string]any{"title": "First dependency task"}), 201)
	second := f.call("POST", "/tasks", f.body(map[string]any{"title": "Second dependency task", "depends_on": []string{first["id"].(string)}}), 201)
	out := f.call("GET", "/task-dependencies?project_id="+url.QueryEscape(f.project), nil, 200)
	if out["project_id"] != f.project || out["total"] != float64(2) || out["limit"] != float64(1000) || out["complete"] != true {
		t.Fatalf("unexpected graph envelope: %#v", out)
	}
	tasks, ok := out["tasks"].([]any)
	if !ok || len(tasks) != 2 {
		t.Fatalf("expected both project tasks: %#v", out)
	}
	found := map[string]map[string]any{}
	for _, raw := range tasks {
		task := raw.(map[string]any)
		found[task["task_id"].(string)] = task
		if task["version"] != float64(1) || task["title"] == "" || task["status"] == "" || task["depends_on"] == nil {
			t.Fatalf("incomplete graph task: %#v", task)
		}
	}
	deps := found[second["id"].(string)]["depends_on"].([]any)
	if len(deps) != 1 || deps[0] != first["id"] {
		t.Fatalf("unexpected dependency snapshot: %#v", deps)
	}
}

func taskDependencyHTTPBody(f *fixture, key string, changes ...map[string]any) map[string]any {
	return map[string]any{"project_id": f.project, "changes": changes, "idempotency_key": key}
}

func TestTaskDependenciesHTTPAtomicRewireAndOriginalReceipt(t *testing.T) {
	f := newFixture(t)
	a := f.call("POST", "/tasks", f.body(map[string]any{"title": "First"}), 201)
	b := f.call("POST", "/tasks", f.body(map[string]any{"title": "Second", "depends_on": []string{a["id"].(string)}}), 201)
	// A->B would be cyclic if checked before removing B->A. Only the final
	// graph determines whether this atomic rewiring is valid.
	body := taskDependencyHTTPBody(f, newID(), map[string]any{"task_id": a["id"], "expected_version": 1, "depends_on": []string{b["id"].(string)}}, map[string]any{"task_id": b["id"], "expected_version": 1, "depends_on": []string{}})
	ack := f.call("POST", "/task-dependencies", body, 200)
	changes := ack["changes"].([]any)
	if ack["operation_id"] != body["idempotency_key"] || len(changes) != 2 {
		t.Fatal(ack)
	}
	for _, change := range changes {
		item := change.(map[string]any)
		if item["previous_version"] != float64(1) || item["version"] != float64(2) {
			t.Fatal(item)
		}
	}
	f.call("PATCH", "/tasks/"+b["id"].(string), f.body(map[string]any{"title": "Newer title", "expected_version": 2}), 200)
	replay := f.call("POST", "/task-dependencies", body, 200)
	if !reflect.DeepEqual(ack, replay) {
		t.Fatalf("receipt replay changed: %#v != %#v", ack, replay)
	}
	current := f.call("GET", "/tasks/"+b["id"].(string)+"?project_id="+f.project, nil, 200)
	if current["version"] != float64(3) || current["title"] != "Newer title" {
		t.Fatal(current)
	}
	changedBody := taskDependencyHTTPBody(f, body["idempotency_key"].(string), map[string]any{"task_id": a["id"], "expected_version": 1, "depends_on": []string{}})
	bad := f.call("POST", "/task-dependencies", changedBody, 409)
	if bad["error"].(map[string]any)["code"] != "idempotency_conflict" {
		t.Fatal(bad)
	}
	// A known key cannot be relabelled with a different semantic error. Current
	// authorization and the original receipt precede self/cycle/version checks.
	selfBody := taskDependencyHTTPBody(f, body["idempotency_key"].(string), map[string]any{"task_id": a["id"], "expected_version": 1, "depends_on": []string{a["id"].(string)}})
	bad = f.call("POST", "/task-dependencies", selfBody, 409)
	if bad["error"].(map[string]any)["code"] != "idempotency_conflict" {
		t.Fatal(bad)
	}
}

func TestTaskDependenciesHTTPFinalGraphAndConflictRollback(t *testing.T) {
	f := newFixture(t)
	a := f.call("POST", "/tasks", f.body(map[string]any{"title": "A"}), 201)
	b := f.call("POST", "/tasks", f.body(map[string]any{"title": "B"}), 201)
	cycle := taskDependencyHTTPBody(f, newID(), map[string]any{"task_id": a["id"], "expected_version": 1, "depends_on": []string{b["id"].(string)}}, map[string]any{"task_id": b["id"], "expected_version": 1, "depends_on": []string{a["id"].(string)}})
	out := f.call("POST", "/task-dependencies", cycle, 400)
	if out["error"].(map[string]any)["message_key"] != "task_dependencies.cycle" {
		t.Fatal(out)
	}
	conflictBody := taskDependencyHTTPBody(f, newID(), map[string]any{"task_id": a["id"], "expected_version": 1, "depends_on": []string{b["id"].(string)}}, map[string]any{"task_id": b["id"], "expected_version": 9, "depends_on": []string{a["id"].(string)}})
	out = f.call("POST", "/task-dependencies", conflictBody, 409)
	details := out["error"].(map[string]any)["details"].(map[string]any)
	conflicts := details["conflicts"].([]any)
	if details["project_id"] != f.project || len(conflicts) != 1 || conflicts[0].(map[string]any)["current_version"] != float64(1) {
		t.Fatal(out)
	}
	for _, id := range []string{a["id"].(string), b["id"].(string)} {
		task := f.call("GET", "/tasks/"+id+"?project_id="+f.project, nil, 200)
		if task["version"] != float64(1) || len(task["depends_on"].([]any)) != 0 {
			t.Fatalf("partial write escaped rollback: %#v", task)
		}
	}
}

func TestTaskDependenciesHTTPRejectsClosedInvalidRequests(t *testing.T) {
	f := newFixture(t)
	a := f.call("POST", "/tasks", f.body(map[string]any{"title": "A"}), 201)
	b := f.call("POST", "/tasks", f.body(map[string]any{"title": "B"}), 201)
	id := a["id"].(string)
	other := b["id"].(string)
	for _, query := range []string{"", "project_id=" + f.project + "&project_id=" + f.project, "project_id=" + f.project + "&limit=1000", "project_id=" + f.project + "&unknown=1"} {
		f.call("GET", "/task-dependencies?"+query, nil, 400)
	}
	for _, item := range []map[string]any{
		{"task_id": id, "expected_version": 1, "depends_on": []string{id}},
		{"task_id": id, "expected_version": 1, "depends_on": []string{other, other}},
		{"task_id": id, "expected_version": 1, "depends_on": []string{"missing"}},
		{"task_id": id, "expected_version": 1, "depends_on": []string{}},
		{"task_id": id, "expected_version": 1.5, "depends_on": []string{other}},
		{"task_id": id, "expected_version": nil, "depends_on": []string{other}},
		{"task_id": id, "expected_version": 1},
		{"task_id": id, "expected_version": 1, "depends_on": nil},
		{"task_id": id, "expected_version": 1, "depends_on": []any{nil}},
		{"task_id": id, "expected_version": 1, "depends_on": []string{other}, "unknown": true},
	} {
		f.call("POST", "/task-dependencies", taskDependencyHTTPBody(f, newID(), item), 400)
	}
	change := map[string]any{"task_id": id, "expected_version": 1, "depends_on": []string{other}}
	f.call("POST", "/task-dependencies", taskDependencyHTTPBody(f, newID(), change, change), 400)
	f.call("POST", "/task-dependencies", taskDependencyHTTPBody(f, newID()), 400)
	oversized := []map[string]any{}
	for i := 0; i <= taskDependencyChangeLimit; i++ {
		oversized = append(oversized, change)
	}
	f.call("POST", "/task-dependencies", taskDependencyHTTPBody(f, newID(), oversized...), 400)
	body := taskDependencyHTTPBody(f, newID(), change)
	body["unknown"] = true
	f.call("POST", "/task-dependencies", body, 400)
	// Exact integer decoding rejects exponent/fraction lexemes rather than
	// rounding them through the generic map-backed request representation.
	for _, version := range []string{"1e0", "1.0", "2147483648", "9007199254740993.1"} {
		raw := `{"project_id":"` + f.project + `","idempotency_key":"version-test-001","changes":[{"task_id":"` + id + `","expected_version":` + version + `,"depends_on":["` + other + `"]}]}`
		r := httptest.NewRequest(http.MethodPost, "/api/v1/task-dependencies", strings.NewReader(raw))
		r.AddCookie(f.cookie)
		r.Header.Set("X-Suma-Expected-User", f.view.ID)
		r.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("version %s => %d: %s", version, w.Code, w.Body.String())
		}
	}
}

func TestTaskDependenciesOrdinaryTaskWritesRejectDuplicateEdges(t *testing.T) {
	f := newFixture(t)
	a := f.call("POST", "/tasks", f.body(map[string]any{"title": "A"}), 201)
	b := f.call("POST", "/tasks", f.body(map[string]any{"title": "B"}), 201)
	before := f.call("GET", "/task-dependencies?project_id="+f.project, nil, 200)
	duplicate := []string{b["id"].(string), b["id"].(string)}
	created := f.call("POST", "/tasks", f.body(map[string]any{"title": "Duplicate edges", "depends_on": duplicate}), 400)
	updated := f.call("PATCH", "/tasks/"+a["id"].(string), f.body(map[string]any{"title": "Must remain unchanged", "depends_on": duplicate, "expected_version": 1}), 400)
	for _, result := range []map[string]any{created, updated} {
		if result["error"].(map[string]any)["message_key"] != "task_dependencies.duplicate_dependency" {
			t.Fatal(result)
		}
	}
	after := f.call("GET", "/task-dependencies?project_id="+f.project, nil, 200)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("a rejected ordinary task command changed the graph: before=%#v after=%#v", before, after)
	}
}

func TestTaskDependenciesHTTPCurrentAuthorizationAndOriginalView(t *testing.T) {
	f := newFixture(t)
	a := f.call("POST", "/tasks", f.body(map[string]any{"title": "A"}), 201)
	b := f.call("POST", "/tasks", f.body(map[string]any{"title": "B"}), 201)
	body := taskDependencyHTTPBody(f, newID(), map[string]any{"task_id": a["id"], "expected_version": 1, "depends_on": []string{b["id"].(string)}})
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/task-dependencies", bytes.NewReader(raw))
	r.AddCookie(f.cookie)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing original view: %d %s", w.Code, w.Body.String())
	}
	f.call("POST", "/task-dependencies", body, 200)
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	if _, err := f.s.ChangeTaskDependencies(context.Background(), f.actor, f.project, TaskDependencyChangeRequest{ProjectID: f.project, IdempotencyKey: body["idempotency_key"].(string), Changes: []TaskDependencyChange{{TaskID: a["id"].(string), ExpectedVersion: 1, DependsOn: []string{b["id"].(string)}}}}); err == nil {
		t.Fatal("logged-out credential replayed a receipt")
	}
}

func TestTaskDependenciesCrossProjectAndAcceptanceInvalidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := f.call("POST", "/tasks", f.body(map[string]any{"title": "A"}), 201)
	b := f.call("POST", "/tasks", f.body(map[string]any{"title": "B"}), 201)
	c := f.call("POST", "/tasks", f.body(map[string]any{"title": "C", "depends_on": []string{a["id"].(string)}}), 201)
	project := f.call("POST", "/projects", map[string]any{"title": "Other project", "idempotency_key": newID()}, 201)
	foreign := f.call("POST", "/tasks", map[string]any{"title": "Foreign", "project_id": project["id"], "idempotency_key": newID()}, 201)
	f.call("POST", "/task-dependencies", taskDependencyHTTPBody(f, newID(), map[string]any{"task_id": a["id"], "expected_version": 1, "depends_on": []string{foreign["id"].(string)}}), 400)
	// Seed accepted evidence solely to verify invalidation propagation; this
	// does not forge an acceptance command or claim a real execution succeeded.
	for _, id := range []any{a["id"], c["id"]} {
		if _, err := f.s.DB.ExecContext(ctx, f.s.q(`UPDATE suma_tasks SET status='accepted',data=? WHERE id=?`), `{"verified_run_id":"old-fixed-evidence"}`, id); err != nil {
			t.Fatal(err)
		}
	}
	f.call("POST", "/task-dependencies", taskDependencyHTTPBody(f, newID(), map[string]any{"task_id": a["id"], "expected_version": 1, "depends_on": []string{b["id"].(string)}}), 200)
	for _, id := range []string{a["id"].(string), c["id"].(string)} {
		task := f.call("GET", "/tasks/"+id+"?project_id="+f.project, nil, 200)
		if task["version"] != float64(2) || task["status"] != "in_progress" || task["data"].(map[string]any)["verified_run_id"] != nil {
			t.Fatalf("stale acceptance retained: %#v", task)
		}
	}
}

func TestTaskDependenciesCompleteGraphLimitAndExistingLargeDependencySet(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ids := []string{}
	for i := 0; i <= taskDependencyGraphLimit; i++ {
		id := newID()
		ids = append(ids, id)
		r := Record{ID: id, TenantID: f.user.TenantID, ProjectID: f.project, Version: 1, Title: "Bounded graph fixture", Status: "draft", DependsOn: []string{}, AcceptanceCriteria: []string{}, CreatedAt: now(), UpdatedAt: now(), Data: map[string]any{}}
		if err = f.s.insertRecord(ctx, tx, r, "tasks"); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	out := f.call("GET", "/task-dependencies?project_id="+f.project, nil, 200)
	if out["complete"] != false || out["total"] != float64(1001) || len(out["tasks"].([]any)) != 0 {
		t.Fatalf("oversized graph was falsely projected: %#v", out)
	}
	// The read limit is not a mutation limit. Existing ordinary task editing
	// remains available, and this API must not introduce a 200-edge restriction.
	body := taskDependencyHTTPBody(f, newID(), map[string]any{"task_id": ids[0], "expected_version": 1, "depends_on": ids[1:252]})
	f.call("POST", "/task-dependencies", body, 200)
	updated := f.call("PATCH", "/tasks/"+ids[0], f.body(map[string]any{"title": "Still editable through the task form", "expected_version": 2}), 200)
	if updated["version"] != float64(3) || updated["title"] != "Still editable through the task form" || len(updated["depends_on"].([]any)) != 251 {
		t.Fatalf("the ordinary task editor diverged from the dependency domain: %#v", updated)
	}
}
