package platform

import (
	"context"
	"encoding/json"
	"strings"
)

func (s *Service) skillCatalogAuthority(ctx context.Context, actor Actor, project, id string, purpose commandPurpose) (context.Context, *authorizedCommand, func(), error) {
	ctx, tx, cancel, e := s.beginAuthorityTx(ctx)
	if e != nil {
		return ctx, nil, func() {}, e
	}
	close := func() { tx.Rollback(); cancel() }
	auth, e := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purpose)
	if e != nil {
		close()
		return ctx, nil, func() {}, e
	}
	return ctx, auth, close, nil
}
func validateSkillCatalogWrite(r SkillCatalogWriteRequest) error {
	if !handoffID(r.ProjectID, false) || !handoffID(r.KnowledgeID, false) || r.ExpectedVersion < 0 || r.ExpectedVersion >= 2147483646 || r.ExpectedKnowledgeVersion < 1 || r.ExpectedKnowledgeVersion >= 2147483646 || r.PublishedRevision < 1 || r.PublishedRevision >= 2147483646 || len(r.PublishedSHA256) != 64 || !skillMetadata(r.Domain, r.Scenarios, r.Tags) || !planningText(r.Reason, 2000) || !planningKey(r.IdempotencyKey) {
		return invalid(skillInvalid)
	}
	if !r.AcknowledgeSharing {
		return invalid(skillShare)
	}
	return nil
}
func (s *Service) skillCatalogWrite(ctx context.Context, actor Actor, id string, r SkillCatalogWriteRequest) (out SkillCatalogDetail, err error) {
	if err = validateSkillCatalogWrite(r); err != nil {
		return out, err
	}
	if id == "" && r.ExpectedVersion != 0 || id != "" && (r.ExpectedVersion < 1 || !handoffID(id, false)) {
		return out, invalid(skillInvalid)
	}
	ctx, auth, close, err := s.skillCatalogAuthority(ctx, actor, r.ProjectID, r.KnowledgeID, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	defer close()
	if !orgAdmin(auth.currentPrincipal) {
		return out, forbidden()
	}
	hash, replay, err := s.commandReceipt(ctx, auth, r.IdempotencyKey, map[string]any{"skill_catalog_write": id, "request": r})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		out.SkillCatalogSummary, err = s.skillCatalogProjectionTx(ctx, auth, r.ProjectID, out.SkillCatalogSummary)
		return out, err
	}
	var old SkillCatalogSummary
	if id != "" {
		old, err = s.skillCatalogStoredTx(ctx, auth, id)
		if err != nil {
			return out, err
		}
		if old.Version != r.ExpectedVersion {
			return out, conflict(skillVersion)
		}
		if old.State == "retired" {
			return out, conflict(skillRetired)
		}
		if old.Origin.ProjectID != r.ProjectID || old.Origin.KnowledgeID != r.KnowledgeID {
			return out, invalid(skillOrigin)
		}
	}
	source, err := s.currentKnowledgeDetailTx(ctx, auth, r.ProjectID, r.KnowledgeID)
	if err != nil {
		return out, err
	}
	if source.Kind != "skill" || source.Version != r.ExpectedKnowledgeVersion || source.Published == nil || !source.PublishedEligible || source.State == "retired" || source.PublishedRevision != r.PublishedRevision || source.PublishedSHA256 != r.PublishedSHA256 {
		return out, conflict(skillSource)
	}
	origin := SkillCatalogOrigin{ProjectID: r.ProjectID, KnowledgeID: r.KnowledgeID, KnowledgeVersion: source.Version, Revision: source.PublishedRevision, SHA256: source.PublishedSHA256}
	current, err := s.skillOriginCurrentTx(ctx, auth.tx, auth.target.tenantID, origin)
	if err != nil {
		return out, err
	}
	if !current {
		return out, conflict(skillSource)
	}
	if id == "" {
		id = newID()
	}
	stamp := now()
	entry := SkillCatalogSummary{ID: id, Version: r.ExpectedVersion + 1, State: "published", Title: source.Published.Title, Summary: source.Published.Summary, Domain: r.Domain, Scenarios: append([]string{}, r.Scenarios...), Tags: append([]string{}, r.Tags...), Origin: origin, CreatedBy: auth.currentPrincipal.ID, CreatedAt: stamp, ReviewedBy: auth.currentPrincipal.ID, UpdatedAt: stamp, Reason: r.Reason, AllowedActions: []string{}}
	if old.ID != "" {
		entry.CreatedBy = old.CreatedBy
		entry.CreatedAt = old.CreatedAt
	}
	if err = s.persistSkillCatalogTx(ctx, auth, entry, r.ExpectedVersion); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "skill.catalog_shared", id, r.Reason); err != nil {
		return out, err
	}
	out, err = s.skillCatalogDetailTx(ctx, auth, r.ProjectID, id)
	if err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, r.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}
func (s *Service) persistSkillCatalogTx(ctx context.Context, auth *authorizedCommand, entry SkillCatalogSummary, expected int) error {
	tenant := auth.target.tenantID
	if _, e := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_skill_catalog_revisions(tenant_id,catalog_id,version,snapshot)VALUES(?,?,?,?)`), tenant, entry.ID, entry.Version, stringify(entry)); e != nil {
		return e
	}
	if expected == 0 {
		_, e := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_skill_catalog(tenant_id,id,version,state,title,summary,domain,scenarios,tags,created_at,updated_at,snapshot)VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`), tenant, entry.ID, entry.Version, entry.State, entry.Title, entry.Summary, entry.Domain, stringify(entry.Scenarios), stringify(entry.Tags), entry.CreatedAt, entry.UpdatedAt, stringify(entry))
		return e
	}
	result, e := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_skill_catalog SET version=?,state=?,title=?,summary=?,domain=?,scenarios=?,tags=?,updated_at=?,snapshot=? WHERE tenant_id=? AND id=? AND version=?`), entry.Version, entry.State, entry.Title, entry.Summary, entry.Domain, stringify(entry.Scenarios), stringify(entry.Tags), entry.UpdatedAt, stringify(entry), tenant, entry.ID, expected)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return conflict(skillVersion)
	}
	return nil
}
func (s *Service) CreateSkillCatalog(ctx context.Context, actor Actor, r SkillCatalogWriteRequest) (SkillCatalogDetail, error) {
	return s.skillCatalogWrite(ctx, actor, "", r)
}
func (s *Service) UpdateSkillCatalog(ctx context.Context, actor Actor, id string, r SkillCatalogWriteRequest) (SkillCatalogDetail, error) {
	return s.skillCatalogWrite(ctx, actor, id, r)
}
func (s *Service) GetSkillCatalog(ctx context.Context, actor Actor, project, id string) (out SkillCatalogDetail, err error) {
	if !handoffID(project, false) || !handoffID(id, false) {
		return out, invalid(skillInvalid)
	}
	ctx, auth, close, err := s.skillCatalogAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	return s.skillCatalogDetailTx(ctx, auth, project, id)
}
func (s *Service) CommandSkillCatalog(ctx context.Context, actor Actor, id string, r SkillCatalogCommandRequest) (out SkillCatalogDetail, err error) {
	if !handoffID(r.ProjectID, false) || !handoffID(id, false) || r.ExpectedVersion < 1 || r.ExpectedVersion >= 2147483646 || r.Action != "retire" || !planningText(r.Reason, 2000) || !planningKey(r.IdempotencyKey) {
		return out, invalid(skillInvalid)
	}
	ctx, auth, close, err := s.skillCatalogAuthority(ctx, actor, r.ProjectID, id, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	defer close()
	if !orgAdmin(auth.currentPrincipal) {
		return out, forbidden()
	}
	hash, replay, err := s.commandReceipt(ctx, auth, r.IdempotencyKey, map[string]any{"skill_catalog_command": id, "request": r})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		out.SkillCatalogSummary, err = s.skillCatalogProjectionTx(ctx, auth, r.ProjectID, out.SkillCatalogSummary)
		return out, err
	}
	entry, err := s.skillCatalogStoredTx(ctx, auth, id)
	if err != nil {
		return out, err
	}
	if entry.Version != r.ExpectedVersion || entry.State == "retired" {
		return out, conflict(skillVersion)
	}
	entry.Version++
	entry.State = "retired"
	entry.ReviewedBy = auth.currentPrincipal.ID
	entry.UpdatedAt = now()
	entry.Reason = r.Reason
	entry.AllowedActions = []string{}
	if err = s.persistSkillCatalogTx(ctx, auth, entry, r.ExpectedVersion); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "skill.catalog_retired", id, r.Reason); err != nil {
		return out, err
	}
	out, err = s.skillCatalogDetailTx(ctx, auth, r.ProjectID, id)
	if err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, r.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}
func (s *Service) AdoptSkillCatalog(ctx context.Context, actor Actor, id string, r SkillCatalogAdoptRequest) (out ProjectKnowledgeDetail, err error) {
	if !handoffID(r.ProjectID, false) || !handoffID(id, false) || r.ExpectedVersion < 1 || len(r.OriginSHA256) != 64 || !planningText(r.Reason, 2000) || !planningKey(r.IdempotencyKey) {
		return out, invalid(skillInvalid)
	}
	if !r.AcknowledgeDraft {
		return out, invalid(skillAdopt)
	}
	ctx, auth, close, err := s.skillCatalogAuthority(ctx, actor, r.ProjectID, id, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, r.IdempotencyKey, map[string]any{"skill_catalog_adopt": id, "request": r})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	catalog, err := s.skillCatalogDetailTx(ctx, auth, r.ProjectID, id)
	if err != nil {
		return out, err
	}
	if err = checkSkillCatalogReference(catalog, r.ExpectedVersion, r.OriginSHA256); err != nil {
		return out, err
	}
	if catalog.State == "retired" {
		return out, conflict(skillRetired)
	}
	if !catalog.SourceCurrent {
		return out, conflict(skillSource)
	}
	draft := ProjectKnowledgeWriteRequest{ProjectID: r.ProjectID, Kind: "skill", Title: catalog.Title, Summary: catalog.Summary, Content: catalog.Content, ExpectedVersion: 0, Sources: []ProjectKnowledgeSourceInput{}}
	if err = validateKnowledgeDraft(draft); err != nil {
		return out, err
	}
	out, err = s.knowledgeCreateTx(ctx, auth, r.ProjectID, draft, []ProjectKnowledgeSource{}, "catalog_adoption")
	if err != nil {
		return out, err
	}
	lineage := SkillCatalogAdoption{CatalogID: id, CatalogVersion: catalog.Version, Origin: catalog.Origin, ProjectID: r.ProjectID, KnowledgeID: out.ID, Revision: out.Latest.Revision, SHA256: out.Latest.SHA256, AdoptedBy: auth.currentPrincipal.ID, AdoptedAt: now(), Reason: r.Reason}
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_skill_catalog_adoptions(tenant_id,project_id,knowledge_id,catalog_id,catalog_version,created_at,snapshot)VALUES(?,?,?,?,?,?,?)`), auth.target.tenantID, r.ProjectID, out.ID, id, catalog.Version, lineage.AdoptedAt, stringify(lineage)); err != nil {
		return out, err
	}
	// Re-read through the ordinary knowledge projection after the immutable
	// lineage exists, so its optional origin facts use the same standard DTO.
	out, err = s.knowledgeDetailTx(ctx, auth, r.ProjectID, out.ID)
	if err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "skill.catalog_adopted", out.ID, r.Reason); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, r.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}

func skillCatalogLike(value string) string {
	return strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(value)
}
