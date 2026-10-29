package platform

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func TestApplicationReleaseRolloutDigestPinsOriginalPlansAndDependencyOrder(t *testing.T) {
	r := ApplicationReleaseRollout{ProjectID: "project", ReleaseSetID: "set", ReleaseSetSHA256: strings.Repeat("a", 64), PlanGroupID: "group", PlanGroupSHA256: strings.Repeat("b", 64), Reason: "人审 <deploy> & exact original plans", Members: []ApplicationReleaseRolloutMember{{ApplicationID: "library", ApplicationName: "Library", PlanID: "plan-library", PlanVersion: 2, PlanRevision: 1, PlanSHA256: strings.Repeat("c", 64), ParentNodeID: "app-library", ChildRunID: "reserved-library", ChildNodeID: "deploy", DependsOn: []string{}}, {ApplicationID: "web", ApplicationName: "Web", PlanID: "plan-web", PlanVersion: 2, PlanRevision: 1, PlanSHA256: strings.Repeat("d", 64), ParentNodeID: "app-web", ChildRunID: "reserved-web", ChildNodeID: "deploy", DependsOn: []string{"library"}}}}
	preview := applicationRolloutPreviewDigest(r)
	r.ID, r.ParentRunID, r.CreatedBy, r.CreatedAt, r.StartedPreviewSHA256 = "registered", "rollout-registered", "person", now(), preview
	for i := range r.Members {
		r.Members[i].BindingID = digest(r.Members[i].ApplicationID)[:32]
	}
	if got := applicationRolloutPreviewDigest(r); got != preview {
		t.Fatal("runtime identities changed the reviewed preview", preview, got)
	}
	r.SnapshotSHA256 = applicationRolloutDigest(r)
	if r.SnapshotSHA256 == preview {
		t.Fatal("registered execution identity was not bound")
	}
	w := rolloutWorkflow(r)
	if err := dag.Compile(w); err != nil {
		t.Fatal(err)
	}
	if len(w.Nodes[1].DependsOn) != 1 || w.Nodes[1].DependsOn[0] != "app-library" {
		t.Fatal("application dependency was not reversed into execution order", w)
	}
	changed := r
	changed.Members = append([]ApplicationReleaseRolloutMember{}, r.Members...)
	changed.Members[1].PlanVersion++
	if applicationRolloutPreviewDigest(changed) == preview {
		t.Fatal("changed original approval version reused old preview")
	}
	changed = r
	changed.Members = append([]ApplicationReleaseRolloutMember{}, r.Members...)
	changed.Members[1].ChildRunID = "other-reserved-run"
	if applicationRolloutPreviewDigest(changed) == preview {
		t.Fatal("different original child execution reused old preview")
	}
	r.Members[0].DependsOn = []string{"web"}
	if err := dag.Compile(rolloutWorkflow(r)); err == nil {
		t.Fatal("cyclic application dependencies were silently flattened")
	}
}

func TestApplicationReleaseRolloutStrictHTTPAndAPIReadOnly(t *testing.T) {
	f := newFixture(t)
	server := &Server{Service: f.s}
	q := ApplicationReleaseRolloutCommandRequest{ProjectID: f.project, ExpectedRunVersion: 1, ExpectedRolloutSHA256: strings.Repeat("a", 64), Action: "pause", Reason: "Stop new child dispatch", IdempotencyKey: newID()}
	for _, bad := range []string{"unknown-key", "null-version", "fractional-version", "missing-preview-key"} {
		var body map[string]any
		if err := json.Unmarshal([]byte(stringify(q)), &body); err != nil {
			t.Fatal(err)
		}
		switch bad {
		case "unknown-key":
			body["RunVersion"] = 1
		case "null-version":
			body["expected_run_version"] = nil
		case "fractional-version":
			body["expected_run_version"] = 1.5
		case "missing-preview-key":
			delete(body, "expected_preview_sha256")
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/v1/application-release-sets/set/rollouts/rollout/commands", nil)
		server.applicationReleaseRolloutHTTP(w, r, f.actor, "", f.project, []string{"application-release-sets", "set", "rollouts", "rollout", "commands"}, body)
		if w.Code != 400 {
			t.Fatal("ambiguous command accepted", bad, w.Code, w.Body.String())
		}
	}
	credential, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	api, err := f.s.AuthenticateActor(context.Background(), credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/application-release-sets/set/rollouts", nil)
	server.applicationReleaseRolloutHTTP(w, r, api, "api", f.project, []string{"application-release-sets", "set", "rollouts"}, map[string]any{})
	if w.Code != 403 {
		t.Fatal("read-only API could authorize an autonomous rollout", w.Code)
	}
}

func TestApplicationReleaseRolloutCommandsRequireExactLiveDomainPermit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.s.InitializeApplicationReleaseRollouts(ctx); err != nil {
		t.Fatal(err)
	}
	bounded, auth, close, err := f.s.planningAuthority(ctx, f.actor, f.project, "", purposeBrowserProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	id, parent, child, binding, plan := newID(), "rollout-"+newID(), "child-"+newID(), newID(), newID()
	// Only the ownership boundary is under test. These are explicit database
	// protocol fixtures, not successful approvals, deployments or health proof.
	if _, err = auth.tx.ExecContext(bounded, f.s.q(`INSERT INTO suma_application_release_rollouts(id,tenant_id,project_id,set_id,group_id,parent_run_id,snapshot_sha256,starter_hash,starter_id,created_at,payload)VALUES(?,?,?,?,?,?,?,?,?,?,?)`), id, f.user.TenantID, f.project, newID(), newID(), parent, strings.Repeat("a", 64), f.actor.credential.digest, f.user.ID, now(), `{}`); err != nil {
		t.Fatal(err)
	}
	if _, err = auth.tx.ExecContext(bounded, f.s.q(`INSERT INTO suma_application_release_rollout_members(binding_id,tenant_id,project_id,rollout_id,parent_run_id,parent_node_id,child_run_id,plan_id)VALUES(?,?,?,?,?,?,?,?)`), binding, f.user.TenantID, f.project, id, parent, "app", child, plan); err != nil {
		t.Fatal(err)
	}
	if err = f.s.AuthorizeBoundRunCommandTx(bounded, auth.tx, f.user.TenantID, f.project, parent, "resume"); err == nil {
		t.Fatal("generic run command bypassed reviewed resume")
	}
	applicationRolloutCommandPermits.Store(auth.tx, applicationRolloutCommandPermit{Service: f.s, Authority: auth, ID: id, Action: "pause"})
	defer applicationRolloutCommandPermits.Delete(auth.tx)
	if err = f.s.AuthorizeBoundRunCommandTx(bounded, auth.tx, f.user.TenantID, f.project, parent, "pause"); err != nil {
		t.Fatal("actual same-transaction domain authority was lost", err)
	}
	if err = f.s.AuthorizeBoundRunCommandTx(bounded, auth.tx, f.user.TenantID, f.project, parent, "resume"); err == nil {
		t.Fatal("pause permit authorized resume")
	}
	if err = f.s.AuthorizeBoundRunCommandTx(bounded, auth.tx, f.user.TenantID, f.project, child, "cancel"); err == nil {
		t.Fatal("direct child mutation bypassed parent authority")
	}
	if err = f.s.guardOriginalReleasePlanOwnershipTx(bounded, auth.tx, auth.currentPrincipal, f.project, plan); err == nil {
		t.Fatal("direct Execute raced a protected owned plan")
	}
}
