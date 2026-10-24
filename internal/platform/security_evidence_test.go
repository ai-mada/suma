package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/ai-mada/suma/internal/dag"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOceanBaseInitialPayloadDDLDoesNotUse64KiBText(t *testing.T) {
	original := `CREATE TABLE x(content TEXT NOT NULL,data TEXT NOT NULL)`
	if got := (&Service{Dialect: "oceanbase"}).ddl(original); strings.Count(got, "LONGTEXT") != 2 {
		t.Fatalf("large payload types: %s", got)
	}
	for _, dialect := range []string{"sqlite", "postgres"} {
		if (&Service{Dialect: dialect}).ddl(original) != original {
			t.Fatal("non-MySQL dialect changed")
		}
	}
}

func (f *fixture) bearer(method, path, token string, body any, want int) map[string]any {
	f.t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, "http://suma.test/api/v1"+path, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	if w.Code != want {
		f.t.Fatalf("%s %s got %d want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	return result
}

func TestScopedTokensCannotEscapeThroughEarlyRoutes(t *testing.T) {
	f := newFixture(t)
	other := f.call("POST", "/projects", map[string]any{"title": "其他项目", "idempotency_key": newID()}, 201)
	for _, kind := range []string{"runner", "agent", "api"} {
		t.Run(kind, func(t *testing.T) {
			key := f.call("POST", "/auth/tokens", map[string]any{"project_id": f.project, "scope": kind}, 201)["token"].(string)
			f.bearer("GET", "/auth/session", key, nil, 403)
			f.bearer("POST", "/auth/tokens", key, map[string]any{"project_id": f.project}, 403)
			f.bearer("POST", "/projects", key, map[string]any{"title": "逃逸项目", "idempotency_key": newID()}, 403)
			f.bearer("GET", "/overview?project_id="+other["id"].(string), key, nil, 403)
			if kind == "api" {
				out := f.bearer("GET", "/projects", key, nil, 200)
				if out["total"] != float64(1) {
					t.Fatal("project scope leaked")
				}
				f.bearer("GET", "/overview?project_id="+f.project, key, nil, 200)
			} else {
				f.bearer("GET", "/projects", key, nil, 403)
				f.bearer("GET", "/overview?project_id="+f.project, key, nil, 403)
			}
			if kind == "agent" {
				f.bearer("GET", "/agent/tools?project_id="+f.project, key, nil, 200)
			}
			f.bearer("POST", "/auth/logout", key, map[string]any{}, 200)
			f.bearer("GET", "/projects", key, nil, 401)
		})
	}
}

func (f *fixture) testedTask(wf map[string]any, title string, deps []string) (map[string]any, map[string]any, map[string]any) {
	f.t.Helper()
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": title, "depends_on": deps}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": wf["id"], "task_id": task["id"]}), 201)
	a, e := f.e.Claim(context.Background(), f.user.TenantID, f.project, "evidence-worker", toolFixtureCapabilities(f.t, f))
	if e != nil || a == nil {
		f.t.Fatalf("claim %v", e)
	}
	if e = f.e.Complete(context.Background(), f.user.TenantID, f.project, dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID()}); e != nil {
		f.t.Fatal(e)
	}
	task = f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
	review := f.body(map[string]any{"expected_version": task["version"], "run_id": run["id"], "decision": "accepted", "reason": "测试输出和交付范围已核验"})
	task = f.call("POST", "/tasks/"+task["id"].(string)+"/review", review, 200)
	return task, run, review
}

func TestCompensationInvalidatesTransitiveBusinessEvidenceAndReceiptReplays(t *testing.T) {
	f := newFixture(t)
	wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "具有显式补偿的交付", "nodes": []any{map[string]any{"id": "test", "name": "验证", "type": "test", "command": []string{"test"}, "idempotent": true, "config": map[string]any{"compensation": map[string]any{"id": "undo", "name": "撤销效果", "type": "command", "command": []string{"undo"}, "idempotent": true}}}}, "edges": []any{}}), 201)
	a, run, review := f.testedTask(wf, "A", nil)
	b, _, _ := f.testedTask(wf, "B", []string{a["id"].(string)})
	current := f.call("GET", "/runs/"+run["id"].(string)+"?project_id="+f.project, nil, 200)
	f.call("POST", "/runs/"+run["id"].(string)+"/commands", f.body(map[string]any{"expected_version": current["version"], "action": "compensate", "payload": map[string]any{"reason": "撤销此前已交付效果"}}), 200)
	for _, task := range []map[string]any{a, b} {
		out := f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
		if out["status"] == "accepted" {
			t.Fatal("invalid acceptance survived compensation")
		}
	}
	// Lost-response replay returns the immutable receipt; it never reapplies acceptance.
	f.call("POST", "/tasks/"+a["id"].(string)+"/review", review, 200)
	actual := f.call("GET", "/tasks/"+a["id"].(string)+"?project_id="+f.project, nil, 200)
	if actual["status"] == "accepted" {
		t.Fatal("receipt replay mutated current state")
	}
	c := f.call("POST", "/tasks", f.body(map[string]any{"title": "C", "depends_on": []string{b["id"].(string)}}), 201)
	f.call("POST", "/runs", f.body(map[string]any{"workflow_id": wf["id"], "task_id": c["id"]}), 409)
}

func TestCapabilityProbeDoesNotClaimProductAcceptance(t *testing.T) {
	f := newFixture(t)
	out := f.call("GET", "/capabilities?project_id="+f.project, nil, http.StatusOK)
	for _, raw := range out["items"].([]any) {
		if raw.(map[string]any)["verified"] == true {
			t.Fatal("existence probe presented as full verification")
		}
	}
}

func TestUnselectedConditionalTestsDoNotBlockExecutedEvidence(t *testing.T) {
	f := newFixture(t)
	wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "条件测试范围", "nodes": []any{map[string]any{"id": "executed", "type": "test", "command": []string{"test"}}, map[string]any{"id": "unselected", "type": "test", "command": []string{"test"}, "condition": "false"}}, "edges": []any{}}), 201)
	f.testedTask(wf, "只验收本次已选范围", nil)
}

func TestAllSkippedTestsCannotSatisfyAcceptance(t *testing.T) {
	f := newFixture(t)
	wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "没有实际测试", "nodes": []any{map[string]any{"id": "unselected", "type": "test", "command": []string{"test"}, "condition": "false"}}, "edges": []any{}}), 201)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "空测试不能验收"}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": wf["id"], "task_id": task["id"]}), 201)
	task = f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
	f.call("POST", "/tasks/"+task["id"].(string)+"/review", f.body(map[string]any{"expected_version": task["version"], "run_id": run["id"], "decision": "accepted", "reason": "全部跳过仍不能满足真实测试要求"}), 400)
}
