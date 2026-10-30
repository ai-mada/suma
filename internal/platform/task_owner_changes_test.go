package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func ownerChangeRequest(t *testing.T, h *handoffFixture, owner Record, tasks ...Record) TaskOwnerChangeRequest {
	t.Helper()
	selections := make([]TaskOwnerSelection, 0, len(tasks))
	for _, task := range tasks {
		selections = append(selections, TaskOwnerSelection{TaskID: task.ID, ExpectedVersion: task.Version})
	}
	req := TaskOwnerChangePreviewRequest{ProjectID: h.f.project, OwnerID: owner.ID, ExpectedOwnerVersion: owner.Version, Tasks: selections}
	preview, err := h.f.s.PreviewTaskOwnerChanges(context.Background(), h.f.actor, h.f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	return TaskOwnerChangeRequest{ProjectID: req.ProjectID, OwnerID: req.OwnerID, ExpectedOwnerVersion: req.ExpectedOwnerVersion, Tasks: req.Tasks, PreviewSHA256: preview.PreviewSHA256, AcknowledgeInvalidation: true, IdempotencyKey: newID()}
}

func ownerChangeRaw(t *testing.T, f *fixture, path, raw string, want int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1"+path, strings.NewReader(raw))
	req.AddCookie(f.cookie)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Suma-Expected-User", f.view.ID)
	req.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	if w.Code != want {
		t.Fatalf("owner change status %d want %d: %s", w.Code, want, w.Body.String())
	}
}

func ownerChangeSnapshot(t *testing.T, f *fixture) map[string]any {
	t.Helper()
	out := map[string]any{}
	for _, table := range []string{"suma_notification_inbox", "suma_operations", "suma_activity"} {
		var count int
		if err := f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		out[table] = count
	}
	var sequence int64
	if err := f.s.DB.QueryRow(`SELECT last_sequence FROM suma_notification_inbox_sequences WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&sequence); err != nil {
		t.Fatal(err)
	}
	out["sequence"] = sequence
	tasks, err := f.s.List(context.Background(), f.actor, f.project, "tasks")
	if err != nil {
		t.Fatal(err)
	}
	out["tasks"] = tasks
	return out
}

// Dropping the shared update's assignment notification or acceptance reset must
// fail this real HTTP/SQLite test; unchanged owners must not get empty writes.
func TestTaskOwnerChangesHTTPPreviewMixedOwnersAndOriginalReceipt(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	unchanged := inboxCreateTask(t, h, "Already assigned", h.resource.ID, "bulk150-unchanged-task")
	body := map[string]any{"project_id": f.project, "owner_id": h.resource.ID, "expected_owner_version": h.resource.Version, "tasks": []any{
		map[string]any{"task_id": h.task.ID, "expected_version": h.task.Version},
		map[string]any{"task_id": unchanged.ID, "expected_version": unchanged.Version},
	}}
	before := inboxPage(t, f, h.receiver, "inbox", 20, "").Total
	beforePreview := ownerChangeSnapshot(t, f)
	preview := f.call("POST", "/task-owner-changes/preview", body, 200)
	if preview["changed_count"] != float64(1) || preview["unchanged_count"] != float64(1) || preview["can_apply"] != true {
		t.Fatal(preview)
	}
	if len(preview["preview_sha256"].(string)) != 64 || inboxPage(t, f, h.receiver, "inbox", 20, "").Total != before {
		t.Fatal("preview mutated assignment facts", preview)
	}
	if after := ownerChangeSnapshot(t, f); !reflect.DeepEqual(beforePreview, after) {
		t.Fatal("preview wrote tasks, activity or receipts", beforePreview, after)
	}
	body["preview_sha256"] = preview["preview_sha256"]
	body["acknowledge_invalidation"] = true
	body["idempotency_key"] = "bulk150-mixed-original"
	ack := f.call("POST", "/task-owner-changes", body, 200)
	changes := ack["changes"].([]any)
	if len(changes) != 2 || ack["operation_id"] != "bulk150-mixed-original" {
		t.Fatal(ack)
	}
	first, second := changes[0].(map[string]any), changes[1].(map[string]any)
	if first["task_id"] != h.task.ID || first["version"] != float64(2) || first["owner_changed"] != true || first["status"] != "in_progress" || second["task_id"] != unchanged.ID || second["version"] != float64(1) || second["owner_changed"] != false {
		t.Fatal(ack)
	}
	if inboxPage(t, f, h.receiver, "inbox", 20, "").Total != before+1 {
		t.Fatal("missing exact assignment notification")
	}
	f.call("POST", "/task-owner-changes", body, 200)
	if inboxPage(t, f, h.receiver, "inbox", 20, "").Total != before+1 {
		t.Fatal("original recovery duplicated notification")
	}
	f.call("PATCH", "/tasks/"+h.task.ID, f.body(map[string]any{"expected_version": 2, "title": "Later task edit"}), 200)
	replay := f.call("POST", "/task-owner-changes", body, 200)
	if !reflect.DeepEqual(ack, replay) {
		t.Fatal("recovery returned current state instead of the original receipt", ack, replay)
	}
}

func TestTaskOwnerChangesSelectedAcceptedDescendantsHaveFinalReceipts(t *testing.T) {
	for _, unchanged := range []bool{false, true} {
		t.Run(fmt.Sprint("unchanged_owner_", unchanged), func(t *testing.T) {
			h := newHandoffFixture(t)
			f := h.f
			owner := h.owner.ID
			if unchanged {
				owner = h.resource.ID
			}
			child, err := f.s.Create(context.Background(), f.actor, f.project, "tasks", f.body(map[string]any{"title": "Selected accepted child", "owner_id": owner, "depends_on": []any{h.task.ID}}))
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{h.task.ID, child.ID} {
				// Seed only the invalidation precondition, not a claimed execution.
				if _, err = f.s.DB.Exec(`UPDATE suma_tasks SET status='accepted',run_id='preserved-run',data='{"verified_run_id":"fixed-evidence","run_task_basis":"original-basis"}' WHERE id=?`, id); err != nil {
					t.Fatal(err)
				}
			}
			req := ownerChangeRequest(t, h, h.resource, h.task, child)
			result, err := f.s.ChangeTaskOwners(context.Background(), f.actor, f.project, req)
			if err != nil || len(result.Changes) != 2 {
				t.Fatal(result, err)
			}
			for i, id := range []string{h.task.ID, child.ID} {
				current, e := f.s.Get(context.Background(), f.actor, f.project, "tasks", id)
				if e != nil || current.Version != 2 || current.Status != "in_progress" || current.OwnerID != h.resource.ID || current.RunID != "preserved-run" || current.Data["verified_run_id"] != nil || current.Data["run_task_basis"] != "original-basis" || result.Changes[i].Version != 2 || result.Changes[i].Status != "in_progress" {
					t.Fatal("stale acceptance, false selected CAS or stale receipt", current, result, e)
				}
			}
			if result.Changes[1].OwnerChanged == unchanged {
				t.Fatal("unchanged owner receipt lost its distinct fact", result)
			}
		})
	}
}

func TestTaskOwnerChangesAnySelectedVersionConflictWritesNothing(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	second := inboxCreateTask(t, h, "Unchanged but stale", h.resource.ID, "bulk150-stale-unchanged")
	req := ownerChangeRequest(t, h, h.resource, h.task, second)
	if _, err := f.s.Update(context.Background(), f.actor, f.project, "tasks", second.ID, f.body(map[string]any{"expected_version": second.Version, "title": "Concurrent update"})); err != nil {
		t.Fatal(err)
	}
	before := ownerChangeSnapshot(t, f)
	_, err := f.s.ChangeTaskOwners(context.Background(), f.actor, f.project, req)
	handoffWantStatus(t, err, 409)
	if after := ownerChangeSnapshot(t, f); !reflect.DeepEqual(before, after) {
		t.Fatal("stale unchanged selection permitted partial write", before, after)
	}
}

func TestTaskOwnerChangesNotificationFailureRollsBackEntireBatch(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	second := inboxCreateTask(t, h, "Later projection fails", h.owner.ID, "bulk150-rollback-second")
	req := ownerChangeRequest(t, h, h.resource, h.task, second)
	before := ownerChangeSnapshot(t, f)
	trigger := fmt.Sprintf(`CREATE TRIGGER bulk150_reject_projection BEFORE INSERT ON suma_notification_inbox WHEN NEW.task_id='%s' BEGIN SELECT RAISE(ABORT,'owned batch projection failure'); END`, second.ID)
	if _, err := f.s.DB.Exec(trigger); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ChangeTaskOwners(context.Background(), f.actor, f.project, req); err == nil {
		t.Fatal("batch committed without its second assignment notification")
	}
	if after := ownerChangeSnapshot(t, f); !reflect.DeepEqual(before, after) {
		t.Fatal("failed batch retained task, sequence, activity or receipt changes", before, after)
	}
}

func TestTaskOwnerChangesNoopPreviewAndCommitPreserveAcceptedTask(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET status='accepted',version=2147483647,data='{"verified_run_id":"fixed-evidence"}' WHERE id=?`, h.task.ID); err != nil {
		t.Fatal(err)
	}
	h.task.Version = 2147483647
	req := ownerChangeRequest(t, h, h.owner, h.task)
	preview, err := f.s.PreviewTaskOwnerChanges(context.Background(), f.actor, f.project, TaskOwnerChangePreviewRequest{ProjectID: req.ProjectID, OwnerID: req.OwnerID, ExpectedOwnerVersion: req.ExpectedOwnerVersion, Tasks: req.Tasks})
	if err != nil || preview.CanApply || preview.ChangedCount != 0 || preview.UnchangedCount != 1 || preview.Tasks[0].WillChange {
		t.Fatal(preview, err)
	}
	before := ownerChangeSnapshot(t, f)
	_, err = f.s.ChangeTaskOwners(context.Background(), f.actor, f.project, req)
	handoffWantStatus(t, err, 422)
	if after := ownerChangeSnapshot(t, f); !reflect.DeepEqual(before, after) {
		t.Fatal("empty update removed accepted task evidence", before, after)
	}
}

func TestTaskOwnerChangesTargetMustStillBeCanonicalActiveAndCurrent(t *testing.T) {
	for name, statement := range map[string]string{
		"resource_version": `UPDATE suma_members SET version=version+1 WHERE id=?`,
		"inactive":         `UPDATE suma_members SET status='inactive' WHERE id=?`,
		"nonhuman":         `UPDATE suma_members SET kind='agent' WHERE id=?`,
		"noncanonical":     `DELETE FROM suma_human_resources WHERE resource_id=?`,
		"binding_mismatch": `UPDATE suma_members SET data='{"user_id":"another-user"}' WHERE id=?`,
	} {
		t.Run(name, func(t *testing.T) {
			h := newHandoffFixture(t)
			f := h.f
			req := ownerChangeRequest(t, h, h.resource, h.task)
			if _, err := f.s.DB.Exec(statement, h.resource.ID); err != nil {
				t.Fatal(err)
			}
			before := ownerChangeSnapshot(t, f)
			_, err := f.s.ChangeTaskOwners(context.Background(), f.actor, f.project, req)
			handoffWantStatus(t, err, 409)
			if after := ownerChangeSnapshot(t, f); !reflect.DeepEqual(before, after) {
				t.Fatal("ineligible owner retained batch effects", before, after)
			}
		})
	}
}

func TestTaskOwnerChangesPreviewDigestBindsCurrentFactsAndOrder(t *testing.T) {
	for _, change := range []string{"owner_name", "run_snapshot", "task_order"} {
		t.Run(change, func(t *testing.T) {
			h := newHandoffFixture(t)
			f := h.f
			second := inboxCreateTask(t, h, "Second explicit task", h.owner.ID, "bulk150-digest-second")
			req := ownerChangeRequest(t, h, h.resource, h.task, second)
			switch change {
			case "owner_name":
				if _, err := f.s.DB.Exec(`UPDATE suma_members SET title='Changed preview label' WHERE id=?`, h.resource.ID); err != nil {
					t.Fatal(err)
				}
			case "run_snapshot":
				if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET run_id='another-current-run' WHERE id=?`, h.task.ID); err != nil {
					t.Fatal(err)
				}
			case "task_order":
				req.Tasks[0], req.Tasks[1] = req.Tasks[1], req.Tasks[0]
			}
			before := ownerChangeSnapshot(t, f)
			_, err := f.s.ChangeTaskOwners(context.Background(), f.actor, f.project, req)
			handoffWantStatus(t, err, 409)
			if after := ownerChangeSnapshot(t, f); !reflect.DeepEqual(before, after) {
				t.Fatal("stale preview committed", before, after)
			}
		})
	}
}

func TestTaskOwnerChangesReceiptReplayRequiresOriginalBodyAndCurrentCaller(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	req := ownerChangeRequest(t, h, h.resource, h.task)
	original, err := f.s.ChangeTaskOwners(context.Background(), f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_members SET status='inactive',version=version+1 WHERE id=?`, h.resource.ID); err != nil {
		t.Fatal(err)
	}
	replay, err := f.s.ChangeTaskOwners(context.Background(), f.actor, f.project, req)
	if err != nil || !reflect.DeepEqual(original, replay) {
		t.Fatal("legitimate later owner change hid original receipt", original, replay, err)
	}
	changed := req
	changed.AcknowledgeInvalidation = false
	_, err = f.s.ChangeTaskOwners(context.Background(), f.actor, f.project, changed)
	handoffWantStatus(t, err, 409)
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	_, err = f.s.ChangeTaskOwners(context.Background(), f.actor, f.project, req)
	handoffWantStatus(t, err, 401)
}

func TestTaskOwnerChangesDirectoryIsPagedCanonicalLiteralAndWriteAuthorized(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	if _, err := f.s.Update(ctx, f.actor, f.project, "members", h.owner.ID, f.body(map[string]any{"expected_version": h.owner.Version, "title": "A%literal"})); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	page, err := f.s.ListTaskOwnerTargets(ctx, f.actor, f.project, TaskOwnerTargetQuery{Limit: 1})
	if err != nil || page.Total != 2 || len(page.Items) != 1 || page.Items[0].ResourceID != h.owner.ID || page.NextCursor == "" {
		t.Fatal(page, err)
	}
	last, err := f.s.ListTaskOwnerTargets(ctx, f.actor, f.project, TaskOwnerTargetQuery{Limit: 1, Cursor: page.NextCursor})
	if err != nil || last.Total != 2 || len(last.Items) != 1 || last.Items[0].ResourceID != h.resource.ID || last.NextCursor != "" {
		t.Fatal("viewer target was omitted or paging repeated", last, err)
	}
	literal, err := f.s.ListTaskOwnerTargets(ctx, f.actor, f.project, TaskOwnerTargetQuery{Q: "%"})
	if err != nil || literal.Total != 1 || literal.Items[0].ResourceID != h.owner.ID {
		t.Fatal("wildcard was not treated as literal", literal, err)
	}
	_, err = f.s.ListTaskOwnerTargets(ctx, h.receiver, f.project, TaskOwnerTargetQuery{})
	handoffWantStatus(t, err, 403)
	_, err = f.s.ListTaskOwnerTargets(ctx, f.actor, f.project, TaskOwnerTargetQuery{Limit: 2, Cursor: page.NextCursor})
	handoffWantStatus(t, err, 400)
	raw, _ := base64.RawURLEncoding.DecodeString(page.NextCursor)
	var cursor map[string]any
	if err = json.Unmarshal(raw, &cursor); err != nil {
		t.Fatal(err)
	}
	cursor["extra"] = true
	_, err = f.s.ListTaskOwnerTargets(ctx, f.actor, f.project, TaskOwnerTargetQuery{Limit: 1, Cursor: base64.RawURLEncoding.EncodeToString([]byte(stringify(cursor)))})
	handoffWantStatus(t, err, 400)
	f.call("GET", "/task-owner-targets?project_id="+f.project+"&limit=1", nil, 200)
	for _, query := range []string{"&limit=01", "&limit=1.0", "&limit=101", "&limit=1&limit=2", "&project_id=" + f.project, "&all=true"} {
		f.call("GET", "/task-owner-targets?project_id="+f.project+query, nil, 400)
	}
}

func TestTaskOwnerChangesHTTPClosedExactVersionsAndSelectionBounds(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	valid := TaskOwnerChangePreviewRequest{ProjectID: f.project, OwnerID: h.resource.ID, ExpectedOwnerVersion: h.resource.Version, Tasks: []TaskOwnerSelection{{TaskID: h.task.ID, ExpectedVersion: h.task.Version}}}
	body := stringify(valid)
	for _, raw := range []string{
		strings.Replace(body, `"expected_owner_version":1`, `"expected_owner_version":1.0000000000000001`, 1),
		strings.Replace(body, `"expected_version":1`, `"expected_version":9007199254740992.1`, 1),
		strings.Replace(body, `"expected_version":1`, `"expected_version":2147483648`, 1),
		strings.Replace(body, `"expected_version":1`, `"expected_version":null`, 1),
		strings.Replace(body, `"expected_version":1`, `"ExpectedVersion":1`, 1),
		strings.Replace(body, `"tasks":[`, `"extra":true,"tasks":[`, 1),
		strings.Replace(body, `"tasks":[`, `"tasks":null,"tasks":[`, 1),
		fmt.Sprintf(`{"project_id":%q,"owner_id":%q,"expected_owner_version":1,"tasks":null}`, f.project, h.resource.ID),
		fmt.Sprintf(`{"project_id":%q,"owner_id":%q,"expected_owner_version":1}`, f.project, h.resource.ID),
	} {
		ownerChangeRaw(t, f, "/task-owner-changes/preview", raw, 400)
	}
	for _, tasks := range [][]TaskOwnerSelection{{}, {valid.Tasks[0], valid.Tasks[0]}, make([]TaskOwnerSelection, 17)} {
		bad := valid
		bad.Tasks = tasks
		f.call("POST", "/task-owner-changes/preview", bad, 400)
	}
	req := ownerChangeRequest(t, h, h.resource, h.task)
	req.AcknowledgeInvalidation = false
	f.call("POST", "/task-owner-changes", req, 400)
	req.AcknowledgeInvalidation = true
	req.PreviewSHA256 = strings.Repeat("0", 64)
	f.call("POST", "/task-owner-changes", req, 409)
}

func TestTaskOwnerChangesForeignSelectionsAndTargetAccessRevocation(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	req := ownerChangeRequest(t, h, h.resource, h.task)
	other := f.call("POST", "/projects", map[string]any{"title": "Other owner project", "idempotency_key": newID()}, 201)["id"].(string)
	foreign := f.call("POST", "/tasks", map[string]any{"project_id": other, "title": "Foreign task", "idempotency_key": newID()}, 201)["id"].(string)
	bad := req
	bad.Tasks = []TaskOwnerSelection{{TaskID: h.task.ID, ExpectedVersion: 1}, {TaskID: foreign, ExpectedVersion: 1}}
	_, err := f.s.ChangeTaskOwners(ctx, f.actor, f.project, bad)
	handoffWantStatus(t, err, 404)
	if _, err = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	before := ownerChangeSnapshot(t, f)
	_, err = f.s.ChangeTaskOwners(ctx, f.actor, f.project, req)
	handoffWantStatus(t, err, 409)
	if after := ownerChangeSnapshot(t, f); !reflect.DeepEqual(before, after) {
		t.Fatal("revoked target retained writes", before, after)
	}
}

func TestTaskOwnerChangesRejectsExhaustedDependentVersionAtomically(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	child, err := f.s.Create(context.Background(), f.actor, f.project, "tasks", f.body(map[string]any{"title": "Version exhausted accepted dependent", "owner_id": h.resource.ID, "depends_on": []any{h.task.ID}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_tasks SET status='accepted',version=2147483647,data='{"verified_run_id":"fixed-evidence"}' WHERE id=?`, child.ID); err != nil {
		t.Fatal(err)
	}
	req := ownerChangeRequest(t, h, h.resource, h.task)
	before := ownerChangeSnapshot(t, f)
	_, err = f.s.ChangeTaskOwners(context.Background(), f.actor, f.project, req)
	handoffWantStatus(t, err, 409)
	if after := ownerChangeSnapshot(t, f); !reflect.DeepEqual(before, after) {
		t.Fatal("exhausted dependent version retained changes", before, after)
	}
}

func TestTaskOwnerChangesSharedUpdateRejectsExhaustedRecordVersion(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET version=2147483647 WHERE id=?`, h.task.ID); err != nil {
		t.Fatal(err)
	}
	before := ownerChangeSnapshot(t, f)
	_, err := f.s.Update(context.Background(), f.actor, f.project, "tasks", h.task.ID, f.body(map[string]any{"expected_version": 2147483647, "owner_id": h.resource.ID}))
	handoffWantStatus(t, err, 409)
	if after := ownerChangeSnapshot(t, f); !reflect.DeepEqual(before, after) {
		t.Fatal("ordinary update exceeded version range", before, after)
	}
}

func TestTaskOwnerChangesSharedUpdateKeepsOrdinaryAcceptedNoopSemantics(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET status='accepted',data='{"verified_run_id":"fixed-evidence"}' WHERE id=?`, h.task.ID); err != nil {
		t.Fatal(err)
	}
	notifications := inboxPage(t, f, f.actor, "inbox", 20, "").Total
	after, err := f.s.Update(context.Background(), f.actor, f.project, "tasks", h.task.ID, f.body(map[string]any{"expected_version": h.task.Version, "owner_id": h.owner.ID}))
	if err != nil || after.Version != 2 || after.Status != "in_progress" || after.Data["verified_run_id"] != nil || after.OwnerID != h.owner.ID || inboxPage(t, f, f.actor, "inbox", 20, "").Total != notifications {
		t.Fatal("shared extraction changed ordinary Update semantics", after, err)
	}
}

func TestTaskOwnerChangesCurrentWriteRolePrecedesReceiptReplay(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	req := ownerChangeRequest(t, h, h.resource, h.task)
	if _, err := f.s.ChangeTaskOwners(ctx, h.receiver, f.project, req); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	_, err := f.s.ChangeTaskOwners(ctx, h.receiver, f.project, req)
	handoffWantStatus(t, err, 403)
	_, err = f.s.PreviewTaskOwnerChanges(ctx, h.receiver, f.project, TaskOwnerChangePreviewRequest{ProjectID: req.ProjectID, OwnerID: req.OwnerID, ExpectedOwnerVersion: req.ExpectedOwnerVersion, Tasks: req.Tasks})
	handoffWantStatus(t, err, 403)
}

func TestTaskOwnerChangesHTTPProjectCredentialScopeAndOriginalBrowserView(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	body := TaskOwnerChangePreviewRequest{ProjectID: f.project, OwnerID: h.resource.ID, ExpectedOwnerVersion: h.resource.Version, Tasks: []TaskOwnerSelection{{TaskID: h.task.ID, ExpectedVersion: h.task.Version}}}
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, Label: "bulk owner scope fixture", IdempotencyKey: "bulk150-scope-" + scope})
		if err != nil {
			t.Fatal(err)
		}
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			path := "/api/v1/task-owner-targets?project_id=" + f.project
			if method == http.MethodPost {
				path = "/api/v1/task-owner-changes/preview"
			}
			req := httptest.NewRequest(method, path, strings.NewReader(stringify(body)))
			req.Header.Set("Authorization", "Bearer "+issued.Token)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, req)
			want := http.StatusForbidden
			if scope == "api" {
				want = http.StatusOK
			}
			if w.Code != want {
				t.Fatalf("scope %s %s => %d want %d: %s", scope, method, w.Code, want, w.Body.String())
			}
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/task-owner-changes/preview", strings.NewReader(stringify(body)))
	req.AddCookie(f.cookie)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	if w.Code != http.StatusPreconditionRequired {
		t.Fatal("browser silently adopted a different original account view", w.Code, w.Body.String())
	}
}

func TestTaskOwnerChangesAllSixteenExplicitTasksCommitOnce(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	tasks := []Record{h.task}
	for i := 1; i < 16; i++ {
		tasks = append(tasks, inboxCreateTask(t, h, fmt.Sprintf("Explicit task %d", i), h.owner.ID, fmt.Sprintf("bulk150-sixteen-%02d", i)))
	}
	req := ownerChangeRequest(t, h, h.resource, tasks...)
	before := inboxPage(t, f, h.receiver, "inbox", 20, "").Total
	out, err := f.s.ChangeTaskOwners(context.Background(), f.actor, f.project, req)
	if err != nil || len(out.Changes) != 16 || inboxPage(t, f, h.receiver, "inbox", 20, "").Total != before+16 {
		t.Fatal("explicit boundary selection was rejected or partially committed", out, err)
	}
	for i, task := range tasks {
		if out.Changes[i].TaskID != task.ID || out.Changes[i].PreviousVersion != 1 || out.Changes[i].Version != 2 || !out.Changes[i].OwnerChanged || out.Changes[i].OwnerID != h.resource.ID {
			t.Fatal("receipt reordered or changed the explicit bounded selection", out)
		}
	}
}
