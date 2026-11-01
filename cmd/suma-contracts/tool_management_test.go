package main

import (
	"strings"
	"testing"
)

func TestToolManagementContractKeepsFilteredReadsAndScopedImpact(t *testing.T) {
	api, types := generate()
	s := mapAt(api, "components", "schemas")
	get := mapAt(api, "paths", "/tool-profiles", "get")
	if get["requestBody"] != nil {
		t.Fatal("filtered list gained a write body")
	}
	found := map[string]bool{}
	for _, p := range get["parameters"].([]any) {
		value := p.(map[string]any)
		if name, ok := value["name"].(string); ok {
			if found[name] {
				t.Fatal("duplicate query filter", name)
			}
			found[name] = true
			if name == "cursor" && (mapAt(value, "schema")["maxLength"] != 1024 || !strings.Contains(value["description"].(string), "actor") || !strings.Contains(value["description"].(string), "state/query")) {
				t.Fatal("operation cursor lost its enforced boundary or filter/actor identity")
			}
		}
	}
	if !found["state"] || !found["query"] || mapAt(s, "ToolProfileListQuery", "properties", "state")["default"] != "active" || mapAt(s, "ToolProfileListQuery", "properties", "query")["x-max-utf8-bytes"] != 120 {
		t.Fatal("public filter contract drifted")
	}
	impact := mapAt(s, "ToolProfileImpact")
	if impact["additionalProperties"] != false || mapAt(impact, "properties", "scope")["const"] != "active_project_runs" || mapAt(impact, "properties", "items")["maxItems"] != 5 || len(impact["allOf"].([]any)) != 3 {
		t.Fatal("impact became an unbounded/global availability claim")
	}
	if mapAt(s, "ToolProfileImpactItem", "properties", "attempt")["minimum"] != 0 || mapAt(s, "ToolProfileImpactItem", "properties", "fence")["minimum"] != 0 {
		t.Fatal("unassigned nodes fabricated an attempt/fence")
	}
	if len(types) == 0 || mapAt(s, "ToolProfileDiagnostics", "properties")["impact"] == nil {
		t.Fatal("shared clients lost impact facts")
	}
	for _, field := range []string{"ready", "repaired", "process_stopped", "available"} {
		if mapAt(impact, "properties")[field] != nil {
			t.Fatal("read-only impact acquired side-effect authority", field)
		}
	}
}
