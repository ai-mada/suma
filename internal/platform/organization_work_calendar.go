package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/ai-mada/suma/internal/planningcalendar"
)

type OrganizationWorkCalendar struct {
	OrganizationID string                   `json:"organization_id"`
	UserID         string                   `json:"user_id"`
	Version        int                      `json:"version"`
	Schedule       ResourceCalendarSchedule `json:"schedule"`
	UpdatedAt      string                   `json:"updated_at"`
	CanEdit        bool                     `json:"can_edit"`
}

type OrganizationWorkCalendarSaveRequest struct {
	ExpectedVersion int                      `json:"expected_version"`
	IdempotencyKey  string                   `json:"idempotency_key"`
	Schedule        ResourceCalendarSchedule `json:"schedule"`
}

const organizationWorkCalendarInvalid = "Invalid organization work calendar request"

func organizationWorkCalendarTarget(actor Actor, user string) commandTarget {
	return commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope, resourceID: user, subjectID: user}
}

// Check the caller and subject under the same locks as organization membership
// changes. Stored calendars and original receipts never grant membership.
func (s *Service) organizationWorkCalendarMemberTx(ctx context.Context, auth *authorizedCommand, user string) (string, error) {
	if err := assertOrganizationRead(auth); err != nil {
		return "", err
	}
	p := auth.currentPrincipal
	if auth.target.subjectID != user || !validOrgRole(p.Role) ||
		(!orgAdmin(p) && (auth.purpose == purposeOrganizationWrite || p.ID != user)) {
		return "", forbidden()
	}
	var name, role string
	err := auth.tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT u.name,m.role FROM suma_org_members m JOIN suma_users u ON u.id=m.user_id WHERE m.tenant_id=? AND m.user_id=?`)), p.TenantID, user).Scan(&name, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", missing()
	}
	if err != nil {
		return "", err
	}
	if !validOrgRole(role) {
		return "", forbidden()
	}
	return name, nil
}

func (s *Service) organizationWorkCalendarHeadTx(ctx context.Context, auth *authorizedCommand, user string) (OrganizationWorkCalendar, error) {
	out := OrganizationWorkCalendar{
		OrganizationID: auth.currentPrincipal.TenantID,
		UserID:         user,
		Schedule:       unknownResourceCalendar("", "").Schedule,
		CanEdit:        orgAdmin(auth.currentPrincipal),
	}
	var raw string
	err := auth.tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT version,schedule,updated_at FROM suma_organization_work_calendars WHERE tenant_id=? AND user_id=?`)), out.OrganizationID, user).Scan(&out.Version, &raw, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return OrganizationWorkCalendar{}, err
	}
	if err = json.Unmarshal([]byte(raw), &out.Schedule); err != nil {
		return OrganizationWorkCalendar{}, err
	}
	if out.Version < 1 || len(out.Schedule.Weekdays) != 7 || out.Schedule.Exceptions == nil {
		return OrganizationWorkCalendar{}, errors.New("invalid stored organization work calendar")
	}
	if err = planningcalendar.Validate(out.Schedule); err != nil {
		return OrganizationWorkCalendar{}, err
	}
	return out, nil
}

func (s *Service) GetOrganizationWorkCalendar(ctx context.Context, actor Actor, user string) (out OrganizationWorkCalendar, err error) {
	if !handoffID(user, false) {
		return out, invalid(organizationWorkCalendarInvalid)
	}
	err = s.withCurrentRead(ctx, actor, organizationWorkCalendarTarget(actor, user), purposeOrganizationRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		if _, e := s.organizationWorkCalendarMemberTx(bounded, auth, user); e != nil {
			return e
		}
		out, err = s.organizationWorkCalendarHeadTx(bounded, auth, user)
		return err
	})
	if err != nil {
		return OrganizationWorkCalendar{}, err
	}
	return out, nil
}

func (s *Service) SaveOrganizationWorkCalendar(ctx context.Context, actor Actor, user string, req OrganizationWorkCalendarSaveRequest) (out OrganizationWorkCalendar, err error) {
	if !handoffID(user, false) || req.ExpectedVersion < 0 || req.ExpectedVersion >= math.MaxInt32 ||
		!planningKey(req.IdempotencyKey) || len(req.Schedule.Weekdays) != 7 || req.Schedule.Exceptions == nil {
		return out, invalid(organizationWorkCalendarInvalid)
	}
	if err = planningcalendar.Validate(req.Schedule); err != nil {
		return out, invalid("Organization work calendar schedule is invalid")
	}

	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, organizationWorkCalendarTarget(actor, user), purposeOrganizationWrite)
	if err != nil {
		return out, err
	}
	name, err := s.organizationWorkCalendarMemberTx(ctx, auth, user)
	if err != nil {
		return out, err
	}

	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"organization_work_calendar_save": user, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}

	out, err = s.organizationWorkCalendarHeadTx(ctx, auth, user)
	if err != nil {
		return OrganizationWorkCalendar{}, err
	}
	if out.Version != req.ExpectedVersion {
		return OrganizationWorkCalendar{}, conflict("Organization work calendar version changed")
	}

	out.Version++
	out.Schedule = req.Schedule
	out.UpdatedAt = auth.checkedAt.UTC().Format(time.RFC3339Nano)
	if req.ExpectedVersion == 0 {
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_organization_work_calendars(tenant_id,user_id,version,schedule,updated_at) VALUES(?,?,?,?,?)`), out.OrganizationID, user, out.Version, stringify(out.Schedule), out.UpdatedAt)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, s.q(`UPDATE suma_organization_work_calendars SET version=?,schedule=?,updated_at=? WHERE tenant_id=? AND user_id=? AND version=?`), out.Version, stringify(out.Schedule), out.UpdatedAt, out.OrganizationID, user, req.ExpectedVersion)
		if err == nil {
			var changed int64
			changed, err = result.RowsAffected()
			if err == nil && changed != 1 {
				return OrganizationWorkCalendar{}, conflict("Organization work calendar version changed")
			}
		}
	}
	if err != nil {
		return OrganizationWorkCalendar{}, err
	}
	if err = s.commandActivity(ctx, auth, "organization.work_calendar.updated", user, name); err != nil {
		return OrganizationWorkCalendar{}, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return OrganizationWorkCalendar{}, err
	}
	if err = tx.Commit(); err != nil {
		return OrganizationWorkCalendar{}, err
	}
	return out, nil
}
