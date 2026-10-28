package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixture struct {
	t       *testing.T
	s       *Service
	e       *dag.Engine
	handler http.Handler
	cookie  *http.Cookie
	project string
	user    Principal
	actor   Actor     // Actual original browser credential, retained for late-authority tests.
	view    Principal // Last session actually loaded by this browser fixture.
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "suma.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := &Service{DB: db, Dialect: "sqlite", DataDir: t.TempDir()}
	if err = s.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	e, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, s: s, e: e}
	f.handler = (&Server{Service: s, Engine: e, WebDir: t.TempDir()}).Handler()
	setup := f.call("POST", "/auth/setup", map[string]any{"name": "开发负责人", "email": "owner@example.test", "password": "test-password-strong-42", "organization": "验证团队", "project": "验证项目"}, 200)
	raw, _ := json.Marshal(setup["user"])
	json.Unmarshal(raw, &f.user)
	f.actor, err = s.AuthenticateActor(context.Background(), f.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	projects := setup["projects"].([]any)
	f.project = projects[0].(map[string]any)["id"].(string)
	return f
}
func (f *fixture) call(method, path string, body any, want int) map[string]any {
	f.t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, "http://suma.test/api/v1"+path, bytes.NewReader(raw))
	if f.cookie != nil {
		req.AddCookie(f.cookie)
	}
	req.Header.Set("Content-Type", "application/json")
	if f.view.ID != "" && BrowserCommandRequiresView(method, strings.TrimPrefix(path, "/")) && !strings.HasPrefix(path, "/auth/setup") && !strings.HasPrefix(path, "/auth/login") && !strings.HasPrefix(path, "/auth/invitations/accept") {
		req.Header.Set("X-Suma-Expected-User", f.view.ID)
		if path != "/account/preferences" {
			req.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
		}
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	if w.Code != want {
		f.t.Fatalf("%s %s => %d, want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "suma_session" {
			f.cookie = c
		}
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		f.t.Fatalf("invalid JSON: %s", w.Body.String())
	}
	if user, ok := result["user"].(map[string]any); ok {
		encoded, _ := json.Marshal(user)
		if err := json.Unmarshal(encoded, &f.view); err != nil {
			f.t.Fatal(err)
		}
	}
	return result
}
func (f *fixture) body(m map[string]any) map[string]any {
	m["project_id"] = f.project
	if _, ok := m["idempotency_key"]; !ok {
		m["idempotency_key"] = newID()
	}
	return m
}
func TestAuthenticationAndTenantIsolation(t *testing.T) {
	f := newFixture(t)
	f.call("GET", "/auth/session", nil, 200)
	f.call("POST", "/auth/setup", map[string]any{"name": "x", "email": "x@example.test", "password": "another-strong-pass"}, 409)
	f.call("GET", "/tasks?project_id=other-tenant", nil, 404)
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	f.call("GET", "/auth/session", nil, 401)
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "wrong"}, 401)
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "test-password-strong-42"}, 200)
}
func TestReceiptsDecompositionAndManualGate(t *testing.T) {
	f := newFixture(t)
	request := f.body(map[string]any{"title": "订单需求", "acceptance_criteria": []string{"权限隔离", "测试通过"}})
	req := f.call("POST", "/requirements", request, 201)
	replay := f.call("POST", "/requirements", request, 201)
	if replay["id"] != req["id"] {
		t.Fatal("idempotent create duplicated")
	}
	request["title"] = "不同需求"
	f.call("POST", "/requirements", request, 409)
	draft := f.body(map[string]any{"expected_version": 1, "drafts": []any{map[string]any{"key": "api", "title": "接口"}, map[string]any{"key": "ui", "title": "界面", "depends_on": []string{"api"}}}})
	out := f.call("POST", "/requirements/"+req["id"].(string)+"/decompose", draft, 200)
	items := out["items"].([]any)
	if len(items) != 2 {
		t.Fatal("wrong task count")
	}
	f.call("POST", "/tasks", f.body(map[string]any{"title": "绕过验收", "status": "accepted"}), 400)
	task := items[0].(map[string]any)
	f.call("PATCH", "/tasks/"+task["id"].(string), f.body(map[string]any{"expected_version": 99, "title": "冲突"}), 409)
	cycle := f.body(map[string]any{"expected_version": 1, "drafts": []any{map[string]any{"key": "a", "title": "A", "depends_on": []string{"b"}}, map[string]any{"key": "b", "title": "B", "depends_on": []string{"a"}}}})
	f.call("POST", "/requirements/"+req["id"].(string)+"/decompose", cycle, 400)
	tasks := f.call("GET", "/tasks?project_id="+f.project, nil, 200)
	if tasks["total"] != float64(2) {
		t.Fatal("invalid plan partially committed")
	}
}
func TestRunnerToReviewAndIssueIntake(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "经过测试的功能"}), 201)
	workflow := f.call("POST", "/workflows", f.body(map[string]any{"name": "验证流程", "nodes": []any{map[string]any{"id": "test", "name": "运行测试", "type": "test", "command": []string{"fixture-go-test"}, "idempotent": true}}, "edges": []any{}}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow["id"], "task_id": task["id"]}), 201)
	a, err := f.e.Claim(ctx, f.user.TenantID, f.project, "test-worker", toolFixtureCapabilities(t, f))
	if err != nil || a == nil {
		t.Fatalf("claim: %v %v", a, err)
	}
	if err = f.e.Complete(ctx, f.user.TenantID, f.project, dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: 0, Output: "test evidence", OperationID: newID()}); err != nil {
		t.Fatal(err)
	}
	current := f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
	accepted := f.call("POST", "/tasks/"+task["id"].(string)+"/review", f.body(map[string]any{"expected_version": current["version"], "run_id": run["id"], "decision": "accepted", "reason": "已核对固定测试节点证据"}), 200)
	if accepted["status"] != "accepted" {
		t.Fatal("review not applied")
	}
	changed := f.call("PATCH", "/tasks/"+task["id"].(string), f.body(map[string]any{"expected_version": accepted["version"], "description": "改变交付范围"}), 200)
	if changed["status"] == "accepted" {
		t.Fatal("changed task kept acceptance")
	}
	f.call("POST", "/tasks/"+task["id"].(string)+"/review", f.body(map[string]any{"expected_version": changed["version"], "run_id": run["id"], "decision": "accepted", "reason": "尝试复用旧证据"}), 409)
	issue := f.call("POST", "/issues", f.body(map[string]any{"title": "回归失败", "description": "来自真实运行证据", "status": "open"}), 201)
	path := "/issues/" + issue["id"].(string) + "/intake"
	first := f.call("POST", path, f.body(map[string]any{}), 201)
	second := f.call("POST", path, f.body(map[string]any{}), 201)
	if first["id"] != second["id"] {
		t.Fatal("issue intake duplicated requirement")
	}
}
func TestWorkspaceScopeHashAndNoMutationOnRead(t *testing.T) {
	f := newFixture(t)
	root := t.TempDir()
	t.Setenv("SUMA_WORKSPACE_ROOT", root)
	if err := os.WriteFile(filepath.Join(root, "main.ts"), []byte("const x = 1;"), 0600); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/workspace/binding", f.body(map[string]any{"path": root}), 200)
	file := f.call("GET", "/workspace/file?project_id="+f.project+"&path=main.ts", nil, 200)
	f.call("POST", "/workspace/file", f.body(map[string]any{"path": "main.ts", "expected_hash": strings.Repeat("0", 64), "expected_context_sha256": file["context_sha256"], "content": "bad"}), 409)
	saved := f.call("POST", "/workspace/file", f.body(map[string]any{"path": "main.ts", "expected_hash": file["hash"], "expected_context_sha256": file["context_sha256"], "content": "const x = 2;"}), 200)
	if saved["content"] != "const x = 2;" {
		t.Fatal("save failed")
	}
	f.call("GET", "/workspace/file?project_id="+f.project+"&path=../outside", nil, 404)
	os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=value"), 0600)
	f.call("GET", "/workspace/file?project_id="+f.project+"&path=.env", nil, 403)
	var before, after int
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_activity`).Scan(&before)
	for i := 0; i < 2; i++ {
		f.call("GET", fmt.Sprintf("/overview?project_id=%s", f.project), nil, 200)
	}
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_activity`).Scan(&after)
	if before != after {
		t.Fatal("read mutated activity")
	}
}

func TestRunStartReplaysOriginalVersionAndChecksDependencies(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := f.call("POST", "/tasks", f.body(map[string]any{"title": "前置"}), 201)
	wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "依赖验证", "nodes": []any{map[string]any{"id": "test", "name": "测试", "type": "test", "command": []string{"fixture-go-version"}}}}), 201)
	preRun := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": wf["id"], "task_id": a["id"]}), 201)
	preAssignment, err := f.e.Claim(ctx, f.user.TenantID, f.project, "pre-worker", toolFixtureCapabilities(t, f))
	if err != nil || preAssignment == nil {
		t.Fatalf("pre claim %v", err)
	}
	if err = f.e.Complete(ctx, f.user.TenantID, f.project, dag.Result{RunID: preAssignment.RunID, NodeID: preAssignment.NodeID, WorkerID: preAssignment.WorkerID, Attempt: preAssignment.Attempt, Fence: preAssignment.Fence, OperationID: newID()}); err != nil {
		t.Fatal(err)
	}
	a = f.call("GET", "/tasks/"+a["id"].(string)+"?project_id="+f.project, nil, 200)
	a = f.call("POST", "/tasks/"+a["id"].(string)+"/review", f.body(map[string]any{"expected_version": a["version"], "run_id": preRun["id"], "decision": "accepted", "reason": "前置测试真实通过"}), 200)
	b := f.call("POST", "/tasks", f.body(map[string]any{"title": "后继", "depends_on": []string{a["id"].(string)}}), 201)
	start := f.body(map[string]any{"workflow_id": wf["id"], "workflow_version": wf["version"], "task_id": b["id"], "expected_version": 1})
	run := f.call("POST", "/runs", start, 201)
	again := f.call("POST", "/runs", start, 201)
	if run["id"] != again["id"] {
		t.Fatal("run start replay duplicated")
	}
	f.call("PATCH", "/tasks/"+a["id"].(string), f.body(map[string]any{"expected_version": a["version"], "title": "前置已变更"}), 200)
	assignment, err := f.e.Claim(ctx, f.user.TenantID, f.project, "fixture-worker", toolFixtureCapabilities(t, f))
	if err != nil || assignment == nil {
		t.Fatalf("claim %v", err)
	}
	err = f.e.Complete(ctx, f.user.TenantID, f.project, dag.Result{RunID: assignment.RunID, NodeID: assignment.NodeID, WorkerID: assignment.WorkerID, Attempt: assignment.Attempt, Fence: assignment.Fence, OperationID: newID(), ExitCode: 0})
	if err != nil {
		t.Fatal(err)
	}
	task := f.call("GET", "/tasks/"+b["id"].(string)+"?project_id="+f.project, nil, 200)
	f.call("POST", "/tasks/"+b["id"].(string)+"/review", f.body(map[string]any{"expected_version": task["version"], "run_id": run["id"], "decision": "accepted", "reason": "检查依赖失效"}), 409)
}
