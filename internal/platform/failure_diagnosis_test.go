package platform

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

type diagnosisFixture struct {
	f       *fixture
	token   string
	profile ToolProfile
	context FailureDiagnosisContext
	request FailureDiagnosisCreateRequest
	output  string
}

func TestFailureDiagnosisSourceDriftBeforeAndAfterClaim(t *testing.T) {
	for _, phase := range []string{"before-claim", "after-claim"} {
		t.Run(phase, func(t *testing.T) {
			h := newDiagnosisFixture(t)
			f, ctx := h.f, context.Background()
			a := h.create(t)
			var assigned dag.Assignment
			if phase == "after-claim" {
				assigned = h.claim(t)
			}
			_, e := f.s.Update(ctx, f.actor, f.project, "issues", h.request.IssueID, f.body(map[string]any{"expected_version": h.request.ExpectedIssueVersion, "description": "Human correction of the fixed failure issue"}))
			if e != nil {
				t.Fatal(e)
			}
			if phase == "before-claim" {
				reply := f.bearer("POST", "/runner/claim", h.token, f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}, "idempotency_key": newID()}), 200)
				if reply["assignment"] != nil {
					t.Fatal("changed source was claimable", reply)
				}
				if expertCount(t, f, "suma_failure_diagnosis_executions") != 0 {
					t.Fatal("unstarted source drift fabricated a result")
				}
			} else {
				f.bearer("POST", "/runner/heartbeat", h.token, f.body(map[string]any{"assignment": assigned}), 200)
				h.complete(t, h.result(assigned, h.output))
				a = h.get(t, a.ID)
				if a.State != "ready" || a.BasisCurrent || a.OriginalOutput != h.output || a.OriginalOutputSHA256 != digest(h.output) {
					t.Fatal("source drift erased occurred model facts", a)
				}
				_, e = f.s.CommandFailureDiagnosis(ctx, f.e, f.actor, f.project, a.ID, h.command(a, "adopt"))
				expertStatus(t, e, 409)
			}
			view, e := f.s.FailureDiagnosisContext(ctx, f.e, f.actor, f.project, h.request.RunID, h.request.NodeID, h.request.Attempt)
			if e != nil || view.Source == nil || view.Source.SHA256 == h.request.ExpectedSourceSHA256 {
				t.Fatal(view, e)
			}
		})
	}
}

func TestFailureDiagnosisCurrentAuthorityAndGrantedFactAfterLogout(t *testing.T) {
	h := newDiagnosisFixture(t)
	f, ctx := h.f, context.Background()
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api"}), 201)["token"].(string)
	api, e := f.s.AuthenticateActor(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.CreateFailureDiagnosis(ctx, f.e, api, h.request)
	expertStatus(t, e, 403)
	other := newFixture(t)
	_, e = f.s.FailureDiagnosisContext(ctx, f.e, f.actor, other.project, h.request.RunID, h.request.NodeID, 1)
	expertStatus(t, e, 404)
	a := h.create(t)
	changed := h.request
	changed.Goal = "Changed use of the same operation"
	_, e = f.s.CreateFailureDiagnosis(ctx, f.e, f.actor, changed)
	expertStatus(t, e, 409)
	assigned := h.claim(t)
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	// The authenticated Runner's already-occurred final fact survives creator
	// logout; it cannot authorize another execution or browser-only adoption.
	h.complete(t, h.result(assigned, h.output))
	a, e = f.s.GetFailureDiagnosis(ctx, f.e, api, f.project, a.ID)
	if e != nil || a.State != "ready" || a.OriginalOutput != h.output || len(a.AllowedActions) != 0 {
		t.Fatal(a, e)
	}
	_, e = f.s.CommandFailureDiagnosis(ctx, f.e, api, f.project, a.ID, h.command(a, "adopt"))
	expertStatus(t, e, 403)
	_, e = f.s.CreateFailureDiagnosis(ctx, f.e, f.actor, h.request)
	if e == nil {
		t.Fatal("revoked original credential recovered a write receipt")
	}
}

func TestFailureDiagnosisUnknownAndMalformedKeepActualOriginal(t *testing.T) {
	for _, mode := range []string{"unknown", "malformed", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			h := newDiagnosisFixture(t)
			a := h.create(t)
			assigned := h.claim(t)
			raw := h.output
			if mode == "malformed" {
				raw = `{"schema_version":1,"summary":"Incomplete proposal"}`
			}
			if mode == "oversize" {
				raw = strings.Repeat("x", (64<<10)+1)
			}
			r := h.result(assigned, raw)
			if mode == "unknown" {
				r.OutcomeUnknown = true
				r.ToolEvidence.Terminal = "unknown"
				r.Output = "partial observation"
			}
			h.complete(t, r)
			a = h.get(t, a.ID)
			if mode == "unknown" {
				if a.State != "reconciling" || a.OriginalOutput != "" || a.OriginalOutputSHA256 != "" || len(a.AllowedActions) != 0 {
					t.Fatal(a)
				}
				known := h.result(assigned, h.output)
				h.f.bearer("POST", "/runner/results", h.token, h.f.body(map[string]any{"result": known}), 409)
			} else if a.State != "failed" || a.Proposal != nil || a.OriginalOutputSHA256 != digest(raw) || mode == "malformed" && a.OriginalOutput != raw || mode == "oversize" && a.OriginalOutput != "" {
				t.Fatal(a)
			}
			if expertCount(t, h.f, "suma_failure_diagnosis_executions") != 1 {
				t.Fatal("invalid or unknown proposal caused a retry")
			}
		})
	}
}

func TestFailureDiagnosisExistingIssueRequirementOnlyOpensCurrentDraft(t *testing.T) {
	h := newDiagnosisFixture(t)
	f, ctx := h.f, context.Background()
	original, e := f.s.IssueToRequirement(ctx, f.actor, f.project, h.request.IssueID, f.body(map[string]any{"title": "Existing independently prepared repair"}))
	if e != nil {
		t.Fatal(e)
	}
	a := h.create(t)
	h.complete(t, h.result(h.claim(t), h.output))
	a = h.get(t, a.ID)
	if a.AdoptedRequirementID != original.ID {
		t.Fatal("existing repair link missing", a)
	}
	before := a.Version
	reply, e := f.s.CommandFailureDiagnosis(ctx, f.e, f.actor, f.project, a.ID, h.command(a, "adopt"))
	if e != nil || reply.AdoptedRequirementID != original.ID || reply.Version != before || reply.State != "ready" {
		t.Fatal(reply, e)
	}
	current, e := f.s.Get(ctx, f.actor, f.project, "requirements", original.ID)
	if e != nil || current.Title != original.Title || current.Description != original.Description || current.Version != original.Version || current.FailureDiagnosisAdoption != nil || expertCount(t, f, "suma_failure_diagnosis_adoptions") != 0 {
		t.Fatal("existing draft was overwritten or falsely attributed", current, e)
	}
}

func TestFailureDiagnosisAdoptionRollbackAndConcurrentSingleIssueDraft(t *testing.T) {
	h := newDiagnosisFixture(t)
	f, ctx := h.f, context.Background()
	first := h.create(t)
	h.complete(t, h.result(h.claim(t), h.output))
	first = h.get(t, first.ID)
	before := expertCount(t, f, "suma_requirements")
	if _, e := f.s.DB.Exec(`CREATE TRIGGER diagnosis_adoption_failure BEFORE INSERT ON suma_failure_diagnosis_adoptions BEGIN SELECT RAISE(ABORT,'fixture diagnosis adoption rollback'); END`); e != nil {
		t.Fatal(e)
	}
	_, e := f.s.CommandFailureDiagnosis(ctx, f.e, f.actor, f.project, first.ID, h.command(first, "adopt"))
	if e == nil {
		t.Fatal("adoption transaction ignored the injected failure")
	}
	var stable int
	if e = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`, f.user.TenantID, f.project, "issue-intake-"+h.request.IssueID).Scan(&stable); e != nil {
		t.Fatal(e)
	}
	if expertCount(t, f, "suma_requirements") != before || expertCount(t, f, "suma_failure_diagnosis_adoptions") != 0 || stable != 0 || h.get(t, first.ID).Version != first.Version {
		t.Fatal("adoption failure left partial draft, receipt or lineage")
	}
	if _, e = f.s.DB.Exec(`DROP TRIGGER diagnosis_adoption_failure`); e != nil {
		t.Fatal(e)
	}
	h.request.IdempotencyKey = newID()
	second := h.create(t)
	h.complete(t, h.result(h.claim(t), h.output))
	second = h.get(t, second.ID)
	var wg sync.WaitGroup
	type outcome struct {
		a FailureDiagnosisAnalysis
		e error
	}
	results := make(chan outcome, 2)
	for _, a := range []FailureDiagnosisAnalysis{first, second} {
		wg.Add(1)
		go func(a FailureDiagnosisAnalysis) {
			defer wg.Done()
			reply, err := f.s.CommandFailureDiagnosis(ctx, f.e, f.actor, f.project, a.ID, h.command(a, "adopt"))
			results <- outcome{reply, err}
		}(a)
	}
	wg.Wait()
	close(results)
	id := ""
	for result := range results {
		if result.e != nil || result.a.AdoptedRequirementID == "" {
			t.Fatal(result.a, result.e)
		}
		if id == "" {
			id = result.a.AdoptedRequirementID
		} else if id != result.a.AdoptedRequirementID {
			t.Fatal("same issue created two drafts")
		}
	}
	if expertCount(t, f, "suma_requirements") != before+1 || expertCount(t, f, "suma_failure_diagnosis_adoptions") != 1 {
		t.Fatal("concurrent adoption fabricated a second requirement or model lineage")
	}
}

func newDiagnosisFixture(t *testing.T) *diagnosisFixture {
	t.Helper()
	f := newFixture(t)
	ctx := context.Background()
	if e := f.s.InitializeFailureDiagnosis(ctx); e != nil {
		t.Fatal(e)
	}
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Actual failed engineering task", "description": "Retain the failed execution and require independent validation", "acceptance_criteria": []string{"Exercise the regression"}}), 201)
	w := f.call("POST", "/workflows", f.body(map[string]any{"name": "Known failure evidence fixture", "nodes": []dag.Node{{ID: "test", Type: "test", Command: []string{"fixture-failure"}, MaxRetries: 0}}}), 201)
	r := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": w["id"], "task_id": task["id"], "expected_version": task["version"]}), 201)
	a, e := f.e.Claim(ctx, f.user.TenantID, f.project, "failure-source-worker", []string{"command:fixture-failure"})
	if e != nil || a == nil {
		t.Fatal(a, e)
	}
	sourceOutput := strings.Repeat("诊断证据", 900) + "actual failing assertion"
	if e = f.e.Complete(ctx, f.user.TenantID, f.project, dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: 1, Error: "Expected 42, observed 0", Output: sourceOutput, OperationID: newID()}); e != nil {
		t.Fatal(e)
	}
	if e = f.s.ProjectFailures(ctx); e != nil {
		t.Fatal(e)
	}
	view, e := f.s.FailureDiagnosisContext(ctx, f.e, f.actor, f.project, r["id"].(string), "test", 1)
	if e != nil || !view.Eligible || view.Source == nil || view.Issue == nil {
		t.Fatal(view, e)
	}
	token := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "diagnosis-worker", "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	p := toolTestProfile()
	p.ReadOnly = true
	p.FailureDiagnosis = &toolcontract.FailureDiagnosisCapability{Protocol: toolcontract.FailureDiagnosisProtocol, Model: "local-diagnosis-protocol-fixture", EndpointOrigin: "http://127.0.0.1:45678"}
	profile, e := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "diagnosis-worker", Name: "Fixed one-call diagnosis fixture", Profile: p, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: p, Available: true}}}); e != nil {
		t.Fatal(e)
	}
	profile, e = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	h := &diagnosisFixture{f: f, token: token, profile: profile, context: view}
	h.request = FailureDiagnosisCreateRequest{ProjectID: f.project, IssueID: view.Issue.ID, ExpectedIssueVersion: view.Issue.Version, RunID: view.RunID, NodeID: view.NodeID, Attempt: view.Attempt, ExpectedSourceSHA256: view.Source.SHA256, Goal: "Explain this fixed failure with testable hypotheses and a draft repair", Locale: "en-US", ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version, AcknowledgeModelUse: true, IdempotencyKey: newID()}
	h.output = stringify(FailureDiagnosisOutput{SchemaVersion: 1, Summary: "A fixed assertion failed; its cause requires confirmation", Observations: []FailureDiagnosisObservation{{Statement: "The expected result differs from the observed value", EvidenceIndices: []int{len(view.Source.Evidence) - 2}}}, Hypotheses: []FailureDiagnosisHypothesis{{Statement: "An unchecked boundary may return zero", EvidenceIndices: []int{len(view.Source.Evidence) - 1}, Confidence: "medium"}}, RecommendedActions: []string{"Inspect the relevant boundary without rerunning the unknown operation"}, TestPlan: []string{"Add an independent regression asserting the intended value"}, Repair: FailureRepairDraft{Title: "Investigate incorrect calculation output", Description: "Confirm the proposed cause against the fixed execution, then implement the reviewed correction.", Priority: "high", AcceptanceCriteria: []string{"The regression passes independently"}}})
	return h
}
func (h *diagnosisFixture) create(t *testing.T) FailureDiagnosisAnalysis {
	t.Helper()
	a, e := h.f.s.CreateFailureDiagnosis(context.Background(), h.f.e, h.f.actor, h.request)
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func (h *diagnosisFixture) claim(t *testing.T) dag.Assignment {
	t.Helper()
	r := h.f.bearer("POST", "/runner/claim", h.token, h.f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}, "idempotency_key": newID()}), 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(r["assignment"])), &a) != nil || a.NodeID != "diagnose" || a.Config["purpose"] != "failure_diagnosis" {
		t.Fatal(r)
	}
	return a
}
func (h *diagnosisFixture) result(a dag.Assignment, raw string) dag.Result {
	return dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), Output: raw, ToolEvidence: &toolcontract.Evidence{Profile: h.profile.Profile, Terminal: "completed", EventCount: 1, UsageState: toolcontract.UsageUnreported}}
}
func (h *diagnosisFixture) complete(t *testing.T, r dag.Result) {
	t.Helper()
	h.f.bearer("POST", "/runner/results", h.token, h.f.body(map[string]any{"result": r}), 200)
}
func (h *diagnosisFixture) get(t *testing.T, id string) FailureDiagnosisAnalysis {
	t.Helper()
	a, e := h.f.s.GetFailureDiagnosis(context.Background(), h.f.e, h.f.actor, h.f.project, id)
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func (h *diagnosisFixture) command(a FailureDiagnosisAnalysis, action string) FailureDiagnosisCommandRequest {
	return FailureDiagnosisCommandRequest{ProjectID: h.f.project, ExpectedVersion: a.Version, Action: action, Reason: "Explicit test-owner review of the fixed proposal", AcknowledgeDraft: true, IdempotencyKey: newID()}
}

func TestFailureDiagnosisStrictOutputAndTailEvidence(t *testing.T) {
	source := FailureDiagnosisSource{Evidence: []FailureDiagnosisEvidence{{Kind: "error"}}}
	valid := stringify(FailureDiagnosisOutput{SchemaVersion: 1, Summary: "Review the failure", Observations: []FailureDiagnosisObservation{{Statement: "Failed assertion", EvidenceIndices: []int{0}}}, Hypotheses: []FailureDiagnosisHypothesis{}, RecommendedActions: []string{"Review the boundary"}, TestPlan: []string{"Add an independent test"}, Repair: FailureRepairDraft{Title: "Repair observed behavior", Description: "Verify the cause before editing", Priority: "medium", AcceptanceCriteria: []string{"Independent test passes"}}})
	if _, e := parseFailureDiagnosisOutput(valid, source); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{strings.Replace(valid, `"schema_version":1`, `"schema_version":1.0`, 1), strings.Replace(valid, `"summary":`, `"Summary":`, 1), strings.Replace(valid, `"statement":`, `"Statement":`, 1), strings.Replace(valid, `"priority":"medium"`, `"priority":"High"`, 1), strings.Replace(valid, `"evidence_indices":[0]`, `"evidence_indices":[0,0]`, 1), strings.Replace(valid, `"hypotheses":[]`, `"hypotheses":null`, 1), strings.Replace(valid, `"summary":`, `"summary":"duplicate","summary":`, 1), valid + ` {}`} {
		if _, e := parseFailureDiagnosisOutput(bad, source); e == nil {
			t.Fatalf("invalid proposal accepted: %s", bad)
		}
	}
	full := strings.Repeat("字", 4001)
	e, err := diagnosisEvidence("output", full)
	if err != nil || e.Selection != "tail" || e.TotalBytes != len(full) || e.SelectedBytes != len(e.Text) || e.SelectedBytes > 8000 || !utf8.ValidString(e.Text) || e.SHA256 != digest(full) {
		t.Fatal(e, err)
	}
}

func TestFailureDiagnosisActualDAGRepairAdoptionAndOriginalIntakeReplay(t *testing.T) {
	h := newDiagnosisFixture(t)
	f, ctx := h.f, context.Background()
	a := h.create(t)
	replayed, e := f.s.CreateFailureDiagnosis(ctx, f.e, f.actor, h.request)
	if e != nil || replayed.RunID != a.RunID {
		t.Fatal(replayed, e)
	}
	assignment := h.claim(t)
	result := h.result(assignment, h.output)
	h.complete(t, result)
	h.complete(t, result)
	a = h.get(t, a.ID)
	if a.State != "ready" || !a.BasisCurrent || a.Proposal == nil || a.Repair == nil || a.OriginalOutput != h.output || a.OriginalOutputSHA256 != digest(h.output) || expertCount(t, f, "suma_failure_diagnosis_executions") != 1 {
		t.Fatal(a)
	}
	originalProposal := stringify(a.Proposal)
	revise := h.command(a, "revise")
	repair := *a.Repair
	repair.Title = "Human-reviewed repair scope"
	revise.Repair = &repair
	a, e = f.s.CommandFailureDiagnosis(ctx, f.e, f.actor, f.project, a.ID, revise)
	if e != nil || a.Repair.Title != repair.Title || stringify(a.Proposal) != originalProposal || a.OriginalOutput != h.output {
		t.Fatal(a, e)
	}
	adopt := h.command(a, "adopt")
	a, e = f.s.CommandFailureDiagnosis(ctx, f.e, f.actor, f.project, a.ID, adopt)
	if e != nil || a.State != "adopted" || a.AdoptedRequirementID == "" {
		t.Fatal(a, e)
	}
	requirement, e := f.s.Get(ctx, f.actor, f.project, "requirements", a.AdoptedRequirementID)
	if e != nil || requirement.Status != "draft" || requirement.Title != repair.Title || len(requirement.AcceptanceCriteria) != 1 || requirement.Data["source_issue_id"] != a.Source.IssueID || requirement.FailureDiagnosisAdoption == nil || requirement.FailureDiagnosisAdoption.AnalysisID != a.ID {
		t.Fatal(requirement, e)
	}
	originalRequirement := stringify(requirement)
	old, e := f.s.IssueToRequirement(ctx, f.actor, f.project, a.Source.IssueID, f.body(map[string]any{"title": "Do not overwrite the reviewed draft"}))
	if e != nil || old.ID != requirement.ID || stringify(old) != originalRequirement {
		t.Fatal("old intake did not open the same current draft", old, e)
	}
	repeat, e := f.s.CommandFailureDiagnosis(ctx, f.e, f.actor, f.project, a.ID, adopt)
	if e != nil || repeat.AdoptedRequirementID != a.AdoptedRequirementID || expertCount(t, f, "suma_failure_diagnosis_adoptions") != 1 {
		t.Fatal(repeat, e)
	}
	issue, e := f.s.Get(ctx, f.actor, f.project, "issues", a.Source.IssueID)
	if e != nil || issue.Status != "open" || issue.Version != a.Source.IssueVersion {
		t.Fatal("diagnosis pretended to fix the failure", issue, e)
	}
}
