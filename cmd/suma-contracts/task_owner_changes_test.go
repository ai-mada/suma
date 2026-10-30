package main

import "testing"

func TestTaskOwnerChangesPublicContract(t *testing.T) {
	api, _ := generate()
	paths := toolContractMap(api, "paths")
	preview := toolContractMap(paths, "/task-owner-changes/preview", "post")
	apply := toolContractMap(paths, "/task-owner-changes", "post")
	if preview["x-read-only"] != true || preview["x-no-external-effects"] != true || apply["x-read-only"] == true {
		t.Fatal("preview read and effectful command must remain distinct")
	}
	for _, operation := range []map[string]any{preview, apply} {
		for _, raw := range operation["parameters"].([]any) {
			parameter := raw.(map[string]any)
			if parameter["in"] == "query" {
				t.Fatal("batch POST scope must come only from the exact JSON body")
			}
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	selection := toolContractMap(schemas, "TaskOwnerSelection", "properties", "expected_version")
	if selection["minimum"] != 1 || selection["maximum"] != 2147483647 {
		t.Fatal("versions need the exact positive integer bound")
	}
	request := toolContractMap(schemas, "TaskOwnerChangeRequest")
	if request["additionalProperties"] != false || toolContractMap(request, "properties", "acknowledge_invalidation")["const"] != true || toolContractMap(request, "properties", "preview_sha256")["pattern"] != "^[a-f0-9]{64}$" {
		t.Fatal("command requires closed, preview-bound explicit acknowledgement")
	}
	tasks := toolContractMap(request, "properties", "tasks")
	if tasks["minItems"] != 1 || tasks["maxItems"] != 16 {
		t.Fatal("implicit or unbounded task selection is not a valid batch")
	}
}
