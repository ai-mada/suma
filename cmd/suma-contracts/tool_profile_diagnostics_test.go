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
	for key, source := range map[string]string{"authorization_current": "配置启用授权当前有效", "worker_current": "执行节点身份当前有效", "inventory_fresh": "工具清单当前新鲜", "profile_matches": "节点已报告完整相同配置可用"} {
		if displaycontract.BindToolReason(source, "display.tool."+key, nil) == nil {
			t.Fatalf("producer cannot bind a canonical bilingual passing fact: %s", key)
		}
	}
}
