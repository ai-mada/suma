package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

// This private snapshot joins each enabled tool definition to its Worker's
// fixed release command. ReleaseProfileSHA256 identifies the CommandProfile,
// including the package_test ArtifactInput and TestEvidence requirements.
type testPlanPackageBinding struct {
	NodeID                     string                     `json:"node_id"`
	ToolProfileID              string                     `json:"tool_profile_id"`
	ToolProfileVersion         int                        `json:"tool_profile_version"`
	ToolProfileSHA256          string                     `json:"tool_profile_sha256"`
	WorkerID                   string                     `json:"worker_id"`
	ReleaseProfileSHA256       string                     `json:"release_profile_sha256"`
	SourceNodeID               string                     `json:"source_node_id"`
	SourceToolProfileID        string                     `json:"source_tool_profile_id"`
	SourceToolProfileVersion   int                        `json:"source_tool_profile_version"`
	SourceToolProfileSHA256    string                     `json:"source_tool_profile_sha256"`
	SourceWorkerID             string                     `json:"source_worker_id"`
	SourceReleaseProfileSHA256 string                     `json:"source_release_profile_sha256"`
	PoolRoute                  *dag.ProjectExecutionRoute `json:"pool_route,omitempty"`
}

func validateTestPlanPackageBindings(items []testPlanPackageBinding) error {
	seen := map[string]bool{}
	if len(items) > 1000 {
		return fmt.Errorf("test plan package binding is inconsistent")
	}
	for _, v := range items {
		if v.NodeID == "" || v.SourceNodeID == "" || v.NodeID == v.SourceNodeID || seen[v.NodeID] || !handoffID(v.ToolProfileID, false) || !handoffID(v.SourceToolProfileID, false) || v.ToolProfileVersion < 1 || v.SourceToolProfileVersion < 1 || v.WorkerID == "" || v.SourceWorkerID == "" || !contentDigest.MatchString(v.ToolProfileSHA256) || !contentDigest.MatchString(v.SourceToolProfileSHA256) || !contentDigest.MatchString(v.ReleaseProfileSHA256) || !contentDigest.MatchString(v.SourceReleaseProfileSHA256) {
			return fmt.Errorf("test plan package binding is inconsistent")
		}
		if r := v.PoolRoute; r != nil && (r.TenantID == "" || r.ProjectID == "" || r.Version < 1 || r.PoolID == "" || r.GrantID == "" || r.GrantVersion < 1 || v.WorkerID == v.SourceWorkerID) {
			return fmt.Errorf("test plan package pool route is inconsistent")
		}
		seen[v.NodeID] = true
	}
	return nil
}

func testPlanPackageBindingFor(fixed testPlanRunSnapshot, node string) (testPlanPackageBinding, bool) {
	for _, v := range fixed.PackageTests {
		if v.NodeID == node {
			return v, true
		}
	}
	return testPlanPackageBinding{}, false
}

// Pool authority validates the active grant separately. Package evidence also
// needs both profile revisions and that grant to match the published plan's
// frozen execution basis, even when a replacement profile has the same digest.
func (s *Service) checkPoolTestPlanArtifactProfileTx(ctx context.Context, tx *sql.Tx, tenant, project, run, nodeID string, p ToolProfile, scope dag.PoolScope) error {
	fixed, found, err := s.testPlanRunTx(ctx, tx, tenant, project, run)
	if err != nil || !found || fixed.Revision == nil {
		return err
	}
	for _, b := range fixed.PackageTests {
		var kind, id, digest, worker, releaseDigest string
		var version int
		switch nodeID {
		case b.NodeID:
			kind, id, version, digest, worker, releaseDigest = "package_test", b.ToolProfileID, b.ToolProfileVersion, b.ToolProfileSHA256, b.WorkerID, b.ReleaseProfileSHA256
		case b.SourceNodeID:
			kind, id, version, digest, worker, releaseDigest = "build", b.SourceToolProfileID, b.SourceToolProfileVersion, b.SourceToolProfileSHA256, b.SourceWorkerID, b.SourceReleaseProfileSHA256
		default:
			continue
		}
		route := b.PoolRoute
		if route == nil || route.TenantID != scope.TenantID || route.ProjectID != scope.ProjectID || route.PoolID != scope.PoolID || route.GrantID != scope.GrantID || route.GrantVersion != scope.GrantVersion ||
			p.ID != id || p.Version != version || p.WorkerID != worker || p.Profile.ProfileSHA256 != digest || p.Profile.ArtifactKind != kind || p.Profile.ArtifactProfileSHA256 != releaseDigest {
			return conflict(testPlanGate)
		}
	}
	return nil
}

func (s *Service) testPlanReleaseProfileTx(ctx context.Context, tx *sql.Tx, tenant, project string, profile ToolProfile, kind string) (string, error) {
	var raw string
	err := tx.QueryRowContext(ctx, s.q(`SELECT profiles FROM suma_release_workers WHERE tenant_id=? AND project_id=? AND worker_id=? AND status='active'`), tenant, project, profile.WorkerID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", invalid(testPlanBinding)
	}
	if err != nil {
		return "", err
	}
	var registered []ReleaseProfileRegistration
	if json.Unmarshal([]byte(raw), &registered) != nil {
		return "", fmt.Errorf("release worker profiles are inconsistent")
	}
	for _, v := range registered {
		if v.ID == profile.Profile.CommandID && v.Kind == kind && contentDigest.MatchString(v.SHA256) {
			return v.SHA256, nil
		}
	}
	return "", invalid(testPlanBinding)
}

func (s *Service) freezeTestPlanPackageBindingTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project, run string, node dag.Node, profile ToolProfile, nodes []dag.Node, route dag.ProjectExecutionRoute) (out testPlanPackageBinding, err error) {
	source, ok := node.Config["artifact_from"].(string)
	if !ok || source == "" || profile.Profile.TestReportProtocol != "" || profile.Profile.Adapter != "command" || profile.Profile.Protocol != "text-v1" || profile.Profile.ReadOnly {
		return out, invalid(testPlanBinding)
	}
	var producer dag.Node
	for _, candidate := range nodes {
		if candidate.ID == source {
			producer = candidate
			break
		}
	}
	if producer.Type != "command" && producer.Type != "agent" {
		return out, invalid(testPlanBinding)
	}
	id, sha, bound, err := dag.ToolBinding(producer)
	if err != nil || !bound {
		return out, invalid(testPlanBinding)
	}
	build, err := s.toolProfileTx(ctx, tx, auth.currentPrincipal, project, id)
	if err != nil {
		return out, err
	}
	if len(producer.Command) != 1 || build.Profile.CommandID != producer.Command[0] || build.Profile.ProfileSHA256 != sha || build.Profile.Adapter != "command" || build.Profile.Protocol != "text-v1" || build.Profile.ReadOnly || build.Profile.TestReportProtocol != "" {
		return out, invalid(testPlanBinding)
	}
	if _, err = s.validateToolDispatchTx(ctx, tx, auth, project, id, build.Version, sha); err != nil {
		return out, err
	}
	// DAG compilation already validates artifact_from as a static ancestor.
	// Re-read its immutable published graph so this fact is not inferred from
	// two matching aliases or a caller-supplied flag in a result.
	var raw string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT w.spec FROM dag_workflows w JOIN dag_runs r ON r.tenant_id=w.tenant_id AND r.project_id=w.project_id AND r.workflow_id=w.id AND r.workflow_version=w.version WHERE r.tenant_id=? AND r.project_id=? AND r.id=?`), auth.target.tenantID, project, run).Scan(&raw); err != nil {
		return out, err
	}
	var wf dag.Workflow
	if json.Unmarshal([]byte(raw), &wf) != nil {
		return out, invalid(testPlanBinding)
	}
	parents := map[string][]string{}
	for _, edge := range wf.Edges {
		parents[edge.To] = append(parents[edge.To], edge.From)
	}
	seen := map[string]bool{}
	queue := append([]string{}, parents[node.ID]...)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		queue = append(queue, parents[id]...)
	}
	if !seen[source] {
		return out, invalid(testPlanBinding)
	}
	var poolRoute *dag.ProjectExecutionRoute
	var packageSHA, buildSHA string
	if route.PoolID != "" {
		if profile.WorkerID == build.WorkerID {
			return out, invalid(testPlanBinding)
		}
		if err = s.checkDispatchProfileRouteTx(ctx, tx, auth.target.tenantID, project, build, route); err != nil {
			return out, err
		}
		if kind, e := poolArtifactNodeKind(node, profile); e != nil || kind != "package_test" {
			return out, invalid(testPlanBinding)
		}
		if kind, e := poolArtifactNodeKind(producer, build); e != nil || kind != "build" {
			return out, invalid(testPlanBinding)
		}
		basis, err := rc.DecodeApplicationBuild(producer.Config)
		if err != nil {
			return out, invalid(testPlanBinding)
		}
		if err = s.checkApplicationBuildBasisTx(ctx, tx, auth.currentPrincipal, project, basis); err != nil {
			return out, err
		}
		poolRoute = &route
		packageSHA, buildSHA = profile.Profile.ArtifactProfileSHA256, build.Profile.ArtifactProfileSHA256
	} else {
		packageSHA, err = s.testPlanReleaseProfileTx(ctx, tx, auth.target.tenantID, project, profile, "package_test")
		if err != nil {
			return out, err
		}
		buildSHA, err = s.testPlanReleaseProfileTx(ctx, tx, auth.target.tenantID, project, build, "build")
		if err != nil {
			return out, err
		}
	}
	return testPlanPackageBinding{
		NodeID:                     node.ID,
		ToolProfileID:              profile.ID,
		ToolProfileVersion:         profile.Version,
		ToolProfileSHA256:          profile.Profile.ProfileSHA256,
		WorkerID:                   profile.WorkerID,
		ReleaseProfileSHA256:       packageSHA,
		SourceNodeID:               source,
		SourceToolProfileID:        build.ID,
		SourceToolProfileVersion:   build.Version,
		SourceToolProfileSHA256:    build.Profile.ProfileSHA256,
		SourceWorkerID:             build.WorkerID,
		SourceReleaseProfileSHA256: buildSHA,
		PoolRoute:                  poolRoute,
	}, nil
}

func validateTestPlanPackageResult(fixed testPlanPackageBinding, node dag.Node, result dag.Result) error {
	if node.Type != "test" || node.Config["tool_profile_id"] != fixed.ToolProfileID || node.Config["tool_profile_sha256"] != fixed.ToolProfileSHA256 || node.Config["artifact_from"] != fixed.SourceNodeID || result.WorkerID != fixed.WorkerID || len(node.Command) != 1 || result.TestReport != nil {
		return invalid(testPlanBinding)
	}
	if result.ToolEvidence != nil && (result.ToolEvidence.Profile.CommandID != node.Command[0] || result.ToolEvidence.Profile.ProfileSHA256 != fixed.ToolProfileSHA256 || result.ToolEvidence.Profile.Adapter != "command" || result.ToolEvidence.Profile.Protocol != "text-v1" || result.ToolEvidence.Profile.TestReportProtocol != "") {
		return invalid(testPlanBinding)
	}
	if fixed.PoolRoute != nil && result.ToolEvidence != nil && (result.ToolEvidence.Profile.ArtifactKind != "package_test" || result.ToolEvidence.Profile.ArtifactProfileSHA256 != fixed.ReleaseProfileSHA256) {
		return invalid(testPlanBinding)
	}
	if e := result.ArtifactEvidence; e != nil && (e.Kind != "package_test" || e.ProfileID != node.Command[0] || e.ProfileSHA256 != fixed.ReleaseProfileSHA256) {
		return invalid(testPlanBinding)
	}
	return nil
}

// Missing or unknown artifact evidence leaves the quality plan incomplete.
// For completed Pool attempts the immutable run route and verified receipts
// remain the basis even after a later grant renewal or revocation.
func (s *Service) testPlanPackageProofCurrentTx(ctx context.Context, tx *sql.Tx, tenant, project, run string, fixed testPlanPackageBinding, node testPlanNodeFact) (*ArtifactProof, error) {
	if node.State != "succeeded" || node.Worker != fixed.WorkerID || node.Spec.Config["tool_profile_id"] != fixed.ToolProfileID || node.Spec.Config["tool_profile_sha256"] != fixed.ToolProfileSHA256 || node.Spec.Config["artifact_from"] != fixed.SourceNodeID {
		return nil, nil
	}
	var id string
	err := tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_artifact_proofs WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=? AND kind='package_test'`), tenant, project, run, fixed.NodeID, node.Attempt, node.Fence, node.Worker).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	proof, err := s.currentArtifactProofTx(ctx, tx, Principal{TenantID: tenant}, project, id)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) && (api.Status == 404 || api.Status == 409) {
			return nil, nil
		}
		return nil, err
	}
	if proof.ProfileSHA256 != fixed.ReleaseProfileSHA256 || len(node.Spec.Command) != 1 || proof.ProfileID != node.Spec.Command[0] {
		return nil, nil
	}
	input, err := s.testPlanPackageInputCurrentTx(ctx, tx, tenant, project, run, fixed, node)
	if err != nil || input == nil {
		return nil, err
	}
	if input.ArtifactID != proof.ArtifactID || input.ManifestSHA256 != proof.ManifestSHA256 || input.SourceCommit != proof.SourceCommit {
		return nil, nil
	}
	return &proof, nil
}

// Both a verified package proof and a failed test must refer to the exact
// sealed input resolved by that attempt and its still-current build ancestor.
func (s *Service) testPlanPackageInputCurrentTx(ctx context.Context, tx *sql.Tx, tenant, project, run string, fixed testPlanPackageBinding, node testPlanNodeFact) (*rc.ArtifactBinding, error) {
	if fixed.PoolRoute != nil {
		route, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, run)
		if errors.Is(err, dag.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if route != *fixed.PoolRoute {
			return nil, nil
		}
	}
	var raw string
	err := tx.QueryRowContext(ctx, s.q(`SELECT binding FROM suma_artifact_bindings WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=? AND source_node_id=?`), tenant, project, run, fixed.NodeID, node.Attempt, node.Fence, node.Worker, fixed.SourceNodeID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var input rc.ArtifactBinding
	if json.Unmarshal([]byte(raw), &input) != nil || input.BuildNodeID != fixed.SourceNodeID || input.BuildRunID != run {
		return nil, nil
	}
	var id string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_artifact_proofs WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=? AND kind='build'`), tenant, project, run, fixed.SourceNodeID, input.BuildAttempt, input.BuildFence, fixed.SourceWorkerID).Scan(&id); errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	build, err := s.currentArtifactProofTx(ctx, tx, Principal{TenantID: tenant}, project, id)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) && (api.Status == 404 || api.Status == 409) {
			return nil, nil
		}
		return nil, err
	}
	if build.ProfileSHA256 != fixed.SourceReleaseProfileSHA256 || build.ArtifactID != input.ArtifactID || build.ManifestSHA256 != input.ManifestSHA256 || build.SourceCommit != input.SourceCommit {
		return nil, nil
	}
	if fixed.PoolRoute != nil && (build.ApplicationBuild == nil || build.Assignment.WorkerID == node.Worker) {
		return nil, nil
	}
	var specRaw, resultRaw string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT n.spec,a.result FROM dag_nodes n JOIN dag_attempts a ON a.run_id=n.run_id AND a.node_id=n.id AND a.attempt=n.attempt AND a.fence=n.fence AND a.worker_id=n.worker_id WHERE n.run_id=? AND n.id=? AND n.attempt=? AND n.fence=? AND n.worker_id=?`), run, fixed.SourceNodeID, input.BuildAttempt, input.BuildFence, fixed.SourceWorkerID).Scan(&specRaw, &resultRaw); errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var source dag.Node
	var result dag.Result
	if json.Unmarshal([]byte(specRaw), &source) != nil || json.Unmarshal([]byte(resultRaw), &result) != nil || source.Config["tool_profile_id"] != fixed.SourceToolProfileID || source.Config["tool_profile_sha256"] != fixed.SourceToolProfileSHA256 || len(source.Command) != 1 || source.Command[0] != build.ProfileID || result.ToolEvidence == nil || result.ToolEvidence.Profile.ProfileSHA256 != fixed.SourceToolProfileSHA256 || result.ToolEvidence.Profile.CommandID != build.ProfileID {
		return nil, nil
	}
	if fixed.PoolRoute != nil && (result.ToolEvidence.Profile.ArtifactKind != "build" || result.ToolEvidence.Profile.ArtifactProfileSHA256 != fixed.SourceReleaseProfileSHA256) {
		return nil, nil
	}
	return &input, nil
}
