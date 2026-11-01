package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
)

func renewal178Reason(check DesktopRunnerRenewalCheck, want string) bool {
	for _, reason := range check.Reasons {
		if reason == want {
			return true
		}
	}
	return false
}
func renewal178Files(t *testing.T, dir string) map[string]string {
	t.Helper()
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]string{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		result[file.Name()] = digest(raw)
	}
	return result
}
func renewal178AssertUnchanged(t *testing.T, dir string, before map[string]string) {
	t.Helper()
	after := renewal178Files(t, dir)
	raw, _ := json.Marshal(before)
	next, _ := json.Marshal(after)
	if string(raw) != string(next) {
		t.Fatal("renewal inspection changed original state")
	}
}

func TestDesktopRunnerRenewalCheckUsesOriginalLockWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	for _, path := range []string{dir, filepath.Join(dir, "not-created")} {
		got := InspectDesktopRunnerRenewalState(path)
		if !got.Safe || len(got.Reasons) != 0 {
			t.Fatalf("never-started state unsafe: %+v", got)
		}
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 0 {
		t.Fatal("inspection created state or a lock")
	}
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	got := InspectDesktopRunnerRenewalState(dir)
	if got.Safe || !renewal178Reason(got, "active_worker") {
		t.Fatalf("active original lock not respected: %+v", got)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	before := renewal178Files(t, dir)
	got = InspectDesktopRunnerRenewalState(dir)
	if !got.Safe || len(got.Reasons) != 0 {
		t.Fatalf("drained journal rejected: %+v", got)
	}
	renewal178AssertUnchanged(t, dir, before)
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := Assignment{RunID: "finished178", NodeID: "node178", WorkerID: "worker178", Attempt: 1, Fence: 2}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	r := Result{Assignment: a, OperationID: strings.Repeat("2", 32)}
	if err = j.Finish(r); err != nil {
		t.Fatal(err)
	}
	if err = j.Ack(r.OperationID); err != nil {
		t.Fatal(err)
	}
	_ = j.Close()
	before = renewal178Files(t, dir)
	got = InspectDesktopRunnerRenewalState(dir)
	if !got.Safe {
		t.Fatalf("known acknowledged completion was not drained: %+v", got)
	}
	renewal178AssertUnchanged(t, dir, before)
}

func TestDesktopRunnerRenewalCheckPreservesUnknownAndPendingReceipts(t *testing.T) {
	for _, kind := range []string{"unfinished-inbox", "unknown-acked", "pending-inventory", "pending-claim"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			j, err := OpenJournal(dir)
			if err != nil {
				t.Fatal(err)
			}
			want := "pending_receipt"
			switch kind {
			case "unfinished-inbox", "unknown-acked":
				a := Assignment{RunID: "run178", NodeID: "node178", WorkerID: "worker178", Attempt: 1, Fence: 2}
				if err = j.Accept(a); err != nil {
					t.Fatal(err)
				}
				want = "pending_execution"
				if kind == "unknown-acked" {
					r := Result{Assignment: a, OperationID: strings.Repeat("1", 32), OutcomeUnknown: true, ExitCode: -1}
					if err = j.Finish(r); err != nil {
						t.Fatal(err)
					}
					if err = j.Ack(r.OperationID); err != nil {
						t.Fatal(err)
					}
					want = "unknown_outcome"
				}
			case "pending-inventory":
				_, err = j.NewInventory("http://127.0.0.1:44178", "project178", "worker178", nil)
			case "pending-claim":
				_, err = j.BeginClaim("http://127.0.0.1:44178", "project178", "worker178", nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = j.Close(); err != nil {
				t.Fatal(err)
			}
			before := renewal178Files(t, dir)
			got := InspectDesktopRunnerRenewalState(dir)
			if got.Safe || !renewal178Reason(got, want) {
				t.Fatalf("%s was discarded: %+v", kind, got)
			}
			renewal178AssertUnchanged(t, dir, before)
		})
	}
	t.Run("permission-outbox", func(t *testing.T) {
		a, p, request, permission := codexPermissionFixture174(t)
		j, dir := savedCodexPermission174(t, a, p, request, permission)
		observation := cc.CodexApprovalObservation{Sequence: 1, ParamsSHA256: request.Codex.ParamsSHA256, Decision: "accept", DecisionObserved: true, ReplyStarted: true, ReplyWritten: true, RequestResolved: true, ItemStatus: "completed"}
		if err := j.recordToolPermissionObservation(a, request, permission.ID, observation, false); err != nil {
			t.Fatal(err)
		}
		r := Result{Assignment: a, OperationID: strings.Repeat("3", 32)}
		if err := j.Finish(r); err != nil {
			t.Fatal(err)
		}
		if err := j.Ack(r.OperationID); err != nil {
			t.Fatal(err)
		}
		_ = j.Close()
		before := renewal178Files(t, dir)
		got := InspectDesktopRunnerRenewalState(dir)
		if got.Safe || !renewal178Reason(got, "pending_receipt") {
			t.Fatalf("permission receipt hidden by acknowledged main result: %+v", got)
		}
		renewal178AssertUnchanged(t, dir, before)
	})
}

func TestDesktopRunnerRenewalCheckRejectsDamagedOrUnknownState(t *testing.T) {
	for _, kind := range []string{"unknown-file", "damaged-journal", "missing-lock"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			j, err := OpenJournal(dir)
			if err != nil {
				t.Fatal(err)
			}
			_ = j.Close()
			switch kind {
			case "unknown-file":
				err = os.WriteFile(filepath.Join(dir, "unknown-private.json"), []byte("private178"), 0600)
			case "damaged-journal":
				err = os.WriteFile(filepath.Join(dir, "journal.json"), []byte(`{"duplicate":{},"duplicate":{}}`), 0600)
			case "missing-lock":
				err = os.Remove(filepath.Join(dir, "journal.lock"))
			}
			if err != nil {
				t.Fatal(err)
			}
			before := renewal178Files(t, dir)
			got := InspectDesktopRunnerRenewalState(dir)
			if got.Safe || len(got.Reasons) == 0 {
				t.Fatalf("unverifiable state accepted: %+v", got)
			}
			raw, _ := json.Marshal(got)
			if strings.Contains(string(raw), "private178") || strings.Contains(string(raw), dir) {
				t.Fatal("private state escaped result")
			}
			renewal178AssertUnchanged(t, dir, before)
		})
	}
}
