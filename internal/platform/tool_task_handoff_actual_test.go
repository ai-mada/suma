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
)

// This opt-in journey verifies the production selection and preparation UI
// against real HTTP/SQLite. Its writable profile is owned inventory metadata,
// not an executable probe or vendor/model result. No plan is started.
func TestToolTaskHandoffActualCurrentProfileToFixedPlan(t *testing.T) {
	if os.Getenv("SUMA_TOOL_TASK_HANDOFF_213_ACTUAL") != "1" {
		t.Skip("explicit isolated tool-to-task actual journey required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	node := os.Getenv("SUMA_TOOL_TASK_HANDOFF_213_NODE")
	driver := filepath.Join(root, "tests", "integration", "tool-task-handoff", "driver.cjs")
	web := filepath.Join(root, ".cache", "tool-task-handoff-213", "web", "out")
	for _, file := range []string{node, driver, filepath.Join(web, "index.html")} {
		if !filepath.IsAbs(file) {
			t.Fatal("prepared absolute Node, driver and UI assets required")
		}
		if info, e := os.Stat(file); e != nil || !info.Mode().IsRegular() {
			t.Fatal("prepared actual fixture assets unavailable")
		}
	}
	dir := filepath.Join(root, ".cache", "tool-task-handoff-213", "actual", strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000Z"), ".", ""))
	if err = os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal("each actual journey requires a new evidence directory")
	}
	// Reuse the original authenticated fixture and fixed engineering workflow.
	// This performs real owned control operations but starts no Worker/model.
	f, token, request, task := taskDispatchFixture(t)
	receipt := map[string]any{"stage": "T-TOOL-TASK-HANDOFF-213", "passed": false, "scope": "Production UI with actual HTTP/SQLite and owned writable inventory; one fixed preview, no Start/Worker/vendor/model/acceptance", "worker_runs": 0, "model_calls": 0, "vendor_calls": 0, "human_acceptance": false}
	t.Cleanup(func() {
		if _, e := f.s.DB.ExecContext(context.Background(), "VACUUM INTO ?", filepath.Join(dir, "private.sqlite")); e != nil {
			t.Error("preserve isolated DB snapshot", e)
		}
		if t.Failed() {
			receipt["passed"] = false
		}
		raw, _ := json.MarshalIndent(receipt, "", "  ")
		if e := os.WriteFile(filepath.Join(dir, "receipt.json"), append(raw, '\n'), 0600); e != nil {
			t.Error("preserve actual receipt", e)
		}
	})
	profile, err := f.s.GetToolProfile(context.Background(), f.actor, f.project, request["tool_profile_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if !profile.Available || profile.State != "enabled" || profile.Profile.ReadOnly || profile.Profile.Protocol != "suma-jsonl-v1" {
		t.Fatal("original owned writable profile is not current")
	}
	publicSession := f.call("GET", "/auth/session", nil, 200)
	var originalWorkflow string
	if err = f.s.DB.QueryRow(`SELECT spec FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=? AND version=?`, f.user.TenantID, f.project, request["workflow_id"], request["workflow_version"]).Scan(&originalWorkflow); err != nil {
		t.Fatal(err)
	}
	count := func(table string) int {
		t.Helper()
		var n int
		if e := f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	beforeProfiles := count("suma_tool_profiles")
	var requestsMu sync.Mutex
	requests := []map[string]any{}
	app := (&Server{Service: f.s, Engine: f.e, WebDir: web}).Handler()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/__fixture/view" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"session": publicSession, "project_id": f.project, "tool_id": profile.ID, "task_id": task["id"], "workflow_id": request["workflow_id"], "tool_snapshot": profile, "task": task})
			return
		}
		recorder := &toolTaskHandoffResponse213{ResponseWriter: w}
		app.ServeHTTP(recorder, r)
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			requestsMu.Lock()
			requests = append(requests, map[string]any{"method": r.Method, "path": r.URL.Path, "status": recorder.status})
			requestsMu.Unlock()
		}
	}))
	defer server.Close()
	private := map[string]any{"protocol": "suma-tool-task-handoff-actual-v1", "origin": server.URL, "directory": dir, "sessionCookie": map[string]string{"name": f.cookie.Name, "value": f.cookie.Value}, "project": f.project, "tool_id": profile.ID, "task_id": task["id"], "workflow_id": request["workflow_id"]}
	input, _ := json.Marshal(private)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, node, driver)
	attributes := &syscall.SysProcAttr{}
	if hidden := reflect.ValueOf(attributes).Elem().FieldByName("HideWindow"); hidden.IsValid() && hidden.CanSet() && hidden.Kind() == reflect.Bool {
		hidden.SetBool(true)
		command.SysProcAttr = attributes
	}
	var stdout, stderr bytes.Buffer
	command.Dir, command.Stdin, command.Stdout, command.Stderr = root, bytes.NewReader(input), &stdout, &stderr
	runErr := command.Run()
	redact := func(raw string) string {
		for _, secret := range []string{f.cookie.Value, token} {
			raw = strings.ReplaceAll(raw, secret, "[private fixture value removed]")
		}
		return raw
	}
	for name, raw := range map[string]string{"driver.stdout.json": stdout.String(), "driver.stderr.txt": stderr.String()} {
		if e := os.WriteFile(filepath.Join(dir, name), []byte(redact(raw)), 0600); e != nil {
			t.Fatal(e)
		}
	}
	var summary struct {
		Passed bool `json:"passed"`
	}
	if runErr != nil || json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &summary) != nil || !summary.Passed {
		t.Fatalf("actual UI driver failed; isolated evidence retained at %s", dir)
	}
	resultFile := filepath.Join(dir, "result.json")
	resultRaw, e := os.ReadFile(resultFile)
	if e != nil || len(resultRaw) > 2<<20 {
		t.Fatal("bounded driver result unavailable")
	}
	var result struct {
		Passed     bool `json:"passed"`
		NativePID  int  `json:"native_pid"`
		NativeExit struct {
			Code   *int `json:"code"`
			Signal any  `json:"signal"`
		} `json:"native_exit"`
	}
	if json.Unmarshal(resultRaw, &result) != nil || !result.Passed || result.NativePID < 1 || result.NativeExit.Code == nil || *result.NativeExit.Code != 0 || result.NativeExit.Signal != nil {
		t.Fatal("native browser did not close normally")
	}
	if count("suma_task_dispatch_plans") != 1 || count("dag_runs") != 0 || count("dag_attempts") != 0 || count("suma_usage_attempts") != 0 || count("suma_tool_profiles") != beforeProfiles {
		t.Fatal("selection/preview created an unrequested run, attempt or profile")
	}
	var payload string
	if err = f.s.DB.QueryRow(`SELECT payload FROM suma_task_dispatch_plans WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var plan TaskDispatchPlan
	if json.Unmarshal([]byte(payload), &plan) != nil || plan.ProjectID != f.project || plan.TaskID != task["id"] || plan.TaskVersion != int(task["version"].(float64)) || plan.ToolProfileID != profile.ID || plan.ToolProfileVersion != profile.Version || plan.ToolProfileSHA256 != profile.Profile.ProfileSHA256 || plan.WorkflowID != request["workflow_id"] || plan.WorkflowVersion != request["workflow_version"] || plan.AgentNodeID != "develop" || plan.State != "ready" {
		t.Fatal("prepared plan did not preserve the original exact task/tool/workflow pins")
	}
	var currentWorker string
	if err = f.s.DB.QueryRow(`SELECT worker_id FROM suma_tool_profiles WHERE tenant_id=? AND project_id=? AND id=? AND version=?`, f.user.TenantID, f.project, profile.ID, profile.Version).Scan(&currentWorker); err != nil || currentWorker != profile.WorkerID {
		t.Fatal("tool identity changed its worker", err)
	}
	matched := false
	for _, n := range plan.Workflow.Nodes {
		if n.ID == plan.AgentNodeID {
			goal, _ := n.Config["goal"].(string)
			matched = len(n.Command) == 1 && n.Command[0] == profile.Profile.CommandID && n.Config["tool_profile_id"] == profile.ID && n.Config["tool_profile_sha256"] == profile.Profile.ProfileSHA256 && strings.Contains(goal, task["title"].(string))
		}
	}
	if !matched || plan.Context.Task.ID != task["id"] || plan.Context.Task.Title != task["title"] {
		t.Fatal("fixed workflow or task context lost its exact source")
	}
	var afterWorkflow string
	if err = f.s.DB.QueryRow(`SELECT spec FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=? AND version=?`, f.user.TenantID, f.project, request["workflow_id"], request["workflow_version"]).Scan(&afterWorkflow); err != nil || afterWorkflow != originalWorkflow {
		t.Fatal("preview mutated the published workflow", err)
	}
	requestsMu.Lock()
	defer requestsMu.Unlock()
	previews := 0
	for _, r := range requests {
		path := r["path"].(string)
		if strings.Contains(path, "/runner/") || strings.Contains(path, "/pool-runner/") || strings.HasSuffix(path, "/start") {
			t.Fatal("actual UI started an execution")
		}
		if r["method"] == "POST" && path == "/api/v1/task-dispatch-plans" {
			previews++
		}
	}
	if previews != 1 {
		t.Fatal("expected exactly one explicit preview", previews)
	}
	receipt["passed"], receipt["origin"], receipt["requests"] = true, server.URL, requests
	receipt["driver_result_sha256"], receipt["native_pid"], receipt["native_exit"] = digest(string(resultRaw)), result.NativePID, result.NativeExit
	receipt["plan_id"], receipt["project_id"], receipt["task_id"], receipt["tool_id"], receipt["tool_version"], receipt["tool_profile_sha256"], receipt["worker_id"] = plan.ID, f.project, plan.TaskID, profile.ID, profile.Version, profile.Profile.ProfileSHA256, profile.WorkerID
	receipt["preview_calls"], receipt["plan_count"], receipt["run_count"], receipt["attempt_count"], receipt["profiles_created_by_journey"], receipt["published_workflow_preserved"] = previews, 1, 0, 0, 0, true
}

type toolTaskHandoffResponse213 struct {
	http.ResponseWriter
	status int
}

func (w *toolTaskHandoffResponse213) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *toolTaskHandoffResponse213) Write(raw []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(raw)
}
