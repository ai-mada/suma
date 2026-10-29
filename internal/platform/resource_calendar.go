package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"

	"github.com/ai-mada/suma/internal/planningcalendar"
)

type ResourceCalendarSchedule = planningcalendar.ResourceCalendarSchedule
type ResourceCalendarException = planningcalendar.ResourceCalendarException
type ResourceCalendarDay = planningcalendar.ResourceCalendarDay

type ResourceCalendar struct {
	ProjectID string                   `json:"project_id"`
	MemberID  string                   `json:"member_id"`
	Version   int                      `json:"version"`
	Schedule  ResourceCalendarSchedule `json:"schedule"`
	UpdatedAt string                   `json:"updated_at"`
	CanEdit   bool                     `json:"can_edit"`
}

type ResourceCalendarSaveRequest struct {
	ProjectID       string                   `json:"project_id"`
	ExpectedVersion int                      `json:"expected_version"`
	IdempotencyKey  string                   `json:"idempotency_key"`
	Schedule        ResourceCalendarSchedule `json:"schedule"`
}

func unknownResourceCalendar(project, member string) ResourceCalendar {
	return ResourceCalendar{ProjectID: project, MemberID: member, Schedule: ResourceCalendarSchedule{Weekdays: make([]*float64, 7), Exceptions: []ResourceCalendarException{}}}
}

// Resource eligibility is rechecked before reading a calendar or recovering its
// original receipt. Retained rows never restore a revoked person's access.
func (s *Service) resourceCalendarMemberTx(ctx context.Context, auth *authorizedCommand, project, member string) (string, error) {
	if err := assertProjectRead(auth, project); err != nil {
		return "", err
	}
	var name, kind, metadata string
	err := auth.tx.QueryRowContext(ctx, s.q(`SELECT title,kind,data FROM suma_members WHERE tenant_id=? AND project_id=? AND id=? AND status<>'archived'`), auth.currentPrincipal.TenantID, project, member).Scan(&name, &kind, &metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return "", missing()
	}
	if err != nil {
		return "", err
	}
	if kind != "human" && kind != "" {
		return name, nil
	}
	var data map[string]any
	if err = json.Unmarshal([]byte(metadata), &data); err != nil {
		return "", err
	}
	user, _ := data["user_id"].(string)
	var allowed int
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_org_members om JOIN suma_users u ON u.id=om.user_id JOIN suma_projects p ON p.tenant_id=om.tenant_id AND p.id=? LEFT JOIN suma_project_members pm ON pm.project_id=p.id AND pm.user_id=om.user_id WHERE om.tenant_id=? AND om.user_id=? AND (om.role IN ('owner','admin') OR pm.role IN ('admin','reviewer','developer','viewer'))`), project, auth.currentPrincipal.TenantID, user).Scan(&allowed)
	if err != nil {
		return "", err
	}
	if allowed != 1 {
		return "", forbidden()
	}
	return name, nil
}

func (s *Service) resourceCalendarHeadTx(ctx context.Context, auth *authorizedCommand, project, member string) (ResourceCalendar, error) {
	out := unknownResourceCalendar(project, member)
	if err := assertProjectRead(auth, project); err != nil {
		return out, err
	}
	var schedule string
	err := auth.tx.QueryRowContext(ctx, s.q(`SELECT version,schedule,updated_at FROM suma_resource_calendars WHERE tenant_id=? AND project_id=? AND member_id=?`), auth.currentPrincipal.TenantID, project, member).Scan(&out.Version, &schedule, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(schedule), &out.Schedule); err != nil {
		return out, err
	}
	return out, planningcalendar.Validate(out.Schedule)
}

func (s *Service) resourceCalendarCanEditTx(ctx context.Context, auth *authorizedCommand, project string) (bool, error) {
	if auth.actor.kind != actorBrowser && auth.actor.kind != actorProjectAPI {
		return false, nil
	}
	if orgAdmin(auth.currentPrincipal) {
		return true, nil
	}
	var role string
	err := auth.tx.QueryRowContext(ctx, s.q(`SELECT role FROM suma_project_members WHERE project_id=? AND user_id=?`), project, auth.currentPrincipal.ID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return role == "admin" || role == "reviewer" || role == "developer", err
}

func (s *Service) GetResourceCalendar(ctx context.Context, actor Actor, project, member string) (out ResourceCalendar, err error) {
	if !handoffID(project, false) || !handoffID(member, false) {
		return out, invalid("Invalid resource calendar request")
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: member}, purposeProjectRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		if _, e := s.resourceCalendarMemberTx(bounded, auth, project, member); e != nil {
			return e
		}
		out, err = s.resourceCalendarHeadTx(bounded, auth, project, member)
		if err != nil {
			return err
		}
		out.CanEdit, err = s.resourceCalendarCanEditTx(bounded, auth, project)
		return err
	})
	return out, err
}

func (s *Service) SaveResourceCalendar(ctx context.Context, actor Actor, project, member string, req ResourceCalendarSaveRequest) (out ResourceCalendar, err error) {
	if req.ProjectID != project || !handoffID(project, false) || !handoffID(member, false) || req.ExpectedVersion < 0 || req.ExpectedVersion >= math.MaxInt32 || !planningKey(req.IdempotencyKey) || len(req.Schedule.Weekdays) != 7 || req.Schedule.Exceptions == nil {
		return out, invalid("Invalid resource calendar request")
	}
	if err = planningcalendar.Validate(req.Schedule); err != nil {
		return out, invalid("Resource calendar schedule is invalid")
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: member}, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	name, err := s.resourceCalendarMemberTx(ctx, auth, project, member)
	if err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"resource_calendar_save": member, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		out.CanEdit = true
		return out, nil
	}
	old, err := s.resourceCalendarHeadTx(ctx, auth, project, member)
	if err != nil {
		return out, err
	}
	if old.Version != req.ExpectedVersion {
		return out, conflict("Resource calendar version changed")
	}
	out = ResourceCalendar{ProjectID: project, MemberID: member, Version: old.Version + 1, Schedule: req.Schedule, UpdatedAt: now(), CanEdit: true}
	if old.Version == 0 {
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_resource_calendars(tenant_id,project_id,member_id,version,schedule,updated_at) VALUES(?,?,?,?,?,?)`), auth.currentPrincipal.TenantID, project, member, out.Version, stringify(out.Schedule), out.UpdatedAt)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, s.q(`UPDATE suma_resource_calendars SET version=?,schedule=?,updated_at=? WHERE tenant_id=? AND project_id=? AND member_id=? AND version=?`), out.Version, stringify(out.Schedule), out.UpdatedAt, auth.currentPrincipal.TenantID, project, member, old.Version)
		if err == nil {
			var changed int64
			changed, err = result.RowsAffected()
			if err == nil && changed != 1 {
				return out, conflict("Resource calendar version changed")
			}
		}
	}
	if err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "resource_calendar.updated", member, name); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
