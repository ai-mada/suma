package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestExpertCodexSelectionPublicRequestRoundTrip(t *testing.T) {
	// Losing this public field silently would turn an explicit expert choice into
	// a missing selection at the unified dispatch boundary.
	raw := `{"task_id":"task-one","expected_task_version":1,"role_key":"developer","codex_execution_selection":{"catalog_sha256":"` + strings.Repeat("a", 64) + `","model":"codex-a","reasoning_effort":"high","token_notice_limit":7}}`
	var item ExpertTeamPlanItemRequest
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(item)
	if err != nil || !strings.Contains(string(encoded), `"codex_execution_selection"`) || !strings.Contains(string(encoded), `"token_notice_limit":7`) {
		t.Fatal("explicit Codex selection was lost from expert request", string(encoded), err)
	}
}

func expertCodexFixture(t *testing.T, dependent bool) (*expertFixture, ToolProfile, cc.Catalog) {
	t.Helper()
	h := newExpertFixture(t, false)
	f, ctx := h.f, context.Background()
	profile := tc.Profile{CommandID: "expert-codex", Adapter: "codex", Protocol: cc.Protocol, ProfileSHA256: strings.Repeat("c", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "Codex domain boundary fixture"}
	p, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "tool-worker", Name: "Explicit expert Codex", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := cc.NewCatalog(strings.Repeat("d", 64), []cc.ModelOption{{Model: "codex-a", ReasoningEfforts: []string{"low", "high"}, DefaultReasoningEffort: "low"}, {Model: "codex-b", ReasoningEfforts: []string{"medium"}, DefaultReasoningEffort: "medium"}})
	if err != nil {
		t.Fatal(err)
	}
	reportCodexCatalog(t, f, h.runnerToken, p, catalog)
	p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.role.ToolProfileID, h.role.ExpectedToolVersion = p.ID, p.Version
	h.configuration, err = f.s.SaveExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamSaveRequest{ProjectID: f.project, ExpectedVersion: h.configuration.Version, Roles: []ExpertTeamRoleRequest{h.role}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.configuration, err = f.s.CommandExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamCommandRequest{ProjectID: f.project, ExpectedVersion: h.configuration.Version, Action: "publish", Reason: "Approve tool; each task chooses its own Codex configuration", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if dependent {
		h.tasks[1], err = f.s.Update(ctx, f.actor, f.project, "tasks", h.tasks[1].ID, f.body(map[string]any{"expected_version": h.tasks[1].Version, "depends_on": []any{h.tasks[0].ID}}))
		if err != nil {
			t.Fatal(err)
		}
	}
	f.e.AttemptAccounting = f.s
	return h, p, catalog
}

func expertCodexRequest(h *expertFixture, catalog cc.Catalog) ExpertTeamPlanCreateRequest {
	request := h.request()
	request.Items[0].CodexExecutionSelection = &cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "codex-a", ReasoningEffort: "high", TokenNoticeLimit: usageInt(1)}
	request.Items[1].CodexExecutionSelection = &cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "codex-b", ReasoningEffort: "medium", TokenNoticeLimit: usageInt(19)}
	return request
}

func expertCodexAssertBinding(t *testing.T, item ExpertTeamPlanItem, model, effort string, notice int64) {
	t.Helper()
	b := item.Plan.CodexExecution
	if b == nil || b.Model != model || b.ReasoningEffort != effort || b.TokenNoticeLimit == nil || *b.TokenNoticeLimit != notice || b.BudgetMode != "observe_only" || b.IdentityEvidence != "configuration_ack_only" || item.Plan.ModelExecution != nil {
		t.Fatal("expert item changed Codex configuration or invented a native budget", b)
	}
	n := codexPlanNode(t, item.Plan)
	bound, err := dag.CodexBinding(n)
	if err != nil || stringify(bound) != stringify(b) || n.MaxRetries != 0 {
		t.Fatal("workflow differs from fixed Codex binding", err)
	}
}

func TestExpertCodexPerTaskBindingStartClaimAndOriginalReceipts(t *testing.T) {
	h, profile, catalog := expertCodexFixture(t, false)
	f, ctx := h.f, context.Background()
	capacity := schedulingPreview(f, 2, nil)
	f.call("POST", "/scheduling/policy-previews/"+capacity["id"].(string)+"/commit", f.body(map[string]any{"expected_version": 1}), 200)
	codexPolicy(t, f, "project", "observe", 0)
	request := expertCodexRequest(h, catalog)
	preview, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	expertCodexAssertBinding(t, preview.Items[0], "codex-a", "high", 1)
	expertCodexAssertBinding(t, preview.Items[1], "codex-b", "medium", 19)
	if expertCount(t, f, "dag_runs") != 0 || expertCount(t, f, "suma_usage_attempts") != 0 {
		t.Fatal("preview started work or reserved a native budget")
	}
	start := h.start(preview)
	started, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, start)
	if err != nil {
		t.Fatal(err)
	}
	byRun := map[string]*cc.Binding{}
	for _, item := range started.Items {
		byRun[item.RunID] = item.Plan.CodexExecution
	}
	claimed := map[string]bool{}
	for poll := 0; poll < 6 && len(claimed) < 2; poll++ {
		wire := f.bearer("POST", "/runner/claim", h.runnerToken, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}}), 200)
		if wire["assignment"] == nil {
			continue
		}
		var a dag.Assignment
		if json.Unmarshal([]byte(stringify(wire["assignment"])), &a) != nil || byRun[a.RunID] == nil {
			t.Fatal("unexpected expert claim", wire)
		}
		binding, err := dag.CodexBinding(dag.Node{Type: "agent", Command: a.Command, Config: a.Config})
		if err != nil || stringify(binding) != stringify(byRun[a.RunID]) {
			t.Fatal("claim replaced reviewed Codex binding", err)
		}
		claimed[a.RunID] = true
	}
	if len(claimed) != 2 {
		t.Fatal("both independent tasks must claim", claimed)
	}
	ledger, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 10, "")
	if err != nil || len(ledger.Items) != 2 {
		t.Fatal(ledger, err)
	}
	for _, item := range ledger.Items {
		if stringify(item.CodexExecution) != stringify(byRun[item.RunID]) || item.Rate != nil || item.ReservedTokens != 0 || item.EstimatedCostMicros != nil || item.Model != "" {
			t.Fatal("Codex notice became native identity, price or reservation", item)
		}
	}
	paused, err := f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, expertPlanCommand(h, started, "pause"))
	if err != nil {
		t.Fatal(err)
	}
	replay, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, start)
	if err != nil {
		t.Fatal(err)
	}
	expertAssertOriginalPlanReceipt(t, replay, started)
	for i := range replay.Items {
		if stringify(replay.Items[i].Plan.CodexExecution) != stringify(started.Items[i].Plan.CodexExecution) {
			t.Fatal("Start receipt replaced Codex binding")
		}
	}
	created, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	expertAssertOriginalPlanReceipt(t, created, preview)
	if paused.Version <= replay.Version || expertCount(t, f, "dag_runs") != 2 {
		t.Fatal("recovery changed current plan or duplicated runs")
	}
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	if _, err = f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, request); err == nil {
		t.Fatal("Codex create receipt bypassed revoked credential")
	}
	if _, err = f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, start); err == nil {
		t.Fatal("Codex Start receipt bypassed revoked credential")
	}
}

func TestExpertCodexRejectsMissingDualAndUnsupportedSelectionsAtomically(t *testing.T) {
	h, _, catalog := expertCodexFixture(t, false)
	f, ctx := h.f, context.Background()
	for _, mode := range []string{"missing", "dual", "stale", "effort", "notice"} {
		request := expertCodexRequest(h, catalog)
		switch mode {
		case "missing":
			request.Items[1].CodexExecutionSelection = nil
		case "dual":
			request.Items[1].ModelExecutionSelection = &mc.Selection{}
		case "stale":
			request.Items[1].CodexExecutionSelection.CatalogSHA256 = strings.Repeat("e", 64)
		case "effort":
			request.Items[1].CodexExecutionSelection.ReasoningEffort = "high"
		case "notice":
			request.Items[1].CodexExecutionSelection.TokenNoticeLimit = usageInt(0)
		}
		if _, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, request); err == nil {
			t.Fatal("invalid expert Codex selection accepted", mode)
		}
		if expertCount(t, f, "suma_expert_team_plans") != 0 || expertCount(t, f, "suma_expert_team_plan_items") != 0 || expertCount(t, f, "dag_runs") != 0 {
			t.Fatal("rejected batch partially persisted", mode)
		}
	}
	ordinary := newExpertFixture(t, false)
	if _, err := ordinary.f.s.CreateExpertTeamPlan(ctx, ordinary.f.e, ordinary.f.actor, ordinary.f.project, expertCodexRequest(ordinary, catalog)); err == nil {
		t.Fatal("ordinary tool accepted independent Codex settings")
	}
}

func TestExpertCodexCurrentCatalogConfigurationAndPolicyGateReadStartClaim(t *testing.T) {
	for _, mode := range []string{"configuration", "withdrawn", "enforce"} {
		t.Run(mode, func(t *testing.T) {
			h, profile, catalog := expertCodexFixture(t, false)
			f, ctx := h.f, context.Background()
			request := expertCodexRequest(h, catalog)
			preview, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, request)
			if err != nil {
				t.Fatal(err)
			}
			var policy UsagePolicy
			switch mode {
			case "configuration":
				catalog.ConfigurationSHA256 = strings.Repeat("f", 64)
				catalog.CatalogSHA256 = cc.CatalogDigest(catalog)
				reportCodexCatalog(t, f, h.runnerToken, profile, catalog)
			case "withdrawn":
				catalog, err = cc.NewCatalog(catalog.ConfigurationSHA256, catalog.Models[:1])
				if err != nil {
					t.Fatal(err)
				}
				reportCodexCatalog(t, f, h.runnerToken, profile, catalog)
			case "enforce":
				policy = codexPolicy(t, f, "organization", "enforce", 0)
			}
			view, err := f.s.GetExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID)
			if err != nil || view.Valid || view.CanStart {
				t.Fatal("stale Codex expert preview stayed eligible", view.Blockers, err)
			}
			expertCodexAssertBinding(t, view.Items[1], "codex-b", "medium", 19)
			if _, err = f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, h.start(preview)); err == nil || expertCount(t, f, "dag_runs") != 0 {
				t.Fatal("Codex drift partially started expert batch", err)
			}
			// A fresh source needs explicit review. Restoring the original catalog
			// permits the original Start, which still does not waive Claim gates.
			if mode == "enforce" {
				codexPolicy(t, f, "organization", "observe", policy.Version)
			}
			original, err := cc.NewCatalog(strings.Repeat("d", 64), []cc.ModelOption{{Model: "codex-a", ReasoningEfforts: []string{"low", "high"}, DefaultReasoningEffort: "low"}, {Model: "codex-b", ReasoningEfforts: []string{"medium"}, DefaultReasoningEffort: "medium"}})
			if err != nil {
				t.Fatal(err)
			}
			reportCodexCatalog(t, f, h.runnerToken, profile, original)
			started, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, h.start(preview))
			if err != nil {
				t.Fatal(err)
			}
			if mode == "enforce" {
				codexPolicy(t, f, "project", "enforce", 0)
			} else {
				original.ConfigurationSHA256 = strings.Repeat("e", 64)
				original.CatalogSHA256 = cc.CatalogDigest(original)
				reportCodexCatalog(t, f, h.runnerToken, profile, original)
			}
			claim := f.bearer("POST", "/runner/claim", h.runnerToken, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}}), 200)
			if claim["assignment"] != nil || expertCount(t, f, "suma_usage_attempts") != 0 || expertCount(t, f, "dag_runs") != 2 {
				t.Fatal("current Codex gate allowed lease/accounting or changed runs", claim)
			}
			expertCodexAssertBinding(t, started.Items[0], "codex-a", "high", 1)
		})
	}
}

func expertCodexCompleteRoot(t *testing.T, h *expertFixture, p ExpertTeamPlan, profile ToolProfile) {
	t.Helper()
	f, ctx := h.f, context.Background()
	for range 2 {
		wire := f.bearer("POST", "/runner/claim", h.runnerToken, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID, "command:independent-tests"}}), 200)
		var a dag.Assignment
		if json.Unmarshal([]byte(stringify(wire["assignment"])), &a) != nil || a.RunID != p.Items[0].RunID {
			t.Fatal("missing original Codex root assignment", wire)
		}
		proof := reviewWorkspaceEvidence("c", a.Type == "test")
		r := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), WorkspaceEvidence: proof}
		if a.Type == "agent" {
			b := p.Items[0].Plan.CodexExecution
			r.ToolEvidence = &tc.Evidence{Profile: profile.Profile, SessionID: "thread-original", Terminal: "completed", EventCount: 4, UsageState: tc.UsageUnreported, CodexExecution: &cc.Evidence{BindingSHA256: b.BindingSHA256, ThreadID: "thread-original", TurnID: "turn-original", RequestedModel: b.Model, RequestedReasoningEffort: b.ReasoningEffort, AcknowledgedModel: b.Model, AcknowledgedReasoningEffort: b.ReasoningEffort, AcknowledgedProvider: "configured-provider", ConfigurationAcknowledged: true}}
		} else {
			proof["base_commit"], proof["head_commit"] = a.CandidateCommit, a.CandidateCommit
			proof["candidate_from"], proof["candidate_attempt"], proof["candidate_fence"], proof["candidate_evidence_sha256"] = a.CandidateFrom, a.CandidateAttempt, a.CandidateFence, a.CandidateEvidenceSHA256
		}
		if err := f.e.Complete(ctx, f.user.TenantID, f.project, r); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExpertCodexPendingRevisionHistoryAndWaitingDispatchPreserveBindings(t *testing.T) {
	h, profile, catalog := expertCodexFixture(t, true)
	f, ctx := h.f, context.Background()
	create := expertCodexRequest(h, catalog)
	preview, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, create)
	if err != nil {
		t.Fatal(err)
	}
	start := h.start(preview)
	started, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, start)
	if err != nil || started.Items[1].RunID != "" || expertCount(t, f, "dag_runs") != 1 {
		t.Fatal("waiting Codex task dispatched early", err)
	}
	paused, err := f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, expertPlanCommand(h, started, "pause"))
	if err != nil {
		t.Fatal(err)
	}
	revise := expertPlanCommand(h, paused, "revise")
	revise.ExpectedConfigurationVersion = h.configuration.Version
	choice := &cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "codex-a", ReasoningEffort: "low", TokenNoticeLimit: usageInt(31)}
	revise.Items = []ExpertTeamPlanItemRequest{{TaskID: h.tasks[1].ID, ExpectedTaskVersion: h.tasks[1].Version, RoleKey: h.role.Key, CodexExecutionSelection: choice}}
	revised, err := f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, revise)
	if err != nil || revised.Revision != 2 || revised.Items[0].RunID != started.Items[0].RunID {
		t.Fatal("pending revision changed started work", err)
	}
	expertCodexAssertBinding(t, revised.Items[0], "codex-a", "high", 1)
	expertCodexAssertBinding(t, revised.Items[1], "codex-a", "low", 31)
	invalid := expertPlanCommand(h, revised, "revise")
	invalid.ExpectedConfigurationVersion = h.configuration.Version
	invalid.Items = []ExpertTeamPlanItemRequest{{TaskID: h.tasks[0].ID, ExpectedTaskVersion: h.tasks[0].Version + 1, RoleKey: h.role.Key, CodexExecutionSelection: choice}}
	if _, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, invalid); err == nil {
		t.Fatal("started Codex item was revised")
	}
	original, err := f.s.GetExpertTeamPlanRevision(ctx, f.e, f.actor, f.project, preview.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	expertCodexAssertBinding(t, original.Items[1], "codex-b", "medium", 19)
	resumed, err := f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, expertPlanCommand(h, revised, "resume"))
	if err != nil {
		t.Fatal(err)
	}
	expertCodexCompleteRoot(t, h, resumed, profile)
	orchestrationDue(t, h)
	if err = f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
		t.Fatal(err)
	}
	if expertCount(t, f, "dag_runs") != 1 {
		t.Fatal("run success accepted predecessor implicitly")
	}
	orchestrationAcceptRoot(t, h, resumed)
	orchestrationDue(t, h)
	if err = f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
		t.Fatal(err)
	}
	current := orchestrationCurrent(t, h, preview.ID)
	if current.Items[1].RunID == "" || expertCount(t, f, "dag_runs") != 2 {
		t.Fatal("accepted predecessor did not dispatch frozen Codex item", current.Blockers)
	}
	expertCodexAssertBinding(t, current.Items[1], "codex-a", "low", 31)
	for _, operation := range []string{"create", "revise", "start"} {
		var replay, fixed ExpertTeamPlan
		switch operation {
		case "create":
			replay, err = f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, create)
			fixed = preview
		case "revise":
			replay, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, revise)
			fixed = revised
		case "start":
			replay, err = f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, start)
			fixed = started
		}
		if err != nil {
			t.Fatal(operation, err)
		}
		expertAssertOriginalPlanReceipt(t, replay, fixed)
		for i := range replay.Items {
			if stringify(replay.Items[i].Plan.CodexExecution) != stringify(fixed.Items[i].Plan.CodexExecution) {
				t.Fatal("recovery replaced original Codex source", operation)
			}
		}
	}
}

func TestExpertCodexHTTPAcceptsPublicSelectionAndRejectsPrivateFields(t *testing.T) {
	h, _, catalog := expertCodexFixture(t, false)
	f := h.f
	for _, field := range []string{"api_key", "configuration_sha256", "binding_sha256", "provider", "budget_mode", "max_output_tokens"} {
		var body map[string]any
		if err := json.Unmarshal([]byte(stringify(expertCodexRequest(h, catalog))), &body); err != nil {
			t.Fatal(err)
		}
		selection := body["items"].([]any)[0].(map[string]any)["codex_execution_selection"].(map[string]any)
		selection[field] = "private-value"
		f.call("POST", "/expert-team-plans", body, 400)
	}
	if expertCount(t, f, "suma_expert_team_plans") != 0 {
		t.Fatal("private selection fields persisted a plan")
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(stringify(expertCodexRequest(h, catalog))), &body); err != nil {
		t.Fatal(err)
	}
	wire := f.call("POST", "/expert-team-plans", body, 201)
	var preview ExpertTeamPlan
	if err := json.Unmarshal([]byte(stringify(wire)), &preview); err != nil {
		t.Fatal(err)
	}
	expertCodexAssertBinding(t, preview.Items[0], "codex-a", "high", 1)
	expertCodexAssertBinding(t, preview.Items[1], "codex-b", "medium", 19)
}

func TestExpertCodexHTTPRejectsExplicitNullAndDualSelectionOnFixedTools(t *testing.T) {
	h := newExpertFixture(t, false)
	f := h.f
	preview := h.preview(t)
	for _, operation := range []string{"create", "revise"} {
		for _, fields := range []map[string]any{{"codex_execution_selection": nil}, {"model_execution_selection": nil}, {"model_execution_selection": nil, "codex_execution_selection": nil}} {
			item := map[string]any{"task_id": h.tasks[0].ID, "expected_task_version": h.tasks[0].Version, "role_key": h.role.Key}
			for key, value := range fields {
				item[key] = value
			}
			body := f.body(map[string]any{"team_id": h.team, "expected_configuration_version": h.configuration.Version, "items": []any{item}})
			path := "/expert-team-plans"
			if operation == "revise" {
				delete(body, "team_id")
				body["expected_version"], body["action"], body["reason"], body["acknowledge"] = preview.Version, "revise", "Reject ambiguous selection", true
				path += "/" + preview.ID + "/commands"
			}
			f.call("POST", path, body, 400)
		}
	}
	if expertCount(t, f, "suma_expert_team_plans") != 1 || expertCount(t, f, "suma_expert_team_plan_revisions") != 1 || expertCount(t, f, "dag_runs") != 0 {
		t.Fatal("ambiguous input changed fixed-tool plan")
	}
}

func TestExpertCodexWaitingDispatchRechecksCurrentSourceWithoutReplacingBinding(t *testing.T) {
	for _, mode := range []string{"configuration", "enforce", "credential"} {
		t.Run(mode, func(t *testing.T) {
			h, profile, catalog := expertCodexFixture(t, true)
			f, ctx := h.f, context.Background()
			preview, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, expertCodexRequest(h, catalog))
			if err != nil {
				t.Fatal(err)
			}
			started, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, h.start(preview))
			if err != nil {
				t.Fatal(err)
			}
			expertCodexCompleteRoot(t, h, started, profile)
			orchestrationAcceptRoot(t, h, started)
			var policy UsagePolicy
			switch mode {
			case "configuration":
				changed := cc.CloneCatalog(catalog)
				changed.ConfigurationSHA256 = strings.Repeat("e", 64)
				changed.CatalogSHA256 = cc.CatalogDigest(changed)
				reportCodexCatalog(t, f, h.runnerToken, profile, changed)
			case "enforce":
				policy = codexPolicy(t, f, "project", "enforce", 0)
			case "credential":
				if _, err = f.s.DB.Exec(`UPDATE suma_sessions SET expires_at='2000-01-01T00:00:00Z' WHERE token_hash=?`, f.actor.credential.digest); err != nil {
					t.Fatal(err)
				}
			}
			orchestrationDue(t, h)
			if err = f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
				t.Fatal(err)
			}
			if expertCount(t, f, "dag_runs") != 1 {
				t.Fatal("waiting Codex item bypassed current source or activation authority")
			}
			// Inspect immutable source through the database for the expired-session
			// case; possession of an old Actor cannot read or dispatch that source.
			var raw string
			if err = f.s.DB.QueryRow(`SELECT payload FROM suma_expert_team_plans WHERE id=?`, preview.ID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var current ExpertTeamPlan
			if err = json.Unmarshal([]byte(raw), &current); err != nil {
				t.Fatal(err)
			}
			expertCodexAssertBinding(t, current.Items[1], "codex-b", "medium", 19)
			if current.Items[1].RunID != "" {
				t.Fatal("blocked waiting item acquired a run")
			}
			if mode == "credential" {
				if _, err = f.s.GetExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID); err == nil {
					t.Fatal("revoked credential retained Codex source access")
				}
				return
			}
			if mode == "enforce" {
				codexPolicy(t, f, "project", "observe", policy.Version)
			} else {
				reportCodexCatalog(t, f, h.runnerToken, profile, catalog)
			}
			orchestrationDue(t, h)
			if err = f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
				t.Fatal(err)
			}
			current = orchestrationCurrent(t, h, preview.ID)
			if current.Items[1].RunID == "" || expertCount(t, f, "dag_runs") != 2 {
				t.Fatal("restored current authority failed to dispatch original source", current.Blockers)
			}
			expertCodexAssertBinding(t, current.Items[1], "codex-b", "medium", 19)
		})
	}
}
