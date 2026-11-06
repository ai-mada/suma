package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	rc "github.com/ai-mada/suma/internal/releasecontract"
)

func releaseCandidateMatchesPlan(candidate ReleaseCandidate, content rc.ReleasePlanContent) bool {
	return candidate.ArtifactID == content.ArtifactID &&
		candidate.ArtifactSHA256 == content.ArtifactSHA256 &&
		candidate.SourceCommit == content.SourceCommit &&
		candidate.BuildAttestationID == content.BuildAttestationID &&
		candidate.PackageTestAttestationID == content.PackageTestAttestationID &&
		candidate.AcceptedTaskBasisSHA256 == content.AcceptedTaskBasisSHA256
}

// Release details can link back to historical evidence without making an
// eligibility decision or reopening the candidate's sealed blobs.
func (s *Service) releasePlanRepairSourceReadTx(ctx context.Context, tx *sql.Tx, tenant, project string, plan ReleasePlan) (*ReleaseRepairSource, error) {
	content := plan.Content
	if plan.ProjectID != project || plan.SHA256 != releasePlanHash(content) {
		return nil, nil
	}
	var raw string
	err := tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_release_candidates WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, content.CandidateID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var candidate ReleaseCandidate
	if json.Unmarshal([]byte(raw), &candidate) != nil || candidate.ProjectID != project || candidate.ID != content.CandidateID || !releaseCandidateMatchesPlan(candidate, content) {
		return nil, nil
	}
	repair := candidate.RepairHandoff
	if repair == nil || !handoffID(repair.IssueID, false) || !designVersion(repair.Cycle) || repair.Cycle < 1 || !repair.matchesCandidate(candidate) {
		return nil, nil
	}
	return &ReleaseRepairSource{IssueID: repair.IssueID, Cycle: repair.Cycle, CandidateID: candidate.ID}, nil
}

func (s *Service) releaseDeploymentRepairSourceReadTx(ctx context.Context, tx *sql.Tx, tenant, project string, deployment ReleaseDeployment) (*ReleaseRepairSource, error) {
	plan, err := s.releasePlanTx(ctx, tx, tenant, project, deployment.PlanID)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == 404 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	content := plan.Content
	if plan.ID != deployment.PlanID || deployment.ProjectID != project || plan.ExecutionOperationID != deployment.OperationID ||
		plan.Revision != deployment.PlanRevision || plan.SHA256 != deployment.PlanSHA256 ||
		content.ArtifactID != deployment.ArtifactID || content.ArtifactSHA256 != deployment.ArtifactSHA256 ||
		content.EnvironmentID != deployment.EnvironmentID || content.ProviderID != deployment.ProviderID || content.Generation != deployment.Generation ||
		content.ReservedExecution.RunID != deployment.RunID || content.ReservedExecution.NodeID != deployment.NodeID {
		return nil, nil
	}
	return s.releasePlanRepairSourceReadTx(ctx, tx, tenant, project, plan)
}
