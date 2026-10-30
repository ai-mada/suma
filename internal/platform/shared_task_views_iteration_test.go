package platform

import (
	"strings"
	"testing"
)

func TestSharedTaskViewsIterationScopePersistsAndUsesOriginalReceipt(t *testing.T) {
	f := newFixture(t)
	// A saved selector is not a claim that an iteration exists or grants access.
	prefs := map[string]any{"iteration_id": "unknown-iteration", "group_by": "iteration", "layout": "board"}
	req := f.body(map[string]any{"name": "Iteration scope", "visibility": "project", "preferences": prefs})
	saved := f.call("POST", "/task-views", req, 201)
	id := saved["id"].(string)
	assertScope := func(row map[string]any, wantID string, wantUnset bool) {
		t.Helper()
		p := row["preferences"].(map[string]any)
		actualID, _ := p["iteration_id"].(string)
		actualUnset, _ := p["iteration_unset"].(bool)
		if actualID != wantID || actualUnset != wantUnset || p["group_by"] != "iteration" {
			t.Fatalf("iteration selection was lost or broadened: %#v", p)
		}
	}
	assertScope(saved, "unknown-iteration", false)
	assertScope(f.call("GET", "/task-views/"+id+"?project_id="+f.project, nil, 200), "unknown-iteration", false)
	update := f.body(map[string]any{"name": "No iteration", "visibility": "project", "expected_version": 1, "preferences": map[string]any{"iteration_unset": true, "group_by": "iteration", "layout": "board"}})
	next := f.call("PATCH", "/task-views/"+id, update, 200)
	assertScope(next, "", true)
	if next["version"] != float64(2) {
		t.Fatal("updated selection must have a new observed version", next["version"])
	}
	assertScope(f.call("PATCH", "/task-views/"+id, update, 200), "", true)
	// Recover the original create without overwriting the newer saved scope.
	assertScope(f.call("POST", "/task-views", req, 201), "unknown-iteration", false)
	assertScope(f.call("GET", "/task-views/"+id+"?project_id="+f.project, nil, 200), "", true)
	stale := f.body(map[string]any{"name": "Stale scope", "visibility": "project", "expected_version": 1, "preferences": prefs})
	f.call("PATCH", "/task-views/"+id, stale, 409)
	// The original operation cannot be reused to choose a different iteration.
	req["preferences"] = map[string]any{"iteration_id": "another-iteration", "group_by": "iteration"}
	f.call("POST", "/task-views", req, 409)
}

func TestSharedTaskViewsIterationRejectsAmbiguousOrInvalidScope(t *testing.T) {
	f := newFixture(t)
	for _, tc := range []struct {
		name        string
		preferences map[string]any
	}{
		{"conflicting_empty", map[string]any{"iteration_id": "iteration-a", "iteration_unset": true}},
		{"oversized_id", map[string]any{"iteration_id": strings.Repeat("a", 65)}},
		{"control_id", map[string]any{"iteration_id": "bad\niteration"}},
		{"null_id", map[string]any{"iteration_id": nil}},
		{"nonboolean_empty", map[string]any{"iteration_unset": "true"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := f.body(map[string]any{"name": "Invalid iteration scope", "visibility": "private", "preferences": tc.preferences})
			f.call("POST", "/task-views", body, 400)
		})
	}
	// Unrestricted and empty selection are both explicit valid current formats.
	for _, preferences := range []map[string]any{{"group_by": "iteration"}, {"iteration_id": "", "iteration_unset": true, "group_by": "iteration"}} {
		f.call("POST", "/task-views", f.body(map[string]any{"name": "Valid iteration scope", "visibility": "private", "preferences": preferences}), 201)
	}
}
