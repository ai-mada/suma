package platform

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

const artifactProofColumns = `id,project_id,run_id,node_id,attempt,fence,worker_id,kind,artifact_id,manifest_sha256,source_commit,profile_id,profile_sha256,workspace_sha256,created_at`

func scanArtifactProof(row interface{ Scan(...any) error }, extra ...any) (ArtifactProof, error) {
	var p ArtifactProof
	dest := []any{&p.ID, &p.ProjectID, &p.Assignment.RunID, &p.Assignment.NodeID, &p.Assignment.Attempt, &p.Assignment.Fence, &p.Assignment.WorkerID, &p.Kind, &p.ArtifactID, &p.ManifestSHA256, &p.SourceCommit, &p.ProfileID, &p.ProfileSHA256, &p.WorkspaceSHA256, &p.CreatedAt}
	err := row.Scan(append(dest, extra...)...)
	return p, err
}

// Application build provenance identifies the registered source scope and its
// inspected Git tree. A shared commit alone cannot identify the application.
func applicationBuildMatchesVersion(basis *rc.ApplicationBuildBasis, v ApplicationCodeVersion) bool {
	if basis == nil || v.ApplicationID != basis.ApplicationID || v.ID != basis.VersionID || v.SnapshotSHA256 != basis.VersionSHA256 || len(v.Sources) != 1 {
		return false
	}
	source := v.Sources[0]
	return source.Scope.ID == basis.ScopeID && source.Scope.RelativeRoot == basis.ScopeRoot &&
		source.Inspection.ScopeTreeOID == basis.ScopeTreeOID && source.Commit == basis.SourceCommit &&
		source.ReceiptSHA256 == basis.ReceiptSHA256
}

func (s *Service) checkApplicationBuildBasisTx(ctx context.Context, tx *sql.Tx, p Principal, project string, basis *rc.ApplicationBuildBasis) error {
	if basis == nil {
		return nil
	}
	v, err := s.applicationVersionReadTx(ctx, tx, p, project, basis.ApplicationID, basis.VersionID)
	if err != nil {
		return err
	}
	if !applicationBuildMatchesVersion(basis, v) {
		return conflict("应用构建来源与固定源码版本不一致")
	}
	view, err := s.applicationVersionViewReadTx(ctx, tx, p, v)
	if err != nil {
		return err
	}
	if !view.BasisCurrent {
		return conflict("应用构建的当前授权或源码依据已变化")
	}
	return nil
}

// Both the frozen graph and the final Runner receipt must agree. A package
// test attests sealed bytes and cannot add or remove application provenance.
func checkApplicationBuildEvidence(spec dag.Node, e *rc.ArtifactEvidence) (*rc.ApplicationBuildBasis, error) {
	basis, err := rc.DecodeApplicationBuild(spec.Config)
	if err != nil {
		return nil, conflict("固定应用构建配置无效")
	}
	if basis == nil {
		if e != nil && e.ApplicationBuild != nil {
			return nil, conflict("应用构建证明缺少固定执行配置")
		}
		return nil, nil
	}
	if spec.Type != "command" || e == nil || e.Kind != "build" || e.ApplicationBuild == nil || *e.ApplicationBuild != *basis || e.SourceCommit != basis.SourceCommit {
		return nil, conflict("应用构建证明与固定执行配置不一致")
	}
	return basis, nil
}

func artifactFinalResultMatchesProof(result dag.Result, proof ArtifactProof) bool {
	e := result.ArtifactEvidence
	return e != nil && result.ExitCode == 0 && !result.Stopped && !result.OutcomeUnknown && result.Error == "" &&
		e.ArtifactID == proof.ArtifactID && e.ManifestSHA256 == proof.ManifestSHA256 && e.SourceCommit == proof.SourceCommit &&
		e.Kind == proof.Kind && e.ProfileID == proof.ProfileID && e.ProfileSHA256 == proof.ProfileSHA256 &&
		result.WorkspaceEvidence["evidence_sha256"] == proof.WorkspaceSHA256
}

func applicationBuildFromFinalResult(spec dag.Node, result dag.Result, proof ArtifactProof) (*rc.ApplicationBuildBasis, error) {
	basis, err := checkApplicationBuildEvidence(spec, result.ArtifactEvidence)
	if err != nil || basis == nil {
		return basis, err
	}
	if artifactRef(result) != proof.Assignment {
		return nil, conflict("应用构建证明与最终执行身份不一致")
	}
	return basis, nil
}

// Artifact lists retain ordinary historical rows even when older executions
// have no final receipt to join. Scoped provenance is projected only when the
// frozen node, final receipt and stored proof all identify the same build.
func decodeApplicationBuildProof(specRaw, resultRaw string, proof ArtifactProof) (*rc.ApplicationBuildBasis, error) {
	var spec dag.Node
	var result dag.Result
	if json.Unmarshal([]byte(specRaw), &spec) != nil || json.Unmarshal([]byte(resultRaw), &result) != nil {
		return nil, nil
	}
	if _, scoped := spec.Config[rc.ApplicationBuildConfigKey]; !scoped && (result.ArtifactEvidence == nil || result.ArtifactEvidence.ApplicationBuild == nil) {
		return nil, nil
	}
	if !artifactFinalResultMatchesProof(result, proof) {
		return nil, conflict("应用构建证明与最终回执不一致")
	}
	return applicationBuildFromFinalResult(spec, result, proof)
}

func (s *Service) verifyArtifactResultTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, spec dag.Node, result dag.Result, state string) error {
	if auth == nil {
		return unauthenticated()
	}
	if err := auth.assertTx(tx, auth.target, purposeRunnerWrite); err != nil {
		return err
	}
	p, project := auth.currentPrincipal, auth.target.projectID

	e := result.ArtifactEvidence
	if e != nil {
		if err := s.releaseWorkerBindingTx(ctx, tx, auth, result.WorkerID, e.ProfileID, e.ProfileSHA256, e.Kind); err != nil {
			return err
		}
	}
	// Failed or uncertain attempts retain observations, never publication proof.
	if state != "succeeded" {
		return nil
	}
	basis, err := checkApplicationBuildEvidence(spec, e)
	if err != nil || e == nil {
		return err
	}
	if err := s.checkApplicationBuildBasisTx(ctx, tx, p, project, basis); err != nil {
		return err
	}
	if result.Stopped || result.OutcomeUnknown || result.ExitCode != 0 || result.Error != "" {
		return invalid("停止或结果未知的执行不能形成成功制品证明")
	}
	head, err := workspaceRevision(result.WorkspaceEvidence)
	if err != nil {
		return err
	}
	if !rc.ValidCommit(e.SourceCommit) || e.SourceCommit != head || result.WorkspaceEvidence["base_commit"] != head || !rc.ValidDigest(e.ManifestSHA256) {
		return invalid("制品证明必须来自同一固定源码，构建及包测试不得改变代码")
	}
	if len(spec.Command) != 1 || spec.Command[0] != e.ProfileID {
		return invalid("命令与登记的制品配置不符")
	}
	manifest, err := s.releaseArtifactManifestTx(ctx, tx, p, project, e.ArtifactID)
	if err != nil {
		return err
	}
	_, hash, err := rc.CanonicalManifest(manifest)
	if err != nil || hash != e.ManifestSHA256 {
		return conflict("实际封存制品与执行回执不一致")
	}
	switch e.Kind {
	case "build":
		if spec.Type != "command" && spec.Type != "agent" {
			return invalid("构建证明需来自明确构建命令")
		}
		var uploadID string
		if err = tx.QueryRowContext(ctx, s.q(`SELECT upload_id FROM suma_release_artifacts WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, e.ArtifactID).Scan(&uploadID); err != nil {
			return err
		}
		u, err := s.loadArtifactUpload(ctx, tx, p, project, uploadID)
		if err != nil {
			return err
		}
		if u.State != "sealed" || u.Assignment != artifactRef(result) || u.TokenHash != auth.actor.credential.digest || u.ProfileID != e.ProfileID || u.ProfileSHA256 != e.ProfileSHA256 {
			return conflict("制品不属于本次构建身份或固定配置")
		}
	case "package_test":
		if spec.Type != "test" || result.WorkspaceEvidence["test_command"] != true {
			return invalid("包测试必须使用管理员固定的测试命令")
		}
		var binding string
		if err = tx.QueryRowContext(ctx, s.q(`SELECT binding FROM suma_artifact_bindings WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=?`), p.TenantID, project, result.RunID, result.NodeID, result.Attempt, result.Fence, result.WorkerID).Scan(&binding); err != nil {
			return conflict("缺少本次测试冻结的制品输入")
		}
		var b rc.ArtifactBinding
		if json.Unmarshal([]byte(binding), &b) != nil || b.ArtifactID != e.ArtifactID || b.ManifestSHA256 != e.ManifestSHA256 || b.SourceCommit != e.SourceCommit {
			return conflict("测试制品、来源代码与冻结输入不一致")
		}
		var buildID string
		if err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_artifact_proofs WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND kind='build' AND artifact_id=?`), p.TenantID, project, b.BuildRunID, b.BuildNodeID, b.BuildAttempt, b.BuildFence, b.ArtifactID).Scan(&buildID); err != nil {
			return conflict("原构建证明不存在")
		}
		if _, err = s.currentArtifactProofTx(ctx, tx, p, project, buildID); err != nil {
			return err
		}
	default:
		return invalid("未知制品证明种类")
	}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_artifact_proofs(id,tenant_id,project_id,run_id,node_id,attempt,fence,worker_id,kind,artifact_id,manifest_sha256,source_commit,profile_id,profile_sha256,workspace_sha256,created_at)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), newID(), p.TenantID, project, result.RunID, result.NodeID, result.Attempt, result.Fence, result.WorkerID, e.Kind, e.ArtifactID, e.ManifestSHA256, e.SourceCommit, e.ProfileID, e.ProfileSHA256, result.WorkspaceEvidence["evidence_sha256"], now())
	return err
}

// A persisted proof remains historical; eligibility requires the exact current
// successful attempt and its immutable final result. Generic reconcile has no
// result/receipt from the trusted attester and cannot manufacture this fact.
func (s *Service) artifactProofTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string) (ArtifactProof, error) {
	return s.artifactProofReadTx(ctx, tx, p, project, id, false)
}

func (s *Service) currentArtifactProofTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string) (ArtifactProof, error) {
	return s.artifactProofReadTx(ctx, tx, p, project, id, true)
}

func (s *Service) artifactProofReadTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string, current bool) (ArtifactProof, error) {
	proof, err := scanArtifactProof(tx.QueryRowContext(ctx, s.q(`SELECT `+artifactProofColumns+` FROM suma_artifact_proofs WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id))
	if err != nil {
		return proof, missing()
	}
	var state, currentState, raw, rawSpec string
	var attempt int
	var fence int64
	err = tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT a.state,n.state,n.attempt,n.fence,a.result,n.spec FROM dag_attempts a JOIN dag_nodes n ON n.run_id=a.run_id AND n.id=a.node_id JOIN dag_runs r ON r.id=a.run_id WHERE r.tenant_id=? AND r.project_id=? AND a.run_id=? AND a.node_id=? AND a.attempt=? AND a.fence=? AND a.worker_id=?`, current)), p.TenantID, project, proof.Assignment.RunID, proof.Assignment.NodeID, proof.Assignment.Attempt, proof.Assignment.Fence, proof.Assignment.WorkerID).Scan(&state, &currentState, &attempt, &fence, &raw, &rawSpec)
	if err != nil || state != "succeeded" || currentState != "succeeded" || attempt != proof.Assignment.Attempt || fence != proof.Assignment.Fence {
		return proof, conflict("构建或包测试当前尝试已变化")
	}
	var result dag.Result
	if json.Unmarshal([]byte(raw), &result) != nil || result.ArtifactEvidence == nil || result.ExitCode != 0 || result.Stopped || result.OutcomeUnknown || result.Error != "" {
		return proof, conflict("缺少可信执行的最终制品回执")
	}
	if !artifactFinalResultMatchesProof(result, proof) {
		return proof, conflict("制品证明与最终回执不一致")
	}
	var spec dag.Node
	if json.Unmarshal([]byte(rawSpec), &spec) != nil {
		return proof, conflict("缺少固定制品执行配置")
	}
	proof.ApplicationBuild, err = applicationBuildFromFinalResult(spec, result, proof)
	if err != nil {
		return proof, err
	}
	if current {
		if err = s.checkApplicationBuildBasisTx(ctx, tx, p, project, proof.ApplicationBuild); err != nil {
			return proof, err
		}
	}
	return proof, nil
}

func (s *Service) ResolveArtifactInput(ctx context.Context, actor Actor, project string, r rc.ArtifactResolveRequest) (rc.ArtifactBinding, error) {
	p := actor.Projection()

	var out rc.ArtifactBinding
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.releaseWorkerTx(ctx, tx, actor, project, r.Assignment.WorkerID, "", "", "identity")
	if err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, r.OperationID, map[string]any{"resolve_artifact": r})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	spec, err := s.currentArtifactAssignmentTx(ctx, tx, p, project, r.Assignment)
	if err != nil {
		return out, err
	}
	if spec.Type != "test" || spec.Config["artifact_from"] != r.SourceNodeID {
		return out, invalid("制品输入仅能取自本次测试的明确构建祖先")
	}
	if len(spec.Command) != 1 {
		return out, invalid("包测试需要唯一的管理员测试配置")
	}
	var registered string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT profiles FROM suma_release_workers WHERE token_hash=? AND tenant_id=? AND project_id=?`), actor.credential.digest, p.TenantID, project).Scan(&registered); err != nil {
		return out, forbidden()
	}
	var profiles []ReleaseProfileRegistration
	if json.Unmarshal([]byte(registered), &profiles) != nil {
		return out, forbidden()
	}
	allowed := false
	for _, pr := range profiles {
		if pr.ID == spec.Command[0] && pr.Kind == "package_test" {
			allowed = true
		}
	}
	if !allowed {
		return out, forbidden()
	}
	var raw string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT w.spec FROM dag_workflows w JOIN dag_runs r ON r.tenant_id=w.tenant_id AND r.project_id=w.project_id AND r.workflow_id=w.id AND r.workflow_version=w.version WHERE r.id=?`), r.Assignment.RunID).Scan(&raw); err != nil {
		return out, err
	}
	var wf dag.Workflow
	if json.Unmarshal([]byte(raw), &wf) != nil {
		return out, invalid("流程定义无效")
	}
	parents := map[string][]string{}
	types := map[string]string{}
	for _, n := range wf.Nodes {
		types[n.ID] = n.Type
	}
	for _, edge := range wf.Edges {
		parents[edge.To] = append(parents[edge.To], edge.From)
	}
	seen := map[string]bool{}
	queue := append([]string(nil), parents[r.Assignment.NodeID]...)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		queue = append(queue, parents[id]...)
	}
	if !seen[r.SourceNodeID] || types[r.SourceNodeID] != "command" && types[r.SourceNodeID] != "agent" {
		return out, invalid("制品来源必须是流程的静态构建祖先")
	}
	var proofID string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT a.id FROM suma_artifact_proofs a JOIN dag_nodes n ON n.run_id=a.run_id AND n.id=a.node_id AND n.attempt=a.attempt AND n.fence=a.fence WHERE a.tenant_id=? AND a.project_id=? AND a.run_id=? AND a.node_id=? AND a.kind='build'`), p.TenantID, project, r.Assignment.RunID, r.SourceNodeID).Scan(&proofID); err != nil {
		return out, conflict("上游尚无可用构建证明")
	}
	proof, err := s.currentArtifactProofTx(ctx, tx, p, project, proofID)
	if err != nil {
		return out, err
	}
	manifest, err := s.releaseArtifactManifestTx(ctx, tx, p, project, proof.ArtifactID)
	if err != nil {
		return out, err
	}
	out = rc.ArtifactBinding{ArtifactID: proof.ArtifactID, ManifestSHA256: proof.ManifestSHA256, SourceCommit: proof.SourceCommit, BuildRunID: proof.Assignment.RunID, BuildNodeID: proof.Assignment.NodeID, BuildAttempt: proof.Assignment.Attempt, BuildFence: proof.Assignment.Fence, Manifest: manifest}
	var previous string
	err = tx.QueryRowContext(ctx, s.q(`SELECT binding FROM suma_artifact_bindings WHERE run_id=? AND node_id=? AND attempt=?`), r.Assignment.RunID, r.Assignment.NodeID, r.Assignment.Attempt).Scan(&previous)
	if err == nil {
		return out, conflict("测试已固定制品，请恢复原解析操作")
	}
	if err != sql.ErrNoRows {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_artifact_bindings(tenant_id,project_id,run_id,node_id,attempt,fence,worker_id,source_node_id,binding)VALUES(?,?,?,?,?,?,?,?,?)`), p.TenantID, project, r.Assignment.RunID, r.Assignment.NodeID, r.Assignment.Attempt, r.Assignment.Fence, r.Assignment.WorkerID, r.SourceNodeID, stringify(out)); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, r.OperationID, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) RunnerArtifactBlob(ctx context.Context, actor Actor, project, id, hash string, a rc.AssignmentRef) ([]byte, error) {
	var ref artifactReadRef
	target := commandTarget{tenantID: actor.credential.tenantID, projectID: project}
	read := func(auth *authorizedCommand) error {
		tx, p := auth.tx, auth.currentPrincipal
		if err := s.releaseWorkerBindingTx(ctx, tx, auth, a.WorkerID, "", "", "identity"); err != nil {
			return err
		}
		var err error
		if _, err = s.currentArtifactAssignmentTx(ctx, tx, p, project, a); err != nil {
			return err
		}
		var binding string
		if err = tx.QueryRowContext(ctx, s.q(`SELECT binding FROM suma_artifact_bindings WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=?`), p.TenantID, project, a.RunID, a.NodeID, a.Attempt, a.Fence, a.WorkerID).Scan(&binding); err != nil {
			return forbidden()
		}
		var b rc.ArtifactBinding
		if json.Unmarshal([]byte(binding), &b) != nil || b.ArtifactID != id {
			return forbidden()
		}
		ref, err = s.artifactBlobReferenceTx(ctx, tx, p, project, id, hash)
		return err

	}
	if err := s.withCurrentRead(ctx, actor, target, purposeRunnerRead, read); err != nil {
		return nil, err
	}
	initial := ref
	b, err := s.readArtifactBlob(ref.path, ref.hash, ref.size)
	if err != nil {
		return nil, err
	}
	if err = s.withCurrentRead(ctx, actor, target, purposeRunnerRead, read); err != nil {
		return nil, err
	}
	if ref != initial {
		return nil, conflict("制品读取依据已变化，请重试")
	}
	return b, nil
}
