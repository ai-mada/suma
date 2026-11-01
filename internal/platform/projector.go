package platform

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	ic "github.com/ai-mada/suma/internal/indexcontract"
	"github.com/ai-mada/suma/internal/strictjson"
)

// ProjectIndexes sends only the original immutable typed event after releasing
// its source query. An HTTP success alone is never a persistence receipt.
func (s *Service) ProjectIndexes(ctx context.Context) error {
	if os.Getenv("SUMA_KNOWLEDGE_URL") == "" || os.Getenv("SUMA_KNOWLEDGE_TOKEN") == "" {
		return nil
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT id,tenant_id,project_id,source_kind,source_id,revision,content_revision,source_sha256,action,payload FROM suma_index_outbox WHERE state='pending' ORDER BY created_at,id LIMIT 20")
	if err != nil {
		return err
	}
	type event struct {
		id, tenant, project, kind, source, sha, action, payload string
		sequence, contentRevision                               int
	}
	items := []event{}
	for rows.Next() {
		var i event
		if err = rows.Scan(&i.id, &i.tenant, &i.project, &i.kind, &i.source, &i.sequence, &i.contentRevision, &i.sha, &i.action, &i.payload); err != nil {
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
	failures := []error{}
	for _, i := range items {
		raw := []byte(i.payload)
		var request ic.Request
		source := ic.Source{Kind: i.kind, ID: i.source, Sequence: i.sequence, Revision: i.contentRevision, SHA256: i.sha}
		if strictjson.Decode(raw, &request, 8<<20) != nil || ic.ValidateRequest(request) != nil || request.EventID != i.id || request.TenantID != i.tenant || request.ProjectID != i.project || request.Source != source || request.Action != i.action {
			failures = append(failures, fmt.Errorf("index event %s has invalid typed payload", i.id))
			if e := s.failIndexEvent(ctx, i.id, i.tenant, i.project, i.kind, i.source, i.sequence, 0); e != nil {
				failures = append(failures, e)
			}
			continue
		}
		receipt, status, e := s.postTypedIndex(ctx, request, raw)
		if e != nil {
			failures = append(failures, fmt.Errorf("index event %s: %w", i.id, e))
			if status >= 400 && status < 500 && status != 408 && status != 429 {
				if e = s.failIndexEvent(ctx, i.id, i.tenant, i.project, i.kind, i.source, i.sequence, status); e != nil {
					failures = append(failures, e)
				}
			}
			continue
		}
		if _, e = s.DB.ExecContext(ctx, s.q("UPDATE suma_index_outbox SET state='indexed',receipt=?,indexed_at=? WHERE id=? AND state='pending'"), stringify(receipt), now(), i.id); e != nil {
			failures = append(failures, fmt.Errorf("index acknowledgement %s: %w", i.id, e))
		}
	}
	return errors.Join(failures...)
}

func (s *Service) failIndexEvent(ctx context.Context, id, tenant, project, kind, source string, revision, status int) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, s.q("UPDATE suma_index_outbox SET state='failed' WHERE id=? AND state='pending'"), id)
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
	query := "INSERT INTO suma_index_failures(event_id,tenant_id,project_id,source_kind,source_id,revision,http_status,reason,failed_at)VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(event_id) DO NOTHING"
	if s.Dialect == "oceanbase" {
		query = "INSERT IGNORE INTO suma_index_failures(event_id,tenant_id,project_id,source_kind,source_id,revision,http_status,reason,failed_at)VALUES(?,?,?,?,?,?,?,?,?)"
	}
	reason := fmt.Sprintf("provider rejected immutable index event: HTTP %d", status)
	if status == 0 {
		reason = "invalid persisted typed index event payload"
	}
	if _, err = tx.ExecContext(ctx, s.q(query), id, tenant, project, kind, source, revision, status, reason, now()); err != nil {
		return err
	}
	return tx.Commit()
}

// IndexStatus reads canonical delivery state; provider presence alone never
// makes an index verified. It performs no schema or activity mutations.
func (s *Service) IndexStatus(ctx context.Context, actor Actor, project string) (out map[string]any, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.indexStatusTx(bounded, auth, project)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) indexStatusTx(ctx context.Context, auth *authorizedCommand, project string) (map[string]any, error) {
	if err := assertProjectRead(auth, project); err != nil {
		return nil, err
	}
	p := auth.currentPrincipal
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT state,COUNT(*) FROM suma_index_outbox WHERE tenant_id=? AND project_id=? GROUP BY state`), p.TenantID, project)
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

func (s *Service) SearchIndex(ctx context.Context, actor Actor, project, query string) (map[string]any, error) {
	if os.Getenv("SUMA_KNOWLEDGE_URL") == "" {
		return s.SearchKeywords(ctx, actor, project, query)
	}
	var tenant string
	target := commandTarget{tenantID: actor.credential.tenantID, projectID: project}
	purpose := knowledgeReadPurpose(actor)
	if err := s.withCurrentRead(ctx, actor, target, purpose, func(auth *authorizedCommand) error { tenant = auth.target.tenantID; return nil }); err != nil {
		return nil, err
	}
	result, err := s.queryTypedIndex(ctx, ic.SearchRequest{Protocol: ic.Protocol, TenantID: tenant, ProjectID: project, SourceKind: "documents", Query: query, Limit: 50})
	if err != nil {
		if authErr := s.withCurrentRead(ctx, actor, target, purpose, func(*authorizedCommand) error { return nil }); authErr != nil {
			return nil, authErr
		}
		return nil, err
	}
	items := []any{}
	discarded := 0
	// Provider locators identify candidates only. Current source, version, digest,
	// permissions and excerpt bytes are resolved again in one current snapshot.
	err = s.withCurrentRead(ctx, actor, target, purpose, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		for _, hit := range result.Items {
			record, e := s.getRecordTx(bounded, auth, project, "documents", hit.Source.ID)
			if e != nil {
				var api *APIError
				if errors.As(e, &api) && api.Status == http.StatusNotFound {
					discarded++
					continue
				}
				return e
			}
			content := indexedDocumentContent(record)
			if record.Status == "archived" || record.Status == "deleted" || record.Version != hit.Source.Sequence || record.Version != hit.Source.Revision || ic.Digest([]byte(content)) != hit.Source.SHA256 {
				discarded++
				continue
			}
			points := []rune(content)
			if hit.Locator.End > len(points) {
				discarded++
				continue
			}
			excerpt := string(points[hit.Locator.Start:hit.Locator.End])
			var preview strings.Builder
			for _, point := range excerpt {
				if preview.Len()+len(string(point)) > 600 {
					break
				}
				preview.WriteRune(point)
			}
			locator := map[string]any{"project_id": project, "type": "documents", "id": record.ID, "unit": hit.Locator.Unit, "start": hit.Locator.Start, "end": hit.Locator.End}
			items = append(items, map[string]any{"id": ic.Digest([]byte(stringify(hit))), "type": "documents", "title": record.Title, "snippet": preview.String(), "source_ref": record.ID, "revision": record.Version, "source": hit.Source, "locator": locator})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": items, "mode": result.Mode, "generation": result.Generation, "index_watermark": result.IndexWatermark, "watermark_kind": "provider_local_projection_position", "candidates_examined": len(result.Items), "discarded": discarded, "coverage": "current_authorized_document_candidates"}, nil
}
