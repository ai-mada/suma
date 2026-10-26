package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
)

type skillCatalogCursor struct {
	Tenant, Project, Kind, Filter, Created, ID string
	Version                                    int
}

func skillCursorEncode(c skillCatalogCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(stringify(c)))
}
func skillCursorDecode(value string) (c skillCatalogCursor, err error) {
	if len(value) > 4096 {
		return c, invalid(skillInvalid)
	}
	raw, e := base64.RawURLEncoding.Strict().DecodeString(value)
	if e != nil {
		return c, invalid(skillInvalid)
	}
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || skillCursorEncode(c) != value {
		return c, invalid(skillInvalid)
	}
	return c, nil
}
func (s *Service) ListSkillCatalog(ctx context.Context, actor Actor, q SkillCatalogListQuery) (out SkillCatalogPage, err error) {
	if !handoffID(q.ProjectID, false) {
		return out, invalid(skillInvalid)
	}
	ctx, auth, close, err := s.skillCatalogAuthority(ctx, actor, q.ProjectID, "", purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	return s.listSkillCatalogTx(ctx, auth, q)
}
func (s *Service) listSkillCatalogTx(ctx context.Context, auth *authorizedCommand, q SkillCatalogListQuery) (out SkillCatalogPage, err error) {
	out.Items = []SkillCatalogSummary{}
	if q.Limit == 0 {
		q.Limit = 20
	}
	if !handoffID(q.ProjectID, false) || q.Limit < 1 || q.Limit > 100 || q.State != "" && q.State != "published" && q.State != "retired" || !skillText(q.Domain, 80, false) || !skillText(q.Scenario, 80, false) || !skillText(q.Tag, 40, false) || !skillText(q.Query, 200, false) {
		return out, invalid(skillInvalid)
	}
	out.CanManage, out.CanAdopt, err = s.skillCatalogCapabilitiesTx(ctx, auth, q.ProjectID)
	if err != nil {
		return out, err
	}
	filter := digest(stringify([]string{q.State, q.Domain, q.Scenario, q.Tag, q.Query}))
	position := skillCatalogCursor{}
	if q.Cursor != "" {
		position, err = skillCursorDecode(q.Cursor)
		if err != nil || position.Tenant != auth.target.tenantID || position.Project != q.ProjectID || position.Kind != "list" || position.Filter != filter || !handoffID(position.ID, false) {
			return out, invalid(skillInvalid)
		}
	}
	where := ` WHERE tenant_id=?`
	args := []any{auth.target.tenantID}
	if q.State != "" {
		where += ` AND state=?`
		args = append(args, q.State)
	}
	if q.Domain != "" {
		where += ` AND domain=?`
		args = append(args, q.Domain)
	}
	for _, entry := range []struct{ column, value string }{{"scenarios", q.Scenario}, {"tags", q.Tag}} {
		if entry.value != "" {
			where += ` AND ` + entry.column + ` LIKE ? ESCAPE '!'`
			args = append(args, "%"+skillCatalogLike(stringify(entry.value))+"%")
		}
	}
	if q.Query != "" {
		where += ` AND (LOWER(title) LIKE LOWER(?) ESCAPE '!' OR LOWER(summary) LIKE LOWER(?) ESCAPE '!' OR LOWER(domain) LIKE LOWER(?) ESCAPE '!' OR LOWER(scenarios) LIKE LOWER(?) ESCAPE '!' OR LOWER(tags) LIKE LOWER(?) ESCAPE '!')`
		value := "%" + skillCatalogLike(q.Query) + "%"
		args = append(args, value, value, value, value, value)
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_skill_catalog`+where), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	if q.Cursor != "" {
		where += ` AND (created_at<? OR (created_at=? AND id<?))`
		args = append(args, position.Created, position.Created, position.ID)
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT snapshot FROM suma_skill_catalog`+where+` ORDER BY created_at DESC,id DESC LIMIT ?`), append(args, q.Limit+1)...)
	if err != nil {
		return out, err
	}
	entries := []SkillCatalogSummary{}
	for rows.Next() {
		var raw string
		var entry SkillCatalogSummary
		if err = rows.Scan(&raw); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(raw), &entry); err != nil {
			break
		}
		entries = append(entries, entry)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(entries) > q.Limit {
		entries = entries[:q.Limit]
		last := entries[len(entries)-1]
		out.NextCursor = skillCursorEncode(skillCatalogCursor{Tenant: auth.target.tenantID, Project: q.ProjectID, Kind: "list", Filter: filter, Created: last.CreatedAt, ID: last.ID})
	}
	for _, entry := range entries {
		item, e := s.skillCatalogProjectionTx(ctx, auth, q.ProjectID, entry)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}
func (s *Service) SkillCatalogRevisions(ctx context.Context, actor Actor, project, id string, limit int, cursor string) (page SkillCatalogRevisionPage, err error) {
	page.Items = []SkillCatalogDetail{}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || !handoffID(id, false) || limit < 1 || limit > 100 {
		return page, invalid(skillInvalid)
	}
	ctx, auth, close, err := s.skillCatalogAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return page, err
	}
	defer close()
	if _, _, err = s.skillCatalogCapabilitiesTx(ctx, auth, project); err != nil {
		return page, err
	}
	if _, err = s.skillCatalogStoredTx(ctx, auth, id); err != nil {
		return page, err
	}
	position := skillCatalogCursor{}
	if cursor != "" {
		position, err = skillCursorDecode(cursor)
		if err != nil || position.Tenant != auth.target.tenantID || position.Project != project || position.Kind != "revisions" || position.ID != id || position.Version < 1 {
			return page, invalid(skillInvalid)
		}
	}
	where := ` WHERE tenant_id=? AND catalog_id=?`
	args := []any{auth.target.tenantID, id}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_skill_catalog_revisions`+where), args...).Scan(&page.Total); err != nil {
		return page, err
	}
	if cursor != "" {
		where += ` AND version<?`
		args = append(args, position.Version)
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT snapshot FROM suma_skill_catalog_revisions`+where+` ORDER BY version DESC LIMIT ?`), append(args, limit+1)...)
	if err != nil {
		return page, err
	}
	entries := []SkillCatalogSummary{}
	for rows.Next() {
		var raw string
		var entry SkillCatalogSummary
		if err = rows.Scan(&raw); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(raw), &entry); err != nil {
			break
		}
		entries = append(entries, entry)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return page, err
	}
	if len(entries) > limit {
		entries = entries[:limit]
		page.NextCursor = skillCursorEncode(skillCatalogCursor{Tenant: auth.target.tenantID, Project: project, Kind: "revisions", ID: id, Version: entries[len(entries)-1].Version})
	}
	for _, entry := range entries {
		item, e := s.skillCatalogProjectionTx(ctx, auth, project, entry)
		if e != nil {
			return page, e
		}
		item.CanAdopt = false
		item.AllowedActions = []string{}
		revision, e := s.skillOriginRevisionTx(ctx, auth.tx, auth.target.tenantID, entry.Origin)
		if e != nil {
			return page, e
		}
		page.Items = append(page.Items, SkillCatalogDetail{SkillCatalogSummary: item, Content: revision.Content})
	}
	return page, nil
}
