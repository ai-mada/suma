package executioncontract

import (
	"encoding/json"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestStartupFailureWireRequiresExplicitHostAssertions(t *testing.T) {
	proof := StartupFailureProof{Protocol: StartupFailureProtocol, Stage: "before_workspace", Assignment: cc.AssignmentRef{RunID: "run", NodeID: "node", WorkerID: "worker", Attempt: 1, Fence: 2}, Profile: tc.Profile{CommandID: "native", Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "fixture"}, Purpose: "candidate_conflict_repair"}
	raw, _ := json.Marshal(proof)
	var decoded StartupFailureProof
	if err := json.Unmarshal(raw, &decoded); err != nil || ValidateStartupFailureProof(decoded, proof.Assignment, proof.Profile, proof.Purpose) != nil {
		t.Fatal("complete host fact rejected", err)
	}
	for _, key := range []string{"workspace_started", "process_started"} {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		delete(fields, key)
		next, _ := json.Marshal(fields)
		if err := json.Unmarshal(next, &decoded); err == nil {
			t.Fatal("missing host assertion decoded as false", key)
		}
		fields[key] = json.RawMessage("null")
		next, _ = json.Marshal(fields)
		if err := json.Unmarshal(next, &decoded); err == nil {
			t.Fatal("null host assertion decoded as false", key)
		}
	}
}
