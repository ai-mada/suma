package platform

import (
	"context"
	"testing"
)

type requirementTraceabilityReader interface {
	GetRequirementTraceability(context.Context, Actor, string, string) (RequirementTraceability, error)
}

func traceabilityReader(t *testing.T, s *Service) requirementTraceabilityReader {
	t.Helper()
	r, ok := any(s).(requirementTraceabilityReader)
	if !ok {
		t.Fatal("requirement traceability service is not implemented")
	}
	return r
}

func traceabilityRead(t *testing.T, d *designIntegrationFixture) RequirementTraceability {
	t.Helper()
	out, err := traceabilityReader(t, d.f.s).GetRequirementTraceability(context.Background(), d.f.actor, d.f.project, d.req["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRequirementTraceabilityUnmappedCriteriaAndReadOnlyHTTP(t *testing.T) {
	d := newDesignIntegrationFixture(t, false)
	f := d.f
	// Editable metadata and ordinary requirement association cannot establish
	// reviewed design coverage, even when the title and covers appear to match.
	f.call("POST", "/tasks", f.body(map[string]any{"title": "Untrusted coverage", "requirement_id": d.req["id"], "data": map[string]any{"covers": []int{1}, "task_key": "access"}}), 201)
	before := designIntegrationEffects(t, d)
	out := traceabilityRead(t, d)
	if out.RequirementID != d.req["id"] || out.RequirementVersion != 1 || out.Eligibility != "none" || len(out.Criteria) != 1 || out.Criteria[0].Index != 1 || out.Criteria[0].Text != "Cross-project access is rejected" || len(out.Criteria[0].TaskKeys) != 0 || len(out.Tasks) != 0 {
		t.Fatalf("unreviewed metadata established coverage: %#v", out)
	}
	path := "/requirements/" + d.req["id"].(string) + "/traceability"
	f.call("GET", path+"?project_id="+f.project, nil, 200)
	for _, suffix := range []string{"", "?project_id=" + f.project + "&project_id=" + f.project, "?project_id=" + f.project + "&limit=20"} {
		f.call("GET", path+suffix, nil, 400)
	}
	if after := designIntegrationEffects(t, d); after != before {
		t.Fatal("traceability read changed business, adoption, execution or receipt state")
	}
}

func TestRequirementTraceabilityPublishedAdoptionAndVersionedPositions(t *testing.T) {
	d := newDesignIntegrationFixture(t, false)
	// Two equal strings remain distinct positions in this requirement version.
	d.req = d.f.call("PATCH", "/requirements/"+d.req["id"].(string), d.f.body(map[string]any{"expected_version": d.req["version"], "acceptance_criteria": []string{"Same text", "Same text"}}), 200)
	proposals := designIntegrationTasks()
	proposals[0].Covers = []int{1, 2}
	published := d.publish(designIntegrationContent(), proposals)
	out := traceabilityRead(t, d)
	if out.Eligibility != "current" || len(out.Criteria) != 2 || len(out.Tasks) != 1 || out.Tasks[0].BindingState != "not_adopted" || out.Tasks[0].TaskID != "" || out.Tasks[0].FormalAcceptance || out.Tasks[0].Run != nil || len(out.Criteria[1].TaskKeys) != 1 || out.Criteria[1].TaskKeys[0] != "access" {
		t.Fatalf("publication was confused with adoption or duplicate criteria merged: %#v", out)
	}
	adoption := d.adopt(published)
	task := d.task(adoption, "access")
	out = traceabilityRead(t, d)
	if out.Tasks[0].BindingState != "current" || out.Tasks[0].TaskID != task["id"] || out.Tasks[0].TaskVersion != 1 || out.Tasks[0].RunBasisState != "not_dispatched" || out.Tasks[0].FormalAcceptance {
		t.Fatalf("protected adoption was not projected: %#v", out)
	}
	d.req = d.f.call("PATCH", "/requirements/"+d.req["id"].(string), d.f.body(map[string]any{"expected_version": d.req["version"], "acceptance_criteria": []string{"New first position"}}), 200)
	out = traceabilityRead(t, d)
	if out.Eligibility != "requirement_changed" || out.RequirementVersion != 3 || len(out.Criteria) != 1 || out.Criteria[0].Text != "New first position" || len(out.Criteria[0].TaskKeys) != 0 || len(out.Tasks) != 0 {
		t.Fatalf("old Covers indices were mapped onto a new requirement revision: %#v", out)
	}
	d.command(d.current(), "retire")
	out = traceabilityRead(t, d)
	if out.Eligibility != "retired" || len(out.Criteria) != 1 || len(out.Criteria[0].TaskKeys) != 0 || len(out.Tasks) != 0 {
		t.Fatalf("retired publication retained current coverage: %#v", out)
	}
	if current := d.current(); current["eligibility"] != "retired" {
		t.Fatalf("retired design detail disagrees with traceability: %s", current["eligibility"])
	}
}

func TestRequirementTraceabilityExactRunReviewAndNewPublication(t *testing.T) {
	d := newDesignIntegrationFixture(t, true)
	published := d.publish(designIntegrationContent(), designIntegrationTasks())
	task := d.task(d.adopt(published), "access")
	plan := d.dispatch(task, 201)
	run := d.f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", d.f.body(map[string]any{"expected_version": plan["version"]}), 201)
	runID := run["id"].(string)
	designIntegrationCompleteProtocol(t, d, runID)
	out := traceabilityRead(t, d)
	if len(out.Tasks) != 1 || out.Tasks[0].Run == nil || out.Tasks[0].Run.RunID != runID || out.Tasks[0].Run.Tests.Succeeded != 1 || out.Tasks[0].FormalAcceptance {
		t.Fatalf("execution success was confused with formal review: %#v", out)
	}
	current, err := d.f.s.Get(context.Background(), d.f.actor, d.f.project, "tasks", task["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.f.s.Review(context.Background(), d.f.actor, d.f.project, current.ID, d.f.body(map[string]any{"expected_version": current.Version, "run_id": runID, "decision": "accepted", "reason": "Reviewed current exact design and independent test evidence"}), d.f.e); err != nil {
		t.Fatal(err)
	}
	out = traceabilityRead(t, d)
	if !out.Tasks[0].FormalAcceptance || out.Tasks[0].BindingState != "current" || out.Tasks[0].RunBasisState != "current" {
		t.Fatalf("current formal review was lost: %#v", out)
	}
	content := designIntegrationContent()
	content.Product = "New reviewed product basis"
	d.publish(content, designIntegrationTasks())
	out = traceabilityRead(t, d)
	if out.Tasks[0].BindingState != "publication_changed" || out.Tasks[0].FormalAcceptance || out.Tasks[0].Run == nil || out.Tasks[0].Run.RunID != runID {
		t.Fatalf("old adoption or old review represented a new publication: %#v", out)
	}
}

func TestRequirementTraceabilityFailsClosedOnScopeAndStorageFailure(t *testing.T) {
	d := newDesignIntegrationFixture(t, false)
	r := traceabilityReader(t, d.f.s)
	if _, err := r.GetRequirementTraceability(context.Background(), Actor{}, d.f.project, d.req["id"].(string)); err == nil {
		t.Fatal("unproved actor read traceability")
	}
	foreign := d.f.call("POST", "/projects", map[string]any{"title": "Foreign trace scope", "idempotency_key": newID()}, 201)
	if _, err := r.GetRequirementTraceability(context.Background(), d.f.actor, foreign["id"].(string), d.req["id"].(string)); err == nil {
		t.Fatal("a requirement crossed its project scope")
	}
	issued, err := d.f.s.IssueProjectCredential(context.Background(), d.f.actor, ProjectCredentialIssueRequest{ProjectID: d.f.project, Scope: "api", Label: "Traceability reader", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	apiActor, err := d.f.s.AuthenticateActor(context.Background(), issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.GetRequirementTraceability(context.Background(), apiActor, d.f.project, d.req["id"].(string)); err != nil {
		t.Fatal(err)
	}
	_, err = r.GetRequirementTraceability(context.Background(), apiActor, foreign["id"].(string), d.req["id"].(string))
	requireAuthorityStatus(t, err, 403)
	if err = logoutCredentialForTest(context.Background(), d.f.s, issued.Token); err != nil {
		t.Fatal(err)
	}
	out, err := r.GetRequirementTraceability(context.Background(), apiActor, d.f.project, d.req["id"].(string))
	requireAuthorityStatus(t, err, 401)
	if out.RequirementID != "" || len(out.Criteria) != 0 || len(out.Tasks) != 0 {
		t.Fatal("revoked credential received traceability references")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.GetRequirementTraceability(ctx, d.f.actor, d.f.project, d.req["id"].(string)); err == nil {
		t.Fatal("cancelled read returned an empty success")
	}
	if _, err := d.f.s.DB.Exec(`DROP TABLE suma_requirement_designs`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetRequirementTraceability(context.Background(), d.f.actor, d.f.project, d.req["id"].(string)); err == nil {
		t.Fatal("storage failure was disguised as an unplanned requirement")
	}
}
