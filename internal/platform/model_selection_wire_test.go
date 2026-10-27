package platform

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestModelSelectionWireRejectsCaseAliasesBeforePointerDecoding(t *testing.T) {
	f, _, request, _ := taskDispatchFixture(t)
	for _, key := range []string{"MODEL_EXECUTION_SELECTION", "CODEX_EXECUTION_SELECTION", "Model_Execution_Selection", "model_execution_selection"} {
		body := f.body(candidateJSON(t, request))
		body[key] = nil
		f.call("POST", "/task-dispatch-plans", body, 400)
	}
	if expertCount(t, f, "suma_task_dispatch_plans") != 0 || expertCount(t, f, "dag_runs") != 0 {
		t.Fatal("invalid model key created effects")
	}
}

func TestModelSelectionWireCanonicalNestedKeys(t *testing.T) {
	raw := `{"deployment_id":"deployment-one","deployment_revision":1,"deployment_sha256":"` + strings.Repeat("a", 64) + `","parameters":{"max_steps":2,"max_output_tokens":100},"budget":{"token_limit":65000}}`
	decode := func(text string) map[string]any {
		var value map[string]any
		d := json.NewDecoder(strings.NewReader(text))
		d.UseNumber()
		if err := d.Decode(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if err := validateModelSelectionFields(map[string]any{"model_execution_selection": decode(raw)}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"deployment_id", "max_steps", "token_limit"} {
		value := decode(strings.Replace(raw, `"`+name+`"`, `"`+strings.ToUpper(name)+`"`, 1))
		if err := validateModelSelectionFields(map[string]any{"model_execution_selection": value}); err == nil {
			t.Fatalf("noncanonical key accepted: %s", name)
		}
	}
}
