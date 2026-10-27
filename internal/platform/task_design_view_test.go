package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// The assertions exercise protected service reads, not editable task metadata.
// Removing current proof, following a newer publication, or treating keep as
// adoption would expose a different caller's data or rewrite historical facts.
type taskDesignViewReader interface {
	GetTaskDesignContext(context.Context, Actor, string, string) (TaskDesignContextView, error)
	InitializeTaskDesignView(context.Context) error
}

func taskDesignViewService(t *testing.T, s *Service) taskDesignViewReader {
	t.Helper()
	reader, ok := any(s).(taskDesignViewReader)
	if !ok {
		t.Fatal("task design provenance service is not implemented")
	}
	return reader
}

func taskDesignViewFixture(t *testing.T) (*designIntegrationFixture, map[string]any, map[string]any, map[string]any) {
	t.Helper()
	d := newDesignIntegrationFixture(t, false)
	view := d.publish(designIntegrationContent(), designIntegrationTasks())
	adoption := d.adopt(view)
	task := d.task(adoption, "access")
	reader := taskDesignViewService(t, d.f.s)
	if err := reader.InitializeTaskDesignView(context.Background()); err != nil {
		t.Fatal(err)
	}
	return d, view, adoption, task
}

func taskDesignRead(t *testing.T, d *designIntegrationFixture, task map[string]any) TaskDesignContextView {
	t.Helper()
	out, err := taskDesignViewService(t, d.f.s).GetTaskDesignContext(context.Background(), d.f.actor, d.f.project, task["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func assertTaskDesignOriginal(t *testing.T, out TaskDesignContextView, adoption map[string]any, requirement string) {
	t.Helper()
	if !out.Bound || out.Context == nil || out.Requirement == nil || out.AdoptedTask == nil || out.Context.AdoptionID != adoption["id"] || out.Context.RequirementID != requirement || out.Requirement.ID != requirement || out.Requirement.Version != 1 || out.Context.Revision != int(adoption["revision"].(float64)) || out.Context.SHA256 != adoption["sha256"] || out.Context.Content.Product != designIntegrationProduct || out.AdoptedTask.Key != "access" || out.AdoptedTask.Title != "Implement order isolation" || out.AdoptedBy != adoption["created_by"] || out.AdoptedAt != adoption["created_at"] {
		t.Fatalf("original approved evidence was lost or replaced: %#v", out)
	}
}

func TestTaskDesignViewCurrentSnapshotAndUnboundForgery(t *testing.T) {
	d, published, adoption, task := taskDesignViewFixture(t)
	f := d.f
	before := taskKnowledgeEffectCounts(t, f)
	out := taskDesignRead(t, d, task)
	assertTaskDesignOriginal(t, out, adoption, d.req["id"].(string))
	if out.Eligibility != "current" || out.TaskID != task["id"] || out.TaskVersion != 1 || out.CurrentRequirementVersion != 1 || out.CurrentDesignVersion != int(adoption["version"].(float64)) || out.CurrentPublishedRevision != int(published["published_revision"].(float64)) {
		t.Fatalf("current qualification: %#v", out)
	}
	// A routine status/version update preserves the formal execution basis.
	task = f.call("PATCH", "/tasks/"+task["id"].(string), f.body(map[string]any{"expected_version": task["version"], "status": "doing", "data": map[string]any{"design": "forged replacement", "requirement_version": 999}}), 200)
	out = taskDesignRead(t, d, task)
	assertTaskDesignOriginal(t, out, adoption, d.req["id"].(string))
	if out.Eligibility != "current" || out.TaskVersion != 2 {
		t.Fatalf("routine task edit changed design qualification: %#v", out)
	}
	manual := f.call("POST", "/tasks", f.body(map[string]any{"title": task["title"], "requirement_id": d.req["id"], "data": map[string]any{"design": out.Context, "adoption_id": adoption["id"]}}), 201)
	unbound := taskDesignRead(t, d, manual)
	if unbound.Bound || unbound.Eligibility != "none" || unbound.Context != nil || unbound.Requirement != nil || unbound.AdoptedTask != nil {
		t.Fatalf("editable data or matching title forged adoption: %#v", unbound)
	}
	afterWrites := taskKnowledgeEffectCounts(t, f)
	_ = taskDesignRead(t, d, task)
	_ = taskDesignRead(t, d, manual)
	if got := taskKnowledgeEffectCounts(t, f); got != afterWrites || before.runs != got.runs || before.plans != got.plans {
		t.Fatal("design reads created business or execution effects")
	}
}

func TestTaskDesignViewStaleQualificationKeepsOriginal(t *testing.T) {
	for _, state := range []string{"task_changed", "requirement_changed", "publication_changed", "retired", "source_unavailable"} {
		t.Run(state, func(t *testing.T) {
			d, published, adoption, task := taskDesignViewFixture(t)
			f := d.f
			switch state {
			case "task_changed":
				task = f.call("PATCH", "/tasks/"+task["id"].(string), f.body(map[string]any{"expected_version": task["version"], "description": "Independent task edit"}), 200)
			case "requirement_changed":
				d.req = f.call("PATCH", "/requirements/"+d.req["id"].(string), f.body(map[string]any{"expected_version": d.req["version"], "description": "New requirement basis"}), 200)
			case "publication_changed":
				content := designIntegrationContent()
				content.Product = "A newer approved product"
				d.publish(content, designIntegrationTasks())
			case "retired":
				d.command(d.current(), "retire")
			case "source_unavailable":
				d.req = f.call("PATCH", "/requirements/"+d.req["id"].(string), f.body(map[string]any{"expected_version": d.req["version"], "status": "archived"}), 200)
			}
			before := taskKnowledgeEffectCounts(t, f)
			out := taskDesignRead(t, d, task)
			assertTaskDesignOriginal(t, out, adoption, d.req["id"].(string))
			if out.Eligibility != state || out.Context.Revision != int(published["published_revision"].(float64)) {
				t.Fatalf("wrong current qualification: want=%s got=%#v", state, out)
			}
			if after := taskKnowledgeEffectCounts(t, f); after != before {
				t.Fatal("stale design read mutated business facts")
			}
		})
	}
}

func TestTaskDesignViewExcludedUsesOriginalAdoptionAcrossKeeps(t *testing.T) {
	d := newDesignIntegrationFixture(t, false)
	tasks := append(designIntegrationTasks(), RequirementDesignTask{Key: "other", Title: "Other outcome", Priority: "medium", DependsOn: []string{}, AcceptanceCriteria: []string{"Other outcome works"}, Covers: []int{1}})
	published := d.publish(designIntegrationContent(), tasks)
	first := d.adopt(published)
	task := d.task(first, "access")
	other := d.task(first, "other")
	for n := 0; n < 3; n++ {
		other = d.f.call("PATCH", "/tasks/"+other["id"].(string), d.f.body(map[string]any{"expected_version": other["version"], "description": fmt.Sprintf("Independent other edit %d", n)}), 200)
		later := d.adopt(d.current())
		other = d.task(later, "other")
	}
	content := designIntegrationContent()
	content.Product = "New publication excludes the access task"
	exclusion := d.adopt(d.publish(content, tasks[1:]))
	task = d.task(exclusion, "access")
	reader := taskDesignViewService(t, d.f.s)
	if err := reader.InitializeTaskDesignView(context.Background()); err != nil {
		t.Fatal(err)
	}
	out := taskDesignRead(t, d, task)
	assertTaskDesignOriginal(t, out, first, d.req["id"].(string))
	if out.Eligibility != "excluded" || task["status"] != "archived" || out.CurrentPublishedRevision == out.Context.Revision {
		t.Fatalf("exclusion did not retain original adopted evidence: %#v", out)
	}
	// The relation only locates a source. A changed pointer cannot qualify text.
	if _, err := d.f.s.DB.Exec(`UPDATE suma_requirement_design_adoption_tasks SET task_version=999 WHERE adoption_id=? AND task_id=?`, first["id"], task["id"]); err != nil {
		t.Fatal(err)
	}
	unverified := taskDesignRead(t, d, task)
	if !unverified.Bound || unverified.Eligibility != "source_unavailable" || unverified.Context != nil || unverified.Requirement != nil || unverified.AdoptedTask != nil {
		t.Fatalf("unverified index fabricated evidence: %#v", unverified)
	}
}

func TestTaskDesignViewCurrentAuthorityAndExactScope(t *testing.T) {
	d, _, _, task := taskDesignViewFixture(t)
	f := d.f
	ctx := context.Background()
	reader := taskDesignViewService(t, f.s)
	issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", Label: "Design evidence reader", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := f.s.AuthenticateActor(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	if out, e := reader.GetTaskDesignContext(ctx, actor, f.project, task["id"].(string)); e != nil || out.Eligibility != "current" {
		t.Fatalf("actual project API read: %#v %v", out, e)
	}
	other := f.call("POST", "/projects", map[string]any{"title": "Foreign design project", "idempotency_key": newID()}, 201)["id"].(string)
	_, err = reader.GetTaskDesignContext(ctx, actor, other, task["id"].(string))
	requireAuthorityStatus(t, err, 403)
	_, err = reader.GetTaskDesignContext(ctx, f.actor, other, task["id"].(string))
	requireAuthorityStatus(t, err, 404)
	_, err = reader.GetTaskDesignContext(ctx, Actor{}, f.project, task["id"].(string))
	requireAuthorityStatus(t, err, 401)
	grant, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", Label: "Revoked grant reader", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	grantActor, err := f.s.AuthenticateActor(ctx, grant.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_credential_grants SET revoked=1 WHERE id=?`, grantActor.credential.grantID); err != nil {
		t.Fatal(err)
	}
	withdrawn, err := reader.GetTaskDesignContext(ctx, grantActor, f.project, task["id"].(string))
	requireAuthorityStatus(t, err, 401)
	if withdrawn.Context != nil || withdrawn.Bound {
		t.Fatal("withdrawn grant received historical design evidence")
	}
	if err = logoutCredentialForTest(ctx, f.s, issued.Token); err != nil {
		t.Fatal(err)
	}
	out, err := reader.GetTaskDesignContext(ctx, actor, f.project, task["id"].(string))
	requireAuthorityStatus(t, err, 401)
	if out.Context != nil || out.Bound {
		t.Fatal("withdrawn credential received original design")
	}
	if err = logoutCredentialForTest(ctx, f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = reader.GetTaskDesignContext(ctx, f.actor, f.project, task["id"].(string))
	requireAuthorityStatus(t, err, 401)
}

func TestTaskDesignViewUnavailableOriginalNeverUsesNewPublication(t *testing.T) {
	for _, corruption := range []string{"missing_adoption", "changed_revision"} {
		t.Run(corruption, func(t *testing.T) {
			d, _, original, task := taskDesignViewFixture(t)
			content := designIntegrationContent()
			content.Product = "Newer text must not replace unavailable evidence"
			d.publish(content, designIntegrationTasks())
			var err error
			if corruption == "missing_adoption" {
				_, err = d.f.s.DB.Exec(`DELETE FROM suma_requirement_design_adoptions WHERE id=?`, original["id"])
			} else {
				var raw string
				if err = d.f.s.DB.QueryRow(`SELECT snapshot FROM suma_requirement_design_revisions WHERE requirement_id=? AND revision=?`, d.req["id"], original["revision"]).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				var revision RequirementDesignRevision
				if err = json.Unmarshal([]byte(raw), &revision); err != nil {
					t.Fatal(err)
				}
				revision.Content.Product = "Tampered original without matching approved checksum"
				_, err = d.f.s.DB.Exec(`UPDATE suma_requirement_design_revisions SET snapshot=? WHERE requirement_id=? AND revision=?`, stringify(revision), d.req["id"], original["revision"])
			}
			if err != nil {
				t.Fatal(err)
			}
			out := taskDesignRead(t, d, task)
			if !out.Bound || out.Eligibility != "source_unavailable" || out.Context != nil || out.Requirement != nil || out.AdoptedTask != nil || out.AdoptedBy != "" || out.AdoptedAt != "" {
				t.Fatalf("unavailable source was silently replaced: %#v", out)
			}
		})
	}
}

func TestTaskDesignInitializationPreservesCurrentIndexAndSource(t *testing.T) {
	d, _, original, task := taskDesignViewFixture(t)
	f := d.f
	reader := taskDesignViewService(t, f.s)
	var source string
	if err := f.s.DB.QueryRow(`SELECT snapshot FROM suma_requirement_design_adoptions WHERE id=?`, original["id"]).Scan(&source); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_requirement_design_adoption_tasks`).Scan(&before); err != nil || before < 1 {
		t.Fatalf("missing current adoption index: %v", err)
	}
	for n := 0; n < 2; n++ {
		if err := reader.InitializeTaskDesignView(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	var after int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_requirement_design_adoption_tasks`).Scan(&after); err != nil || after != before {
		t.Fatalf("initialization changed current index: %v", err)
	}
	var retained string
	if err := f.s.DB.QueryRow(`SELECT snapshot FROM suma_requirement_design_adoptions WHERE id=?`, original["id"]).Scan(&retained); err != nil || retained != source {
		t.Fatalf("initialization changed approved source: %v", err)
	}
	assertTaskDesignOriginal(t, taskDesignRead(t, d, task), original, d.req["id"].(string))
}
