package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func handoffInboxLocator(t *testing.T, item NotificationInboxItem) string {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal([]byte(stringify(item)), &fields); err != nil {
		t.Fatal(err)
	}
	id, _ := fields["handoff_id"].(string)
	return id
}

func TestTaskHandoffInboxCreationAndOriginalReceipt(t *testing.T) {
	h := newHandoffFixture(t)
	f, ctx := h.f, context.Background()
	req := h.request(t)
	before := h.current(t)
	created, err := f.s.CreateTaskHandoff(ctx, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	page := inboxPage(t, f, h.receiver, "inbox", 20, "")
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("pending handoff did not notify its actual recipient: %+v", page)
	}
	item := page.Items[0]
	if item.Kind != "task_handoff_requested" || item.TaskID != before.ID || item.SourceResourceID != h.resource.ID || item.SourceTaskVersion != before.Version || !item.SourceAvailable || item.CurrentTask == nil || item.AssignmentCurrent || item.Read || handoffInboxLocator(t, item) != created.ID {
		t.Fatalf("handoff source was inferred or changed responsibility: %+v", item)
	}
	if current := h.current(t); current.OwnerID != before.OwnerID || current.Version != before.Version || current.RunID != before.RunID || domainACount(t, f.s, "suma_reviews") != 0 {
		t.Fatal("request notification changed task responsibility or formal review")
	}
	var payload string
	if err = f.s.DB.QueryRow(`SELECT payload FROM suma_task_handoffs WHERE id=?`, created.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var private map[string]any
	if err = json.Unmarshal([]byte(payload), &private); err != nil || private["notification_id"] != item.ID {
		t.Fatalf("source omitted its private reverse notification binding: %s %v", payload, err)
	}
	if strings.Contains(stringify(created), "notification_id") {
		t.Fatal("private binding leaked into the public handoff")
	}
	// The original create receipt is restored before revalidating new target
	// eligibility or task basis, and never tries to emit another notification.
	if _, err = f.s.Update(ctx, f.actor, f.project, "tasks", before.ID, map[string]any{"expected_version": before.Version, "title": "Current title after proposal", "idempotency_key": newID()}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	replayed, err := f.s.CreateTaskHandoff(ctx, f.actor, f.project, req)
	if err != nil || stringify(replayed) != stringify(created) || inboxPage(t, f, h.receiver, "inbox", 20, "").Total != 1 {
		t.Fatalf("original create receipt was lost or duplicated after drift: %+v %v", replayed, err)
	}
	req.Note += " changed"
	_, err = f.s.CreateTaskHandoff(ctx, f.actor, f.project, req)
	handoffWantStatus(t, err, 409)
	if _, err = f.s.DB.Exec(`DELETE FROM suma_org_members WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	req.Note = created.Note
	_, err = f.s.CreateTaskHandoff(ctx, f.actor, f.project, req)
	handoffWantStatus(t, err, 403)
}

func TestTaskHandoffInboxProjectionFailureRollsBackWholeSource(t *testing.T) {
	h := newHandoffFixture(t)
	f, ctx := h.f, context.Background()
	var before, after int64
	if err := f.s.DB.QueryRow(`SELECT last_sequence FROM suma_notification_inbox_sequences WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&before); err != nil {
		t.Fatal(err)
	}
	receipts := domainACount(t, f.s, "suma_operations")
	if _, err := f.s.DB.Exec(`CREATE TRIGGER handoff_reject_projection BEFORE INSERT ON suma_notification_inbox WHEN NEW.kind='task_handoff_requested' BEGIN SELECT RAISE(ABORT,'owned handoff projection failure'); END`); err != nil {
		t.Fatal(err)
	}
	req := h.request(t)
	if _, err := f.s.CreateTaskHandoff(ctx, f.actor, f.project, req); err == nil {
		t.Fatal("handoff source committed after its notification failed")
	}
	if err := f.s.DB.QueryRow(`SELECT last_sequence FROM suma_notification_inbox_sequences WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after || domainACount(t, f.s, "suma_task_handoffs") != 0 || domainACount(t, f.s, "suma_task_handoff_pending") != 0 || domainACount(t, f.s, "suma_operations") != receipts || inboxPage(t, f, h.receiver, "inbox", 20, "").Total != 0 {
		t.Fatal("failed source left a handoff, pending slot, receipt or sequence gap")
	}
	if _, err := f.s.DB.Exec(`DROP TRIGGER handoff_reject_projection`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.CreateTaskHandoff(ctx, f.actor, f.project, req); err != nil || inboxPage(t, f, h.receiver, "inbox", 20, "").Total != 1 {
		t.Fatalf("original failed operation could not commit once: %v", err)
	}
}

func TestTaskHandoffInboxTerminalHistoryRetainsPrivateBinding(t *testing.T) {
	for _, action := range []string{"accept", "return", "withdraw"} {
		t.Run(action, func(t *testing.T) {
			h := newHandoffFixture(t)
			f, ctx := h.f, context.Background()
			proposal := h.send(t)
			actor := h.receiver
			if action == "withdraw" {
				actor = f.actor
			}
			command := handoffCommand(h, action, proposal.Version)
			decided, err := f.s.CommandTaskHandoff(ctx, actor, f.project, proposal.ID, command)
			if err != nil {
				t.Fatal(err)
			}
			page := inboxPage(t, f, h.receiver, "inbox", 20, "")
			want := 1
			if action == "accept" {
				want++
			}
			if page.Total != want || domainACount(t, f.s, "suma_reviews") != 0 {
				t.Fatalf("handoff decision changed review or request event count: %+v", page)
			}
			for _, item := range page.Items {
				if item.Kind == "task_handoff_requested" && (handoffInboxLocator(t, item) != proposal.ID || !item.SourceAvailable || item.Read || item.AssignmentCurrent) {
					t.Fatalf("terminal source lost reverse binding or auto-read: %+v", item)
				}
			}
			if replay, err := f.s.CommandTaskHandoff(ctx, actor, f.project, proposal.ID, command); err != nil || stringify(replay) != stringify(decided) || inboxPage(t, f, h.receiver, "inbox", 20, "").Total != want {
				t.Fatalf("decision original receipt changed: %+v %v", replay, err)
			}
		})
	}
}

func TestTaskHandoffInboxRejectsIncompleteReverseSources(t *testing.T) {
	for _, broken := range []string{"missing", "missing_task", "malformed", "id", "project_id", "task_id", "version", "state", "to_user_id", "to_resource_id", "basis", "notification_id", "row_task"} {
		t.Run(broken, func(t *testing.T) {
			h := newHandoffFixture(t)
			f := h.f
			proposal := h.send(t)
			page := inboxPage(t, f, h.receiver, "inbox", 20, "")
			if len(page.Items) != 1 {
				t.Fatalf("missing pending request event: %+v", page)
			}
			item := page.Items[0]
			_, err := f.s.CommandNotificationInbox(context.Background(), h.receiver, f.project, item.ID, NotificationInboxCommand{ProjectID: f.project, ExpectedVersion: item.Version, Action: "read", IdempotencyKey: newID()})
			if err != nil {
				t.Fatal(err)
			}
			var raw string
			if err = f.s.DB.QueryRow(`SELECT payload FROM suma_task_handoffs WHERE id=?`, proposal.ID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err = json.Unmarshal([]byte(raw), &fields); err != nil {
				t.Fatal(err)
			}
			switch broken {
			case "missing":
				_, err = f.s.DB.Exec(`DELETE FROM suma_task_handoffs WHERE id=?`, proposal.ID)
			case "missing_task":
				_, err = f.s.DB.Exec(`DELETE FROM suma_tasks WHERE id=?`, h.task.ID)
			case "malformed":
				_, err = f.s.DB.Exec(`UPDATE suma_task_handoffs SET payload='{' WHERE id=?`, proposal.ID)
			case "row_task":
				_, err = f.s.DB.Exec(`UPDATE suma_task_handoffs SET task_id=? WHERE id=?`, newID(), proposal.ID)
			default:
				if broken == "basis" {
					fields["basis"].(map[string]any)["task_version"] = proposal.Basis.TaskVersion + 1
				} else if broken == "version" {
					fields[broken] = proposal.Version + 1
				} else {
					fields[broken] = newID()
				}
				_, err = f.s.DB.Exec(`UPDATE suma_task_handoffs SET payload=? WHERE id=?`, stringify(fields), proposal.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			next := inboxPage(t, f, h.receiver, "inbox", 20, "")
			if next.Total != 1 || len(next.Items) != 1 || next.Items[0].SourceAvailable || next.Items[0].CurrentTask != nil || handoffInboxLocator(t, next.Items[0]) != "" || !next.Items[0].Read {
				t.Fatalf("broken reverse source disclosed a target or lost personal state: %+v", next)
			}
		})
	}
}

func TestTaskHandoffInboxSQLFailureIsNotUnavailableSource(t *testing.T) {
	h := newHandoffFixture(t)
	h.send(t)
	if _, err := h.f.s.DB.Exec(`ALTER TABLE suma_task_handoffs RENAME TO owned_unavailable_handoffs`); err != nil {
		t.Fatal(err)
	}
	if _, err := h.f.s.NotificationInbox(context.Background(), h.receiver, h.f.project, "inbox", 20, ""); err == nil {
		t.Fatal("database failure was presented as a missing handoff source")
	}
}

func TestTaskHandoffInboxExactHTTPHistoryAndClosedScope(t *testing.T) {
	h := newHandoffFixture(t)
	f, ctx := h.f, context.Background()
	old := h.send(t)
	if _, err := f.s.CommandTaskHandoff(ctx, f.actor, f.project, old.ID, handoffCommand(h, "withdraw", old.Version)); err != nil {
		t.Fatal(err)
	}
	latest := h.send(t)
	base := "/task-handoffs/" + old.ID + "?project_id=" + f.project + "&task_id=" + h.task.ID
	before := domainACount(t, f.s, "suma_operations")
	detail := f.call("GET", base, nil, 200)
	if detail["id"] != old.ID || detail["state"] != "withdrawn" || strings.Contains(stringify(detail), "notification_id") {
		t.Fatalf("exact old source was replaced by latest %s or leaked metadata: %+v", latest.ID, detail)
	}
	if domainACount(t, f.s, "suma_operations") != before || inboxPage(t, f, h.receiver, "inbox", 20, "").Counts.Unread != 2 {
		t.Fatal("opening exact history wrote a receipt or marked a notification read")
	}
	for _, suffix := range []string{"&limit=1", "&before=" + latest.ID, "&extra=x", "&task_id=" + h.task.ID, "&project_id=" + f.project} {
		f.call("GET", base+suffix, nil, 400)
	}
	f.call("GET", "/task-handoffs/"+old.ID+"?project_id="+f.project+"&task_id=", nil, 400)
	other, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Different exact task"}))
	if err != nil {
		t.Fatal(err)
	}
	f.call("GET", "/task-handoffs/"+old.ID+"?project_id="+f.project+"&task_id="+other.ID, nil, 404)
	foreign := f.call("POST", "/projects", map[string]any{"title": "Different exact project", "idempotency_key": newID()}, 201)["id"].(string)
	f.call("GET", "/task-handoffs/"+old.ID+"?project_id="+foreign+"&task_id="+h.task.ID, nil, 404)
	for _, scope := range []string{"agent", "runner"} {
		issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("GET", "/api/v1"+base, nil)
		req.Header.Set("Authorization", "Bearer "+issued.Token)
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, req)
		if response.Code != 403 {
			t.Fatalf("%s entered exact human handoff read: %d %s", scope, response.Code, response.Body.String())
		}
	}
	if _, err = f.s.DB.Exec(`DELETE FROM suma_org_members WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	f.call("GET", base, nil, 403)
}

func TestTaskHandoffInboxHTTPRejectsRoundedVersionsWithoutWrites(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	receipts := domainACount(t, f.s, "suma_operations")
	for _, version := range []string{"1.0", "1e0", "1.000000000000000000001"} {
		raw := fmt.Sprintf(`{"project_id":%q,"task_id":%q,"expected_task_version":%s,"to_resource_id":%q,"note":"No rounded version","idempotency_key":%q}`, f.project, h.task.ID, version, h.resource.ID, newID())
		req := httptest.NewRequest("POST", "/api/v1/task-handoffs", strings.NewReader(raw))
		req.AddCookie(f.cookie)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Suma-Expected-User", f.view.ID)
		req.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("raw version %s was rounded into a handoff CAS: %d %s", version, response.Code, response.Body.String())
		}
	}
	if domainACount(t, f.s, "suma_task_handoffs") != 0 || domainACount(t, f.s, "suma_task_handoff_pending") != 0 || domainACount(t, f.s, "suma_operations") != receipts {
		t.Fatal("invalid numeric boundary persisted a source or receipt")
	}
	proposal := h.send(t)
	receipts = domainACount(t, f.s, "suma_operations")
	for _, version := range []string{"1.0", "1e0", "1.000000000000000000001"} {
		raw := fmt.Sprintf(`{"project_id":%q,"expected_version":%s,"action":"withdraw","reason":"No rounded decision","acknowledge_invalidation":true,"idempotency_key":%q}`, f.project, version, newID())
		req := httptest.NewRequest("POST", "/api/v1/task-handoffs/"+proposal.ID+"/commands", strings.NewReader(raw))
		req.AddCookie(f.cookie)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Suma-Expected-User", f.view.ID)
		req.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("raw version %s was rounded into a decision CAS: %d %s", version, response.Code, response.Body.String())
		}
	}
	var version int
	var state string
	if err := f.s.DB.QueryRow(`SELECT version,state FROM suma_task_handoffs WHERE id=?`, proposal.ID).Scan(&version, &state); err != nil {
		t.Fatal(err)
	}
	if version != 1 || state != "pending" || domainACount(t, f.s, "suma_task_handoff_pending") != 1 || domainACount(t, f.s, "suma_operations") != receipts {
		t.Fatal("invalid decision numeric boundary persisted a decision or receipt")
	}
}

func TestTaskHandoffInboxUnnotifiedHistoryDoesNotBackfill(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	proposal := h.send(t)
	// Model an existing source payload without a notification reference. The
	// current initializer and later reads/decisions must not invent its history.
	if _, err := f.s.DB.Exec(`UPDATE suma_task_handoffs SET payload=? WHERE id=?`, stringify(proposal), proposal.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`DELETE FROM suma_notification_inbox WHERE kind='task_handoff_requested' AND source_event_key=?`, proposal.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeTaskHandoffs(context.Background()); err != nil {
		t.Fatal(err)
	}
	base := "/task-handoffs/" + proposal.ID + "?project_id=" + f.project + "&task_id=" + h.task.ID
	f.call("GET", base, nil, 200)
	if _, err := f.s.CommandTaskHandoff(context.Background(), f.actor, f.project, proposal.ID, handoffCommand(h, "withdraw", proposal.Version)); err != nil {
		t.Fatal(err)
	}
	if page := inboxPage(t, f, h.receiver, "inbox", 20, ""); page.Total != 0 {
		t.Fatalf("read or decision manufactured a historical notification: %+v", page)
	}
}
