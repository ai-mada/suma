package desktoprecoverycontract

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Keep the plan's fence as text across the Go/desktop JSON boundary. This
// fixture also covers Go's HTML and line-separator escaping for HMAC input.
func TestProofBindsExactOriginalPlanAndExpiresForWrites(t *testing.T) {
	plan := Plan{Protocol: PlanProtocol, ProjectID: strings.Repeat("a", 32), WorkerID: "desktop-" + strings.Repeat("b", 32), SnapshotSHA256: strings.Repeat("c", 64), Items: []Item{{JournalKey: "运行/<&>\u2028/1/9007199254740993", RunID: "运行", NodeID: "<&>\u2028", Attempt: 1, Fence: "9007199254740993", OperationID: "result-1", BodySHA256: strings.Repeat("d", 64), ResultSHA256: strings.Repeat("e", 64)}}}
	if err := ValidatePlan(plan); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(plan)
	if !bytes.Contains(raw, []byte(`"fence":"9007199254740993"`)) || !bytes.Contains(raw, []byte(`\u003c\u0026\u003e\u2028`)) {
		t.Fatalf("cross-runtime plan lost canonical content: %s", raw)
	}
	now := time.Date(2026, 11, 1, 1, 2, 3, 0, time.UTC)
	key := bytes.Repeat([]byte{0x17}, 32)
	proof := SignProof(Proof{Protocol: ProofProtocol, Scope: Scope{ProjectID: plan.ProjectID, UserID: strings.Repeat("1", 32), TenantID: strings.Repeat("2", 32), WorkerID: plan.WorkerID, AdapterID: "codex-jsonl", InstanceID: strings.Repeat("3", 64), ExpectedBindingSHA256: strings.Repeat("4", 64), BaseCommit: strings.Repeat("5", 40), RegistrationOperationID: "desktop-runner-" + strings.Repeat("6", 32), CredentialSHA256: strings.Repeat("7", 64)}, PlanSHA256: PlanSHA256(plan), IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano), Items: []Receipt{{JournalKey: plan.Items[0].JournalKey, OperationID: "result-1", Status: "matched"}}}, key)
	if err := CheckProof(proof, plan, key, now, false); err != nil {
		t.Fatal(err)
	}
	if CheckProof(proof, plan, key, now.Add(time.Minute), false) == nil || CheckProof(proof, plan, key, now.Add(time.Minute), true) != nil {
		t.Fatal("expired proof authorized a write or could not inspect original facts")
	}
	proof.Items[0].Status = "missing"
	if CheckProof(proof, plan, key, now, false) == nil {
		t.Fatal("tampered receipt accepted")
	}
}
