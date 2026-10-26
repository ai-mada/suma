package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func intelligenceAdmission(message string) error {
	return &APIError{Status: 409, Code: "knowledge_intelligence_basis_changed", Message: message}
}
func (s *Service) checkKnowledgeIntelligenceExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run string, requireCurrentSource bool) error {
	r, found, e := s.knowledgeIntelligenceExecutionRecordTx(ctx, tx, tenant, project, run)
	if e != nil || !found {
		return e
	}
	a := r.Analysis
	if requireCurrentSource && a.State != "running" && a.State != "reconciling" {
		return intelligenceAdmission(intelligenceVersion)
	}
	c, e := s.readCredential(ctx, tx, r.CreatorCredentialHash, true)
	if e != nil {
		var api *APIError
		if errors.As(e, &api) {
			return intelligenceAdmission(intelligenceAuthority)
		}
		return e
	}
	clock, e := s.authorityNow(ctx, tx)
	if e != nil {
		return e
	}
	if c.kind != actorBrowser || c.ref.tenantID != tenant || c.ref.subjectID != a.CreatedBy || !c.expires.After(clock) {
		return intelligenceAdmission(intelligenceAuthority)
	}
	if _, e = s.currentProjectMembershipLockedTx(ctx, tx, tenant, a.CreatedBy, project, true, false); e != nil {
		var api *APIError
		if errors.As(e, &api) {
			return intelligenceAdmission(intelligenceAuthority)
		}
		return e
	}
	if requireCurrentSource {
		if e = s.checkKnowledgeIntelligenceSourceTx(ctx, tx, tenant, project, a.Source); e != nil {
			var api *APIError
			if errors.As(e, &api) {
				return intelligenceAdmission(intelligenceSource)
			}
			return e
		}
	}
	p, e := s.toolProfileTx(ctx, tx, Principal{TenantID: tenant}, project, a.ToolProfileID)
	if e != nil {
		return e
	}
	if !knowledgeIntelligenceProfile(p) || p.Version != a.ToolProfileVersion || p.Profile.ProfileSHA256 != a.ToolProfileSHA256 || p.WorkerID != r.WorkerID {
		return intelligenceAdmission(intelligenceProfile)
	}
	p, e = s.toolProfileObservationLockedTx(ctx, tx, tenant, project, p)
	if e != nil {
		return e
	}
	if !p.Available {
		return intelligenceAdmission(intelligenceProfile)
	}
	route, e := dag.ReadProjectExecutionRouteTx(ctx, tx, s.Dialect, tenant, project)
	if e != nil {
		return e
	}
	bound, e := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, run)
	if e != nil {
		return e
	}
	if route != r.Route || bound != r.Route {
		return intelligenceAdmission(intelligenceSource)
	}
	return s.checkDispatchProfileRouteTx(ctx, tx, tenant, project, p, route)
}
func (s *Service) recordKnowledgeIntelligenceOutcomeTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result, authorize func(*sql.Tx) error) error {
	r, found, e := s.knowledgeIntelligenceExecutionRecordTx(ctx, tx, tenant, project, result.RunID)
	if e != nil || !found {
		return e
	}
	if authorize == nil {
		return forbidden()
	}
	if e = authorize(tx); e != nil {
		return e
	}
	a := r.Analysis
	var attempt int
	var fence int64
	var worker, raw string
	e = tx.QueryRowContext(ctx, s.q(`SELECT n.attempt,n.fence,n.worker_id,n.spec FROM dag_nodes n JOIN dag_runs d ON d.id=n.run_id WHERE d.tenant_id=? AND d.project_id=? AND n.run_id=? AND n.id=?`), tenant, project, result.RunID, result.NodeID).Scan(&attempt, &fence, &worker, &raw)
	if e != nil {
		return e
	}
	if result.NodeID != "extract" || result.Attempt != attempt || result.Fence != fence || result.WorkerID != worker || worker != r.WorkerID {
		return forbidden()
	}
	var node dag.Node
	if json.Unmarshal([]byte(raw), &node) != nil || node.Config["purpose"] != "knowledge_extraction" || node.Config["tool_profile_id"] != a.ToolProfileID || node.Config["tool_profile_sha256"] != a.ToolProfileSHA256 {
		return forbidden()
	}
	sha := digest(stringify(result))
	var count int
	e = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_knowledge_intelligence_outcomes WHERE analysis_id=? AND attempt=? AND fence=? AND result_sha256=?`), a.ID, attempt, fence, sha).Scan(&count)
	if e != nil {
		return e
	}
	if count > 0 {
		return nil
	}
	if a.State != "running" && a.State != "reconciling" {
		return conflict(intelligenceVersion)
	}
	old := a.Version
	a.Version++
	a.UpdatedAt = now()
	a.AllowedActions = []string{}
	// Unknown observations remain immutable outcome facts, but are not a
	// completed model original. Freeze the analysis original on the first known
	// result so an empty/partial observation cannot replace its final JSON.
	if !result.OutcomeUnknown && a.OriginalOutputSHA256 == "" {
		a.OriginalOutputSHA256 = digest(result.Output)
		if len(result.Output) <= 64<<10 && utf8.ValidString(result.Output) {
			a.OriginalOutput = result.Output
		}
	}
	a.BasisCurrent = true
	if e = s.checkKnowledgeIntelligenceSourceTx(ctx, tx, tenant, project, a.Source); e != nil {
		var api *APIError
		if !errors.As(e, &api) {
			return e
		}
		a.BasisCurrent = false
	}
	switch {
	case result.OutcomeUnknown:
		a.State = "reconciling"
		a.Reason = intelligenceUnknown
	case result.Stopped || result.ExitCode != 0 || result.Error != "":
		a.State = "failed"
		a.Reason = intelligenceFailed
	default:
		proof := result.ToolEvidence
		if proof == nil || proof.Terminal != "completed" || proof.Profile.ProfileSHA256 != a.ToolProfileSHA256 || proof.Profile.Adapter != "suma" || !proof.Profile.ReadOnly || proof.Profile.KnowledgeExtraction == nil || proof.Profile.KnowledgeExtraction.Protocol != toolcontract.KnowledgeExtractionProtocol {
			a.State = "failed"
			a.Reason = intelligenceFailed
			break
		}
		proposal, err := parseKnowledgeIntelligenceOutput(result.Output, a.Source)
		if err != nil {
			a.State = "failed"
			a.Reason = intelligenceOutput
			break
		}
		a.State = "ready"
		a.Proposal = &proposal
		a.Reason = ""
		if !a.BasisCurrent {
			a.Reason = intelligenceSource
		}
	}
	if _, e = tx.ExecContext(ctx, s.q(`INSERT INTO suma_knowledge_intelligence_outcomes(tenant_id,project_id,analysis_id,attempt,fence,result_sha256,created_at,result)VALUES(?,?,?,?,?,?,?,?)`), tenant, project, a.ID, attempt, fence, sha, a.UpdatedAt, stringify(result)); e != nil {
		return e
	}
	r.Analysis = a
	return s.saveKnowledgeIntelligenceTx(ctx, tx, tenant, r, old)
}
