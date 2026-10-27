package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

type intelligenceFixture struct {
	f       *fixture
	token   string
	profile ToolProfile
	doc     Record
	request KnowledgeIntelligenceCreateRequest
}

func newKnowledgeIntelligenceFixture(t *testing.T) *intelligenceFixture {
	t.Helper()
	f := newFixture(t)
	ctx := context.Background()
	if e := f.s.InitializeKnowledgeIntelligence(ctx); e != nil {
		t.Fatal(e)
	}
	token := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "extraction-worker", "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	p := toolTestProfile()
	p.ReadOnly = true
	p.KnowledgeExtraction = &toolcontract.KnowledgeExtractionCapability{Protocol: toolcontract.KnowledgeExtractionProtocol, Model: "local-protocol-extraction-fixture", EndpointOrigin: "http://127.0.0.1:45678"}
	profile, e := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "extraction-worker", Name: "Explicit knowledge extraction fixture", Profile: p, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: p, Available: true}}})
	if e != nil {
		t.Fatal(e)
	}
	profile, e = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: profile.Version, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	doc, e := f.s.Create(ctx, f.actor, f.project, "documents", f.body(map[string]any{"title": "Authorized extraction source", "description": "Review migrations before executing them. Preserve source provenance. 原文"}))
	if e != nil {
		t.Fatal(e)
	}
	return &intelligenceFixture{f: f, token: token, profile: profile, doc: doc, request: KnowledgeIntelligenceCreateRequest{ProjectID: f.project, Kind: "skill", Goal: "Extract a reviewable reusable skill from this source", Locale: "en-US", Sources: []ProjectKnowledgeSourceInput{{Kind: "documents", ID: doc.ID, Version: doc.Version}}, ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version, AcknowledgeModelUse: true, IdempotencyKey: newID()}}
}
func (h *intelligenceFixture) create(t *testing.T) KnowledgeIntelligenceAnalysis {
	t.Helper()
	out, e := h.f.s.CreateKnowledgeIntelligence(context.Background(), h.f.e, h.f.actor, h.f.project, h.request)
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func (h *intelligenceFixture) claim(t *testing.T) dag.Assignment {
	t.Helper()
	reply := h.f.bearer("POST", "/runner/claim", h.token, h.f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}}), 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(reply["assignment"])), &a) != nil || a.RunID == "" {
		t.Fatal("no actual extraction assignment", reply)
	}
	return a
}
func (h *intelligenceFixture) output() string {
	return stringify(KnowledgeIntelligenceOutput{SchemaVersion: 1, Title: "Review database migrations", Summary: "Explicitly review evidence before execution", Content: "Inspect migration and rollback evidence before authorizing execution. 保留原文", SourceIndices: []int{0}, Rationale: "The fixed source explicitly requires review and provenance."})
}
func (h *intelligenceFixture) result(a dag.Assignment, raw string) dag.Result {
	return dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, Output: raw, OperationID: newID(), ToolEvidence: &toolcontract.Evidence{Profile: h.profile.Profile, Terminal: "completed", EventCount: 1, UsageState: toolcontract.UsageUnreported}}
}
func (h *intelligenceFixture) complete(t *testing.T, r dag.Result) {
	t.Helper()
	h.f.bearer("POST", "/runner/results", h.token, h.f.body(map[string]any{"result": r}), 200)
}
func (h *intelligenceFixture) get(t *testing.T, id string) KnowledgeIntelligenceAnalysis {
	t.Helper()
	out, e := h.f.s.GetKnowledgeIntelligence(context.Background(), h.f.actor, h.f.project, id)
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func (h *intelligenceFixture) command(a KnowledgeIntelligenceAnalysis, action string) KnowledgeIntelligenceCommandRequest {
	return KnowledgeIntelligenceCommandRequest{ProjectID: h.f.project, ExpectedVersion: a.Version, Action: action, Reason: "Explicit reviewer instruction, no automatic publication", AcknowledgeDraft: true, IdempotencyKey: newID()}
}

func TestKnowledgeIntelligenceStrictProposalContract(t *testing.T) {
	source := KnowledgeIntelligenceSource{Sources: []ProjectKnowledgeSource{{ID: "source"}}}
	raw := `{"schema_version":1,"title":"t","summary":"","content":"body","source_indices":[0],"rationale":"because"}`
	if _, e := parseKnowledgeIntelligenceOutput(raw, source); e != nil {
		t.Fatal(e)
	}
	for name, bad := range map[string]string{"foreign-index": strings.Replace(raw, `[0]`, `[1]`, 1), "duplicate-index": strings.Replace(raw, `[0]`, `[0,0]`, 1), "fraction": strings.Replace(raw, `[0]`, `[0.0]`, 1), "source-injection": strings.Replace(raw, `"title":"t"`, `"title":"t","sources":[]`, 1), "null-indices": strings.Replace(raw, `[0]`, `null`, 1), "duplicate-key": strings.Replace(raw, `"title":"t"`, `"title":"t","title":"b"`, 1), "oversize": strings.Replace(raw, `"body"`, stringify(strings.Repeat("x", 16001)), 1)} {
		t.Run(name, func(t *testing.T) {
			if _, e := parseKnowledgeIntelligenceOutput(bad, source); e == nil {
				t.Fatal("invalid model proposal accepted")
			}
		})
	}
}

func TestKnowledgeIntelligenceCreateBoundSourcesAndSingleRun(t *testing.T) {
	h := newKnowledgeIntelligenceFixture(t)
	a := h.create(t)
	again := h.create(t)
	if a.ID != again.ID || a.RunID != again.RunID || a.MaxSteps != 1 || a.Source.Sources[0].Quote != h.doc.Description || a.Source.SHA256 != digest(stringify(a.Source.Sources)) {
		t.Fatal(a, again)
	}
	bad := h.request
	bad.IdempotencyKey = newID()
	bad.MaxSteps = 2
	if _, e := h.f.s.CreateKnowledgeIntelligence(context.Background(), h.f.e, h.f.actor, h.f.project, bad); e == nil {
		t.Fatal("unbounded extra model steps")
	}
	bad = h.request
	bad.IdempotencyKey = newID()
	bad.Sources = []ProjectKnowledgeSourceInput{{Kind: "documents", ID: h.doc.ID, Version: h.doc.Version, Quote: "Invented external quote"}}
	if _, e := h.f.s.CreateKnowledgeIntelligence(context.Background(), h.f.e, h.f.actor, h.f.project, bad); e == nil {
		t.Fatal("unapproved source quote")
	}
	api, e := h.f.s.IssueProjectCredential(context.Background(), h.f.actor, ProjectCredentialIssueRequest{ProjectID: h.f.project, Scope: "api", IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	actor, e := h.f.s.AuthenticateActor(context.Background(), api.Token)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = h.f.s.CreateKnowledgeIntelligence(context.Background(), h.f.e, actor, h.f.project, h.request); e == nil {
		t.Fatal("API initiated model consumption")
	}
	if _, e = h.f.s.GetKnowledgeIntelligence(context.Background(), actor, h.f.project, a.ID); e != nil {
		t.Fatal("current scoped project API could not read source analysis", e)
	}
}

func TestKnowledgeIntelligenceHTTPResultReviseAdoptDraftAndReplay(t *testing.T) {
	h := newKnowledgeIntelligenceFixture(t)
	a := h.create(t)
	assignment := h.claim(t)
	if assignment.NodeID != "extract" || assignment.Config["purpose"] != "knowledge_extraction" || assignment.Idempotent || assignment.TimeoutSeconds != 120 {
		t.Fatal(assignment)
	}
	original := h.output()
	result := h.result(assignment, original)
	h.complete(t, result)
	h.complete(t, result)
	a = h.get(t, a.ID)
	if a.State != "ready" || a.Proposal == nil || a.OriginalOutput != original || a.OriginalOutputSHA256 != digest(original) || !a.BasisCurrent {
		t.Fatal(a)
	}
	revision := h.command(a, "revise")
	proposal := *a.Proposal
	proposal.Content += " Human-reviewed amendment."
	revision.Proposal = &proposal
	revised, e := h.f.s.CommandKnowledgeIntelligence(context.Background(), h.f.actor, h.f.project, a.ID, revision)
	if e != nil || revised.OriginalOutput != original {
		t.Fatal(revised, e)
	}
	adopt := h.command(revised, "adopt")
	adopt.AcknowledgeDraft = false
	if _, e = h.f.s.CommandKnowledgeIntelligence(context.Background(), h.f.actor, h.f.project, a.ID, adopt); e == nil {
		t.Fatal("adopted without draft acknowledgment")
	}
	adopt = h.command(revised, "adopt")
	invitation := governanceInvite(h.f, "model-draft-writer@example.test", "member", []any{map[string]any{"project_id": h.f.project, "role": "developer"}})
	governanceAccept(h.f, invitation, "model-draft-writer@example.test")
	writer, e := h.f.s.AuthenticateActor(context.Background(), h.f.cookie.Value)
	if e != nil {
		t.Fatal(e)
	}
	adopted, e := h.f.s.CommandKnowledgeIntelligence(context.Background(), writer, h.f.project, a.ID, adopt)
	if e != nil || adopted.State != "adopted" || adopted.AdoptedKnowledgeID == "" {
		t.Fatal(adopted, e)
	}
	replay, e := h.f.s.CommandKnowledgeIntelligence(context.Background(), writer, h.f.project, a.ID, adopt)
	if e != nil || replay.AdoptedKnowledgeID != adopted.AdoptedKnowledgeID {
		t.Fatal(replay, e)
	}
	staleRevision, e := h.f.s.CommandKnowledgeIntelligence(context.Background(), h.f.actor, h.f.project, a.ID, revision)
	if e != nil || staleRevision.State != "ready" || len(staleRevision.AllowedActions) != 0 {
		t.Fatal("old revise receipt regained adoption authority", staleRevision, e)
	}
	draft, e := h.f.s.GetProjectKnowledge(context.Background(), h.f.actor, h.f.project, adopted.AdoptedKnowledgeID)
	if e != nil || draft.State != "draft" || draft.Published != nil || draft.Latest.Method != "model_assisted" || draft.Latest.Content != proposal.Content || len(draft.Latest.Sources) != 1 || draft.Latest.Sources[0] != a.Source.Sources[0] {
		t.Fatal(draft, e)
	}
	if _, e = h.f.s.CommandProjectKnowledge(context.Background(), writer, h.f.project, draft.ID, ProjectKnowledgeCommandRequest{ProjectID: h.f.project, Action: "publish", ExpectedVersion: draft.Version, IdempotencyKey: newID()}); e == nil {
		t.Fatal("draft writer bypassed separate publication approval")
	}
	tx, e := h.f.s.DB.BeginTx(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	lineage, e := h.f.s.knowledgeIntelligenceAdoptionTx(context.Background(), tx, h.f.user.TenantID, h.f.project, draft.ID)
	tx.Rollback()
	if e != nil || lineage == nil || lineage.SourceSHA256 != a.Source.SHA256 || lineage.OriginalOutputSHA256 != digest(original) || lineage.SHA256 != draft.Latest.SHA256 {
		t.Fatal(lineage, e)
	}
	var facts, knowledge int
	if e = h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_knowledge_intelligence_outcomes WHERE analysis_id=?`, a.ID).Scan(&facts); e != nil || facts != 1 {
		t.Fatal(facts, e)
	}
	if e = h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_project_knowledge WHERE project_id=?`, h.f.project).Scan(&knowledge); e != nil || knowledge != 1 {
		t.Fatal("duplicate or automatically approved knowledge", knowledge, e)
	}
}

func TestKnowledgeIntelligenceSourceDriftKeepsActiveResultButPreventsAdoption(t *testing.T) {
	h := newKnowledgeIntelligenceFixture(t)
	a := h.create(t)
	assignment := h.claim(t)
	if _, e := h.f.s.DB.Exec(`UPDATE suma_documents SET version=version+1,description='Changed authorized source' WHERE id=?`, h.doc.ID); e != nil {
		t.Fatal(e)
	}
	h.f.bearer("POST", "/runner/heartbeat", h.token, h.f.body(map[string]any{"assignment": assignment}), 200)
	h.complete(t, h.result(assignment, h.output()))
	a = h.get(t, a.ID)
	if a.State != "ready" || a.Proposal == nil || a.BasisCurrent || a.OriginalOutput != h.output() || skillHasAction(a.AllowedActions, "adopt") {
		t.Fatal("source change erased model result or allowed stale adoption", a)
	}
	if _, e := h.f.s.CommandKnowledgeIntelligence(context.Background(), h.f.actor, h.f.project, a.ID, h.command(a, "adopt")); e == nil {
		t.Fatal("adopted changed source")
	}
	run := h.f.call("GET", "/runs/"+a.RunID+"?project_id="+h.f.project, nil, 200)
	if run["state"] != "succeeded" {
		t.Fatal("source drift lost actual process completion", run)
	}
	h.request.IdempotencyKey = newID()
	h.request.Sources[0].Version++
	queued := h.create(t)
	if _, e := h.f.s.DB.Exec(`UPDATE suma_documents SET version=version+1 WHERE id=?`, h.doc.ID); e != nil {
		t.Fatal(e)
	}
	reply := h.f.bearer("POST", "/runner/claim", h.token, h.f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}}), 200)
	if reply["assignment"] != nil {
		t.Fatal("new claim used stale source")
	}
	var attempt int
	if e := h.f.s.DB.QueryRow(`SELECT attempt FROM dag_nodes WHERE run_id=?`, queued.RunID).Scan(&attempt); e != nil || attempt != 0 {
		t.Fatal(attempt, e)
	}
}

func TestKnowledgeIntelligenceUnknownMalformedAndOversizedResults(t *testing.T) {
	for _, mode := range []string{"unknown", "malformed", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			h := newKnowledgeIntelligenceFixture(t)
			a := h.create(t)
			assignment := h.claim(t)
			raw := h.output()
			if mode == "malformed" {
				raw = `{"schema_version":1,"sources":[]}`
			} else if mode == "oversized" {
				raw = strings.Repeat("x", 65537)
			}
			result := h.result(assignment, raw)
			if mode == "unknown" {
				result.OutcomeUnknown = true
				result.ToolEvidence.Terminal = "unknown"
			}
			h.complete(t, result)
			a = h.get(t, a.ID)
			if mode != "unknown" && a.OriginalOutputSHA256 != digest(raw) {
				t.Fatal("original digest lost", a)
			}
			if mode == "oversized" && a.OriginalOutput != "" {
				t.Fatal("oversized raw output was truncated and presented as complete")
			}
			if mode != "unknown" {
				if a.State != "failed" || a.Proposal != nil {
					t.Fatal(a)
				}
				return
			}
			if a.State != "reconciling" || a.Proposal != nil || a.OriginalOutput != "" || a.OriginalOutputSHA256 != "" {
				t.Fatal(a)
			}
			if _, e := h.f.s.CommandKnowledgeIntelligence(context.Background(), h.f.actor, h.f.project, a.ID, h.command(a, "dismiss")); e == nil {
				t.Fatal("discarded unknown model execution")
			}
			stopped := h.result(assignment, "")
			stopped.Stopped = true
			stopped.ToolEvidence.Terminal = "unknown"
			h.complete(t, stopped)
			a = h.get(t, a.ID)
			if a.State != "failed" || a.OriginalOutputSHA256 != digest(stopped.Output) {
				t.Fatal("known stop did not freeze its received original", a)
			}
			var original string
			if e := h.f.s.DB.QueryRow(`SELECT result FROM suma_knowledge_intelligence_outcomes WHERE analysis_id=? AND result_sha256=?`, a.ID, digest(stringify(result))).Scan(&original); e != nil || original != stringify(result) {
				t.Fatal("unknown observation was not preserved independently", e)
			}
		})
	}
}

func TestKnowledgeIntelligenceLateKnownOriginalHTTP(t *testing.T) {
	for _, scenario := range []struct{ name, partial, final, state string }{
		{"empty", "", "", "ready"},
		{"partial", `{"schema_version":1,"title":"partial`, "", "ready"},
		{"invalid-json", "", `{"schema_version":1,"sources":[]}`, "failed"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			h := newKnowledgeIntelligenceFixture(t)
			h.f.e.AttemptReconciliation = h.f.s
			h.f.e.AttemptAccounting = h.f.s
			a := h.create(t)
			assignment := h.claim(t)
			unknown := h.result(assignment, scenario.partial)
			unknown.OutcomeUnknown = true
			unknown.ToolEvidence.Terminal = "unknown"
			h.complete(t, unknown)
			observed := h.get(t, a.ID)
			if observed.State != "reconciling" || observed.OriginalOutput != "" || observed.OriginalOutputSHA256 != "" {
				t.Fatal("unknown observation masqueraded as a model original", observed)
			}
			known := h.result(assignment, h.output())
			if scenario.final != "" {
				known.Output = scenario.final
			}
			known.ToolEvidence.Usage = &toolcontract.Usage{InputTokens: usageInt(7), OutputTokens: usageInt(3)}
			known.ToolEvidence.UsageState = toolcontract.UsageComplete
			h.complete(t, known)
			h.complete(t, known)
			ready := h.get(t, a.ID)
			if ready.State != scenario.state || (scenario.state == "ready") != (ready.Proposal != nil) || ready.OriginalOutput != known.Output || ready.OriginalOutputSHA256 != digest(known.Output) || ready.RunID != a.RunID {
				t.Fatal("late completion did not freeze the actual final model JSON", ready)
			}
			for _, fact := range []dag.Result{unknown, known} {
				var original string
				if e := h.f.s.DB.QueryRow(`SELECT result FROM suma_knowledge_intelligence_outcomes WHERE analysis_id=? AND result_sha256=?`, a.ID, digest(stringify(fact))).Scan(&original); e != nil || original != stringify(fact) {
					t.Fatal("recovery rewrote original observation", e)
				}
			}
			var count, attempt int
			if e := h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_knowledge_intelligence_outcomes WHERE analysis_id=?`, a.ID).Scan(&count); e != nil || count != 2 {
				t.Fatal("result replay duplicated observations", count, e)
			}
			if e := h.f.s.DB.QueryRow(`SELECT attempt FROM dag_nodes WHERE run_id=? AND id=?`, a.RunID, assignment.NodeID).Scan(&attempt); e != nil || attempt != 1 {
				t.Fatal("recovery reran model attempt", attempt, e)
			}
			usage, e := h.f.s.ListUsageAttempts(context.Background(), h.f.actor, h.f.project, "", "", 20, "")
			if e != nil || len(usage.Items) != 1 || usage.Items[0].ReportedTokens == nil || *usage.Items[0].ReportedTokens != 10 || usage.Items[0].UsageState != toolcontract.UsageComplete {
				t.Fatal("known completion did not preserve measured usage independently of proposal schema", usage, e)
			}
			run, e := h.f.e.GetRun(context.Background(), h.f.user.TenantID, h.f.project, a.RunID)
			if e != nil || run.State != "succeeded" {
				t.Fatal("known process completion was confused with proposal validity", run, e)
			}
		})
	}
	for _, mode := range []string{"stopped", "expired", "creator"} {
		t.Run(mode+"-refused", func(t *testing.T) {
			h := newKnowledgeIntelligenceFixture(t)
			h.f.e.AttemptReconciliation = h.f.s
			reader := h.f.actor
			if mode == "creator" {
				h.f.e.AttemptAccounting = h.f.s
				api, e := h.f.s.IssueProjectCredential(context.Background(), h.f.actor, ProjectCredentialIssueRequest{ProjectID: h.f.project, Scope: "api", IdempotencyKey: newID()})
				if e != nil {
					t.Fatal(e)
				}
				reader, e = h.f.s.AuthenticateActor(context.Background(), api.Token)
				if e != nil {
					t.Fatal(e)
				}
			}
			a := h.create(t)
			assignment := h.claim(t)
			unknown := h.result(assignment, "partial observation")
			unknown.OutcomeUnknown = true
			unknown.ToolEvidence.Terminal = "unknown"
			unknown.Stopped = mode == "stopped"
			h.complete(t, unknown)
			if mode == "expired" {
				if _, e := h.f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, assignment.RunID, assignment.NodeID); e != nil {
					t.Fatal(e)
				}
			}
			if mode == "creator" {
				if e := logoutCredentialForTest(context.Background(), h.f.s, h.f.cookie.Value); e != nil {
					t.Fatal(e)
				}
			}
			known := h.result(assignment, h.output())
			h.f.bearer("POST", "/runner/results", h.token, h.f.body(map[string]any{"result": known}), 409)
			observed, e := h.f.s.GetKnowledgeIntelligence(context.Background(), reader, h.f.project, a.ID)
			if e != nil {
				t.Fatal(e)
			}
			if observed.State != "reconciling" || observed.Proposal != nil || observed.OriginalOutput != "" || observed.OriginalOutputSHA256 != "" {
				t.Fatal("ineligible late result changed original or proposal", observed)
			}
			var count, attempt int
			if e := h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_knowledge_intelligence_outcomes WHERE analysis_id=?`, a.ID).Scan(&count); e != nil || count != 1 {
				t.Fatal("ineligible completion persisted outcome", count, e)
			}
			if e := h.f.s.DB.QueryRow(`SELECT attempt FROM dag_nodes WHERE run_id=? AND id=?`, a.RunID, assignment.NodeID).Scan(&attempt); e != nil || attempt != 1 {
				t.Fatal("ineligible recovery changed attempt", attempt, e)
			}
			if mode == "creator" {
				var raw string
				if e := h.f.s.DB.QueryRow(`SELECT payload FROM suma_usage_attempts WHERE tenant_id=? AND project_id=? AND run_id=?`, h.f.user.TenantID, h.f.project, a.RunID).Scan(&raw); e != nil {
					t.Fatal(e)
				}
				var ledger usageRecord
				if e := json.Unmarshal([]byte(raw), &ledger); e != nil || !ledger.Unresolved || ledger.Attempt.State != "reconciling" || ledger.Attempt.ReportedTokens != nil || ledger.Attempt.SourceResultSHA256 != digest(stringify(unknown)) {
					t.Fatal("revoked creator recovery changed unknown accounting", ledger, e)
				}
				if e := h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_usage_facts WHERE attempt_id=? AND kind='result'`, ledger.Attempt.ID).Scan(&count); e != nil || count != 1 {
					t.Fatal("revoked creator added a known usage fact", count, e)
				}
				if e := h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_project_knowledge WHERE tenant_id=? AND project_id=?`, h.f.user.TenantID, h.f.project).Scan(&count); e != nil || count != 0 {
					t.Fatal("revoked creator generated a draft", count, e)
				}
			}
		})
	}
}

func TestKnowledgeIntelligenceCurrentCreatorAndAtomicDraftRollback(t *testing.T) {
	h := newKnowledgeIntelligenceFixture(t)
	a := h.create(t)
	h.complete(t, h.result(h.claim(t), h.output()))
	a = h.get(t, a.ID)
	if _, e := h.f.s.DB.Exec(`CREATE TRIGGER intelligence_adoption_failure BEFORE INSERT ON suma_knowledge_intelligence_adoptions BEGIN SELECT RAISE(ABORT,'fixture lineage failure'); END`); e != nil {
		t.Fatal(e)
	}
	if _, e := h.f.s.CommandKnowledgeIntelligence(context.Background(), h.f.actor, h.f.project, a.ID, h.command(a, "adopt")); e == nil {
		t.Fatal("lineage failure was acknowledged")
	}
	var drafts int
	if e := h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_project_knowledge WHERE project_id=?`, h.f.project).Scan(&drafts); e != nil || drafts != 0 {
		t.Fatal("failed transaction leaked draft", drafts, e)
	}
	if current := h.get(t, a.ID); current.Version != a.Version || current.State != "ready" {
		t.Fatal(current)
	}
	h.request.IdempotencyKey = newID()
	queued := h.create(t)
	if _, e := h.f.s.DB.Exec(`UPDATE suma_credential_slots SET revoked=1 WHERE current_digest=?`, h.f.actor.credential.digest); e != nil {
		t.Fatal(e)
	}
	reply := h.f.bearer("POST", "/runner/claim", h.token, h.f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}}), 200)
	if reply["assignment"] != nil {
		t.Fatal("worker replaced revoked source browser authority")
	}
	var attempt int
	if e := h.f.s.DB.QueryRow(`SELECT attempt FROM dag_nodes WHERE run_id=?`, queued.RunID).Scan(&attempt); e != nil || attempt != 0 {
		t.Fatal(attempt, e)
	}
	if _, e := h.f.s.CreateKnowledgeIntelligence(context.Background(), h.f.e, h.f.actor, h.f.project, h.request); e == nil {
		t.Fatal("revoked creator replayed model operation")
	}
}
