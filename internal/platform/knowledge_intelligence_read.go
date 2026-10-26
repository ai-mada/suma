package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

func (s *Service) ListKnowledgeIntelligence(ctx context.Context, actor Actor, project string, limit int, before string) (page KnowledgeIntelligencePage, err error) {
	page.Items = []KnowledgeIntelligenceAnalysis{}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || !handoffID(before, true) || limit < 1 || limit > 100 {
		return page, invalid(intelligenceInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeProjectRead)
	if err != nil {
		return page, err
	}
	defer close()
	if auth.actor.kind != actorBrowser && auth.actor.kind != actorProjectAPI {
		return page, forbidden()
	}
	tenant := auth.target.tenantID
	where := ` WHERE tenant_id=? AND project_id=?`
	args := []any{tenant, project}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_knowledge_intelligence`+where), args...).Scan(&page.Total); err != nil {
		return page, err
	}
	if before != "" {
		var stamp string
		err = auth.tx.QueryRowContext(ctx, s.q(`SELECT created_at FROM suma_knowledge_intelligence`+where+` AND id=?`), tenant, project, before).Scan(&stamp)
		if errors.Is(err, sql.ErrNoRows) {
			return page, invalid(intelligenceInvalid)
		}
		if err != nil {
			return page, err
		}
		where += ` AND (created_at<? OR (created_at=? AND id<?))`
		args = append(args, stamp, stamp, before)
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT payload FROM suma_knowledge_intelligence`+where+` ORDER BY created_at DESC,id DESC LIMIT ?`), append(args, limit+1)...)
	if err != nil {
		return page, err
	}
	entries := []knowledgeIntelligenceRecord{}
	for rows.Next() {
		var raw string
		var r knowledgeIntelligenceRecord
		if err = rows.Scan(&raw); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(raw), &r); err != nil {
			break
		}
		entries = append(entries, r)
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
		page.NextCursor = entries[len(entries)-1].Analysis.ID
	}
	for _, r := range entries {
		item, e := s.knowledgeIntelligenceProjectionTx(ctx, auth, r.Analysis)
		if e != nil {
			return page, e
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}
