package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestTestPlansRoutesPreserveCurrentAuthorityAndOriginalEvidence(t *testing.T) {
	api, types := generate()
	paths := mapAt(api, "paths")
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/test-plans", "get", "", "TestPlanList", "200"},
		{"/test-plans", "post", "TestPlanCreateRequest", "TestPlanView", "201"},
		{"/test-plans/{id}", "get", "", "TestPlanView", "200"},
		{"/test-plans/{id}", "put", "TestPlanSaveRequest", "TestPlanView", "200"},
		{"/test-plans/{id}/commands", "post", "TestPlanCommandRequest", "TestPlanView", "200"},
		{"/test-plans/{id}/executions", "get", "", "TestPlanExecutionList", "200"},
		{"/test-plans/{id}/executions/{run}", "get", "", "TestPlanExecution", "200"},
		{"/test-plans/{id}/executions/{run}/defects", "post", "TestPlanDefectRequest", "Record", "201"},
		{"/test-plans/{id}/reports/{report}", "get", "", "TestPlanReport", "200"},
	} {
		if paths[route.path] == nil {
			t.Fatal("test plan route absent", route.path)
		}
		op := mapAt(paths, route.path, route.method)
		if mapAt(op, "responses", route.status, "content", "application/json", "schema")["$ref"] != "#/components/schemas/"+route.output {
			t.Fatal("route lost exact response", route)
		}
		if route.input != "" && mapAt(op, "requestBody", "content", "application/json", "schema")["$ref"] != "#/components/schemas/"+route.input {
			t.Fatal("request uses generic envelope", route)
		}
		want := 2
		if route.input == "TestPlanCommandRequest" {
			want = 1
		}
		if len(op["security"].([]any)) != want {
			t.Fatal("current actor boundary changed", route)
		}
	}
	if mapAt(paths, "/test-plans/{id}/executions")["post"] != nil {
		t.Fatal("a second test dispatcher was invented")
	}
	if !reflect.DeepEqual(mapAt(paths, "/test-plans/{id}/commands", "post")["x-disallowed-token-scopes"], []string{"api", "agent", "runner"}) {
		t.Fatal("machines or scoped tokens can approve plans")
	}
	for _, name := range []string{"TestPlanView", "QualityTestCase", "TestPlanReport", "TestCaseSelector", "TestReport"} {
		if !strings.Contains(types, "export interface "+name+" {") {
			t.Fatal("generated wire type missing", name)
		}
	}
}

func TestTestPlansSchemasKeepIncompleteDraftsAndCaseProofSeparate(t *testing.T) {
	api, _ := generate()
	schemas := mapAt(api, "components", "schemas")
	for _, name := range []string{"TestPlanCreateRequest", "TestPlanSaveRequest", "TestPlanCommandRequest", "TestPlanDefectRequest", "QualityTestCase", "TestCaseSelector", "TestReport"} {
		if schemas[name] == nil || mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatal("unbounded or missing current schema", name)
		}
	}
	for _, name := range []string{"TestPlanCreateRequest", "TestPlanSaveRequest"} {
		p := mapAt(schemas, name, "properties")
		if mapAt(p, "expected_requirement_version")["minimum"] != 0 || mapAt(p, "cases")["maxItems"] != 100 || mapAt(p, "cases")["x-max-json-utf8-bytes"] != 128*1024 {
			t.Fatal("draft bounds/source versions changed", name)
		}
	}
	if mapAt(schemas, "QualityTestCase", "properties", "covers", "items")["minimum"] != 1 || mapAt(schemas, "QualityTestCase", "properties", "covers")["uniqueItems"] != true || mapAt(schemas, "QualityTestCase", "properties", "title")["minLength"] != nil || mapAt(schemas, "TestCaseSelector", "properties", "name")["minLength"] != nil {
		t.Fatal("draft was forced complete or coverage lost exact task positions")
	}
	if mapAt(schemas, "TestReport", "properties", "xml")["x-max-utf8-bytes"] != 256*1024 || mapAt(schemas, "ParsedTestReport", "properties", "cases")["maxItems"] != 1000 {
		t.Fatal("original report/case limits drifted")
	}
	if !reflect.DeepEqual(mapAt(schemas, "TestPlanCommandRequest", "properties", "action")["enum"], []string{"publish", "retire"}) || mapAt(schemas, "TestPlanSource", "properties", "task_basis_sha256")["readOnly"] != true || mapAt(schemas, "TestPlanReport", "properties", "xml")["readOnly"] != true || mapAt(schemas, "TestPlanReportRef", "properties", "fence")["readOnly"] != true {
		t.Fatal("source/review/report authority leaked into caller fields")
	}
	if !strings.Contains(mapAt(schemas, "QualityTestCase", "properties", "covers")["description"].(string), "task") {
		t.Fatal("coverage no longer identifies exact task criteria")
	}
}
