package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/store"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

func inspectionCreate194(t *testing.T, f *fixture, p ToolProfile) tic.Inspection {
	t.Helper()
	v, e := f.s.CreateToolInspection(context.Background(), f.actor, f.project, p.ID, tic.CreateRequest{ProjectID: f.project, ExpectedVersion: p.Version, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func inspectionChangeClock194(t *testing.T, f *fixture, id string, change func(*tic.Inspection)) {
	t.Helper()
	var raw string
	if e := f.s.DB.QueryRow(`SELECT payload FROM suma_tool_inspections WHERE id=?`, id).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var r toolInspectionRow
	if e := json.Unmarshal([]byte(raw), &r); e != nil {
		t.Fatal(e)
	}
	change(&r.Value)
	if e := tic.ValidateInspection(r.Value); e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.DB.Exec(`UPDATE suma_tool_inspections SET payload=? WHERE id=?`, stringify(r), id); e != nil {
		t.Fatal(e)
	}
}
func TestToolInspectionLifecycleAtomicCorrelationAndRecovery(t *testing.T) {
	f, token, p := toolFixture(t)
	inspectionMaintenance194(t, f, token, p)
	ctx := context.Background()
	worker := toolActorForTest(t, f, token)
	req := tic.CreateRequest{ProjectID: f.project, ExpectedVersion: p.Version, IdempotencyKey: newID()}
	created, e := f.s.CreateToolInspection(ctx, f.actor, f.project, p.ID, req)
	if e != nil || created.State != "pending" {
		t.Fatal(created, e)
	}
	if _, e = f.s.CreateToolInspection(ctx, f.actor, f.project, p.ID, tic.CreateRequest{ProjectID: f.project, ExpectedVersion: p.Version, IdempotencyKey: newID()}); e == nil {
		t.Fatal("second unfinished request acquired the same worker")
	}
	claimReq := tic.ClaimRequest{ProjectID: f.project, WorkerID: p.WorkerID, ClaimID: newID()}
	claimed, e := f.s.ClaimToolInspection(ctx, worker, f.project, claimReq)
	if e != nil || claimed.Inspection == nil || claimed.Inspection.ID != created.ID || claimed.Inspection.Fence != 1 {
		t.Fatal(claimed, e)
	}
	again, e := f.s.ClaimToolInspection(ctx, worker, f.project, claimReq)
	if e != nil || stringify(again) != stringify(claimed) {
		t.Fatal("lost claim ACK did not restore exact original lease", again, e)
	}
	nullReq := claimReq
	nullReq.ClaimID = newID()
	empty, e := f.s.ClaimToolInspection(ctx, worker, f.project, nullReq)
	if e != nil || empty.Inspection != nil {
		t.Fatal("new nonce reacquired running inspection", empty, e)
	}
	replay, e := f.s.CreateToolInspection(ctx, f.actor, f.project, p.ID, req)
	if e != nil || stringify(replay) != stringify(created) {
		t.Fatal("create replay changed its original response", replay, e)
	}
	periodic := ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true}}}
	ack, e := f.s.ReportToolInventory(ctx, worker, f.project, periodic)
	if e != nil || ack.Inspection != nil {
		t.Fatal(ack, e)
	}
	head, e := f.s.GetToolInspection(ctx, f.actor, f.project, created.ID)
	if e != nil || head.State != "claimed" {
		t.Fatal("periodic freshness completed a requested probe", head, e)
	}
	ref := tic.Reference{InspectionID: created.ID, ClaimID: claimReq.ClaimID, Fence: 1}
	bad := ref
	bad.Fence++
	report := periodic
	report.OperationID = newID()
	report.Inspection = &bad
	before := expertCount(t, f, "suma_operations")
	if _, e = f.s.ReportToolInventory(ctx, worker, f.project, report); e == nil {
		t.Fatal("old/wrong fence changed inventory")
	}
	if expertCount(t, f, "suma_operations") != before {
		t.Fatal("rejected report persisted an ACK")
	}
	inspectionChangeClock194(t, f, created.ID, func(v *tic.Inspection) {
		at, _ := time.Parse(time.RFC3339Nano, v.CreatedAt)
		v.ClaimedAt = at.Format(time.RFC3339Nano)
		v.LeaseUntil = at.Add(time.Millisecond).Format(time.RFC3339Nano)
	})
	// The clock bound is data, not a sleep. Force a definitely past lease while
	// retaining valid source/time relationships in this isolated SQLite fixture.
	inspectionChangeClock194(t, f, created.ID, func(v *tic.Inspection) {
		v.CreatedAt = "2000-01-01T00:00:00Z"
		v.ExpiresAt = "2000-01-01T00:05:00Z"
		v.ClaimedAt = "2000-01-01T00:00:01Z"
		v.LeaseUntil = "2000-01-01T00:01:01Z"
	})
	// Keep the indexed creation fact in sync with the deliberate clock fixture.
	if _, e = f.s.DB.Exec(`UPDATE suma_tool_inspections SET created_at='2000-01-01T00:00:00Z' WHERE id=?`, created.ID); e != nil {
		t.Fatal(e)
	}
	head, e = f.s.GetToolInspection(ctx, f.actor, f.project, created.ID)
	if e != nil || head.State != "unknown" {
		t.Fatal("expired claim was automatically requeued", head, e)
	}
	failure := tic.FailureRequest{ProjectID: f.project, WorkerID: p.WorkerID, Inspection: ref, IdempotencyKey: newID(), Outcome: "unknown", FailureCode: "probe_interrupted"}
	unknown, e := f.s.FailToolInspection(ctx, worker, f.project, failure)
	if e != nil || unknown.State != "unknown" {
		t.Fatal(unknown, e)
	}
	report.Inspection = &ref
	ack, e = f.s.ReportToolInventory(ctx, worker, f.project, report)
	if e != nil || ack.Inspection == nil || *ack.Inspection != ref || ack.InspectionState != "completed" {
		t.Fatal("late original report could not reconcile unknown", ack, e)
	}
	same, e := f.s.ReportToolInventory(ctx, worker, f.project, report)
	if e != nil || stringify(same) != stringify(ack) {
		t.Fatal("inventory receipt replay changed", same, e)
	}
	head, e = f.s.GetToolInspection(ctx, f.actor, f.project, created.ID)
	if e != nil || head.State != "completed" || head.InventoryOperationID != report.OperationID || head.ResultCode != "reported_available" {
		t.Fatal(head, e)
	}
	again, e = f.s.ClaimToolInspection(ctx, worker, f.project, claimReq)
	if e != nil || again.Inspection == nil || again.Inspection.State != "completed" {
		t.Fatal("same claim did not read current head", again, e)
	}
	next := inspectionCreate194(t, f, p)
	empty, e = f.s.ClaimToolInspection(ctx, worker, f.project, nullReq)
	if e != nil || empty.Inspection != nil {
		t.Fatal("original null claim silently acquired later work", empty, e)
	}
	claimReq.ClaimID = newID()
	again, e = f.s.ClaimToolInspection(ctx, worker, f.project, claimReq)
	if e != nil || again.Inspection == nil || again.Inspection.ID != next.ID {
		t.Fatal(again, e)
	}
	// Current profile authority is checked before a previously committed ACK.
	if _, e = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "revoke", ExpectedVersion: p.Version, IdempotencyKey: newID()}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ReportToolInventory(ctx, worker, f.project, report); e == nil {
		t.Fatal("old inventory ACK bypassed current profile revocation")
	}
	list, e := f.s.ListToolInspections(ctx, f.actor, f.project, p.ID)
	if e != nil || len(list.Items) != 2 {
		t.Fatal(list, e)
	}
}

func TestToolInspectionCurrentAuthorityAndInitialSchema(t *testing.T) {
	f, token, p := toolFixture(t)
	inspectionMaintenance194(t, f, token, p)
	ctx := context.Background()
	// Two distinct browser intents contend for the same real worker, rather than
	// only checking the one-request limit sequentially.
	type createResult struct {
		value tic.Inspection
		err   error
	}
	start := make(chan struct{})
	results := make(chan createResult, 2)
	for range 2 {
		go func() {
			<-start
			v, e := f.s.CreateToolInspection(ctx, f.actor, f.project, p.ID, tic.CreateRequest{ProjectID: f.project, ExpectedVersion: p.Version, IdempotencyKey: newID()})
			results <- createResult{v, e}
		}()
	}
	close(start)
	var created tic.Inspection
	success := 0
	for range 2 {
		r := <-results
		if r.err == nil {
			success++
			created = r.value
		} else {
			var api *APIError
			if !errors.As(r.err, &api) || api.Code != "tool_inspection_in_progress" {
				t.Fatal("concurrent intent failed outside worker exclusion", r.err)
			}
		}
	}
	if success != 1 {
		t.Fatal("worker exclusion admitted multiple intents", success)
	}
	worker := toolActorForTest(t, f, token)
	f.call("GET", "/tool-inspections/"+created.ID+"?project_id="+f.project, nil, 200)
	f.call("GET", "/tool-profiles/"+p.ID+"/inspections?project_id="+f.project, nil, 200)
	f.bearer("POST", "/runner/tool-inventory", token, map[string]any{"project_id": f.project, "worker_id": p.WorkerID, "operation_id": newID(), "items": []tc.Observation{}, "inspection": nil}, 400)
	f.call("POST", "/tool-profiles/"+p.ID+"/inspections", f.body(map[string]any{"expected_version": p.Version}), 409)
	for locale, want := range map[string]string{"zh-CN": "执行节点已有未完成的工具重检，请先核对原请求", "en-US": "This worker already has an unfinished inspection. Reconcile the original request first."} {
		body := strings.NewReader(stringify(tic.CreateRequest{ProjectID: f.project, ExpectedVersion: p.Version, IdempotencyKey: newID()}))
		r := httptest.NewRequest(http.MethodPost, "/api/v1/tool-profiles/"+p.ID+"/inspections", body)
		r.AddCookie(f.cookie)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Suma-Expected-User", f.view.ID)
		r.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
		r.Header.Set("Accept-Language", locale)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		var result struct {
			Error struct {
				Message, MessageKey string
				Localized           bool
			}
		}
		if json.Unmarshal(w.Body.Bytes(), &result) != nil || w.Code != 409 || result.Error.Message != want || !result.Error.Localized {
			t.Fatal("inspection conflict lost requested language", locale, w.Code, w.Body.String())
		}
	}
	if _, e := f.s.GetToolInspection(ctx, worker, f.project, created.ID); e == nil {
		t.Fatal("Runner gained browser request visibility")
	}
	if _, e := f.s.CreateToolInspection(ctx, Actor{}, f.project, p.ID, tic.CreateRequest{}); e == nil {
		t.Fatal("invalid actor gained maintenance authority")
	}
	if _, e := f.s.DB.Exec(`UPDATE suma_tool_profiles SET version=version+1 WHERE id=?`, p.ID); e != nil {
		t.Fatal(e)
	}
	head, e := f.s.GetToolInspection(ctx, f.actor, f.project, created.ID)
	if e != nil || head.State != "superseded" {
		t.Fatal(head, e)
	}
	claim, e := f.s.ClaimToolInspection(ctx, worker, f.project, tic.ClaimRequest{ProjectID: f.project, WorkerID: p.WorkerID, ClaimID: newID()})
	if e != nil || claim.Inspection != nil {
		t.Fatal("changed profile acquired a probe", claim, e)
	}
	if _, e = f.s.DB.Exec(`ALTER TABLE suma_tool_inspections ADD COLUMN unsupported TEXT`); e != nil {
		t.Fatal(e)
	}
	if e = f.s.Initialize(ctx); !errors.Is(e, store.ErrInitialSchema) {
		t.Fatal("unknown inspection layout was repaired instead of rejected", e)
	}
	var payload string
	if e = f.s.DB.QueryRow(`SELECT payload FROM suma_tool_inspections WHERE id=?`, created.ID).Scan(&payload); e != nil || !strings.Contains(payload, created.ID) {
		t.Fatal("initial schema rejection changed original data", e)
	}
}

func TestToolInspectionPoolScopeFailureAndLateCompletion(t *testing.T) {
	p := newPoolMachineFixture(t)
	f := p.f
	ctx := context.Background()
	scope := p.scopes[0]
	credential := p.credential
	profile, e := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: credential.Runner.WorkerID, Name: "Pool inspection", Profile: toolTestProfile(), IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	inspectionMaintenance194(t, f, credential.Token, profile)
	f.bearer("POST", "/pool-runner/tool-inventory", credential.Token, map[string]any{"tenant_id": scope.TenantID, "project_id": scope.ProjectID, "runner_id": credential.Runner.ID, "operation_id": newID(), "items": []tc.Observation{}, "inspection": nil}, 400)
	created := inspectionCreate194(t, f, profile)
	req := tic.PoolToolInspectionClaimRequest{TenantID: scope.TenantID, ProjectID: scope.ProjectID, RunnerID: credential.Runner.ID, ClaimID: newID()}
	wrong := req
	wrong.ProjectID = p.scopes[1].ProjectID
	wrong.TenantID = scope.TenantID
	if _, e = f.s.ClaimPoolToolInspection(ctx, credential.Token, wrong); e == nil {
		t.Fatal("Pool claim borrowed another project's grant")
	}
	claim := f.bearer("POST", "/pool-runner/tool-inspections/claim", credential.Token, req, 200)
	var reply tic.ClaimReply
	if json.Unmarshal([]byte(stringify(claim)), &reply) != nil || reply.Inspection == nil || reply.Inspection.ID != created.ID {
		t.Fatal(claim)
	}
	ref := tic.Reference{InspectionID: created.ID, ClaimID: req.ClaimID, Fence: reply.Inspection.Fence}
	fail := tic.PoolToolInspectionFailureRequest{TenantID: scope.TenantID, ProjectID: scope.ProjectID, RunnerID: credential.Runner.ID, Inspection: ref, IdempotencyKey: newID(), Outcome: "unknown", FailureCode: "probe_timeout"}
	unknown, e := f.s.FailPoolToolInspection(ctx, credential.Token, fail)
	if e != nil || unknown.State != "unknown" {
		t.Fatal(unknown, e)
	}
	r := PoolToolInventoryRequest{TenantID: scope.TenantID, ProjectID: scope.ProjectID, RunnerID: credential.Runner.ID, OperationID: newID(), Inspection: &ref, Items: []tc.Observation{{Profile: profile.Profile, Available: false, Reason: "configured executable unavailable"}}}
	ack, e := f.s.ReportPoolToolInventory(ctx, credential.Token, r)
	if e != nil || ack.InspectionState != "completed" {
		t.Fatal(ack, e)
	}
	head, e := f.s.GetToolInspection(ctx, f.actor, f.project, created.ID)
	if e != nil || head.ResultCode != "reported_unavailable" {
		t.Fatal(head, e)
	}
	// The old snapshot cannot become an ACK after exact grant version drift.
	if _, e = f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET version=version+1 WHERE id=?`, p.grants[0].ID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ReportPoolToolInventory(ctx, credential.Token, r); e == nil {
		t.Fatal("old correlated receipt bypassed current exact Pool route")
	}
}
