package main

import (
	"encoding/json"
	"testing"
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
	if json.Unmarshal(data, &catalogue) != nil || len(catalogue.Items) != 15 {
		t.Fatal("tool catalogue drift")
	}
	writes := 0
	for _, tool := range catalogue.Items {
		if !tool.ReadOnly {
			writes++
		}
	}
	if writes != 3 {
		t.Fatal("write effect contract drift")
	}
}
