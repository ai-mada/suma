package platform

import (
	"encoding/json"
	"testing"
)

// Only ingress is under test here. This original composition fixture uses
// protected command producers, so a well-shaped task origin reaches and is
// rejected by the ordinary-agent domain guard instead of transport parsing.
func TestAcceptedTaskBaselineHTTPRequiresExactExclusiveOrigin(t *testing.T) {
	h := newCompositionFixture(t)
	f := h.f
	source := h.req.Sources[0]
	req := TaskCodeBaselineBindRequest{ProjectID: f.project, ExpectedTaskVersion: h.req.ExpectedTaskVersion, ExpectedBindingVersion: 0, SourceTaskID: source.TaskID, ExpectedSourceTaskVersion: source.ExpectedTaskVersion, SourceNodeID: source.SourceNodeID, Acknowledge: true, Reason: "Explicit accepted ordinary task candidate", IdempotencyKey: newID()}
	route := "/tasks/" + h.req.TaskID + "/code-baseline"
	f.call("POST", route, req, 409)
	for _, key := range []string{"source_task_id", "expected_source_task_version", "source_node_id"} {
		bad := candidateJSON(t, req)
		delete(bad, key)
		f.call("POST", route, bad, 400)
	}
	for _, key := range []string{"composition_id", "expected_composition_version", "terminal_candidate_plan_id", "expected_terminal_candidate_plan_version"} {
		bad := candidateJSON(t, req)
		bad[key] = nil
		f.call("POST", route, bad, 400)
	}
	for _, value := range []any{nil, "1", json.Number("1.0"), json.Number("1e0")} {
		bad := candidateJSON(t, req)
		bad["expected_source_task_version"] = value
		f.call("POST", route, bad, 400)
	}
	bad := candidateJSON(t, req)
	bad["source"] = map[string]any{"head_commit": "untrusted"}
	f.call("POST", route, bad, 400)
}
