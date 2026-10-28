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

// This private snapshot joins two existing administrator-controlled contracts:
// an enabled tool definition and the same Worker's fixed release-command kind.
// ReleaseProfileSHA256 hashes the validated CommandProfile (including the
// package_test ArtifactInput/TestEvidence requirements), not the tool digest.
type testPlanPackageBinding struct {
	NodeID                     string `json:"node_id"`
	ToolProfileID              string `json:"tool_profile_id"`
	ToolProfileVersion         int    `json:"tool_profile_version"`
	ToolProfileSHA256          string `json:"tool_profile_sha256"`
	WorkerID                   string `json:"worker_id"`
	ReleaseProfileSHA256       string `json:"release_profile_sha256"`
	SourceNodeID               string `json:"source_node_id"`
	SourceToolProfileID        string `json:"source_tool_profile_id"`
	SourceToolProfileVersion   int    `json:"source_tool_profile_version"`
	SourceToolProfileSHA256    string `json:"source_tool_profile_sha256"`
	SourceWorkerID             string `json:"source_worker_id"`
	SourceReleaseProfileSHA256 string `json:"source_release_profile_sha256"`
}

func validateTestPlanPackageBindings(items []testPlanPackageBinding) error {
	seen := map[string]bool{}
	if len(items) > 10000 {
		return fmt.Errorf("test plan package binding is inconsistent")
	}
	for _, v := range items {
		if v.NodeID == "" || v.SourceNodeID == "" || v.NodeID == v.SourceNodeID || seen[v.NodeID] || !handoffID(v.ToolProfileID, false) || !handoffID(v.SourceToolProfileID, false) || v.ToolProfileVersion < 1 || v.SourceToolProfileVersion < 1 || v.WorkerID == "" || v.SourceWorkerID == "" || !contentDigest.MatchString(v.ToolProfileSHA256) || !contentDigest.MatchString(v.SourceToolProfileSHA256) || !contentDigest.MatchString(v.ReleaseProfileSHA256) || !contentDigest.MatchString(v.SourceReleaseProfileSHA256) {
			return fmt.Errorf("test plan package binding is inconsistent")
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

func (s *Service) freezeTestPlanPackageBindingTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project, run string, node dag.Node, profile ToolProfile, nodes []dag.Node) (out testPlanPackageBinding, err error) {
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
	packageSHA, err := s.testPlanReleaseProfileTx(ctx, tx, auth.target.tenantID, project, profile, "package_test")
	if err != nil {
		return out, err
	}
	buildSHA, err := s.testPlanReleaseProfileTx(ctx, tx, auth.target.tenantID, project, build, "build")
	if err != nil {
		return out, err
	}
	return testPlanPackageBinding{NodeID: node.ID, ToolProfileID: profile.ID, ToolProfileVersion: profile.Version, ToolProfileSHA256: profile.Profile.ProfileSHA256, WorkerID: profile.WorkerID, ReleaseProfileSHA256: packageSHA, SourceNodeID: source, SourceToolProfileID: build.ID, SourceToolProfileVersion: build.Version, SourceToolProfileSHA256: build.Profile.ProfileSHA256, SourceWorkerID: build.WorkerID, SourceReleaseProfileSHA256: buildSHA}, nil
}

func validateTestPlanPackageResult(fixed testPlanPackageBinding, node dag.Node, result dag.Result) error {
	if node.Type != "test" || node.Config["tool_profile_id"] != fixed.ToolProfileID || node.Config["tool_profile_sha256"] != fixed.ToolProfileSHA256 || node.Config["artifact_from"] != fixed.SourceNodeID || result.WorkerID != fixed.WorkerID || len(node.Command) != 1 || result.TestReport != nil {
		return invalid(testPlanBinding)
	}
	if result.ToolEvidence != nil && (result.ToolEvidence.Profile.CommandID != node.Command[0] || result.ToolEvidence.Profile.ProfileSHA256 != fixed.ToolProfileSHA256 || result.ToolEvidence.Profile.Adapter != "command" || result.ToolEvidence.Profile.Protocol != "text-v1" || result.ToolEvidence.Profile.TestReportProtocol != "") {
		return invalid(testPlanBinding)
	}
	if e := result.ArtifactEvidence; e != nil && (e.Kind != "package_test" || e.ProfileID != node.Command[0] || e.ProfileSHA256 != fixed.ReleaseProfileSHA256) {
		return invalid(testPlanBinding)
	}
	return nil
}

// Missing or unknown artifact evidence is never a fabricated JUnit failure or
// pass. It makes the plan incomplete until the original release proof exists.
func (s *Service) testPlanPackageProofCurrentTx(ctx context.Context, tx *sql.Tx, tenant, project, run string, fixed testPlanPackageBinding, node testPlanNodeFact) (bool, error) {
	if node.State == "skipped" {
		return true, nil
	}
	if node.State != "succeeded" || node.Worker != fixed.WorkerID {
		return false, nil
	}
	var id string
	err := tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_artifact_proofs WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=? AND kind='package_test'`), tenant, project, run, fixed.NodeID, node.Attempt, node.Fence, node.Worker).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	proof, err := s.currentArtifactProofTx(ctx, tx, Principal{TenantID: tenant}, project, id)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) && (api.Status == 404 || api.Status == 409) {
			return false, nil
		}
		return false, err
	}
	if proof.ProfileSHA256 != fixed.ReleaseProfileSHA256 || len(node.Spec.Command) != 1 || proof.ProfileID != node.Spec.Command[0] {
		return false, nil
	}
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT binding FROM suma_artifact_bindings WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=? AND source_node_id=?`), tenant, project, run, fixed.NodeID, node.Attempt, node.Fence, node.Worker, fixed.SourceNodeID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var input rc.ArtifactBinding
	if json.Unmarshal([]byte(raw), &input) != nil || input.BuildNodeID != fixed.SourceNodeID || input.BuildRunID != run || input.ArtifactID != proof.ArtifactID || input.ManifestSHA256 != proof.ManifestSHA256 || input.SourceCommit != proof.SourceCommit {
		return false, nil
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_artifact_proofs WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=? AND kind='build'`), tenant, project, run, fixed.SourceNodeID, input.BuildAttempt, input.BuildFence, fixed.SourceWorkerID).Scan(&id); errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	build, err := s.currentArtifactProofTx(ctx, tx, Principal{TenantID: tenant}, project, id)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) && (api.Status == 404 || api.Status == 409) {
			return false, nil
		}
		return false, err
	}
	if build.ProfileSHA256 != fixed.SourceReleaseProfileSHA256 || build.ArtifactID != proof.ArtifactID || build.ManifestSHA256 != proof.ManifestSHA256 || build.SourceCommit != proof.SourceCommit {
		return false, nil
	}
	var specRaw, resultRaw string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT n.spec,a.result FROM dag_nodes n JOIN dag_attempts a ON a.run_id=n.run_id AND a.node_id=n.id AND a.attempt=n.attempt AND a.fence=n.fence AND a.worker_id=n.worker_id WHERE n.run_id=? AND n.id=? AND n.attempt=? AND n.fence=? AND n.worker_id=?`), run, fixed.SourceNodeID, input.BuildAttempt, input.BuildFence, fixed.SourceWorkerID).Scan(&specRaw, &resultRaw); errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	var source dag.Node
	var result dag.Result
	if json.Unmarshal([]byte(specRaw), &source) != nil || json.Unmarshal([]byte(resultRaw), &result) != nil || source.Config["tool_profile_id"] != fixed.SourceToolProfileID || source.Config["tool_profile_sha256"] != fixed.SourceToolProfileSHA256 || len(source.Command) != 1 || source.Command[0] != build.ProfileID || result.ToolEvidence == nil || result.ToolEvidence.Profile.ProfileSHA256 != fixed.SourceToolProfileSHA256 || result.ToolEvidence.Profile.CommandID != build.ProfileID {
		return false, nil
	}
	return true, nil
}
