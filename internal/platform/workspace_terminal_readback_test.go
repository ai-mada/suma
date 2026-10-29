package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
)

type terminalRetainedTableSnapshot struct {
	Columns []string `json:"columns"`
	Rows    int      `json:"rows"`
	SHA256  string   `json:"sha256"`
}

// Hash the original business and execution tables without publishing their row
// bodies. Authentication issuance is intentionally separate: normal login to
// this disposable copy may create a fresh browser session and authority rows.
func terminalRetainedBusinessTables(t *testing.T, db *sql.DB) map[string]terminalRetainedTableSnapshot {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND (name LIKE 'dag_%' OR name IN ('suma_projects','suma_tasks','suma_requirements','suma_members','suma_issues','suma_documents','suma_releases','suma_tests','suma_evidence','suma_reviews') OR name LIKE 'suma_task_%' OR name LIKE 'suma_test_%' OR name LIKE 'suma_workspace_%' OR name LIKE 'suma_candidate_%' OR name LIKE 'suma_usage_%' OR name LIKE 'suma_model_%' OR name LIKE 'suma_tool_%' OR name LIKE 'suma_release_%' OR name LIKE 'suma_application_%' OR name LIKE 'suma_repository_%' OR name LIKE 'suma_expert_team_%' OR name LIKE 'suma_project_risk%') ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	result := map[string]terminalRetainedTableSnapshot{}
	for _, table := range names {
		for _, ch := range table {
			if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '_') {
				t.Fatal("retained table name is invalid")
			}
		}
		entries, err := db.Query(`SELECT * FROM ` + table)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := entries.Columns()
		if err != nil {
			entries.Close()
			t.Fatal(err)
		}
		serialized := []string{}
		for entries.Next() {
			values := make([]any, len(columns))
			targets := make([]any, len(columns))
			for i := range targets {
				targets[i] = &values[i]
			}
			if err = entries.Scan(targets...); err != nil {
				entries.Close()
				t.Fatal(err)
			}
			for i, value := range values {
				if raw, ok := value.([]byte); ok {
					values[i] = string(raw)
				}
			}
			serialized = append(serialized, stringify(values))
		}
		if err = entries.Err(); err != nil {
			entries.Close()
			t.Fatal(err)
		}
		entries.Close()
		sort.Strings(serialized)
		result[table] = terminalRetainedTableSnapshot{Columns: columns, Rows: len(serialized), SHA256: digest(stringify(struct {
			Columns []string `json:"columns"`
			Rows    []string `json:"rows"`
		}{columns, serialized}))}
	}
	if len(result) < 15 {
		t.Fatal("retained business table snapshot is incomplete")
	}
	return result
}

// A readback of the original completed journey: copy exact retained DB bytes,
// open the copy with the current service, and hold only HTTP/Web for three
// minutes. No Fixture.Setup, worker, scheduler Tick, PTY or model is started.
func TestWorkspaceTerminalRetainedBrowserReadback(t *testing.T) {
	source := os.Getenv("SUMA_TERMINAL_REPLAY_DB")
	if source == "" {
		t.Skip("an explicit retained original terminal database is required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(root, ".cache", "e2e", "terminal-129")
	web := workbenchRetainedDirectory(t, os.Getenv("SUMA_TERMINAL_WEB_DIR"), owned, false)
	if _, err = os.Stat(filepath.Join(web, "index.html")); err != nil {
		t.Fatal(err)
	}
	source, err = filepath.Abs(source)
	if err != nil || !under(owned, source) {
		t.Fatal("retained source DB must stay inside the exact terminal stage cache")
	}
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil || resolved != source {
		t.Fatal("retained source DB must be an ordinary exact path")
	}
	info, err := os.Stat(source)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64*1024*1024 {
		t.Fatal("retained source DB is unavailable or too large")
	}
	dir := workbenchRetainedDirectory(t, os.Getenv("SUMA_TERMINAL_UI_DIR"), owned, true)
	if under(dir, source) || under(dir, web) || under(web, dir) {
		t.Fatal("retained DB, private readback and Web assets must remain separate")
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	sourceSHA := digest(string(raw))
	copyPath := filepath.Join(dir, "fixture-copy.sqlite")
	if err = os.WriteFile(copyPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	copied, err := os.ReadFile(copyPath)
	if err != nil || digest(string(copied)) != sourceSHA {
		t.Fatal("retained database copy differs from original bytes")
	}
	db, err := store.Open("sqlite", copyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	before := terminalRetainedBusinessTables(t, db)
	service := &Service{DB: db, Dialect: "sqlite", DataDir: filepath.Join(dir, "service-data")}
	if err = os.MkdirAll(service.DataDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = service.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	afterInitialize := terminalRetainedBusinessTables(t, db)
	if stringify(before) != stringify(afterInitialize) {
		t.Fatal("current initialization changed retained business/execution facts")
	}
	engine, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	var project, task, sessionID, tenant, ownerID string
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM suma_workspace_terminal_sessions WHERE state IN ('stopped','exited')`).Scan(&count); err != nil || count != 1 {
		t.Fatal("readback requires exactly the original completed terminal", err)
	}
	if err = db.QueryRow(`SELECT id,tenant_id,project_id,task_id FROM suma_workspace_terminal_sessions WHERE state IN ('stopped','exited')`).Scan(&sessionID, &tenant, &project, &task); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT id FROM suma_users WHERE email='owner@example.test'`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, s: service, e: engine, project: project, user: Principal{ID: ownerID, TenantID: tenant}}
	beforeTask := workspaceTerminalTaskSQL(t, f, task)
	server, origin, port := modelSelectionGUIServer(t, f, web, "terminal-readback129.localhost")
	// This fixture is deliberately read-only beyond ordinary sign-in/out. If
	// the UI tries to mutate a retained session it gets a real denial, never a
	// simulated success or an acknowledgement that changes the stored run.
	baseHandler := server.Config.Handler
	denied := []string{}
	var deniedMu sync.Mutex
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		read := r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions
		auth := r.Method == http.MethodPost && (r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/auth/logout")
		if !read && !auth {
			deniedMu.Lock()
			denied = append(denied, r.Method+" "+r.URL.Path)
			deniedMu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "read_only_fixture", "message": "Retained terminal browser readback is read-only"})
			return
		}
		baseHandler.ServeHTTP(w, r)
	})
	status, stopReason := "ready_for_readback", "timeout"
	defer func() {
		server.Close()
		after := terminalRetainedBusinessTables(t, db)
		afterTask := workspaceTerminalTaskSQL(t, f, task)
		unchanged := stringify(before) == stringify(after) && beforeTask.SHA256 == afterTask.SHA256
		if !unchanged {
			t.Error("retained browser readback changed business/execution tables")
		}
		current, e := os.ReadFile(source)
		originalUnchanged := e == nil && digest(string(current)) == sourceSHA
		if !originalUnchanged {
			t.Error("retained source DB bytes changed")
		}
		if len(denied) > 0 {
			t.Errorf("readback UI attempted unsupported writes: %v", denied)
		}
		if t.Failed() {
			status = "failed"
		}
		modelSelectionGUIJSON(t, filepath.Join(dir, "readback.before.public.json"), map[string]any{"business": before, "task": beforeTask})
		modelSelectionGUIJSON(t, filepath.Join(dir, "readback.after.public.json"), map[string]any{"business": after, "task": afterTask})
		modelSelectionGUIJSON(t, filepath.Join(dir, "result.public.json"), map[string]any{"stage": "T-TERMINAL-129", "kind": "actual_retained_terminal_browser_readback", "status": status, "stop_reason": stopReason, "source_db": source, "source_db_sha256": sourceSHA, "copied_exact_source_bytes": true, "original_source_unchanged": originalUnchanged, "business_tables_unchanged": unchanged, "business_table_count": len(before), "project_id": project, "task_id": task, "terminal_session_id": sessionID, "worker_runs": 0, "scheduler_ticks": 0, "new_pty_processes": 0, "new_model_requests": 0, "denied_writes": denied, "authentication": "Normal owner test-account login may write only authentication state in the disposable copy"})
	}()
	modelSelectionGUIJSON(t, filepath.Join(dir, "web.files.public.json"), modelSelectionGUIWebFiles(t, web))
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.local.json"), map[string]any{"origin": origin, "email": "owner@example.test", "password": "test-password-strong-42", "project_id": project, "task_id": task, "terminal_session_id": sessionID, "allowed_actions": []string{"read_original_terminal", "switch_locale", "refresh", "inspect_390"}})
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.ready"), map[string]any{"status": "ready", "origin": origin, "port": port, "project_id": project, "task_id": task, "terminal_session_id": sessionID, "read_only": true})
	t.Logf("retained terminal readback ready at %s; no worker, PTY, scheduler Tick or model", origin)
	deadline := time.NewTimer(3 * time.Minute)
	defer deadline.Stop()
	poll := time.NewTicker(250 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-deadline.C:
			t.Error("terminal readback timed out")
			return
		case <-poll.C:
			if _, e := os.Stat(filepath.Join(dir, "stop.request")); e == nil {
				status, stopReason = "closed_after_read_only_browser_readback", "explicit_stop"
				return
			}
		}
	}
}
