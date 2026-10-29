package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/runner"
	term "github.com/ai-mada/suma/internal/terminalcontract"
	"github.com/ai-mada/suma/internal/terminalruntime"
)

// Like the production suma-runner entrypoint, the platform test executable
// explicitly forwards only the dedicated PTY child mode to the real bootstrap.
// Ordinary test startup has no console/process side effect.
func init() {
	if handled, code := terminalruntime.HandleChildProcess(os.Args[1:]); handled {
		os.Exit(code)
	}
}

// This snapshot binds all persistent task columns, including their exact JSON
// text and timestamps. Read projections such as Record.Name are separate facts.
type workspaceTerminalTaskSQLSnapshot struct {
	Columns []string `json:"columns"`
	Values  []any    `json:"values"`
	SHA256  string   `json:"sha256"`
}

func workspaceTerminalTaskSQL(t *testing.T, f *fixture, id string) workspaceTerminalTaskSQLSnapshot {
	t.Helper()
	result := workspaceTerminalTaskSQLSnapshot{Columns: strings.Split(recordColumns, ",")}
	result.Values = make([]any, len(result.Columns))
	targets := make([]any, len(result.Columns))
	for i := range targets {
		targets[i] = &result.Values[i]
	}
	if err := f.s.DB.QueryRow(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`, f.user.TenantID, f.project, id).Scan(targets...); err != nil {
		t.Fatal(err)
	}
	for i, value := range result.Values {
		if b, ok := value.([]byte); ok {
			result.Values[i] = string(b)
		}
	}
	result.SHA256 = digest(stringify(struct {
		Columns []string `json:"columns"`
		Values  []any    `json:"values"`
	}{result.Columns, result.Values}))
	return result
}

func TestWorkspaceTerminalTaskPersistenceAcrossCreateAndGet(t *testing.T) {
	f := newFixture(t)
	created := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "Exact persistent terminal task", "description": "No PTY is started by this Create/Get regression."}), 201))
	before := workspaceTerminalTaskSQL(t, f, created.ID)
	loaded, err := f.s.Get(context.Background(), f.actor, f.project, "tasks", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	after := workspaceTerminalTaskSQL(t, f, created.ID)
	if before.SHA256 != after.SHA256 || stringify(before) != stringify(after) {
		t.Fatalf("Create/Get changed persistent task: before=%s after=%s", stringify(before), stringify(after))
	}
	if created.Name != "" || loaded.Name != loaded.Title {
		t.Fatal("expected current derived Name projection was not observed")
	}
	projection := loaded
	projection.Name = ""
	if stringify(projection) != stringify(created) {
		t.Fatalf("Create/Get differ beyond derived Name: created=%s loaded=%s", stringify(created), stringify(loaded))
	}
	t.Logf("all %d persistent columns unchanged, sha256=%s; only derived Name differs between Create and Get", len(before.Columns), before.SHA256)
}

// This opt-in journey holds one fresh, actual Windows/Go/SQLite/Runner/ConPTY
// environment for explicit browser input. It never fabricates terminal output,
// starts a second shell after a lost acknowledgement, or uses a customer DB.
func TestWorkspaceTerminalActualBrowserAndOriginalExchange(t *testing.T) {
	dir, web := os.Getenv("SUMA_TERMINAL_UI_DIR"), os.Getenv("SUMA_TERMINAL_WEB_DIR")
	if dir == "" && web == "" {
		t.Skip("explicit isolated browser evidence paths are required")
	}
	if runtime.GOOS != "windows" {
		t.Skip("Windows ConPTY/Job actual journey; other OS containment is separately pending")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(root, ".cache", "e2e", "terminal-129")
	web = workbenchRetainedDirectory(t, web, owned, false)
	if _, err = os.Stat(filepath.Join(web, "index.html")); err != nil {
		t.Fatal(err)
	}
	dir = workbenchRetainedDirectory(t, dir, owned, true)
	if under(web, dir) || under(dir, web) {
		t.Fatal("terminal browser assets and private evidence must be separate")
	}
	f := newFixture(t)
	code := filepath.Join(dir, "source")
	if err = os.Mkdir(code, 0700); err != nil {
		t.Fatal(err)
	}
	workspaceGitTestCommand(t, code, "init", "--initial-branch=main")
	const original = "module.exports=(a,b)=>a+b;\n"
	workspaceGitTestWrite(t, code, "sum.cjs", original)
	workspaceGitTestWrite(t, code, ".gitignore", ".cache/\n")
	workspaceGitTestCommand(t, code, "add", "--", "sum.cjs", ".gitignore")
	workspaceGitTestCommand(t, code, "commit", "-m", "Owned terminal fixture source")
	base := workspaceGitTestCommand(t, code, "rev-parse", "HEAD")
	t.Setenv("SUMA_WORKSPACE_ROOT", code)
	f.call("POST", "/workspace/binding", f.body(map[string]any{"path": code}), 200)
	task := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "真实工作区终端 / Real workspace terminal", "description": "Terminal activity remains separate from native development, independent tests and human acceptance."}), 201))
	createdTask := task
	task, err = f.s.Get(context.Background(), f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	beforeTaskSQL := workspaceTerminalTaskSQL(t, f, task.ID)
	modelSelectionGUIJSON(t, filepath.Join(dir, "task.before.public.json"), map[string]any{"create_response": createdTask, "get_projection": task, "persistent": beforeTaskSQL})
	executable, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Fatal(err)
	}
	cfg := runner.ExecConfig{AllowExec: true, Workspace: code, Repository: code, BaseCommit: base, WorktreeRoot: filepath.Join(code, ".cache", "terminal-worktrees"), Commands: map[string]runner.CommandSpec{"terminal129": {Adapter: "command", Protocol: "text-v1", Executable: executable, Args: []string{"/d", "/q", "/k"}, TerminalSession: true}}}
	profile, token := issueRepairRuntimeTool(t, f, "terminal-worker129", &cfg)
	server, origin, port := modelSelectionGUIServer(t, f, web, "terminal129.localhost")
	var mu sync.Mutex
	exchanges, originalReplays, outputChunks, inputReceipts := 0, 0, 0, 0
	dropped := false
	requestHashes := map[string]string{}
	originalHandler := server.Config.Handler
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || !strings.Contains(r.URL.Path, "/runner/terminal-sessions/") || !strings.HasSuffix(r.URL.Path, "/exchange") {
			originalHandler.ServeHTTP(w, r)
			return
		}
		raw, e := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		if e != nil {
			w.WriteHeader(400)
			return
		}
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var request term.ExchangeRequest
		if e = json.Unmarshal(raw, &request); e != nil {
			w.WriteHeader(400)
			return
		}
		hash := digest(string(raw))
		mu.Lock()
		exchanges++
		if previous, ok := requestHashes[request.OperationID]; ok {
			if previous != hash {
				t.Error("original exchange operation changed bytes")
			}
			originalReplays++
		} else {
			requestHashes[request.OperationID] = hash
		}
		mu.Unlock()
		response := httptest.NewRecorder()
		originalHandler.ServeHTTP(response, r)
		mu.Lock()
		if response.Code >= 200 && response.Code < 300 {
			outputChunks += len(request.OutputChunks)
			inputReceipts += len(request.InputReceipts)
		}
		lose := !dropped && response.Code >= 200 && response.Code < 300 && len(request.OutputChunks) > 0
		if lose {
			dropped = true
		}
		mu.Unlock()
		if lose {
			connection, _, e := w.(http.Hijacker).Hijack()
			if e != nil {
				t.Error(e)
				return
			}
			_ = connection.Close()
			return
		}
		for key, values := range response.Header() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(response.Code)
		_, _ = w.Write(response.Body.Bytes())
	})
	journal, err := runner.OpenJournal(filepath.Join(dir, "runner-journal"))
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	client := &runner.Client{BaseURL: fmt.Sprintf("http://127.0.0.1:%d", port), Token: token, ProjectID: f.project}
	worker := &runner.Worker{Client: client, Journal: journal, ID: profile.WorkerID, Exec: cfg, PollInterval: 150 * time.Millisecond, HeartbeatInterval: time.Second}
	workerDone := make(chan error, 1)
	var observedWorkerErr error
	workerEnded := false
	go func() { workerDone <- worker.Run(workerCtx) }()
	tickDone := make(chan struct{})
	go func() {
		defer close(tickDone)
		tick := time.NewTicker(150 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-tick.C:
				if e := f.e.Tick(workerCtx); e != nil && workerCtx.Err() == nil {
					t.Error(e)
				}
			}
		}
	}()
	status, stopReason := "failed", "not_ready"
	defer func() {
		cancelWorker()
		workerErr := observedWorkerErr
		if !workerEnded {
			select {
			case workerErr = <-workerDone:
			case <-time.After(20 * time.Second):
				workerErr = fmt.Errorf("terminal worker did not stop within deadline")
				t.Error(workerErr)
			}
		}
		<-tickDone
		_ = journal.Close()
		server.Close()
		if workerErr != nil && !errors.Is(workerErr, context.Canceled) {
			t.Error(workerErr)
		}
		current, e := f.s.Get(context.Background(), f.actor, f.project, "tasks", task.ID)
		afterTaskSQL := workspaceTerminalTaskSQL(t, f, task.ID)
		taskUnchanged := e == nil && stringify(current) == stringify(task) && beforeTaskSQL.SHA256 == afterTaskSQL.SHA256
		modelSelectionGUIJSON(t, filepath.Join(dir, "task.after.public.json"), map[string]any{"get_projection": current, "persistent": afterTaskSQL, "all_persistent_columns_equal": beforeTaskSQL.SHA256 == afterTaskSQL.SHA256, "get_projection_equal": stringify(current) == stringify(task)})
		if e != nil {
			t.Error(e)
		} else if !taskUnchanged {
			t.Error("terminal changed original task business facts")
		}
		parentBytes, e := os.ReadFile(filepath.Join(code, "sum.cjs"))
		if e != nil || string(parentBytes) != original {
			t.Error("terminal changed parent source")
		}
		if _, e = os.Stat(filepath.Join(code, "terminal-note.txt")); !os.IsNotExist(e) {
			t.Error("terminal wrote into parent checkout")
		}
		page, e := f.s.ListWorkspaceTerminals(context.Background(), f.actor, f.project, task.ID)
		if e != nil {
			t.Error(e)
		}
		var terminal WorkspaceTerminalSession
		if e == nil && len(page.Items) == 1 {
			terminal = page.Items[0]
		} else {
			t.Errorf("expected one explicit UI terminal, got %d", len(page.Items))
		}
		var treeStopped bool
		var finalJSON string
		if terminal.ID != "" {
			if e = f.s.DB.QueryRow(`SELECT payload FROM suma_workspace_terminal_sessions WHERE tenant_id=? AND project_id=? AND id=?`, f.user.TenantID, f.project, terminal.ID).Scan(&finalJSON); e == nil {
				var stored workspaceTerminalRecord
				if e = json.Unmarshal([]byte(finalJSON), &stored); e == nil {
					treeStopped = stored.TreeStopped
				}
			}
		}
		directory := terminal.Directory
		edit, e := os.ReadFile(filepath.Join(directory, "terminal-note.txt"))
		if terminal.ID == "" || !under(cfg.WorktreeRoot, directory) || directory == cfg.WorktreeRoot || e != nil || strings.TrimSpace(string(edit)) != "terminal-ui-129" {
			t.Errorf("UI did not write expected isolated attempt file: %v", e)
		}
		if terminal.State != "stopped" && terminal.State != "exited" {
			t.Errorf("terminal original stop is not confirmed: %s", terminal.State)
		}
		if !treeStopped {
			t.Error("original terminal process tree stop was not recorded")
		}
		counts := map[string]int{}
		for _, table := range []string{"dag_runs", "dag_attempts", "suma_reviews", "suma_usage_attempts", "suma_test_plan_runs", "suma_evidence", "suma_workspace_writes"} {
			counts[table] = expertCount(t, f, table)
		}
		for _, table := range []string{"suma_reviews", "suma_usage_attempts", "suma_test_plan_runs", "suma_evidence", "suma_workspace_writes"} {
			if counts[table] != 0 {
				t.Errorf("terminal produced unrelated formal facts in %s", table)
			}
		}
		mu.Lock()
		network := map[string]any{"exchange_http_requests": exchanges, "original_operation_replays": originalReplays, "postcommit_output_ack_lost": dropped, "successful_output_chunks_including_replays": outputChunks, "reported_input_receipts_including_replays": inputReceipts, "operation_request_sha256": requestHashes}
		mu.Unlock()
		if !dropped || originalReplays < 1 {
			t.Error("original output ACK-loss recovery was not exercised")
		}
		workerError := ""
		if workerErr != nil {
			workerError = workerErr.Error()
		}
		public := map[string]any{"stage": "T-TERMINAL-129", "kind": "actual_windows_pty_browser_worker", "status": status, "stop_reason": stopReason, "original_task_unchanged": taskUnchanged, "parent_source_sha256": digest(string(parentBytes)), "parent_base_commit": base, "terminal": terminal, "tree_stop_record_detected": treeStopped, "isolated_note_sha256": digest(string(edit)), "counts": counts, "network": network, "commercial_model_requests": 0, "formal_candidate": false, "independent_test_claimed": false, "human_acceptance": false, "worker_error": workerError}
		if t.Failed() {
			public["status"] = "failed"
		}
		modelSelectionGUIJSON(t, filepath.Join(dir, "result.public.json"), public)
		snapshot := filepath.Join(dir, "fixture.sqlite")
		if _, e = f.s.DB.Exec("VACUUM INTO '" + strings.ReplaceAll(snapshot, "'", "''") + "'"); e != nil {
			t.Error(e)
		} else if raw, e := os.ReadFile(snapshot); e == nil {
			modelSelectionGUIJSON(t, filepath.Join(dir, "database.public.json"), map[string]any{"bytes": len(raw), "sha256": digest(string(raw))})
		}
	}()
	modelSelectionGUIJSON(t, filepath.Join(dir, "web.files.public.json"), modelSelectionGUIWebFiles(t, web))
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.local.json"), map[string]any{"origin": origin, "email": "owner@example.test", "password": "test-password-strong-42", "project_id": f.project, "task_id": task.ID, "tool_profile_id": profile.ID, "tool_profile_version": profile.Version, "command": "echo terminal-ui-129>terminal-note.txt\r", "allowed_actions": []string{"create_one_terminal", "input_exact_command", "resize", "read_current_attempt", "refresh", "stop_original_terminal"}, "terminal_columns": 100, "terminal_rows": 28, "timeout_seconds": 600})
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.ready"), map[string]any{"status": "ready", "origin": origin, "project_id": f.project, "task_id": task.ID, "tool_profile_id": profile.ID, "expected_terminals": 1})
	status, stopReason = "ready_for_actual_ui", "timeout"
	t.Logf("actual terminal UI ready at %s; create one terminal, write isolated note, resize, refresh and stop", origin)
	deadline := time.NewTimer(7 * time.Minute)
	defer deadline.Stop()
	poll := time.NewTicker(250 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-workerDone:
			workerEnded = true
			observedWorkerErr = fmt.Errorf("terminal worker ended before explicit GUI stop")
			t.Error(observedWorkerErr)
			return
		case <-deadline.C:
			t.Error("terminal GUI hold timed out")
			return
		case <-poll.C:
			if _, e := os.Stat(filepath.Join(dir, "stop.request")); e == nil {
				status, stopReason = "closed_after_actual_terminal_ui", "explicit_stop"
				return
			}
		}
	}
}
