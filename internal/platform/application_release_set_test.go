package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplicationReleaseSetDependencyClosureKeepsCyclesAndExactPins(t *testing.T) {
	a, b := newID(), newID()
	av, bv := newID(), newID()
	as, bs := strings.Repeat("a", 64), strings.Repeat("b", 64)
	inputs := map[string]ApplicationReleaseSetMemberInput{a: {ApplicationID: a, ApplicationVersionID: av, ApplicationVersionSHA256: as}, b: {ApplicationID: b, ApplicationVersionID: bv, ApplicationVersionSHA256: bs}}
	versions := map[string]ApplicationCodeVersion{a: {ID: av, Dependencies: []ApplicationDependency{{ApplicationDependencyInput: ApplicationDependencyInput{TargetApplicationID: b, TargetVersionID: bv, TargetVersionSHA256: bs}, Compatibility: "unverified"}}}, b: {ID: bv, Dependencies: []ApplicationDependency{{ApplicationDependencyInput: ApplicationDependencyInput{TargetApplicationID: a, TargetVersionID: av, TargetVersionSHA256: as}, Compatibility: "unverified"}}}}
	edges, cycles, blockers := applicationReleaseSetDependencyClosure(versions, inputs, true)
	if len(edges) != 2 || len(cycles) != 1 || len(cycles[0]) != 2 || len(blockers) != 0 {
		t.Fatal("exact cyclic application versions were forced into a DAG", edges, cycles, blockers)
	}
	_, _, blockers = applicationReleaseSetDependencyClosure(versions, inputs, false)
	if len(blockers) != 2 || blockers[0].Code != "release_set_compatibility_unverified" {
		t.Fatal("unverified compatibility was silently accepted", blockers)
	}
	known := versions[a]
	known.Dependencies[0].Compatibility = "incompatible"
	versions[a] = known
	_, _, blockers = applicationReleaseSetDependencyClosure(versions, inputs, true)
	if len(blockers) != 1 || blockers[0].Code != "release_set_dependency_incompatible" {
		t.Fatal("unknown-compatibility acknowledgement accepted a known negative", blockers)
	}
	known.Dependencies[0].Compatibility = "unverified"
	versions[a] = known
	wrong := inputs[b]
	wrong.ApplicationVersionSHA256 = as
	inputs[b] = wrong
	_, _, blockers = applicationReleaseSetDependencyClosure(versions, inputs, true)
	if len(blockers) != 1 || blockers[0].Code != "release_set_dependency_conflict" {
		t.Fatal("different immutable dependency SHA was accepted", blockers)
	}
	delete(inputs, b)
	_, _, blockers = applicationReleaseSetDependencyClosure(versions, inputs, true)
	if len(blockers) != 1 || blockers[0].Code != "release_set_dependency_missing" {
		t.Fatal("omitted required application was accepted", blockers)
	}
}
func TestApplicationReleaseSetClosedHTTPAndSafeGeneration(t *testing.T) {
	f := newFixture(t)
	q := ApplicationReleaseSetPreviewRequest{ProjectID: f.project, Label: "Review fixture", Members: []ApplicationReleaseSetMemberInput{{ApplicationID: newID(), ApplicationVersionID: newID(), ApplicationVersionSHA256: strings.Repeat("a", 64), CandidateID: newID()}}, Reason: "Explicit candidate association", AcknowledgeUnverifiedCompatibility: true}
	var body map[string]any
	json.Unmarshal([]byte(stringify(q)), &body)
	server := &Server{Service: f.s}
	request := httptest.NewRequest("POST", "/api/v1/application-release-sets/preview", nil)
	body["Label"] = body["label"]
	delete(body, "label")
	w := httptest.NewRecorder()
	server.applicationReleaseSetHTTP(w, request, f.actor, "", f.project, []string{"application-release-sets", "preview"}, body)
	if w.Code != 400 {
		t.Fatal("case-folded root key was accepted", w.Code)
	}
	body = map[string]any{}
	json.Unmarshal([]byte(stringify(q)), &body)
	body["members"] = nil
	w = httptest.NewRecorder()
	server.applicationReleaseSetHTTP(w, request, f.actor, "", f.project, []string{"application-release-sets", "preview"}, body)
	if w.Code != 400 {
		t.Fatal("null members accepted", w.Code)
	}
	generation := ApplicationReleaseSetDeploymentPreviewRequest{ProjectID: f.project, ExpectedSetSHA256: strings.Repeat("a", 64), Environments: []ApplicationReleaseSetEnvironmentInput{{ApplicationID: newID(), EnvironmentID: newID(), ExpectedEnvironmentGeneration: 9007199254740991}}, Reason: "Bounded transport fixture"}
	if applicationReleaseSetDeploymentValid(f.project, generation) {
		t.Fatal("unsafe JS integer accepted")
	}
	generation.Environments[0].ExpectedEnvironmentGeneration = 0
	generation.Environments = append(generation.Environments, generation.Environments[0])
	if applicationReleaseSetDeploymentValid(f.project, generation) {
		t.Fatal("duplicate environment/generation race accepted")
	}
	if _, err := f.s.RegisterApplicationReleaseSet(context.Background(), Actor{}, f.project, ApplicationReleaseSetRegisterRequest{ApplicationReleaseSetPreviewRequest: q, ExpectedPreviewSHA256: strings.Repeat("b", 64), IdempotencyKey: newID()}); err == nil {
		t.Fatal("unproven actor registered a release set")
	}
}
func TestApplicationReleaseSetOriginalReceiptKeepsHistoryAndReauthorizes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	member := ApplicationReleaseSetMemberInput{ApplicationID: newID(), ApplicationVersionID: newID(), ApplicationVersionSHA256: strings.Repeat("a", 64), CandidateID: newID()}
	q := ApplicationReleaseSetRegisterRequest{ApplicationReleaseSetPreviewRequest: ApplicationReleaseSetPreviewRequest{ProjectID: f.project, Label: "Historical reply fixture", Members: []ApplicationReleaseSetMemberInput{member}, AcknowledgeUnverifiedCompatibility: true, Reason: "Persisted original response identity"}, ExpectedPreviewSHA256: strings.Repeat("b", 64), IdempotencyKey: newID()}
	// This stores an immutable historical receipt protocol fixture. Missing
	// current application/candidate rows must invalidate readiness, not history.
	set := ApplicationReleaseSet{ID: newID(), ProjectID: f.project, Label: q.Label, Reason: q.Reason, Members: []ApplicationReleaseSetMember{{ApplicationReleaseSetMemberInput: member, SourceTasks: []ReleaseTaskBasis{}, CandidateTasks: []ReleaseTaskBasis{}}}, Dependencies: []ApplicationReleaseSetDependency{}, Cycles: [][]string{}, DependencyClosureSHA256: digest("fixture closure"), UnverifiedCompatibilityAcknowledged: true, RegisteredPreviewSHA256: q.ExpectedPreviewSHA256, CreatedBy: f.user.ID, CreatedAt: now()}
	set.SnapshotSHA256 = applicationReleaseSetDigest(set)
	if _, err := f.s.DB.ExecContext(ctx, f.s.q(`INSERT INTO suma_application_release_sets(id,tenant_id,project_id,label,label_key,snapshot_sha256,member_count,created_by,created_at,payload)VALUES(?,?,?,?,?,?,?,?,?,?)`), set.ID, f.user.TenantID, f.project, set.Label, digest(set.Label), set.SnapshotSHA256, 1, set.CreatedBy, set.CreatedAt, stringify(set)); err != nil {
		t.Fatal(err)
	}
	bounded, auth, close, err := f.s.planningAuthority(ctx, f.actor, f.project, "", purposeBrowserProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	hash, _, err := f.s.commandReceipt(bounded, auth, q.IdempotencyKey, map[string]any{"application_release_set_register": q})
	if err == nil {
		err = f.s.saveCommandReceipt(bounded, auth, q.IdempotencyKey, hash, ApplicationReleaseSetView{Set: set, BasisCurrent: true, Blockers: []ApplicationReleaseSetBlocker{}, AllowedActions: []string{"prepare_deployment"}, PlanGroups: []ApplicationReleaseSetPlanGroupView{}, PlanGroupsComplete: true})
	}
	if err == nil {
		err = auth.tx.Commit()
	}
	close()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := f.s.RegisterApplicationReleaseSet(ctx, f.actor, f.project, q)
	if err != nil || restored.Set.ID != set.ID || restored.Set.SnapshotSHA256 != set.SnapshotSHA256 || restored.BasisCurrent || len(restored.AllowedActions) != 0 {
		t.Fatal("receipt replay recomputed identity or retained stale readiness", restored, err)
	}
	var count int
	if err = f.s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM suma_application_release_sets`).Scan(&count); err != nil || count != 1 {
		t.Fatal("receipt replay registered another set", count, err)
	}
	if _, err = f.s.DB.ExecContext(ctx, f.s.q(`DELETE FROM suma_sessions WHERE token_hash=?`), f.actor.credential.digest); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.RegisterApplicationReleaseSet(ctx, f.actor, f.project, q)
	var api *APIError
	if !errors.As(err, &api) || (api.Status != 401 && api.Status != 403) {
		t.Fatal("revoked caller recovered privileged original response", err)
	}
}
