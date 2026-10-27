package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func codexExecutionFixture(t *testing.T) (*fixture, string, TaskDispatchPreviewRequest, ToolProfile, cc.Catalog) {
	t.Helper()
	f, token, request, _ := taskDispatchFixture(t)
	ctx := context.Background()
	profile := tc.Profile{CommandID: "codex-app", Adapter: "codex", Protocol: cc.Protocol, ProfileSHA256: strings.Repeat("c", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "codex-cli domain fixture"}
	p, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "tool-worker", Name: "Explicit Codex app-server", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := cc.NewCatalog(strings.Repeat("d", 64), []cc.ModelOption{{Model: "codex-a", ReasoningEfforts: []string{"low", "high"}, DefaultReasoningEffort: "low"}, {Model: "codex-b", ReasoningEfforts: []string{"medium"}, DefaultReasoningEffort: "medium"}})
	if err != nil {
		t.Fatal(err)
	}
	reportCodexCatalog(t, f, token, p, catalog)
	p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	var req TaskDispatchPreviewRequest
	if err = json.Unmarshal([]byte(stringify(request)), &req); err != nil {
		t.Fatal(err)
	}
	req.ToolProfileID, req.ExpectedToolVersion = p.ID, p.Version
	req.CodexExecutionSelection = &cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "codex-a", ReasoningEffort: "high", TokenNoticeLimit: usageInt(1)}
	f.e.AttemptAccounting = f.s
	return f, token, req, p, catalog
}

func reportCodexCatalog(t *testing.T, f *fixture, token string, p ToolProfile, c cc.Catalog) {
	t.Helper()
	_, err := f.s.ReportToolInventory(context.Background(), toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true, CodexCatalog: &c}}})
	if err != nil {
		t.Fatal(err)
	}
}

func codexPolicy(t *testing.T, f *fixture, scope, mode string, version int) UsagePolicy {
	t.Helper()
	p, err := f.s.WriteUsagePolicy(context.Background(), f.actor, f.project, UsagePolicyWriteRequest{ProjectID: f.project, Scope: scope, Mode: mode, ExpectedVersion: version, Currency: "CNY", TokenLimit: usageInt(1), CostLimitMicros: usageInt(0), AcknowledgeEnforcement: mode == "enforce", Reason: "Explicit policy capability boundary", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func codexPlanNode(t *testing.T, plan TaskDispatchPlan) dag.Node {
	t.Helper()
	for _, n := range plan.Workflow.Nodes {
		if n.ID == plan.AgentNodeID {
			return n
		}
	}
	t.Fatal("missing fixed development node")
	return dag.Node{}
}

func TestCodexExecutionControlFreezesSelectionAndObservesUnpricedUsage(t *testing.T) {
	f, token, req, p, catalog := codexExecutionFixture(t)
	ctx := context.Background()
	// Observe-mode thresholds and this one-token notice do not become hard caps.
	codexPolicy(t, f, "project", "observe", 0)
	options, err := f.s.GetModelExecutionOptions(ctx, f.actor, f.project, ModelExecutionOptionsRequest{ProjectID: f.project, ToolProfileID: p.ID, ExpectedToolVersion: p.Version})
	if err != nil || options.Supported || !options.CodexSupported || options.CodexCatalog == nil || options.CodexCatalog.CatalogSHA256 != catalog.CatalogSHA256 || len(options.Items) != 0 {
		t.Fatal("Codex catalog was absent or presented as native execution", options, err)
	}
	tools, err := f.s.GetTaskDispatchOptions(ctx, f.e, f.actor, f.project, req.TaskID, TaskDispatchOptionsRequest{ProjectID: f.project, TaskID: req.TaskID, ExpectedTaskVersion: req.ExpectedTaskVersion, WorkflowID: req.WorkflowID, WorkflowVersion: req.WorkflowVersion, AgentNodeID: req.AgentNodeID})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range tools.Items {
		found = found || item.Tool.ID == p.ID && item.Eligible
	}
	if !found {
		t.Fatal("Codex app-server was filtered out of the ordinary task entry")
	}
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	n := codexPlanNode(t, plan)
	bound, decodeErr := cc.DecodeBinding(n.Config["codex_execution"])
	if plan.ModelExecution != nil || plan.CodexExecution == nil || plan.CodexExecution.Model != req.CodexExecutionSelection.Model || plan.CodexExecution.ReasoningEffort != "high" || plan.CodexExecution.ConfigurationSHA256 != catalog.ConfigurationSHA256 || plan.CodexExecution.BudgetMode != cc.BudgetMode || plan.CodexExecution.IdentityEvidence != cc.IdentityEvidence || n.MaxRetries != 0 || decodeErr != nil || stringify(bound) != stringify(plan.CodexExecution) {
		t.Fatal("explicit Codex selection was lost or treated as native", plan.CodexExecution)
	}
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	claimed := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(claimed["assignment"])), &a) != nil || a.RunID != run.ID {
		t.Fatal("exact Codex attempt did not claim", claimed)
	}
	bound, decodeErr = cc.DecodeBinding(a.Config["codex_execution"])
	if decodeErr != nil || stringify(bound) != stringify(plan.CodexExecution) {
		t.Fatal("claim changed the Codex binding", decodeErr)
	}
	ref := dag.AccountingAttempt{TenantID: f.user.TenantID, ProjectID: f.project, RunID: a.RunID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence, Spec: n}
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence, OperationID: newID(), ExitCode: 1, ToolEvidence: &tc.Evidence{Profile: p.Profile, Usage: &tc.Usage{InputTokens: usageInt(23), OutputTokens: usageInt(17)}, UsageState: tc.UsageComplete}}
	tx, err := f.s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = f.s.RecordResultTx(ctx, tx, ref, result); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	page, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	usage := page.Items[0]
	if usage.Rate != nil || usage.EstimatedCostMicros != nil || usage.ReservedTokens != 0 || usage.ReportedTokens == nil || *usage.ReportedTokens != 40 || usage.State != "reconciling" || usage.UsageState != tc.UsagePartial || usage.Model != "" || stringify(usage.CodexExecution) != stringify(plan.CodexExecution) {
		t.Fatal("configuration acknowledgement became a price or complete usage proof", usage)
	}
}

func TestCodexExecutionControlEnforceBlocksPreviewStartAndClaim(t *testing.T) {
	f, token, req, p, _ := codexExecutionFixture(t)
	ctx := context.Background()
	org := codexPolicy(t, f, "organization", "enforce", 0)
	if _, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req); err == nil {
		t.Fatal("enforced organization budget silently became observe-only")
	}
	org = codexPolicy(t, f, "organization", "observe", org.Version)
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	project := codexPolicy(t, f, "project", "enforce", 0)
	start := TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}
	if _, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, start); err == nil || expertCount(t, f, "dag_runs") != 0 {
		t.Fatal("policy drift did not block Start", err)
	}
	codexPolicy(t, f, "project", "observe", project.Version)
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, start)
	if err != nil {
		t.Fatal(err)
	}
	codexPolicy(t, f, "organization", "enforce", org.Version)
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
	if claim["assignment"] != nil || expertCount(t, f, "suma_usage_attempts") != 0 {
		t.Fatal("enforced Codex attempt acquired a lease or accounting lane", claim)
	}
	tx, err := f.s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ref := dag.AccountingAttempt{TenantID: f.user.TenantID, ProjectID: f.project, RunID: run.ID, NodeID: plan.AgentNodeID, WorkerID: p.WorkerID, Attempt: 1, Fence: 1, Spec: codexPlanNode(t, plan)}
	allowed, err := f.s.CheckAttemptTx(ctx, tx, ref)
	if err != nil || allowed {
		t.Fatal("accounting Check bypassed observe-only capability", allowed, err)
	}
	if err = f.s.ReserveAttemptTx(ctx, tx, ref); err == nil {
		t.Fatal("accounting Reserve bypassed observe-only capability")
	}
}

func TestCodexExecutionControlRejectsFallbackDriftAndAutomaticRetry(t *testing.T) {
	f, token, req, p, catalog := codexExecutionFixture(t)
	ctx := context.Background()
	for _, kind := range []string{"missing", "native", "stale"} {
		bad := req
		bad.IdempotencyKey = newID()
		switch kind {
		case "missing":
			bad.CodexExecutionSelection = nil
		case "native":
			bad.ModelExecutionSelection = &modelcontract.Selection{}
		case "stale":
			selection := *req.CodexExecutionSelection
			selection.CatalogSHA256 = strings.Repeat("e", 64)
			bad.CodexExecutionSelection = &selection
		}
		if _, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, bad); err == nil {
			t.Fatal("unsupported selection silently fell back", kind)
		}
	}
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	n := codexPlanNode(t, plan)
	n.MaxRetries, n.Idempotent = 1, true
	if err = dag.Compile(dag.Workflow{Nodes: []dag.Node{n}}); err == nil {
		t.Fatal("bound Codex turn inherited automatic retry")
	}
	n.MaxRetries = 0
	n.Config["model_execution"] = map[string]any{}
	if err = dag.Compile(dag.Workflow{Nodes: []dag.Node{n}}); err == nil {
		t.Fatal("Codex/native simultaneous bindings compiled")
	}
	delete(n.Config, "model_execution")
	catalog.ConfigurationSHA256 = strings.Repeat("f", 64)
	catalog.CatalogSHA256 = cc.CatalogDigest(catalog)
	reportCodexCatalog(t, f, token, p, catalog)
	view, err := f.s.GetTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID)
	if err != nil || view.Valid {
		t.Fatal("changed local configuration remained a valid preview", err)
	}
	if _, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}); err == nil || expertCount(t, f, "dag_runs") != 0 {
		t.Fatal("stale local configuration started or fell back", err)
	}
	// A valid Start is still not permission to claim a model subsequently
	// withdrawn from the current exact worker catalog.
	req.IdempotencyKey = newID()
	req.CodexExecutionSelection.CatalogSHA256 = catalog.CatalogSHA256
	currentPlan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, currentPlan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: currentPlan.Version, IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	catalog, err = cc.NewCatalog(catalog.ConfigurationSHA256, catalog.Models[1:])
	if err != nil {
		t.Fatal(err)
	}
	reportCodexCatalog(t, f, token, p, catalog)
	tx, err := f.s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	eligible, err := f.s.modelExecutionClaimEligibleTx(ctx, tx, f.user.TenantID, f.project, p, codexPlanNode(t, currentPlan))
	tx.Rollback()
	if err != nil || eligible {
		t.Fatal("withdrawn model was eligible for a new attempt", eligible, err)
	}
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
	if claim["assignment"] != nil || expertCount(t, f, "suma_usage_attempts") != 0 {
		t.Fatal("withdrawn model acquired an execution/accounting lease", claim)
	}
}
