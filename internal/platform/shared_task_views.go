package platform

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/ai-mada/suma/internal/store"
	"math"
	"strings"
	"unicode/utf8"
)

const sharedTaskViewInvalid = "Task view request is invalid"
const sharedTaskViewChanged = "Task view changed; reload the current view"

func (s *Service) InitializeSharedTaskViews(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, s.ddl(`CREATE TABLE IF NOT EXISTS suma_shared_task_views(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,id VARCHAR(64) NOT NULL,owner_id VARCHAR(64) NOT NULL,visibility VARCHAR(16) NOT NULL,version INTEGER NOT NULL,state VARCHAR(16) NOT NULL,snapshot TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,id))`))
	if err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_shared_task_views_page", "suma_shared_task_views", "tenant_id", "project_id", "state", "id")
}

func normalizeSharedTaskViewPreferences(p SharedTaskViewPreferences) (SharedTaskViewPreferences, error) {
	q, err := normalizeTaskWorkspaceQuery(TaskWorkspaceQuery{Q: p.Q, Status: p.Status, StatusUnset: p.StatusUnset, OwnerID: p.OwnerID, Priority: p.Priority, RequirementID: p.RequirementID, Unassigned: p.Unassigned, PriorityUnset: p.PriorityUnset, Sort: p.Sort})
	if err != nil {
		return p, err
	}
	p.Sort = q.Sort
	if p.Layout == "" {
		p.Layout = "list"
	}
	if p.GroupBy == "" {
		p.GroupBy = "status"
	}
	if p.Layout != "list" && p.Layout != "board" || p.GroupBy != "status" && p.GroupBy != "owner" && p.GroupBy != "priority" {
		return p, invalid(sharedTaskViewInvalid)
	}
	return p, nil
}
func validSharedTaskViewName(name string) bool {
	return taskWorkspaceText(name) && strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= 80
}
func validSharedTaskViewVisibility(v string) bool { return v == "private" || v == "project" }
func validSharedTaskViewVersion(v int) bool       { return v > 0 && v < math.MaxInt32 }

// Private preferences remain private even from project administrators. Project
// write authority is needed only for shared preferences, never private saves.
func (s *Service) sharedTaskViewPermissionsTx(ctx context.Context, a *authorizedCommand, v *SharedTaskView) error {
	v.CanEdit, v.CanDelete = false, false
	if v.Visibility == "private" && v.OwnerID != a.currentPrincipal.ID {
		return missing()
	}
	if v.State != "active" {
		return nil
	}
	if v.Visibility == "private" {
		v.CanEdit = true
		v.CanDelete = true
		return nil
	}
	write, _, _, err := s.repositoryCapabilitiesTx(ctx, a)
	if err != nil {
		return err
	}
	v.CanEdit = write && (v.OwnerID == a.currentPrincipal.ID || orgAdmin(a.currentPrincipal))
	v.CanDelete = v.CanEdit
	return nil
}
func (s *Service) sharedTaskViewTx(ctx context.Context, a *authorizedCommand, id string) (out SharedTaskView, err error) {
	var raw string
	err = a.tx.QueryRowContext(ctx, s.q(`SELECT snapshot FROM suma_shared_task_views WHERE tenant_id=? AND project_id=? AND id=?`), a.target.tenantID, a.target.projectID, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return
	}
	err = s.sharedTaskViewPermissionsTx(ctx, a, &out)
	return
}
func (s *Service) GetSharedTaskView(ctx context.Context, actor Actor, project, id string) (out SharedTaskView, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(id, false) {
		return out, invalid(sharedTaskViewInvalid)
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(a *authorizedCommand) error { out, err = s.sharedTaskViewTx(a.ctx, a, id); return err })
	return
}

type sharedTaskViewCursor struct {
	Subject string `json:"subject"`
	Tenant  string `json:"tenant"`
	Project string `json:"project"`
	Limit   int    `json:"limit"`
	ID      string `json:"id"`
}

func (s *Service) ListSharedTaskViews(ctx context.Context, actor Actor, project string, limit int, cursor string) (out SharedTaskViewPage, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || limit < 1 || limit > 100 || len(cursor) > 4096 {
		return out, invalid(sharedTaskViewInvalid)
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(a *authorizedCommand) error {
		c := sharedTaskViewCursor{Subject: a.currentPrincipal.ID, Tenant: a.target.tenantID, Project: project, Limit: limit}
		if cursor != "" {
			raw, e := base64.RawURLEncoding.Strict().DecodeString(cursor)
			var saved sharedTaskViewCursor
			if e != nil || json.Unmarshal(raw, &saved) != nil || base64.RawURLEncoding.EncodeToString([]byte(stringify(saved))) != cursor || saved.Subject != c.Subject || saved.Tenant != c.Tenant || saved.Project != c.Project || saved.Limit != c.Limit || !handoffID(saved.ID, false) {
				return invalid(sharedTaskViewInvalid)
			}
			c.ID = saved.ID
		}
		out = SharedTaskViewPage{ProjectID: project, Items: []SharedTaskView{}, Limit: limit, AsOf: now()}
		where := ` WHERE tenant_id=? AND project_id=? AND state='active' AND (visibility='project' OR owner_id=?)`
		args := []any{a.target.tenantID, project, a.currentPrincipal.ID}
		if e := a.tx.QueryRowContext(a.ctx, s.q(`SELECT COUNT(*) FROM suma_shared_task_views`+where), args...).Scan(&out.Total); e != nil {
			return e
		}
		if c.ID != "" {
			where += ` AND id>?`
			args = append(args, c.ID)
		}
		args = append(args, limit+1)
		rows, e := a.tx.QueryContext(a.ctx, s.q(`SELECT snapshot FROM suma_shared_task_views`+where+` ORDER BY id LIMIT ?`), args...)
		if e != nil {
			return e
		}
		for rows.Next() {
			var raw string
			var v SharedTaskView
			if e = rows.Scan(&raw); e != nil {
				rows.Close()
				return e
			}
			if e = json.Unmarshal([]byte(raw), &v); e != nil {
				rows.Close()
				return e
			}
			out.Items = append(out.Items, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(out.Items) > limit {
			out.Items = out.Items[:limit]
			c.ID = out.Items[limit-1].ID
			out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(stringify(c)))
		}
		for i := range out.Items {
			if e = s.sharedTaskViewPermissionsTx(a.ctx, a, &out.Items[i]); e != nil {
				return e
			}
		}
		return nil
	})
	return
}

func (s *Service) sharedTaskViewCommitTx(ctx context.Context, a *authorizedCommand, key, hash string, v SharedTaskView) (SharedTaskView, error) {
	// A fixed event title avoids leaking private view names or search terms into
	// the project-wide activity feed. The snapshot is only in protected receipts.
	if err := s.commandActivity(ctx, a, "task_view."+v.State, v.ID, "Task view updated"); err != nil {
		return v, err
	}
	if err := s.saveCommandReceipt(ctx, a, key, hash, v); err != nil {
		return v, err
	}
	if err := s.sharedTaskViewPermissionsTx(ctx, a, &v); err != nil {
		return v, err
	}
	if err := a.assertTx(a.tx, a.target, purposeProjectRead); err != nil {
		return v, err
	}
	return v, a.tx.Commit()
}
func (s *Service) sharedTaskViewReplayTx(ctx context.Context, a *authorizedCommand, raw string) (v SharedTaskView, err error) {
	if err = json.Unmarshal([]byte(raw), &v); err != nil {
		return
	}
	current, err := s.sharedTaskViewTx(ctx, a, v.ID)
	if err != nil {
		return v, err
	}
	// Never let an old success resurrect an archived view in a client cache.
	if current.State == "archived" && v.State != "archived" {
		return v, conflict(sharedTaskViewChanged)
	}
	if current.State == "active" && !current.CanEdit {
		return v, forbidden()
	}
	if current.State == "archived" && current.Visibility == "project" {
		write, _, _, e := s.repositoryCapabilitiesTx(ctx, a)
		if e != nil {
			return v, e
		}
		if !write || current.OwnerID != a.currentPrincipal.ID && !orgAdmin(a.currentPrincipal) {
			return v, forbidden()
		}
	}
	v.CanEdit = current.CanEdit && current.Version == v.Version
	v.CanDelete = current.CanDelete && current.Version == v.Version
	return v, a.assertTx(a.tx, a.target, purposeProjectRead)
}
func (s *Service) CreateSharedTaskView(ctx context.Context, actor Actor, project string, req SharedTaskViewCreateRequest) (out SharedTaskView, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if project != req.ProjectID || !handoffID(project, false) || !validSharedTaskViewName(req.Name) || !validSharedTaskViewVisibility(req.Visibility) || !planningKey(req.IdempotencyKey) {
		return out, invalid(sharedTaskViewInvalid)
	}
	req.Preferences, err = normalizeSharedTaskViewPreferences(req.Preferences)
	if err != nil {
		return
	}
	ctx, a, close, err := s.planningAuthority(ctx, actor, project, "", purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	out = SharedTaskView{ProjectID: project, OwnerID: a.currentPrincipal.ID, Name: req.Name, Visibility: req.Visibility, Preferences: req.Preferences, Version: 1, State: "active"}
	if err = s.sharedTaskViewPermissionsTx(ctx, a, &out); err != nil {
		return
	}
	if !out.CanEdit {
		return out, forbidden()
	}
	hash, replay, err := s.commandReceipt(ctx, a, req.IdempotencyKey, map[string]any{"task_view_create": req})
	if err != nil {
		return out, err
	}
	if replay {
		return s.sharedTaskViewReplayTx(ctx, a, hash)
	}
	out.ID = newID()
	out.CreatedAt = now()
	out.UpdatedAt = out.CreatedAt
	_, err = a.tx.ExecContext(ctx, s.q(`INSERT INTO suma_shared_task_views(tenant_id,project_id,id,owner_id,visibility,version,state,snapshot) VALUES(?,?,?,?,?,?,?,?)`), a.target.tenantID, project, out.ID, out.OwnerID, out.Visibility, out.Version, out.State, stringify(out))
	if err != nil {
		return
	}
	return s.sharedTaskViewCommitTx(ctx, a, req.IdempotencyKey, hash, out)
}
func (s *Service) UpdateSharedTaskView(ctx context.Context, actor Actor, project, id string, req SharedTaskViewUpdateRequest) (out SharedTaskView, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if project != req.ProjectID || !handoffID(project, false) || !handoffID(id, false) || !validSharedTaskViewVersion(req.ExpectedVersion) || !validSharedTaskViewName(req.Name) || !validSharedTaskViewVisibility(req.Visibility) || !planningKey(req.IdempotencyKey) {
		return out, invalid(sharedTaskViewInvalid)
	}
	req.Preferences, err = normalizeSharedTaskViewPreferences(req.Preferences)
	if err != nil {
		return
	}
	ctx, a, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	out, err = s.sharedTaskViewTx(ctx, a, id)
	if err != nil {
		return
	}
	if out.Visibility != req.Visibility {
		return out, invalid(sharedTaskViewInvalid)
	}
	hash, replay, err := s.commandReceipt(ctx, a, req.IdempotencyKey, map[string]any{"task_view_update": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		return s.sharedTaskViewReplayTx(ctx, a, hash)
	}
	if out.State != "active" || out.Version != req.ExpectedVersion {
		return out, conflict(sharedTaskViewChanged)
	}
	if !out.CanEdit {
		return out, forbidden()
	}
	out.Version++
	out.Name = req.Name
	out.Preferences = req.Preferences
	out.UpdatedAt = now()
	return s.sharedTaskViewSaveTx(ctx, a, req.IdempotencyKey, hash, req.ExpectedVersion, out)
}
func (s *Service) ArchiveSharedTaskView(ctx context.Context, actor Actor, project, id string, req SharedTaskViewArchiveRequest) (out SharedTaskView, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if project != req.ProjectID || !handoffID(project, false) || !handoffID(id, false) || !validSharedTaskViewVersion(req.ExpectedVersion) || !planningKey(req.IdempotencyKey) {
		return out, invalid(sharedTaskViewInvalid)
	}
	ctx, a, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	out, err = s.sharedTaskViewTx(ctx, a, id)
	if err != nil {
		return
	}
	hash, replay, err := s.commandReceipt(ctx, a, req.IdempotencyKey, map[string]any{"task_view_archive": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		return s.sharedTaskViewReplayTx(ctx, a, hash)
	}
	if out.State != "active" || out.Version != req.ExpectedVersion {
		return out, conflict(sharedTaskViewChanged)
	}
	if !out.CanDelete {
		return out, forbidden()
	}
	out.Version++
	out.State = "archived"
	out.UpdatedAt = now()
	out.CanEdit = false
	out.CanDelete = false
	return s.sharedTaskViewSaveTx(ctx, a, req.IdempotencyKey, hash, req.ExpectedVersion, out)
}
func (s *Service) sharedTaskViewSaveTx(ctx context.Context, a *authorizedCommand, key, hash string, expected int, v SharedTaskView) (SharedTaskView, error) {
	result, err := a.tx.ExecContext(ctx, s.q(`UPDATE suma_shared_task_views SET version=?,state=?,snapshot=? WHERE tenant_id=? AND project_id=? AND id=? AND version=? AND state='active'`), v.Version, v.State, stringify(v), a.target.tenantID, v.ProjectID, v.ID, expected)
	if err != nil {
		return v, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return v, err
	}
	if count != 1 {
		return v, conflict(sharedTaskViewChanged)
	}
	return s.sharedTaskViewCommitTx(ctx, a, key, hash, v)
}
