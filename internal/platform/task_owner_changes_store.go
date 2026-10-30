package platform

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"unicode/utf8"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

type taskOwnerTargetCursor struct {
	Domain  string `json:"domain"`
	Version int    `json:"version"`
	Tenant  string `json:"tenant"`
	User    string `json:"user"`
	Project string `json:"project"`
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
	Name    string `json:"name"`
	ID      string `json:"id"`
}

func scanTaskOwnerTarget(row scanner) (out TaskOwnerTarget, err error) {
	err = row.Scan(&out.ResourceID, &out.ResourceVersion, &out.UserID, &out.Name)
	if err == nil && (!handoffID(out.ResourceID, false) || !handoffID(out.UserID, false) || out.ResourceVersion < 1 || !taskWorkspaceText(out.Name)) {
		err = conflict(taskOwnerTargetUnavailable)
	}
	return out, err
}

func (s *Service) ListTaskOwnerTargets(ctx context.Context, actor Actor, project string, q TaskOwnerTargetQuery) (out TaskOwnerTargetPage, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	if !handoffID(project, false) || q.Limit < 1 || q.Limit > 100 || !taskWorkspaceText(q.Q) || utf8.RuneCountInString(q.Q) > 200 || len(q.Cursor) > 4096 {
		return out, invalid(taskOwnerTargetInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, project, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	out = TaskOwnerTargetPage{ProjectID: project, Limit: q.Limit, Items: []TaskOwnerTarget{}}
	where, args := s.canonicalHumanTargetsWhere(auth.target.tenantID, project, false)
	if q.Q != "" {
		literal := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(q.Q) + "%"
		where += ` AND (LOWER(m.title) LIKE LOWER(?) ESCAPE '!' OR LOWER(m.id) LIKE LOWER(?) ESCAPE '!')`
		args = append(args, literal, literal)
	}
	scope := taskOwnerTargetCursor{Domain: "task_owner_targets", Version: 1, Tenant: auth.target.tenantID, User: auth.currentPrincipal.ID, Project: project, Query: digest(q.Q), Limit: q.Limit}
	var position taskOwnerTargetCursor
	if q.Cursor != "" {
		raw, e := base64.RawURLEncoding.Strict().DecodeString(q.Cursor)
		if e != nil || woc.Decode(raw, &position) != nil || base64.RawURLEncoding.EncodeToString([]byte(stringify(position))) != q.Cursor {
			return out, invalid(taskOwnerTargetInvalid)
		}
		identity := position
		identity.Name, identity.ID = "", ""
		if identity != scope || !handoffID(position.ID, false) || !taskWorkspaceText(position.Name) {
			return out, invalid(taskOwnerTargetInvalid)
		}
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+where), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	if q.Cursor != "" {
		where += ` AND (m.title>? OR (m.title=? AND m.id>?))`
		args = append(args, position.Name, position.Name, position.ID)
	}
	args = append(args, q.Limit+1)
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT m.id,m.version,h.user_id,m.title`+where+` ORDER BY m.title ASC,m.id ASC LIMIT ?`), args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		item, e := scanTaskOwnerTarget(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > q.Limit {
		out.Items = out.Items[:q.Limit]
		last := out.Items[q.Limit-1]
		scope.Name, scope.ID = last.Name, last.ResourceID
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(stringify(scope)))
	}
	return out, auth.assertTx(auth.tx, auth.target, purposeProjectWrite)
}

// Read every selected task before any write, including owners that will remain
// unchanged. A caller cannot smuggle a foreign or stale task through a skip.
func (s *Service) taskOwnerChangePreviewTx(ctx context.Context, auth *authorizedCommand, project string, req TaskOwnerChangePreviewRequest) (out TaskOwnerChangePreview, selected []Record, err error) {
	if err = assertProjectCommand(auth.tx, auth, project); err != nil {
		return out, nil, err
	}
	selected = make([]Record, 0, len(req.Tasks))
	conflicts := []map[string]any{}
	for _, selection := range req.Tasks {
		task, e := scanRecord(auth.tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, selection.TaskID))
		if errors.Is(e, sql.ErrNoRows) {
			return out, nil, missing()
		}
		if e != nil {
			return out, nil, e
		}
		if task.Version != selection.ExpectedVersion {
			conflicts = append(conflicts, map[string]any{"task_id": task.ID, "expected_version": selection.ExpectedVersion, "current_version": task.Version, "owner_id": task.OwnerID})
		}
		selected = append(selected, task)
	}
	if len(conflicts) != 0 {
		return out, nil, &APIError{409, "version_conflict", taskOwnerChangeChanged, map[string]any{"project_id": project, "conflicts": conflicts}}
	}
	where, args := s.canonicalHumanTargetsWhere(auth.target.tenantID, project, false)
	args = append(args, req.OwnerID)
	owner, err := scanTaskOwnerTarget(auth.tx.QueryRowContext(ctx, s.q(`SELECT m.id,m.version,h.user_id,m.title`+where+` AND m.id=?`), args...))
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil, conflict(taskOwnerTargetUnavailable)
	}
	if err != nil {
		return out, nil, err
	}
	if owner.ResourceVersion != req.ExpectedOwnerVersion {
		return out, nil, conflict(taskOwnerTargetChanged)
	}
	out = TaskOwnerChangePreview{ProjectID: project, Owner: owner, Tasks: []TaskOwnerChangePreviewItem{}}
	for _, task := range selected {
		item := TaskOwnerChangePreviewItem{TaskID: task.ID, Version: task.Version, Title: task.Title, Status: task.Status, OwnerID: task.OwnerID, RunID: task.RunID, WillChange: task.OwnerID != owner.ResourceID}
		if task.OwnerID != "" {
			e := auth.tx.QueryRowContext(ctx, s.q(`SELECT title FROM suma_members WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, task.OwnerID).Scan(&item.OwnerName)
			if e != nil && !errors.Is(e, sql.ErrNoRows) {
				return out, nil, e
			}
		}
		if item.WillChange {
			if task.Version == 2147483647 {
				return out, nil, conflict(taskOwnerVersionExhausted)
			}
			out.ChangedCount++
		} else {
			out.UnchangedCount++
		}
		out.Tasks = append(out.Tasks, item)
	}
	out.CanApply = out.ChangedCount > 0
	// Full selected snapshots bind protected basis/data and immutable request
	// order. Downstream graph state is deliberately not presented as frozen.
	out.PreviewSHA256 = digest(stringify(struct {
		Domain  string
		Tenant  string
		Preview TaskOwnerChangePreview
		Tasks   []Record
	}{"task_owner_changes", auth.target.tenantID, out, selected}))
	return out, selected, nil
}
