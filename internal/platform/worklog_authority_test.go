package platform

import (
	"context"
	"testing"
)

func worklogHumanRequest(project string, member Record, task Record, minutes int) WorklogCreateRequest {
	return WorklogCreateRequest{ProjectID: project, MemberID: member.ID, TaskID: task.ID, ExpectedTaskVersion: task.Version, WorkDate: "2026-10-29", Minutes: minutes, Note: "Actual human activity", IdempotencyKey: newID()}
}

func TestWorklogDeveloperSelfOtherAcknowledgementAndHistoricalRepair(t *testing.T) {
	h := newHandoffFixture(t)
	f, ctx := h.f, context.Background()
	self := worklogHumanRequest(f.project, h.resource, h.task, 30)
	entry, err := f.s.CreateWorklog(ctx, h.receiver, f.project, self)
	if err != nil || entry.UserID != h.receiver.Projection().ID || entry.ActorID != entry.UserID || !entry.CanCorrect {
		t.Fatal(entry, err)
	}
	other := worklogHumanRequest(f.project, h.owner, h.task, 20)
	other.OnBehalfAck = true
	_, err = f.s.CreateWorklog(ctx, h.receiver, f.project, other)
	requireAuthorityStatus(t, err, 403)
	admin := worklogHumanRequest(f.project, h.resource, h.task, 40)
	_, err = f.s.CreateWorklog(ctx, f.actor, f.project, admin)
	requireAuthorityStatus(t, err, 403)
	admin.OnBehalfAck = true
	managed, err := f.s.CreateWorklog(ctx, f.actor, f.project, admin)
	if err != nil || managed.ActorID != f.user.ID || managed.UserID != h.receiver.Projection().ID {
		t.Fatal(managed, err)
	}
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": h.receiver.Projection().ID, "action": "remove", "expected_version": 1, "idempotency_key": newID()}, 200)
	_, err = f.s.CreateWorklog(ctx, h.receiver, f.project, self)
	requireAuthorityStatus(t, err, 403)
	if _, err = f.s.GetWorklog(ctx, f.actor, f.project, entry.ID); err != nil {
		t.Fatal("current authorized reader lost historical data", err)
	}
	correction := WorklogCorrectionRequest{ProjectID: f.project, ExpectedVersion: managed.Version, TaskID: h.task.ID, ExpectedTaskVersion: h.task.Version, WorkDate: managed.WorkDate, Minutes: 35, Note: "Historical repair", Reason: "Correct retained evidence", IdempotencyKey: newID()}
	_, err = f.s.CorrectWorklog(ctx, f.actor, f.project, managed.ID, correction)
	requireAuthorityStatus(t, err, 403)
	correction.OnBehalfAck = true
	corrected, err := f.s.CorrectWorklog(ctx, f.actor, f.project, managed.ID, correction)
	if err != nil || corrected.MemberID != managed.MemberID || corrected.UserID != managed.UserID || corrected.MemberVersion != managed.MemberVersion {
		t.Fatal("historical identity changed", corrected, err)
	}
	if _, err = f.s.WorklogRevisions(ctx, f.actor, f.project, managed.ID, 20, ""); err != nil {
		t.Fatal(err)
	}
	admin.IdempotencyKey = newID()
	_, err = f.s.CreateWorklog(ctx, f.actor, f.project, admin)
	requireAuthorityStatus(t, err, 403)
}

func TestWorklogCurrentWriteRoleBeforeOriginalReceipt(t *testing.T) {
	h := newHandoffFixture(t)
	f, ctx := h.f, context.Background()
	req := worklogHumanRequest(f.project, h.resource, h.task, 30)
	entry, err := f.s.CreateWorklog(ctx, h.receiver, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, entry.UserID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CreateWorklog(ctx, h.receiver, f.project, req)
	requireAuthorityStatus(t, err, 403)
	read, err := f.s.GetWorklog(ctx, h.receiver, f.project, entry.ID)
	if err != nil || read.CanCorrect || read.CanVoid {
		t.Fatal(read, err)
	}
	page, err := f.s.ListWorklogs(ctx, h.receiver, f.project, WorklogListRequest{FromDate: "2026-10-29", ToDate: "2026-10-29"})
	if err != nil || page.CanCreate || page.CanManageOthers || page.Total != 1 {
		t.Fatal(page, err)
	}
}

func TestWorklogProjectAPIIsSelfOnlyAndMachinesCannotImpersonateHuman(t *testing.T) {
	h := newHandoffFixture(t)
	f, ctx := h.f, context.Background()
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, Label: "human", IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		actor := domainAActor(t, f.s, issued.Token)
		req := worklogHumanRequest(f.project, h.owner, h.task, 10)
		entry, err := f.s.CreateWorklog(ctx, actor, f.project, req)
		if scope != "api" {
			requireAuthorityStatus(t, err, 403)
			continue
		}
		if err != nil || entry.UserID != f.user.ID || entry.ActorID != f.user.ID {
			t.Fatal(entry, err)
		}
		req = worklogHumanRequest(f.project, h.resource, h.task, 10)
		req.OnBehalfAck = true
		_, err = f.s.CreateWorklog(ctx, actor, f.project, req)
		requireAuthorityStatus(t, err, 403)
		other, err := f.s.CreateProject(ctx, f.actor, map[string]any{"title": "Private other project", "idempotency_key": newID()})
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.s.ListWorklogs(ctx, actor, other.ID, WorklogListRequest{FromDate: req.WorkDate, ToDate: req.WorkDate})
		requireAuthorityStatus(t, err, 403)
	}
	tool := f.call("POST", "/members", f.body(map[string]any{"title": "Not a human", "kind": "agent"}), 201)
	body := f.body(map[string]any{"task_id": h.task.ID, "expected_task_version": h.task.Version, "member_id": tool["id"], "work_date": "2026-10-29", "minutes": 10, "note": "", "on_behalf_ack": true})
	f.call("POST", "/worklogs", body, 403)
}

func TestWorklogCanonicalResourceBindingRequired(t *testing.T) {
	h := newHandoffFixture(t)
	f, ctx := h.f, context.Background()
	if _, err := f.s.DB.Exec(`DELETE FROM suma_human_resources WHERE tenant_id=? AND project_id=? AND resource_id=?`, f.user.TenantID, f.project, h.resource.ID); err != nil {
		t.Fatal(err)
	}
	_, err := f.s.CreateWorklog(ctx, h.receiver, f.project, worklogHumanRequest(f.project, h.resource, h.task, 10))
	requireAuthorityStatus(t, err, 403)
	page, err := f.s.ListWorklogs(ctx, h.receiver, f.project, WorklogListRequest{FromDate: "2026-10-29", ToDate: "2026-10-29"})
	if err != nil || page.CanCreate || page.SelfMemberID != "" {
		t.Fatal(page, err)
	}
}
