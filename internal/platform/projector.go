package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ProjectIndexes performs network effects after closing the source query. Failed events remain replayable.
func (s *Service) ProjectIndexes(ctx context.Context) error {
	endpoint := strings.TrimRight(os.Getenv("SUMA_KNOWLEDGE_URL"), "/")
	token := os.Getenv("SUMA_KNOWLEDGE_TOKEN")
	if endpoint == "" || token == "" {
		return nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id,tenant_id,project_id,source_id,revision,action,payload FROM suma_index_outbox WHERE state='pending' ORDER BY created_at,id LIMIT 20`)
	if err != nil {
		return err
	}
	type item struct {
		id, tenant, project, source, action, payload string
		revision                                     int
	}
	items := []item{}
	for rows.Next() {
		var i item
		if err = rows.Scan(&i.id, &i.tenant, &i.project, &i.source, &i.revision, &i.action, &i.payload); err != nil {
			rows.Close()
			return err
		}
		items = append(items, i)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	failures := []error{}
	for _, i := range items {
		var r Record
		if err = json.Unmarshal([]byte(i.payload), &r); err != nil {
			failures = append(failures, fmt.Errorf("index event %s has invalid payload", i.id))
			if e := s.failIndexEvent(ctx, i.id, i.tenant, i.project, i.source, i.revision, 0); e != nil {
				failures = append(failures, e)
			}
			continue
		}
		content := r.Description
		if body, ok := r.Data["content"].(string); ok {
			content += "\n" + body
		}
		body := map[string]any{"tenant_id": i.tenant, "project_id": i.project, "source_id": i.source, "revision": i.revision, "action": i.action}
		if i.action != "delete" {
			body["title"] = r.Title
			body["content"] = content
		}
		req, e := http.NewRequestWithContext(ctx, "POST", endpoint+"/index", bytes.NewBufferString(stringify(body)))
		if e != nil {
			failures = append(failures, fmt.Errorf("index event %s: %w", i.id, e))
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		response, e := client.Do(req)
		if e != nil {
			failures = append(failures, fmt.Errorf("index event %s: %w", i.id, e))
			continue
		}
		io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		response.Body.Close()
		if response.StatusCode/100 != 2 {
			failures = append(failures, fmt.Errorf("index event %s rejected: HTTP %d", i.id, response.StatusCode))
			if response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != 408 && response.StatusCode != 429 {
				if e = s.failIndexEvent(ctx, i.id, i.tenant, i.project, i.source, i.revision, response.StatusCode); e != nil {
					failures = append(failures, e)
				}
			}
			continue
		}
		if _, e = s.DB.ExecContext(ctx, s.q(`UPDATE suma_index_outbox SET state='indexed' WHERE id=? AND state='pending'`), i.id); e != nil {
			failures = append(failures, fmt.Errorf("index acknowledgement %s: %w", i.id, e))
			continue
		}
	}
	return errors.Join(failures...)
}

func (s *Service) failIndexEvent(ctx context.Context, id, tenant, project, source string, revision, status int) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_index_outbox SET state='failed' WHERE id=? AND state='pending'`), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return tx.Commit()
	}
	query := `INSERT INTO suma_index_failures(event_id,tenant_id,project_id,source_id,revision,http_status,reason,failed_at)VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(event_id) DO NOTHING`
	if s.Dialect == "oceanbase" {
		query = `INSERT IGNORE INTO suma_index_failures(event_id,tenant_id,project_id,source_id,revision,http_status,reason,failed_at)VALUES(?,?,?,?,?,?,?,?)`
	}
	reason := fmt.Sprintf("provider rejected immutable index event: HTTP %d", status)
	if status == 0 {
		reason = "invalid persisted index event payload"
	}
	if _, err = tx.ExecContext(ctx, s.q(query), id, tenant, project, source, revision, status, reason, now()); err != nil {
		return err
	}
	return tx.Commit()
}

// IndexStatus reads canonical delivery state; provider presence alone never
// makes an index verified. It performs no schema or activity mutations.
func (s *Service) IndexStatus(ctx context.Context, p Principal, project string) (map[string]any, error) {
	if err := s.Authorize(ctx, p, project, false, false); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, s.q(`SELECT state,COUNT(*) FROM suma_index_outbox WHERE tenant_id=? AND project_id=? GROUP BY state`), p.TenantID, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]any{"pending": 0, "indexed": 0, "failed": 0, "total": 0, "as_of": now()}
	total := 0
	for rows.Next() {
		var state string
		var count int
		if err = rows.Scan(&state, &count); err != nil {
			return nil, err
		}
		out[state] = count
		total += count
	}
	out["total"] = total
	return out, rows.Err()
}

func (s *Service) SearchIndex(ctx context.Context, p Principal, project, query string) (map[string]any, error) {
	if err := s.Authorize(ctx, p, project, false, false); err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(os.Getenv("SUMA_KNOWLEDGE_URL"), "/")
	if endpoint == "" {
		return s.SearchKeywords(ctx, p, project, query)
	}
	if len(query) < 1 || len(query) > 500 {
		return nil, invalid("查询长度超出限制")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint+"/search", bytes.NewBufferString(stringify(map[string]any{"tenant_id": p.TenantID, "project_id": project, "query": query, "limit": 50})))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("SUMA_KNOWLEDGE_TOKEN"))
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, &APIError{503, "index_unavailable", "检索服务暂不可用；资料原件仍可访问", nil}
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, &APIError{503, "index_unavailable", "检索服务尚未通过查询验证", nil}
	}
	var result map[string]any
	if err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&result); err != nil {
		return nil, err
	}
	// Recheck canonical source revision before revealing titles/snippets to the caller.
	filtered := []any{}
	hits, _ := result["items"].([]any)
	for _, v := range hits {
		hit, ok := v.(map[string]any)
		if !ok {
			continue
		}
		source := str(hit, "source_ref")
		record, e := s.Get(ctx, p, project, "documents", source)
		if e != nil || record.Status == "archived" || record.Status == "deleted" {
			continue
		}
		revision := number(hit, "revision")
		if revision != record.Version {
			continue
		}
		hit["locator"] = map[string]any{"project_id": project, "type": "documents", "id": source}
		filtered = append(filtered, hit)
	}
	result["items"] = filtered
	return result, nil
}
