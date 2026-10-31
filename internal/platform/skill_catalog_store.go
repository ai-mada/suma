package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ai-mada/suma/internal/store"
)

const (
	skillInvalid = "Invalid shared skill catalog request"
	skillVersion = "The shared skill has changed; reload its reviewed version"
	skillSource  = "The source skill is no longer the current eligible publication"
	skillShare   = "Confirm that this reviewed skill text will be shared with the organization"
	skillAdopt   = "Confirm adoption as a project draft requiring a separate publication review"
	skillRetired = "This shared skill is retired and cannot be adopted"
	skillOrigin  = "A shared skill cannot change its original project or knowledge identity"
)

func (s *Service) InitializeSkillCatalog(ctx context.Context) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS suma_skill_catalog(tenant_id VARCHAR(64) NOT NULL,id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,title VARCHAR(200) NOT NULL,summary TEXT NOT NULL,domain VARCHAR(80) NOT NULL,scenarios TEXT NOT NULL,tags TEXT NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL,snapshot TEXT NOT NULL,PRIMARY KEY(tenant_id,id))`,
		`CREATE TABLE IF NOT EXISTS suma_skill_catalog_revisions(tenant_id VARCHAR(64) NOT NULL,catalog_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,snapshot TEXT NOT NULL,PRIMARY KEY(tenant_id,catalog_id,version))`,
		`CREATE TABLE IF NOT EXISTS suma_skill_catalog_adoptions(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,knowledge_id VARCHAR(64) NOT NULL,catalog_id VARCHAR(64) NOT NULL,catalog_version INTEGER NOT NULL,created_at VARCHAR(40) NOT NULL,snapshot TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,knowledge_id))`,
	} {
		if _, e := s.DB.ExecContext(ctx, s.ddl(q)); e != nil {
			return e
		}
	}
	for _, i := range []struct {
		name, table string
		columns     []string
	}{
		{"suma_skill_catalog_page", "suma_skill_catalog", []string{"tenant_id", "state", "created_at", "id"}},
		{"suma_skill_catalog_domain", "suma_skill_catalog", []string{"tenant_id", "domain", "state", "created_at", "id"}},
		{"suma_skill_catalog_adoption_origin", "suma_skill_catalog_adoptions", []string{"tenant_id", "catalog_id", "catalog_version", "created_at"}},
	} {
		if e := store.EnsureIndex(ctx, s.DB, s.Dialect, i.name, i.table, i.columns...); e != nil {
			return e
		}
	}
	return nil
}
func skillText(value string, max int, required bool) bool {
	return knowledgeText(value, max) && (!required || strings.TrimSpace(value) != "") && value == strings.TrimSpace(value)
}
func skillMetadata(domain string, scenarios, tags []string) bool {
	if !skillText(domain, 80, true) || len(scenarios) < 1 || len(scenarios) > 8 || len(tags) > 16 || scenarios == nil || tags == nil {
		return false
	}
	for _, group := range []struct {
		items []string
		max   int
	}{{scenarios, 80}, {tags, 40}} {
		seen := map[string]bool{}
		for _, item := range group.items {
			if !skillText(item, group.max, true) || seen[item] {
				return false
			}
			seen[item] = true
		}
	}
	return true
}
func (s *Service) skillCatalogStoredTx(ctx context.Context, auth *authorizedCommand, id string) (out SkillCatalogSummary, err error) {
	if auth == nil || auth.assertTx(auth.tx, auth.target, auth.purpose) != nil {
		return out, forbidden()
	}
	var raw string
	var version int
	var state string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT version,state,snapshot FROM suma_skill_catalog WHERE tenant_id=? AND id=?`), auth.target.tenantID, id).Scan(&version, &state, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	if out.ID != id || out.Version != version || out.State != state {
		return out, conflict(skillVersion)
	}
	return
}
func (s *Service) skillOriginRevisionTx(ctx context.Context, tx *sql.Tx, tenant string, origin SkillCatalogOrigin) (out ProjectKnowledgeRevision, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT snapshot FROM suma_project_knowledge_revisions WHERE tenant_id=? AND project_id=? AND knowledge_id=? AND revision=?`), tenant, origin.ProjectID, origin.KnowledgeID, origin.Revision).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, conflict(skillSource)
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	if out.Revision != origin.Revision || out.Kind != "skill" || out.State != "published" || out.SHA256 != origin.SHA256 || knowledgeRevisionHash(out) != origin.SHA256 {
		return out, conflict(skillSource)
	}
	return
}

// Catalog consent authorizes only shared text and this validity check. It does
// not create source-project membership or return source records/quotes.
func (s *Service) skillOriginCurrentTx(ctx context.Context, tx *sql.Tx, tenant string, origin SkillCatalogOrigin) (bool, error) {
	var state, projectState string
	var revision int
	err := tx.QueryRowContext(ctx, s.q(`SELECT k.state,k.published_revision,p.status FROM suma_project_knowledge k JOIN suma_projects p ON p.id=k.project_id AND p.tenant_id=k.tenant_id WHERE k.tenant_id=? AND k.project_id=? AND k.id=?`), tenant, origin.ProjectID, origin.KnowledgeID).Scan(&state, &revision, &projectState)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if state == "retired" || revision != origin.Revision || projectState == "archived" || projectState == "deleted" {
		return false, nil
	}
	published, err := s.skillOriginRevisionTx(ctx, tx, tenant, origin)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return false, nil
		}
		return false, err
	}
	states, err := s.knowledgeSourcesCurrentTx(ctx, tx, tenant, origin.ProjectID, published.Sources, nil, false)
	if err != nil {
		return false, err
	}
	for _, current := range states {
		if !current {
			return false, nil
		}
	}
	return true, nil
}
func (s *Service) skillCatalogCapabilitiesTx(ctx context.Context, auth *authorizedCommand, project string) (manage, adopt bool, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return
	}
	if auth.actor.kind != actorBrowser && auth.actor.kind != actorProjectAPI {
		return false, false, forbidden()
	}
	manage = auth.actor.kind == actorBrowser && orgAdmin(auth.currentPrincipal)
	if auth.actor.kind != actorBrowser {
		return manage, false, nil
	}
	_, err = s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, project, true, true)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return manage, false, nil
		}
		return false, false, err
	}
	return manage, true, nil
}
func (s *Service) skillCatalogProjectionTx(ctx context.Context, auth *authorizedCommand, project string, out SkillCatalogSummary) (SkillCatalogSummary, error) {
	manage, adopt, e := s.skillCatalogCapabilitiesTx(ctx, auth, project)
	if e != nil {
		return out, e
	}
	out.CanManage = manage
	out.CanAdopt = false
	out.AllowedActions = []string{}
	out.SourceCurrent, e = s.skillOriginCurrentTx(ctx, auth.tx, auth.target.tenantID, out.Origin)
	if e != nil {
		return out, e
	}
	out.Eligibility = "current"
	if !out.SourceCurrent {
		out.Eligibility = "source_changed"
	}
	if out.State == "retired" {
		out.Eligibility = "retired"
	}
	var headVersion int
	var headState string
	e = auth.tx.QueryRowContext(ctx, s.q(`SELECT version,state FROM suma_skill_catalog WHERE tenant_id=? AND id=?`), auth.target.tenantID, out.ID).Scan(&headVersion, &headState)
	if errors.Is(e, sql.ErrNoRows) {
		return out, missing()
	}
	if e != nil {
		return out, e
	}
	if headVersion != out.Version || headState != out.State {
		out.Eligibility = "publication_changed"
		if headState == "retired" {
			out.Eligibility = "retired"
		}
		return out, nil
	}
	if manage && out.State != "retired" {
		refresh := out.SourceCurrent
		if !refresh {
			var version, revision int
			var raw string
			e = auth.tx.QueryRowContext(ctx, s.q(`SELECT k.version,k.published_revision,r.snapshot FROM suma_project_knowledge k JOIN suma_project_knowledge_revisions r ON r.tenant_id=k.tenant_id AND r.project_id=k.project_id AND r.knowledge_id=k.id AND r.revision=k.published_revision WHERE k.tenant_id=? AND k.project_id=? AND k.id=? AND k.state<>'retired'`), auth.target.tenantID, out.Origin.ProjectID, out.Origin.KnowledgeID).Scan(&version, &revision, &raw)
			if e != nil && !errors.Is(e, sql.ErrNoRows) {
				return out, e
			}
			if e == nil {
				var published ProjectKnowledgeRevision
				if e = json.Unmarshal([]byte(raw), &published); e != nil {
					return out, e
				}
				refresh, e = s.skillOriginCurrentTx(ctx, auth.tx, auth.target.tenantID, SkillCatalogOrigin{ProjectID: out.Origin.ProjectID, KnowledgeID: out.Origin.KnowledgeID, KnowledgeVersion: version, Revision: revision, SHA256: published.SHA256})
				if e != nil {
					return out, e
				}
			}
		}
		if refresh {
			out.AllowedActions = append(out.AllowedActions, "update")
		}
		if out.State != "retired" {
			out.AllowedActions = append(out.AllowedActions, "retire")
		}
	}
	if adopt && out.State == "published" && out.SourceCurrent {
		out.CanAdopt = true
		out.AllowedActions = append(out.AllowedActions, "adopt")
	}
	return out, nil
}
func (s *Service) skillCatalogDetailTx(ctx context.Context, auth *authorizedCommand, project, id string) (out SkillCatalogDetail, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return out, err
	}
	stored, err := s.skillCatalogStoredTx(ctx, auth, id)
	if err != nil {
		return out, err
	}
	out.SkillCatalogSummary, err = s.skillCatalogProjectionTx(ctx, auth, project, stored)
	if err != nil {
		return out, err
	}
	revision, err := s.skillOriginRevisionTx(ctx, auth.tx, auth.target.tenantID, stored.Origin)
	if err != nil {
		return out, err
	}
	out.Content = revision.Content
	return
}
func checkSkillCatalogReference(detail SkillCatalogDetail, version int, sha string) error {
	if version != detail.Version || sha != detail.Origin.SHA256 {
		return conflict(skillVersion)
	}
	return nil
}
func (s *Service) skillCatalogAdoptionTx(ctx context.Context, tx *sql.Tx, tenant, project, knowledge string) (out *SkillCatalogAdoption, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT snapshot FROM suma_skill_catalog_adoptions WHERE tenant_id=? AND project_id=? AND knowledge_id=?`), tenant, project, knowledge).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out = &SkillCatalogAdoption{}
	err = json.Unmarshal([]byte(raw), out)
	return
}
