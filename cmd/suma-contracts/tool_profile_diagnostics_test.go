package main

import (
	"testing"

	"github.com/ai-mada/suma/internal/displaycontract"
)

func TestToolProfileDiagnosticsContractKeepsOriginalAvailabilityAndFourFacts(t *testing.T) {
	api, _ := generate()
	op := mapAt(api, "paths", "/tool-profiles/{id}/diagnostics", "get")
	if op["x-read-only"] != true || op["x-disallowed-token-scopes"] == nil || op["requestBody"] != nil {
		t.Fatal("diagnostics gained a write request or lost current human read authority")
	}
	schemas := mapAt(api, "components", "schemas")
	properties := mapAt(schemas, "ToolProfileDiagnostics", "properties")
	if properties["available"] != nil || properties["ready"] != nil || properties["tool"] == nil || mapAt(properties, "checks")["minItems"] != 4 || mapAt(properties, "checks")["maxItems"] != 4 {
		t.Fatal("diagnostics introduced another availability authority or omitted prerequisite facts")
	}
	// Maintenance suggestions must remain a distinct current-read contract. A
	// caller cannot interpret a dispatch prerequisite as permission to re-probe.
	resolution := mapAt(schemas, "ToolProfileMaintenanceResolution")
	if properties["maintenance_resolution"] == nil || resolution["additionalProperties"] != false || len(resolution["oneOf"].([]any)) != 6 {
		t.Fatal("missing closed maintenance action shapes, separate from dispatch facts")
	}
	for _, field := range []string{"profile_id", "profile_version", "worker_id", "action", "reason"} {
		found := false
		for _, name := range resolution["required"].([]string) {
			found = found || name == field
		}
		if !found {
			t.Fatalf("maintenance recommendation lost its required basis: %s", field)
		}
	}
	for key, source := range map[string]string{"authorization_current": "配置启用授权当前有效", "worker_current": "执行节点身份当前有效", "inventory_fresh": "工具清单当前新鲜", "profile_matches": "节点已报告完整相同配置可用"} {
		if displaycontract.BindToolReason(source, "display.tool."+key, nil) == nil {
			t.Fatalf("producer cannot bind a canonical bilingual passing fact: %s", key)
		}
	}
}
