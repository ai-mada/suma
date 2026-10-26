package main

import (
	"testing"

	"github.com/ai-mada/suma/internal/localization"
)

func TestNativeDiagnosticContractVersionAndEventScope(t *testing.T) {
	api, _ := generate()
	d := mapAt(api, "components", "schemas", "ToolDiagnostic")
	if d["additionalProperties"] != false || mapAt(d, "properties", "schema_version")["const"] != 1 {
		t.Fatal("unbounded native diagnostic schema")
	}
	if len(d["allOf"].([]any)) != len(localization.NativeCatalog()) {
		t.Fatal("parameter cases drifted from native catalog")
	}
	e := mapAt(api, "components", "schemas", "ToolEvent")
	if e["allOf"] == nil {
		t.Fatal("missing native error condition")
	}
}
