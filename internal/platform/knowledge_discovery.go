package platform

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const knowledgeDiscoveryScanLimit = 200
const knowledgeDiscoveryEntryBudget = 12 << 10
const knowledgeDiscoveryBodyBudget = 46 << 10

type knowledgeDiscoveryCursor struct {
	Tenant       string `json:"tenant"`
	Project      string `json:"project"`
	Actor        string `json:"actor"`
	Filter       string `json:"filter"`
	UpperCreated string `json:"upper_created"`
	UpperID      string `json:"upper_id"`
	AfterCreated string `json:"after_created"`
	AfterID      string `json:"after_id"`
}
type knowledgeDiscoveryCandidate struct {
	ID, Created, Kind string
	Revision          int
	Snapshot          sql.NullString
}

func knowledgeDiscoveryInvalid() error {
	return invalidDiagnostic("validation.knowledge_discovery", "项目知识发现筛选或游标无效", nil)
}
func normalizeKnowledgeDiscovery(q ProjectKnowledgeDiscoveryQuery) (ProjectKnowledgeDiscoveryQuery, error) {
	if !utf8.ValidString(q.Query) || len(q.Query) > 200 || strings.IndexFunc(q.Query, unicode.IsControl) >= 0 || len(q.Cursor) > 2048 || q.Kind != "" && !knowledgeKind(q.Kind) {
		return q, knowledgeDiscoveryInvalid()
	}
	q.Query = strings.ToLower(strings.TrimSpace(q.Query))
	if len(q.Query) > 200 {
		return q, knowledgeDiscoveryInvalid()
	}
	if q.Limit == 0 {
		q.Limit = 10
	}
	if q.Limit < 1 || q.Limit > 20 {
		return q, knowledgeDiscoveryInvalid()
	}
	return q, nil
}
func knowledgeDiscoveryFilter(q ProjectKnowledgeDiscoveryQuery) string {
	return digest(stringify([]any{q.Query, q.Kind, q.Limit}))
}
func knowledgeDiscoveryCursorRead(auth *authorizedCommand, project string, q ProjectKnowledgeDiscoveryQuery) (out knowledgeDiscoveryCursor, err error) {
	if q.Cursor == "" {
		return out, nil
	}
	raw, e := base64.RawURLEncoding.DecodeString(q.Cursor)
	if e != nil {
		return out, knowledgeDiscoveryInvalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil || decoder.Decode(new(any)) != io.EOF || out.Tenant != auth.target.tenantID || out.Project != project || out.Actor != auth.currentPrincipal.ID || out.Filter != knowledgeDiscoveryFilter(q) || !handoffID(out.UpperID, false) || !handoffID(out.AfterID, false) {
		return out, knowledgeDiscoveryInvalid()
	}
	if _, e = time.Parse(time.RFC3339Nano, out.UpperCreated); e != nil {
		return out, knowledgeDiscoveryInvalid()
	}
	if _, e = time.Parse(time.RFC3339Nano, out.AfterCreated); e != nil {
		return out, knowledgeDiscoveryInvalid()
	}
	if out.AfterCreated > out.UpperCreated || out.AfterCreated == out.UpperCreated && out.AfterID < out.UpperID {
		return out, knowledgeDiscoveryInvalid()
	}
	return out, nil
}

func (s *Service) DiscoverProjectKnowledge(ctx context.Context, actor Actor, project string, q ProjectKnowledgeDiscoveryQuery) (out ProjectKnowledgeDiscovery, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, knowledgeReadPurpose(actor), func(auth *authorizedCommand) error {
		var e error
		out, e = s.discoverProjectKnowledgeTx(auth.ctx, auth, project, q)
		return e
	})
	if err != nil {
		return ProjectKnowledgeDiscovery{}, err
	}
	return out, nil
}

// A bounded, resumable scan of the domain's current publication pointers avoids
// JSON LIKE false positives/escaping losses and cross-dialect Unicode folding.
// It never calls a provider/index or reports the first 200 records as complete.
func (s *Service) discoverProjectKnowledgeTx(ctx context.Context, auth *authorizedCommand, project string, q ProjectKnowledgeDiscoveryQuery) (out ProjectKnowledgeDiscovery, err error) {
	out = ProjectKnowledgeDiscovery{Items: []ProjectKnowledgeDiscoveryHit{}, Mode: "keyword", Scope: "current_project_publications", ScanLimit: knowledgeDiscoveryScanLimit, Exhausted: true}
	if err = assertProjectRead(auth, project); err != nil {
		return out, err
	}
	q, err = normalizeKnowledgeDiscovery(q)
	if err != nil {
		return out, err
	}
	cursor, err := knowledgeDiscoveryCursorRead(auth, project, q)
	if err != nil {
		return out, err
	}
	where := ` WHERE a.tenant_id=? AND a.project_id=? AND a.state<>'retired' AND a.published_revision>0`
	args := []any{auth.target.tenantID, project}
	if q.Kind != "" {
		where += ` AND a.kind=?`
		args = append(args, q.Kind)
	}
	if q.Cursor != "" {
		where += ` AND (a.created_at<? OR (a.created_at=? AND a.id>=?)) AND (a.created_at<? OR (a.created_at=? AND a.id>?))`
		args = append(args, cursor.UpperCreated, cursor.UpperCreated, cursor.UpperID, cursor.AfterCreated, cursor.AfterCreated, cursor.AfterID)
	}
	args = append(args, knowledgeDiscoveryScanLimit+1)
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT a.id,a.created_at,a.kind,a.published_revision,p.snapshot FROM suma_project_knowledge a LEFT JOIN suma_project_knowledge_revisions p ON p.tenant_id=a.tenant_id AND p.project_id=a.project_id AND p.knowledge_id=a.id AND p.revision=a.published_revision`+where+` ORDER BY a.created_at DESC,a.id ASC LIMIT ?`), args...)
	if err != nil {
		return out, err
	}
	candidates := []knowledgeDiscoveryCandidate{}
	for rows.Next() {
		var candidate knowledgeDiscoveryCandidate
		if err = rows.Scan(&candidate.ID, &candidate.Created, &candidate.Kind, &candidate.Revision, &candidate.Snapshot); err != nil {
			rows.Close()
			return out, err
		}
		candidates = append(candidates, candidate)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(candidates) == 0 {
		return out, nil
	}
	if q.Cursor == "" {
		cursor = knowledgeDiscoveryCursor{Tenant: auth.target.tenantID, Project: project, Actor: auth.currentPrincipal.ID, Filter: knowledgeDiscoveryFilter(q), UpperCreated: candidates[0].Created, UpperID: candidates[0].ID}
	}
	bodyBytes := 0
	for index, candidate := range candidates {
		// Budget is checked before consuming a candidate, leaving it for the next
		// page. Reserve the maximum encoded entry and 2KiB for envelope/cursor.
		if out.Scanned >= knowledgeDiscoveryScanLimit || len(out.Items) >= q.Limit || bodyBytes > knowledgeDiscoveryBodyBudget-knowledgeDiscoveryEntryBudget {
			out.Exhausted = false
			break
		}
		out.Scanned++
		cursor.AfterCreated, cursor.AfterID = candidate.Created, candidate.ID
		published, e := knowledgeDiscoveryRevision(candidate)
		if e != nil {
			return out, e
		}
		field, text, matched := knowledgeDiscoveryMatch(published, q.Query)
		if matched {
			reference := ProjectKnowledgeReference{ID: candidate.ID, Revision: published.Revision, SHA256: published.SHA256}
			resolved, e := s.resolvePublishedKnowledgeTx(ctx, auth, project, []ProjectKnowledgeReference{reference})
			if e != nil {
				var api *APIError
				if !errors.As(e, &api) || api.Status != 409 {
					return out, e
				}
			} else {
				if len(resolved) != 1 || resolved[0].ID != reference.ID || resolved[0].Revision != reference.Revision || resolved[0].SHA256 != reference.SHA256 {
					return out, fmt.Errorf("published knowledge discovery resolution is inconsistent")
				}
				fixed := resolved[0]
				hit := ProjectKnowledgeDiscoveryHit{Reference: reference, ProjectID: project, Kind: fixed.Kind, Title: fixed.Title, Summary: fixed.Summary, Snippet: knowledgeDiscoverySnippet(text, q.Query), MatchedField: field, SourceCount: len(fixed.Sources), PublishedAt: published.CreatedAt}
				encoded, e := json.Marshal(hit)
				if e != nil {
					return out, e
				}
				if len(encoded)+1 > knowledgeDiscoveryEntryBudget {
					return out, fmt.Errorf("published knowledge discovery entry exceeds its encoded bound")
				}
				bodyBytes += len(encoded) + 1
				out.Items = append(out.Items, hit)
			}
		}
		if index == len(candidates)-1 {
			out.Exhausted = true
		}
	}
	if !out.Exhausted {
		encoded, e := json.Marshal(cursor)
		if e != nil {
			return out, e
		}
		out.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	return out, nil
}

func knowledgeDiscoveryRevision(candidate knowledgeDiscoveryCandidate) (out ProjectKnowledgeRevision, err error) {
	if !candidate.Snapshot.Valid || len(candidate.Snapshot.String) > 2<<20 || json.Unmarshal([]byte(candidate.Snapshot.String), &out) != nil || out.Revision != candidate.Revision || out.Kind != candidate.Kind || !knowledgeKind(out.Kind) || out.State != "published" || !knowledgeMethod(out.Method) || !contentDigest.MatchString(out.SHA256) || knowledgeRevisionHash(out) != out.SHA256 || !knowledgeText(out.Title, 200) || !knowledgeText(out.Summary, 1000) || !knowledgeText(out.Content, 16000) || len(out.Sources) > 8 {
		return out, fmt.Errorf("published knowledge discovery source is inconsistent")
	}
	if _, e := time.Parse(time.RFC3339Nano, out.CreatedAt); e != nil || len(out.CreatedAt) > 40 {
		return out, fmt.Errorf("published knowledge discovery time is inconsistent")
	}
	return out, nil
}
func knowledgeDiscoveryMatch(revision ProjectKnowledgeRevision, query string) (field, text string, matched bool) {
	if query == "" {
		return "none", revision.Content, true
	}
	for _, part := range []struct{ field, text string }{{"title", revision.Title}, {"summary", revision.Summary}, {"content", revision.Content}} {
		if strings.Contains(strings.ToLower(part.text), query) {
			return part.field, part.text, true
		}
	}
	return "", "", false
}
func knowledgeDiscoverySnippet(text, query string) string {
	runes := []rune(text)
	start := 0
	if query != "" {
		folded := strings.ToLower(text)
		if pos := strings.Index(folded, query); pos >= 0 {
			start = utf8.RuneCountInString(folded[:pos]) - 40
			if start < 0 {
				start = 0
			}
		}
	}
	end, size := start, 0
	for end < len(runes) {
		n := utf8.RuneLen(runes[end])
		if size+n > 594 {
			break
		}
		size += n
		end++
	}
	snippet := string(runes[start:end])
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(runes) {
		snippet += "…"
	}
	return snippet
}
