package main

import (
	"encoding/json"
	"testing"

	"github.com/ai-mada/suma/internal/platform"
)

func TestApplicationToolContractDeclaresEffectsWithoutVerifiedConnection(t *testing.T) {
	api, _ := generate()
	props := toolContractMap(api, "components", "schemas", "DeveloperConnectionMetadata", "properties")
	if props["client_connection_verified"].(map[string]any)["const"] != false {
		t.Fatal("metadata claims verification")
	}
	data, err := applicationToolCatalogue()
	if err != nil {
		t.Fatal(err)
	}
	var catalogue struct {
		Items []struct {
			Name     string `json:"name"`
			MCPName  string `json:"mcp_name"`
			ReadOnly bool   `json:"read_only"`
		} `json:"items"`
	}
	if json.Unmarshal(data, &catalogue) != nil || len(catalogue.Items) != len(platform.DeclaredApplicationTools()) {
		t.Fatal("tool catalogue drift")
	}
	writes := 0
	qualityReads := map[string]bool{"project.task_delivery": false, "project.quality_execution": false}
	for _, tool := range catalogue.Items {
		if _, wanted := qualityReads[tool.Name]; wanted {
			if !tool.ReadOnly {
				t.Fatal("quality observations gained write effects")
			}
			qualityReads[tool.Name] = true
		}
		if !tool.ReadOnly {
			writes++
		}
	}
	if writes != 3 {
		t.Fatal("write effect contract drift")
	}
	for name, present := range qualityReads {
		if !present {
			t.Fatalf("missing current quality read %s", name)
		}
	}
	page := toolContractMap(api, "components", "schemas", "ApplicationQualityExecutionPage")
	if page["readOnly"] != true || toolContractMap(page, "properties", "cases")["maxItems"] != 20 || toolContractMap(api, "components", "schemas", "ApplicationQualityCase", "properties", "evidence")["maxItems"] != 3 {
		t.Fatal("quality read bounds drifted")
	}
}
