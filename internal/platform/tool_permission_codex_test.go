package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

func TestCodexToolPermissionRejectsRegisteredPreviousSchemaBeforeDDL(t *testing.T) {
	db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "previous-permissions.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		`CREATE TABLE suma_schema_baselines(component VARCHAR(40) PRIMARY KEY,version VARCHAR(80) NOT NULL)`,
		`CREATE TABLE suma_tool_permissions(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,worker_id VARCHAR(100) NOT NULL,dispatch_id VARCHAR(128) NOT NULL,profile_id VARCHAR(64) NOT NULL,profile_version INTEGER NOT NULL,profile_payload TEXT NOT NULL,operation_id VARCHAR(128) NOT NULL,request_sha256 VARCHAR(64) NOT NULL,session_id VARCHAR(128) NOT NULL,tool_call_id VARCHAR(128) NOT NULL,version BIGINT NOT NULL,state VARCHAR(32) NOT NULL,expires_at VARCHAR(40) NOT NULL,assignment TEXT NOT NULL,payload TEXT NOT NULL,UNIQUE(tenant_id,project_id,worker_id,operation_id),UNIQUE(tenant_id,project_id,run_id,node_id,attempt,fence,session_id,tool_call_id))`,
		`CREATE TABLE user_kept_fact(value TEXT NOT NULL)`,
		`INSERT INTO user_kept_fact VALUES('preserve-original-user-data')`,
	} {
		if _, err = db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`INSERT INTO suma_schema_baselines VALUES('platform',?)`, store.InitialDataContract); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: db, Dialect: "sqlite"}
	if err = s.Initialize(context.Background()); !errors.Is(err, store.ErrInitialSchema) {
		t.Fatal("previous registered ACP permission schema was accepted", err)
	}
	var kept string
	var count int
	if err = db.QueryRow(`SELECT value FROM user_kept_fact`).Scan(&kept); err != nil || kept != "preserve-original-user-data" {
		t.Fatal("initial schema rejection changed user data", err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&count); err != nil || count != 3 {
		t.Fatal("initial schema rejection performed DDL", count, err)
	}
}

func newCodexToolPermissionFixture(t *testing.T, pooled bool, goals ...string) (toolPermissionFixture, cc.Binding) {
	return newCodexToolInteractionFixture(t, pooled, false, goals...)
}

func newCodexToolInteractionFixture(t *testing.T, pooled, userInput bool, goals ...string) (toolPermissionFixture, cc.Binding) {
	t.Helper()
	ctx := context.Background()
	h := toolPermissionFixture{pool: pooled}
	worker, runnerID, poolID := "codex-approval-worker174", "", ""
	if pooled {
		p := newPoolMachineFixture(t)
		h.f, h.token = p.f, p.credential.Token
		worker, runnerID, poolID = p.credential.Runner.WorkerID, p.credential.Runner.ID, p.pool.ID
	} else {
		h.f = newFixture(t)
		registration := h.f.call("POST", "/release-workers", h.f.body(map[string]any{"worker_id": worker, "profiles": []any{}, "deployment": false}), 201)
		h.token = registration["token"].(string)
		h.actor = toolActorForTest(t, h.f, h.token)
	}
	f := h.f
	profile := tc.Profile{CommandID: "codex-approval174", Adapter: "codex", Protocol: cc.Protocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "Controlled Codex approval domain fixture"}
	if userInput {
		profile.UserInputProtocol = cc.UserInputProtocol
	}
	tool, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: worker, Name: "Explicit Codex once approval fixture", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := cc.NewCatalog(strings.Repeat("c", 64), []cc.ModelOption{{Model: "owned-approval-model", ReasoningEfforts: []string{"low"}, DefaultReasoningEffort: "low"}})
	if err != nil {
		t.Fatal(err)
	}
	observation := tc.Observation{Profile: profile, Available: true, CodexCatalog: &catalog}
	if pooled {
		f.bearer("POST", "/pool-runner/tool-inventory", h.token, PoolToolInventoryRequest{RunnerID: runnerID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{observation}}, 200)
	} else if _, err = f.s.ReportToolInventory(ctx, h.actor, f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: worker, OperationID: newID(), Items: []tc.Observation{observation}}); err != nil {
		t.Fatal(err)
	}
	tool, err = f.s.CommandToolProfile(ctx, f.actor, f.project, tool.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: tool.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.profile = tool
	binding, err := cc.NewBinding(catalog, cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "owned-approval-model", ReasoningEffort: "low"})
	if err != nil {
		t.Fatal(err)
	}
	var native map[string]any
	if err = json.Unmarshal([]byte(stringify(binding)), &native); err != nil {
		t.Fatal(err)
	}
	goal := "Controlled original approval domain objective"
	if len(goals) != 0 {
		goal = goals[0]
	}
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Native approval fixture", Nodes: []dag.Node{{ID: "work", Type: "agent", Command: []string{profile.CommandID}, Config: map[string]any{"goal": goal, "tool_profile_id": tool.ID, "tool_profile_sha256": profile.ProfileSHA256, "codex_execution": native}}}})
	if err != nil {
		t.Fatal(err)
	}
	f.e.AttemptAccounting = f.s
	if _, err = f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID()); err != nil {
		t.Fatal(err)
	}
	var claim map[string]any
	if pooled {
		claim = f.bearer("POST", "/pool-runner/claim", h.token, dag.PoolClaimRequest{PoolID: poolID, RunnerID: runnerID, WorkerID: worker, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:" + profile.CommandID}}}}, 200)
	} else {
		claim = f.bearer("POST", "/runner/claim", h.token, f.body(map[string]any{"worker_id": worker, "capabilities": []string{"command:" + profile.CommandID}}), 200)
	}
	if json.Unmarshal([]byte(stringify(claim["assignment"])), &h.assignment) != nil || h.assignment.RunID == "" {
		t.Fatal("native fixture did not claim", claim)
	}
	return h, binding
}

func codexToolRequest174(binding cc.Binding, id string) pc.Request {
	return pc.Request{IdempotencyKey: newID(), Protocol: cc.Protocol, Codex: &cc.CodexApprovalRequest{RequestID: cc.CodexRPCRequestID{Kind: "string", Value: id}, Method: "item/commandExecution/requestApproval", ThreadID: "thread174", TurnID: "turn174", ItemID: "item174", StartedAtMS: 1000, BindingSHA256: binding.BindingSHA256, ParamsSHA256: strings.Repeat("d", 64), ItemSHA256: strings.Repeat("e", 64), Title: "Review this exact native command", Details: "git status --short", DetailsComplete: true, ScopeSupported: true, Command: "git status --short", CWD: "."}}
}

func codexToolChoice174(value string) pc.Choice {
	return pc.Choice{Protocol: cc.Protocol, Codex: &cc.CodexApprovalChoice{Decision: value}}
}

func codexToolDecide174(t *testing.T, h toolPermissionFixture, permission pc.Permission, choice string) pc.Permission {
	t.Helper()
	decided, err := h.f.s.DecideToolPermission(context.Background(), h.f.actor, permission.ID, pc.DecisionRequest{Choice: codexToolChoice174(choice), ProjectID: h.f.project, ExpectedVersion: permission.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return decided
}

func TestCodexToolPermissionKeepsTypedSourceIdentityAndOnceChoices(t *testing.T) {
	h, binding := newCodexToolPermissionFixture(t, false)
	ctx := context.Background()
	request := codexToolRequest174(binding, "7")
	first, err := h.open(ctx, request)
	if err != nil || first.Request.ACP != nil || first.Request.Codex == nil {
		t.Fatal("native request was absent or converted to ACP", err)
	}
	next := codexToolRequest174(binding, "7")
	next.Codex.RequestID.Kind = "number"
	second, err := h.open(ctx, next)
	if err != nil || second.ID == first.ID {
		t.Fatal("string and numeric RPC identities collapsed", err)
	}
	next = codexToolRequest174(binding, "other-native-request")
	third, err := h.open(ctx, next)
	if err != nil || third.ID == first.ID {
		t.Fatal("different RPC requests for the same item collided", err)
	}
	changed := request
	changed.IdempotencyKey = newID()
	if _, err = h.open(ctx, changed); err == nil {
		t.Fatal("same native RPC acquired another request with a new operation key")
	}
	changed = codexToolRequest174(binding, "different-binding")
	changed.Codex.BindingSHA256 = strings.Repeat("f", 64)
	if _, err = h.open(ctx, changed); err == nil {
		t.Fatal("approval for another fixed Codex binding was admitted")
	}
	for _, denied := range []string{"acceptForSession", "execpolicy_amendment", "network_policy_amendment"} {
		if _, err = h.f.s.DecideToolPermission(ctx, h.f.actor, first.ID, pc.DecisionRequest{Choice: codexToolChoice174(denied), ProjectID: h.f.project, ExpectedVersion: first.Version, IdempotencyKey: newID()}); err == nil {
			t.Fatal("persistent or expanded policy was accepted", denied)
		}
	}
	accepted := codexToolDecide174(t, h, first, "accept")
	if accepted.Decision == nil || accepted.Decision.Codex.Decision != "accept" {
		t.Fatal("exact native accept was lost")
	}
	unsafe := codexToolRequest174(binding, "unsupported-native-scope")
	unsafe.Codex.ScopeSupported, unsafe.Codex.UnsupportedReason = false, "network_context"
	unsafe.Codex.DetailsComplete = false
	p, err := h.open(ctx, unsafe)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.f.s.DecideToolPermission(ctx, h.f.actor, p.ID, pc.DecisionRequest{Choice: codexToolChoice174("accept"), ProjectID: h.f.project, ExpectedVersion: p.Version, IdempotencyKey: newID()}); err == nil {
		t.Fatal("incomplete or unsupported native request was granted")
	}
	codexToolDecide174(t, h, p, "cancel")
	codexPolicy(t, h.f, "project", "enforce", 0)
	codexToolDecide174(t, h, second, "decline")
}

func TestCodexToolPermissionObservationIsMonotonicAndOriginalReceiptIsExact(t *testing.T) {
	h, binding := newCodexToolPermissionFixture(t, false)
	ctx := context.Background()
	request := codexToolRequest174(binding, "observe174")
	p, err := h.open(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	forged := cc.CodexApprovalObservation{Sequence: 1, ParamsSHA256: request.Codex.ParamsSHA256, Decision: "accept", DecisionObserved: true}
	if _, err = h.f.s.ObserveToolPermission(ctx, h.actor, h.f.project, h.assignment, p.ID, newID(), forged); err == nil {
		t.Fatal("machine invented a human decision")
	}
	initial := cc.CodexApprovalObservation{Sequence: 1, ParamsSHA256: request.Codex.ParamsSHA256, ItemStatus: "inProgress"}
	key := newID()
	first, err := h.f.s.ObserveToolPermission(ctx, h.actor, h.f.project, h.assignment, p.ID, key, initial)
	if err != nil || first.Observation == nil || first.Decision != nil {
		t.Fatal("initial machine observation was not a real permission record", err)
	}
	list, err := h.f.s.ListToolPermissions(ctx, h.f.actor, h.f.project, p.RunID, p.NodeID)
	if err != nil || len(list.Items) != 1 || len(list.Items[0].AllowedActions) != 3 {
		t.Fatal("in-progress item incorrectly disabled pending native choices", err)
	}
	decided := codexToolDecide174(t, h, first, "decline")
	latest := cc.CodexApprovalObservation{Sequence: 3, ParamsSHA256: request.Codex.ParamsSHA256, Decision: "decline", DecisionObserved: true, ReplyStarted: true, ItemStatus: "inProgress"}
	if _, err = h.f.s.DB.Exec(`CREATE TRIGGER fail_observation_receipt174 BEFORE INSERT ON suma_operations WHEN NEW.operation_id='observation-receipt-fail174' BEGIN SELECT RAISE(ABORT,'controlled observation receipt failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = h.f.s.ObserveToolPermission(ctx, h.actor, h.f.project, h.assignment, p.ID, "observation-receipt-fail174", latest); err == nil {
		t.Fatal("observation committed without its operation receipt")
	}
	current, err := h.poll(ctx, p.ID)
	if err != nil || current.Version != decided.Version || stringify(current.Observation) != stringify(first.Observation) {
		t.Fatal("failed receipt left a newer observation or version", err)
	}
	if _, err = h.f.s.DB.Exec(`DROP TRIGGER fail_observation_receipt174`); err != nil {
		t.Fatal(err)
	}
	stored, err := h.f.s.ObserveToolPermission(ctx, h.actor, h.f.project, h.assignment, p.ID, newID(), latest)
	if err != nil || stored.Observation.Sequence != 3 || stored.Decision.ActorID != decided.Decision.ActorID {
		t.Fatal("cumulative observation gap or human fact was lost", err)
	}
	replayed, err := h.f.s.ObserveToolPermission(ctx, h.actor, h.f.project, h.assignment, p.ID, key, initial)
	if err != nil || replayed.Version != first.Version || stringify(replayed.Observation) != stringify(first.Observation) || replayed.Decision != nil {
		t.Fatal("old observation receipt mixed a newer read or decision", err)
	}
	changedBody := initial
	changedBody.Sequence = 2
	_, err = h.f.s.ObserveToolPermission(ctx, h.actor, h.f.project, h.assignment, p.ID, key, changedBody)
	var conflict *APIError
	if !errors.As(err, &conflict) || conflict.Code != "idempotency_conflict" {
		t.Fatal("original observation operation accepted a different body", err)
	}
	backwards := latest
	backwards.Sequence = 4
	backwards.ReplyStarted = false
	if _, err = h.f.s.ObserveToolPermission(ctx, h.actor, h.f.project, h.assignment, p.ID, newID(), backwards); err == nil {
		t.Fatal("observation cleared an already-started reply")
	}
	cleanupRequest := codexToolRequest174(binding, "remote-cleanup174")
	cleanup, err := h.open(ctx, cleanupRequest)
	if err != nil {
		t.Fatal(err)
	}
	remote := cc.CodexApprovalObservation{Sequence: 1, ParamsSHA256: cleanupRequest.Codex.ParamsSHA256, RequestResolved: true, ItemStatus: "declined"}
	cleaned, err := h.f.s.ObserveToolPermission(ctx, h.actor, h.f.project, h.assignment, cleanup.ID, newID(), remote)
	if err != nil || cleaned.Decision != nil || cleaned.Observation == nil {
		t.Fatal("no-human remote cleanup fabricated or required a decision", err)
	}
	if _, err = h.f.s.DecideToolPermission(ctx, h.f.actor, cleanup.ID, pc.DecisionRequest{Choice: codexToolChoice174("accept"), ProjectID: h.f.project, ExpectedVersion: cleaned.Version, IdempotencyKey: newID()}); err == nil {
		t.Fatal("resolved native request admitted new approval")
	}
}

func TestCodexToolPermissionPoolObservationKeepsMachineScopeAndFence(t *testing.T) {
	h, binding := newCodexToolPermissionFixture(t, true)
	ctx := context.Background()
	request := codexToolRequest174(binding, "pool-observation174")
	p, err := h.open(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	observation := cc.CodexApprovalObservation{Sequence: 1, ParamsSHA256: request.Codex.ParamsSHA256, ItemStatus: "inProgress"}
	body := poolAssignmentBody(h.assignment)
	body["permission_id"], body["idempotency_key"], body["observation"] = p.ID, newID(), observation
	reply := h.f.bearer("POST", "/pool-runner/tool-permissions/observations", h.token, body, 200)
	var recorded pc.Permission
	if json.Unmarshal([]byte(stringify(reply)), &recorded) != nil || recorded.Observation == nil || recorded.Observation.Sequence != 1 || recorded.ProjectID != h.f.project {
		t.Fatal("pool observation was not stored in its original project")
	}
	wrong := h.assignment
	wrong.Fence++
	if _, err = h.f.s.ObservePoolToolPermission(ctx, h.token, h.f.project, wrong, p.ID, body["idempotency_key"].(string), observation); err == nil {
		t.Fatal("changed pool fence recovered an observation receipt")
	}
	if _, err = h.f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET status='revoked' WHERE id=?`, h.assignment.PoolGrantID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.f.s.ObservePoolToolPermission(ctx, h.token, h.f.project, h.assignment, p.ID, body["idempotency_key"].(string), observation); err == nil {
		t.Fatal("withdrawn machine scope recovered an observation receipt")
	}
	var raw string
	if err = h.f.s.DB.QueryRow(`SELECT payload FROM suma_tool_permissions WHERE id=?`, p.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var unchanged pc.Permission
	if json.Unmarshal([]byte(raw), &unchanged) != nil || stringify(unchanged.Observation) != stringify(observation) || unchanged.Version != recorded.Version {
		t.Fatal("rejected pool source changed the original observation")
	}
}

func TestCodexToolPermissionResultRequiresTheStoredNativeObservation(t *testing.T) {
	h, binding := newCodexToolPermissionFixture(t, false)
	ctx := context.Background()
	request := codexToolRequest174(binding, "result174")
	p, err := h.open(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	p = codexToolDecide174(t, h, p, "decline")
	observation := cc.CodexApprovalObservation{Sequence: 1, ParamsSHA256: request.Codex.ParamsSHA256, Decision: "decline", DecisionObserved: true, ReplyStarted: true, ReplyWritten: true, RequestResolved: true, ItemStatus: "declined"}
	if _, err = h.f.s.ObserveToolPermission(ctx, h.actor, h.f.project, h.assignment, p.ID, newID(), observation); err != nil {
		t.Fatal(err)
	}
	source := request.Codex
	proof := cc.CodexApprovalEvidence{PermissionID: p.ID, RequestID: source.RequestID, Method: source.Method, ThreadID: source.ThreadID, TurnID: source.TurnID, ItemID: source.ItemID, ParamsSHA256: source.ParamsSHA256, Observation: &observation}
	result := dag.Result{RunID: h.assignment.RunID, NodeID: h.assignment.NodeID, Attempt: h.assignment.Attempt, Fence: h.assignment.Fence, WorkerID: h.assignment.WorkerID, OperationID: newID(), ExitCode: 1, Error: "Controlled failure after native decline", ToolEvidence: &tc.Evidence{Profile: h.profile.Profile, Terminal: "failed", SessionID: source.ThreadID, UsageState: tc.UsageUnreported, CodexExecution: &cc.Evidence{BindingSHA256: binding.BindingSHA256, ThreadID: source.ThreadID, TurnID: source.TurnID, RequestedModel: binding.Model, RequestedReasoningEffort: binding.ReasoningEffort, AcknowledgedModel: binding.Model, AcknowledgedReasoningEffort: binding.ReasoningEffort, AcknowledgedProvider: "owned-provider", ConfigurationAcknowledged: true, Approvals: []cc.CodexApprovalEvidence{proof}}}}
	var forged dag.Result
	if err = json.Unmarshal([]byte(stringify(result)), &forged); err != nil {
		t.Fatal(err)
	}
	forged.ToolEvidence.CodexExecution.Approvals[0].Observation.Sequence = 2
	h.f.bearer("POST", "/runner/results", h.token, h.f.body(map[string]any{"result": forged}), 400)
	if err = json.Unmarshal([]byte(stringify(result)), &forged); err != nil {
		t.Fatal(err)
	}
	forged.ToolEvidence.CodexExecution.BindingSHA256 = strings.Repeat("f", 64)
	h.f.bearer("POST", "/runner/results", h.token, h.f.body(map[string]any{"result": forged}), 400)
	tx, err := h.f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	missing := result
	missing.ExitCode, missing.Error, missing.ToolEvidence = 0, "", nil
	err = h.f.s.validateCodexApprovalResultTx(ctx, tx, h.f.user.TenantID, h.f.project, missing)
	tx.Rollback()
	if err == nil {
		t.Fatal("successful result omitted an actual persisted native request")
	}
	body := h.f.body(map[string]any{"result": result})
	h.f.bearer("POST", "/runner/results", h.token, body, 200)
	h.f.bearer("POST", "/runner/results", h.token, body, 200)
	var count int
	var raw string
	if err = h.f.s.DB.QueryRow(`SELECT COUNT(*),MIN(payload) FROM suma_usage_facts WHERE kind='result'`).Scan(&count, &raw); err != nil || count != 1 {
		t.Fatal("original native completion receipt was duplicated", count, err)
	}
	var stored dag.Result
	if json.Unmarshal([]byte(raw), &stored) != nil || stored.ToolEvidence == nil || stored.ToolEvidence.CodexExecution == nil || len(stored.ToolEvidence.CodexExecution.Approvals) != 1 || stringify(stored.ToolEvidence.CodexExecution.Approvals[0].Observation) != stringify(observation) {
		t.Fatal("native result omitted its actual stored observation")
	}
}

func TestCodexToolPermissionHTTPSeparatesEnvelopeAndTypedRequestLimits(t *testing.T) {
	h, binding := newCodexToolPermissionFixture(t, false, strings.Repeat("fixed-goal-", 800))
	request := codexToolRequest174(binding, "large-envelope174")
	request.Codex.Details, request.Codex.Command = strings.Repeat("d", 30000), strings.Repeat("c", 30000)
	body := ToolPermissionOpenRequest{ProjectID: h.f.project, Assignment: h.assignment, Request: request}
	raw := stringify(body)
	if len(raw) <= 64<<10 || len(raw) >= 1<<20 || len(stringify(request)) > 64<<10 {
		t.Fatal("fixture does not isolate the envelope from the typed request cap")
	}
	h.f.bearer("POST", "/runner/tool-permissions", h.token, body, 200)
	serve := func(raw string) int {
		t.Helper()
		r := httptest.NewRequest("POST", "http://suma.test/api/v1/runner/tool-permissions", strings.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+h.token)
		w := httptest.NewRecorder()
		h.f.handler.ServeHTTP(w, r)
		return w.Code
	}
	if serve(raw+strings.Repeat(" ", 1<<20)) != 400 {
		t.Fatal("transport envelope exceeded 1 MiB")
	}
	if serve(strings.Replace(raw, `"details_complete":true,`, "", 1)) != 400 {
		t.Fatal("missing native boolean was inferred as false")
	}
	request.IdempotencyKey = newID()
	request.Codex.RequestID.Value = "over-detail174"
	request.Codex.Details = strings.Repeat("d", (32<<10)+1)
	body.Request = request
	h.f.bearer("POST", "/runner/tool-permissions", h.token, body, 400)
}
