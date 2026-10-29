package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func workBuddyJournalFixture(t *testing.T) (*Journal, Assignment, WorkBuddyBinding) {
	t.Helper()
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	a := Assignment{TenantID: "tenant", ProjectID: "project", RunID: "run", NodeID: "develop", Attempt: 1, Fence: 7, WorkerID: "worker", Type: "agent", Command: []string{"cloud-task"}, Config: map[string]any{"fixed": "value"}}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	return j, a, WorkBuddyBinding{RequestSHA256: strings.Repeat("a", 64), CredentialRef: "credential-reference", CredentialVersion: 2, Endpoint: "https://www.workbuddy.cn/openapi/v2/tasks"}
}

func TestWorkBuddyJournalRestartPreservesSubmissionDisposition(t *testing.T) {
	for _, known := range []bool{false, true} {
		t.Run(map[bool]string{false: "prepared", true: "task_known"}[known], func(t *testing.T) {
			j, a, b := workBuddyJournalFixture(t)
			first, err := j.BeginWorkBuddySubmission(a, b)
			if err != nil || !first {
				t.Fatal(first, err)
			}
			if known {
				if err = j.RecordWorkBuddyTask(a, b, "task_123"); err != nil {
					t.Fatal(err)
				}
			}
			first, err = j.BeginWorkBuddySubmission(a, b)
			if err != nil || first {
				t.Fatal("repeat authorized another submission", first, err)
			}
			dir := filepath.Dir(j.path)
			if err = j.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenJournal(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			snapshot, exists, err := reopened.WorkBuddySubmission(a)
			if err != nil || !exists || snapshot.Binding != b {
				t.Fatal(snapshot, exists, err)
			}
			if known && snapshot.TaskID != "task_123" || !known && (snapshot.TaskID != "" || snapshot.State != "prepared") {
				t.Fatal(snapshot)
			}
			if first, _ = reopened.BeginWorkBuddySubmission(a, b); first {
				t.Fatal("restart authorized another POST")
			}
			if reopened.entries[key(a)].Result == nil || !reopened.entries[key(a)].Result.OutcomeUnknown {
				t.Fatal("checkpoint bypassed existing restart reconciliation")
			}
			snapshot.Binding.CredentialRef = "mutated"
			again, _, _ := reopened.WorkBuddySubmission(a)
			if again.Binding != b {
				t.Fatal("snapshot aliases journal")
			}
		})
	}
}

func TestWorkBuddyJournalConcurrentBeginHasOneWinner(t *testing.T) {
	j, a, b := workBuddyJournalFixture(t)
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			first, err := j.BeginWorkBuddySubmission(a, b)
			if err != nil {
				t.Error(err)
			}
			if first {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatal("submission winners", winners.Load())
	}
}

func TestWorkBuddyJournalRejectsFullEnvelopeAndBindingDrift(t *testing.T) {
	j, a, b := workBuddyJournalFixture(t)
	if _, err := j.BeginWorkBuddySubmission(a, b); err != nil {
		t.Fatal(err)
	}
	assignments := []func(*Assignment){func(x *Assignment) { x.TenantID = "foreign" }, func(x *Assignment) { x.ProjectID = "foreign" }, func(x *Assignment) { x.WorkerID = "foreign" }, func(x *Assignment) { x.PoolGrantID = "foreign" }, func(x *Assignment) { x.PoolCredentialVersion = 3 }, func(x *Assignment) { x.DispatchID = "foreign" }, func(x *Assignment) { x.Command = []string{"other"} }, func(x *Assignment) { x.Config = map[string]any{"fixed": "changed"} }}
	for i, mutate := range assignments {
		foreign := a
		mutate(&foreign)
		if _, err := j.BeginWorkBuddySubmission(foreign, b); err == nil {
			t.Fatalf("scope drift %d accepted", i)
		}
		if _, _, err := j.WorkBuddySubmission(foreign); err == nil {
			t.Fatalf("foreign read %d accepted", i)
		}
	}
	bindings := []func(*WorkBuddyBinding){func(x *WorkBuddyBinding) { x.RequestSHA256 = strings.Repeat("b", 64) }, func(x *WorkBuddyBinding) { x.CredentialRef = "other" }, func(x *WorkBuddyBinding) { x.CredentialVersion++ }, func(x *WorkBuddyBinding) { x.Endpoint = "http://127.0.0.1:43959/openapi/v2/tasks" }}
	for i, mutate := range bindings {
		foreign := b
		mutate(&foreign)
		if _, err := j.BeginWorkBuddySubmission(a, foreign); err == nil {
			t.Fatalf("binding drift %d accepted", i)
		}
		if j.RecordWorkBuddyTask(a, foreign, "task_123") == nil {
			t.Fatal("foreign binding recorded task")
		}
	}
}

func TestWorkBuddyJournalTaskIDIsImmutableAndRequiresPreparation(t *testing.T) {
	j, a, b := workBuddyJournalFixture(t)
	if j.RecordWorkBuddyTask(a, b, "task_123") == nil {
		t.Fatal("unprepared task recorded")
	}
	if _, err := j.BeginWorkBuddySubmission(a, b); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "../task", "task?token=secret", strings.Repeat("x", 201)} {
		if j.RecordWorkBuddyTask(a, b, id) == nil {
			t.Fatal("invalid task ID accepted")
		}
	}
	if err := j.RecordWorkBuddyTask(a, b, "task_123"); err != nil {
		t.Fatal(err)
	}
	if err := j.RecordWorkBuddyTask(a, b, "task_123"); err != nil {
		t.Fatal(err)
	}
	if j.RecordWorkBuddyTask(a, b, "task_other") == nil {
		t.Fatal("known task replaced")
	}
	if err := j.Finish(Result{Assignment: a, ExitCode: 0}); err != nil {
		t.Fatal(err)
	}
	if j.RecordWorkBuddyTask(a, b, "task_123") == nil {
		t.Fatal("final receipt accepted a new write")
	}
	if _, exists, err := j.WorkBuddySubmission(a); err != nil || !exists {
		t.Fatal("final safe snapshot unavailable", err)
	}
}

func TestWorkBuddyJournalRequiresDurableInboxAndRejectsOtherChannels(t *testing.T) {
	j, a, b := workBuddyJournalFixture(t)
	foreign := a
	foreign.RunID = "absent"
	if _, err := j.BeginWorkBuddySubmission(foreign, b); err == nil {
		t.Fatal("absent inbox accepted")
	}
	path := j.path
	j.path = filepath.Join(t.TempDir(), "missing", "journal.json")
	if j.Accept(foreign) == nil {
		t.Fatal("expected save failure")
	}
	j.path = path
	if _, err := j.BeginWorkBuddySubmission(foreign, b); err == nil {
		t.Fatal("failed Accept became durable inbox")
	}
	a.Command[0] = "mutated-after-accept"
	if _, err := j.BeginWorkBuddySubmission(a, b); err == nil {
		t.Fatal("caller changed shallow inbox")
	}
	a.Command[0] = "cloud-task"
	for _, channel := range []string{"result", "terminal", "model", "workspace", "artifact", "candidate"} {
		t.Run(channel, func(t *testing.T) {
			j, a, b := workBuddyJournalFixture(t)
			e := j.entries[key(a)]
			switch channel {
			case "result":
				e.Result = &Result{Assignment: a}
			case "terminal":
				e.Terminal = &terminalCheckpoint{}
			case "model":
				e.ModelGateway = &modelGatewayCheckpoint{}
			case "workspace":
				e.WorkspaceTrace = &workspaceTraceCheckpoint{}
			case "artifact":
				e.ArtifactPending = &artifactTransfer{}
			case "candidate":
				e.CandidatePending = &candidateTransfer{}
			}
			j.entries[key(a)] = e
			if err := j.save(); err != nil {
				t.Fatal(err)
			}
			if _, err := j.BeginWorkBuddySubmission(a, b); err == nil {
				t.Fatal("mixed or final channel admitted")
			}
		})
	}
}

func TestWorkBuddyJournalSaveFailureCannotIssueAnotherSubmission(t *testing.T) {
	j, a, b := workBuddyJournalFixture(t)
	// An unrelated invalid value fails j.save after the checkpoint was staged,
	// without bypassing the original entry's durable-inbox check.
	j.entries["unserializable"] = entry{Assignment: Assignment{Config: map[string]any{"unsupported": make(chan int)}}}
	if first, err := j.BeginWorkBuddySubmission(a, b); err == nil || first || j.entries[key(a)].WorkBuddy != nil {
		t.Fatal("failed durable Begin left a phantom checkpoint")
	}
	delete(j.entries, "unserializable")
	path := j.path
	j.path = filepath.Join(t.TempDir(), "missing", "journal.json")
	if first, err := j.BeginWorkBuddySubmission(a, b); err == nil || first {
		t.Fatal("undurable begin authorized POST")
	}
	j.path = path
	if first, err := j.BeginWorkBuddySubmission(a, b); err != nil || !first {
		t.Fatal(first, err)
	}
	j.entries["unserializable"] = entry{Assignment: Assignment{Config: map[string]any{"unsupported": make(chan int)}}}
	if j.RecordWorkBuddyTask(a, b, "task_123") == nil {
		t.Fatal("expected save failure after staging task ID")
	}
	if j.entries[key(a)].WorkBuddy.State != "prepared" || j.entries[key(a)].WorkBuddy.TaskID != "" {
		t.Fatal("failed save mutated shared previous pointer")
	}
	delete(j.entries, "unserializable")
	j.path = filepath.Join(t.TempDir(), "missing", "journal.json")
	if j.RecordWorkBuddyTask(a, b, "task_123") == nil {
		t.Fatal("expected task checkpoint failure")
	}
	j.path = path
	snapshot, _, err := j.WorkBuddySubmission(a)
	if err != nil || snapshot.State != "prepared" || snapshot.TaskID != "" {
		t.Fatal("failed save replaced durable checkpoint", snapshot, err)
	}
	if first, err := j.BeginWorkBuddySubmission(a, b); err != nil || first {
		t.Fatal("failed receipt allowed another POST", first, err)
	}
}

func TestWorkBuddyJournalPoolEnvelopeDriftIsRejected(t *testing.T) {
	j, a, b := workBuddyJournalFixture(t)
	a.RunID = "pool-run"
	a.PoolID = "pool"
	a.PoolRunnerID = "pool-runner"
	a.PoolSlotID = "0"
	a.PoolGrantID = "grant"
	a.PoolGrantVersion = 1
	a.PoolCredentialID = "credential"
	a.PoolCredentialVersion = 1
	a.DispatchID = "dispatch"
	a.DispatchState = "active"
	a.LeaseUntil = "2030-01-01T00:00:00Z"
	if err := j.Accept(a); err != nil {
		t.Fatal(err)
	}
	if first, err := j.BeginWorkBuddySubmission(a, b); err != nil || !first {
		t.Fatal(first, err)
	}
	for _, change := range []func(*Assignment){func(x *Assignment) { x.PoolGrantID = "other" }, func(x *Assignment) { x.PoolGrantVersion++ }, func(x *Assignment) { x.LeaseUntil = "2031-01-01T00:00:00Z" }, func(x *Assignment) { x.DispatchState = "reconciling" }, func(x *Assignment) { x.PoolRunnerID = "other" }, func(x *Assignment) { x.PoolSlotID = "1" }} {
		foreign := a
		change(&foreign)
		if first, err := j.BeginWorkBuddySubmission(foreign, b); err == nil || first {
			t.Fatal("pool envelope drift accepted", first, err)
		}
	}
}

func TestWorkBuddyJournalDiskMemoryDivergenceRequiresReopen(t *testing.T) {
	j, a, b := workBuddyJournalFixture(t)
	if _, err := j.BeginWorkBuddySubmission(a, b); err != nil {
		t.Fatal(err)
	}
	// Simulate replace succeeding but the following directory sync failing:
	// disk can contain the newer receipt while memory rolled back to prepared.
	e := j.entries[key(a)]
	cp := *e.WorkBuddy
	cp.State = "task_known"
	cp.TaskID = "task_123"
	e.WorkBuddy = &cp
	raw, err := json.Marshal(map[string]entry{key(a): e})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(j.path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if first, err := j.BeginWorkBuddySubmission(a, b); err == nil || first {
		t.Fatal("divergent state authorized submission")
	}
	if _, _, err = j.WorkBuddySubmission(a); err == nil {
		t.Fatal("divergent state advertised as durable")
	}
	dir := filepath.Dir(j.path)
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	known, exists, err := reopened.WorkBuddySubmission(a)
	if err != nil || !exists || known.TaskID != "task_123" {
		t.Fatal(known, exists, err)
	}
}

func TestWorkBuddyJournalCorruptionFailsClosed(t *testing.T) {
	for _, field := range []string{"state", "assignment_sha256", "task_id", "version", "credential", "extra"} {
		t.Run(field, func(t *testing.T) {
			j, a, b := workBuddyJournalFixture(t)
			if _, err := j.BeginWorkBuddySubmission(a, b); err != nil {
				t.Fatal(err)
			}
			path := j.path
			_ = j.Close()
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var all map[string]map[string]any
			if err = json.Unmarshal(raw, &all); err != nil {
				t.Fatal(err)
			}
			cp := all[key(a)]["workbuddy"].(map[string]any)
			switch field {
			case "state":
				cp["state"] = "submitted"
			case "assignment_sha256":
				cp[field] = strings.Repeat("b", 64)
			case "task_id":
				cp[field] = "task-without-known-state"
			case "version":
				cp[field] = 2
			case "credential":
				cp["binding"].(map[string]any)["credential_version"] = 0
			case "extra":
				cp["prompt"] = "must not persist"
			}
			changed, _ := json.Marshal(all)
			if err = os.WriteFile(path, changed, 0600); err != nil {
				t.Fatal(err)
			}
			if reopened, err := OpenJournal(filepath.Dir(path)); err == nil {
				reopened.Close()
				t.Fatal("corrupt checkpoint accepted")
			}
		})
	}
}
