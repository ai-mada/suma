package platform

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

// Only ordinary publication, result reporting and formal Review create this
// source. The supplied JUnit/workspace proofs are a typed contract fixture,
// not a claim that a real repository or external vendor was executed.
func deliveryKnowledgeAcceptedQualityFixture(t *testing.T) (*qualityPlanFixture, dag.Assignment, Record) {
	t.Helper()
	h := newQualityPlanFixture(t, true)
	a := h.run(t)
	h.f.bearer("POST", "/runner/results", h.token, h.f.body(map[string]any{"result": h.result(a, `<testsuite tests="1"><testcase classname="delivery" name="contract"/></testsuite>`)}), 200)
	var current Record
	if err := json.Unmarshal([]byte(stringify(h.f.call("GET", "/tasks/"+h.task.ID+"?project_id="+h.f.project, nil, 200))), &current); err != nil {
		t.Fatal(err)
	}
	accepted, err := h.f.s.Review(context.Background(), h.f.actor, h.f.project, h.task.ID, h.f.body(map[string]any{"expected_version": current.Version, "run_id": a.RunID, "decision": "accepted", "reason": "Explicit review of the published case and persisted test report"}), h.f.e)
	if err != nil {
		t.Fatal(err)
	}
	return h, a, accepted
}

func TestDeliveryKnowledgeExactFormalQualitySource(t *testing.T) {
	h, a, accepted := deliveryKnowledgeAcceptedQualityFixture(t)
	f, ctx := h.f, context.Background()
	view, err := f.s.GetDeliveryKnowledgeSource(ctx, f.actor, f.project, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	source := view.Source
	if view.ProjectID != f.project || source.Kind != "accepted_delivery" || source.ID != accepted.ID || source.Version != accepted.Version || source.Title != accepted.Title || source.Delivery == nil || source.SHA256 != digest(source.Quote) {
		t.Fatal("source lost its exact formal task identity", view)
	}
	basis := source.Delivery
	if basis.Protocol != "suma-accepted-delivery-v1" || basis.Task.ID != accepted.ID || basis.Task.Version != accepted.Version || basis.Task.RunID != a.RunID || len(basis.Tests) != 1 || basis.Candidates == nil || len(basis.Candidates) != 0 {
		t.Fatal("test-only delivery became code or lost exact evidence", basis)
	}
	var reviewedVersion int
	if err = f.s.DB.QueryRow(`SELECT task_version FROM suma_reviews WHERE id=? AND task_id=? AND run_id=? AND decision='accepted'`, basis.Task.ReviewID, accepted.ID, a.RunID).Scan(&reviewedVersion); err != nil || reviewedVersion+1 != accepted.Version {
		t.Fatal("accepted basis did not bind the pre-review task version", reviewedVersion, err)
	}
	proof := basis.Tests[0]
	var raw string
	if err = f.s.DB.QueryRow(`SELECT result FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=?`, a.RunID, a.NodeID, a.Attempt, a.Fence, a.WorkerID).Scan(&raw); err != nil || proof.ResultSHA256 != digest(raw) {
		t.Fatal("result digest was not the exact persisted result", proof, err)
	}
	if proof.RunID != a.RunID || proof.NodeID != a.NodeID || proof.Attempt != a.Attempt || proof.Fence != a.Fence || proof.WorkerID != a.WorkerID || proof.PlanID != h.plan.ID || proof.PlanRevision != h.plan.Published.Revision || proof.PlanSHA256 != h.plan.Published.SHA256 || proof.ReportID == "" || !contentDigest.MatchString(proof.ReportSHA256) {
		t.Fatal("quality evidence identity was weakened", proof)
	}
	copy := *basis
	copy.SHA256 = ""
	if basis.SHA256 != digest(stringify(copy)) || !strings.Contains(source.Quote, basis.SHA256) || strings.Contains(stringify(source), "<testsuite") || strings.Contains(stringify(source), `"xml"`) || strings.Contains(stringify(source), `"output"`) {
		t.Fatal("source hash or safe metadata projection is wrong", source)
	}
	repeated, err := f.s.GetDeliveryKnowledgeSource(ctx, f.actor, f.project, accepted.ID)
	if err != nil || stringify(repeated) != stringify(view) {
		t.Fatal("read changed or regenerated delivery facts", repeated, err)
	}
	ctx, auth, close, err := f.s.planningAuthority(ctx, f.actor, f.project, accepted.ID, purposeProjectRead)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	snapshot, err := f.s.resolveDeliveryKnowledgeSourceTx(ctx, auth.tx, f.user.TenantID, f.project, accepted.ID, false)
	if err != nil || stringify(snapshot) != stringify(source) {
		t.Fatal("snapshot and current proof disagree", snapshot, err)
	}
}

func TestDeliveryKnowledgeSuccessfulRunStillRequiresFormalReview(t *testing.T) {
	h := newQualityPlanFixture(t, true)
	a := h.run(t)
	h.f.bearer("POST", "/runner/results", h.token, h.f.body(map[string]any{"result": h.result(a, `<testsuite tests="1"><testcase classname="delivery" name="contract"/></testsuite>`)}), 200)
	_, err := h.f.s.GetDeliveryKnowledgeSource(context.Background(), h.f.actor, h.f.project, h.task.ID)
	requireAuthorityStatus(t, err, 409)
}

func TestDeliveryKnowledgeCurrentSourceDrift(t *testing.T) {
	for _, scenario := range []string{"task", "quality-plan"} {
		t.Run(scenario, func(t *testing.T) {
			h, _, accepted := deliveryKnowledgeAcceptedQualityFixture(t)
			f := h.f
			original, err := f.s.GetDeliveryKnowledgeSource(context.Background(), f.actor, f.project, accepted.ID)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "task" {
				f.call("PATCH", "/tasks/"+accepted.ID, f.body(map[string]any{"expected_version": accepted.Version, "description": "New business scope requires a fresh review"}), 200)
			} else {
				_, err = f.s.CommandTestPlan(context.Background(), f.actor, f.project, h.plan.ID, TestPlanCommandRequest{ProjectID: f.project, ExpectedVersion: h.plan.Version, Action: "retire", Reason: "Retired quality authority does not certify a prior run", IdempotencyKey: newID()})
				if err != nil {
					t.Fatal(err)
				}
			}
			_, err = f.s.GetDeliveryKnowledgeSource(context.Background(), f.actor, f.project, accepted.ID)
			requireAuthorityStatus(t, err, 409)
			if original.Source.Delivery.Task.ReviewID == "" || original.Source.Delivery.SHA256 == "" {
				t.Fatal("source drift erased the original immutable observation")
			}
		})
	}
}

func TestDeliveryKnowledgeSealedCandidateIdentity(t *testing.T) {
	f, token, request := candidateTransferFixture(t)
	accepted := compositionAcceptSource(t, f, token, request)
	view, err := f.s.GetDeliveryKnowledgeSource(context.Background(), f.actor, f.project, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Source.Delivery.Candidates) != 1 || len(view.Source.Delivery.Tests) != 1 {
		t.Fatal("exact candidate/test identity was lost", view)
	}
	candidate := view.Source.Delivery.Candidates[0]
	if candidate.Kind != "sealed_bundle" || candidate.RunID != request.Assignment.RunID || candidate.NodeID != request.Assignment.NodeID || candidate.Attempt != request.Assignment.Attempt || candidate.Fence != request.Assignment.Fence || candidate.WorkerID != request.Assignment.WorkerID || candidate.HeadCommit != request.HeadCommit || candidate.EvidenceSHA256 != request.EvidenceSHA256 || candidate.TransferID == "" || candidate.BundleSHA256 != request.BundleSHA256 {
		t.Fatal("candidate projection invented or changed a sealed fact", candidate)
	}
}

func TestDeliveryKnowledgeWorkspaceRevisionDoesNotClaimSealedBundle(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Reviewed workspace-only delivery"}), 201)
	wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "Exact workspace candidate and independent test", "nodes": []any{map[string]any{"id": "code", "type": "command", "command": []string{"edit"}}, map[string]any{"id": "test", "type": "test", "command": []string{"verify"}, "depends_on": []string{"code"}}}}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": wf["id"], "task_id": task["id"]}), 201)
	for _, worker := range []string{"workspace-producer", "independent-test-worker"} {
		a, err := f.e.Claim(ctx, f.user.TenantID, f.project, worker, toolFixtureCapabilities(t, f))
		if err != nil || a == nil {
			t.Fatal(a, err)
		}
		proof := reviewWorkspaceEvidence("c", a.Type == "test")
		if a.Type == "test" {
			proof["base_commit"] = proof["head_commit"]
		}
		if err = f.e.Complete(ctx, f.user.TenantID, f.project, dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), WorkspaceEvidence: proof}); err != nil {
			t.Fatal(err)
		}
	}
	current := f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
	f.call("POST", "/tasks/"+task["id"].(string)+"/review", f.body(map[string]any{"expected_version": current["version"], "run_id": run["id"], "decision": "accepted", "reason": "Explicitly review the same workspace candidate and separate worker test"}), 200)
	view, err := f.s.GetDeliveryKnowledgeSource(ctx, f.actor, f.project, task["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Source.Delivery.Candidates) != 1 || len(view.Source.Delivery.Tests) != 1 {
		t.Fatal(view)
	}
	proof := view.Source.Delivery.Candidates[0]
	if proof.Kind != "workspace_revision" || proof.TransferID != "" || proof.BundleSHA256 != "" || proof.HeadCommit != strings.Repeat("c", 40) || proof.WorkerID == view.Source.Delivery.Tests[0].WorkerID {
		t.Fatal("workspace fact invented a sealed bundle or lost the independent test", proof)
	}
}

func TestDeliveryKnowledgeCurrentAuthorityAndHTTPQuery(t *testing.T) {
	h, _, accepted := deliveryKnowledgeAcceptedQualityFixture(t)
	f := h.f
	server := &Server{Service: f.s}
	for _, query := range []string{"project_id=" + f.project + "&project_id=" + f.project, "project_id=" + f.project + "&cursor=anything", "project_id=" + newID()} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/tasks/"+accepted.ID+"/delivery-knowledge-source?"+query, nil)
		server.deliveryKnowledgeSourceHTTP(w, r, f.actor, f.project, accepted.ID)
		if w.Code != 400 {
			t.Fatal("query admitted unrelated or duplicate scope", query, w.Code)
		}
	}
	w := httptest.NewRecorder()
	server.deliveryKnowledgeSourceHTTP(w, httptest.NewRequest("GET", "/?project_id="+f.project, nil), f.actor, f.project, accepted.ID)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("authorized preview is unavailable or cacheable", w.Code, w.Body.String())
	}
	_, err := f.s.GetDeliveryKnowledgeSource(context.Background(), f.actor, newID(), accepted.ID)
	requireAuthorityStatus(t, err, 404)
	_, err = f.s.GetDeliveryKnowledgeSource(context.Background(), f.actor, f.project, newID())
	requireAuthorityStatus(t, err, 404)
	if err = logoutCredentialForTest(context.Background(), f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.GetDeliveryKnowledgeSource(context.Background(), f.actor, f.project, accepted.ID)
	requireAuthorityStatus(t, err, 401)
}

func TestDeliveryKnowledgeEvidenceBoundsAndCanonicalOrder(t *testing.T) {
	basis := DeliveryKnowledgeBasis{Tests: []DeliveryKnowledgeTestEvidence{{RunID: "run", NodeID: "z"}, {RunID: "run", NodeID: "a"}}, Candidates: []DeliveryKnowledgeCandidateEvidence{}}
	if err := deliveryKnowledgeFinalizeBasis(&basis); err != nil || basis.Tests[0].NodeID != "a" || basis.Protocol != "suma-accepted-delivery-v1" {
		t.Fatal("evidence ordering is not deterministic", basis, err)
	}
	previous := basis.SHA256
	if err := deliveryKnowledgeFinalizeBasis(&basis); err != nil || basis.SHA256 != previous {
		t.Fatal("rehashing included its previous digest", basis, err)
	}
	for _, counts := range [][2]int{{257, 0}, {0, 129}} {
		over := DeliveryKnowledgeBasis{Tests: make([]DeliveryKnowledgeTestEvidence, counts[0]), Candidates: make([]DeliveryKnowledgeCandidateEvidence, counts[1])}
		requireAuthorityStatus(t, deliveryKnowledgeFinalizeBasis(&over), 409)
	}
}
