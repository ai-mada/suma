package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	rc "github.com/ai-mada/suma/internal/desktoprecoverycontract"
)

func recovery179Document(t *testing.T, dir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "journal.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err = decoder.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	return doc
}
func recovery179Write(t *testing.T, dir string, doc map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "journal.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	return raw
}
func recovery179Has(result rc.Result, reason string) bool {
	for _, value := range result.Reasons {
		if value == reason {
			return true
		}
	}
	return false
}

func resultRecovery179Fixture(t *testing.T) (string, string, string, []Result) {
	t.Helper()
	dir := t.TempDir()
	project := strings.Repeat("1", 32)
	worker := "desktop-" + strings.Repeat("2", 32)
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	var results []Result
	for _, node := range []string{"one", "two"} {
		a := Assignment{RunID: "run179", NodeID: node, WorkerID: worker, ProjectID: project, Attempt: 1, Fence: 9007199254740997}
		r := Result{Assignment: a, OperationID: "operation179-" + node, WorkspaceEvidence: map[string]any{"large": json.Number("9007199254740999")}}
		if err = j.Accept(a); err != nil {
			t.Fatal(err)
		}
		if err = j.Finish(r); err != nil {
			t.Fatal(err)
		}
		results = append(results, r)
	}
	return dir, project, worker, results
}
func resultRecovery179Proof(plan rc.Plan, statuses ...string) (rc.Proof, []byte, time.Time) {
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	key := []byte(strings.Repeat("k", 32))
	proof := rc.Proof{Protocol: rc.ProofProtocol, Scope: rc.Scope{ProjectID: plan.ProjectID, WorkerID: plan.WorkerID, UserID: strings.Repeat("3", 32), TenantID: strings.Repeat("4", 32), AdapterID: "codex-app-server", InstanceID: strings.Repeat("5", 64), ExpectedBindingSHA256: strings.Repeat("6", 64), BaseCommit: strings.Repeat("a", 40), RegistrationOperationID: "desktop-runner-" + strings.Repeat("7", 32), CredentialSHA256: strings.Repeat("8", 64)}, PlanSHA256: rc.PlanSHA256(plan), IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano), Items: []rc.Receipt{}}
	for i, item := range plan.Items {
		status := "matched"
		if i < len(statuses) {
			status = statuses[i]
		}
		proof.Items = append(proof.Items, rc.Receipt{JournalKey: item.JournalKey, OperationID: item.OperationID, Status: status})
	}
	return rc.SignProof(proof, key), key, now
}
func TestDesktopResultRecoveryPrepareKeepsExactCallbackAndNumbers(t *testing.T) {
	dir, project, worker, results := resultRecovery179Fixture(t)
	before := renewal178Files(t, dir)
	prepared := PrepareDesktopResultRecovery(dir, project, worker)
	if prepared.Plan == nil || prepared.Total != 2 || prepared.Remaining != 2 || len(prepared.Plan.Items) != 2 {
		t.Fatalf("saved callbacks not planned: %+v", prepared)
	}
	if rc.ValidatePlan(*prepared.Plan) != nil || prepared.Plan.Items[0].Fence != "9007199254740997" {
		t.Fatal("invalid or rounded recovery plan")
	}
	var sent []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sent, _ = io.ReadAll(r.Body); w.WriteHeader(204) }))
	defer server.Close()
	client := Client{BaseURL: server.URL, ProjectID: project, Token: "only-controlled-http"}
	if err := client.Complete(context.Background(), results[0]); err != nil {
		t.Fatal(err)
	}
	if rc.Digest(sent) != prepared.Plan.Items[0].BodySHA256 || !strings.Contains(string(sent), "9007199254740999") {
		t.Fatal("planned callback differs from actual Complete encoding")
	}
	renewal178AssertUnchanged(t, dir, before)
}

func TestDesktopResultRecoveryAtomicACKAndLostResponse(t *testing.T) {
	dir, project, worker, _ := resultRecovery179Fixture(t)
	plan := PrepareDesktopResultRecovery(dir, project, worker).Plan
	if plan == nil {
		t.Fatal("missing plan")
	}
	proof, key, now := resultRecovery179Proof(*plan)
	before := renewal178Files(t, dir)
	status := InspectDesktopResultRecovery(dir, project, worker, *plan, rc.Proof{}, nil, now)
	if status.Recovered != 0 || status.Remaining != 2 || !recovery179Has(status, "result_unknown") {
		t.Fatalf("plan-only status changed authority: %+v", status)
	}
	renewal178AssertUnchanged(t, dir, before)
	// Simulate a process exit during replacement write. The approved prefix
	// may be completed; no unrelated temporary file can be overwritten.
	expected := recovery179Document(t, dir)
	for _, item := range plan.Items {
		expected[item.JournalKey].(map[string]any)["acked"] = true
	}
	data, _ := json.Marshal(expected)
	temp := filepath.Join(dir, "journal-recovery-"+rc.PlanSHA256(*plan)+".tmp")
	if err := os.WriteFile(temp, data[:len(data)/2], 0600); err != nil {
		t.Fatal(err)
	}
	result := ApplyDesktopResultRecovery(dir, project, worker, *plan, proof, key, now)
	if result.Recovered != 2 || result.Remaining != 0 || len(result.Reasons) != 0 {
		t.Fatalf("exact receipt not applied atomically: %+v", result)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "journal.json"))
	if !bytes.Equal(raw, data) || !bytes.Contains(raw, []byte("9007199254740999")) {
		t.Fatal("non-ACK state or large number changed")
	}
	if _, err := os.Stat(temp); !os.IsNotExist(err) {
		t.Fatal("original replacement was not consumed")
	}
	info, _ := os.Stat(filepath.Join(dir, "journal.json"))
	after := renewal178Files(t, dir)
	again := ApplyDesktopResultRecovery(dir, project, worker, *plan, proof, key, now)
	if again.Recovered != 2 || again.Remaining != 0 {
		t.Fatal("lost apply response could not be recovered")
	}
	latest, _ := os.Stat(filepath.Join(dir, "journal.json"))
	if !latest.ModTime().Equal(info.ModTime()) {
		t.Fatal("idempotent apply rewrote original journal")
	}
	status = InspectDesktopResultRecovery(dir, project, worker, *plan, proof, key, now.Add(2*time.Minute))
	if status.Recovered != 2 || status.Remaining != 0 {
		t.Fatal("read-only status rejected originally valid expired proof")
	}
	expired := ApplyDesktopResultRecovery(dir, project, worker, *plan, proof, key, now.Add(2*time.Minute))
	if !recovery179Has(expired, "proof_expired") {
		t.Fatal("expired proof could write")
	}
	renewal178AssertUnchanged(t, dir, after)
}

func TestDesktopResultRecoveryRefusesUnapprovedChanges(t *testing.T) {
	for _, kind := range []string{"active-lock", "bad-MAC", "result-drift", "unproved-ACK", "unknown-temp"} {
		t.Run(kind, func(t *testing.T) {
			dir, project, worker, _ := resultRecovery179Fixture(t)
			plan := PrepareDesktopResultRecovery(dir, project, worker).Plan
			if plan == nil {
				t.Fatal("missing plan")
			}
			proof, key, now := resultRecovery179Proof(*plan, "matched", "missing")
			want := "state_changed"
			switch kind {
			case "active-lock":
				unlock, err := lockJournal(filepath.Join(dir, "journal.lock"))
				if err != nil {
					t.Fatal(err)
				}
				defer unlock()
				want = "active_worker"
			case "bad-MAC":
				proof.Signature = strings.Repeat("0", 64)
				want = "access_changed"
			case "result-drift":
				doc := recovery179Document(t, dir)
				doc[plan.Items[1].JournalKey].(map[string]any)["result"].(map[string]any)["output"] = "changed179"
				recovery179Write(t, dir, doc)
			case "unproved-ACK":
				doc := recovery179Document(t, dir)
				doc[plan.Items[1].JournalKey].(map[string]any)["acked"] = true
				recovery179Write(t, dir, doc)
			case "unknown-temp":
				if err := os.WriteFile(filepath.Join(dir, "journal-recovery-"+rc.PlanSHA256(*plan)+".tmp"), []byte("unique179-do-not-remove"), 0600); err != nil {
					t.Fatal(err)
				}
				want = "result_unknown"
			}
			// Reading journal.lock while its real Windows owner is active is denied.
			var before map[string]string
			if kind != "active-lock" {
				before = renewal178Files(t, dir)
			}
			result := ApplyDesktopResultRecovery(dir, project, worker, *plan, proof, key, now)
			if !recovery179Has(result, want) || result.Recovered != 0 {
				t.Fatalf("%s escaped precise checks: %+v", kind, result)
			}
			if kind != "active-lock" {
				renewal178AssertUnchanged(t, dir, before)
			}
		})
	}
}

func TestDesktopResultRecoveryLeavesMissingAndUnknownFacts(t *testing.T) {
	dir, project, worker, _ := resultRecovery179Fixture(t)
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	unfinished := Assignment{RunID: "unfinished179", NodeID: "inbox", WorkerID: worker, ProjectID: project, Attempt: 1, Fence: 8}
	if err = j.Accept(unfinished); err != nil {
		t.Fatal(err)
	}
	if _, err = j.NewInventory(worker, nil); err != nil {
		t.Fatal(err)
	}
	_ = j.Close()
	doc := recovery179Document(t, dir)
	doc["run179/one/1/9007199254740997"].(map[string]any)["result"].(map[string]any)["outcome_unknown"] = true
	recovery179Write(t, dir, doc)
	before := renewal178Files(t, dir)
	prepared := PrepareDesktopResultRecovery(dir, project, worker)
	if prepared.Plan == nil || prepared.Total != 2 || !recovery179Has(prepared, "unsupported_pending") {
		t.Fatal("unsupported original facts not preserved")
	}
	renewal178AssertUnchanged(t, dir, before)
	proof, key, now := resultRecovery179Proof(*prepared.Plan, "matched", "missing")
	result := ApplyDesktopResultRecovery(dir, project, worker, *prepared.Plan, proof, key, now)
	if result.Recovered != 1 || result.Remaining != 1 || !recovery179Has(result, "receipt_missing") || !recovery179Has(result, "unsupported_pending") {
		t.Fatalf("missing receipt invented: %+v", result)
	}
	current := recovery179Document(t, dir)
	if current[keyForRecovery179(unfinished)].(map[string]any)["result"] != nil {
		t.Fatal("recovery generated a result for unfinished inbox")
	}
	if current[prepared.Plan.Items[0].JournalKey].(map[string]any)["result"].(map[string]any)["outcome_unknown"] != true {
		t.Fatal("transport ACK erased unknown execution")
	}
	if current[prepared.Plan.Items[1].JournalKey].(map[string]any)["acked"] != false {
		t.Fatal("missing receipt was acknowledged")
	}
	sidecar, _ := os.ReadFile(filepath.Join(dir, "tool-inventory.json"))
	if digest(sidecar) != before["tool-inventory.json"] {
		t.Fatal("recovery modified sidecar")
	}
	if InspectDesktopRunnerRenewalState(dir).Safe {
		t.Fatal("remaining execution was bypassed by recovery")
	}
}
func keyForRecovery179(a Assignment) string { return key(a) }
