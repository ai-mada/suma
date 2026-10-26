package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/ai-mada/suma/internal/store"
)

const expertActivationUnavailable = expertAuthority

// This is an explicit nonsecret credential reference, never a serialized Actor
// or a principal promoted into browser authority.
type expertTeamCredentialReference struct {
	Kind           actorKind `json:"kind"`
	Digest         string    `json:"digest"`
	SubjectID      string    `json:"subject_id"`
	TenantID       string    `json:"tenant_id"`
	ProjectID      string    `json:"project_id"`
	SourceID       string    `json:"source_id"`
	SourceRevision int64     `json:"source_revision"`
	FamilyID       string    `json:"family_id"`
	FamilyRevision int64     `json:"family_revision"`
	GrantID        string    `json:"grant_id"`
	SlotID         string    `json:"slot_id"`
}

func expertCredentialSnapshot(a Actor) expertTeamCredentialReference {
	r := a.credential
	return expertTeamCredentialReference{a.kind, r.digest, r.subjectID, r.tenantID, r.projectID, r.sourceID, r.sourceRevision, r.familyID, r.familyRevision, r.grantID, r.slotID}
}
func (r expertTeamCredentialReference) ref() credentialRef {
	return credentialRef{digest: r.Digest, subjectID: r.SubjectID, tenantID: r.TenantID, projectID: r.ProjectID, sourceID: r.SourceID, sourceRevision: r.SourceRevision, familyID: r.FamilyID, familyRevision: r.FamilyRevision, grantID: r.GrantID, slotID: r.SlotID}
}

type expertTeamActivation struct {
	PlanID, TenantID, ProjectID, State, ActivatedBy, CredentialHash, NextCheckAt, LastCheckedAt, LastError string
	Generation, SourceRevision                                                                             int
	Credential                                                                                             expertTeamCredentialReference
}

func (s *Service) MigrateExpertTeamActivations(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, s.ddl(`CREATE TABLE IF NOT EXISTS suma_expert_team_plan_activations(plan_id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,generation INTEGER NOT NULL,source_revision INTEGER NOT NULL,state VARCHAR(20) NOT NULL,activated_by VARCHAR(64) NOT NULL,credential_hash VARCHAR(64) NOT NULL,credential_kind INTEGER NOT NULL,credential_reference TEXT NOT NULL,next_check_at VARCHAR(40) NOT NULL,last_checked_at VARCHAR(40) NOT NULL,last_error TEXT NOT NULL)`))
	if err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_expert_team_activation_due", "suma_expert_team_plan_activations", "state", "next_check_at", "last_checked_at", "plan_id")
}
func (s *Service) expertTeamActivationTx(ctx context.Context, q authorityQuery, tenant, project, id string, current bool) (a expertTeamActivation, err error) {
	query := `SELECT plan_id,tenant_id,project_id,generation,source_revision,state,activated_by,credential_hash,credential_kind,credential_reference,next_check_at,last_checked_at,last_error FROM suma_expert_team_plan_activations WHERE tenant_id=? AND project_id=? AND plan_id=?`
	if current {
		query = s.currentSQL(query)
	}
	var raw string
	var kind int
	err = q.QueryRowContext(ctx, s.q(query), tenant, project, id).Scan(&a.PlanID, &a.TenantID, &a.ProjectID, &a.Generation, &a.SourceRevision, &a.State, &a.ActivatedBy, &a.CredentialHash, &kind, &raw, &a.NextCheckAt, &a.LastCheckedAt, &a.LastError)
	if err != nil {
		return a, err
	}
	if json.Unmarshal([]byte(raw), &a.Credential) != nil || int(a.Credential.Kind) != kind || a.Credential.Digest != a.CredentialHash || a.Credential.SubjectID != a.ActivatedBy || a.Credential.TenantID != tenant || a.Generation < 1 || a.SourceRevision < 1 {
		return a, expertError("expert_team_activation_unavailable", expertActivationUnavailable)
	}
	return a, nil
}
func (s *Service) saveExpertTeamActivationTx(ctx context.Context, auth *authorizedCommand, plan ExpertTeamPlan) error {
	if auth == nil {
		return forbidden()
	}
	if err := assertProjectCommand(auth.tx, auth, plan.ProjectID); err != nil {
		return err
	}
	if auth.actor.kind != actorBrowser && auth.actor.kind != actorProjectAPI || auth.actor.credential.subjectID != auth.currentPrincipal.ID || plan.ActivationGeneration < 1 || plan.Revision < 1 {
		return forbidden()
	}
	clock, err := s.authorityNow(ctx, auth.tx)
	if err != nil {
		return err
	}
	at := automationStamp(clock)
	ref := expertCredentialSnapshot(auth.actor)
	raw := stringify(ref)
	old, err := s.expertTeamActivationTx(ctx, auth.tx, auth.target.tenantID, plan.ProjectID, plan.ID, true)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_expert_team_plan_activations(plan_id,tenant_id,project_id,generation,source_revision,state,activated_by,credential_hash,credential_kind,credential_reference,next_check_at,last_checked_at,last_error)VALUES(?,?,?,?,?,'active',?,?,?,?,?,'','')`), plan.ID, auth.target.tenantID, plan.ProjectID, plan.ActivationGeneration, plan.Revision, ref.SubjectID, ref.Digest, int(ref.Kind), raw, at)
		return err
	}
	if err != nil {
		return err
	}
	if old.Generation > plan.ActivationGeneration || old.Generation == plan.ActivationGeneration && old.Credential != ref {
		return expertError("expert_team_changed", expertPlanChanged)
	}
	result, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_expert_team_plan_activations SET generation=?,source_revision=?,state='active',activated_by=?,credential_hash=?,credential_kind=?,credential_reference=?,next_check_at=?,last_checked_at='',last_error='' WHERE tenant_id=? AND project_id=? AND plan_id=? AND generation=?`), plan.ActivationGeneration, plan.Revision, ref.SubjectID, ref.Digest, int(ref.Kind), raw, at, auth.target.tenantID, plan.ProjectID, plan.ID, old.Generation)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return expertError("expert_team_changed", expertPlanChanged)
	}
	return nil
}
func (s *Service) setExpertTeamActivationStateTx(ctx context.Context, auth *authorizedCommand, plan ExpertTeamPlan, state string) error {
	if auth == nil {
		return forbidden()
	}
	if err := assertProjectCommand(auth.tx, auth, plan.ProjectID); err != nil {
		return err
	}
	if state != "active" && state != "paused" && state != "stopped" {
		return forbidden()
	}
	at, err := s.authorityNow(ctx, auth.tx)
	if err != nil {
		return err
	}
	_, err = auth.tx.ExecContext(ctx, s.q(`UPDATE suma_expert_team_plan_activations SET state=?,generation=?,source_revision=?,next_check_at=?,last_error='' WHERE tenant_id=? AND project_id=? AND plan_id=? AND generation<=?`), state, plan.ActivationGeneration, plan.Revision, automationStamp(at), auth.target.tenantID, plan.ProjectID, plan.ID, plan.ActivationGeneration)
	return err
}
func (s *Service) restoreExpertTeamActor(ctx context.Context, tenant, project, id string) (Actor, expertTeamActivation, error) {
	a, err := s.expertTeamActivationTx(ctx, s.DB, tenant, project, id, false)
	if err != nil {
		return Actor{}, a, err
	}
	if a.State != "active" {
		return Actor{}, a, expertError("expert_team_activation_unavailable", expertActivationUnavailable)
	}
	c, err := s.readCredential(ctx, s.DB, a.CredentialHash, false)
	if err != nil {
		return Actor{}, a, expertError("expert_team_activation_unavailable", expertActivationUnavailable)
	}
	clock, err := s.authorityNow(ctx, s.DB)
	if err != nil {
		return Actor{}, a, err
	}
	if (c.kind != actorBrowser && c.kind != actorProjectAPI) || c.kind != a.Credential.Kind || c.ref != a.Credential.ref() || c.ref.subjectID != a.ActivatedBy || c.ref.tenantID != tenant || c.kind == actorProjectAPI && c.ref.projectID != project || !c.expires.After(clock) {
		return Actor{}, a, expertError("expert_team_activation_unavailable", expertActivationUnavailable)
	}
	p := c.p
	p.ID = c.ref.subjectID
	p.TenantID = c.ref.tenantID
	return Actor{kind: c.kind, credential: c.ref, projection: p}, a, nil
}
func (s *Service) checkExpertTeamActivationTx(ctx context.Context, tx *sql.Tx, tenant, project, id string, generation, revision int, actor Actor) error {
	a, err := s.expertTeamActivationTx(ctx, tx, tenant, project, id, true)
	if err != nil {
		return err
	}
	if a.State != "active" || a.Generation != generation || a.SourceRevision != revision || a.Credential != expertCredentialSnapshot(actor) {
		return expertError("expert_team_activation_unavailable", expertActivationUnavailable)
	}
	return nil
}
func (s *Service) expertTeamScheduleCheck(ctx context.Context, a expertTeamActivation, reason string) error {
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	defer tx.Rollback()
	if err = s.authorityWriter(ctx, tx); err != nil {
		return err
	}
	if err = s.lockOrganization(ctx, tx, a.TenantID); err != nil {
		return err
	}
	clock, err := s.authorityNow(ctx, tx)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, s.q(`UPDATE suma_expert_team_plan_activations SET next_check_at=?,last_checked_at=?,last_error=? WHERE tenant_id=? AND project_id=? AND plan_id=? AND generation=? AND state IN ('active','paused','stopped')`), automationStamp(clock.Add(time.Second)), automationStamp(clock), reason, a.TenantID, a.ProjectID, a.PlanID, a.Generation)
	if err != nil {
		return err
	}
	return tx.Commit()
}
