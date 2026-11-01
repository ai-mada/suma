package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	ic "github.com/ai-mada/suma/internal/indexcontract"
)

func knowledgeIndexInvalid() error {
	return invalidDiagnostic("knowledge.index_query_invalid", "项目知识索引查询无效", nil)
}
func normalizeKnowledgeIndexQuery(q ProjectKnowledgeIndexQuery) (ProjectKnowledgeIndexQuery, error) {
	if !utf8.ValidString(q.Query) || len(q.Query) > 200 || strings.IndexFunc(q.Query, unicode.IsControl) >= 0 || q.Kind != "" && !knowledgeKind(q.Kind) {
		return q, knowledgeIndexInvalid()
	}
	q.Query = strings.TrimSpace(q.Query)
	if q.Query == "" {
		return q, knowledgeIndexInvalid()
	}
	if q.Limit == 0 {
		q.Limit = 10
	}
	if q.Limit < 1 || q.Limit > 20 {
		return q, knowledgeIndexInvalid()
	}
	return q, nil
}

func (s *Service) SearchProjectKnowledgeIndex(ctx context.Context, actor Actor, project string, query ProjectKnowledgeIndexQuery) (ProjectKnowledgeIndexSearch, error) {
	return s.searchProjectKnowledgeIndex(ctx, actor, project, func() (ProjectKnowledgeIndexQuery, error) { return query, nil })
}

// Validation belongs after current authorization. SQL locks are released before
// provider IO and acquired anew even when the provider returns zero candidates.
func (s *Service) searchProjectKnowledgeIndex(ctx context.Context, actor Actor, project string, parse func() (ProjectKnowledgeIndexQuery, error)) (out ProjectKnowledgeIndexSearch, err error) {
	target := commandTarget{tenantID: actor.credential.tenantID, projectID: project}
	var tenant string
	var query ProjectKnowledgeIndexQuery
	err = s.withCurrentRead(ctx, actor, target, knowledgeReadPurpose(actor), func(auth *authorizedCommand) error {
		q, e := parse()
		if e != nil {
			return e
		}
		query, e = normalizeKnowledgeIndexQuery(q)
		tenant = auth.target.tenantID
		return e
	})
	if err != nil {
		return out, err
	}
	result, err := s.queryTypedIndex(ctx, ic.SearchRequest{Protocol: ic.Protocol, TenantID: tenant, ProjectID: project, SourceKind: "project_knowledge", Query: query.Query, Limit: ic.MaxCandidates})
	if err != nil {
		// A phase/error reply is still information obtained across the IO
		// boundary. Revocation wins over disclosing provider availability.
		if authErr := s.withCurrentRead(ctx, actor, target, knowledgeReadPurpose(actor), func(*authorizedCommand) error { return nil }); authErr != nil {
			return out, authErr
		}
		return out, err
	}
	out = ProjectKnowledgeIndexSearch{Items: []ProjectKnowledgeIndexHit{}, Mode: result.Mode, Generation: result.Generation, IndexWatermark: result.IndexWatermark, Scope: "provider_candidates"}
	err = s.withCurrentRead(ctx, actor, target, knowledgeReadPurpose(actor), func(auth *authorizedCommand) error {
		seen := map[string]bool{}
		for _, candidate := range result.Items {
			if len(out.Items) == query.Limit {
				break
			}
			out.CandidatesExamined++
			source := candidate.Source
			if source.Kind != "project_knowledge" || seen[source.ID] {
				out.Discarded++
				continue
			}
			ref := ProjectKnowledgeReference{ID: source.ID, Revision: source.Revision, SHA256: source.SHA256}
			items, e := s.resolvePublishedKnowledgeTx(auth.ctx, auth, project, []ProjectKnowledgeReference{ref})
			if e != nil {
				var api *APIError
				if errors.As(e, &api) && (api.Status == 404 || api.Status == 409) {
					out.Discarded++
					continue
				}
				return e
			}
			item := items[0]
			// A provider may recall an exact publication while forging its event
			// order. Bind every exposed source tuple to its original local upsert,
			// not today's aggregate version (which may include a newer draft).
			var eventID string
			e = auth.tx.QueryRowContext(auth.ctx, s.q(`SELECT id FROM suma_index_outbox WHERE tenant_id=? AND project_id=? AND source_kind='project_knowledge' AND source_id=? AND revision=? AND content_revision=? AND source_sha256=? AND action='upsert'`), auth.target.tenantID, project, source.ID, source.Sequence, source.Revision, source.SHA256).Scan(&eventID)
			if errors.Is(e, sql.ErrNoRows) {
				out.Discarded++
				continue
			}
			if e != nil {
				return e
			}
			if query.Kind != "" && query.Kind != item.Kind {
				out.Discarded++
				continue
			}
			body := []rune(item.Content)
			if ic.ValidateLocator(candidate.Locator) != nil || candidate.Locator.End > len(body) {
				out.Discarded++
				continue
			}
			end := candidate.Locator.Start
			bytes := 0
			for end < candidate.Locator.End && bytes+utf8.RuneLen(body[end]) <= 600 {
				bytes += utf8.RuneLen(body[end])
				end++
			}
			if end == candidate.Locator.Start {
				out.Discarded++
				continue
			}
			detail, e := s.knowledgeDetailTx(auth.ctx, auth, project, source.ID)
			if e != nil {
				return e
			}
			locator := candidate.Locator
			locator.End = end
			hit := ProjectKnowledgeIndexHit{Reference: ref, ProjectID: project, Kind: item.Kind, Title: item.Title, Summary: item.Summary, Snippet: string(body[locator.Start:locator.End]), SourceCount: len(item.Sources), PublishedAt: detail.Published.CreatedAt, Source: source, Locator: locator}
			// Enforce the actual serialized response budget, including JSON escaping.
			trial := out
			trial.Items = append(append([]ProjectKnowledgeIndexHit{}, out.Items...), hit)
			raw, e := json.Marshal(trial)
			if e != nil {
				return e
			}
			if len(raw) > (48<<10)-128 {
				out.Discarded++
				continue
			}
			seen[source.ID] = true
			out.Items = append(out.Items, hit)
		}
		return nil
	})
	if err != nil {
		return ProjectKnowledgeIndexSearch{}, err
	}
	return out, nil
}

func (s *Service) ProjectKnowledgeIndexStatus(ctx context.Context, actor Actor, project string) (out ProjectKnowledgeIndexStatus, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, knowledgeReadPurpose(actor), func(auth *authorizedCommand) error {
		var e error
		out, e = s.knowledgeIndexStatusTx(auth.ctx, auth, project)
		return e
	})
	return
}
func (s *Service) knowledgeIndexStatusTx(ctx context.Context, auth *authorizedCommand, project string) (out ProjectKnowledgeIndexStatus, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return out, err
	}
	out = ProjectKnowledgeIndexStatus{Domain: "project_knowledge", AsOf: auth.checkedAt.UTC().Format(time.RFC3339Nano), Configured: strings.TrimSpace(os.Getenv("SUMA_KNOWLEDGE_URL")) != "" && strings.TrimSpace(os.Getenv("SUMA_KNOWLEDGE_TOKEN")) != "", Coverage: "outbox_delivery_only"}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT state,COUNT(*) FROM suma_index_outbox WHERE tenant_id=? AND project_id=? AND source_kind='project_knowledge' GROUP BY state`), auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var state string
		var count int
		if err = rows.Scan(&state, &count); err != nil {
			rows.Close()
			return out, err
		}
		switch state {
		case "pending":
			out.Pending = count
		case "indexed":
			out.Indexed = count
		case "failed":
			out.Failed = count
		default:
			rows.Close()
			return out, errors.New("invalid index delivery state")
		}
		out.Total += count
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_project_knowledge k WHERE k.tenant_id=? AND k.project_id=? AND k.state<>'retired' AND k.published_revision>0`), auth.target.tenantID, project).Scan(&out.CurrentPublications)
	if err != nil {
		return out, err
	}
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_project_knowledge k WHERE k.tenant_id=? AND k.project_id=? AND k.state<>'retired' AND k.published_revision>0 AND NOT EXISTS(SELECT 1 FROM suma_index_outbox o WHERE o.tenant_id=k.tenant_id AND o.project_id=k.project_id AND o.source_kind='project_knowledge' AND o.source_id=k.id AND o.content_revision=k.published_revision AND o.action='upsert')`), auth.target.tenantID, project).Scan(&out.UntrackedPublications)
	if err != nil {
		return out, err
	}
	var event ProjectKnowledgeProjectionEvent
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT id,source_kind,source_id,revision,content_revision,source_sha256,action,state,created_at FROM suma_index_outbox WHERE tenant_id=? AND project_id=? AND source_kind='project_knowledge' ORDER BY created_at DESC,id DESC LIMIT 1`), auth.target.tenantID, project).Scan(&event.EventID, &event.Source.Kind, &event.Source.ID, &event.Source.Sequence, &event.Source.Revision, &event.Source.SHA256, &event.Action, &event.State, &event.CreatedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if err == nil {
		out.LatestEvent = &event
	}
	var raw, receiptRaw string
	var acknowledged ProjectKnowledgeProjectionEvent
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT payload,receipt,id,source_kind,source_id,revision,content_revision,source_sha256,action FROM suma_index_outbox WHERE tenant_id=? AND project_id=? AND source_kind='project_knowledge' AND state='indexed' AND receipt<>'' ORDER BY indexed_at DESC,id DESC LIMIT 1`), auth.target.tenantID, project).Scan(&raw, &receiptRaw, &acknowledged.EventID, &acknowledged.Source.Kind, &acknowledged.Source.ID, &acknowledged.Source.Sequence, &acknowledged.Source.Revision, &acknowledged.Source.SHA256, &acknowledged.Action)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	var request ic.Request
	var receipt ic.Receipt
	if json.Unmarshal([]byte(raw), &request) != nil || ic.ValidateRequest(request) != nil || request.TenantID != auth.target.tenantID || request.ProjectID != project || request.EventID != acknowledged.EventID || request.Source != acknowledged.Source || request.Action != acknowledged.Action || json.Unmarshal([]byte(receiptRaw), &receipt) != nil || !ic.ReceiptMatches(receipt, request, []byte(raw)) {
		return out, errors.New("invalid persisted index acknowledgement")
	}
	out.LastAcknowledgement = &receipt
	return out, nil
}
