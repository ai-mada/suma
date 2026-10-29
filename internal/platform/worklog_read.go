package platform

import (
	"context"
	"encoding/base64"
	"time"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

type worklogCursor struct {
	Tenant     string `json:"tenant"`
	Project    string `json:"project"`
	User       string `json:"user"`
	Collection string `json:"collection"`
	Filter     string `json:"filter"`
	Scope      int    `json:"scope"`
	Date       string `json:"date"`
	ID         string `json:"id"`
	Revision   int    `json:"revision"`
}

func worklogCursorEncode(cursor worklogCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(stringify(cursor)))
}

func worklogCursorDecode(encoded string, expected worklogCursor) (cursor worklogCursor, err error) {
	if encoded == "" {
		return cursor, nil
	}
	bad := func() (worklogCursor, error) { return cursor, invalid(worklogInvalid) }
	if len(encoded) > 4096 {
		return bad()
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || woc.Decode(raw, &cursor) != nil || worklogCursorEncode(cursor) != encoded || cursor.Tenant != expected.Tenant || cursor.Project != expected.Project || cursor.User != expected.User || cursor.Collection != expected.Collection || cursor.Filter != expected.Filter || !handoffID(cursor.ID, false) {
		return bad()
	}
	if cursor.Scope != expected.Scope {
		return cursor, conflict("Worklog scope changed; reload the first page")
	}
	return cursor, nil
}

func normalizeWorklogList(req *WorklogListRequest) bool {
	if req.Limit == 0 {
		req.Limit = 20
	}
	if req.Status == "" {
		req.Status = "all"
	}
	if req.Limit < 1 || req.Limit > 50 || len(req.Cursor) > 4096 || !handoffID(req.TaskID, true) || !handoffID(req.MemberID, true) || !worklogDate(req.FromDate) || !worklogDate(req.ToDate) || req.Status != "all" && req.Status != "active" && req.Status != "voided" {
		return false
	}
	from, _ := time.Parse("2006-01-02", req.FromDate)
	to, _ := time.Parse("2006-01-02", req.ToDate)
	return !to.Before(from) && !to.After(from.AddDate(0, 0, 30))
}

func (s *Service) GetWorklog(ctx context.Context, actor Actor, project, id string) (out Worklog, err error) {
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(id, false) {
		return out, invalid(worklogInvalid)
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeProjectRead, func(auth *authorizedCommand) error {
		out, err = s.worklogHeadTx(auth.ctx, auth, project, id)
		if err != nil {
			return err
		}
		return s.worklogPermissionsTx(auth.ctx, auth, &out)
	})
	return
}

func (s *Service) ListWorklogs(ctx context.Context, actor Actor, project string, req WorklogListRequest) (out WorklogPage, err error) {
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !normalizeWorklogList(&req) {
		return out, invalid(worklogInvalid)
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		out = WorklogPage{ProjectID: project, Items: []Worklog{}, Limit: req.Limit}
		out.ScopeVersion, err = s.worklogScopeTx(auth.ctx, auth)
		if err != nil {
			return err
		}
		filter := req
		filter.Cursor = ""
		expected := worklogCursor{Tenant: auth.target.tenantID, Project: project, User: auth.currentPrincipal.ID, Collection: "worklogs", Filter: digest(stringify(filter)), Scope: out.ScopeVersion}
		after, e := worklogCursorDecode(req.Cursor, expected)
		if e != nil {
			return e
		}
		if req.Cursor != "" && (!worklogDate(after.Date) || after.Date < req.FromDate || after.Date > req.ToDate || after.Revision != 0) {
			return invalid(worklogInvalid)
		}
		write, _, manage, e := s.repositoryCapabilitiesTx(auth.ctx, auth)
		if e != nil {
			return e
		}
		out.SelfMemberID, e = s.worklogSelfMemberTx(auth.ctx, auth)
		if e != nil {
			return e
		}
		out.CanCreate, out.CanManageOthers = write && (out.SelfMemberID != "" || manage), manage
		where := ` WHERE w.tenant_id=? AND w.project_id=? AND w.work_date>=? AND w.work_date<=?`
		args := []any{auth.target.tenantID, project, req.FromDate, req.ToDate}
		if req.TaskID != "" {
			where += ` AND w.task_id=?`
			args = append(args, req.TaskID)
		}
		if req.MemberID != "" {
			where += ` AND w.member_id=?`
			args = append(args, req.MemberID)
		}
		if req.Status != "all" {
			where += ` AND w.status=?`
			args = append(args, req.Status)
		}
		if e = auth.tx.QueryRowContext(auth.ctx, s.q(`SELECT COUNT(*),COALESCE(SUM(CASE WHEN w.status='active' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN w.status='voided' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN w.status='active' THEN w.minutes ELSE 0 END),0) FROM suma_worklogs w`+where), args...).Scan(&out.Total, &out.Summary.ActiveCount, &out.Summary.VoidedCount, &out.Summary.TotalMinutes); e != nil {
			return e
		}
		if req.Cursor != "" {
			where += ` AND (w.work_date<? OR (w.work_date=? AND w.id>?))`
			args = append(args, after.Date, after.Date, after.ID)
		}
		args = append(args, req.Limit+1)
		rows, e := auth.tx.QueryContext(auth.ctx, s.q(worklogSelect+where+` ORDER BY w.work_date DESC,w.id ASC LIMIT ?`), args...)
		if e != nil {
			return e
		}
		for rows.Next() {
			entry, scanErr := scanWorklog(rows)
			if scanErr != nil {
				rows.Close()
				return scanErr
			}
			out.Items = append(out.Items, entry)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(out.Items) > req.Limit {
			out.Items = out.Items[:req.Limit]
			last := out.Items[len(out.Items)-1]
			expected.Date, expected.ID = last.WorkDate, last.ID
			out.NextCursor = worklogCursorEncode(expected)
		}
		for i := range out.Items {
			if e = s.worklogPermissionsTx(auth.ctx, auth, &out.Items[i]); e != nil {
				return e
			}
		}
		return nil
	})
	return
}

func (s *Service) WorklogRevisions(ctx context.Context, actor Actor, project, id string, limit int, cursor string) (out WorklogRevisionPage, err error) {
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || !handoffID(id, false) || limit < 1 || limit > 50 || len(cursor) > 4096 {
		return out, invalid(worklogInvalid)
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeProjectRead, func(auth *authorizedCommand) error {
		out = WorklogRevisionPage{ProjectID: project, WorklogID: id, Items: []Worklog{}, Limit: limit}
		head, e := s.worklogHeadTx(auth.ctx, auth, project, id)
		if e != nil {
			return e
		}
		if e = s.worklogPermissionsTx(auth.ctx, auth, &head); e != nil {
			return e
		}
		out.Total = head.Version
		out.ScopeVersion, e = s.worklogScopeTx(auth.ctx, auth)
		if e != nil {
			return e
		}
		expected := worklogCursor{Tenant: auth.target.tenantID, Project: project, User: auth.currentPrincipal.ID, Collection: "worklog_revisions", Filter: digest(id + ":" + stringify(limit)), Scope: out.ScopeVersion}
		after, e := worklogCursorDecode(cursor, expected)
		if e != nil {
			return e
		}
		query := `SELECT snapshot FROM suma_worklog_revisions WHERE tenant_id=? AND project_id=? AND worklog_id=?`
		args := []any{auth.target.tenantID, project, id}
		if cursor != "" {
			if after.ID != id || after.Date != "" || after.Revision < 1 || after.Revision > head.Version {
				return invalid(worklogInvalid)
			}
			query += ` AND revision<?`
			args = append(args, after.Revision)
		}
		args = append(args, limit+1)
		rows, e := auth.tx.QueryContext(auth.ctx, s.q(query+` ORDER BY revision DESC LIMIT ?`), args...)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			entry, scanErr := scanWorklog(rows)
			if scanErr != nil {
				return scanErr
			}
			entry.CanCorrect, entry.CanVoid = false, false
			out.Items = append(out.Items, entry)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > limit {
			out.Items = out.Items[:limit]
			expected.ID, expected.Revision = id, out.Items[len(out.Items)-1].Version
			out.NextCursor = worklogCursorEncode(expected)
		}
		return nil
	})
	return
}
