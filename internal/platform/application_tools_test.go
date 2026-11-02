package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func applicationActor(t *testing.T, f *fixture, scope string) Actor {
	t.Helper()
	issued, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	a, err := f.s.AuthenticateActor(context.Background(), issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func applicationCall(t *testing.T, f *fixture, actor Actor, tool, args string) (any, error) {
	t.Helper()
	return f.s.InvokeApplicationTool(context.Background(), actor, ApplicationToolCall{ProjectID: f.project, Tool: tool, Arguments: json.RawMessage(args)})
}

func applicationStatus(err error) int {
	var api *APIError
	if errors.As(err, &api) {
		return api.Status
	}
	return 0
}

func TestApplicationToolsCurrentCatalogueAndAuthority(t *testing.T) {
	f := newFixture(t)
	agent, api, runner := applicationActor(t, f, "agent"), applicationActor(t, f, "api"), applicationActor(t, f, "runner")
	wantReadOnly := []string{"project.overview", "project.tasks", "project.search", "project.knowledge", "project.knowledge.search", "project.knowledge.index_search"}
	for _, actor := range []Actor{agent, f.actor} {
		list, err := f.s.ApplicationTools(context.Background(), actor, f.project)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, d := range list.Items {
			names = append(names, d.Name)
			if !d.ReadOnly {
				t.Fatal("read-only catalogue gained mutation")
			}
		}
		if !reflect.DeepEqual(names, wantReadOnly) {
			t.Fatalf("current read-only catalogue %v", names)
		}
	}
	list, err := f.s.ApplicationTools(context.Background(), api, f.project)
	if err != nil || len(list.Items) != len(DeclaredApplicationTools()) {
		t.Fatalf("API catalogue %d: %v", len(list.Items), err)
	}
	if _, err = f.s.ApplicationTools(context.Background(), runner, f.project); applicationStatus(err) != 403 {
		t.Fatalf("runner catalogue: %v", err)
	}
	if _, err = f.s.ApplicationTools(context.Background(), api, "foreign-project"); applicationStatus(err) != 403 {
		t.Fatalf("cross-project catalogue: %v", err)
	}
	for _, actor := range []Actor{agent, runner, f.actor} {
		if _, err = applicationCall(t, f, actor, "project.requirement_draft_create", `{"title":"forbidden","idempotency_key":"abcdefgh"}`); applicationStatus(err) != 403 {
			t.Fatalf("non-API mutation: %v", err)
		}
	}
	if _, err = applicationCall(t, f, agent, "project.requirement", `{"id":"anything"}`); applicationStatus(err) != 403 {
		t.Fatalf("agent new read: %v", err)
	}
	if _, err = applicationCall(t, f, api, "project.review", `{}`); applicationStatus(err) != 422 {
		t.Fatalf("unknown review tool: %v", err)
	}
	// A catalogue snapshot cannot make a revoked credential current.
	if _, err = f.s.DB.Exec(`UPDATE suma_credential_grants SET revoked=1 WHERE id=?`, api.credential.grantID); err != nil {
		t.Fatal(err)
	}
	if _, err = applicationCall(t, f, api, "project.tasks", `{}`); applicationStatus(err) != 401 {
		t.Fatalf("revoked call: %v", err)
	}
}

func TestApplicationToolsDeclarationsAreIndependent(t *testing.T) {
	first := DeclaredApplicationTools()
	want := map[string]bool{
		"project.overview": true, "project.tasks": true, "project.search": true, "project.knowledge": true,
		"project.knowledge.search": true, "project.knowledge.index_search": true,
		"project.requirements": true, "project.requirement": true, "project.task": true,
		"project.requirement_design": true, "project.task_design": true, "project.knowledge_list": true,
		"project.requirement_draft_create": true, "project.task_draft_create": true, "project.requirement_design_save": true,
		"project.skill_catalog_list": true, "project.skill_catalog_get": true,
		"project.task_delivery": true, "project.quality_execution": true,
	}
	if len(first) != len(want) {
		t.Fatalf("declared %d, expected exactly %d current tools", len(first), len(want))
	}
	first[0].Name = "changed"
	first[0].CredentialScopes[0] = "runner"
	first[1].Parameters["properties"].(map[string]any)["limit"].(map[string]any)["type"] = "string"
	second := DeclaredApplicationTools()
	if second[0].Name != "project.overview" || second[0].CredentialScopes[0] != "agent" || second[1].Parameters["properties"].(map[string]any)["limit"].(map[string]any)["type"] != "integer" {
		t.Fatal("caller mutated shared catalogue")
	}
	seen := map[string]bool{}
	for _, d := range second {
		if !want[d.Name] {
			t.Fatal("unexpected or missing current declaration", d.Name)
		}
		delete(want, d.Name)
		if seen[d.MCPName] || d.MCPName != "suma_"+strings.ReplaceAll(d.Name, ".", "_") || d.DescriptionKey == "" {
			t.Fatalf("invalid declaration: %+v", d)
		}
		seen[d.MCPName] = true
	}
}

func TestApplicationToolsStrictInputsAndReadBounds(t *testing.T) {
	f := newFixture(t)
	api := applicationActor(t, f, "api")
	for _, args := range []string{` `, `null`, `[]`, `{"limit":null}`, `{"limit":1,"limit":2}`, `{"limit":1.0}`, `{"limit":1e0}`, `{"Limit":1}`, `{"limit":true}`, `{"project_id":"elsewhere"}`, `{"limit":201}`, `{"cursor":null}`, `{} {}`} {
		if _, err := applicationCall(t, f, api, "project.tasks", args); applicationStatus(err) != 422 {
			t.Fatalf("accepted %s: %v", args, err)
		}
	}
	for _, args := range []string{`{}`, `{"query":null}`, `{"query":"q","extra":true}`} {
		if _, err := applicationCall(t, f, api, "project.search", args); applicationStatus(err) != 422 {
			t.Fatalf("search accepted %s: %v", args, err)
		}
	}
	if _, err := applicationCall(t, f, api, "project.knowledge", `{"references":[]}`); applicationStatus(err) != 400 {
		t.Fatalf("empty knowledge selection: %v", err)
	}
	for _, args := range []string{`{"references":[{"id":"x","revision":1.0,"sha256":"x"}]}`, `{"references":[{"id":"x","revision":1,"sha256":"x","id":"y"}]}`, `{"references":[null]}`} {
		if _, err := applicationCall(t, f, api, "project.knowledge", args); applicationStatus(err) != 422 {
			t.Fatalf("knowledge accepted %s: %v", args, err)
		}
	}
	f.call("POST", "/tasks", f.body(map[string]any{"title": "large", "description": strings.Repeat("x", 70000)}), 201)
	if _, err := applicationCall(t, f, api, "project.tasks", `{"limit":1}`); applicationStatus(err) != 422 {
		t.Fatalf("oversized read not rejected: %v", err)
	}
}

func TestApplicationToolsDraftCreationReceiptAndFixedStatus(t *testing.T) {
	f := newFixture(t)
	api := applicationActor(t, f, "api")
	args := `{"title":"formal draft","description":"original","acceptance_criteria":["must pass"],"idempotency_key":"application-requirement-1"}`
	first, err := applicationCall(t, f, api, "project.requirement_draft_create", args)
	if err != nil {
		t.Fatal(err)
	}
	second, err := applicationCall(t, f, api, "project.requirement_draft_create", args)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("replay %v %v: %v", first, second, err)
	}
	if _, err = applicationCall(t, f, api, "project.requirement_draft_create", strings.Replace(args, "original", "changed", 1)); applicationStatus(err) != 409 {
		t.Fatalf("changed replay %v", err)
	}
	var requirements, activities int
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_requirements WHERE project_id=? AND status='draft'`, f.project).Scan(&requirements)
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_activity WHERE project_id=? AND kind='requirements.created'`, f.project).Scan(&activities)
	if requirements != 1 || activities != 1 {
		t.Fatalf("duplicate effects %d/%d", requirements, activities)
	}
	var criteria string
	if err = f.s.DB.QueryRow(`SELECT acceptance_criteria FROM suma_requirements WHERE project_id=?`, f.project).Scan(&criteria); err != nil || criteria != `["must pass"]` {
		t.Fatalf("criteria lost %q: %v", criteria, err)
	}
	for _, field := range []string{`"status":"accepted"`, `"run_id":"forged"`, `"data":{"verified_run_id":"forged"}`, `"decision":"accepted"`, `"acceptance_criteria":null`} {
		if _, err = applicationCall(t, f, api, "project.task_draft_create", `{"title":"blocked","idempotency_key":"application-forbidden",`+field+`}`); applicationStatus(err) != 422 {
			t.Fatalf("unsafe field accepted %s: %v", field, err)
		}
	}
	out, err := applicationCall(t, f, api, "project.task_draft_create", `{"title":"unstarted draft","idempotency_key":"application-task-1"}`)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if len(raw) > 1024 || !strings.Contains(string(raw), `"status":"todo"`) || strings.Contains(string(raw), "description") {
		t.Fatalf("not compact fixed-status result %s", raw)
	}
	if domainACount(t, f.s, "dag_runs") != 0 {
		t.Fatal("draft created a run")
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_credential_grants SET revoked=1 WHERE id=?`, api.credential.grantID); err != nil {
		t.Fatal(err)
	}
	if _, err = applicationCall(t, f, api, "project.requirement_draft_create", args); applicationStatus(err) != 401 {
		t.Fatalf("receipt bypassed revocation: %v", err)
	}
}

func TestApplicationToolsDesignDraftRetainsApprovalBoundary(t *testing.T) {
	f := newFixture(t)
	api := applicationActor(t, f, "api")
	requirement := f.call("POST", "/requirements", f.body(map[string]any{"title": "design source", "acceptance_criteria": []string{"A"}}), 201)
	args := `{"requirement_id":"` + requirement["id"].(string) + `","expected_version":0,"expected_requirement_version":1,"content":{"product":"P","experience":"E","technical":"T"},"tasks":[],"idempotency_key":"application-design-1"}`
	first, err := applicationCall(t, f, api, "project.requirement_design_save", args)
	if err != nil {
		t.Fatal(err)
	}
	second, err := applicationCall(t, f, api, "project.requirement_design_save", args)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("design replay: %v", err)
	}
	view, err := f.s.GetRequirementDesign(context.Background(), api, f.project, requirement["id"].(string))
	if err != nil || view.Version != 1 || view.LatestRevision != 1 || view.PublishedRevision != 0 || !view.HasDraft {
		t.Fatalf("draft publication: %+v %v", view, err)
	}
	for _, bad := range []string{strings.Replace(args, `"expected_version":0`, `"expected_version":0.0`, 1), strings.Replace(args, `"tasks":[]`, `"tasks":null`, 1), strings.Replace(args, `"technical":"T"`, `"technical":"T","technical":"other"`, 1), strings.Replace(args, `"technical":"T"`, `"technical":null`, 1), strings.Replace(args, `"technical":"T"`, `"extra":"x"`, 1)} {
		if _, err = applicationCall(t, f, api, "project.requirement_design_save", bad); applicationStatus(err) != 422 {
			t.Fatalf("design accepted invalid input %s: %v", bad, err)
		}
	}
	if _, err = applicationCall(t, f, api, "project.requirement_design_save", strings.Replace(args, "application-design-1", "application-design-2", 1)); applicationStatus(err) != 409 {
		t.Fatalf("design stale version: %v", err)
	}
	if _, err = f.s.CommandRequirementDesign(context.Background(), api, f.project, requirement["id"].(string), RequirementDesignCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: 1, ExpectedRequirementVersion: 1, IdempotencyKey: newID()}); applicationStatus(err) != 403 {
		t.Fatalf("API approved design: %v", err)
	}
}

func TestApplicationToolsCurrentViewerCannotRecoverWriteReceipts(t *testing.T) {
	f := newFixture(t)
	api := applicationActor(t, f, "api")
	args := `{"title":"earlier authorized draft","idempotency_key":"application-viewer-replay"}`
	if _, err := applicationCall(t, f, api, "project.requirement_draft_create", args); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role) VALUES(?,?,'viewer') ON CONFLICT(project_id,user_id) DO UPDATE SET role='viewer'`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	list, err := f.s.ApplicationTools(context.Background(), api, f.project)
	if err != nil {
		t.Fatal(err)
	}
	// Discovery must expose only the current read tools with their declared
	// credential scopes, rather than a count that predates catalogue additions.
	expectedReads := map[string][]string{
		"project.overview":               {"agent", "api"},
		"project.tasks":                  {"agent", "api"},
		"project.search":                 {"agent", "api"},
		"project.knowledge":              {"agent", "api"},
		"project.knowledge.search":       {"agent", "api"},
		"project.knowledge.index_search": {"agent", "api"},
		"project.requirements":           {"api"},
		"project.requirement":            {"api"},
		"project.task":                   {"api"},
		"project.requirement_design":     {"api"},
		"project.task_design":            {"api"},
		"project.knowledge_list":         {"api"},
		"project.skill_catalog_list":     {"api"},
		"project.skill_catalog_get":      {"api"},
		"project.task_delivery":          {"api"},
		"project.quality_execution":      {"api"},
	}
	for _, definition := range list.Items {
		scopes, exists := expectedReads[definition.Name]
		if !exists || !definition.ReadOnly || !reflect.DeepEqual(definition.CredentialScopes, scopes) {
			t.Fatalf("viewer discovered unexpected tool or permission: %s read_only=%t scopes=%v", definition.Name, definition.ReadOnly, definition.CredentialScopes)
		}
		delete(expectedReads, definition.Name)
	}
	if len(expectedReads) != 0 {
		t.Fatalf("viewer lost permitted read tools: %v", expectedReads)
	}
	for _, body := range []string{args, strings.Replace(args, "application-viewer-replay", "application-viewer-fresh", 1)} {
		if _, err = applicationCall(t, f, api, "project.requirement_draft_create", body); applicationStatus(err) != 403 {
			t.Fatalf("viewer mutation/receipt: %v", err)
		}
	}
	if _, err = applicationCall(t, f, api, "project.requirements", `{}`); err != nil {
		t.Fatalf("viewer lost permitted read: %v", err)
	}
	if _, err = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = applicationCall(t, f, api, "project.requirements", `{}`); applicationStatus(err) != 403 {
		t.Fatalf("removed membership read: %v", err)
	}
}

func TestApplicationToolsDraftReferencesAndReadProjections(t *testing.T) {
	f := newFixture(t)
	api := applicationActor(t, f, "api")
	raw, err := applicationCall(t, f, api, "project.requirement_draft_create", `{"title":"source","acceptance_criteria":["criterion"],"idempotency_key":"application-read-source"}`)
	if err != nil {
		t.Fatal(err)
	}
	requirement := raw.(ApplicationToolDraftResult)
	first, err := applicationCall(t, f, api, "project.task_draft_create", `{"title":"dependency","requirement_id":"`+requirement.ID+`","idempotency_key":"application-read-dependency"}`)
	if err != nil {
		t.Fatal(err)
	}
	dependency := first.(ApplicationToolDraftResult)
	second, err := applicationCall(t, f, api, "project.task_draft_create", `{"title":"dependent","requirement_id":"`+requirement.ID+`","depends_on":["`+dependency.ID+`"],"acceptance_criteria":["test"],"idempotency_key":"application-read-dependent"}`)
	if err != nil {
		t.Fatal(err)
	}
	task := second.(ApplicationToolDraftResult)
	read, err := applicationCall(t, f, api, "project.task", `{"id":"`+task.ID+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	record := read.(Record)
	if record.RequirementID != requirement.ID || !reflect.DeepEqual(record.DependsOn, []string{dependency.ID}) || !reflect.DeepEqual(record.AcceptanceCriteria, []string{"test"}) {
		t.Fatalf("lost typed fields: %+v", record)
	}
	for _, tt := range []struct{ tool, args string }{
		{"project.requirement", `{"id":"` + requirement.ID + `"}`},
		{"project.requirements", `{"limit":1}`},
		{"project.requirement_design", `{"requirement_id":"` + requirement.ID + `"}`},
		{"project.task_design", `{"task_id":"` + task.ID + `"}`},
		{"project.knowledge_list", `{"limit":1}`},
	} {
		if _, err = applicationCall(t, f, api, tt.tool, tt.args); err != nil {
			t.Fatalf("%s failed: %v", tt.tool, err)
		}
	}
	before := domainACount(t, f.s, "suma_tasks")
	if _, err = applicationCall(t, f, api, "project.task_draft_create", `{"title":"missing dependency","depends_on":["foreign-task"],"idempotency_key":"application-invalid-dependency"}`); err == nil {
		t.Fatal("foreign dependency accepted")
	}
	if domainACount(t, f.s, "suma_tasks") != before {
		t.Fatal("failed draft partially persisted")
	}
	if _, err = applicationCall(t, f, api, "project.knowledge_list", `{"cursor":"`+strings.Repeat("x", 4097)+`"}`); err == nil {
		t.Fatal("oversized knowledge cursor accepted")
	}
}

func TestApplicationToolsNativeHTTPPreservesStrictReadOnlyBoundary(t *testing.T) {
	f := newFixture(t)
	issued, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "agent", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, tool, args string
		want             int
	}{
		{"read", "project.tasks", `{}`, 200},
		{"legacy-default-page", "project.tasks", `{"limit":0}`, 200},
		{"duplicate", "project.tasks", `{"limit":1,"limit":2}`, 400},
		{"fraction", "project.tasks", `{"limit":1.0}`, 422},
		{"null", "project.tasks", `{"limit":null}`, 422},
		{"unknown", "project.tasks", `{"project_id":"foreign"}`, 422},
		{"new-read", "project.requirements", `{}`, 422},
		{"mutation", "project.requirement_draft_create", `{"title":"blocked","idempotency_key":"native-blocked-write"}`, 422},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://suma.test/api/v1/agent/tools/invoke", strings.NewReader(`{"project_id":"`+f.project+`","tool":"`+tt.tool+`","arguments":`+tt.args+`}`))
			req.Header.Set("Authorization", "Bearer "+issued.Token)
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, req)
			if w.Code != tt.want {
				t.Fatalf("HTTP %d want%d: %s", w.Code, tt.want, w.Body.String())
			}
		})
	}
	if domainACount(t, f.s, "suma_requirements") != 0 {
		t.Fatal("native read adapter performed mutation")
	}
}
