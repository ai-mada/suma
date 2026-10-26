package platform

import (
	"encoding/json"
	nwc "github.com/ai-mada/suma/internal/nativeworkspacecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	"strings"
	"testing"
)

func nativeEvidenceFixture() (map[string]any, *tc.Evidence) {
	policy := strings.Repeat("a", 64)
	base := strings.Repeat("b", 40)
	head := strings.Repeat("c", 40)
	files := []nwc.NativeWorkspaceFileEffect{{Path: "src/code.ts", OperationID: "write-op", SHA256: strings.Repeat("d", 64), Generation: 1}}
	evidence := nwc.NativeWorkspaceEvidence{SchemaVersion: 1, PolicySHA256: policy, Generation: 1, State: "closed", Files: files, Checks: []nwc.NativeWorkspaceCheckResult{{OperationID: "check-op", CheckID: "unit", ProfileSHA256: strings.Repeat("e", 64), WorkspaceSHA256: strings.Repeat("f", 64), Generation: 1, ExitCode: 0}}, Seal: &nwc.NativeWorkspaceSeal{SchemaVersion: 1, OperationID: "seal-op", PolicySHA256: policy, State: "sealed", PreviousCommit: base, Commit: head, Tree: strings.Repeat("d", 40), ChangedPaths: 1}}
	return map[string]any{"base_commit": base, "head_commit": head, "native_workspace": evidence}, &tc.Evidence{Profile: tc.Profile{Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: policy, WorkspaceToolsVersion: nwc.Version}}
}

func TestNativeWorkspaceAcceptanceRequiresCurrentClosedSeal(t *testing.T) {
	proof, tool := nativeEvidenceFixture()
	if err := validateNativeWorkspaceEvidence(proof, tool); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"missing", "unknown", "policy", "head", "check_generation", "check_failed", "check_unknown", "check_stopped", "seal_missing", "untyped"} {
		t.Run(name, func(t *testing.T) {
			proof, tool := nativeEvidenceFixture()
			e := proof["native_workspace"].(nwc.NativeWorkspaceEvidence)
			switch name {
			case "missing":
				delete(proof, "native_workspace")
			case "unknown":
				e.State = "unknown"
			case "policy":
				e.PolicySHA256 = strings.Repeat("b", 64)
			case "head":
				proof["head_commit"] = strings.Repeat("f", 40)
			case "check_generation":
				e.Checks[0].Generation = 0
			case "check_failed":
				e.Checks[0].ExitCode = 1
			case "check_unknown":
				e.Checks[0].OutcomeUnknown = true
			case "check_stopped":
				e.Checks[0].Stopped = true
			case "seal_missing":
				e.Seal = nil
			case "untyped":
				raw, _ := json.Marshal(e)
				var m map[string]any
				_ = json.Unmarshal(raw, &m)
				m["approved"] = true
				proof["native_workspace"] = m
			}
			if name != "missing" && name != "untyped" {
				proof["native_workspace"] = e
			}
			if err := validateNativeWorkspaceEvidence(proof, tool); err == nil {
				t.Fatal("invalid native evidence accepted")
			}
		})
	}
	if err := validateNativeWorkspaceEvidence(map[string]any{}, nil); err != nil {
		t.Fatal("legacy evidence was broadened", err)
	}
}
