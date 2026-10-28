package platform

import (
	"strings"
	"testing"
)

// One boundary test guards JSON's case-insensitive struct decoding. Every
// rejected mutation must leave the real risk/review state unchanged; canonical
// nullable ratings remain supported. There are no model or Runner calls here.
func TestProjectRiskHTTPRejectsNonCanonicalMutationShapes(t *testing.T) {
	f := newFixture(t)
	owner := projectRiskOwnerFixture(t, f)
	request := ProjectRiskCreateRequest{ProjectID: f.project, ProjectRiskFields: ProjectRiskFields{Title: "Canonical risk boundary", Detail: "Reject aliases before domain decisions", OwnerID: owner.ID, ReviewDate: "2026-11-12", Strategy: "accept", Plan: "Review the remaining exposure explicitly", Links: []ProjectRiskReference{}}, Reason: "Owned HTTP boundary fixture", IdempotencyKey: newID()}
	created := issueRepairRuntimeDecode[ProjectRiskView](t, f.call("POST", "/risks", request, 201))
	if created.Risk.Probability != nil || created.Risk.Impact != nil || created.Risk.Score != nil {
		t.Fatal("canonical null ratings invented an assessment")
	}
	copyBody := func(value any) map[string]any { return issueRepairRuntimeDecode[map[string]any](t, value) }
	for _, alias := range []string{"TITLE", "SOURCE_ALERT_ID"} {
		t.Run("create-"+alias, func(t *testing.T) {
			body := copyBody(request)
			body["idempotency_key"] = newID()
			body[alias] = "untrusted-alias"
			if alias == "SOURCE_ALERT_ID" {
				body[alias] = strings.Repeat("a", 32)
				body["expected_alert_version"] = 1
			}
			f.call("POST", "/risks", body, 400)
		})
	}
	task := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "Canonical linked risk response"}), 201))
	save := ProjectRiskSaveRequest{ProjectID: f.project, ExpectedVersion: created.Risk.Version, ProjectRiskFields: created.Risk.ProjectRiskFields, Reason: "Keep exact link keys", IdempotencyKey: newID()}
	for _, mixed := range []bool{false, true} {
		t.Run("link-alias-"+map[bool]string{false: "missing", true: "mixed"}[mixed], func(t *testing.T) {
			body := copyBody(save)
			body["idempotency_key"] = newID()
			link := map[string]any{"Kind": "task", "id": task.ID}
			if mixed {
				link = map[string]any{"kind": "task", "id": task.ID, "ID": strings.Repeat("b", 32)}
			}
			body["links"] = []any{link}
			f.call("PATCH", "/risks/"+created.Risk.ID, body, 400)
		})
	}
	unchanged := issueRepairRuntimeDecode[ProjectRiskView](t, f.call("GET", "/risks/"+created.Risk.ID+"?project_id="+f.project, nil, 200))
	if unchanged.Risk.Version != created.Risk.Version || len(unchanged.Risk.Links) != 0 {
		t.Fatal("non-canonical mutation reached the risk domain", unchanged)
	}
	started := issueRepairRuntimeDecode[ProjectRiskView](t, f.call("POST", "/risks/"+created.Risk.ID+"/commands", ProjectRiskCommandRequest{ProjectID: f.project, ExpectedVersion: created.Risk.Version, Action: "start", Reason: "Prepare the canonical review boundary", IdempotencyKey: newID()}, 200))
	review := map[string]any{"disposition": "accepted", "residual_probability": nil, "residual_impact": nil, "rationale": "Explicit owner exposure decision", "evidence": "Owned decision fixture; no code mitigation is claimed"}
	command := ProjectRiskCommandRequest{ProjectID: f.project, ExpectedVersion: started.Risk.Version, Action: "submit_review", Reason: "Canonical explicit review", IdempotencyKey: newID()}
	for _, shape := range []string{"outer-alias", "mixed-outer", "mixed-review", "missing-residual", "explicit-null"} {
		t.Run(shape, func(t *testing.T) {
			body := copyBody(command)
			body["idempotency_key"] = newID()
			nested := copyBody(review)
			switch shape {
			case "outer-alias":
				delete(nested, "residual_probability")
				delete(nested, "residual_impact")
				body["Review"] = nested
			case "mixed-outer":
				body["review"] = nested
				body["Review"] = copyBody(review)
			case "mixed-review":
				nested["Disposition"] = "mitigated"
				body["review"] = nested
			case "missing-residual":
				delete(nested, "residual_probability")
				body["review"] = nested
			case "explicit-null":
				body["review"] = nil
			}
			f.call("POST", "/risks/"+created.Risk.ID+"/commands", body, 400)
		})
	}
	current := issueRepairRuntimeDecode[ProjectRiskView](t, f.call("GET", "/risks/"+created.Risk.ID+"?project_id="+f.project, nil, 200))
	page := issueRepairRuntimeDecode[ProjectRiskPage](t, f.call("GET", "/risks?project_id="+f.project, nil, 200))
	if current.Risk.Version != started.Risk.Version || current.Risk.State != "mitigating" || current.CurrentReview != nil || current.HistoryTotal != 0 || page.Total != 1 {
		t.Fatal("alias review created formal state", current, page)
	}
	canonical := copyBody(command)
	canonical["idempotency_key"] = newID()
	canonical["review"] = review
	accepted := issueRepairRuntimeDecode[ProjectRiskView](t, f.call("POST", "/risks/"+created.Risk.ID+"/commands", canonical, 200))
	if accepted.Risk.State != "in_review" || accepted.CurrentReview == nil || accepted.CurrentReview.ResidualProbability != nil || accepted.CurrentReview.ResidualImpact != nil || accepted.CurrentReview.ResidualScore != nil {
		t.Fatal("strict transport rejected or replaced valid nullable review fields", accepted)
	}
}
