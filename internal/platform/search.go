package platform

import (
	"context"
	"encoding/json"
	"strings"
)

// SearchKeywords filters in SQL across the entire authorized project. The
// result limit bounds the response, never the set of records searched.
func knowledgeReadPurpose(actor Actor) commandPurpose {
	if actor.kind == actorAgentRead {
		return purposeAgentRead
	}
	return purposeProjectRead
}

func (s *Service) SearchKeywords(ctx context.Context, actor Actor, project, query string) (out map[string]any, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, knowledgeReadPurpose(actor), func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.searchKeywordsTx(bounded, auth, project, query)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) searchKeywordsTx(ctx context.Context, auth *authorizedCommand, project, query string) (map[string]any, error) {
	if err := assertProjectRead(auth, project); err != nil {
		return nil, err
	}
	p := auth.currentPrincipal
	tx := auth.tx
	query = strings.TrimSpace(query)
	if len(query) < 1 || len(query) > 500 {
		return nil, invalid("请输入 1–500 字节的查询")
	}
	pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(query)) + "%"
	var selects []string
	var args []any
	for _, kind := range []string{"requirements", "tasks", "issues", "documents"} {
		// Identifiers come exclusively from this fixed, server-owned set.
		visibility := ""
		if kind == "documents" {
			visibility = ` AND status NOT IN ('archived','deleted')`
		}
		selects = append(selects, `SELECT id,title,description,version,data,updated_at,'`+kind+`' AS source_kind FROM suma_`+kind+` WHERE tenant_id=? AND project_id=?`+visibility+` AND (LOWER(title) LIKE ? ESCAPE '!' OR LOWER(description) LIKE ? ESCAPE '!' OR LOWER(data) LIKE ? ESCAPE '!')`)
		args = append(args, p.TenantID, project, pattern, pattern, pattern)
	}
	base := strings.Join(selects, " UNION ALL ")
	var total int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM (`+base+`) AS matches`), args...).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, s.q(base+` ORDER BY updated_at DESC,id ASC,source_kind ASC LIMIT 50`), args...)
	if err != nil {
		return nil, err
	}
	items := []map[string]any{}
	for rows.Next() {
		var id, title, body, data, updated, kind string
		var version int
		if err = rows.Scan(&id, &title, &body, &version, &data, &updated, &kind); err != nil {
			rows.Close()
			return nil, err
		}
		var detail map[string]any
		if json.Unmarshal([]byte(data), &detail) == nil {
			if content, ok := detail["content"].(string); ok {
				body += "\n" + content
			}
		}
		runes := []rune(body)
		if len(runes) > 200 {
			body = string(runes[:200])
		}
		items = append(items, map[string]any{"id": id, "type": kind, "title": title, "snippet": body, "source_ref": id, "revision": version, "locator": map[string]any{"project_id": project, "type": kind, "id": id}})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": items, "total": total, "limit": 50, "truncated": total > len(items), "mode": "keyword", "index_watermark": nil, "coverage": "当前授权项目的全部需求、任务、问题和文档；返回最多 50 条"}, nil
}
