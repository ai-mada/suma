package main

import (
	"reflect"
	"slices"
	"testing"
)

func TestTaskIterationContractUsesSameBoundedScopeForRowsGroupsAndSavedViews(t *testing.T) {
	api, _ := generate()
	query := func(path string) map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, raw := range mapAt(api, "paths", path, "get")["parameters"].([]any) {
			p := raw.(map[string]any)
			if p["in"] == "query" {
				out[p["name"].(string)] = p
			}
		}
		return out
	}
	rows, groups := query("/task-workspace"), query("/task-collection/summary")
	prefs := mapAt(api, "components", "schemas", "SharedTaskViewPreferences")
	for _, field := range []string{"iteration_id", "iteration_unset"} {
		if rows[field] == nil || !reflect.DeepEqual(rows[field], groups[field]) {
			t.Fatal("row and group scopes must share the same iteration contract", field)
		}
		if mapAt(prefs, "properties")[field] == nil {
			t.Fatal("saved scope must retain iteration selection", field)
		}
	}
	for _, schema := range []map[string]any{rows["iteration_id"]["schema"].(map[string]any), mapAt(prefs, "properties", "iteration_id")} {
		if schema["maxLength"] != 64 || schema["x-max-utf8-bytes"] != 64 || schema["enum"] != nil {
			t.Fatal("iteration reference must be a bounded literal ID", schema)
		}
	}
	if rows["iteration_unset"]["schema"].(map[string]any)["type"] != "boolean" || mapAt(prefs, "properties", "iteration_unset")["type"] != "boolean" {
		t.Fatal("empty iteration must be an explicit boolean")
	}
	if !reflect.DeepEqual(groups["group_by"]["schema"].(map[string]any)["enum"], []string{"status", "owner", "priority", "iteration"}) || !reflect.DeepEqual(mapAt(prefs, "properties", "group_by")["enum"], []string{"", "status", "owner", "priority", "iteration"}) {
		t.Fatal("iteration grouping must be available for shared views and current groups")
	}
	if prefs["additionalProperties"] != false || groups["group_limit"]["schema"].(map[string]any)["maximum"] != 100 || rows["limit"]["schema"].(map[string]any)["maximum"] != 100 {
		t.Fatal("iteration selection must preserve closed preferences and bounded pages")
	}
}

func TestTaskIterationContractProjectsCurrentPlanVersionsOnlyOnRowPage(t *testing.T) {
	api, _ := generate()
	page := mapAt(api, "components", "schemas", "TaskWorkspacePage")
	plans := mapAt(page, "properties", "task_plans")
	if !slices.Contains(page["required"].([]string), "task_plans") || plans["type"] != "array" || plans["maxItems"] != 100 || plans["readOnly"] != true {
		t.Fatal("task plans must be a required bounded current projection", plans)
	}
	if mapAt(plans, "items")["$ref"] != "#/components/schemas/TaskWorkspacePlanRef" {
		t.Fatal("task plans must use the typed formal planning projection")
	}
	plan := mapAt(api, "components", "schemas", "TaskWorkspacePlanRef")
	if plan["additionalProperties"] != false || len(mapAt(plan, "properties")) != 3 {
		t.Fatal("plan projection must not admit task payload or authority fields")
	}
	for _, key := range []string{"task_id", "iteration_id", "plan_version"} {
		if !slices.Contains(plan["required"].([]string), key) || mapAt(plan, "properties", key)["readOnly"] != true {
			t.Fatal("missing required current plan fact", key)
		}
	}
	if mapAt(plan, "properties", "task_id")["minLength"] != 1 || mapAt(plan, "properties", "iteration_id")["maxLength"] != 64 || mapAt(plan, "properties", "plan_version")["minimum"] != 0 {
		t.Fatal("missing exact task reference, bounded iteration or no-plan version semantics")
	}
	if mapAt(api, "components", "schemas", "TaskCollectionSummary", "properties")["task_plans"] != nil {
		t.Fatal("aggregate groups must not claim to project task rows")
	}
}

// The batch uses the task workspace's independently versioned plan reference.
func TestTaskIterationAssignmentPublicContract(t *testing.T) {
	api, _ := generate()
	op := mapAt(api, "paths", "/planning/iteration-assignments", "post")
	for _, raw := range op["parameters"].([]any) {
		if raw.(map[string]any)["in"] == "query" {
			t.Fatal("the exact request body owns project scope")
		}
	}
	body := mapAt(op, "requestBody", "content", "application/json", "schema")
	if body["$ref"] != "#/components/schemas/TaskIterationAssignmentRequest" {
		t.Fatal("iteration assignment requires its closed command")
	}
	response := mapAt(op, "responses", "200", "content", "application/json", "schema")
	if response["$ref"] != "#/components/schemas/TaskIterationAssignmentResult" {
		t.Fatal("iteration assignment must return a typed operation result")
	}
	schemas := mapAt(api, "components", "schemas")
	request := mapAt(schemas, "TaskIterationAssignmentRequest")
	rows := mapAt(request, "properties", "tasks")
	if request["additionalProperties"] != false || len(request["required"].([]string)) != 5 || rows["minItems"] != 1 || rows["maxItems"] != 16 {
		t.Fatal("request must contain a bounded explicit selection")
	}
	selection := mapAt(schemas, "TaskIterationAssignmentSelection", "properties")
	if mapAt(selection, "expected_task_version")["minimum"] != 1 || mapAt(selection, "expected_plan_version")["minimum"] != 0 {
		t.Fatal("task and planning versions need independent integer bounds")
	}
}
