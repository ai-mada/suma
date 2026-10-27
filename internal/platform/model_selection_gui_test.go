package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Explicitly opted-in browser hold. Both catalogs and the seeded source facts
// are protocol fixtures; no Runner, installed CLI or model endpoint is started.
func TestModelSelectionIsolatedGUIProtocolFixtures(t *testing.T) {
	dir, web := os.Getenv("SUMA_MODEL_SELECTION_UI_DIR"), os.Getenv("SUMA_MODEL_SELECTION_WEB_DIR")
	if dir == "" || web == "" {
		t.Skip("explicit model-selection GUI directory and final web directory are required")
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(repository, ".cache", "e2e", "expert-codex-102")
	dir, err = filepath.Abs(dir)
	if err != nil || !under(owned, dir) {
		t.Fatal("GUI evidence must stay in the owned model-selection cache")
	}
	web, err = filepath.Abs(web)
	if err != nil || !under(owned, web) {
		t.Fatal("GUI web assets must stay in the owned model-selection cache")
	}
	if _, err = os.Stat(filepath.Join(web, "index.html")); err != nil {
		t.Fatal("final GUI web entry must exist before fixture initialization")
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	resolvedOwned, err := filepath.EvalSymlinks(owned)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, web} {
		resolved, e := filepath.EvalSymlinks(path)
		if e != nil || !under(resolvedOwned, resolved) {
			t.Fatal("GUI paths must resolve inside the exact owned cache")
		}
	}
	for _, name := range []string{"login.local.json", "login.ready", "stop.request", "result.public.json"} {
		if _, e := os.Stat(filepath.Join(dir, name)); e == nil || !os.IsNotExist(e) {
			t.Fatal("GUI evidence directory must be fresh; an earlier receipt or stop signal exists")
		}
	}

	expert, codexProfile, catalog := expertCodexFixture(t, false)
	repair, original, repairRequest, nativeProfile, repairToken, deployments := repairModelFixture(t)
	ctx := context.Background()
	for i, task := range expert.tasks {
		expert.tasks[i], err = expert.f.s.Update(ctx, expert.f.actor, expert.f.project, "tasks", task.ID, expert.f.body(map[string]any{"expected_version": task.Version, "title": fmt.Sprintf("协议夹具：专家 Codex 预览任务 %d", i+1), "description": "授权工具目录协议夹具；仅预览与未开始项修订，不运行 CLI 或模型。"}))
		if err != nil {
			t.Fatal(err)
		}
	}
	repairRequest.Repair.Title = "协议夹具：原生模型冲突修复预览"
	repairRequest.Repair.Description = "类型化冲突与模型目录协议夹具；仅创建预览、未开始项修订和刷新，不启动运行或模型。"

	expertServer, expertOrigin, expertPort := modelSelectionGUIServer(t, expert.f, web, "expert102.localhost")
	repairServer, repairOrigin, repairPort := modelSelectionGUIServer(t, repair.f, web, "repair102.localhost")
	t.Cleanup(expertServer.Close)
	t.Cleanup(repairServer.Close)
	seedExpert, seedRepair := modelSelectionGUICounts(t, expert.f), modelSelectionGUICounts(t, repair.f)
	state := map[string]any{"status": "ready", "kind": "protocol_fixtures", "model_calls": 0, "actual_cli_calls": 0, "runner_processes": 0, "seed_expert": seedExpert, "seed_repair": seedRepair, "expert_origin": expertOrigin, "repair_origin": repairOrigin}
	t.Cleanup(func() {
		// Public service projections and schema/count digests are retained before
		// the fixtures' independent temporary databases and data directories close.
		modelSelectionGUIExport(t, dir, "expert", expert.f)
		modelSelectionGUIExport(t, dir, "repair", repair.f)
		state["final_expert"] = modelSelectionGUICounts(t, expert.f)
		state["final_repair"] = modelSelectionGUICounts(t, repair.f)
		modelSelectionGUIJSON(t, filepath.Join(dir, "result.public.json"), state)
	})
	modelSelectionGUIJSON(t, filepath.Join(dir, "web.files.public.json"), modelSelectionGUIWebFiles(t, web))
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.local.json"), map[string]any{
		"kind": "protocol_fixtures", "allowed_actions": []string{"create_preview", "revise_unstarted", "refresh"},
		"expert": map[string]any{"title": "专家 Codex 选择（协议夹具）", "origin": expertOrigin, "base_url": expertServer.URL, "email": "owner@example.test", "password": "test-password-strong-42", "project_id": expert.f.project, "user_id": expert.f.user.ID, "team_id": expert.team, "configuration_version": expert.configuration.Version, "tasks": expert.tasks, "workflow_id": expert.role.WorkflowID, "workflow_version": expert.role.WorkflowVersion, "tool_profile_id": codexProfile.ID, "tool_profile_version": codexProfile.Version, "catalog": catalog, "create_request": expertCodexRequest(expert, catalog)},
		"repair": map[string]any{"title": "原生模型冲突修复（协议夹具）", "origin": repairOrigin, "base_url": repairServer.URL, "email": "owner@example.test", "password": "test-password-strong-42", "project_id": repair.f.project, "user_id": repair.f.user.ID, "composition_id": original.ID, "composition_version": original.Version, "task_id": original.TaskID, "conflict_sha256": repairRequest.ExpectedConflictSHA256, "workflow_id": repairRequest.WorkflowID, "workflow_version": repairRequest.WorkflowVersion, "producer_node_id": repairRequest.ProducerNodeID, "tool_profile_id": nativeProfile.ID, "tool_profile_version": nativeProfile.Version, "deployments": deployments, "create_request": repairRequest},
	})
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.ready"), map[string]any{"status": "ready", "kind": "protocol_fixtures", "expert": map[string]any{"origin": expertOrigin, "port": expertPort, "seed_runs": seedExpert["dag_runs"]}, "repair": map[string]any{"origin": repairOrigin, "port": repairPort, "seed_runs": seedRepair["dag_runs"]}, "model_calls": 0, "actual_cli_calls": 0})
	t.Logf("protocol GUI fixtures ready: expert=%s repair=%s; private login metadata is retained only in the owned cache", expertOrigin, repairOrigin)
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()
	refresh := time.NewTicker(30 * time.Second)
	defer refresh.Stop()
	stop := time.NewTicker(200 * time.Millisecond)
	defer stop.Stop()
	state["stop_reason"] = "timeout"
	waiting := true
	for waiting {
		select {
		case <-deadline.C:
			waiting = false
		case <-stop.C:
			if _, e := os.Stat(filepath.Join(dir, "stop.request")); e == nil {
				state["stop_reason"] = "explicit_stop"
				waiting = false
			} else if !os.IsNotExist(e) {
				t.Fatal(e)
			}
		case <-refresh.C:
			reportCodexCatalog(t, expert.f, expert.runnerToken, codexProfile, catalog)
			reportRepairModelInventory(t, repair, nativeProfile, repairToken, deployments)
		}
	}
	for label, entry := range map[string]struct {
		fixture *fixture
		seed    map[string]int
	}{"expert": {expert.f, seedExpert}, "repair": {repair.f, seedRepair}} {
		current := modelSelectionGUICounts(t, entry.fixture)
		for _, table := range []string{"dag_runs", "dag_attempts", "suma_reviews", "suma_usage_attempts", "suma_candidate_composition_executions"} {
			if current[table] != entry.seed[table] {
				state["status"] = "execution_guard_failed"
				t.Fatalf("%s GUI changed seeded execution/review count for %s", label, table)
			}
		}
	}
	state["status"] = "closed_without_new_execution"
	t.Log("protocol GUI fixtures closed; seeded runs/attempts/reviews/accounting are unchanged; no CLI or model process was started")
}

func modelSelectionGUIServer(t *testing.T, f *fixture, web, hostname string) (*httptest.Server, string, int) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	origin := fmt.Sprintf("http://%s:%d", hostname, port)
	server := httptest.NewUnstartedServer(nil)
	if err = server.Listener.Close(); err != nil {
		t.Fatal(err)
	}
	server.Listener = listener
	config := &Server{Service: f.s, Engine: f.e, WebDir: web, PublicURL: origin, PublicURLSource: "configured", AllowedOrigins: []string{origin, fmt.Sprintf("http://127.0.0.1:%d", port)}}
	f.handler = config.Handler()
	server.Config.Handler = f.handler
	server.Start()
	return server, origin, port
}

func modelSelectionGUIJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func modelSelectionGUICounts(t *testing.T, f *fixture) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, table := range []string{"dag_runs", "dag_attempts", "suma_reviews", "suma_usage_attempts", "suma_candidate_composition_executions", "suma_expert_team_plans", "suma_candidate_compositions"} {
		out[table] = expertCount(t, f, table)
	}
	return out
}

func modelSelectionGUIExport(t *testing.T, dir, label string, f *fixture) {
	t.Helper()
	type schemaRow struct {
		Name   string `json:"name"`
		SHA256 string `json:"schema_sha256"`
		Rows   int    `json:"rows"`
	}
	schema := []schemaRow{}
	rows, err := f.s.DB.Query(`SELECT name,sql FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name, definition string
		if err = rows.Scan(&name, &definition); err != nil {
			break
		}
		hash := sha256.Sum256([]byte(definition))
		schema = append(schema, schemaRow{Name: name, SHA256: hex.EncodeToString(hash[:])})
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	for i := range schema {
		if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM "` + strings.ReplaceAll(schema[i].Name, `"`, `""`) + `"`).Scan(&schema[i].Rows); err != nil {
			t.Fatal(err)
		}
	}
	modelSelectionGUIJSON(t, filepath.Join(dir, label+".schema.public.json"), map[string]any{"tables": schema, "table_count": len(schema), "counts": modelSelectionGUICounts(t, f)})
	expertPlans := []ExpertTeamPlan{}
	for _, id := range modelSelectionGUIIDs(t, f, "suma_expert_team_plans") {
		plan, e := f.s.GetExpertTeamPlan(context.Background(), f.e, f.actor, f.project, id)
		if e != nil {
			t.Fatal(e)
		}
		expertPlans = append(expertPlans, plan)
	}
	repairPlans := []CandidateCompositionPlan{}
	for _, id := range modelSelectionGUIIDs(t, f, "suma_candidate_compositions") {
		plan, e := f.s.GetCandidateComposition(context.Background(), f.e, f.actor, f.project, id)
		if e != nil {
			t.Fatal(e)
		}
		if plan.ConflictBasis != nil && plan.ModelExecution != nil {
			repairPlans = append(repairPlans, plan)
		}
	}
	modelSelectionGUIJSON(t, filepath.Join(dir, label+".plans.public.json"), map[string]any{"expert_plans": expertPlans, "native_repair_plans": repairPlans})
}

func modelSelectionGUIIDs(t *testing.T, f *fixture, table string) []string {
	t.Helper()
	rows, err := f.s.DB.Query(`SELECT id FROM `+table+` WHERE tenant_id=? AND project_id=? ORDER BY id`, f.user.TenantID, f.project)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func modelSelectionGUIWebFiles(t *testing.T, root string) []map[string]any {
	t.Helper()
	files := []map[string]any{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("GUI web assets must be ordinary local files")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(raw)
		files = append(files, map[string]any{"path": filepath.ToSlash(relative), "sha256": hex.EncodeToString(hash[:]), "bytes": len(raw)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
