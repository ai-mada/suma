package platform

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// This opt-in hold serves the already completed actual mitigation journey.
// Browser actions are read/refresh only. Unique owned database and committed
// source archives are retained before any test TempDir is cleaned.
func projectRiskRuntimeHold(t *testing.T, f *fixture, risk string, evidence actualFixedCandidateEvidence, receipt map[string]any) {
	t.Helper()
	dir, web := os.Getenv("SUMA_PROJECT_RISK_UI_DIR"), os.Getenv("SUMA_PROJECT_RISK_WEB_DIR")
	if dir == "" && web == "" {
		return
	}
	if dir == "" || web == "" {
		t.Fatal("both owned project-risk GUI directories are required")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".cache", "e2e", "project-risk-121"))
	if err != nil {
		t.Fatal(err)
	}
	dir, err = filepath.Abs(dir)
	if err != nil || dir == root || !under(root, dir) {
		t.Fatal("project-risk private receipts must be inside the exact owned cache")
	}
	web, err = filepath.Abs(web)
	if err != nil || web == root || !under(root, web) || under(web, dir) || under(dir, web) {
		t.Fatal("project-risk web assets and private receipts must be separate owned directories")
	}
	if _, err = os.Stat(filepath.Join(web, "index.html")); err != nil {
		t.Fatal("the final project-risk web build must exist", err)
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, web} {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || !under(resolvedRoot, resolved) {
			t.Fatal("project-risk GUI paths must resolve inside their exact owned cache")
		}
	}
	for _, name := range []string{"login.local.json", "login.ready", "stop.request", "result.public.json", "fixture.sqlite", "base-source.tar", "candidate-source.tar", "source.manifest.public.json"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatal("project-risk GUI receipt directory must be fresh", name)
		}
	}
	server, origin, port := modelSelectionGUIServer(t, f, web, "risk121.localhost")
	t.Cleanup(server.Close)
	counts := func() map[string]int {
		out := modelSelectionGUICounts(t, f)
		for _, name := range []string{"suma_tasks", "suma_requirements", "suma_planning_analyses", "suma_planning_alerts", "suma_test_plan_reports", "dag_events", "suma_risks", "suma_risk_sources", "suma_risk_reviews"} {
			out[name] = expertCount(t, f, name)
		}
		return out
	}
	seed := counts()
	seedRiskSHA := projectRiskProtectedSnapshot(t, f)
	state := map[string]any{"status": "ready", "kind": "actual_os_http_project_risk", "origin": origin, "receipt": receipt, "seed_counts": seed, "seed_risk_sha256": seedRiskSHA, "model_calls": 0, "actual_node_processes": 2, "customer_human_signoff": false}
	t.Cleanup(func() {
		server.Close()
		final := counts()
		state["final_counts"] = final
		finalRiskSHA := projectRiskProtectedSnapshot(t, f)
		state["final_risk_sha256"] = finalRiskSHA
		if !reflect.DeepEqual(seed, final) || seedRiskSHA != finalRiskSHA {
			state["status"] = "read_only_guard_failed"
			t.Error("read-only risk inspection changed formal business, execution or review facts")
		}
		// Both archives are produced from exact committed IDs by mature Git.
		// Worktree caches, local credential files and process state are omitted.
		archives := []map[string]any{}
		for _, source := range []struct{ name, revision string }{{"base-source.tar", evidence.BaseCommit}, {"candidate-source.tar", evidence.CandidateCommit}} {
			path := filepath.Join(dir, source.name)
			workspaceGitTestCommand(t, evidence.Repository, "archive", "--format=tar", "--output="+path, source.revision)
			metadata := projectRiskSourceArchive(t, path)
			metadata["revision"] = source.revision
			archives = append(archives, metadata)
		}
		modelSelectionGUIJSON(t, filepath.Join(dir, "source.manifest.public.json"), map[string]any{"kind": "owned_committed_source_archives", "archives": archives})
		state["source_archives"] = archives
		database := filepath.Join(dir, "fixture.sqlite")
		if _, err := f.s.DB.Exec("VACUUM INTO '" + strings.ReplaceAll(database, "'", "''") + "'"); err != nil {
			t.Error("retain owned risk fixture database", err)
		} else if raw, err := os.ReadFile(database); err != nil {
			t.Error("hash retained risk fixture database", err)
		} else {
			state["database_sha256"] = fmt.Sprintf("%x", sha256.Sum256(raw))
			state["database_bytes"] = len(raw)
		}
		modelSelectionGUIJSON(t, filepath.Join(dir, "result.public.json"), state)
	})
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.local.json"), map[string]any{"kind": "actual_os_http_project_risk", "allowed_actions": []string{"read", "refresh"}, "origin": origin, "base_url": server.URL, "email": "owner@example.test", "password": "test-password-strong-42", "project_id": f.project, "user_id": f.user.ID, "risk_id": risk, "receipt": receipt})
	modelSelectionGUIJSON(t, filepath.Join(dir, "web.files.public.json"), modelSelectionGUIWebFiles(t, web))
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.ready"), map[string]any{"status": "ready", "kind": "actual_os_http_project_risk", "origin": origin, "port": port, "seed_counts": seed, "model_calls": 0, "actual_node_processes": 2})
	t.Logf("actual project-risk GUI ready: %s; read/refresh only; private fixture login stays in the owned cache", origin)
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	state["stop_reason"] = "timeout"
	for waiting := true; waiting; {
		select {
		case <-timer.C:
			waiting = false
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(dir, "stop.request")); err == nil {
				state["stop_reason"] = "explicit_stop"
				waiting = false
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
	}
	state["status"] = "closed_without_new_execution"
}

func projectRiskSourceArchive(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 4<<20 {
		t.Fatal("generated source archive exceeded its fixture bound")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader := tar.NewReader(file)
	files := []map[string]any{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeXGlobalHeader || header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA || header.Size < 0 || header.Size > 1<<20 || header.Name != "sum.cjs" && header.Name != ".gitignore" {
			t.Fatal("unexpected entry in owned source archive")
		}
		content, err := io.ReadAll(io.LimitReader(reader, 1<<20+1))
		if err != nil || len(content) > 1<<20 {
			t.Fatal("read generated source entry", err)
		}
		files = append(files, map[string]any{"path": header.Name, "bytes": len(content), "sha256": fmt.Sprintf("%x", sha256.Sum256(content))})
	}
	if len(files) != 2 {
		t.Fatal("owned source archive is incomplete")
	}
	return map[string]any{"archive": filepath.Base(path), "bytes": len(raw), "sha256": fmt.Sprintf("%x", sha256.Sum256(raw)), "files": files}
}

// Row counts alone cannot detect edits to an existing risk. Retain a digest of
// every protected risk/review/source row while keeping their bodies private.
func projectRiskProtectedSnapshot(t *testing.T, f *fixture) string {
	t.Helper()
	queries := []string{`SELECT id,version,state,cycle,assessment_revision,payload FROM suma_risks ORDER BY id`, `SELECT alert_id,risk_id FROM suma_risk_sources ORDER BY alert_id`, `SELECT id,cycle,decision,snapshot FROM suma_risk_reviews ORDER BY id`}
	facts := [][][]any{}
	for _, query := range queries {
		rows, err := f.s.DB.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		items := [][]any{}
		for rows.Next() {
			values := make([]any, len(columns))
			dest := make([]any, len(columns))
			for index := range values {
				dest[index] = &values[index]
			}
			if err = rows.Scan(dest...); err != nil {
				_ = rows.Close()
				t.Fatal(err)
			}
			items = append(items, values)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		facts = append(facts, items)
	}
	raw, err := json.Marshal(facts)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
