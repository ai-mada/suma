package platform

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Aggregates contain pointers and CAS state only. Every draft edit, publish and
// retirement appends a complete immutable revision; old runs keep their snapshot.
func (s *Service) InitializeProjectKnowledge(ctx context.Context) error {
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS suma_project_knowledge(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,kind VARCHAR(20) NOT NULL,state VARCHAR(20) NOT NULL,latest_revision INTEGER NOT NULL,published_revision INTEGER NOT NULL,title VARCHAR(200) NOT NULL,summary TEXT NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL,PRIMARY KEY(tenant_id,project_id,id))`,
		`CREATE TABLE IF NOT EXISTS suma_project_knowledge_revisions(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,knowledge_id VARCHAR(64) NOT NULL,revision INTEGER NOT NULL,snapshot TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,knowledge_id,revision))`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
			return err
		}
	}
	if err := s.InitializeSkillCatalog(ctx); err != nil {
		return err
	}
	return s.InitializeKnowledgeIntelligence(ctx)
}

const knowledgeSelect = `SELECT a.id,a.project_id,a.version,a.kind,a.state,a.latest_revision,a.published_revision,a.created_at,a.updated_at,l.snapshot,p.snapshot FROM suma_project_knowledge a JOIN suma_project_knowledge_revisions l ON l.tenant_id=a.tenant_id AND l.project_id=a.project_id AND l.knowledge_id=a.id AND l.revision=a.latest_revision LEFT JOIN suma_project_knowledge_revisions p ON p.tenant_id=a.tenant_id AND p.project_id=a.project_id AND p.knowledge_id=a.id AND p.revision=a.published_revision`

func knowledgeKind(kind string) bool { return kind == "memory" || kind == "spec" || kind == "skill" }
func knowledgeState(state string) bool {
	return state == "draft" || state == "published" || state == "retired"
}
func knowledgeSourceKind(kind string) bool {
	return kind == "documents" || kind == "requirements" || kind == "tasks" || kind == "issues"
}
func knowledgeText(value string, max int) bool {
	return utf8.ValidString(value) && len(value) <= max && !strings.ContainsRune(value, 0)
}

func knowledgeRevisionHash(r ProjectKnowledgeRevision) string {
	return digest(stringify(struct {
		Kind, Title, Summary, Content, Method string
		Sources                               []ProjectKnowledgeSource
	}{r.Kind, r.Title, r.Summary, r.Content, r.Method, r.Sources}))
}
func scanKnowledge(row scanner) (ProjectKnowledgeDetail, error) {
	var out ProjectKnowledgeDetail
	var latest string
	var published sql.NullString
	err := row.Scan(&out.ID, &out.ProjectID, &out.Version, &out.Kind, &out.State, &out.LatestRevision, &out.PublishedRevision, &out.CreatedAt, &out.UpdatedAt, &latest, &published)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(latest), &out.Latest); err != nil {
		return out, err
	}
	if out.Latest.Revision != out.LatestRevision || out.Latest.Kind != out.Kind || !knowledgeMethod(out.Latest.Method) || knowledgeRevisionHash(out.Latest) != out.Latest.SHA256 {
		return out, fmt.Errorf("project knowledge latest revision is inconsistent")
	}
	// Response metadata comes from the validated latest revision; aggregate
	// title/summary are only query accelerators and must not replace approval.
	out.Title, out.Summary = out.Latest.Title, out.Latest.Summary
	out.HasDraft = out.Latest.State == "draft"
	out.Eligibility = "none"
	if out.State == "retired" {
		out.Eligibility = "retired"
	}
	if out.PublishedRevision > 0 {
		if !published.Valid {
			return out, fmt.Errorf("project knowledge published revision is missing")
		}
		var revision ProjectKnowledgeRevision
		if err = json.Unmarshal([]byte(published.String), &revision); err != nil {
			return out, err
		}
		if revision.Revision != out.PublishedRevision || revision.Kind != out.Kind || revision.State != "published" || !knowledgeMethod(revision.Method) || knowledgeRevisionHash(revision) != revision.SHA256 {
			return out, fmt.Errorf("project knowledge published revision is inconsistent")
		}
		out.Published = &revision
		out.PublishedSHA256 = revision.SHA256
		out.PublishedTitle = revision.Title
		out.PublishedSummary = revision.Summary
		out.Eligibility = "current"
		out.PublishedEligible = true
	}
	return out, nil
}
func (s *Service) knowledgeDetailTx(ctx context.Context, auth *authorizedCommand, project, id string) (ProjectKnowledgeDetail, error) {
	if err := assertProjectRead(auth, project); err != nil {
		return ProjectKnowledgeDetail{}, err
	}
	out, err := scanKnowledge(auth.tx.QueryRowContext(ctx, s.q(knowledgeSelect+` WHERE a.tenant_id=? AND a.project_id=? AND a.id=?`), auth.target.tenantID, project, id))
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	out.CatalogAdoption, err = s.skillCatalogAdoptionTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	out.ModelAdoption, err = s.knowledgeIntelligenceAdoptionTx(ctx, auth.tx, auth.target.tenantID, project, id)
	return out, err
}

// Visible source text excludes metadata and configuration. It includes the same
// description/document body and acceptance criteria that project members see.
func knowledgeSourceText(kind string, r Record) string {
	parts := []string{}
	if r.Description != "" {
		parts = append(parts, r.Description)
	}
	if content, ok := r.Data["content"].(string); kind == "documents" && ok && content != "" {
		parts = append(parts, content)
	}
	if (kind == "requirements" || kind == "tasks") && len(r.AcceptanceCriteria) > 0 {
		parts = append(parts, strings.Join(r.AcceptanceCriteria, "\n"))
	}
	return strings.Join(parts, "\n")
}
func knowledgeSourceKey(kind, id string) string { return kind + ":" + id }

// Batch the current source facts by their fixed allowlisted table. A page may
// reference the same document many times; this avoids per-item source reads.
func (s *Service) knowledgeSourceRecordsTx(ctx context.Context, auth *authorizedCommand, project string, refs []ProjectKnowledgeSource) (map[string]Record, error) {
	if err := assertProjectRead(auth, project); err != nil {
		return nil, err
	}
	groups := map[string]map[string]bool{}
	for _, ref := range refs {
		if !knowledgeSourceKind(ref.Kind) {
			return nil, invalid("知识来源类型无效")
		}
		if groups[ref.Kind] == nil {
			groups[ref.Kind] = map[string]bool{}
		}
		groups[ref.Kind][ref.ID] = true
	}
	records := map[string]Record{}
	for _, kind := range []string{"documents", "requirements", "tasks", "issues"} {
		ids := []string{}
		for id := range groups[kind] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for offset := 0; offset < len(ids); offset += 200 {
			last := offset + 200
			if last > len(ids) {
				last = len(ids)
			}
			chunk := ids[offset:last]
			args := []any{auth.target.tenantID, project}
			for _, id := range chunk {
				args = append(args, id)
			}
			rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_`+kind+` WHERE tenant_id=? AND project_id=? AND id IN (`+strings.TrimRight(strings.Repeat("?,", len(chunk)), ",")+`)`), args...)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				r, err := scanRecord(rows)
				if err != nil {
					rows.Close()
					return nil, err
				}
				records[knowledgeSourceKey(kind, r.ID)] = r
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, err
			}
		}
	}
	return records, nil
}
func knowledgeSourceCurrent(ref ProjectKnowledgeSource, records map[string]Record) bool {
	r, ok := records[knowledgeSourceKey(ref.Kind, ref.ID)]
	if !ok || r.Status == "archived" || r.Status == "deleted" || r.Version != ref.Version {
		return false
	}
	text := knowledgeSourceText(ref.Kind, r)
	return digest(text) == ref.SHA256 && ref.Quote != "" && strings.Contains(text, ref.Quote)
}
func (s *Service) knowledgeEligibilityTx(ctx context.Context, auth *authorizedCommand, project string, items []ProjectKnowledgeDetail) error {
	refs := []ProjectKnowledgeSource{}
	for _, item := range items {
		if item.Published != nil {
			refs = append(refs, item.Published.Sources...)
		}
	}
	records, err := s.knowledgeSourceRecordsTx(ctx, auth, project, refs)
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].Published == nil {
			continue
		}
		for _, ref := range items[i].Published.Sources {
			if !knowledgeSourceCurrent(ref, records) {
				items[i].Eligibility = "source_changed"
				items[i].PublishedEligible = false
				break
			}
		}
	}
	return nil
}
func (s *Service) resolveKnowledgeSourcesTx(ctx context.Context, auth *authorizedCommand, project string, input []ProjectKnowledgeSourceInput) ([]ProjectKnowledgeSource, error) {
	if len(input) > 8 {
		return nil, invalid("知识来源最多为8项")
	}
	refs := []ProjectKnowledgeSource{}
	seen := map[string]bool{}
	for _, in := range input {
		key := knowledgeSourceKey(in.Kind, in.ID)
		if !knowledgeSourceKind(in.Kind) || in.ID == "" || len(in.ID) > 64 || in.Version < 1 || in.Version > 2147483646 || !knowledgeText(in.Quote, 16000) || seen[key] {
			return nil, invalid("知识来源需要唯一的同项目类型、标识及版本")
		}
		seen[key] = true
		refs = append(refs, ProjectKnowledgeSource{Kind: in.Kind, ID: in.ID, Version: in.Version, Quote: in.Quote})
	}
	records, err := s.knowledgeSourceRecordsTx(ctx, auth, project, refs)
	if err != nil {
		return nil, err
	}
	for i := range refs {
		r, ok := records[knowledgeSourceKey(refs[i].Kind, refs[i].ID)]
		if !ok || r.Status == "archived" || r.Status == "deleted" || r.Version != refs[i].Version {
			return nil, conflict("知识来源已变化或不可访问，请重新选择")
		}
		body := knowledgeSourceText(refs[i].Kind, r)
		if refs[i].Quote == "" {
			refs[i].Quote = body
		}
		if refs[i].Quote == "" || !knowledgeText(refs[i].Quote, 16000) || !strings.Contains(body, refs[i].Quote) {
			return nil, invalid("引用必须属于来源正文且不超过16000字节，不会自动截断")
		}
		refs[i].Title = r.Title
		refs[i].SHA256 = digest(body)
	}
	return refs, nil
}

// Only the current approved revision may enter a new execution context. The
// complete selected snapshot is returned as untrusted project data, never a
// credential, command grant, model system instruction or plugin registration.
func (s *Service) resolvePublishedKnowledgeTx(ctx context.Context, auth *authorizedCommand, project string, refs []ProjectKnowledgeReference) ([]ProjectKnowledgeContextItem, error) {
	if err := assertProjectRead(auth, project); err != nil {
		return nil, err
	}
	if len(refs) > 16 {
		return nil, invalid("单次上下文最多选择16项知识")
	}
	out := []ProjectKnowledgeContextItem{}
	details := []ProjectKnowledgeDetail{}
	seen := map[string]bool{}
	for _, ref := range refs {
		if ref.ID == "" || len(ref.ID) > 64 || ref.Revision < 1 || len(ref.SHA256) != 64 || seen[ref.ID] {
			return nil, invalid("知识引用需要唯一标识、修订及摘要")
		}
		seen[ref.ID] = true
		detail, err := s.knowledgeDetailTx(ctx, auth, project, ref.ID)
		if err != nil {
			return nil, err
		}
		if detail.State == "retired" || detail.Published == nil || detail.PublishedRevision != ref.Revision || detail.Published.SHA256 != ref.SHA256 {
			return nil, conflict("知识发布依据已变化，请重新核对上下文")
		}
		details = append(details, detail)
	}
	if err := s.knowledgeEligibilityTx(ctx, auth, project, details); err != nil {
		return nil, err
	}
	for _, detail := range details {
		if !detail.PublishedEligible {
			return nil, conflict("知识来源已变化或不可访问，请重新选择")
		}
		r := detail.Published
		out = append(out, ProjectKnowledgeContextItem{ID: detail.ID, Revision: r.Revision, Kind: r.Kind, Title: r.Title, Summary: r.Summary, Content: r.Content, SHA256: r.SHA256, Sources: knowledgeContextSources(r.Sources), CatalogAdoption: detail.CatalogAdoption, ModelAdoption: detail.ModelAdoption})
	}
	return out, nil
}

type knowledgeCursor struct{ Tenant, Project, Filter, Created, ID string }

func knowledgeFilterHash(q ProjectKnowledgeListQuery) string {
	return digest(stringify([]string{q.Kind, q.State, q.Query}))
}
func (s *Service) ListProjectKnowledge(ctx context.Context, actor Actor, project string, q ProjectKnowledgeListQuery) (out ProjectKnowledgeList, err error) {
	out.Items = []ProjectKnowledgeSummary{}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		var e error
		out, e = s.listProjectKnowledgeTx(auth.ctx, auth, project, q)
		return e
	})
	return
}
func (s *Service) listProjectKnowledgeTx(ctx context.Context, auth *authorizedCommand, project string, q ProjectKnowledgeListQuery) (ProjectKnowledgeList, error) {
	out := ProjectKnowledgeList{Items: []ProjectKnowledgeSummary{}}
	if err := assertProjectRead(auth, project); err != nil {
		return out, err
	}
	if q.Limit == 0 {
		q.Limit = 50
	}
	if q.Limit < 1 || q.Limit > 100 || q.Kind != "" && !knowledgeKind(q.Kind) || q.State != "" && !knowledgeState(q.State) || !knowledgeText(q.Query, 200) {
		return out, invalid("知识筛选或分页参数无效")
	}
	var cursor knowledgeCursor
	if q.Cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(q.Cursor)
		if len(q.Cursor) > 4096 || e != nil || json.Unmarshal(raw, &cursor) != nil || cursor.Tenant != auth.target.tenantID || cursor.Project != project || cursor.Filter != knowledgeFilterHash(q) || cursor.Created == "" || cursor.ID == "" {
			return out, invalid("知识分页游标无效或不属于当前筛选")
		}
	}
	where := ` WHERE a.tenant_id=? AND a.project_id=?`
	args := []any{auth.target.tenantID, project}
	if q.Kind != "" {
		where += ` AND a.kind=?`
		args = append(args, q.Kind)
	}
	if q.State != "" {
		where += ` AND a.state=?`
		args = append(args, q.State)
	}
	if q.Query != "" {
		escape := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
		pattern := "%" + escape.Replace(strings.ToLower(q.Query)) + "%"
		where += ` AND (LOWER(a.title) LIKE ? ESCAPE '!' OR LOWER(a.summary) LIKE ? ESCAPE '!')`
		args = append(args, pattern, pattern)
	}
	if err := auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_project_knowledge a`+where), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	if q.Cursor != "" {
		where += ` AND (a.created_at<? OR (a.created_at=? AND a.id>?))`
		args = append(args, cursor.Created, cursor.Created, cursor.ID)
	}
	args = append(args, q.Limit+1)
	rows, e := auth.tx.QueryContext(ctx, s.q(knowledgeSelect+where+` ORDER BY a.created_at DESC,a.id ASC LIMIT ?`), args...)
	if e != nil {
		return out, e
	}
	matches := []ProjectKnowledgeDetail{}
	for rows.Next() {
		item, err := scanKnowledge(rows)
		if err != nil {
			rows.Close()
			return out, err
		}
		matches = append(matches, item)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(matches) > q.Limit {
		matches = matches[:q.Limit]
		last := matches[len(matches)-1]
		raw, _ := json.Marshal(knowledgeCursor{auth.target.tenantID, project, knowledgeFilterHash(q), last.CreatedAt, last.ID})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	if e = s.knowledgeEligibilityTx(ctx, auth, project, matches); e != nil {
		return out, e
	}
	for _, item := range matches {
		out.Items = append(out.Items, item.ProjectKnowledgeSummary)
	}
	return out, nil
}
func (s *Service) GetProjectKnowledge(ctx context.Context, actor Actor, project, id string) (out ProjectKnowledgeDetail, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeProjectRead, func(auth *authorizedCommand) error {
		detail, e := s.knowledgeDetailTx(auth.ctx, auth, project, id)
		if e != nil {
			return e
		}
		items := []ProjectKnowledgeDetail{detail}
		if e = s.knowledgeEligibilityTx(auth.ctx, auth, project, items); e != nil {
			return e
		}
		out = items[0]
		return nil
	})
	return
}

type knowledgeRevisionCursor struct {
	Tenant, Project, ID string
	Revision            int
}

func (s *Service) ProjectKnowledgeRevisions(ctx context.Context, actor Actor, project, id string, limit int, cursor string) (out ProjectKnowledgeRevisionList, err error) {
	out.Items = []ProjectKnowledgeRevision{}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeProjectRead, func(auth *authorizedCommand) error {
		if limit == 0 {
			limit = 20
		}
		if limit < 1 || limit > 100 {
			return invalid("知识历史分页范围应为1至100")
		}
		// The aggregate must exist in this exact authorized project before either
		// the history count or immutable body is read.
		if _, e := s.knowledgeDetailTx(auth.ctx, auth, project, id); e != nil {
			return e
		}
		var position knowledgeRevisionCursor
		if cursor != "" {
			raw, e := base64.RawURLEncoding.DecodeString(cursor)
			if len(cursor) > 4096 || e != nil || json.Unmarshal(raw, &position) != nil || position.Tenant != auth.target.tenantID || position.Project != project || position.ID != id || position.Revision < 1 {
				return invalid("知识历史游标无效或不属于当前知识")
			}
		}
		args := []any{auth.target.tenantID, project, id}
		if e := auth.tx.QueryRowContext(auth.ctx, s.q(`SELECT COUNT(*) FROM suma_project_knowledge_revisions WHERE tenant_id=? AND project_id=? AND knowledge_id=?`), args...).Scan(&out.Total); e != nil {
			return e
		}
		query := `SELECT revision,snapshot FROM suma_project_knowledge_revisions WHERE tenant_id=? AND project_id=? AND knowledge_id=?`
		if cursor != "" {
			query += ` AND revision<?`
			args = append(args, position.Revision)
		}
		args = append(args, limit+1)
		query += ` ORDER BY revision DESC LIMIT ?`
		rows, e := auth.tx.QueryContext(auth.ctx, s.q(query), args...)
		if e != nil {
			return e
		}
		items := []ProjectKnowledgeRevision{}
		for rows.Next() {
			var number int
			var raw string
			if e = rows.Scan(&number, &raw); e != nil {
				rows.Close()
				return e
			}
			var revision ProjectKnowledgeRevision
			if e = json.Unmarshal([]byte(raw), &revision); e != nil {
				rows.Close()
				return e
			}
			if revision.Revision != number || revision.SHA256 != knowledgeRevisionHash(revision) {
				rows.Close()
				return fmt.Errorf("project knowledge history is inconsistent")
			}
			items = append(items, revision)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(items) > limit {
			items = items[:limit]
			last := items[len(items)-1]
			raw, _ := json.Marshal(knowledgeRevisionCursor{auth.target.tenantID, project, id, last.Revision})
			out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
		}
		out.Items = items
		return nil
	})
	return
}

// Source quotes are immutable review evidence, not repeated model context.
func knowledgeContextSources(sources []ProjectKnowledgeSource) []ProjectKnowledgeContextSource {
	out := []ProjectKnowledgeContextSource{}
	for _, source := range sources {
		out = append(out, ProjectKnowledgeContextSource{Kind: source.Kind, ID: source.ID, Version: source.Version, Title: source.Title, SHA256: source.SHA256})
	}
	return out
}

// Methods describe provenance only; imported text never grants executable authority.
func knowledgeMethod(method string) bool {
	return method == "manual" || method == "extractive" || method == "catalog_adoption" || method == "model_assisted"
}
