package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

type taskReviewTargetCursor struct {
	Domain  string `json:"domain"`
	Version int    `json:"version"`
	Tenant  string `json:"tenant"`
	User    string `json:"user"`
	Project string `json:"project"`
	Task    string `json:"task"`
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
	After   string `json:"after"`
}

func normalizeTaskReviewTargetQuery(q TaskReviewTargetQuery) (TaskReviewTargetQuery, bool) {
	if q.Limit == 0 {
		q.Limit = 20
	}
	valid := q.Limit >= 1 && q.Limit <= 50 && taskWorkspaceText(q.Q) && utf8.RuneCountInString(q.Q) <= 200 && len(q.Cursor) <= 4096 && validTaskConversationMentionIDs(q.ResourceIDs)
	return q, valid && (len(q.ResourceIDs) == 0 || q.Q == "" && q.Cursor == "")
}

func scanTaskReviewTarget(row scanner) (out TaskReviewTarget, err error) {
	err = row.Scan(&out.ResourceID, &out.UserID, &out.Name, &out.ResourceVersion)
	if err == nil && (!handoffID(out.ResourceID, false) || !handoffID(out.UserID, false) || out.ResourceVersion < 1 || !utf8.ValidString(out.Name)) {
		err = conflict(taskReviewRequestSource)
	}
	return out, err
}

func (s *Service) ListTaskReviewTargets(ctx context.Context, actor Actor, project, taskID string, q TaskReviewTargetQuery) (out TaskReviewTargetPage, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	var valid bool
	q, valid = normalizeTaskReviewTargetQuery(q)
	if !valid || !handoffID(project, false) || !handoffID(taskID, false) {
		return out, invalid(taskReviewRequestInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, taskID, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	if _, err = s.getRecordTx(ctx, auth, project, "tasks", taskID); err != nil {
		return out, err
	}
	out = TaskReviewTargetPage{ProjectID: project, TaskID: taskID, Limit: q.Limit, Items: []TaskReviewTarget{}}
	where, args := s.canonicalHumanTargetsWhere(auth.target.tenantID, project, true)
	if len(q.ResourceIDs) != 0 {
		markers := make([]string, len(q.ResourceIDs))
		for i, id := range q.ResourceIDs {
			markers[i] = "?"
			args = append(args, id)
		}
		where += ` AND m.id IN (` + strings.Join(markers, ",") + `)`
		rows, queryErr := auth.tx.QueryContext(ctx, s.q(`SELECT m.id,h.user_id,m.title,m.version`+where+` ORDER BY m.id LIMIT 20`), args...)
		if queryErr != nil {
			return out, queryErr
		}
		byID := map[string]TaskReviewTarget{}
		for rows.Next() {
			item, scanErr := scanTaskReviewTarget(rows)
			if scanErr != nil {
				rows.Close()
				return out, scanErr
			}
			byID[item.ResourceID] = item
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
		for _, id := range q.ResourceIDs {
			if item, ok := byID[id]; ok {
				out.Items = append(out.Items, item)
			}
		}
		out.Total = len(out.Items)
		return out, auth.assertTx(auth.tx, auth.target, purposeProjectRead)
	}
	if q.Q != "" {
		literal := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(q.Q) + "%"
		where += ` AND (LOWER(m.title) LIKE LOWER(?) ESCAPE '!' OR LOWER(m.id) LIKE LOWER(?) ESCAPE '!')`
		args = append(args, literal, literal)
	}
	scope := taskReviewTargetCursor{Domain: "task_review_targets", Version: 1, Tenant: auth.target.tenantID, User: auth.currentPrincipal.ID, Project: project, Task: taskID, Query: digest(q.Q), Limit: q.Limit}
	var position taskReviewTargetCursor
	if q.Cursor != "" {
		raw, decodeErr := base64.RawURLEncoding.Strict().DecodeString(q.Cursor)
		if decodeErr != nil || json.Unmarshal(raw, &position) != nil || base64.RawURLEncoding.EncodeToString([]byte(stringify(position))) != q.Cursor {
			return out, invalid(taskReviewRequestInvalid)
		}
		identity := position
		identity.After = ""
		if identity != scope || !handoffID(position.After, false) {
			return out, invalid(taskReviewRequestInvalid)
		}
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+where), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	if q.Cursor != "" {
		where += ` AND m.id>?`
		args = append(args, position.After)
	}
	args = append(args, q.Limit+1)
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT m.id,h.user_id,m.title,m.version`+where+` ORDER BY m.id ASC LIMIT ?`), args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		item, scanErr := scanTaskReviewTarget(rows)
		if scanErr != nil {
			rows.Close()
			return out, scanErr
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
		scope.After = out.Items[q.Limit-1].ResourceID
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(stringify(scope)))
	}
	return out, auth.assertTx(auth.tx, auth.target, purposeProjectRead)
}
