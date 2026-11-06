package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	rc "github.com/ai-mada/suma/internal/releasecontract"
)

const issueRepairReleaseStale = "The closed repair or its release evidence has changed"

func staleIssueRepairReleaseBasis(err error) error {
	if err != nil {
		var api *APIError
		if !errors.As(err, &api) || api.Status != 400 && api.Status != 404 && api.Status != 409 {
			return err
		}
	}
	return releaseConflict("release_basis_changed", issueRepairReleaseStale)
}

// Reconstruct the handoff from the protected closure and the actual artifact
// input binding. The caller has already authorized the project. A historical
// closure remains readable even when it is no longer eligible for release.
func (s *Service) issueRepairReleaseHandoffTx(ctx context.Context, tx *sql.Tx, p Principal, project, issueID string, current bool) (*IssueRepairReleaseHandoff, error) {
	stale := func() error { return staleIssueRepairReleaseBasis(nil) }
	var status string
	var issueVersion int
	err := tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT status,version FROM suma_issues WHERE tenant_id=? AND project_id=? AND id=?`, current)), p.TenantID, project, issueID).Scan(&status, &issueVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, stale()
	}
	if err != nil {
		return nil, err
	}
	if status != "closed" {
		return nil, stale()
	}
	repair, err := s.issueRepairLatestReadTx(ctx, tx, p.TenantID, project, issueID, current)
	if err != nil {
		return nil, staleIssueRepairReleaseBasis(err)
	}
	if repair == nil || repair.ReopenedAt != "" || repair.Closure == nil || repair.Closure.PackageRegression == nil {
		return nil, stale()
	}
	closure, pkg := repair.Closure, repair.Closure.PackageRegression
	basis, err := s.releaseTaskBasisReadTx(ctx, tx, p, project, repair.TaskID, closure.RunID, current)
	if err != nil {
		return nil, staleIssueRepairReleaseBasis(err)
	}
	if basis.Version != closure.TaskVersion || basis.ReviewID != closure.ReviewID {
		return nil, stale()
	}
	plan, found, err := s.testPlanByTaskTx(ctx, tx, p.TenantID, project, repair.TaskID)
	if err != nil {
		return nil, err
	}
	if !found || plan.ID != closure.PlanID || plan.State != "published" || plan.Published != closure.PlanRevision {
		return nil, stale()
	}
	fixed, found, err := s.testPlanRunTx(ctx, tx, p.TenantID, project, closure.RunID)
	if err != nil {
		return nil, err
	}
	if !found || fixed.TaskID != repair.TaskID || fixed.PlanID != plan.ID || fixed.Revision == nil ||
		fixed.Revision.Revision != closure.PlanRevision || fixed.Revision.SHA256 != closure.PlanSHA256 || fixed.GovernanceRevision != plan.Governance {
		return nil, stale()
	}
	sourceCurrent, err := s.testPlanSourceCurrentTx(ctx, tx, p.TenantID, project, fixed.Revision.Source)
	if err != nil {
		return nil, staleIssueRepairReleaseBasis(err)
	}
	if !sourceCurrent {
		return nil, stale()
	}
	binding, found := testPlanPackageBindingFor(fixed, pkg.NodeID)
	if !found || binding.WorkerID == binding.SourceWorkerID {
		return nil, stale()
	}
	test, err := s.artifactProofReadTx(ctx, tx, p, project, pkg.ProofID, current)
	if err != nil {
		return nil, staleIssueRepairReleaseBasis(err)
	}
	if test.Kind != "package_test" || test.ArtifactID != pkg.ArtifactID || test.ManifestSHA256 != pkg.ManifestSHA256 || test.SourceCommit != pkg.SourceCommit ||
		test.Assignment.RunID != closure.RunID || test.Assignment.NodeID != pkg.NodeID || test.Assignment.Attempt != pkg.Attempt ||
		test.Assignment.Fence != pkg.Fence || test.Assignment.WorkerID != binding.WorkerID || test.ProfileSHA256 != binding.ReleaseProfileSHA256 {
		return nil, stale()
	}
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT binding FROM suma_artifact_bindings WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=? AND source_node_id=?`),
		p.TenantID, project, closure.RunID, pkg.NodeID, pkg.Attempt, pkg.Fence, binding.WorkerID, binding.SourceNodeID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, stale()
	}
	if err != nil {
		return nil, err
	}
	var input rc.ArtifactBinding
	if err = json.Unmarshal([]byte(raw), &input); err != nil {
		return nil, err
	}
	if input.BuildRunID != closure.RunID || input.BuildNodeID != binding.SourceNodeID || input.ArtifactID != pkg.ArtifactID ||
		input.ManifestSHA256 != pkg.ManifestSHA256 || input.SourceCommit != pkg.SourceCommit {
		return nil, stale()
	}
	var buildID string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_artifact_proofs WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=? AND kind='build'`),
		p.TenantID, project, closure.RunID, input.BuildNodeID, input.BuildAttempt, input.BuildFence, binding.SourceWorkerID).Scan(&buildID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, stale()
	}
	if err != nil {
		return nil, err
	}
	build, err := s.artifactProofReadTx(ctx, tx, p, project, buildID, current)
	if err != nil {
		return nil, staleIssueRepairReleaseBasis(err)
	}
	if build.ArtifactID != pkg.ArtifactID || build.ManifestSHA256 != pkg.ManifestSHA256 || build.SourceCommit != pkg.SourceCommit ||
		build.ProfileSHA256 != binding.SourceReleaseProfileSHA256 {
		return nil, stale()
	}
	// Candidate lists use snapshot reads. Still report application source and
	// repository drift for a linked repair instead of promising forward use.
	if !current {
		if err = s.checkApplicationBuildBasisTx(ctx, tx, p, project, build.ApplicationBuild); err != nil {
			return nil, staleIssueRepairReleaseBasis(err)
		}
	}
	// The issue view only needs immutable sealed metadata. Candidate actions
	// separately verify every blob before use under the original release gate.
	_, manifestSHA, _, err := s.sealedArtifactManifestTx(ctx, tx, p, project, pkg.ArtifactID)
	if err != nil {
		return nil, staleIssueRepairReleaseBasis(err)
	}
	if manifestSHA != pkg.ManifestSHA256 {
		return nil, stale()
	}
	return &IssueRepairReleaseHandoff{
		IssueID: issueID, IssueVersion: issueVersion, RepairID: repair.ID, RepairVersion: repair.Version, Cycle: repair.Cycle,
		TaskID: basis.ID, TaskVersion: basis.Version, RunID: basis.RunID, ReviewID: basis.ReviewID, ArtifactID: pkg.ArtifactID,
		BuildAttestationID: build.ID, PackageTestAttestationID: test.ID,
	}, nil
}

func (h IssueRepairReleaseHandoff) matchesCandidate(c ReleaseCandidate) bool {
	return h.ArtifactID == c.ArtifactID && h.BuildAttestationID == c.BuildAttestationID &&
		h.PackageTestAttestationID == c.PackageTestAttestationID && len(c.Tasks) == 1 &&
		c.Tasks[0].ID == h.TaskID && c.Tasks[0].Version == h.TaskVersion && c.Tasks[0].RunID == h.RunID && c.Tasks[0].ReviewID == h.ReviewID
}
