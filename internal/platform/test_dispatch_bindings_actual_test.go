package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
)

// One opt-in vertical journey: the production dialog explicitly chooses a
// test tool, then the original two Workers produce and independently test real
// Git code. The developer is an owned deterministic Suma host, not a vendor
// transport imitation, a model or proof of autonomous engineering intelligence.
func TestTaskDispatchExplicitTestsActualOriginalWorkersAndQuality(t *testing.T) {
	if os.Getenv("SUMA_TEST_DISPATCH_BINDINGS_216_ACTUAL") != "1" {
		t.Skip("explicit isolated test-binding actual journey required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	node := os.Getenv("SUMA_TEST_DISPATCH_BINDINGS_216_NODE")
	driver := filepath.Join(root, "tests", "integration", "test-dispatch-bindings", "driver.cjs")
	web := filepath.Join(root, ".cache", "test-dispatch-bindings-216", "web", "out")
	for _, file := range []string{node, driver, filepath.Join(web, "index.html")} {
		if !filepath.IsAbs(file) {
			t.Fatal("prepared absolute Node, driver and UI assets required")
		}
		if info, e := os.Stat(file); e != nil || !info.Mode().IsRegular() {
			t.Fatal("prepared actual journey assets unavailable")
		}
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal("Git required for the owned candidate journey")
	}
	git, err = filepath.Abs(git)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, ".cache", "test-dispatch-bindings-216", "actual", strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000Z"), ".", ""))
	if err = os.MkdirAll(filepath.Dir(directory), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(directory, 0700); err != nil {
		t.Fatal("actual evidence directory must be new")
	}
	temporary := filepath.Join(directory, "tmp")
	if err = os.Mkdir(temporary, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TEMP", "TMP", "TMPDIR"} {
		t.Setenv(name, temporary)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	f := newFixture(t)
	f.e.AttemptAccounting = f.s
	receipt := map[string]any{"stage": "T-TEST-DISPATCH-BINDINGS-216", "passed": false, "kind": "production_dialog_and_original_workers_deterministic_suma_host_real_git_junit", "vendor_calls": 0, "model_calls": 0, "customer_signoff": false, "process_temp": temporary}
	defer func() {
		if _, e := f.s.DB.ExecContext(context.Background(), "VACUUM INTO ?", filepath.Join(directory, "private.sqlite")); e != nil {
			t.Error("preserve independent private database snapshot", e)
		}
		if t.Failed() {
			receipt["passed"] = false
		}
		raw, _ := json.MarshalIndent(receipt, "", "  ")
		if e := os.WriteFile(filepath.Join(directory, "receipt.json"), append(raw, '\n'), 0600); e != nil {
			t.Error("preserve bounded actual receipt", e)
		}
		t.Log("owned actual evidence retained at", directory)
	}()
	repo := filepath.Join(directory, "repository")
	if err = os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	workspaceGitTestCommand(t, repo, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, repo, ".gitignore", ".cache/\n")
	workspaceGitTestWrite(t, repo, "file.txt", "baseline216\n")
	workspaceGitTestCommand(t, repo, "add", ".gitignore", "file.txt")
	workspaceGitTestCommand(t, repo, "commit", "-m", "fixture: original test-binding baseline")
	base := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD")
	const draft = "original administrator draft216\n"
	workspaceGitTestWrite(t, repo, "file.txt", draft)
	gitJSON, _ := json.Marshal(git)
	// Static file/command literals are administrator owned. The goal is read as
	// task context, never as an executable, path or argument template.
	hostSource := strings.ReplaceAll(`import fs from 'node:fs';import path from 'node:path';import cp from 'node:child_process';import readline from 'node:readline';
if(process.argv.includes('--version')){console.log('suma-agent-runtime 1.0.0-owned216');process.exit(0)}
if(process.argv.includes('--help')){console.log('Owned deterministic Suma JSONL fixture; SUMA_AGENT_TOKEN protocol marker, no model or token access.');process.exit(0)}
const git=__GIT__,lines=readline.createInterface({input:process.stdin,crlfDelay:Infinity});
lines.once('line',line=>{const input=JSON.parse(line);if(typeof input.request_id!=='string'||typeof input.goal!=='string'||!input.goal.trim())throw Error('Missing original Suma request');const emit=value=>console.log(JSON.stringify({...value,request_id:input.request_id}));emit({type:'started'});
const privateGit=path.resolve('.cache','owned216-git'),hooks=path.join(privateGit,'empty-hooks'),config=path.join(privateGit,'empty.gitconfig');fs.mkdirSync(hooks,{recursive:true,mode:0o700});fs.writeFileSync(config,'',{flag:'wx',mode:0o600});const env={...process.env,GIT_CONFIG_NOSYSTEM:'1',GIT_CONFIG_GLOBAL:config,GIT_TERMINAL_PROMPT:'0'};
fs.writeFileSync('file.txt','candidate216\n');const flags=['-c','core.hooksPath='+hooks,'-c','user.name=Owned Suma fixture','-c','user.email=owned-suma@example.invalid','-c','commit.gpgsign=false','-c','core.autocrlf=false'];for(const args of [['add','--','file.txt'],['commit','-m','fixture: deterministic exact candidate216']])cp.execFileSync(git,[...flags,...args],{env,stdio:'pipe',windowsHide:true});emit({type:'completed',output:'Owned deterministic code host committed the candidate; independent testing remains required.'});lines.close();});
`, "__GIT__", string(gitJSON))
	entry := filepath.Join(directory, "owned-suma216.mjs")
	if err = os.WriteFile(entry, []byte(hostSource), 0600); err != nil {
		t.Fatal(err)
	}
	verify := strings.ReplaceAll(`const fs=require('node:fs'),path=require('node:path'),cp=require('node:child_process'),git=__GIT__;const own=path.resolve('.cache','owned216-test-git'),config=path.join(own,'empty.gitconfig');fs.mkdirSync(own,{recursive:true,mode:0o700});fs.writeFileSync(config,'',{flag:'wx',mode:0o600});const env={...process.env,GIT_CONFIG_NOSYSTEM:'1',GIT_CONFIG_GLOBAL:config,GIT_TERMINAL_PROMPT:'0'};const passed=fs.readFileSync('file.txt','utf8')==='candidate216\n'&&cp.execFileSync(git,['-c','core.fsmonitor=false','status','--porcelain=v1'],{env,encoding:'utf8',windowsHide:true}).trim()==='';fs.writeFileSync(process.argv[1],'<testsuite name="delivery216" tests="1" failures="'+(passed?'0':'1')+'"><testcase classname="delivery" name="candidate216">'+(passed?'':'<failure message="exact clean candidate required"/>')+'</testcase></testsuite>');process.exitCode=passed?0:1;`, "__GIT__", string(gitJSON))
	developer := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, WorktreeRoot: filepath.Join(repo, ".cache", "developer-worktrees"), Commands: map[string]runner.CommandSpec{"owned-develop216": {Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: node, Args: []string{entry}, Input: "agent-jsonl"}}}
	independent := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, WorktreeRoot: filepath.Join(repo, ".cache", "independent-worktrees"), Commands: map[string]runner.CommandSpec{"owned-test216": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", verify, "{{test_report_path}}"}, TestEvidence: true, TestReport: true}}}
	developerProfile, developerToken := issueRepairRuntimeTool(t, f, "owned-developer216", &developer)
	testProfile, testToken := issueRepairRuntimeTool(t, f, "owned-tester216", &independent)
	receipt["developer_profile"], receipt["test_profile"], receipt["owned_host_sha256"] = developerProfile, testProfile, digest(hostSource)
	task := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "Explicit independent tool binding", "description": "Owned deterministic Suma host, real fixed Git candidate, different Worker and JUnit; no model", "acceptance_criteria": []string{"Another Worker tests candidate216 at the exact committed source"}}), 201))
	quality := issueRepairRuntimePlan(t, f, task, QualityTestCase{Key: "candidate216", Title: "Independent exact candidate", Description: "Real current candidate and original JUnit", Preconditions: "Owned producer committed candidate216", Steps: []string{"Read file.txt", "Check the clean exact candidate"}, ExpectedResult: "candidate216 at the fixed commit", Priority: "high", Covers: []int{1}, Selector: tr.TestCaseSelector{ClassName: "delivery", Name: "candidate216"}})
	workflow, _, _ := taskDispatchBuildFixture()
	workflow.ID, workflow.Version = "", 0
	workflow.Nodes[0].Command = []string{"old-published-developer216"}
	workflow.Nodes[0].Config = map[string]any{"goal": "Implement the owned exact candidate216 objective; preserve the independent test and human review gates."}
	workflow.Nodes[1].Command = []string{"old-published-test216"}
	workflow.Nodes[1].Config = map[string]any{"candidate_from": "develop", "capability": "command:old-published-test216"}
	workflow, err = f.e.Publish(ctx, f.user.TenantID, f.project, workflow)
	if err != nil {
		t.Fatal(err)
	}
	var originalWorkflow string
	if err = f.s.DB.QueryRow(`SELECT spec FROM dag_workflows WHERE id=? AND version=?`, workflow.ID, workflow.Version).Scan(&originalWorkflow); err != nil {
		t.Fatal(err)
	}
	publicSession := f.call("GET", "/auth/session", nil, 200)
	app := (&Server{Service: f.s, Engine: f.e, WebDir: web}).Handler()
	var traceMu sync.Mutex
	type requestFact struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Status int    `json:"status"`
	}
	trace := []requestFact{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/__fixture/view" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			current, e := f.s.Get(r.Context(), f.actor, f.project, "tasks", task.ID)
			if e != nil {
				http.Error(w, "Current task unavailable", 500)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"session": publicSession, "task": current, "project_id": f.project, "workflow_id": workflow.ID, "workflow_version": workflow.Version, "developer_profile_id": developerProfile.ID, "test_profile_id": testProfile.ID})
			return
		}
		observed := &toolTaskHandoffResponse213{ResponseWriter: w}
		app.ServeHTTP(observed, r)
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			traceMu.Lock()
			trace = append(trace, requestFact{Method: r.Method, Path: r.URL.Path, Status: observed.status})
			traceMu.Unlock()
		}
	}))
	defer server.Close()
	privateInput, _ := json.Marshal(map[string]any{"protocol": "suma-test-dispatch-bindings-actual-v1", "origin": server.URL, "directory": directory, "sessionCookie": map[string]any{"name": f.cookie.Name, "value": f.cookie.Value}, "project": f.project, "task_id": task.ID, "workflow_id": workflow.ID, "workflow_version": workflow.Version, "developer_profile_id": developerProfile.ID, "test_profile_id": testProfile.ID})
	command := exec.CommandContext(ctx, node, driver)
	command.Dir, command.Stdin = root, bytes.NewReader(privateInput)
	// Do not pass provider, Runner or interactive-login environment values to
	// the browser driver. Its session credential travels only over private stdin.
	for _, name := range []string{"PATH", "SystemRoot", "WINDIR", "SystemDrive", "COMSPEC", "PATHEXT", "PROGRAMFILES", "PROGRAMW6432", "COMMONPROGRAMFILES", "COMMONPROGRAMW6432", "SUMA_PLAYWRIGHT_MODULE"} {
		if value, present := os.LookupEnv(name); present {
			command.Env = append(command.Env, name+"="+value)
		}
	}
	command.Env = append(command.Env, "TEMP="+temporary, "TMP="+temporary, "TMPDIR="+temporary)
	attributes := &syscall.SysProcAttr{}
	if hidden := reflect.ValueOf(attributes).Elem().FieldByName("HideWindow"); hidden.IsValid() && hidden.CanSet() && hidden.Kind() == reflect.Bool {
		hidden.SetBool(true)
		command.SysProcAttr = attributes
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err = command.Run()
	for name, raw := range map[string][]byte{"driver.stdout.txt": stdout.Bytes(), "driver.stderr.txt": stderr.Bytes()} {
		redacted := bytes.ReplaceAll(raw, []byte(f.cookie.Value), []byte("[private-session]"))
		if e := os.WriteFile(filepath.Join(directory, name), redacted, 0600); e != nil {
			t.Fatal("preserve redacted browser driver diagnostics", e)
		}
	}
	if err != nil {
		t.Fatal("original browser preparation failed; inspect owned evidence directory")
	}
	uiRaw, err := os.ReadFile(filepath.Join(directory, "ui-result.json"))
	var ui struct {
		Passed bool   `json:"passed"`
		PlanID string `json:"plan_id"`
	}
	if err != nil || len(uiRaw) > 2<<20 || json.Unmarshal(uiRaw, &ui) != nil || !ui.Passed || ui.PlanID == "" {
		t.Fatal("bounded browser selection result unavailable")
	}
	count := func(table string) int {
		var n int
		// This newFixture database is exclusively owned by this one journey.
		// DAG attempts inherit scope through run_id and have no tenant columns.
		if e := f.s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	traceMu.Lock()
	previews := 0
	for _, request := range trace {
		if request.Method == http.MethodPost && request.Path == "/api/v1/task-dispatch-plans" {
			if request.Status != 201 {
				t.Error("preview did not return original created receipt")
			}
			previews++
		}
	}
	traceMu.Unlock()
	if previews != 1 || count("suma_task_dispatch_plans") != 1 || count("dag_runs") != 0 || count("dag_attempts") != 0 {
		t.Fatal("browser did not produce exactly one explicit preview without execution")
	}
	var payload string
	if err = f.s.DB.QueryRow(`SELECT payload FROM suma_task_dispatch_plans WHERE tenant_id=? AND project_id=? AND id=?`, f.user.TenantID, f.project, ui.PlanID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var plan TaskDispatchPlan
	if json.Unmarshal([]byte(payload), &plan) != nil || plan.TaskID != task.ID || plan.WorkflowID != workflow.ID || plan.WorkflowVersion != workflow.Version || plan.ToolProfileID != developerProfile.ID || len(plan.TestBindings) != 1 || !taskDispatchTestBindingMatchesProfile(plan.TestBindings[0], testProfile, developerProfile) || !taskDispatchTestBindingsConsistent(plan) {
		t.Fatal("browser plan did not retain the exact selected tool/Worker/source")
	}
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal("start original explicitly bound plan", err)
	}
	shared := originalDevelopmentRuntime{f: f, control: server, directory: directory, receipt: receipt}
	stopDeveloper := shared.startWorker(t, ctx, developerProfile.WorkerID, developerToken, "developer-journal", developer)
	defer func() {
		if stopDeveloper != nil {
			stopDeveloper()
		}
	}()
	var produced dag.NodeRun
	for produced.State != "succeeded" {
		current, e := f.e.GetRun(ctx, f.user.TenantID, f.project, run.ID)
		if e != nil {
			t.Fatal(e)
		}
		for _, node := range current.Nodes {
			if node.ID == "develop" {
				produced = node
			}
		}
		if produced.State == "failed" || produced.State == "reconciling" || ctx.Err() != nil {
			receipt["producer"] = produced
			t.Fatal("owned original agent did not produce a known successful candidate", produced.State, produced.Error)
		}
		if produced.State == "succeeded" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(75 * time.Millisecond):
		}
	}
	stopDeveloper()
	stopDeveloper = nil
	candidate, valid := produced.WorkspaceEvidence["head_commit"].(string)
	if !valid || !gitRevision.MatchString(candidate) || candidate == base || produced.WorkerID != developerProfile.WorkerID || produced.WorkspaceEvidence["clean"] != true {
		t.Fatal("owned producer lacks a clean committed code candidate")
	}
	verified, execution, accepted := shared.finishQuality(t, ctx, originalDevelopmentQuality{Repository: repo, BaseCommit: base, CandidateCommit: candidate, ExpectedDraft: draft, Task: task, Plan: quality, Run: run, Producer: produced, TestProfile: testProfile, TestToken: testToken, Independent: independent, Reviewer: invitedReviewer(t, f)})
	var afterWorkflow string
	if err = f.s.DB.QueryRow(`SELECT spec FROM dag_workflows WHERE id=? AND version=?`, workflow.ID, workflow.Version).Scan(&afterWorkflow); err != nil || afterWorkflow != originalWorkflow {
		t.Fatal("explicit override changed the original published workflow", err)
	}
	if count("dag_runs") != 1 || count("dag_attempts") != 2 || count("suma_tool_profiles") != 2 {
		t.Fatal("actual preparation created extra execution or tool configuration")
	}
	traceMu.Lock()
	receipt["requests"] = append([]requestFact{}, trace...)
	traceMu.Unlock()
	receipt["passed"], receipt["origin"], receipt["ui_result_sha256"] = true, server.URL, digest(string(uiRaw))
	receipt["plan_id"], receipt["run_id"], receipt["task_id"], receipt["project_id"] = plan.ID, run.ID, task.ID, f.project
	receipt["test_bindings"], receipt["base_commit"], receipt["candidate_commit"] = plan.TestBindings, base, candidate
	receipt["producer"], receipt["independent_test"], receipt["test_plan_execution"], receipt["accepted_task"] = produced, verified, execution, accepted
	receipt["preview_calls"], receipt["run_count"], receipt["attempt_count"], receipt["published_workflow_preserved"], receipt["administrator_checkout_retained"] = previews, 1, 2, true, true
}
