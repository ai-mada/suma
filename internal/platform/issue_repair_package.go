package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

type issueRepairPackageOrigin struct {
	IssueRepairPackageOrigin
	PlanID          string                `json:"plan_id"`
	PlanRevision    int                   `json:"plan_revision"`
	PlanSHA256      string                `json:"plan_sha256"`
	TaskID          string                `json:"task_id"`
	ExitCode        int                   `json:"exit_code"`
	WorkerID        string                `json:"worker_id"`
	BuildRunID      string                `json:"build_run_id"`
	BuildNodeID     string                `json:"build_node_id"`
	BuildAttempt    int                   `json:"build_attempt"`
	BuildFence      int64                 `json:"build_fence"`
	BuildWorkerID   string                `json:"build_worker_id"`
	Scope           *ApplicationCodeScope `json:"-"`
	TestProfileSHA  string                `json:"-"`
	BuildProfileSHA string                `json:"-"`
}

// The original failure and build are historical. Look up the recorded attempt,
// without depending on the original run's current node or application version.
func (s *Service) issueRepairPackageBuildTx(ctx context.Context, tx *sql.Tx, p Principal, project string, ref rc.AssignmentRef, artifact, manifest, commit string) (ArtifactProof, *ApplicationCodeScope, error) {
	proof, err := scanArtifactProof(tx.QueryRowContext(ctx, s.q(`SELECT `+artifactProofColumns+` FROM suma_artifact_proofs WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=? AND kind='build'`), p.TenantID, project, ref.RunID, ref.NodeID, ref.Attempt, ref.Fence, ref.WorkerID))
	if errors.Is(err, sql.ErrNoRows) {
		return proof, nil, conflict(issueRepairSource)
	}
	if err != nil {
		return proof, nil, err
	}
	if proof.ArtifactID != artifact || proof.ManifestSHA256 != manifest || proof.SourceCommit != commit {
		return proof, nil, conflict(issueRepairSource)
	}
	var state, spec, result string
	err = tx.QueryRowContext(ctx, s.q(`SELECT a.state,n.spec,a.result FROM dag_attempts a JOIN dag_nodes n ON n.run_id=a.run_id AND n.id=a.node_id JOIN dag_runs r ON r.id=a.run_id WHERE r.tenant_id=? AND r.project_id=? AND a.run_id=? AND a.node_id=? AND a.attempt=? AND a.fence=? AND a.worker_id=?`), p.TenantID, project, ref.RunID, ref.NodeID, ref.Attempt, ref.Fence, ref.WorkerID).Scan(&state, &spec, &result)
	if errors.Is(err, sql.ErrNoRows) {
		return proof, nil, conflict(issueRepairSource)
	}
	if err != nil {
		return proof, nil, err
	}
	var node dag.Node
	var receipt dag.Result
	if state != "succeeded" || json.Unmarshal([]byte(spec), &node) != nil || json.Unmarshal([]byte(result), &receipt) != nil ||
		artifactRef(receipt) != ref || !artifactFinalResultMatchesProof(receipt, proof) {
		return proof, nil, conflict(issueRepairSource)
	}
	basis, err := applicationBuildFromFinalResult(node, receipt, proof)
	if err != nil || basis == nil {
		return proof, nil, err
	}
	proof.ApplicationBuild = basis
	version, err := s.applicationVersionReadTx(ctx, tx, p, project, basis.ApplicationID, basis.VersionID)
	if err != nil {
		return proof, nil, err
	}
	if !applicationBuildMatchesVersion(basis, version) {
		return proof, nil, conflict(issueRepairSource)
	}
	return proof, &version.Sources[0].Scope, nil
}

func (s *Service) issueRepairOriginalPackageTx(ctx context.Context, auth *authorizedCommand, project, issue string) (*issueRepairPackageOrigin, error) {
	tenant, tx := auth.target.tenantID, auth.tx
	rows, err := tx.QueryContext(ctx, s.q(`SELECT plan_id,run_id,node_id,attempt,fence,evidence FROM suma_test_plan_package_defects WHERE tenant_id=? AND project_id=? AND issue_id=? LIMIT 2`), tenant, project, issue)
	if err != nil {
		return nil, err
	}
	if !rows.Next() {
		err = rows.Err()
		rows.Close()
		return nil, err
	}
	var row testPlanPackageAttempt
	var plan, run, raw string
	err = rows.Scan(&plan, &run, &row.NodeID, &row.Attempt, &row.Fence, &raw)
	duplicate := err == nil && rows.Next()
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	var origin issueRepairPackageOrigin
	buildRef := func() rc.AssignmentRef {
		return rc.AssignmentRef{RunID: origin.BuildRunID, NodeID: origin.BuildNodeID, Attempt: origin.BuildAttempt, Fence: origin.BuildFence, WorkerID: origin.BuildWorkerID}
	}
	if duplicate || json.Unmarshal([]byte(raw), &origin) != nil || origin.PlanID != plan || origin.RunID != run || origin.NodeID != row.NodeID ||
		origin.Attempt != row.Attempt || origin.Fence != row.Fence || origin.ExitCode <= 0 || origin.BuildRunID != run || !buildRef().Valid() ||
		!handoffID(origin.ArtifactID, false) || !rc.ValidDigest(origin.ManifestSHA256) || !rc.ValidCommit(origin.SourceCommit) {
		return nil, conflict(issueRepairSource)
	}
	fixed, found, err := s.testPlanRunTx(ctx, tx, tenant, project, run)
	if err != nil {
		return nil, err
	}
	if !found || fixed.PlanID != plan || fixed.TaskID != origin.TaskID || fixed.Revision == nil ||
		fixed.Revision.Revision != origin.PlanRevision || fixed.Revision.SHA256 != origin.PlanSHA256 {
		return nil, conflict(issueRepairSource)
	}
	binding, found := testPlanPackageBindingFor(fixed, origin.NodeID)
	if !found || binding.WorkerID != origin.WorkerID || binding.SourceNodeID != origin.BuildNodeID ||
		binding.SourceWorkerID != origin.BuildWorkerID || origin.BuildWorkerID == origin.WorkerID {
		return nil, conflict(issueRepairSource)
	}
	proof, scope, err := s.issueRepairPackageBuildTx(ctx, tx, auth.currentPrincipal, project, buildRef(), origin.ArtifactID, origin.ManifestSHA256, origin.SourceCommit)
	if err != nil {
		return nil, err
	}
	if proof.ProfileSHA256 != binding.SourceReleaseProfileSHA256 {
		return nil, conflict(issueRepairSource)
	}
	origin.TestProfileSHA = binding.ReleaseProfileSHA256
	origin.BuildProfileSHA = proof.ProfileSHA256
	origin.Scope = scope
	if scope != nil {
		origin.ApplicationID, origin.ScopeID, origin.ScopeRoot, origin.RepositoryID = proof.ApplicationBuild.ApplicationID, scope.ID, scope.RelativeRoot, scope.TargetID
	}
	return &origin, nil
}

func issueRepairPackageScopeMatches(origin *issueRepairPackageOrigin, build ArtifactProof, scope *ApplicationCodeScope) bool {
	if origin.Scope == nil {
		// Ordinary artifacts have no authenticated repository target. A new
		// build can recheck the identical code under the same build authority.
		return scope == nil && build.Assignment.WorkerID == origin.BuildWorkerID &&
			build.ProfileSHA256 == origin.BuildProfileSHA && build.SourceCommit == origin.SourceCommit
	}
	return scope != nil && build.ApplicationBuild.ApplicationID == origin.ApplicationID &&
		scope.ID == origin.Scope.ID && scope.RelativeRoot == origin.Scope.RelativeRoot && scope.TargetID == origin.Scope.TargetID &&
		scope.ProviderID == origin.Scope.ProviderID && scope.TargetRef == origin.Scope.TargetRef && scope.ConfigSHA256 == origin.Scope.ConfigSHA256
}

// Only evidence already verified by the current test-plan projection may be
// used. Rejoin its sealed input and original build to compare governed scope.
func (s *Service) issueRepairPackageRegressionTx(ctx context.Context, auth *authorizedCommand, project string, origin *issueRepairPackageOrigin, fixed testPlanRunSnapshot, execution TestPlanExecution) (*IssueRepairPackageProof, string, error) {
	tenant, tx := auth.target.tenantID, auth.tx
	blocker := "package_regression_missing"
	for _, item := range execution.PackageEvidence {
		if item.State != "verified" {
			continue
		}
		blocker = "package_regression_scope_changed"
		binding, found := testPlanPackageBindingFor(fixed, item.NodeID)
		if !found || binding.ReleaseProfileSHA256 != origin.TestProfileSHA || item.WorkerID == binding.SourceWorkerID ||
			execution.RunID == origin.RunID {
			continue
		}
		var proofID, raw string
		err := tx.QueryRowContext(ctx, s.q(`SELECT p.id,b.binding FROM suma_artifact_proofs p JOIN suma_artifact_bindings b ON b.tenant_id=p.tenant_id AND b.project_id=p.project_id AND b.run_id=p.run_id AND b.node_id=p.node_id AND b.attempt=p.attempt AND b.fence=p.fence AND b.worker_id=p.worker_id WHERE p.tenant_id=? AND p.project_id=? AND p.run_id=? AND p.node_id=? AND p.attempt=? AND p.fence=? AND p.worker_id=? AND p.kind='package_test' AND p.artifact_id=? AND p.manifest_sha256=? AND p.source_commit=? AND p.profile_sha256=? AND b.source_node_id=?`),
			tenant, project, execution.RunID, item.NodeID, item.Attempt, item.Fence, item.WorkerID, item.ArtifactID, item.ManifestSHA256, item.SourceCommit, binding.ReleaseProfileSHA256, binding.SourceNodeID).Scan(&proofID, &raw)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		var input rc.ArtifactBinding
		if json.Unmarshal([]byte(raw), &input) != nil || input.BuildRunID != execution.RunID || input.BuildNodeID != binding.SourceNodeID {
			continue
		}
		buildRef := rc.AssignmentRef{RunID: input.BuildRunID, NodeID: input.BuildNodeID, Attempt: input.BuildAttempt, Fence: input.BuildFence, WorkerID: binding.SourceWorkerID}
		build, scope, err := s.issueRepairPackageBuildTx(ctx, tx, auth.currentPrincipal, project, buildRef, item.ArtifactID, item.ManifestSHA256, item.SourceCommit)
		if err != nil {
			return nil, "", err
		}
		if !issueRepairPackageScopeMatches(origin, build, scope) {
			continue
		}
		return &IssueRepairPackageProof{NodeID: item.NodeID, Attempt: item.Attempt, Fence: item.Fence, ArtifactID: item.ArtifactID, ManifestSHA256: item.ManifestSHA256, SourceCommit: item.SourceCommit, ProofID: proofID}, "", nil
	}
	return nil, blocker, nil
}
