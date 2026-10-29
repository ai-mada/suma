package platform

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
)

type workbenchRetainedFile struct {
	Path   string `json:"path"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// This opt-in journey reopens immutable evidence from an actual prior execution.
// It deliberately does not create a Runner, advance the DAG, reproduce an old
// approval, or label extracted source bytes as a live Git worktree/attempt.
func TestWorkspaceTaskContextActualRetainedSnapshotHTTP(t *testing.T) {
	source := os.Getenv("SUMA_WORKBENCH_SOURCE_DIR")
	if source == "" {
		t.Skip("explicit retained actual source, final web build and fresh GUI directory are required")
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	source = workbenchRetainedDirectory(t, source, filepath.Join(repository, ".cache", "e2e", "rollout-125"), false)
	if source != filepath.Join(repository, ".cache", "e2e", "rollout-125", "actual-ui") {
		t.Fatal("only the pinned T125 actual execution snapshot is accepted")
	}
	owned := filepath.Join(repository, ".cache", "e2e", "workbench-126")
	web := workbenchRetainedDirectory(t, os.Getenv("SUMA_WORKBENCH_WEB_DIR"), owned, false)
	if _, err = os.Stat(filepath.Join(web, "index.html")); err != nil {
		t.Fatal("build final workbench assets before opening retained execution evidence", err)
	}
	dir := workbenchRetainedDirectory(t, os.Getenv("SUMA_WORKBENCH_UI_DIR"), owned, true)
	if under(web, dir) || under(dir, web) {
		t.Fatal("GUI receipts and final assets must be separate directories")
	}

	// Pins come from the committed previous delivery report. The loose local
	// receipt supplies provenance metadata, never the authority to trust bytes.
	var delivery struct {
		Evidence struct {
			Artifacts []workbenchRetainedFile `json:"artifacts"`
		} `json:"evidence"`
	}
	workbenchRetainedJSON(t, filepath.Join(repository, "docs", "quality", "application-release-rollouts-summary.json"), &delivery)
	pins := map[string]workbenchRetainedFile{}
	for _, pin := range delivery.Evidence.Artifacts {
		pins[filepath.ToSlash(pin.Path)] = pin
	}
	var old struct {
		Status         string                  `json:"status"`
		DatabaseSHA256 string                  `json:"database_sha256"`
		RetainedData   []workbenchRetainedFile `json:"retained_data"`
		SourceArchives []struct {
			Archive string                  `json:"archive"`
			Commit  string                  `json:"commit"`
			SHA256  string                  `json:"sha256"`
			Files   []workbenchRetainedFile `json:"files"`
		} `json:"source_archives"`
	}
	workbenchRetainedJSON(t, filepath.Join(source, "result.public.json"), &old)
	if old.Status != "closed_without_new_execution" || old.DatabaseSHA256 != "8a45a7641d5609e5bbea9b6624868810081f3c407e93fade8de249d436738a2e" {
		t.Fatal("the retained original execution receipt is not the verified T125 snapshot")
	}
	provenance := []workbenchRetainedFile{}
	copyPinned := func(relative, target string) []byte {
		t.Helper()
		key := ".cache/e2e/rollout-125/actual-ui/" + relative
		pin, ok := pins[key]
		if !ok {
			t.Fatal("retained bytes lack a committed delivery pin", relative)
		}
		raw := workbenchRetainedRead(t, source, relative, pin)
		if target != "" {
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
		provenance = append(provenance, pin)
		return raw
	}
	database := filepath.Join(dir, "working.sqlite")
	copyPinned("fixture.sqlite", database)
	data := filepath.Join(dir, "data")
	if len(old.RetainedData) != 3 {
		t.Fatal("incomplete original immutable artifact manifest")
	}
	for _, item := range old.RetainedData {
		relative := "retained-data/" + item.Path
		if pin, ok := pins[".cache/e2e/rollout-125/actual-ui/"+relative]; !ok || pin.Bytes != item.Bytes || pin.SHA256 != item.SHA256 {
			t.Fatal("original artifact metadata disagrees with committed evidence", item.Path)
		}
		copyPinned(relative, filepath.Join(data, filepath.FromSlash(item.Path)))
	}
	code := filepath.Join(dir, "code")
	var candidateFiles []workbenchRetainedFile
	for _, archive := range old.SourceArchives {
		raw := copyPinned(archive.Archive, filepath.Join(dir, archive.Archive))
		if archive.Archive != "candidate-source.tar" {
			continue
		}
		if archive.Commit != "c8404be0005fb0e571079396d844f80fbb6485b7" || archive.SHA256 != fmt.Sprintf("%x", sha256.Sum256(raw)) {
			t.Fatal("candidate archive provenance differs from the actual prior candidate")
		}
		candidateFiles = archive.Files
		workbenchRetainedExtract(t, raw, code, candidateFiles)
	}
	if len(candidateFiles) != 6 {
		t.Fatal("missing exact prior candidate source inventory")
	}
	t.Setenv("SUMA_WORKSPACE_ROOT", code)
	db, err := store.Open("sqlite", database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := &Service{DB: db, Dialect: "sqlite", DataDir: data}
	if err = s.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	e, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, s: s, e: e}
	server, origin, port := modelSelectionGUIServer(t, f, web, "workbench126.localhost")
	t.Cleanup(server.Close)
	client := &http.Client{Timeout: 15 * time.Second}
	call := func(method, path string, body any, target any) {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequest(method, server.URL+"/api/v1"+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		request.Host = strings.TrimPrefix(origin, "http://")
		request.Header.Set("Origin", origin)
		request.Header.Set("Content-Type", "application/json")
		if f.cookie != nil {
			request.AddCookie(f.cookie)
		}
		if f.user.ID != "" && method != http.MethodGet {
			request.Header.Set("X-Suma-Expected-User", f.user.ID)
			request.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		payload, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("retained HTTP %s %s => %d: %s (%v)", method, path, response.StatusCode, payload, err)
		}
		for _, cookie := range response.Cookies() {
			if cookie.Name == "suma_session" {
				f.cookie = cookie
			}
		}
		if target != nil {
			if err = json.Unmarshal(payload, target); err != nil {
				t.Fatal(err)
			}
		}
	}
	var login struct {
		User Principal `json:"user"`
	}
	call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "test-password-strong-42"}, &login)
	f.user, f.view = login.User, login.User
	if f.user.ID == "" || f.cookie == nil {
		t.Fatal("retained actual account did not authenticate")
	}
	if err = db.QueryRow(`SELECT project_id FROM suma_tasks ORDER BY id LIMIT 1`).Scan(&f.project); err != nil {
		t.Fatal(err)
	}
	call("POST", "/workspace/binding", f.body(map[string]any{"path": code}), &WorkspaceMetadata{})
	seed, seedSHA := workbenchRetainedProtected(t, f)
	state := map[string]any{"status": "ready", "kind": "actual_retained_execution_readback", "origin": origin, "stage": "T-WORKBENCH-126", "source_database_sha256": old.DatabaseSHA256, "source_provenance": provenance, "candidate_commit_provenance_only": "c8404be0005fb0e571079396d844f80fbb6485b7", "source_files": candidateFiles, "seed_counts": seed, "seed_protected_sha256": seedSHA, "new_executions": 0, "model_calls": 0, "customer_human_signoff": false, "stop_reason": "timeout"}
	t.Cleanup(func() {
		server.Close()
		final, finalSHA := workbenchRetainedProtected(t, f)
		state["final_counts"], state["final_protected_sha256"] = final, finalSHA
		if !reflect.DeepEqual(seed, final) || seedSHA != finalSHA {
			state["status"] = "read_only_guard_failed"
			t.Error("workbench inspection changed original execution, task, test or acceptance facts")
		}
		for _, item := range candidateFiles {
			workbenchRetainedRead(t, code, item.Path, item)
		}
		for _, pin := range provenance {
			relative := strings.TrimPrefix(pin.Path, ".cache/e2e/rollout-125/actual-ui/")
			workbenchRetainedRead(t, source, relative, pin)
		}
		retained := filepath.Join(dir, "retained-fixture.sqlite")
		if _, err := db.Exec("VACUUM INTO '" + strings.ReplaceAll(retained, "'", "''") + "'"); err != nil {
			t.Error(err)
		} else if raw, err := os.ReadFile(retained); err != nil {
			t.Error(err)
		} else {
			state["database_bytes"], state["database_sha256"] = len(raw), fmt.Sprintf("%x", sha256.Sum256(raw))
		}
		modelSelectionGUIJSON(t, filepath.Join(dir, "result.public.json"), state)
	})
	var page WorkspaceTaskPage
	call("GET", "/workspace/task-context/tasks?project_id="+url.QueryEscape(f.project)+"&limit=20", nil, &page)
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatal("actual retained task picker lost its two source tasks")
	}
	contexts := []WorkspaceTaskContext{}
	for _, item := range page.Items {
		var current WorkspaceTaskContext
		call("GET", "/workspace/task-context?project_id="+url.QueryEscape(f.project)+"&task_id="+url.QueryEscape(item.ID), nil, &current)
		if current.Task.ID != item.ID || current.Task.Version != item.Version || current.Task.Status != "accepted" || current.Requirement == nil || current.Delivery.Run == nil || current.Delivery.Run.RunID != current.Task.RunID || !current.Delivery.FormalAcceptance || current.Quality.PlanID == "" || current.Quality.State != "passed" || !current.Quality.BasisCurrent {
			t.Fatal("current workbench context lost original accepted task/run/plan facts", item.ID, current)
		}
		contexts = append(contexts, current)
	}
	var file struct {
		Content string `json:"content"`
		Hash    string `json:"hash"`
	}
	call("GET", "/workspace/file?project_id="+url.QueryEscape(f.project)+"&path=sum.cjs", nil, &file)
	for _, item := range candidateFiles {
		if item.Path == "sum.cjs" && (file.Hash != item.SHA256 || fmt.Sprintf("%x", sha256.Sum256([]byte(file.Content))) != item.SHA256) {
			t.Fatal("IDE file is not the retained candidate bytes")
		}
	}
	state["contexts"] = contexts
	modelSelectionGUIJSON(t, filepath.Join(dir, "web.files.public.json"), modelSelectionGUIWebFiles(t, web))
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.local.json"), map[string]any{"kind": state["kind"], "allowed_actions": []string{"read", "refresh", "local_unsaved_draft"}, "origin": origin, "base_url": server.URL, "email": "owner@example.test", "password": "test-password-strong-42", "project_id": f.project, "user_id": f.user.ID, "tasks": page.Items})
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.ready"), map[string]any{"status": "ready", "kind": state["kind"], "origin": origin, "port": port, "project_id": f.project, "tasks": page.Items, "new_executions": 0, "model_calls": 0})
	t.Logf("actual retained history workbench ready at %s; no new execution; read/refresh only", origin)
	deadline, stop := time.NewTimer(5*time.Minute), time.NewTicker(200*time.Millisecond)
	defer deadline.Stop()
	defer stop.Stop()
	for waiting := true; waiting; {
		select {
		case <-deadline.C:
			waiting = false
		case <-stop.C:
			if _, err = os.Lstat(filepath.Join(dir, "stop.request")); err == nil {
				state["stop_reason"] = "explicit_stop"
				waiting = false
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
	}
	if state["stop_reason"] != "explicit_stop" {
		t.Fatal("workbench GUI verification must close explicitly before timeout")
	}
	state["status"] = "closed_without_new_execution"
}

func workbenchRetainedDirectory(t *testing.T, path, owned string, fresh bool) string {
	t.Helper()
	if path == "" {
		t.Fatal("explicit workbench evidence directories are required")
	}
	path, err := filepath.Abs(path)
	if err != nil || path == owned || !under(owned, path) {
		t.Fatal("workbench directory escapes the exact owned cache")
	}
	if fresh {
		if _, err = os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("workbench evidence directory must be entirely fresh")
		}
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		t.Fatal("workbench directories must be ordinary paths without links", path, err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Fatal("workbench directory is unavailable", path, err)
	}
	return path
}

func workbenchRetainedJSON(t *testing.T, path string, target any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, target); err != nil {
		t.Fatal(err)
	}
}

func workbenchRetainedRead(t *testing.T, root, relative string, pin workbenchRetainedFile) []byte {
	t.Helper()
	if !fsValidRetainedPath(relative) {
		t.Fatal("unsafe retained evidence path", relative)
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path || !under(root, path) {
		t.Fatal("retained evidence must stay in its ordinary owned path", relative, err)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != int64(pin.Bytes) {
		t.Fatal("retained evidence byte count/type changed", relative, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(raw)) != pin.SHA256 {
		t.Fatal("retained evidence digest changed", relative, err)
	}
	return raw
}

func fsValidRetainedPath(path string) bool {
	return path != "" && !strings.ContainsAny(path, "\\:\x00") && !filepath.IsAbs(path) && filepath.ToSlash(filepath.Clean(path)) == path && path != ".." && !strings.HasPrefix(path, "../")
}

func workbenchRetainedExtract(t *testing.T, raw []byte, code string, manifest []workbenchRetainedFile) {
	t.Helper()
	allow := map[string]workbenchRetainedFile{}
	for _, item := range manifest {
		if !fsValidRetainedPath(item.Path) || item.Bytes < 0 || item.Bytes > 1<<20 {
			t.Fatal("invalid candidate source inventory")
		}
		allow[item.Path] = item
	}
	if len(allow) != len(manifest) {
		t.Fatal("duplicate candidate source inventory")
	}
	seen := map[string]bool{}
	reader := tar.NewReader(bytes.NewReader(raw))
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeDir || header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		pin, ok := allow[header.Name]
		if !ok || seen[header.Name] || header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA || header.Size != int64(pin.Bytes) {
			t.Fatal("unlisted, duplicate or special candidate source entry", header.Name)
		}
		content, err := io.ReadAll(io.LimitReader(reader, 1<<20+1))
		if err != nil || len(content) != pin.Bytes || fmt.Sprintf("%x", sha256.Sum256(content)) != pin.SHA256 {
			t.Fatal("candidate source entry differs from original archive manifest", header.Name)
		}
		path := filepath.Join(code, filepath.FromSlash(header.Name))
		if !under(code, path) {
			t.Fatal("candidate source escapes isolated directory")
		}
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
		seen[header.Name] = true
	}
	if len(seen) != len(allow) {
		t.Fatal("incomplete retained candidate source archive")
	}
}

// Compare complete rows, not only counts: a mutation of an original task,
// attempt, test report or approval must fail the same read-only guard.
func workbenchRetainedProtected(t *testing.T, f *fixture) (map[string]int, string) {
	t.Helper()
	tables := []string{"suma_tasks", "suma_requirements", "suma_reviews", "suma_test_plans", "suma_test_plan_revisions", "suma_test_plan_runs", "suma_test_plan_reports", "suma_evidence", "suma_task_run_provenance", "suma_task_code_baselines", "dag_runs", "dag_nodes", "dag_attempts", "dag_events", "suma_usage_attempts", "suma_usage_facts", "suma_candidate_compositions", "suma_candidate_composition_executions", "suma_project_applications", "suma_application_versions", "suma_application_release_sets", "suma_application_release_set_plan_groups", "suma_application_release_set_plan_bindings", "suma_application_release_rollouts", "suma_application_release_rollout_members", "suma_release_candidates", "suma_release_artifacts", "suma_release_plans", "suma_release_approvals", "suma_release_deployments", "suma_repository_plans", "suma_repository_operations", "suma_workspace_file_operations", "suma_workspace_writes"}
	counts, facts := map[string]int{}, map[string][]string{}
	for _, table := range tables {
		rows, err := f.s.DB.Query(`SELECT * FROM "` + table + `"`)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatal(err)
		}
		items := []string{}
		for rows.Next() {
			values, destinations := make([]any, len(columns)), make([]any, len(columns))
			for i := range values {
				destinations[i] = &values[i]
			}
			if err = rows.Scan(destinations...); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			items = append(items, stringify(values))
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(items)
		counts[table], facts[table] = len(items), items
	}
	return counts, fmt.Sprintf("%x", sha256.Sum256([]byte(stringify(facts))))
}
