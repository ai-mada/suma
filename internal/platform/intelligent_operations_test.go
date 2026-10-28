package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// A missing closed-contract field must fail before any model run or draft is
// created; the route is a domain command, not a generic record/API dispatcher.
func TestIntelligentOperationsHTTPRejectsMissingClosedContract(t *testing.T) {
	f := newFixture(t)
	f.call("POST", "/intelligent-operations/analyses", f.body(map[string]any{
		"locale":  "en-US",
		"sources": []any{}, "tool_profile_id": "untrusted-profile", "expected_tool_version": 1,
		"acknowledge_model_use": true,
	}), 400)
}

func operationTestOutput() IntelligentOperationOutput {
	return IntelligentOperationOutput{SchemaVersion: 1, Summary: "Create reviewable local drafts", Rationale: "Keep all formal approval and execution gates", Questions: []string{}, Steps: []IntelligentOperationStep{
		{Key: "requirement", Action: "requirement.create_draft", DependsOn: []string{}, Requirement: &IntelligentOperationRequirement{Title: "Reviewed feature", Description: "Create a bounded feature", AcceptanceCriteria: []string{"Independent test evidence is required"}}},
		{Key: "design", Action: "requirement.design.save_draft", DependsOn: []string{"requirement"}, Design: &IntelligentOperationDesign{RequirementStepKey: "requirement", Content: RequirementDesignContent{Product: "Review the feature", Experience: "Preview the draft", Technical: "Preserve the current authority"}, Tasks: []RequirementDesignTask{}}},
		{Key: "knowledge", Action: "knowledge.create_draft", DependsOn: []string{"design"}, Knowledge: &IntelligentOperationKnowledge{Kind: "spec", Title: "Reviewed guidance", Content: "Preserve human approval", SourceIndices: []int{}}},
	}}
}

func TestIntelligentOperationsClosedOutputRejectsEscalationAndAmbiguity(t *testing.T) {
	source := IntelligentOperationSource{Sources: []ProjectKnowledgeSource{}}
	valid := stringify(operationTestOutput())
	if _, err := parseIntelligentOperationOutput(valid, source); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		strings.Replace(valid, `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1),
		strings.Replace(valid, `"requirement.create_draft"`, `"release.deploy"`, 1),
		strings.Replace(valid, `"title":"Reviewed feature"`, `"title":"Reviewed feature","status":"accepted"`, 1),
		strings.Replace(valid, `"depends_on":[]`, `"depends_on":["design"]`, 1),
		strings.Replace(valid, `"requirement_step_key":"requirement"`, `"requirement_step_key":"knowledge"`, 1),
		strings.Replace(valid, `"source_indices":[]`, `"source_indices":[0]`, 1),
		strings.Replace(valid, `"description":"Create a bounded feature"`, `"description":null`, 1),
		valid + `{}`,
	} {
		if _, err := parseIntelligentOperationOutput(raw, source); err == nil {
			t.Fatalf("unsafe output accepted: %s", raw)
		}
	}
}

func seedOperationAnalysis(t *testing.T, f *fixture, proposal IntelligentOperationOutput) IntelligentOperationAnalysis {
	t.Helper()
	if err := f.s.InitializeIntelligentOperations(context.Background()); err != nil {
		t.Fatal(err)
	}
	source := IntelligentOperationSource{Sources: []ProjectKnowledgeSource{}}
	source.SHA256 = digest(stringify(source.Sources))
	a := IntelligentOperationAnalysis{ID: newID(), ProjectID: f.project, Version: 2, State: "ready", Goal: "Create local reviewed drafts", Locale: "en-US", CreatedBy: f.user.ID, CreatedAt: now(), UpdatedAt: now(), RunID: "operation-fixture-" + newID(), Source: source, Proposal: &proposal, OriginalOutput: stringify(proposal), OriginalOutputSHA256: digest(stringify(proposal)), Results: []IntelligentOperationStepResult{}, AllowedActions: []string{}, BasisCurrent: true}
	raw := stringify(map[string]any{"analysis": a, "creator_credential_hash": f.actor.credential.digest})
	if _, err := f.s.DB.Exec(`INSERT INTO suma_intelligent_operations(id,tenant_id,project_id,version,state,run_id,created_at,payload)VALUES(?,?,?,?,?,?,?,?)`, a.ID, f.user.TenantID, f.project, a.Version, a.State, a.RunID, a.CreatedAt, raw); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestIntelligentOperationsAtomicDraftBatchReceiptAndRollback(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := seedOperationAnalysis(t, f, operationTestOutput())
	preview, err := f.s.PreviewIntelligentOperation(ctx, f.actor, f.project, a.ID, IntelligentOperationPreviewRequest{ProjectID: f.project, ExpectedVersion: a.Version})
	if err != nil || !preview.CanApply || len(preview.Steps) != 3 || len(preview.Defaults) == 0 {
		t.Fatal(preview, err)
	}
	req := IntelligentOperationCommandRequest{ProjectID: f.project, ExpectedVersion: a.Version, Action: "apply", PreviewSHA256: preview.PreviewSHA256, AcknowledgeDrafts: true, Reason: "Explicit review of draft changes", IdempotencyKey: newID()}
	// Fail after the first record and design have been written. No partial local
	// batch, receipt, revision, activity or analysis transition may survive.
	if _, err = f.s.DB.Exec(`CREATE TRIGGER operation_reject_knowledge BEFORE INSERT ON suma_project_knowledge BEGIN SELECT RAISE(ABORT,'fixture domain write refusal'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.CommandIntelligentOperation(ctx, f.actor, f.project, a.ID, req); err == nil {
		t.Fatal("batch unexpectedly succeeded")
	}
	for _, table := range []string{"suma_requirements", "suma_requirement_designs", "suma_project_knowledge"} {
		var count int
		if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	if _, err = f.s.DB.Exec(`DROP TRIGGER operation_reject_knowledge`); err != nil {
		t.Fatal(err)
	}
	applied, err := f.s.CommandIntelligentOperation(ctx, f.actor, f.project, a.ID, req)
	if err != nil || applied.State != "applied" || len(applied.Results) != 3 {
		t.Fatal(applied, err)
	}
	replay, err := f.s.CommandIntelligentOperation(ctx, f.actor, f.project, a.ID, req)
	if err != nil || stringify(replay.Results) != stringify(applied.Results) {
		t.Fatal(replay, err)
	}
	var state string
	if err = f.s.DB.QueryRow(`SELECT status FROM suma_requirements WHERE id=?`, applied.Results[0].ResourceID).Scan(&state); err != nil || state != "draft" {
		t.Fatal(state, err)
	}
	var published int
	if err = f.s.DB.QueryRow(`SELECT published_revision FROM suma_requirement_designs WHERE requirement_id=?`, applied.Results[0].ResourceID).Scan(&published); err != nil || published != 0 {
		t.Fatal(published, err)
	}
	if err = f.s.DB.QueryRow(`SELECT state FROM suma_project_knowledge WHERE id=?`, applied.Results[2].ResourceID).Scan(&state); err != nil || state != "draft" {
		t.Fatal(state, err)
	}
	var runs int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&runs); err != nil || runs != 0 {
		t.Fatal("adoption started execution", runs, err)
	}
	encoded, _ := json.Marshal(applied.Results)
	if strings.Contains(string(encoded), "credential") {
		t.Fatal("private authority leaked")
	}
}
