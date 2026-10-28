package main

import "testing"

func TestProjectRiskContractKeepsAdviceAndQualityAuthoritySeparate(t *testing.T) {
	api, _ := generate()
	for _, path := range []string{"/risks", "/risks/summary", "/risks/{id}", "/risks/{id}/commands"} {
		if mapAt(api, "paths")[path] == nil {
			t.Fatalf("missing risk route %s", path)
		}
	}
	schemas := mapAt(api, "components", "schemas")
	for _, name := range []string{"ProjectRiskCreateRequest", "ProjectRiskSaveRequest", "ProjectRiskCommandRequest", "ProjectRiskReviewInput"} {
		if mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatalf("open risk mutation contract %s", name)
		}
	}
	for _, key := range []string{"source", "score", "state", "current_review_id", "accepted", "verified_run_id"} {
		if mapAt(schemas, "ProjectRiskCreateRequest", "properties")[key] != nil {
			t.Fatalf("caller can forge protected risk field %s", key)
		}
	}
	if mapAt(schemas, "ProjectRiskView", "properties")["history_total"] == nil || mapAt(schemas, "ProjectRiskSummary", "properties", "matrix_scope")["const"] != "active" {
		t.Fatal("risk history or matrix scope is undisclosed")
	}
	if mapAt(schemas, "ProjectRiskCommandRequest")["allOf"] == nil {
		t.Fatal("review and closure prerequisites missing")
	}
}
