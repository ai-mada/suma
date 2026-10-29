package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

const worklogInvalid = "Invalid manual worklog request"

func worklogText(text string, required bool) bool {
	return utf8.ValidString(text) && utf8.RuneCountInString(text) <= 200 && !strings.ContainsRune(text, 0) && (!required || strings.TrimSpace(text) != "")
}

func worklogDate(date string) bool {
	parsed, err := time.Parse("2006-01-02", date)
	return err == nil && parsed.Year() >= 1 && parsed.Format("2006-01-02") == date
}

func validWorklogCreate(req WorklogCreateRequest) bool {
	return handoffID(req.ProjectID, false) && handoffID(req.TaskID, false) && handoffID(req.MemberID, false) && req.ExpectedTaskVersion >= 1 && req.ExpectedTaskVersion < math.MaxInt32 && worklogDate(req.WorkDate) && req.Minutes >= 1 && req.Minutes <= 1440 && worklogText(req.Note, false) && planningKey(req.IdempotencyKey)
}

func validWorklogCorrection(req WorklogCorrectionRequest) bool {
	return handoffID(req.ProjectID, false) && handoffID(req.TaskID, false) && req.ExpectedVersion >= 1 && req.ExpectedVersion < math.MaxInt32 && req.ExpectedTaskVersion >= 1 && req.ExpectedTaskVersion < math.MaxInt32 && worklogDate(req.WorkDate) && req.Minutes >= 1 && req.Minutes <= 1440 && worklogText(req.Note, false) && worklogText(req.Reason, true) && planningKey(req.IdempotencyKey)
}

func validWorklogVoid(req WorklogVoidRequest) bool {
	return handoffID(req.ProjectID, false) && req.ExpectedVersion >= 1 && req.ExpectedVersion < math.MaxInt32 && worklogText(req.Reason, true) && planningKey(req.IdempotencyKey)
}

// Current human resources bind actual organization accounts. Display names,
// task assignees, agent owners and Runner subjects do not confer this identity.
func (s *Service) worklogHumanTx(ctx context.Context, auth *authorizedCommand, member string) (resource Record, person Principal, err error) {
	resource, err = s.getRecordTx(ctx, auth, auth.target.projectID, "members", member)
	if err != nil {
		return
	}
	if resource.Status != "active" || resource.Kind != "human" {
		return resource, person, forbidden()
	}
	user, ok := resource.Data["user_id"].(string)
	if !ok || !handoffID(user, false) {
		return resource, person, forbidden()
	}
	var canonical string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT resource_id FROM suma_human_resources WHERE tenant_id=? AND project_id=? AND user_id=?`), auth.target.tenantID, auth.target.projectID, user).Scan(&canonical)
	if errors.Is(err, sql.ErrNoRows) || err == nil && canonical != member {
		return resource, person, forbidden()
	}
	if err != nil {
		return
	}
	person, err = s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, user, auth.target.projectID, false, false)
	return
}

func (s *Service) worklogSelfMemberTx(ctx context.Context, auth *authorizedCommand) (string, error) {
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT m.id,m.data FROM suma_members m JOIN suma_human_resources h ON h.tenant_id=m.tenant_id AND h.project_id=m.project_id AND h.resource_id=m.id WHERE m.tenant_id=? AND m.project_id=? AND m.status='active' AND m.kind='human' AND h.user_id=? ORDER BY m.id`), auth.target.tenantID, auth.target.projectID, auth.currentPrincipal.ID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var id, raw string
		if err = rows.Scan(&id, &raw); err != nil {
			return "", err
		}
		var metadata map[string]any
		if err = json.Unmarshal([]byte(raw), &metadata); err != nil {
			return "", err
		}
		if metadata["user_id"] == auth.currentPrincipal.ID {
			return id, nil
		}
	}
	return "", rows.Err()
}

// A current manager may repair historical entries even after their original
// account/resource leaves the project. The frozen account and resource cannot
// be changed, and explicit on-behalf acknowledgement is checked before replay.
func (s *Service) worklogMayWriteTx(ctx context.Context, auth *authorizedCommand, entry Worklog, acknowledge bool) error {
	_, _, manage, err := s.repositoryCapabilitiesTx(ctx, auth)
	if err != nil {
		return err
	}
	if entry.UserID == auth.currentPrincipal.ID {
		_, person, e := s.worklogHumanTx(ctx, auth, entry.MemberID)
		if e == nil && person.ID == entry.UserID {
			return nil
		}
		if e != nil && !repositoryKnownError(e) {
			return e
		}
	}
	if !manage || !acknowledge {
		return forbidden()
	}
	return nil
}

func (s *Service) worklogPermissionsTx(ctx context.Context, auth *authorizedCommand, entry *Worklog) error {
	entry.CanCorrect, entry.CanVoid = false, false
	if entry.Status != "active" {
		return nil
	}
	write, _, manage, err := s.repositoryCapabilitiesTx(ctx, auth)
	if err != nil || !write {
		return err
	}
	if manage {
		entry.CanCorrect, entry.CanVoid = true, true
		return nil
	}
	if entry.UserID != auth.currentPrincipal.ID {
		return nil
	}
	_, person, e := s.worklogHumanTx(ctx, auth, entry.MemberID)
	if e != nil {
		if repositoryKnownError(e) {
			return nil
		}
		return e
	}
	entry.CanCorrect, entry.CanVoid = person.ID == entry.UserID, person.ID == entry.UserID
	return nil
}

func (s *Service) worklogTaskTx(ctx context.Context, auth *authorizedCommand, id string, expected int) (Record, error) {
	task, err := s.getRecordTx(ctx, auth, auth.target.projectID, "tasks", id)
	if err != nil {
		return task, err
	}
	if task.Status == "archived" || task.Version != expected {
		return task, conflict("Worklog task version changed")
	}
	return task, nil
}

func (s *Service) CreateWorklog(ctx context.Context, actor Actor, project string, req WorklogCreateRequest) (out Worklog, err error) {
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if project != req.ProjectID || !validWorklogCreate(req) {
		return out, invalid(worklogInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, req.MemberID, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	resource, person, err := s.worklogHumanTx(ctx, auth, req.MemberID)
	if err != nil {
		return out, err
	}
	out = Worklog{MemberID: resource.ID, MemberVersion: resource.Version, MemberName: resource.Title, UserID: person.ID, UserName: person.Name}
	if err = s.worklogMayWriteTx(ctx, auth, out, req.OnBehalfAck); err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"worklog_create": req})
	if err != nil {
		return out, err
	}
	if replay {
		return s.worklogReplayTx(ctx, auth, hash)
	}
	task, err := s.worklogTaskTx(ctx, auth, req.TaskID, req.ExpectedTaskVersion)
	if err != nil {
		return out, err
	}
	at := now()
	out.ID, out.ProjectID, out.Version, out.Status, out.Source = newID(), project, 1, "active", "human_manual"
	out.TaskID, out.TaskVersion, out.TaskTitle = task.ID, task.Version, task.Title
	out.WorkDate, out.Minutes, out.Note = req.WorkDate, req.Minutes, req.Note
	out.ActorID, out.ActorName, out.OnBehalfAck = auth.currentPrincipal.ID, auth.currentPrincipal.Name, req.OnBehalfAck
	out.CreatedAt, out.UpdatedAt = at, at
	if err = s.worklogSaveTx(ctx, auth, nil, out); err != nil {
		return out, err
	}
	return s.worklogCommitTx(ctx, auth, req.IdempotencyKey, hash, out)
}

func (s *Service) worklogReplayTx(ctx context.Context, auth *authorizedCommand, snapshot string) (out Worklog, err error) {
	if err = json.Unmarshal([]byte(snapshot), &out); err != nil {
		return out, err
	}
	head, err := s.worklogHeadTx(ctx, auth, out.ProjectID, out.ID)
	if err != nil {
		return out, err
	}
	if err = s.worklogPermissionsTx(ctx, auth, &head); err != nil {
		return out, err
	}
	out.CanCorrect, out.CanVoid = head.Version == out.Version && head.CanCorrect, head.Version == out.Version && head.CanVoid
	return out, auth.assertTx(auth.tx, auth.target, purposeProjectWrite)
}

func (s *Service) worklogCommitTx(ctx context.Context, auth *authorizedCommand, key, hash string, out Worklog) (Worklog, error) {
	if err := s.commandActivity(ctx, auth, "worklog."+out.Status, out.ID, out.TaskTitle); err != nil {
		return out, err
	}
	if err := s.saveCommandReceipt(ctx, auth, key, hash, out); err != nil {
		return out, err
	}
	if err := s.worklogPermissionsTx(ctx, auth, &out); err != nil {
		return out, err
	}
	if err := auth.assertTx(auth.tx, auth.target, purposeProjectWrite); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}

func (s *Service) CorrectWorklog(ctx context.Context, actor Actor, project, id string, req WorklogCorrectionRequest) (out Worklog, err error) {
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if project != req.ProjectID || !handoffID(id, false) || !validWorklogCorrection(req) {
		return out, invalid(worklogInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	old, err := s.worklogHeadTx(ctx, auth, project, id)
	if err != nil {
		return out, err
	}
	if err = s.worklogMayWriteTx(ctx, auth, old, req.OnBehalfAck); err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"worklog_correct": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		return s.worklogReplayTx(ctx, auth, hash)
	}
	if old.Status != "active" || old.Version != req.ExpectedVersion {
		return out, conflict("Worklog version or status changed")
	}
	task, err := s.worklogTaskTx(ctx, auth, req.TaskID, req.ExpectedTaskVersion)
	if err != nil {
		return out, err
	}
	out = old
	out.Version++
	out.TaskID, out.TaskVersion, out.TaskTitle = task.ID, task.Version, task.Title
	out.WorkDate, out.Minutes, out.Note, out.Reason = req.WorkDate, req.Minutes, req.Note, req.Reason
	out.ActorID, out.ActorName, out.OnBehalfAck, out.UpdatedAt = auth.currentPrincipal.ID, auth.currentPrincipal.Name, req.OnBehalfAck, now()
	if err = s.worklogSaveTx(ctx, auth, &old, out); err != nil {
		return out, err
	}
	return s.worklogCommitTx(ctx, auth, req.IdempotencyKey, hash, out)
}

func (s *Service) VoidWorklog(ctx context.Context, actor Actor, project, id string, req WorklogVoidRequest) (out Worklog, err error) {
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if project != req.ProjectID || !handoffID(id, false) || !validWorklogVoid(req) {
		return out, invalid(worklogInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	old, err := s.worklogHeadTx(ctx, auth, project, id)
	if err != nil {
		return out, err
	}
	if err = s.worklogMayWriteTx(ctx, auth, old, req.OnBehalfAck); err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"worklog_void": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		return s.worklogReplayTx(ctx, auth, hash)
	}
	if old.Status != "active" || old.Version != req.ExpectedVersion {
		return out, conflict("Worklog version or status changed")
	}
	out = old
	out.Version++
	out.Status, out.Reason, out.UpdatedAt = "voided", req.Reason, now()
	out.ActorID, out.ActorName, out.OnBehalfAck = auth.currentPrincipal.ID, auth.currentPrincipal.Name, req.OnBehalfAck
	if err = s.worklogSaveTx(ctx, auth, &old, out); err != nil {
		return out, err
	}
	return s.worklogCommitTx(ctx, auth, req.IdempotencyKey, hash, out)
}
