package platform

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	rp "github.com/ai-mada/suma/internal/repositoryprovider"
)

func TestTaskDeliveryRepositoryIntakeHTTPFreezesOriginalSource(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git is required for repository intake verification")
	}
	git, err = filepath.EvalSymlinks(git)
	if err != nil {
		t.Fatal(err)
	}
	h := newWorkBuddyTaskDispatchFixture(t)
	f := h.f
	f.handler = (&Server{Service: f.s, Engine: f.e, WorkBuddy: h.w}).Handler()
	a := terminalCandidateWaitingWorkBuddy(t, h)
	ctx := context.Background()
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", h.task.ID)
	if err != nil {
		t.Fatal(err)
	}

	const incomingRef, baselineRef = "refs/heads/delivery", "refs/heads/main"
	checkout := t.TempDir()
	workspaceGitTestCommand(t, checkout, "init", "--initial-branch=main", "--object-format=sha1")
	workspaceGitTestCommand(t, checkout, "commit", "--allow-empty", "-m", "Trusted baseline")
	base := workspaceGitTestCommand(t, checkout, "rev-parse", "HEAD")
	workspaceGitTestCommand(t, checkout, "switch", "-c", "delivery")
	workspaceGitTestWrite(t, checkout, "candidate.txt", "Repository intake fixture\n")
	workspaceGitTestCommand(t, checkout, "add", "--", "candidate.txt")
	workspaceGitTestCommand(t, checkout, "commit", "-m", "Exact registered head")
	head := workspaceGitTestCommand(t, checkout, "rev-parse", "HEAD")
	bare := filepath.Join(t.TempDir(), "intake.git")
	workspaceGitTestCommand(t, checkout, "clone", "--bare", "--no-hardlinks", checkout, bare)

	const providerID, alias = "repository-intake-provider", "incoming"
	const controlEnv, runnerEnv, reconcileEnv, exportEnv = "SUMA_TEST_INTAKE_CONTROL", "SUMA_TEST_INTAKE_RUNNER", "SUMA_TEST_INTAKE_RECONCILE", "SUMA_TEST_INTAKE_EXPORT"
	t.Setenv(controlEnv, strings.Repeat("c", 48))
	t.Setenv(runnerEnv, strings.Repeat("r", 48))
	t.Setenv(reconcileEnv, strings.Repeat("x", 48))
	t.Setenv(exportEnv, strings.Repeat("e", 48))
	provider, err := rp.New(rp.Config{
		ProviderID: providerID, TenantID: f.user.TenantID, ProjectID: f.project,
		ControlURL: "http://127.0.0.1:1", ControlTokenEnv: controlEnv, RunnerTokenEnv: runnerEnv,
		ReconcileTokenEnv: reconcileEnv, SourceExportTokenEnv: exportEnv,
		GitExecutable: git, StateDir: filepath.Join(t.TempDir(), "provider"),
		Targets: map[string]rp.TargetConfig{
			alias:      {BareRepository: bare, Ref: incomingRef, DeliveryBaseAlias: "baseline"},
			"baseline": {BareRepository: bare, Ref: baselineRef},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = provider.Close() })

	var reads, exports atomic.Int32
	var moveFence atomic.Bool
	var exported atomic.Pointer[[]byte]
	handler := provider.Handler()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		if r.URL.Path != "/source-exports" {
			handler.ServeHTTP(w, r)
			return
		}
		exports.Add(1)
		observed := httptest.NewRecorder()
		handler.ServeHTTP(observed, r)
		if observed.Code == 200 {
			bundle := bytes.Clone(observed.Body.Bytes())
			exported.Store(&bundle)
			if moveFence.Swap(false) {
				// Change the exact formal origin while the control request is out
				// of its transaction, just before returning the verified bundle.
				ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
				defer cancel()
				if _, err := f.s.DB.ExecContext(ctx, `UPDATE dag_nodes SET fence=fence+1 WHERE run_id=? AND id=?`, a.RunID, a.NodeID); err != nil {
					t.Errorf("move original WorkBuddy fence during provider export: %v", err)
				}
			}
		}
		for name, values := range observed.Header() {
			w.Header()[name] = values
		}
		w.WriteHeader(observed.Code)
		_, _ = w.Write(observed.Body.Bytes())
	}))
	t.Cleanup(remote.Close)
	f.s.RepositoryProviders = []RepositoryProviderConfig{{
		ID: providerID, TenantID: f.user.TenantID, ProjectID: f.project, TargetAliases: []string{alias},
		TokenHash: digest(os.Getenv(controlEnv)), Origin: remote.URL,
		ReconcileToken: os.Getenv(reconcileEnv), SourceExportToken: os.Getenv(exportEnv),
	}}
	target := issueRepairRuntimeDecode[RepositoryIntegrationTarget](t, f.call("POST", "/repository-integration-targets",
		RepositoryIntegrationTargetCreateRequest{ProjectID: f.project, Name: "Incoming source intake", ProviderID: providerID, TargetAlias: alias, IdempotencyKey: newID()}, 201))
	path := "/tasks/" + task.ID + "/delivery/repository-source/intakes"
	request := func(version int, key string) map[string]any {
		return map[string]any{
			"project_id": f.project, "task_version": version, "run_id": a.RunID,
			"target_id": target.ID, "target_version": target.Version,
			"expected_head": head, "expected_base": base, "idempotency_key": key,
		}
	}
	key := newID()
	beforeReads := reads.Load()
	f.call("POST", path, request(task.Version+1, newID()), 409)
	ownerCookie, ownerView := f.cookie, f.view
	for _, role := range []string{"viewer", "developer"} {
		email := "repository-intake-" + role + "@example.test"
		inv := governanceInvite(f, email, "member", []any{map[string]any{"project_id": f.project, "role": role}})
		governanceAccept(f, inv, email)
		f.call("POST", path, request(task.Version, newID()), 403)
		f.cookie, f.view = ownerCookie, ownerView
	}
	if reads.Load() != beforeReads || exports.Load() != 0 {
		t.Fatal("stale or unauthorized approval reached the repository provider")
	}
	wrongBase := request(task.Version, newID())
	wrongBase["expected_base"] = strings.Repeat("a", 40)
	f.call("POST", path, wrongBase, 409)
	if reads.Load() != beforeReads+1 || exports.Load() != 0 {
		t.Fatal("caller-supplied baseline reached the binary export")
	}

	beforeRuns, beforeAttempts := domainACount(t, f.s, "dag_runs"), domainACount(t, f.s, "dag_attempts")
	beforeTransfers := domainACount(t, f.s, "suma_candidate_transfers")
	moveFence.Store(true)
	f.call("POST", path, request(task.Version, newID()), 409)
	if moveFence.Load() || exports.Load() != 1 || domainACount(t, f.s, "suma_repository_source_intakes") != 0 {
		t.Fatal("stale WorkBuddy origin was approved after provider export")
	}
	root, err := f.s.artifactRoot()
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if entries, err := os.ReadDir(filepath.Join(root.Name(), "blobs")); err != nil || len(entries) != 0 {
		t.Fatalf("stale export retained temporary bytes: %v", err)
	}
	if _, err := f.s.DB.ExecContext(ctx, `UPDATE dag_nodes SET fence=? WHERE run_id=? AND id=?`, a.Fence, a.RunID, a.NodeID); err != nil {
		t.Fatal(err)
	}
	first := f.call("POST", path, request(task.Version, key), 201)
	got := issueRepairRuntimeDecode[TaskDeliveryRepositoryIntake](t, first)
	bundle := exported.Load()
	if bundle == nil || exports.Load() != 2 || len(first) != 14 || got.ID == "" ||
		got.ProjectID != f.project || got.TaskID != task.ID || got.TaskVersion != task.Version ||
		got.RunID != a.RunID || got.NodeID != a.NodeID || got.Attempt != a.Attempt ||
		got.TargetID != target.ID || got.TargetVersion != target.Version ||
		got.BaseCommit != base || got.HeadCommit != head || got.BundleSHA256 != digest(string(*bundle)) || got.SizeBytes != int64(len(*bundle)) {
		t.Fatalf("intake did not freeze exact scope, commits, and bytes: %+v", got)
	}
	if _, err := time.Parse(time.RFC3339Nano, got.ApprovedAt); err != nil {
		t.Fatalf("invalid approval time: %v", err)
	}
	var storagePath string
	if err := f.s.DB.QueryRow(`SELECT storage_path FROM suma_repository_source_intakes WHERE id=?`, got.ID).Scan(&storagePath); err != nil || !storageBlobPath.MatchString(storagePath) {
		t.Fatalf("missing confined intake blob: %q, %v", storagePath, err)
	}
	persisted, err := root.ReadFile(storagePath)
	if err != nil || !bytes.Equal(persisted, *bundle) {
		t.Fatalf("approved bundle bytes were not preserved: %v", err)
	}

	afterApproval := reads.Load()
	replay := issueRepairRuntimeDecode[TaskDeliveryRepositoryIntake](t, f.call("POST", path, request(task.Version, key), 201))
	if replay != got || reads.Load() != afterApproval || exports.Load() != 2 || domainACount(t, f.s, "suma_repository_source_intakes") != 1 {
		t.Fatal("idempotent approval fetched again or replaced the intake")
	}
	changed := request(task.Version, key)
	changed["expected_base"] = strings.Repeat("a", 40)
	f.call("POST", path, changed, 409)
	if reads.Load() != afterApproval {
		t.Fatal("conflicting idempotent request reached the provider")
	}

	current, err := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil || current.RunID != task.RunID || current.Version != task.Version || current.Status != task.Status ||
		domainACount(t, f.s, "dag_runs") != beforeRuns || domainACount(t, f.s, "dag_attempts") != beforeAttempts ||
		domainACount(t, f.s, "suma_candidate_transfers") != beforeTransfers || h.creates.Load() != 1 || h.queries.Load() != 0 {
		t.Fatal("source intake changed task execution or manufactured candidate evidence")
	}
}
