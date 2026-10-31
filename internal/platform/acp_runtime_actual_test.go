package platform

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Opt-in host for the separately authorized actual170 journey. This is not a
// vendor integration test: only a task-owned native protocol fixture executes.
func TestACPRuntime170Actual(t *testing.T) {
	if os.Getenv("SUMA_ACP_RUNTIME_170_ACTUAL") != "1" {
		t.Skip("actual170 requires a separately frozen source and runtime gate")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err = os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatal("repository unavailable")
		}
		root = next
	}
	directory := filepath.Join(root, ".cache", "acp-runtime-170", "actual")
	requested, err := filepath.Abs(os.Getenv("SUMA_ACP_RUNTIME_170_ACTUAL_DIR"))
	if err != nil || filepath.Clean(requested) != directory || !strings.HasPrefix(filepath.Clean(os.TempDir()), directory+string(filepath.Separator)) {
		t.Fatal("exact owned actual170 directory and process TEMP required")
	}
	acpActual170Gate(t, root, directory)
	private := filepath.Join(directory, "private")
	for _, retained := range []string{"private/descriptor.json", "private/snapshot.sqlite", "private/repository", "private/developer-journal", "private/test-journal", "host.stop"} {
		if _, err = os.Stat(filepath.Join(directory, retained)); !os.IsNotExist(err) {
			t.Fatal("prior actual170 state retained; no implicit reseed")
		}
	}
	if err = os.MkdirAll(private, 0700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	f := newFixture(t)
	f.e.AttemptAccounting = f.s
	f.call("GET", "/auth/session", nil, 200)
	requirement := f.call("POST", "/requirements", f.body(map[string]any{"title": "一次明确权限的隔离开发", "description": "Controlled local ACP protocol execution; model/vendor identity is not verified.", "acceptance_criteria": []string{"准确一次权限", "同提交独立测试", "仍须人工验收"}}), 201)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "隔离开发与同提交独立验证", "description": "Change file.txt only, commit the candidate and preserve the user checkout.", "requirement_id": requirement["id"], "acceptance_criteria": []string{"Candidate contains candidate170", "Independent worker verifies the exact commit"}}), 201)
	taskID := task["id"].(string)
	quality, err := f.s.CreateTestPlan(ctx, f.actor, f.project, TestPlanCreateRequest{ProjectID: f.project, TaskID: taskID, ExpectedTaskVersion: 1, ExpectedRequirementVersion: 1, Title: "已发布的候选独立测试", Description: "One controlled test asserts candidate content and a clean exact commit; it does not grant formal acceptance.", Cases: []QualityTestCase{{Key: "candidate", Title: "核对候选内容与准确提交", Description: "Run in the independent worker candidate worktree.", Preconditions: "The original developer produced a clean candidate commit.", Steps: []string{"Read file.txt and assert candidate170", "Read HEAD and require a clean worktree"}, ExpectedResult: "The same candidate commit passes both assertions.", Priority: "high", Covers: []int{1, 2}, Selector: tr.TestCaseSelector{ClassName: "delivery", Name: "candidate170"}}}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal("quality plan setup unavailable", err)
	}
	quality, err = f.s.CommandTestPlan(ctx, f.actor, f.project, quality.ID, TestPlanCommandRequest{ProjectID: f.project, ExpectedVersion: quality.Version, Action: "publish", Reason: "Explicit controlled setup review of the one independent test; not GUI or delivery acceptance", IdempotencyKey: newID()})
	if err != nil || quality.Published == nil {
		t.Fatal("quality publication unavailable", err)
	}
	repository := filepath.Join(private, "repository")
	if err = os.Mkdir(repository, 0700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repository
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		raw, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatal("owned local repository setup failed")
		}
		return strings.TrimSpace(string(raw))
	}
	git("init", "--initial-branch=main")
	if err = os.WriteFile(filepath.Join(repository, "file.txt"), []byte("baseline170\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(repository, ".gitignore"), []byte(".cache/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "file.txt", ".gitignore")
	git("-c", "user.name=OwnedFixture170", "-c", "user.email=owned170@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "controlled baseline170")
	base := git("rev-parse", "HEAD")
	if err = os.WriteFile(filepath.Join(repository, "file.txt"), []byte("user draft retained170\n"), 0600); err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(directory, "bin", "controlled-acp170.exe")
	development := runner.CommandSpec{Adapter: "qoder", Protocol: "acp-stdio-v1", Executable: native, Args: []string{"--acp"}, Input: "acp-stdio", ACP: &runner.ACPExecutionSpec{Runtime: "qoder", ReviewedVersion: "Suma controlled native ACP fixture170; not Qoder", Model: "owned-model170"}}
	independent := runner.CommandSpec{Adapter: "command", Protocol: "text-v1", Executable: native, Args: []string{"--verify-candidate"}, TestEvidence: true, TestReport: true}
	developerConfig := runner.ExecConfig{AllowExec: true, Workspace: repository, Repository: repository, BaseCommit: base, WorktreeRoot: filepath.Join(repository, ".cache", "developer-worktrees"), Commands: map[string]runner.CommandSpec{"acp-native170": development}}
	testConfig := runner.ExecConfig{AllowExec: true, Workspace: repository, Repository: repository, BaseCommit: base, WorktreeRoot: filepath.Join(repository, ".cache", "test-worktrees"), Commands: map[string]runner.CommandSpec{"acp-independent170": independent}}
	developerID, testID := "actual170-developer", "actual170-independent-test"
	credentials := map[string]string{}
	configs := map[string]runner.ExecConfig{developerID: developerConfig, testID: testConfig}
	profiles := map[string]ToolProfile{}
	for _, workerID := range []string{developerID, testID} {
		registration := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": workerID, "profiles": []any{}, "deployment": false}), 201)
		credentials[workerID] = registration["token"].(string)
		observations, inventoryErr := runner.DescribeTools(ctx, configs[workerID])
		if inventoryErr != nil || len(observations) != 1 || !observations[0].Available || tc.ValidateProfile(observations[0].Profile, true) != nil {
			t.Fatal("controlled native inventory unavailable")
		}
		profile, e := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: workerID, Name: workerID, Profile: observations[0].Profile, IdempotencyKey: newID()})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, credentials[workerID]), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: workerID, OperationID: newID(), Items: observations}); e != nil {
			t.Fatal(e)
		}
		profile, e = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
		if e != nil || !profile.Available {
			t.Fatal("controlled tool enable unavailable", e)
		}
		profiles[workerID] = profile
	}
	workflow := f.call("POST", "/workflows", f.body(map[string]any{"name": "隔离开发 → 同提交独立测试", "nodes": []dag.Node{{ID: "develop", Name: "Controlled ACP development", Type: "agent", Command: []string{"acp-native170"}, DependsOn: []string{}, TimeoutSeconds: 180, MaxRetries: 0, Config: map[string]any{"goal": "Change file.txt to candidate170 and commit this isolated candidate."}}, {ID: "test", Name: "Independent exact candidate test", Type: "test", Command: []string{"acp-independent170"}, DependsOn: []string{"develop"}, TimeoutSeconds: 60, MaxRetries: 0, Idempotent: true, Config: map[string]any{"candidate_from": "develop", "tool_profile_id": profiles[testID].ID, "tool_profile_sha256": profiles[testID].Profile.ProfileSHA256}}}, "edges": []dag.Edge{}}), 201)
	developerJournal, err := runner.OpenJournal(filepath.Join(private, "developer-journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer developerJournal.Close()
	testJournal, err := runner.OpenJournal(filepath.Join(private, "test-journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer testJournal.Close()
	journals := map[string]*runner.Journal{developerID: developerJournal, testID: testJournal}
	var machineMu sync.Mutex
	machine := map[string]int{}
	workerReturned := map[string]bool{}
	stats := func() map[string]any {
		out := map[string]any{}
		for name, table := range map[string]string{"quality_plans": "suma_test_plans", "quality_runs": "suma_test_plan_runs", "plans": "suma_task_dispatch_plans", "runs": "dag_runs", "nodes": "dag_nodes", "attempts": "dag_attempts", "permissions": "suma_tool_permissions", "candidate_revisions": "dag_candidate_revisions", "reviews": "suma_reviews", "test_reports": "suma_test_plan_reports", "release_candidates": "suma_release_candidates"} {
			var count int
			if e := f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); e != nil {
				out["database_stats_unavailable"] = true
			} else {
				out[name] = count
			}
		}
		current, e := f.s.Get(ctx, f.actor, f.project, "tasks", taskID)
		if e != nil {
			out["task_stats_unavailable"] = true
		} else {
			out["task_version"], out["task_status"], out["task_run_id"] = current.Version, current.Status, current.RunID
			out["task_verified_run_present"] = current.Data["verified_run_id"] != nil && current.Data["verified_run_id"] != ""
			if current.RunID != "" {
				run, e := f.e.GetRun(ctx, f.user.TenantID, f.project, current.RunID)
				if e != nil {
					out["run_stats_unavailable"] = true
				} else {
					out["run"] = run
				}
			}
		}
		pendingResults, pendingProgress := 0, 0
		for _, j := range journals {
			pendingResults += len(j.Pending())
			pendingProgress += len(j.PendingProgress())
		}
		out["journal_pending_results"], out["journal_pending_progress"] = pendingResults, pendingProgress
		machineMu.Lock()
		counts := map[string]int{}
		returned := map[string]bool{}
		for k, v := range machine {
			counts[k] = v
		}
		for k, v := range workerReturned {
			returned[k] = v
		}
		machineMu.Unlock()
		out["machine_http_requests"], out["worker_returned"] = counts, returned
		return out
	}
	initial := stats()
	if initial["database_stats_unavailable"] != nil || initial["task_stats_unavailable"] != nil {
		t.Fatal("initial actual170 stats unavailable")
	}
	for _, name := range []string{"plans", "runs", "nodes", "attempts", "permissions", "candidate_revisions", "reviews", "test_reports", "release_candidates"} {
		if fmt.Sprint(initial[name]) != "0" {
			t.Fatal("seed created execution state", name)
		}
	}
	uiOrigin := "http://127.0.0.1:44047"
	platform := (&Server{Service: f.s, Engine: f.e, PublicURL: uiOrigin, AllowedOrigins: []string{uiOrigin}, WebDir: filepath.Join(directory, "absent-web")}).Handler()
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__actual170/status" {
			cookie, e := r.Cookie(f.cookie.Name)
			if r.Method != http.MethodGet || r.URL.RawQuery != "" || e != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(f.cookie.Value)) != 1 {
				w.WriteHeader(403)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(stats())
			return
		}
		isMachine := strings.HasPrefix(r.URL.Path, "/api/v1/runner/")
		if isMachine {
			machineMu.Lock()
			machine[r.Method+" "+r.URL.Path]++
			machineMu.Unlock()
		}
		browserPost := r.Method == http.MethodPost && (r.URL.Path == "/api/v1/task-dispatch-plans" || strings.HasPrefix(r.URL.Path, "/api/v1/task-dispatch-plans/") && strings.HasSuffix(r.URL.Path, "/start") || strings.HasPrefix(r.URL.Path, "/api/v1/tool-permissions/") && strings.HasSuffix(r.URL.Path, "/decision"))
		if r.Method != http.MethodGet && !isMachine && !browserPost {
			w.WriteHeader(403)
			return
		}
		platform.ServeHTTP(w, r)
	}))
	defer control.Close()
	workerCtx, stopWorkers := context.WithCancel(ctx)
	defer stopWorkers()
	workerDone := make(chan struct {
		id  string
		err error
	}, 2)
	for _, id := range []string{developerID, testID} {
		w := &runner.Worker{Client: &runner.Client{BaseURL: control.URL, ProjectID: f.project, Token: credentials[id], HTTP: control.Client()}, Journal: journals[id], ID: id, Exec: configs[id], PollInterval: 150 * time.Millisecond, HeartbeatInterval: time.Second}
		go func(id string) {
			e := w.Run(workerCtx)
			machineMu.Lock()
			workerReturned[id] = true
			machineMu.Unlock()
			workerDone <- struct {
				id  string
				err error
			}{id, e}
		}(id)
	}
	joined := map[string]string{}
	defer func() {
		stopWorkers()
		for len(joined) < 2 {
			select {
			case result := <-workerDone:
				joined[result.id] = "deferred"
			case <-time.After(12 * time.Second):
				t.Error("actual170 original workers did not close")
				return
			}
		}
	}()
	var sequence int
	var databaseName, databasePath string
	if err = f.s.DB.QueryRow("PRAGMA database_list").Scan(&sequence, &databaseName, &databasePath); err != nil {
		t.Fatal(err)
	}
	acpActual170Write(t, filepath.Join(private, "descriptor.json"), map[string]any{"stage": "T-ACP-RUNTIME-170", "origin": control.URL, "ui_origin": uiOrigin, "pid": os.Getpid(), "project_id": f.project, "task_id": taskID, "task_version": 1, "requirement_id": requirement["id"], "quality_plan_id": quality.ID, "quality_revision": quality.PublishedRevision, "quality_sha256": quality.Published.SHA256, "workflow_id": workflow["id"], "workflow_version": workflow["version"], "developer_profile_id": profiles[developerID].ID, "developer_profile": profiles[developerID], "developer_worker_id": developerID, "test_worker_id": testID, "base_commit": base, "owner": map[string]any{"user_id": f.view.ID, "organization_id": f.view.TenantID, "cookie": map[string]any{"name": f.cookie.Name, "value": f.cookie.Value}}, "initial_stats": initial, "database_source": databasePath, "data_directory_source": f.s.DataDir, "seed": "Fresh fake owner, requirement/task v1, one explicitly published case covering both task acceptance criteria, local owned Git baseline, two registered and explicitly enabled private tool profiles (test profile has suma-junit-report-v1), published two-node workflow; setup uses original handlers/services, not GUI. No Run/attempt/permission/candidate/report/review. Native binary is a controlled protocol fixture, not Qoder."})
	t.Log("actual170 real Server/SQLite/two private workers ready; controlled native CLI only; secrets remain private")
	timeout := time.NewTimer(20 * time.Minute)
	defer timeout.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	timedOut := false
wait:
	for {
		select {
		case <-timeout.C:
			timedOut = true
			break wait
		case result := <-workerDone:
			if result.err != nil {
				joined[result.id] = "error"
			} else {
				joined[result.id] = "nil"
			}
		case <-ticker.C:
			if _, e := os.Stat(filepath.Join(directory, "host.stop")); e == nil {
				break wait
			}
		}
	}
	stopWorkers()
	for len(joined) < 2 {
		select {
		case result := <-workerDone:
			state := "nil"
			if errors.Is(result.err, context.Canceled) {
				state = "context_canceled_after_normal_stop"
			} else if result.err != nil {
				state = "error"
			}
			joined[result.id] = state
		case <-time.After(15 * time.Second):
			t.Fatal("actual170 workers did not join")
		}
	}
	control.Close()
	final := stats()
	if raw, e := os.ReadFile(filepath.Join(repository, "file.txt")); e != nil || string(raw) != "user draft retained170\n" {
		t.Error("owned original checkout changed")
	}
	snapshot := filepath.Join(private, "snapshot.sqlite")
	if _, e := os.Stat(snapshot); !os.IsNotExist(e) {
		t.Fatal("prior snapshot retained")
	}
	if _, err = f.s.DB.ExecContext(ctx, "VACUUM INTO ?", snapshot); err != nil {
		t.Fatal("consistent snapshot unavailable")
	}
	if err = os.Chmod(snapshot, 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	acpActual170Write(t, filepath.Join(private, "host-final.json"), map[string]any{"host_closed": true, "host_timeout": timedOut, "workers_joined": len(joined) == 2, "worker_exits": joined, "stats": final, "original_checkout_retained": true, "snapshot": map[string]any{"path": snapshot, "bytes": len(raw), "sha256": hex.EncodeToString(sum[:]), "method": "Parameterized VACUUM INTO new private target after both Worker.Run joins and HTTP close, before temporary source cleanup"}, "original_database_source": databasePath, "original_data_directory_source": f.s.DataDir})
	if timedOut || joined[developerID] == "error" || joined[testID] == "error" {
		t.Fatal("actual170 host/worker failure; original private receipts retained")
	}
}

func acpActual170Write(t *testing.T, file string, value any) {
	t.Helper()
	out, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("original actual170 receipt retained")
	}
	if err = json.NewEncoder(out).Encode(value); err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err != nil || closeErr != nil {
		t.Fatal("actual170 receipt write failed")
	}
}

func acpActual170Gate(t *testing.T, root, directory string) {
	t.Helper()
	var gate struct {
		Stage             string                          `json:"stage"`
		RuntimeAuthorized bool                            `json:"runtime_authorized"`
		Files             []struct{ Path, SHA256 string } `json:"files"`
	}
	raw, err := os.ReadFile(filepath.Join(directory, "run-authorized.json"))
	if err != nil || json.Unmarshal(raw, &gate) != nil || gate.Stage != "T-ACP-RUNTIME-170" || !gate.RuntimeAuthorized || len(gate.Files) == 0 || len(gate.Files) > 120 {
		t.Fatal("Root final actual170 gate required")
	}
	for _, input := range gate.Files {
		absolute := filepath.Clean(filepath.Join(root, filepath.FromSlash(input.Path)))
		if filepath.IsAbs(input.Path) || !strings.HasPrefix(absolute, root+string(filepath.Separator)) {
			t.Fatal("source outside repository")
		}
		content, e := os.ReadFile(absolute)
		if e != nil {
			t.Fatal("source input unavailable")
		}
		sum := sha256.Sum256(content)
		if hex.EncodeToString(sum[:]) != input.SHA256 {
			t.Fatal("source input changed")
		}
	}
}
