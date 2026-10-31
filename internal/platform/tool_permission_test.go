package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

type toolPermissionFixture struct {
	f          *fixture
	token      string
	actor      Actor
	assignment dag.Assignment
	profile    ToolProfile
	pool       bool
}

func permissionRequest170() pc.Request {
	return pc.Request{IdempotencyKey: "permission-original-170", Protocol: ac.Protocol, ACP: &ac.ACPToolPermissionSource{SessionID: "session170", ToolCallID: "tool170", Title: "Write reviewed file", Kind: "edit", Details: "exact operation\nfile.go", DetailsComplete: true, Options: []ac.PermissionOption{{ID: "once", Name: "Allow once", Kind: "allow_once"}, {ID: "no", Name: "Reject once", Kind: "reject_once"}, {ID: "always", Name: "Always", Kind: "allow_always"}}}}
}

func permissionChoice170(option string) pc.Choice {
	kind := "allow_once"
	if option == "no" {
		kind = "reject_once"
	} else if option == "always" {
		kind = "allow_always"
	}
	return pc.Choice{Protocol: ac.Protocol, ACP: &ac.ACPToolPermissionChoice{OptionID: option, Kind: kind}}
}
func newToolPermissionFixture(t *testing.T, pooled bool) toolPermissionFixture {
	t.Helper()
	ctx := context.Background()
	var h toolPermissionFixture
	h.pool = pooled
	worker := "permission-worker170"
	runnerID, poolID := "", ""
	if pooled {
		p := newPoolMachineFixture(t)
		h.f = p.f
		h.token = p.credential.Token
		worker = p.credential.Runner.WorkerID
		runnerID = p.credential.Runner.ID
		poolID = p.pool.ID
	} else {
		h.f = newFixture(t)
		w := h.f.call("POST", "/release-workers", h.f.body(map[string]any{"worker_id": worker, "profiles": []any{}, "deployment": false}), 201)
		h.token = w["token"].(string)
		h.actor = toolActorForTest(t, h.f, h.token)
	}
	f := h.f
	if err := f.s.InitializeToolPermissions(ctx); err != nil {
		t.Fatal(err)
	}
	var profile tc.Profile
	raw := `{"command_id":"qoder170","adapter":"qoder","protocol":"acp-stdio-v1","profile_sha256":"` + strings.Repeat("a", 64) + `","executable_sha256":"` + strings.Repeat("b", 64) + `","tool_version":"1.0.0","read_only":false,"acp_execution":{"runtime":"qoder","protocol_version":1,"sdk_version":"0.13.5","configuration_sha256":"` + strings.Repeat("c", 64) + `","model":"qoder170-model","model_source":"administrator_session_config","budget_mode":"observe_only","identity_evidence":"configuration_ack_only","auth_strategy":"existing_cli_login","permission_mode":"human_once"}}`
	if err := json.Unmarshal([]byte(raw), &profile); err != nil {
		t.Fatal(err)
	}
	p, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: worker, Name: "ACP permission fixture", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if pooled {
		f.bearer("POST", "/pool-runner/tool-inventory", h.token, PoolToolInventoryRequest{RunnerID: runnerID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: profile, Available: true}}}, 200)
	} else {
		if _, err = f.s.ReportToolInventory(ctx, h.actor, f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: worker, OperationID: newID(), Items: []tc.Observation{{Profile: profile, Available: true}}}); err != nil {
			t.Fatal(err)
		}
	}
	p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: p.Version, IdempotencyKey: newID()})
	if err != nil || !p.Available {
		t.Fatal("enable", err)
	}
	h.profile = p
	var capValue any
	if err = json.Unmarshal([]byte(raw), &capValue); err != nil {
		t.Fatal(err)
	}
	workflow := dag.Workflow{ID: "permission-flow170", Name: "ACP permission fixture", Nodes: []dag.Node{{ID: "work", Type: "agent", Command: []string{"qoder170"}, Config: map[string]any{"goal": "controlled objective", "tool_profile_id": p.ID, "tool_profile_sha256": p.Profile.ProfileSHA256, "acp_execution": capValue.(map[string]any)["acp_execution"]}}}}
	if _, err = f.e.Publish(ctx, f.user.TenantID, f.project, workflow); err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.CreateRun(ctx, f.user.TenantID, f.project, workflow.ID, newID()); err != nil {
		t.Fatal(err)
	}
	var claim map[string]any
	if pooled {
		claim = f.bearer("POST", "/pool-runner/claim", h.token, dag.PoolClaimRequest{PoolID: poolID, RunnerID: runnerID, WorkerID: worker, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:qoder170"}}}}, 200)
	} else {
		claim = f.bearer("POST", "/runner/claim", h.token, f.body(map[string]any{"worker_id": worker, "capabilities": []string{"command:qoder170"}}), 200)
	}
	if err = json.Unmarshal([]byte(stringify(claim["assignment"])), &h.assignment); err != nil || h.assignment.RunID == "" {
		t.Fatal("claim", err, claim)
	}
	return h
}
func (h toolPermissionFixture) open(ctx context.Context, r pc.Request) (pc.Permission, error) {
	if h.pool {
		return h.f.s.OpenPoolToolPermission(ctx, h.token, h.f.project, h.assignment, r)
	}
	return h.f.s.OpenToolPermission(ctx, h.actor, h.f.project, h.assignment, r)
}
func (h toolPermissionFixture) poll(ctx context.Context, id string) (pc.Permission, error) {
	if h.pool {
		return h.f.s.PollPoolToolPermission(ctx, h.token, h.f.project, h.assignment, id)
	}
	return h.f.s.PollToolPermission(ctx, h.actor, h.f.project, h.assignment, id)
}

func TestToolPermissionCurrentSourceReceiptAndOnceDecision(t *testing.T) {
	h := newToolPermissionFixture(t, false)
	ctx := context.Background()
	req := permissionRequest170()
	p, err := h.open(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	again, err := h.open(ctx, req)
	if err != nil || again.ID != p.ID {
		t.Fatal("original request duplicated", err)
	}
	req.ACP.Details = "changed"
	if _, err = h.open(ctx, req); err == nil {
		t.Fatal("changed original body replayed")
	}
	decision := pc.DecisionRequest{ProjectID: h.f.project, ExpectedVersion: p.Version, IdempotencyKey: "decision-original-170", Choice: permissionChoice170("once")}
	p, err = h.f.s.DecideToolPermission(ctx, h.f.actor, p.ID, decision)
	if err != nil || p.State != "decided" || p.Decision == nil || p.Decision.ACP.Kind != "allow_once" {
		t.Fatal("exact once decision", err)
	}
	replay, err := h.f.s.DecideToolPermission(ctx, h.f.actor, p.ID, decision)
	if err != nil || replay.ID != p.ID || replay.Version != p.Version {
		t.Fatal("original decision did not recover", err)
	}
	if _, err = h.f.s.DecideToolPermission(ctx, h.f.actor, p.ID, pc.DecisionRequest{ProjectID: h.f.project, ExpectedVersion: 1, IdempotencyKey: newID(), Choice: permissionChoice170("no")}); err == nil {
		t.Fatal("decided source accepted a second effect")
	}
	observed, err := h.poll(ctx, p.ID)
	if err != nil || observed.Decision == nil || observed.Decision.ACP.OptionID != "once" {
		t.Fatal("machine did not observe exact decision", err)
	}
	if _, err = h.f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, h.assignment.RunID, h.assignment.NodeID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.open(ctx, permissionRequest170()); err == nil {
		t.Fatal("old lease recovered executable request")
	}
	observed, err = h.poll(ctx, p.ID)
	if err != nil || observed.State != "expired" || observed.Decision == nil || len(observed.AllowedActions) != 0 {
		t.Fatal("expired lease lost its historical decision or returned executable actions", err)
	}
	if _, err = h.f.s.DecideToolPermission(ctx, h.f.actor, p.ID, decision); err != nil {
		t.Fatal("safe old receipt required a new live operation", err)
	}
}

func TestToolPermissionIncompleteAlwaysAndAttemptQuota(t *testing.T) {
	h := newToolPermissionFixture(t, false)
	ctx := context.Background()
	req := permissionRequest170()
	req.ACP.DetailsComplete = false
	p, err := h.open(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range []string{"once", "always"} {
		if _, err = h.f.s.DecideToolPermission(ctx, h.f.actor, p.ID, pc.DecisionRequest{ProjectID: h.f.project, ExpectedVersion: p.Version, IdempotencyKey: newID(), Choice: permissionChoice170(option)}); err == nil {
			t.Fatal("incomplete/always granted", option)
		}
	}
	if _, err = h.f.s.DecideToolPermission(ctx, h.f.actor, p.ID, pc.DecisionRequest{ProjectID: h.f.project, ExpectedVersion: p.Version, IdempotencyKey: newID(), Choice: permissionChoice170("no")}); err != nil {
		t.Fatal("exact rejection blocked", err)
	}
	for i := 1; i < 32; i++ {
		req.IdempotencyKey = newID()
		req.ACP.ToolCallID = newID()
		if _, err = h.open(ctx, req); err != nil {
			t.Fatal("bounded prefix", i, err)
		}
	}
	req.IdempotencyKey = newID()
	req.ACP.ToolCallID = newID()
	if _, err = h.open(ctx, req); err == nil {
		t.Fatal("33rd permission accepted")
	}
}

func TestToolPermissionPoolCurrentGrantAndSupersededFence(t *testing.T) {
	h := newToolPermissionFixture(t, true)
	ctx := context.Background()
	p, err := h.open(ctx, permissionRequest170())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.f.s.DB.Exec(`UPDATE dag_nodes SET fence=fence+1 WHERE run_id=? AND id=?`, h.assignment.RunID, h.assignment.NodeID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.open(ctx, permissionRequest170()); err == nil {
		t.Fatal("stale pool fence admitted")
	}
	observed, err := h.poll(ctx, p.ID)
	if err != nil || observed.State != "superseded" || observed.Decision != nil {
		t.Fatal("old fence returned decision", err)
	}
	if _, err = h.f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET status='revoked' WHERE id=?`, h.assignment.PoolGrantID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.poll(ctx, p.ID); err == nil {
		t.Fatal("withdrawn grant retained permission access")
	}
}

func TestToolPermissionHTTPPoolUsesOriginalIdentityEnvelope(t *testing.T) {
	h := newToolPermissionFixture(t, true)
	a := h.assignment
	identity := dag.Assignment{TenantID: a.TenantID, ProjectID: a.ProjectID, PoolID: a.PoolID, PoolRunnerID: a.PoolRunnerID, PoolSlotID: a.PoolSlotID, PoolGrantID: a.PoolGrantID, PoolGrantVersion: a.PoolGrantVersion, PoolCredentialID: a.PoolCredentialID, PoolCredentialVersion: a.PoolCredentialVersion, RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, DispatchID: a.DispatchID, DispatchState: a.DispatchState, LeaseUntil: a.LeaseUntil}
	body := map[string]any{"runner_id": a.PoolRunnerID, "tenant_id": a.TenantID, "project_id": a.ProjectID, "assignment": identity, "request": permissionRequest170()}
	p := h.f.bearer("POST", "/pool-runner/tool-permissions", h.token, body, 200)
	h.f.bearer("POST", "/pool-runner/tool-permissions/poll", h.token, map[string]any{"runner_id": a.PoolRunnerID, "tenant_id": a.TenantID, "project_id": a.ProjectID, "assignment": identity, "permission_id": p["id"]}, 200)
	body["runner_id"] = "another-runner"
	h.f.bearer("POST", "/pool-runner/tool-permissions", h.token, body, 400)
	delete(body, "runner_id")
	h.f.bearer("POST", "/pool-runner/tool-permissions", h.token, body, 400)
}

func TestToolPermissionHTTPRejectsLossyRawAndHumanScope(t *testing.T) {
	h := newToolPermissionFixture(t, false)
	body := ToolPermissionOpenRequest{ProjectID: h.f.project, Assignment: h.assignment, Request: permissionRequest170()}
	raw := stringify(body)
	for _, broken := range []string{strings.Replace(raw, `"details_complete":true`, `"details_complete":null`, 1), strings.Replace(raw, `"details_complete":true`, `"details_complete":true,"details_complete":false`, 1), strings.Replace(raw, `"title":"Write reviewed file"`, `"title":"\ud800"`, 1)} {
		r := httptest.NewRequest("POST", "http://suma.test/api/v1/runner/tool-permissions", strings.NewReader(broken))
		r.Header.Set("Authorization", "Bearer "+h.token)
		w := httptest.NewRecorder()
		h.f.handler.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("raw invalid accepted: %d %s", w.Code, w.Body.String())
		}
	}
	p := h.f.bearer("POST", "/runner/tool-permissions", h.token, body, 200)
	h.f.bearer("POST", "/runner/tool-permissions?project_id="+h.f.project, h.token, body, 400)
	for _, scope := range []string{"api", "agent"} {
		token := h.f.call("POST", "/auth/tokens", h.f.body(map[string]any{"scope": scope}), 201)["token"].(string)
		h.f.bearer("POST", "/tool-permissions/"+p["id"].(string)+"/decision", token, pc.DecisionRequest{ProjectID: h.f.project, ExpectedVersion: 1, IdempotencyKey: newID(), Choice: permissionChoice170("once")}, 403)
	}
	h.f.call("GET", "/runs/"+h.assignment.RunID+"/nodes/"+h.assignment.NodeID+"/tool-permissions?project_id="+h.f.project, nil, 200)
}

func TestToolPermissionQueuedAllowCannotCrossPauseOrBudgetChange(t *testing.T) {
	for _, change := range []string{"pause", "enforce"} {
		t.Run(change, func(t *testing.T) {
			h := newToolPermissionFixture(t, false)
			ctx := context.Background()
			p, err := h.open(ctx, permissionRequest170())
			if err != nil {
				t.Fatal(err)
			}
			q := pc.DecisionRequest{ProjectID: h.f.project, ExpectedVersion: 1, IdempotencyKey: newID(), Choice: permissionChoice170("once")}
			if _, err = h.f.s.DecideToolPermission(ctx, h.f.actor, p.ID, q); err != nil {
				t.Fatal(err)
			}
			if change == "pause" {
				_, err = h.f.s.DB.Exec(`UPDATE dag_runs SET state='paused' WHERE id=?`, h.assignment.RunID)
			} else {
				codexPolicy(t, h.f, "project", "enforce", 0)
			}
			if err != nil {
				t.Fatal(err)
			}
			observed, err := h.poll(ctx, p.ID)
			if err != nil || observed.State != "superseded" || observed.Decision == nil || len(observed.AllowedActions) != 0 {
				t.Fatal("queued allow escaped current new-work authority", err)
			}
			if _, err = h.f.s.DecideToolPermission(ctx, h.f.actor, p.ID, q); err != nil {
				t.Fatal("historical receipt was treated as new permission delivery", err)
			}
		})
	}
}

func TestToolPermissionCurrentAuthoritiesRecheckOriginalReceipts(t *testing.T) {
	t.Run("human_approval", func(t *testing.T) {
		h := newToolPermissionFixture(t, false)
		ctx := context.Background()
		p, err := h.open(ctx, permissionRequest170())
		if err != nil {
			t.Fatal(err)
		}
		inv, err := h.f.s.CreateInvitation(ctx, h.f.actor, map[string]any{"email": "permission-reviewer170@example.test", "role": "member", "grants": []any{map[string]any{"project_id": h.f.project, "role": "developer"}}, "idempotency_key": newID()})
		if err != nil {
			t.Fatal(err)
		}
		user, token, err := h.f.s.AcceptInvitation(ctx, map[string]any{"token": inv["token"], "email": "permission-reviewer170@example.test", "name": "Permission reviewer", "password": "permission-reviewer-strong-170", "idempotency_key": newID()})
		if err != nil {
			t.Fatal(err)
		}
		actor, err := h.f.s.AuthenticateActor(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		list, err := h.f.s.ListToolPermissions(ctx, actor, h.f.project, p.RunID, p.NodeID)
		if err != nil || len(list.Items) != 1 || len(list.Items[0].AllowedActions) != 0 {
			t.Fatal("read/developer acquired approval", err)
		}
		q := pc.DecisionRequest{ProjectID: h.f.project, ExpectedVersion: 1, IdempotencyKey: newID(), Choice: permissionChoice170("once")}
		if _, err = h.f.s.DecideToolPermission(ctx, actor, p.ID, q); err == nil {
			t.Fatal("developer decided permission")
		}
		if _, err = h.f.s.DB.Exec(`UPDATE suma_project_members SET role='reviewer' WHERE project_id=? AND user_id=?`, h.f.project, user.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = h.f.s.DecideToolPermission(ctx, actor, p.ID, q); err != nil {
			t.Fatal(err)
		}
		if _, err = h.f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, h.f.project, user.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = h.f.s.DecideToolPermission(ctx, actor, p.ID, q); err == nil {
			t.Fatal("revoked approval recovered a receipt")
		}
	})
	t.Run("machine_credential", func(t *testing.T) {
		h := newToolPermissionFixture(t, false)
		ctx := context.Background()
		p, err := h.open(ctx, permissionRequest170())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = h.f.s.DB.Exec(`UPDATE suma_sessions SET expires_at='2000-01-01T00:00:00Z' WHERE token_hash=?`, digest(h.token)); err != nil {
			t.Fatal(err)
		}
		if _, err = h.open(ctx, permissionRequest170()); err == nil {
			t.Fatal("cached machine actor restored a revoked request")
		}
		if _, err = h.poll(ctx, p.ID); err == nil {
			t.Fatal("revoked machine observed private permission")
		}
	})
	t.Run("profile_pause", func(t *testing.T) {
		h := newToolPermissionFixture(t, false)
		ctx := context.Background()
		p, err := h.open(ctx, permissionRequest170())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = h.f.s.CommandToolProfile(ctx, h.f.actor, h.f.project, h.profile.ID, ToolProfileCommandRequest{ProjectID: h.f.project, Action: "pause", ExpectedVersion: h.profile.Version, IdempotencyKey: newID()}); err != nil {
			t.Fatal(err)
		}
		if _, err = h.poll(ctx, p.ID); err == nil {
			t.Fatal("paused profile retained permission delivery")
		}
		if _, err = h.f.s.DecideToolPermission(ctx, h.f.actor, p.ID, pc.DecisionRequest{ProjectID: h.f.project, ExpectedVersion: 1, IdempotencyKey: newID(), Choice: permissionChoice170("once")}); err == nil {
			t.Fatal("paused profile accepted a human grant")
		}
	})
}

func TestToolPermissionDecisionReceiptRollbackIsAtomic(t *testing.T) {
	h := newToolPermissionFixture(t, false)
	ctx := context.Background()
	p, err := h.open(ctx, permissionRequest170())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.f.s.DB.Exec(`CREATE TRIGGER permission_receipt_fail BEFORE INSERT ON suma_operations WHEN NEW.operation_id='permission-receipt-fail170' BEGIN SELECT RAISE(ABORT,'controlled receipt storage failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = h.f.s.DecideToolPermission(ctx, h.f.actor, p.ID, pc.DecisionRequest{ProjectID: h.f.project, ExpectedVersion: 1, IdempotencyKey: "permission-receipt-fail170", Choice: permissionChoice170("once")}); err == nil {
		t.Fatal("receipt failure ignored")
	}
	var version int
	var state string
	if err = h.f.s.DB.QueryRow(`SELECT version,state FROM suma_tool_permissions WHERE id=?`, p.ID).Scan(&version, &state); err != nil || version != 1 || state != "pending" {
		t.Fatal("unreceipted grant survived rollback", err)
	}
}

func TestToolPermissionInitialSchemaGuardPrecedesDDL(t *testing.T) {
	db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "partial.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE suma_tool_permissions(id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: db, Dialect: "sqlite"}
	if err = s.InitializeToolPermissions(context.Background()); !errors.Is(err, store.ErrInitialSchema) {
		t.Fatal("partial permission schema accepted", err)
	}
	var tables int
	if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='suma_schema_baselines'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatal("initial guard ran after DDL", err)
	}
}

func TestToolPermissionBrowserHistoryLocatorPreservesFactWithoutGrant(t *testing.T) {
	h := newToolPermissionFixture(t, false)
	ctx := context.Background()
	p, err := h.open(ctx, permissionRequest170())
	if err != nil {
		t.Fatal(err)
	}
	q := pc.DecisionRequest{ProjectID: h.f.project, ExpectedVersion: 1, IdempotencyKey: newID(), Choice: permissionChoice170("once")}
	p, err = h.f.s.DecideToolPermission(ctx, h.f.actor, p.ID, q)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, p.RunID, p.NodeID); err != nil {
		t.Fatal(err)
	}
	list, err := h.f.s.ListToolPermissions(ctx, h.f.actor, h.f.project, p.RunID, p.NodeID)
	if err != nil || len(list.Items) != 1 || list.Items[0].State != "decided" || list.Items[0].Decision == nil || list.Items[0].Decision.ACP.OptionID != "once" || len(list.Items[0].AllowedActions) != 0 {
		t.Fatal("browser expired view erased human fact", err)
	}
	if _, err = h.f.s.DB.Exec(`UPDATE dag_nodes SET attempt=attempt+1,fence=fence+1 WHERE run_id=? AND id=?`, p.RunID, p.NodeID); err != nil {
		t.Fatal(err)
	}
	path := "/runs/" + p.RunID + "/nodes/" + p.NodeID + "/tool-permissions?project_id=" + h.f.project
	current := h.f.call("GET", path, nil, 200)
	if len(current["items"].([]any)) != 0 {
		t.Fatal("default current-attempt list inherited old grants")
	}
	history := h.f.call("GET", path+"&permission_id="+p.ID, nil, 200)
	var restored pc.List
	if err = json.Unmarshal([]byte(stringify(history)), &restored); err != nil || len(restored.Items) != 1 || restored.Items[0].State != "decided" || restored.Items[0].Decision == nil || restored.Items[0].Decision.ActorID != h.f.user.ID || len(restored.Items[0].AllowedActions) != 0 {
		t.Fatal("exact history locator lost original fact", err)
	}
	h.f.call("GET", path+"&permission_id="+p.ID+"&permission_id="+p.ID, nil, 400)
	h.f.call("GET", path+"&permission_id=", nil, 400)
	h.f.call("GET", path+"&permission_id=unknown170", nil, 404)
	if observed, e := h.poll(ctx, p.ID); e != nil || observed.State != "superseded" || observed.Decision == nil || len(observed.AllowedActions) != 0 {
		t.Fatal("historical lookup revived machine grant", e)
	}
}
