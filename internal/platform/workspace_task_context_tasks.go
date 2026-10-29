package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// Cursor identities include the literal search, not the first page's contents.
// Canonical encoding rejects unknown fields, duplicate keys and alternate wire
// representations without adding a separate project search index or authority.
type workspaceTaskCursor struct {
	Tenant  string `json:"tenant"`
	Project string `json:"project"`
	Search  string `json:"search"`
	Updated string `json:"updated"`
	ID      string `json:"id"`
}

func (s *Service) ListWorkspaceTaskOptions(ctx context.Context, actor Actor, project, search string, limit int, cursor string) (out WorkspaceTaskPage, err error) {
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || !utf8.ValidString(search) || utf8.RuneCountInString(search) > 200 || strings.ContainsRune(search, 0) || limit < 1 || limit > 50 || len(cursor) > 4096 {
		return out, invalid(workspaceTaskContextInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	out = WorkspaceTaskPage{ProjectID: project, Search: search, Items: []WorkspaceTaskOption{}, Limit: limit}
	tenant := auth.target.tenantID
	var after workspaceTaskCursor
	if cursor != "" {
		raw, e := base64.RawURLEncoding.Strict().DecodeString(cursor)
		if e != nil || json.Unmarshal(raw, &after) != nil || after.Tenant != tenant || after.Project != project || after.Search != search || !handoffID(after.ID, false) || !validReleaseTime(after.Updated) || base64.RawURLEncoding.EncodeToString([]byte(stringify(after))) != cursor {
			return out, invalid(workspaceTaskContextInvalid)
		}
	}
	like := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(search) + "%"
	where := ` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND title LIKE ? ESCAPE '!'`
	args := []any{tenant, project, like}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+where), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	query := `SELECT id,version,title,status,priority,updated_at` + where
	if cursor != "" {
		query += ` AND (updated_at<? OR (updated_at=? AND id>?))`
		args = append(args, after.Updated, after.Updated, after.ID)
	}
	query += ` ORDER BY updated_at DESC,id ASC LIMIT ?`
	args = append(args, limit+1)
	rows, err := auth.tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	updated := []string{}
	for rows.Next() {
		var item WorkspaceTaskOption
		var at string
		if err = rows.Scan(&item.ID, &item.Version, &item.Title, &item.Status, &item.Priority, &at); err != nil {
			return WorkspaceTaskPage{}, err
		}
		out.Items = append(out.Items, item)
		updated = append(updated, at)
	}
	if err = rows.Err(); err != nil {
		return WorkspaceTaskPage{}, err
	}
	rows.Close()
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(stringify(workspaceTaskCursor{tenant, project, search, updated[limit-1], last.ID})))
	}
	return out, auth.assertTx(auth.tx, auth.target, purposeProjectRead)
}
