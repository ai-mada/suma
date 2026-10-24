package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
)

type recordCursor struct {
	Tenant  string `json:"t"`
	Project string `json:"p"`
	Kind    string `json:"k"`
	Updated string `json:"u"`
	ID      string `json:"i"`
}

// ListPage orders newest updates first, breaking timestamp ties by stable ID.
// The cursor is scoped to its original tenant/project/record kind.
func (s *Service) ListPage(ctx context.Context, p Principal, project, kind string, limit int, cursor string) ([]Record, int, string, error) {
	if !tables[kind] {
		return nil, 0, "", missing()
	}
	if err := s.Authorize(ctx, p, project, false, false); err != nil {
		return nil, 0, "", err
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 200 {
		return nil, 0, "", invalid("limit 应为 1–200")
	}
	var c recordCursor
	if cursor != "" {
		if len(cursor) > 4096 {
			return nil, 0, "", invalid("分页游标无效")
		}
		b, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(b, &c) != nil || c.Tenant != p.TenantID || c.Project != project || c.Kind != kind || c.ID == "" || c.Updated == "" {
			return nil, 0, "", invalid("分页游标无效或不属于当前列表")
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, "", err
	}
	defer tx.Rollback()
	var total int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_`+kind+` WHERE tenant_id=? AND project_id=?`), p.TenantID, project).Scan(&total); err != nil {
		return nil, 0, "", err
	}
	query := `SELECT ` + recordColumns + ` FROM suma_` + kind + ` WHERE tenant_id=? AND project_id=?`
	args := []any{p.TenantID, project}
	if cursor != "" {
		query += ` AND (updated_at<? OR (updated_at=? AND id>?))`
		args = append(args, c.Updated, c.Updated, c.ID)
	}
	query += ` ORDER BY updated_at DESC,id ASC LIMIT ?`
	args = append(args, limit+1)
	rows, err := tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return nil, 0, "", err
	}
	items := []Record{}
	for rows.Next() {
		r, e := scanRecord(rows)
		if e != nil {
			rows.Close()
			return nil, 0, "", e
		}
		items = append(items, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, "", err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		b, _ := json.Marshal(recordCursor{p.TenantID, project, kind, last.UpdatedAt, last.ID})
		next = base64.RawURLEncoding.EncodeToString(b)
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, "", err
	}
	return items, total, next, nil
}
