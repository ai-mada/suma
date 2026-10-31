package platform

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/runner"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// Kept outside the ordinary WorkBuddyTaskDispatch test-name prefix. This host
// runs only after the separate actual168 source/runtime gate is authorized.
func TestToolExecution168Actual(t *testing.T) {
	if os.Getenv("SUMA_TOOL_EXECUTION_168_ACTUAL") != "1" {
		t.Skip("actual168 requires its final source and runtime gate")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal("actual168 working directory unavailable")
	}
	for {
		if _, err = os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatal("actual168 repository root unavailable")
		}
		root = next
	}
	directory := filepath.Join(root, ".cache", "tool-execution-168")
	requested, err := filepath.Abs(os.Getenv("SUMA_TOOL_EXECUTION_168_ACTUAL_DIR"))
	if err != nil || filepath.Clean(requested) != filepath.Clean(directory) {
		t.Fatal("actual168 exact owned directory required")
	}
	temporary, err := filepath.Abs(os.TempDir())
	if err != nil || !strings.HasPrefix(temporary, directory+string(filepath.Separator)) {
		t.Fatal("actual168 process TEMP/TMP must remain in its owned directory")
	}
	toolExecution168Gate(t, root, directory)
	private := filepath.Join(directory, "private")
	if err = os.MkdirAll(private, 0700); err != nil {
		t.Fatal("actual168 private directory unavailable")
	}
	for _, file := range []string{filepath.Join(private, "descriptor.json"), filepath.Join(private, "snapshot.sqlite"), filepath.Join(private, "runner-journal"), filepath.Join(directory, "host.stop")} {
		if _, err = os.Stat(file); !os.IsNotExist(err) {
			t.Fatal("actual168 existing state retained; no implicit new seed or restart")
		}
	}
	var oauth, creates, queries, unexpectedVendor, blockedOutbound atomic.Int64
	f, connections := workBuddyCommandFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/openapi/v2/token":
			oauth.Add(1)
			workBuddyCommandResponse(w, "owned168-user", "user.task.readable user.task.invokable", "private-refresh")
		case r.Method == http.MethodPost && r.URL.Path == "/openapi/v2/tasks":
			creates.Add(1)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"task_id":"owned168-cloud-task","status":"running","name":"Controlled local dispatch168"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/openapi/v2/tasks/owned168-cloud-task":
			queries.Add(1)
			fmt.Fprint(w, `{"task_id":"owned168-cloud-task","status":"completed","name":"Controlled local dispatch168"}`)
		default:
			unexpectedVendor.Add(1)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	// No existing cloudFixture is used: it would already create a run and a
	// delegation before the actual product preview/start interaction.
	ctx := context.Background()
	authorization, _ := workBuddyCommandStart(t, f, connections)
	connection, err := connections.CompleteAuthorization(ctx, f.actor, f.project, authorization.ID, workBuddyCommandComplete(authorization))
	if err != nil || connection.State != "connected" || oauth.Load() != 1 {
		t.Fatal("actual168 controlled OAuth setup unavailable")
	}
	f.call("GET", "/auth/session", nil, 200)
	owner := f.view
	member := f.call("POST", "/members", f.body(map[string]any{"title": "Controlled owner168", "kind": "human", "data": map[string]any{"user_id": owner.ID}}), 201)
	requirement := f.call("POST", "/requirements", f.body(map[string]any{"title": "Explicit WorkBuddy task dispatch", "description": "Preview first, then dispatch one immutable task; provider completion is not product acceptance.", "acceptance_criteria": []string{"One platform run and delegation", "Independent delivery evidence remains required"}}), 201)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Verify a task dispatch through the original private runner", "description": "Implement the fixed controlled scope. Keep tests and acceptance independent.", "owner_id": member["id"], "requirement_id": requirement["id"], "acceptance_criteria": []string{"Explicit preview and confirmation", "No duplicated remote task after original receipt recovery"}}), 201)
	taskID, ok := task["id"].(string)
	if !ok || task["version"] != float64(1) {
		t.Fatal("actual168 original task version1 unavailable")
	}
	workerID := "workbuddy-dispatch168-private"
	credential := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": workerID, "profiles": []any{}, "deployment": false}), 201)
	workerToken, ok := credential["token"].(string)
	if !ok || workerToken == "" {
		t.Fatal("actual168 registered private worker credential unavailable")
	}
	providerOrigin, err := url.Parse(connections.app.BaseURL)
	if err != nil || providerOrigin.Scheme != "http" || providerOrigin.Hostname() != "127.0.0.1" {
		t.Fatal("actual168 controlled provider must be local loopback")
	}
	providerClient := *connections.app.HTTPClient
	providerTransport := providerClient.Transport
	if providerTransport == nil {
		providerTransport = http.DefaultTransport
	}
	providerClient.Transport = toolExecution168Transport(func(request *http.Request) (*http.Response, error) {
		if request.URL.Scheme != providerOrigin.Scheme || request.URL.Host != providerOrigin.Host {
			blockedOutbound.Add(1)
			return nil, fmt.Errorf("actual168 nonlocal provider transport refused")
		}
		return providerTransport.RoundTrip(request)
	})
	providerClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	connections.app.HTTPClient = &providerClient
	journal, err := runner.OpenJournal(filepath.Join(private, "runner-journal"))
	if err != nil {
		t.Fatal("actual168 durable private worker journal unavailable")
	}
	t.Cleanup(func() { journal.Close() })
	var workerReturned atomic.Bool
	var machineMu sync.Mutex
	machineRequests := map[string]int{}
	stats := func() map[string]any {
		out := map[string]any{"controlled_oauth_exchanges": oauth.Load(), "provider_creates": creates.Load(), "provider_queries": queries.Load(), "unexpected_vendor_requests": unexpectedVendor.Load(), "blocked_nonlocal_outbound": blockedOutbound.Load(), "worker_returned": workerReturned.Load(), "journal_pending_results": len(journal.Pending()), "journal_pending_progress": len(journal.PendingProgress()), "journal_pending_inventory": journal.PendingInventory() != nil}
		for name, table := range map[string]string{"plans": "suma_workbuddy_task_dispatch_plans", "runs": "dag_runs", "nodes": "dag_nodes", "attempts": "dag_attempts", "delegations": "suma_workbuddy_cloud_delegations", "submissions": "suma_workbuddy_cloud_submissions", "candidate_revisions": "dag_candidate_revisions", "test_plan_runs": "suma_test_plan_runs", "test_reports": "suma_test_plan_reports", "reviews": "suma_reviews", "artifacts": "suma_artifacts", "release_candidates": "suma_release_candidates"} {
			var count int
			if queryErr := f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); queryErr != nil {
				out["database_stats_unavailable"] = true
			} else {
				out[name] = count
			}
		}
		if current, taskErr := f.s.Get(ctx, f.actor, f.project, "tasks", taskID); taskErr == nil {
			out["task_version"], out["task_status"], out["task_run_id"] = current.Version, current.Status, current.RunID
			out["task_verified_run_present"] = current.Data["verified_run_id"] != nil && current.Data["verified_run_id"] != ""
			if current.RunID != "" {
				if run, runErr := f.e.GetRun(ctx, owner.TenantID, f.project, current.RunID); runErr == nil && len(run.Nodes) == 1 {
					node := run.Nodes[0]
					out["run_id"], out["run_version"], out["run_state"] = run.ID, run.Version, run.State
					out["node_id"], out["node_state"], out["node_reason"] = node.ID, node.State, node.Reason
					out["node_attempt"], out["node_fence"], out["node_worker"] = node.Attempt, node.Fence, node.WorkerID
					out["node_workspace_evidence_present"] = len(node.WorkspaceEvidence) > 0
				} else {
					out["run_stats_unavailable"] = true
				}
			}
		} else {
			out["task_stats_unavailable"] = true
		}
		rows, queryErr := f.s.DB.Query("SELECT state,payload FROM suma_workbuddy_cloud_submissions ORDER BY id")
		if queryErr == nil {
			for rows.Next() {
				var state, payload string
				if scanErr := rows.Scan(&state, &payload); scanErr != nil {
					out["submission_stats_unavailable"] = true
					break
				}
				var submission wc.CloudSubmission
				if json.Unmarshal([]byte(payload), &submission) != nil {
					out["submission_stats_unavailable"] = true
					break
				}
				out["submission_state"], out["submission_id"], out["submission_version"] = state, submission.ID, submission.Version
				if submission.Task != nil {
					out["provider_task_id"], out["provider_task_status"] = submission.Task.ID, submission.Task.Status
				}
			}
			if rows.Err() != nil {
				out["submission_stats_unavailable"] = true
			}
			rows.Close()
		} else {
			out["submission_stats_unavailable"] = true
		}
		machineMu.Lock()
		counts := map[string]int{}
		for path, count := range machineRequests {
			counts[path] = count
		}
		machineMu.Unlock()
		out["machine_http_requests"] = counts
		return out
	}
	initial := stats()
	if initial["database_stats_unavailable"] != nil || initial["task_stats_unavailable"] != nil || initial["submission_stats_unavailable"] != nil {
		t.Fatal("actual168 initialized domain unavailable before GUI")
	}
	for _, name := range []string{"plans", "runs", "nodes", "attempts", "delegations", "submissions", "candidate_revisions", "test_plan_runs", "test_reports", "reviews", "artifacts", "release_candidates", "provider_creates", "provider_queries"} {
		if fmt.Sprint(initial[name]) != "0" {
			t.Fatalf("actual168 seed expected zero %s", name)
		}
	}
	uiOrigin := "http://127.0.0.1:44037"
	platform := (&Server{Service: f.s, Engine: f.e, WorkBuddy: connections, PublicURL: uiOrigin, AllowedOrigins: []string{uiOrigin}, WebDir: filepath.Join(directory, "absent-web")}).Handler()
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__actual168/status" {
			cookie, cookieErr := r.Cookie(f.cookie.Name)
			if r.Method != http.MethodGet || r.URL.RawQuery != "" || cookieErr != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(f.cookie.Value)) != 1 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(stats())
			return
		}
		machine := strings.HasPrefix(r.URL.Path, "/api/v1/runner/")
		if machine {
			machineMu.Lock()
			machineRequests[r.URL.Path]++
			machineMu.Unlock()
		}
		preview := r.URL.Path == "/api/v1/workbuddy-task-dispatch-plans"
		start := strings.HasPrefix(r.URL.Path, "/api/v1/workbuddy-task-dispatch-plans/") && strings.HasSuffix(r.URL.Path, "/start")
		if r.Method != http.MethodGet && !(r.Method == http.MethodPost && (machine || preview || start)) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		platform.ServeHTTP(w, r)
	}))
	defer control.Close()
	workerCtx, stopWorker := context.WithCancel(ctx)
	worker := &runner.Worker{Client: &runner.Client{BaseURL: control.URL, ProjectID: f.project, Token: workerToken, HTTP: control.Client()}, Journal: journal, ID: workerID, Exec: runner.ExecConfig{AllowExec: false, Commands: map[string]runner.CommandSpec{}, CloudPollInterval: 100 * time.Millisecond, CloudObserveWindow: 5 * time.Second}, PollInterval: 150 * time.Millisecond, HeartbeatInterval: time.Second}
	workerDone := make(chan error, 1)
	go func() { result := worker.Run(workerCtx); workerReturned.Store(true); workerDone <- result }()
	joined := false
	var workerErr error
	defer func() {
		stopWorker()
		if !joined {
			select {
			case <-workerDone:
			case <-time.After(12 * time.Second):
				t.Error("actual168 worker did not close after cancellation")
			}
		}
	}()
	var sequence int
	var databaseName, databasePath string
	if err = f.s.DB.QueryRow("PRAGMA database_list").Scan(&sequence, &databaseName, &databasePath); err != nil {
		t.Fatal("actual168 source SQLite path unavailable")
	}
	toolExecution168Write(t, filepath.Join(private, "descriptor.json"), map[string]any{"stage": "T-WORKBUDDY-TASK-DISPATCH-168", "origin": control.URL, "ui_origin": uiOrigin, "pid": os.Getpid(), "project_id": f.project, "task_id": taskID, "task_version": 1, "requirement_id": requirement["id"], "connection_id": connection.ID, "connection_version": connection.Version, "grant_generation": connection.GrantGeneration, "worker_id": workerID, "owner": map[string]any{"user_id": owner.ID, "organization_id": owner.TenantID, "cookie": map[string]any{"name": f.cookie.Name, "value": f.cookie.Value}}, "initial_stats": initial, "database_source": databasePath, "data_directory_source": f.s.DataDir, "seed": "Fresh fake owner, one controlled OAuth exchange, human resource, requirement/task v1 and private worker registration through existing public handlers/services; not GUI; no run/delegation/submission/provider task"})
	toolExecution168Write(t, filepath.Join(private, "provider-config.json"), map[string]any{"fixture_only": true, "base_url": connections.app.BaseURL, "client_id": connections.app.ClientID, "client_secret": connections.app.ClientSecret, "redirect_uri": connections.app.RedirectURI, "encryption_key_base64": base64.StdEncoding.EncodeToString(connections.key)})
	t.Log("actual168 real HTTP/SQLite/private Worker.Run ready; controlled credentials remain private")
	deadline := time.NewTimer(20 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	timedOut, waiting := false, true
	for waiting {
		select {
		case <-deadline.C:
			timedOut, waiting = true, false
		case workerErr = <-workerDone:
			joined = true
			workerDone = nil
		case <-ticker.C:
			if _, stopErr := os.Stat(filepath.Join(directory, "host.stop")); stopErr == nil {
				waiting = false
			}
		}
	}
	stopWorker()
	if !joined {
		select {
		case workerErr = <-workerDone:
			joined = true
		case <-time.After(12 * time.Second):
			t.Fatal("actual168 private Worker.Run did not close")
		}
	}
	control.Close()
	final := stats()
	workerExit := "nil"
	if errors.Is(workerErr, context.Canceled) {
		workerExit = "context_canceled_after_normal_stop"
	} else if workerErr != nil {
		workerExit = "error"
	}
	snapshot := filepath.Join(private, "snapshot.sqlite")
	if _, err = os.Stat(snapshot); !os.IsNotExist(err) {
		t.Fatal("actual168 original snapshot retained")
	}
	snapshotCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err = f.s.DB.ExecContext(snapshotCtx, "VACUUM INTO ?", snapshot); err != nil {
		t.Fatal("actual168 consistent private snapshot export unavailable")
	}
	if err = os.Chmod(snapshot, 0600); err != nil {
		t.Fatal("actual168 private snapshot permissions unavailable")
	}
	data, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal("actual168 private snapshot digest unavailable")
	}
	sum := sha256.Sum256(data)
	toolExecution168Write(t, filepath.Join(private, "host-final.json"), map[string]any{"host_closed": true, "host_timeout": timedOut, "worker_joined": joined, "worker_exit": workerExit, "stats": final, "snapshot": map[string]any{"path": snapshot, "bytes": len(data), "sha256": hex.EncodeToString(sum[:]), "method": "Parameterized VACUUM INTO new private target after worker and HTTP close before t.TempDir cleanup"}, "original_database_source": databasePath, "original_data_directory_source": f.s.DataDir, "original_sources_cleanup_expected": true})
	if timedOut || workerExit == "error" {
		t.Fatal("actual168 host/worker did not finish normally; private state retained")
	}
}

type toolExecution168Transport func(*http.Request) (*http.Response, error)

func (f toolExecution168Transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func toolExecution168Write(t *testing.T, file string, value any) {
	t.Helper()
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("actual168 original private record retained")
	}
	if err = json.NewEncoder(f).Encode(value); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatal("actual168 private record persistence unavailable")
	}
}

func toolExecution168Gate(t *testing.T, root, directory string) {
	t.Helper()
	var gate struct {
		Stage             string `json:"stage"`
		RuntimeAuthorized bool   `json:"runtime_authorized"`
		Files             []struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	data, err := os.ReadFile(filepath.Join(directory, "run-authorized.json"))
	if err != nil || json.Unmarshal(data, &gate) != nil || gate.Stage != "T-WORKBUDDY-TASK-DISPATCH-168" || !gate.RuntimeAuthorized || len(gate.Files) < 1 || len(gate.Files) > 100 {
		t.Fatal("actual168 exact final source/runtime gate required")
	}
	for _, input := range gate.Files {
		absolute := filepath.Join(root, filepath.FromSlash(input.Path))
		if input.Path == "" || filepath.IsAbs(input.Path) || !strings.HasPrefix(absolute, root+string(filepath.Separator)) {
			t.Fatal("actual168 gate input escaped repository")
		}
		bytes, readErr := os.ReadFile(absolute)
		digest := sha256.Sum256(bytes)
		if readErr != nil || len(input.SHA256) != 64 || hex.EncodeToString(digest[:]) != input.SHA256 {
			t.Fatal("actual168 source gate changed before seed")
		}
	}
}
