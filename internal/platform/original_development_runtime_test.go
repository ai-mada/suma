package platform

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
)

// originalDevelopmentRuntime shares only the real platform/Worker/quality
// plumbing. Each adapter still owns its native fixture and interaction logic.
type originalDevelopmentRuntime struct {
	f         *fixture
	control   *httptest.Server
	directory string
	receipt   map[string]any
}

func (h originalDevelopmentRuntime) startWorker(t *testing.T, ctx context.Context, id, token, journalName string, cfg runner.ExecConfig) func() {
	t.Helper()
	f, control, directory, receipt := h.f, h.control, h.directory, h.receipt
	journal, e := runner.OpenJournal(filepath.Join(directory, journalName))
	if e != nil {
		t.Fatal(e)
	}
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	worker := &runner.Worker{Client: &runner.Client{BaseURL: control.URL, ProjectID: f.project, Token: token, HTTP: control.Client()}, Journal: journal, ID: id, Exec: cfg, PollInterval: 75 * time.Millisecond, HeartbeatInterval: time.Second}
	go func() { done <- worker.Run(workerCtx) }()
	return func() {
		stop()
		select {
		case e := <-done:
			if e != nil && !errors.Is(e, context.Canceled) {
				t.Error("original Worker exit", e)
			}
		case <-time.After(8 * time.Second):
			t.Error("original Worker did not stop")
		}
		if len(journal.Pending()) != 0 || len(journal.PendingProgress()) != 0 {
			replayCtx, replayCancel := context.WithTimeout(context.Background(), 5*time.Second)
			e = runner.Replay(replayCtx, worker.Client, journal)
			replayCancel()
			if e != nil {
				t.Error("original Worker outbox recovery", e)
			}
		}
		receipt[journalName+"_pending"] = len(journal.Pending())
		receipt[journalName+"_progress_pending"] = len(journal.PendingProgress())
		if len(journal.Pending()) != 0 || len(journal.PendingProgress()) != 0 {
			t.Error("original Worker retained unacknowledged facts")
		}
		if e := journal.Close(); e != nil {
			t.Error(e)
		}
	}

}

type originalDevelopmentQuality struct {
	Repository, BaseCommit, CandidateCommit, ExpectedDraft string
	Task                                                   Record
	Plan                                                   TestPlanView
	Run                                                    dag.Run
	Producer                                               dag.NodeRun
	TestProfile                                            ToolProfile
	TestToken                                              string
	Independent                                            runner.ExecConfig
	Reviewer                                               Actor
}

// finishQuality proves the same sealed commit on another original Worker and
// JUnit source before exercising a separate isolated human review identity.
func (h originalDevelopmentRuntime) finishQuality(t *testing.T, ctx context.Context, scope originalDevelopmentQuality) (dag.NodeRun, TestPlanExecution, Record) {
	t.Helper()
	f, repo, base, candidate := h.f, scope.Repository, scope.BaseCommit, scope.CandidateCommit
	task, quality, run, produced := scope.Task, scope.Plan, scope.Run, scope.Producer
	testProfile, testToken, independent, reviewer := scope.TestProfile, scope.TestToken, scope.Independent, scope.Reviewer
	receipt := h.receipt
	startWorker := func(id, token, journalName string, cfg runner.ExecConfig) func() {
		return h.startWorker(t, ctx, id, token, journalName, cfg)
	}
	currentTask, err := f.s.Get(ctx, reviewer, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.Review(ctx, reviewer, f.project, task.ID, f.body(map[string]any{"expected_version": currentTask.Version, "run_id": run.ID, "decision": "accepted", "reason": "Must still reject before independent candidate tests"}), f.e)
	if err == nil {
		t.Fatal("producer self-certified independent quality")
	}
	stopTest := startWorker(testProfile.WorkerID, testToken, "independent-journal", independent)
	defer func() {
		if stopTest != nil {
			stopTest()
		}
	}()
	var verified dag.NodeRun
	for {
		current, e := f.e.GetRun(ctx, f.user.TenantID, f.project, run.ID)
		if e != nil {
			t.Fatal(e)
		}
		for _, n := range current.Nodes {
			if n.ID == "test" {
				verified = n
			}
		}
		if current.State == "succeeded" {
			break
		}
		if verified.State == "failed" || verified.State == "reconciling" || ctx.Err() != nil {
			receipt["independent"] = verified
			t.Fatal("independent exact-candidate worker did not succeed", verified.State, verified.Error)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(75 * time.Millisecond):
		}
	}
	stopTest()
	stopTest = nil
	if verified.WorkerID != testProfile.WorkerID || verified.WorkerID == produced.WorkerID || verified.WorkspaceEvidence["head_commit"] != candidate || verified.WorkspaceEvidence["base_commit"] != candidate || verified.WorkspaceEvidence["clean"] != true || verified.WorkspaceEvidence["attempt_key"] == produced.WorkspaceEvidence["attempt_key"] {
		t.Fatal("independent source differs from sealed candidate")
	}
	execution := issueRepairRuntimeDecode[TestPlanExecution](t, f.call("GET", "/test-plans/"+quality.ID+"/executions/"+run.ID+"?project_id="+f.project, nil, 200))
	if execution.State != "passed" || !execution.BasisCurrent || execution.Passed != 1 || !reflect.DeepEqual(execution.SourceCommits, []string{candidate}) || len(execution.Cases) != 1 || len(execution.Cases[0].Evidence) != 1 || execution.Cases[0].Evidence[0].WorkerID != testProfile.WorkerID {
		t.Fatal("original JUnit contract not bound to independent exact candidate", execution)
	}
	currentTask, err = f.s.Get(ctx, reviewer, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := f.s.Review(ctx, reviewer, f.project, task.ID, f.body(map[string]any{"expected_version": currentTask.Version, "run_id": run.ID, "decision": "accepted", "reason": "Isolated fixture reviewer explicitly exercises the exact candidate/JUnit review contract; no customer or vendor acceptance"}), f.e)
	if err != nil || accepted.Status != "accepted" || reviewer.Projection().ID == f.user.ID {
		t.Fatal("independent fixture reviewer could not accept", err)
	}
	if workspaceGitTestCommand(t, repo, "rev-parse", "HEAD") != base {
		t.Fatal("administrator checkout HEAD changed")
	}
	if body, e := os.ReadFile(filepath.Join(repo, "file.txt")); e != nil || string(body) != scope.ExpectedDraft {
		t.Fatal("administrator checkout draft changed")
	}

	return verified, execution, accepted
}
