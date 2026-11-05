package platform

import (
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
)

const repositoryPoolRunnerExecutableEnv = "SUMA_TEST_REPOSITORY_POOL_RUNNER"

type repositoryPoolProcess struct {
	cmd   *exec.Cmd
	state string
	done  chan struct{}
}

func startRepositoryPoolProcess(t *testing.T, executable string, client *runner.Client, tenant, project string, config runner.ExecConfig) repositoryPoolProcess {
	t.Helper()
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	local := runner.PoolLocalConfig{
		SchemaVersion: 1, PoolID: client.Pool.PoolID, RunnerID: client.Pool.RunnerID,
		Slots: 1, PollIntervalMS: 200, HeartbeatIntervalMS: 500,
		Scopes: []runner.PoolLocalScope{{
			TenantID: tenant, ProjectID: project, Workspace: config.Workspace, Repository: config.Repository,
			BaseCommit: config.BaseCommit, Worktrees: config.WorktreeRoot, Commands: config.Commands,
		}},
	}
	raw, err := json.Marshal(local)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "pool.json")
	if err = os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "--control", client.BaseURL, "--pool-config", configPath, "--state", state, "--allow-exec")
	cmd.Dir = dir
	// Only inherit system paths. Fixture credentials must never reach the other Runner.
	cmd.Env = []string{"SUMA_POOL_RUNNER_TOKEN=" + client.Token}
	for _, key := range []string{"PATH", "PATHEXT", "SystemRoot", "WINDIR", "COMSPEC", "TEMP", "TMP", "USERPROFILE", "HOME", "APPDATA", "LOCALAPPDATA", "ProgramData", "ProgramFiles", "ProgramFiles(x86)"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	process := repositoryPoolProcess{cmd: cmd, state: filepath.Join(state, "pool"), done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(process.done)
	}()
	return process
}

func (process repositoryPoolProcess) stop(t *testing.T) {
	t.Helper()
	// Windows does not forward a private console interrupt through os.Process.
	// On success the fixture confirms idle, then terminates only its own CLI.
	_ = process.cmd.Process.Kill()
	select {
	case <-process.done:
	case <-time.After(5 * time.Second):
		t.Errorf("test Runner PID %d did not exit after termination", process.cmd.Process.Pid)
	}
}

func (process repositoryPoolProcess) resultAcknowledged() bool {
	status := runner.InspectPoolState(process.state)
	if status.Status != "available" || !status.Complete || len(status.Slots) != 1 || status.Slots[0].SlotID != "0" {
		return false
	}
	counts := status.Slots[0].JournalCounts
	return counts != nil && counts.AcknowledgedResults == 1 && counts.PendingResults == 0 && counts.UnknownResults == 0 &&
		counts.PendingProgress == 0 && counts.UnresolvedInbox == 0 && counts.PendingArtifacts == 0 && counts.PendingCandidates == 0
}

// runRepositoryPoolProcesses is an opt-in extension of the loopback delivery
// fixture. Each node is claimed and executed by a different production CLI.
func runRepositoryPoolProcesses(t *testing.T, ctx context.Context, f *fixture, runID string, intake TaskDeliveryRepositoryIntake, producer *runner.Client, producerConfig runner.ExecConfig, independent *runner.Client, independentConfig runner.ExecConfig) {
	t.Helper()
	executable := os.Getenv(repositoryPoolRunnerExecutableEnv)
	info, err := os.Stat(executable)
	if !filepath.IsAbs(executable) || err != nil || !info.Mode().IsRegular() {
		t.Fatalf("%s must name an absolute path to a freshly built suma-runner", repositoryPoolRunnerExecutableEnv)
	}
	control, err := url.Parse(producer.BaseURL)
	if err != nil || control.Scheme != "http" || control.User != nil || !net.ParseIP(control.Hostname()).IsLoopback() || independent.BaseURL != producer.BaseURL {
		t.Fatal("real Runner fixture must use the same private loopback control plane")
	}
	if producer.Pool == nil || independent.Pool == nil || producer.Pool.PoolID != independent.Pool.PoolID ||
		producer.Pool.RunnerID == independent.Pool.RunnerID || producer.Token == "" || independent.Token == "" || producer.Token == independent.Token {
		t.Fatal("real Runner fixture must use distinct identities and credentials in the same pool")
	}
	producerProcess := startRepositoryPoolProcess(t, executable, producer, f.user.TenantID, f.project, producerConfig)
	defer producerProcess.stop(t)
	testProcess := startRepositoryPoolProcess(t, executable, independent, f.user.TenantID, f.project, independentConfig)
	defer testProcess.stop(t)
	producerPID, testerPID := producerProcess.cmd.Process.Pid, testProcess.cmd.Process.Pid
	if producerPID == testerPID || producerPID == os.Getpid() || testerPID == os.Getpid() {
		t.Fatal("expected two distinct child Runner processes")
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	var run dag.Run
	for {
		run, err = f.e.GetRun(ctx, f.user.TenantID, f.project, runID)
		if err != nil {
			t.Fatal(err)
		}
		switch run.State {
		case "failed", "cancelled", "canceled":
			for _, node := range run.Nodes {
				t.Logf("node=%s state=%s reason=%s", node.ID, node.State, node.Reason)
			}
			t.Fatalf("repository delivery ended in %s", run.State)
		}
		if run.State == "succeeded" && producerProcess.resultAcknowledged() && testProcess.resultAcknowledged() {
			break
		}
		select {
		case <-producerProcess.done:
			t.Fatalf("producer Runner PID %d exited before acknowledging results", producerPID)
		case <-testProcess.done:
			t.Fatalf("independent Runner PID %d exited before acknowledging results", testerPID)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for both real Runners, final observed run state %s", run.State)
		case <-ticker.C:
		}
	}

	poolID := producer.Pool.PoolID
	if run.ExecutionPoolID != poolID || run.ExecutionPoolGrantID == "" || run.ExecutionPoolGrantVersion < 1 || len(run.Nodes) != 2 {
		t.Fatal("completed repository delivery did not retain the exact pool grant and two nodes")
	}
	assignments := repositoryPoolProcessAssignments(t, ctx, f, run, producer.Pool.RunnerID, independent.Pool.RunnerID)
	source := assignments[repositorySourceProducerNode]
	check := assignments[repositorySourceTestNode]
	if source.PoolRunnerID != producer.Pool.RunnerID || check.PoolRunnerID != independent.Pool.RunnerID || source.PoolCredentialID == check.PoolCredentialID || source.DispatchID == check.DispatchID || check.CandidateCommit != intake.HeadCommit {
		t.Fatal("production dispatches did not retain distinct identities and the approved candidate")
	}
	var sealed cc.Binding
	var imported cc.ImportReceipt
	for _, node := range run.Nodes {
		assignment := assignments[node.ID]
		if node.State != "succeeded" || node.WorkerID != assignment.WorkerID || node.Attempt != assignment.Attempt || node.Fence != assignment.Fence || node.WorkspaceEvidence["head_commit"] != intake.HeadCommit {
			t.Fatalf("node %s did not persist the exact dispatch and commit", node.ID)
		}
		if node.ID == repositorySourceProducerNode {
			sealed = repositoryPoolProcessEvidence[cc.Binding](t, node.WorkspaceEvidence["candidate_bundle"])
		} else {
			imported = repositoryPoolProcessEvidence[cc.ImportReceipt](t, node.WorkspaceEvidence["candidate_import"])
		}
	}
	if !sealed.ValidSealed() || sealed.BaseCommit != intake.BaseCommit || sealed.HeadCommit != intake.HeadCommit || sealed.BundleSHA256 != intake.BundleSHA256 || sealed.SizeBytes != intake.SizeBytes ||
		sealed.Source.DispatchID != source.DispatchID || sealed.Source.PoolRunnerID != source.PoolRunnerID || sealed.Source.PoolSlotID != source.PoolSlotID || sealed.Source.PoolGrantID != source.PoolGrantID || sealed.Source.PoolGrantVersion != source.PoolGrantVersion ||
		!imported.Imported || imported.TargetWorkerID != check.WorkerID || imported.Binding != sealed {
		t.Fatal("independent process did not import the exact sealed source bundle")
	}
	var active int
	err = f.s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM dag_execution_pool_slots WHERE pool_id=? AND runner_id IN (?,?)`, poolID, source.PoolRunnerID, check.PoolRunnerID).Scan(&active)
	if err != nil || active != 0 {
		t.Fatalf("pool retained active slots after both acknowledgements: %d (%v)", active, err)
	}
	t.Logf("real_pool_run=%s pool=%s grant=%s/%d producer_pid=%d producer_runner=%s producer_dispatch=%s tester_pid=%d tester_runner=%s tester_dispatch=%s head=%s bundle_sha256=%s active_slots=%d shutdown=terminate_after_idle",
		run.ID, poolID, run.ExecutionPoolGrantID, run.ExecutionPoolGrantVersion, producerPID, source.PoolRunnerID, source.DispatchID, testerPID, check.PoolRunnerID, check.DispatchID, intake.HeadCommit, intake.BundleSHA256, active)
}

func repositoryPoolProcessAssignments(t *testing.T, ctx context.Context, f *fixture, run dag.Run, producerID, testerID string) map[string]dag.Assignment {
	t.Helper()
	rows, err := f.s.DB.QueryContext(ctx, `SELECT assignment FROM dag_execution_pool_dispatches WHERE pool_id=? AND runner_id IN (?,?) AND assignment<>'null'`, run.ExecutionPoolID, producerID, testerID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	assignments := make(map[string]dag.Assignment, 2)
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var assignment dag.Assignment
		if err = json.Unmarshal(raw, &assignment); err != nil {
			t.Fatal(err)
		}
		if assignment.RunID != run.ID {
			continue
		}
		if _, exists := assignments[assignment.NodeID]; exists || assignment.DispatchID == "" || assignment.PoolSlotID != "0" ||
			assignment.PoolID != run.ExecutionPoolID || assignment.TenantID != run.TenantID || assignment.ProjectID != run.ProjectID ||
			assignment.PoolGrantID != run.ExecutionPoolGrantID || assignment.PoolGrantVersion != run.ExecutionPoolGrantVersion ||
			assignment.PoolCredentialID == "" || assignment.PoolCredentialVersion < 1 {
			t.Fatalf("invalid or duplicated durable dispatch for node %s", assignment.NodeID)
		}
		assignments[assignment.NodeID] = assignment
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 2 {
		t.Fatalf("expected exactly two durable real Runner dispatches, got %d", len(assignments))
	}
	return assignments
}

func repositoryPoolProcessEvidence[T any](t *testing.T, evidence any) T {
	t.Helper()
	var out T
	raw, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
