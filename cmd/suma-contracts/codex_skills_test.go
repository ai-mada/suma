package main

import "testing"

func TestNativeSkillContractKeepsPlanAndAttemptFactsSeparate(t *testing.T) {
	api, _ := generate()
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"ExecutionSkillDocument", "ExecutionSkillBundle", "ExecutionSkillReceipt", "ExecutionSkillEvidence"} {
		if toolContractMap(schemas, name)["additionalProperties"] != false {
			t.Fatalf("native skill schema is not closed: %s", name)
		}
	}
	if toolContractMap(schemas, "TaskDispatchPlan", "properties")["execution_skills"] == nil || toolContractMap(schemas, "CodexEvidence", "properties")["skills"] == nil {
		t.Fatal("planning material and execution observations must both be typed")
	}
	if toolContractMap(schemas, "TaskDispatchPreviewRequest", "properties")["execution_skills"] != nil {
		t.Fatal("caller can supply material instead of published references")
	}
	if toolContractMap(schemas, "ExecutionSkillBundle", "properties", "documents")["maxItems"] != 16 {
		t.Fatal("bounded skill selection lost")
	}
	if toolContractMap(schemas, "ExecutionSkillReceipt", "properties")["content"] != nil || toolContractMap(schemas, "ExecutionSkillReceipt", "properties")["path"] != nil {
		t.Fatal("attempt receipt leaks native instructions or local paths")
	}
	for _, name := range []string{"materialized", "catalog_acknowledged", "invocation_requested", "turn_acknowledged", "files_verified"} {
		if toolContractMap(schemas, "ExecutionSkillEvidence", "properties")[name] == nil {
			t.Fatalf("native skill fact omitted: %s", name)
		}
	}
}
