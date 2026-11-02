package platform

import (
	"context"
	"crypto/sha256"
	"encoding/json"
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
	"github.com/ai-mada/suma/internal/store"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
)

// One opt-in real Git/HTTP/SQLite journey. The code host and reviewer are
// isolated deterministic test identities, not a vendor model or customer signoff.
func TestAcceptedTaskBaselineActualOriginalAgentAndIndependentContinuation(t *testing.T) {
	if os.Getenv("SUMA_ACCEPTED_TASK_BASELINE_222_ACTUAL") != "1" {
		t.Skip("explicit isolated accepted-task baseline journey required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	node := os.Getenv("SUMA_ACCEPTED_TASK_BASELINE_222_NODE")
	git, err := exec.LookPath("git")
	if err != nil || !filepath.IsAbs(node) {
		t.Fatal("existing absolute Node and Git required", err)
	}
	git, err = filepath.Abs(git)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, ".cache", "accepted-task-baseline-222", "actual", strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000Z"), ".", ""))
	if err = os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	temporary := filepath.Join(directory, "tmp")
	if err = os.Mkdir(temporary, 0700); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"TEMP", "TMP", "TMPDIR", "HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		t.Setenv(key, temporary)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	f := acceptedBaselineActualFixture(t, directory)
	f.e.AttemptAccounting = f.s
	// Reuse this isolated organization's actual reviewer across both reviews.
	// Reinviting the fixed helper identity correctly conflicts after first use.
	reviewer := invitedReviewer(t, f)
	web := filepath.Join(root, ".cache", "accepted-task-baseline-222", "web", "out")
	if info, e := os.Stat(filepath.Join(web, "index.html")); e != nil || !info.Mode().IsRegular() {
		t.Fatal("prepared isolated UI required", e)
	}
	receipt := map[string]any{"stage": "T-ACCEPTED-TASK-BASELINE-222", "passed": false, "kind": "ordinary_agent_real_git_http_sqlite_and_independent_followup", "vendor_calls": 0, "model_calls": 0, "customer_signoff": false}
	defer func() {
		if t.Failed() {
			receipt["passed"] = false
		}
		acceptedBaselineActualWrite(t, filepath.Join(directory, "receipt.json"), receipt)
		t.Log("accepted-task baseline evidence retained at", directory)
	}()
	repoA, repoB := filepath.Join(directory, "source-repository"), filepath.Join(directory, "target-repository")
	if err = os.Mkdir(repoA, 0700); err != nil {
		t.Fatal(err)
	}
	workspaceGitTestCommand(t, repoA, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, repoA, ".gitignore", ".cache/\n")
	// This fixture verifies exact bytes on independent Windows checkouts. Keep
	// its repository policy explicit instead of inheriting system autocrlf.
	workspaceGitTestWrite(t, repoA, ".gitattributes", "* -text\n")
	workspaceGitTestWrite(t, repoA, "file.txt", "administrator-base222\n")
	workspaceGitTestCommand(t, repoA, "add", ".gitignore", ".gitattributes", "file.txt")
	workspaceGitTestCommand(t, repoA, "commit", "-m", "fixture: original baseline222")
	base := workspaceGitTestCommand(t, repoA, "rev-parse", "HEAD")
	// Clone before M exists. The target cannot obtain M by shared initial Git
	// objects; its original code-input broker must supply the accepted bundle.
	workspaceGitTestCommand(t, directory, "clone", "--no-hardlinks", repoA, repoB)
	const draftA, draftB = "source administrator draft222\n", "target administrator draft222\n"
	workspaceGitTestWrite(t, repoA, "file.txt", draftA)
	workspaceGitTestWrite(t, repoB, "file.txt", draftB)
	sourceTask := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "Ordinary agent source A", "description": "Owned deterministic Suma host, no model", "acceptance_criteria": []string{"Another Worker verifies the exact candidate M"}}), 201))
	targetTask := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "Continue accepted candidate M as B", "acceptance_criteria": []string{"Preserve candidate M", "Add the owned follow-up N"}}), 201))
	thirdTask := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "Fresh target C for returned-source rejection", "acceptance_criteria": []string{"Do not accept a returned source"}}), 201))
	var stateMu, traceMu sync.Mutex
	phase := "preparing_source"
	trace := []map[string]any{}
	publicSession := f.call("GET", "/auth/session", nil, 200)
	app := (&Server{Service: f.s, Engine: f.e, WebDir: web}).Handler()
	var sourceCommit string
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/__fixture/view" {
			// This cookie belongs only to this isolated fixture database. It never
			// enters a URL, renderer value, public evidence file or console.
			cookie := *f.cookie
			cookie.Path = "/"
			cookie.HttpOnly = true
			cookie.SameSite = http.SameSiteStrictMode
			http.SetCookie(w, &cookie)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			stateMu.Lock()
			currentPhase, commit := phase, sourceCommit
			stateMu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"session": publicSession, "project_id": f.project, "source_task_id": sourceTask.ID, "source_node_id": "develop", "source_commit": commit, "target_task_id": targetTask.ID, "third_task_id": thirdTask.ID, "phase": currentPhase})
			return
		}
		observed := &toolTaskHandoffResponse213{ResponseWriter: w}
		app.ServeHTTP(observed, r)
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			traceMu.Lock()
			trace = append(trace, map[string]any{"method": r.Method, "path": r.URL.Path, "status": observed.status})
			traceMu.Unlock()
		}
	}))
	defer control.Close()
	sourceRuntimeReceipt := map[string]any{}
	followupRuntimeReceipt := map[string]any{}
	receipt["source_runtime"], receipt["followup_runtime"] = sourceRuntimeReceipt, followupRuntimeReceipt
	followupDirectory := filepath.Join(directory, "followup-runtime")
	if e := os.Mkdir(followupDirectory, 0700); e != nil {
		t.Fatal(e)
	}
	shared := originalDevelopmentRuntime{f: f, control: control, directory: directory, receipt: sourceRuntimeReceipt}
	followupRuntime := originalDevelopmentRuntime{f: f, control: control, directory: followupDirectory, receipt: followupRuntimeReceipt}
	developerA := acceptedBaselineActualAgent(t, directory, node, git, repoA, base, "source", "")
	verifierA := acceptedBaselineActualVerifier(node, repoA, base, "source")
	producerA, tokenA := issueRepairRuntimeTool(t, f, "accepted-source-developer222", &developerA)
	testerA, testTokenA := issueRepairRuntimeTool(t, f, "accepted-source-independent222", &verifierA)
	qualityA := acceptedBaselineActualQuality(t, f, sourceTask, "source", []int{1})
	workflowA := acceptedBaselineActualWorkflow(t, ctx, f, "Source ordinary agent A", testerA)
	planA := acceptedBaselineActualPlan(t, ctx, f, sourceTask, workflowA, producerA, testerA)
	runA, e := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, planA.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: planA.Version, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	stopA := shared.startWorker(t, ctx, producerA.WorkerID, tokenA, "source-developer-journal", developerA)
	defer func() {
		if stopA != nil {
			stopA()
		}
	}()
	producedA := acceptedBaselineActualWait(t, ctx, f, runA.ID)
	stopA()
	stopA = nil
	commitM, _ := producedA.WorkspaceEvidence["head_commit"].(string)
	if commitM == "" || commitM == base {
		t.Fatal("source agent did not create a new candidate")
	}
	_, _, acceptedA := shared.finishQuality(t, ctx, originalDevelopmentQuality{Repository: repoA, BaseCommit: base, CandidateCommit: commitM, ExpectedDraft: draftA, Task: sourceTask, Plan: qualityA, Run: runA, Producer: producedA, TestProfile: testerA, TestToken: testTokenA, Independent: verifierA, Reviewer: reviewer})
	if result := exec.CommandContext(ctx, git, "-C", repoB, "cat-file", "-e", commitM+"^{commit}").Run(); result == nil {
		t.Fatal("target already possessed M before protected code import")
	}
	stateMu.Lock()
	sourceCommit, phase = commitM, "awaiting_ui_binding"
	stateMu.Unlock()
	ready := map[string]any{"stage": "T222", "phase": "awaiting_ui_binding", "origin": control.URL, "directory": directory, "project_id": f.project, "source_task_id": acceptedA.ID, "source_task_version": acceptedA.Version, "source_node_id": "develop", "source_commit": commitM, "target_task_id": targetTask.ID, "third_task_id": thirdTask.ID}
	acceptedBaselineActualWrite(t, filepath.Join(directory, "ready.json"), ready)
	t.Log("accepted-task baseline UI ready:", control.URL, "evidence:", directory)
	acceptedBaselineActualMarker(t, ctx, filepath.Join(directory, "continue.json"))
	bound, e := f.s.Get(ctx, f.actor, f.project, "tasks", targetTask.ID)
	if e != nil || bound.CodeBaseline == nil || bound.CodeBaseline.SourceTaskID != acceptedA.ID || bound.CodeBaseline.SourceTaskVersion != acceptedA.Version || bound.CodeBaseline.Source.HeadCommit != commitM || bound.CodeBaseline.Source.Source.NodeID != "develop" {
		t.Fatal("real UI did not bind the exact ordinary accepted source", e)
	}
	receipt["source_task"], receipt["bound_target"] = acceptedA, bound
	developerB := acceptedBaselineActualAgent(t, directory, node, git, repoB, base, "followup", commitM)
	verifierB := acceptedBaselineActualVerifier(node, repoB, base, "followup")
	producerB, tokenB := issueRepairRuntimeTool(t, f, "accepted-followup-developer222", &developerB)
	testerB, testTokenB := issueRepairRuntimeTool(t, f, "accepted-followup-independent222", &verifierB)
	qualityB := acceptedBaselineActualQuality(t, f, bound, "followup", []int{1, 2})
	workflowB := acceptedBaselineActualWorkflow(t, ctx, f, "Continue protected ordinary baseline", testerB)
	planB := acceptedBaselineActualPlan(t, ctx, f, bound, workflowB, producerB, testerB)
	if planB.CodeBaseline == nil || *planB.CodeBaseline != *bound.CodeBaseline {
		t.Fatal("original TaskDispatch lost baseline")
	}
	runB, e := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, planB.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: planB.Version, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	stopB := followupRuntime.startWorker(t, ctx, producerB.WorkerID, tokenB, "followup-developer-journal", developerB)
	defer func() {
		if stopB != nil {
			stopB()
		}
	}()
	producedB := acceptedBaselineActualWait(t, ctx, f, runB.ID)
	stopB()
	stopB = nil
	commitN, _ := producedB.WorkspaceEvidence["head_commit"].(string)
	if commitN == "" || commitN == commitM || producedB.WorkerID == producedA.WorkerID || workspaceGitTestCommand(t, repoB, "rev-parse", commitN+"^") != commitM {
		t.Fatal("follow-up did not develop an independent worktree directly from M")
	}
	_, _, acceptedB := followupRuntime.finishQuality(t, ctx, originalDevelopmentQuality{Repository: repoB, BaseCommit: base, CandidateCommit: commitN, ExpectedDraft: draftB, Task: bound, Plan: qualityB, Run: runB, Producer: producedB, TestProfile: testerB, TestToken: testTokenB, Independent: verifierB, Reviewer: reviewer})
	currentA, e := f.s.Get(ctx, f.actor, f.project, "tasks", sourceTask.ID)
	if e != nil {
		t.Fatal(e)
	}
	returned, e := f.s.Review(ctx, reviewer, f.project, sourceTask.ID, f.body(map[string]any{"expected_version": currentA.Version, "run_id": runA.ID, "decision": "rejected", "reason": "Isolated fixture returns the source to prove current-basis revalidation, not customer signoff"}), f.e)
	if e != nil {
		t.Fatal("return original source through production review", e)
	}
	requestBody := f.body(map[string]any{"project_id": f.project, "expected_task_version": thirdTask.Version, "expected_binding_version": 0, "source_task_id": sourceTask.ID, "expected_source_task_version": returned.Version, "source_node_id": "develop", "acknowledge": true, "reason": "Must reject a returned source even with its current version", "idempotency_key": newID()})
	raw, _ := json.Marshal(requestBody)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, control.URL+"/api/v1/tasks/"+thirdTask.ID+"/code-baseline", strings.NewReader(string(raw)))
	req.AddCookie(f.cookie)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Suma-Expected-User", f.view.ID)
	req.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
	status, sourceDenial := acceptedBaselineActualSourceDenial(t, control.Client(), req)
	stillB, e := f.s.Get(ctx, f.actor, f.project, "tasks", targetTask.ID)
	if e != nil || stillB.Status != "accepted" || stillB.RunID != acceptedB.RunID {
		t.Fatal("source return rewrote completed target facts", e)
	}
	traceMu.Lock()
	brokerReads := 0
	for _, fact := range trace {
		if strings.Contains(fmt.Sprint(fact["path"]), "code-input") {
			brokerReads++
		}
	}
	traceMu.Unlock()
	if brokerReads == 0 {
		t.Fatal("follow-up did not use original protected code-input HTTP broker")
	}
	receipt["candidate_M"], receipt["candidate_N"], receipt["candidate_N_parent"], receipt["code_input_http_requests"] = commitM, commitN, commitM, brokerReads
	receipt["accepted_target"], receipt["returned_source"], receipt["current_returned_source_bind_status"] = acceptedB, returned, status
	receipt["current_returned_source_bind_error"] = sourceDenial
	stateMu.Lock()
	phase = "source_returned"
	stateMu.Unlock()
	acceptedBaselineActualWrite(t, filepath.Join(directory, "source-returned.json"), map[string]any{"phase": "source_returned", "origin": control.URL, "directory": directory, "project_id": f.project, "source_task_id": returned.ID, "source_task_version": returned.Version, "target_task_id": targetTask.ID, "candidate_M": commitM, "candidate_N": commitN, "rejected_new_action_status": status})
	acceptedBaselineActualMarker(t, ctx, filepath.Join(directory, "close.json"))
	control.Close()
	traceMu.Lock()
	receipt["http"] = trace
	traceMu.Unlock()
	receipt["passed"] = true
}

// Reopen only this stage's retained fixture to strengthen the original negative
// HTTP assertion. No Git, Worker, Review, browser or successful bind is replayed.
func TestAcceptedTaskBaselineActualReturnedSourceAuthenticatedDomainRecheck(t *testing.T) {
	directory := os.Getenv("SUMA_ACCEPTED_TASK_BASELINE_222_RECHECK_DIRECTORY")
	if directory == "" {
		t.Skip("explicit retained isolated fixture directory required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(root, ".cache", "accepted-task-baseline-222", "actual")
	directory, err = filepath.Abs(directory)
	relative, relErr := filepath.Rel(base, directory)
	resolved, resolveErr := filepath.EvalSymlinks(directory)
	if err != nil || relErr != nil || relative == "." || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) || resolveErr != nil || !strings.EqualFold(resolved, directory) {
		t.Fatal("explicit retained stage-owned nonlinked fixture required")
	}
	var ready struct {
		Project string `json:"project_id"`
		Source  string `json:"source_task_id"`
		Target  string `json:"target_task_id"`
		Third   string `json:"third_task_id"`
	}
	raw, err := os.ReadFile(filepath.Join(directory, "ready.json"))
	if err != nil || len(raw) > 4096 || json.Unmarshal(raw, &ready) != nil || !handoffID(ready.Project, false) || !handoffID(ready.Source, false) || !handoffID(ready.Target, false) || !handoffID(ready.Third, false) {
		t.Fatal("retained exact fixture references required", err)
	}
	database := filepath.Join(directory, "private.sqlite")
	if info, err := os.Lstat(database); err != nil || !info.Mode().IsRegular() {
		t.Fatal("retained regular SQLite database required", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(database + suffix); !os.IsNotExist(err) {
			t.Fatal("closed database without WAL/SHM required before snapshot", suffix, err)
		}
	}
	original, err := os.ReadFile(database)
	if err != nil {
		t.Fatal(err)
	}
	originalSHA := fmt.Sprintf("%x", sha256.Sum256(original))
	supplement := filepath.Join(directory, "source-domain-supplement", strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000Z"), ".", ""))
	if err = os.MkdirAll(supplement, 0700); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(supplement, "private.sqlite")
	file, err := os.OpenFile(snapshot, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.Write(original)
	closeErr := file.Close()
	copyBytes, readErr := os.ReadFile(snapshot)
	if writeErr != nil || closeErr != nil || readErr != nil || fmt.Sprintf("%x", sha256.Sum256(copyBytes)) != originalSHA {
		t.Fatal("exact closed database snapshot failed", writeErr, closeErr, readErr)
	}
	db, err := store.Open("sqlite", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s := &Service{DB: db, Dialect: "sqlite", DataDir: filepath.Join(directory, "data")}
	engine, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, s: s, e: engine, project: ready.Project}
	f.handler = (&Server{Service: s, Engine: engine, WebDir: directory}).Handler()
	// The existing local test account alone obtains a new real session. Its
	// cookie stays private; the old browser session is neither exposed nor reset.
	f.call("POST", "/auth/login", map[string]any{"email": "owner222@example.test", "password": "isolated-test-password-strong222"}, 200)
	f.actor, err = s.AuthenticateActor(ctx, f.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	control := httptest.NewServer(f.handler)
	defer control.Close()
	read := func(id string) Record {
		t.Helper()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, control.URL+"/api/v1/tasks/"+id+"?project_id="+ready.Project, nil)
		req.AddCookie(f.cookie)
		req.Header.Set("X-Suma-Expected-User", f.view.ID)
		req.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
		response, err := control.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var record Record
		if response.StatusCode != 200 || json.NewDecoder(response.Body).Decode(&record) != nil {
			t.Fatal("current authenticated task read failed", response.StatusCode)
		}
		return record
	}
	source, target, third := read(ready.Source), read(ready.Target), read(ready.Third)
	if source.Status != "blocked" || source.Version != 4 || target.Status != "accepted" || third.Status != "todo" || third.RunID != "" || third.CodeBaseline != nil {
		t.Fatal("retained source-return/current-target checkpoint differs")
	}
	body := f.body(map[string]any{"expected_task_version": third.Version, "expected_binding_version": 0, "source_task_id": source.ID, "expected_source_task_version": source.Version, "source_node_id": "develop", "acknowledge": true, "reason": "Authenticated current-view source-domain recheck only"})
	raw, err = json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, control.URL+"/api/v1/tasks/"+third.ID+"/code-baseline", strings.NewReader(string(raw)))
	req.AddCookie(f.cookie)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Suma-Expected-User", f.view.ID)
	req.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
	status, denial := acceptedBaselineActualSourceDenial(t, control.Client(), req)
	after, targetAfter := read(third.ID), read(target.ID)
	if after.Version != third.Version || after.RunID != "" || after.CodeBaseline != nil || targetAfter.Version != target.Version || targetAfter.RunID != target.RunID || targetAfter.Status != "accepted" {
		t.Fatal("negative recheck changed the fresh or completed target")
	}
	afterOriginal, err := os.ReadFile(database)
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(afterOriginal)) != originalSHA {
		t.Fatal("original retained database changed during snapshot-only recheck", err)
	}
	acceptedBaselineActualWrite(t, filepath.Join(directory, "source-domain-recheck.json"), map[string]any{"passed": true, "kind": "retained_SQLite_copy_authenticated_current_view_domain_rejection", "original_database_sha256_before": originalSHA, "original_database_sha256_after": fmt.Sprintf("%x", sha256.Sum256(afterOriginal)), "snapshot_initial_sha256": fmt.Sprintf("%x", sha256.Sum256(copyBytes)), "snapshot_directory": supplement, "prior_auth_only_attempt": "First recheck before copy-only requirement logged in on original test DB then stopped at incorrect fixture expectation rejected vs actual blocked; no business POST or task/run/baseline writes", "prior_weak_status": 428, "prior_weak_status_reason": "missing_expected_view_headers_not_source_guard", "http_status": status, "error": denial, "expected_user": f.view.ID, "expected_organization": f.view.TenantID, "source_task_id": source.ID, "expected_source_task_version": source.Version, "source_status": source.Status, "target_task_id": third.ID, "expected_task_version": third.Version, "fresh_target_unchanged": true, "completed_target_unchanged": true, "Git_Worker_Review_browser_replayed": false})
}

func acceptedBaselineActualSourceDenial(t *testing.T, client *http.Client, request *http.Request) (int, map[string]any) {
	t.Helper()
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope map[string]any
	if err = json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal("decode source-domain HTTP refusal", err)
	}
	denial, ok := envelope["error"].(map[string]any)
	if response.StatusCode != http.StatusConflict || !ok || denial["code"] != "repository_integration_basis" {
		t.Fatal("expected actual current source-domain refusal, not auth/view/shape/CAS failure", response.StatusCode, envelope)
	}
	return response.StatusCode, denial
}

// Same production fixture bootstrap as newFixture, but the actual database
// and broker artifacts stay in the owned evidence directory after cleanup.
func acceptedBaselineActualFixture(t *testing.T, directory string) *fixture {
	t.Helper()
	db, e := store.Open("sqlite", filepath.Join(directory, "private.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := &Service{DB: db, Dialect: "sqlite", DataDir: filepath.Join(directory, "data")}
	if e = os.MkdirAll(s.DataDir, 0700); e != nil {
		t.Fatal(e)
	}
	if e = s.Initialize(context.Background()); e != nil {
		t.Fatal(e)
	}
	engine, e := dag.New(db, "sqlite")
	if e != nil {
		t.Fatal(e)
	}
	f := &fixture{t: t, s: s, e: engine}
	f.handler = (&Server{Service: s, Engine: engine, WebDir: directory}).Handler()
	setup := f.call("POST", "/auth/setup", map[string]any{"name": "Owned baseline reviewer", "email": "owner222@example.test", "password": "isolated-test-password-strong222", "organization": "Owned fixture organization", "project": "Owned fixture project"}, 200)
	raw, _ := json.Marshal(setup["user"])
	_ = json.Unmarshal(raw, &f.user)
	f.actor, e = s.AuthenticateActor(context.Background(), f.cookie.Value)
	if e != nil {
		t.Fatal(e)
	}
	f.project = setup["projects"].([]any)[0].(map[string]any)["id"].(string)
	return f
}

func acceptedBaselineActualWrite(t *testing.T, file string, value any) {
	t.Helper()
	raw, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(file, append(raw, '\n'), 0600); e != nil {
		t.Fatal("preserve actual receipt", e)
	}
}

func acceptedBaselineActualMarker(t *testing.T, ctx context.Context, file string) {
	t.Helper()
	for {
		if info, e := os.Stat(filepath.Join(filepath.Dir(file), "abort.json")); e == nil && info.Mode().IsRegular() && info.Size() <= 4096 {
			t.Fatal("isolated actual journey explicitly aborted; retained evidence is not a completed journey")
		}
		if info, e := os.Stat(file); e == nil && info.Mode().IsRegular() && info.Size() <= 4096 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("explicit UI checkpoint was not completed", file)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func acceptedBaselineActualAgent(t *testing.T, directory, node, git, repo, base, mode, expected string) runner.ExecConfig {
	t.Helper()
	gitJSON, _ := json.Marshal(git)
	expectedJSON, _ := json.Marshal(expected)
	source := `import fs from 'node:fs';import path from 'node:path';import cp from 'node:child_process';import readline from 'node:readline';
if(process.argv.includes('--version')){console.log('suma-agent-runtime 1.0.0-owned222');process.exit(0)}
if(process.argv.includes('--help')){console.log('Owned Suma JSONL fixture; SUMA_AGENT_TOKEN protocol declaration only; no model or token access.');process.exit(0)}
const git=__GIT__,expected=__EXPECTED__,follow=__FOLLOW__,lines=readline.createInterface({input:process.stdin,crlfDelay:Infinity});
lines.once('line',line=>{const input=JSON.parse(line);if(typeof input.request_id!=='string'||typeof input.goal!=='string'||!input.goal.trim())throw Error('Missing original Suma request');const emit=value=>console.log(JSON.stringify({...value,request_id:input.request_id}));emit({type:'started'});
const own=path.resolve('.cache','owned222-git'),hooks=path.join(own,'empty-hooks'),config=path.join(own,'empty.gitconfig');fs.mkdirSync(hooks,{recursive:true});fs.writeFileSync(config,'',{flag:'wx'});const env={...process.env,GIT_CONFIG_NOSYSTEM:'1',GIT_CONFIG_GLOBAL:config,GIT_TERMINAL_PROMPT:'0'};const flags=['-c','core.hooksPath='+hooks,'-c','user.name=Owned fixture','-c','user.email=owned@example.invalid','-c','commit.gpgsign=false','-c','core.autocrlf=false'];
if(follow){if(fs.readFileSync('file.txt','utf8')!=='candidate-M222\n'||cp.execFileSync(git,['rev-parse','HEAD'],{env,encoding:'utf8',windowsHide:true}).trim()!==expected)throw Error('Protected baseline M was not imported');fs.writeFileSync('followup.txt','candidate-N222\n')}else fs.writeFileSync('file.txt','candidate-M222\n');
for(const args of [['add','--',follow?'followup.txt':'file.txt'],['commit','-m',follow?'fixture: follow accepted M222':'fixture: ordinary candidate M222']])cp.execFileSync(git,[...flags,...args],{env,stdio:'pipe',windowsHide:true});emit({type:'completed',output:'Owned deterministic code committed; independent quality and fixture Review remain required.'});lines.close();});`
	source = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(source, "__GIT__", string(gitJSON)), "__EXPECTED__", string(expectedJSON)), "__FOLLOW__", fmt.Sprint(mode == "followup"))
	entry := filepath.Join(directory, "owned-agent-"+mode+".mjs")
	if e := os.WriteFile(entry, []byte(source), 0600); e != nil {
		t.Fatal(e)
	}
	return runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, WorktreeRoot: filepath.Join(repo, ".cache", "developer-worktrees"), Commands: map[string]runner.CommandSpec{"owned-" + mode + "222": {Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: node, Args: []string{entry}, Input: "agent-jsonl"}}}
}

func acceptedBaselineActualVerifier(node, repo, base, mode string) runner.ExecConfig {
	check := `const fs=require('node:fs');const ok=fs.readFileSync('file.txt','utf8')==='candidate-M222\n'&&(__FOLLOW__?fs.readFileSync('followup.txt','utf8')==='candidate-N222\n':true);fs.writeFileSync(process.argv[1],'<testsuite tests="1" failures="'+(ok?'0':'1')+'"><testcase classname="baseline222" name="__MODE__">'+(ok?'':'<failure message="exact code source mismatch"/>')+'</testcase></testsuite>');process.exitCode=ok?0:1;`
	check = strings.ReplaceAll(strings.ReplaceAll(check, "__FOLLOW__", fmt.Sprint(mode == "followup")), "__MODE__", mode)
	return runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, WorktreeRoot: filepath.Join(repo, ".cache", "independent-worktrees"), Commands: map[string]runner.CommandSpec{"verify-" + mode + "222": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", check, "{{test_report_path}}"}, TestEvidence: true, TestReport: true}}}
}

func acceptedBaselineActualQuality(t *testing.T, f *fixture, task Record, mode string, covers []int) TestPlanView {
	t.Helper()
	return issueRepairRuntimePlan(t, f, task, QualityTestCase{Key: mode, Title: "Exact independent code source", Description: "Owned deterministic source, original JUnit and distinct Worker", Preconditions: "Use the pinned candidate", Steps: []string{"Verify M bytes and requested follow-up"}, ExpectedResult: "Exact code passes independent regression", Priority: "high", Covers: covers, Selector: tr.TestCaseSelector{ClassName: "baseline222", Name: mode}})
}

func acceptedBaselineActualWorkflow(t *testing.T, ctx context.Context, f *fixture, name string, tester ToolProfile) dag.Workflow {
	t.Helper()
	workflow, _, _ := taskDispatchBuildFixture()
	workflow.ID, workflow.Version, workflow.Name = "", 0, name
	workflow.Nodes[0].Command = []string{"published-developer-placeholder222"}
	workflow.Nodes[0].Config = map[string]any{"goal": "Develop the owned exact candidate; preserve independent testing and fixture review."}
	workflow.Nodes[1].Command = []string{tester.Profile.CommandID}
	workflow.Nodes[1].Config = map[string]any{"candidate_from": "develop", "capability": "command:" + tester.Profile.CommandID}
	workflow, e := f.e.Publish(ctx, f.user.TenantID, f.project, workflow)
	if e != nil {
		t.Fatal(e)
	}
	return workflow
}

func acceptedBaselineActualPlan(t *testing.T, ctx context.Context, f *fixture, task Record, workflow dag.Workflow, producer, tester ToolProfile) TaskDispatchPlan {
	t.Helper()
	plan, e := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, TaskDispatchPreviewRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, WorkflowID: workflow.ID, WorkflowVersion: workflow.Version, AgentNodeID: "develop", ToolProfileID: producer.ID, ExpectedToolVersion: producer.Version, TestBindings: []TaskDispatchTestSelection{{NodeID: "test", ToolProfileID: tester.ID, ExpectedToolVersion: tester.Version}}, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	return plan
}

func acceptedBaselineActualWait(t *testing.T, ctx context.Context, f *fixture, run string) dag.NodeRun {
	t.Helper()
	for {
		value, e := f.e.GetRun(ctx, f.user.TenantID, f.project, run)
		if e != nil {
			t.Fatal(e)
		}
		for _, n := range value.Nodes {
			if n.ID == "develop" {
				if n.State == "succeeded" {
					return n
				}
				if n.State == "failed" || n.State == "reconciling" {
					t.Fatal("owned original agent failed", n.State, n.Error)
				}
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(75 * time.Millisecond):
		}
	}
}
