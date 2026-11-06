package platform

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"

	"github.com/ai-mada/suma/internal/dag"
)

type ReleaseCandidateView struct {
	Candidate   ReleaseCandidate `json:"candidate"`
	Eligibility bool             `json:"eligibility"`
	Blockers    []ReleaseBlocker `json:"blockers"`
}

func (s *Service) releaseTaskBasisTx(ctx context.Context, tx *sql.Tx, p Principal, project, id, requiredRun string) (ReleaseTaskBasis, error) {
	return s.releaseTaskBasisReadTx(ctx, tx, p, project, id, requiredRun, false)
}
func (s *Service) currentReleaseTaskBasisTx(ctx context.Context, tx *sql.Tx, p Principal, project, id, requiredRun string) (ReleaseTaskBasis, error) {
	return s.releaseTaskBasisReadTx(ctx, tx, p, project, id, requiredRun, true)
}
func (s *Service) releaseTaskBasisReadTx(ctx context.Context, tx *sql.Tx, p Principal, project, id, requiredRun string, current bool) (ReleaseTaskBasis, error) {
	var out ReleaseTaskBasis
	if err := s.checkTaskEvidenceReadTx(ctx, tx, p, project, id, map[string]bool{}, map[string]bool{}, current); err != nil {
		return out, err
	}
	task, err := scanRecord(tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`, current)), p.TenantID, project, id))
	if err != nil {
		return out, missing()
	}
	if task.RunID != requiredRun {
		return out, invalid("候选范围必须由本次构建和包测试所在运行实际交付并验收")
	}
	var reviewID string
	var reviewVersion int
	if err = tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT id,task_version FROM suma_reviews WHERE tenant_id=? AND project_id=? AND task_id=? AND run_id=? AND decision='accepted' ORDER BY created_at DESC,id DESC LIMIT 1`, current)), p.TenantID, project, id, task.RunID).Scan(&reviewID, &reviewVersion); err != nil || reviewVersion+1 != task.Version {
		return out, conflict("缺少对应版本的正式人工验收")
	}
	out = ReleaseTaskBasis{ID: id, Version: task.Version, RunID: task.RunID, ReviewID: reviewID, SHA256: digest(stringify(map[string]any{"task_id": id, "task_version": task.Version, "run_id": task.RunID, "review_id": reviewID, "definition": taskBasis(task), "requirement_version": task.Data["requirement_version"], "dependency_basis": task.Data["dependency_basis"]}))}
	return out, nil
}

func (s *Service) validateReleaseCandidateTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string) (ReleaseCandidate, error) {
	return s.validateReleaseCandidateReadTx(ctx, tx, p, project, id, false, false)
}
func (s *Service) validateCurrentReleaseCandidateTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string) (ReleaseCandidate, error) {
	return s.validateReleaseCandidateReadTx(ctx, tx, p, project, id, true, true)
}
func (s *Service) validateReleaseCandidateReadTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string, current, forward bool) (ReleaseCandidate, error) {
	var out ReleaseCandidate
	var raw string
	if err := tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_release_candidates WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id).Scan(&raw); err != nil {
		return out, missing()
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	build, err := s.artifactProofReadTx(ctx, tx, p, project, out.BuildAttestationID, current)
	if err != nil {
		return out, err
	}
	test, err := s.artifactProofReadTx(ctx, tx, p, project, out.PackageTestAttestationID, current)
	if err != nil {
		return out, err
	}
	if build.Kind != "build" || test.Kind != "package_test" || build.ArtifactID != out.ArtifactID || test.ArtifactID != out.ArtifactID || build.ManifestSHA256 != out.ArtifactSHA256 || test.ManifestSHA256 != out.ArtifactSHA256 || build.SourceCommit != out.SourceCommit || test.SourceCommit != out.SourceCommit || build.Assignment.RunID != test.Assignment.RunID {
		return out, conflict("候选构建和包测试依据不一致")
	}
	if _, err = s.releaseArtifactManifestTx(ctx, tx, p, project, out.ArtifactID); err != nil {
		return out, err
	}
	if len(out.Tasks) == 0 {
		return out, conflict("候选缺少已验收交付范围")
	}
	for _, basis := range out.Tasks {
		actual, err := s.releaseTaskBasisReadTx(ctx, tx, p, project, basis.ID, build.Assignment.RunID, current)
		if err != nil {
			return out, err
		}
		if actual != basis {
			return out, conflict("需求、任务、执行或人工验收依据已变化，需要重新建立候选")
		}
	}
	if digest(stringify(out.Tasks)) != out.AcceptedTaskBasisSHA256 {
		return out, conflict("候选范围摘要不一致")
	}
	if out.RepairHandoff != nil {
		if !out.RepairHandoff.matchesCandidate(out) {
			return out, releaseConflict("release_basis_changed", issueRepairReleaseStale)
		}
		if forward {
			actual, err := s.issueRepairReleaseHandoffTx(ctx, tx, p, project, out.RepairHandoff.IssueID, current)
			if err != nil {
				return out, err
			}
			if *actual != *out.RepairHandoff {
				return out, releaseConflict("release_basis_changed", issueRepairReleaseStale)
			}
		}
	}
	return out, nil
}

func (s *Service) CreateReleaseCandidate(ctx context.Context, actor Actor, project string, m map[string]any) (ReleaseCandidateView, error) {
	p := actor.Projection()

	var out ReleaseCandidateView
	var request struct {
		ProjectID                string `json:"project_id"`
		Name                     string `json:"name"`
		ArtifactID               string `json:"artifact_id"`
		BuildAttestationID       string `json:"build_attestation_id"`
		PackageTestAttestationID string `json:"package_test_attestation_id"`
		Tasks                    []struct {
			ID              string `json:"id"`
			ExpectedVersion int    `json:"expected_version"`
		} `json:"tasks"`
		IdempotencyKey string                          `json:"idempotency_key"`
		RepairHandoff  *IssueRepairReleaseHandoffInput `json:"repair_handoff,omitempty"`
	}
	if err := releaseDecode(m, &request); err != nil {
		return out, err
	}
	if _, supplied := m["repair_handoff"]; supplied {
		ref := request.RepairHandoff
		if ref == nil || !handoffID(ref.IssueID, false) || !designVersion(ref.ExpectedIssueVersion) ||
			!designVersion(ref.ExpectedRepairVersion) || ref.ExpectedIssueVersion < 1 || ref.ExpectedRepairVersion < 1 {
			return out, invalid(issueRepairInvalid)
		}
	}
	name := strings.TrimSpace(str(m, "name"))
	if name == "" || len(name) > 200 {
		return out, invalid("请填写 1–200 字节的候选名称")
	}
	drafts, ok := m["tasks"].([]any)
	if !ok || len(drafts) == 0 || len(drafts) > 100 {
		return out, invalid("请选择 1–100 个已验收任务作为交付范围")
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: p.TenantID, projectID: project}, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	p = auth.currentPrincipal
	if err = dag.GuardProjectTx(ctx, tx, s.Dialect, p.TenantID, project); err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, str(m, "idempotency_key"), map[string]any{"release_candidate": m})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		return s.currentCandidateViewTx(ctx, tx, p, project, out.Candidate.ID)
	}
	var handoff *IssueRepairReleaseHandoff
	if ref := request.RepairHandoff; ref != nil {
		handoff, err = s.issueRepairReleaseHandoffTx(ctx, tx, p, project, ref.IssueID, true)
		if err != nil {
			return out, err
		}
		if handoff.IssueVersion != ref.ExpectedIssueVersion || handoff.RepairVersion != ref.ExpectedRepairVersion ||
			handoff.ArtifactID != request.ArtifactID || handoff.BuildAttestationID != request.BuildAttestationID ||
			handoff.PackageTestAttestationID != request.PackageTestAttestationID || len(request.Tasks) != 1 ||
			handoff.TaskID != request.Tasks[0].ID || handoff.TaskVersion != request.Tasks[0].ExpectedVersion {
			return out, staleIssueRepairReleaseBasis(nil)
		}
	}
	build, err := s.currentArtifactProofTx(ctx, tx, p, project, str(m, "build_attestation_id"))
	if err != nil {
		return out, err
	}
	test, err := s.currentArtifactProofTx(ctx, tx, p, project, str(m, "package_test_attestation_id"))
	if err != nil {
		return out, err
	}
	artifact := str(m, "artifact_id")
	if build.Kind != "build" || test.Kind != "package_test" || build.ArtifactID != artifact || test.ArtifactID != artifact || build.ManifestSHA256 != test.ManifestSHA256 || build.SourceCommit != test.SourceCommit || build.Assignment.RunID != test.Assignment.RunID {
		return out, invalid("请选择同一已封存制品及其真实构建、包测试证明")
	}
	if _, err = s.releaseArtifactManifestTx(ctx, tx, p, project, artifact); err != nil {
		return out, err
	}
	bases := []ReleaseTaskBasis{}
	seen := map[string]bool{}
	for _, raw := range drafts {
		d, ok := raw.(map[string]any)
		if !ok {
			return out, invalid("任务版本格式无效")
		}
		if err = exactVersion(d, "expected_version", 1); err != nil {
			return out, err
		}
		id := str(d, "id")
		if seen[id] {
			return out, invalid("交付范围有重复任务")
		}
		seen[id] = true
		basis, err := s.currentReleaseTaskBasisTx(ctx, tx, p, project, id, build.Assignment.RunID)
		if err != nil {
			return out, err
		}
		if basis.Version != number(d, "expected_version") {
			return out, conflict("任务版本已变化，请刷新候选范围")
		}
		bases = append(bases, basis)
	}
	sort.Slice(bases, func(i, j int) bool { return bases[i].ID < bases[j].ID })
	candidate := ReleaseCandidate{ID: newID(), ProjectID: project, Name: name, ArtifactID: artifact, ArtifactSHA256: build.ManifestSHA256, SourceCommit: build.SourceCommit, BuildAttestationID: build.ID, PackageTestAttestationID: test.ID, AcceptedTaskBasisSHA256: digest(stringify(bases)), BuildProof: build, PackageTestProof: test, Tasks: bases, CreatedAt: now()}
	if handoff != nil {
		if !handoff.matchesCandidate(candidate) {
			return out, staleIssueRepairReleaseBasis(nil)
		}
		candidate.RepairHandoff = handoff
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_release_candidates(id,tenant_id,project_id,name,payload,created_at)VALUES(?,?,?,?,?,?)`), candidate.ID, p.TenantID, project, name, stringify(candidate), candidate.CreatedAt); err != nil {
		return out, err
	}
	out = ReleaseCandidateView{Candidate: candidate, Eligibility: true, Blockers: []ReleaseBlocker{}}
	if err = s.saveCommandReceipt(ctx, auth, str(m, "idempotency_key"), hash, out); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "release_candidate.created", candidate.ID, name); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) candidateViewTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string) (ReleaseCandidateView, error) {
	return s.candidateViewReadTx(ctx, tx, p, project, id, false)
}
func (s *Service) currentCandidateViewTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string) (ReleaseCandidateView, error) {
	return s.candidateViewReadTx(ctx, tx, p, project, id, true)
}
func (s *Service) candidateViewReadTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string, current bool) (ReleaseCandidateView, error) {
	var out ReleaseCandidateView
	var raw string
	if err := tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_release_candidates WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id).Scan(&raw); err != nil {
		return out, missing()
	}
	if err := json.Unmarshal([]byte(raw), &out.Candidate); err != nil {
		return out, err
	}
	out.Blockers = []ReleaseBlocker{}
	_, err := s.validateReleaseCandidateReadTx(ctx, tx, p, project, id, current, true)
	out.Eligibility = err == nil
	if err != nil {
		if api, ok := err.(*APIError); ok {
			out.Blockers = append(out.Blockers, ReleaseBlocker{Code: api.Code, Message: api.Message})
		} else {
			return out, err
		}
	}
	return out, nil
}
func (s *Service) ReleaseCandidate(ctx context.Context, actor Actor, project, id string) (ReleaseCandidateView, error) {
	var result0 ReleaseCandidateView
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, e = s.readReleaseCandidateTx(bounded, auth, project, id)
		return e
	})
	return result0, err
}
func (s *Service) readReleaseCandidateTx(ctx context.Context, auth *authorizedCommand, project, id string) (ReleaseCandidateView, error) {
	if auth == nil {
		return ReleaseCandidateView{}, unauthenticated()
	}
	if e := auth.assertTx(auth.tx, auth.target, auth.purpose); e != nil {
		return ReleaseCandidateView{}, e
	}
	if auth.target.projectID != project {
		return ReleaseCandidateView{}, forbidden()
	}

	tx, p := auth.tx, auth.currentPrincipal

	return s.candidateViewTx(ctx, tx, p, project, id)

}

func (s *Service) releasePageIDs(ctx context.Context, tx *sql.Tx, p Principal, project, kind string, limit int, cursor string) ([]string, int, string, error) {
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return nil, 0, "", invalid("limit 应为 1–100")
	}
	table := "suma_release_artifacts"
	if kind == "release-candidates" {
		table = "suma_release_candidates"
	} else if kind != "artifacts" {
		return nil, 0, "", invalid("分页对象无效")
	}
	var c recordCursor
	if cursor != "" {
		b, err := base64.RawURLEncoding.DecodeString(cursor)
		if len(cursor) > 4096 || err != nil || json.Unmarshal(b, &c) != nil || c.Tenant != p.TenantID || c.Project != project || c.Kind != kind || c.ID == "" || c.Updated == "" {
			return nil, 0, "", invalid("分页游标无效或不属于当前项目")
		}
	}
	var total int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM `+table+` WHERE tenant_id=? AND project_id=?`), p.TenantID, project).Scan(&total); err != nil {
		return nil, 0, "", err
	}
	query := `SELECT id,created_at FROM ` + table + ` WHERE tenant_id=? AND project_id=?`
	args := []any{p.TenantID, project}
	if cursor != "" {
		query += ` AND (created_at<? OR (created_at=? AND id>?))`
		args = append(args, c.Updated, c.Updated, c.ID)
	}
	query += ` ORDER BY created_at DESC,id ASC LIMIT ?`
	args = append(args, limit+1)
	rows, err := tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return nil, 0, "", err
	}
	var ids, dates []string
	for rows.Next() {
		var id, date string
		if err = rows.Scan(&id, &date); err != nil {
			rows.Close()
			return nil, 0, "", err
		}
		ids = append(ids, id)
		dates = append(dates, date)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, "", err
	}
	next := ""
	if len(ids) > limit {
		ids = ids[:limit]
		next = base64.RawURLEncoding.EncodeToString([]byte(stringify(recordCursor{p.TenantID, project, kind, dates[limit-1], ids[limit-1]})))
	}
	return ids, total, next, nil
}
func (s *Service) ReleaseCandidates(ctx context.Context, actor Actor, project string, limit int, cursor string) ([]ReleaseCandidateView, int, string, error) {
	var result0 []ReleaseCandidateView
	var result1 int
	var result2 string
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, result1, result2, e = s.readReleaseCandidatesTx(bounded, auth, project, limit, cursor)
		return e
	})
	return result0, result1, result2, err
}
func (s *Service) readReleaseCandidatesTx(ctx context.Context, auth *authorizedCommand, project string, limit int, cursor string) ([]ReleaseCandidateView, int, string, error) {
	if auth == nil {
		return nil, 0, "", unauthenticated()
	}
	if e := auth.assertTx(auth.tx, auth.target, auth.purpose); e != nil {
		return nil, 0, "", e
	}
	if auth.target.projectID != project {
		return nil, 0, "", forbidden()
	}

	tx, p := auth.tx, auth.currentPrincipal
	var err error

	ids, total, next, err := s.releasePageIDs(ctx, tx, p, project, "release-candidates", limit, cursor)
	if err != nil {
		return nil, 0, "", err
	}
	out := []ReleaseCandidateView{}
	for _, id := range ids {
		v, err := s.candidateViewTx(ctx, tx, p, project, id)
		if err != nil {
			return nil, 0, "", err
		}
		out = append(out, v)
	}
	return out, total, next, nil

}

func (s *Service) releaseArtifactTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string) (ReleaseArtifact, error) {
	var out ReleaseArtifact
	var raw string
	err := tx.QueryRowContext(ctx, s.q(`SELECT id,project_id,manifest_sha256,manifest,size_bytes,created_at FROM suma_release_artifacts WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id).Scan(&out.ID, &out.ProjectID, &out.ManifestSHA256, &raw, &out.SizeBytes, &out.CreatedAt)
	if err != nil {
		return out, missing()
	}
	if err = json.Unmarshal([]byte(raw), &out.Manifest); err != nil {
		return out, err
	}
	out.State = "sealed"
	out.Proofs = []ArtifactProof{}
	// Keep historical proof visibility independent of current execution eligibility.
	columns := "p." + strings.ReplaceAll(artifactProofColumns, ",", ",p.")
	rows, err := tx.QueryContext(ctx, s.q(`SELECT `+columns+`,COALESCE(n.spec,''),COALESCE(a.result,'')
 FROM suma_artifact_proofs p
 LEFT JOIN dag_nodes n ON p.kind='build' AND n.run_id=p.run_id AND n.id=p.node_id AND n.spec LIKE '%"application_build"%'
 LEFT JOIN dag_attempts a ON n.run_id=a.run_id AND n.id=a.node_id AND a.attempt=p.attempt AND a.fence=p.fence AND a.worker_id=p.worker_id
 WHERE p.tenant_id=? AND p.project_id=? AND p.artifact_id=? ORDER BY p.created_at DESC,p.id`), p.TenantID, project, id)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var spec, result string
		proof, err := scanArtifactProof(rows, &spec, &result)
		if err == nil && proof.Kind == "build" {
			proof.ApplicationBuild, err = decodeApplicationBuildProof(spec, result, proof)
		}
		if err != nil {
			rows.Close()
			return out, err
		}
		out.Proofs = append(out.Proofs, proof)
	}
	err = rows.Err()
	rows.Close()
	return out, err
}
func (s *Service) ReleaseArtifact(ctx context.Context, actor Actor, project, id string) (ReleaseArtifact, error) {
	var result0 ReleaseArtifact
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, e = s.readReleaseArtifactTx(bounded, auth, project, id)
		return e
	})
	return result0, err
}
func (s *Service) readReleaseArtifactTx(ctx context.Context, auth *authorizedCommand, project, id string) (ReleaseArtifact, error) {
	if auth == nil {
		return ReleaseArtifact{}, unauthenticated()
	}
	if e := auth.assertTx(auth.tx, auth.target, auth.purpose); e != nil {
		return ReleaseArtifact{}, e
	}
	if auth.target.projectID != project {
		return ReleaseArtifact{}, forbidden()
	}

	tx, p := auth.tx, auth.currentPrincipal

	return s.releaseArtifactTx(ctx, tx, p, project, id)

}

func (s *Service) ReleaseArtifacts(ctx context.Context, actor Actor, project string, limit int, cursor string) ([]ReleaseArtifact, int, string, error) {
	var result0 []ReleaseArtifact
	var result1 int
	var result2 string
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, result1, result2, e = s.readReleaseArtifactsTx(bounded, auth, project, limit, cursor)
		return e
	})
	return result0, result1, result2, err
}
func (s *Service) readReleaseArtifactsTx(ctx context.Context, auth *authorizedCommand, project string, limit int, cursor string) ([]ReleaseArtifact, int, string, error) {
	if auth == nil {
		return nil, 0, "", unauthenticated()
	}
	if e := auth.assertTx(auth.tx, auth.target, auth.purpose); e != nil {
		return nil, 0, "", e
	}
	if auth.target.projectID != project {
		return nil, 0, "", forbidden()
	}

	tx, p := auth.tx, auth.currentPrincipal
	var err error

	ids, total, next, err := s.releasePageIDs(ctx, tx, p, project, "artifacts", limit, cursor)
	if err != nil {
		return nil, 0, "", err
	}
	out := []ReleaseArtifact{}
	for _, id := range ids {
		a, err := s.releaseArtifactTx(ctx, tx, p, project, id)
		if err != nil {
			return nil, 0, "", err
		}
		out = append(out, a)
	}
	return out, total, next, nil

}
