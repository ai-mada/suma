package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func (s *Service) loadRepositorySourceIntakeTx(ctx context.Context, tx *sql.Tx, tenant, project, task, id string) (out repositorySourceIntakeRecord, path string, err error) {
	var raw, run string
	err = tx.QueryRowContext(ctx, s.q(s.currentSQL("SELECT storage_path,data,run_id FROM suma_repository_source_intakes WHERE tenant_id=? AND project_id=? AND task_id=? AND id=?")), tenant, project, task, id).Scan(&path, &raw, &run)
	if errors.Is(err, sql.ErrNoRows) {
		return out, "", missing()
	}
	if err != nil {
		return out, "", err
	}
	if json.Unmarshal([]byte(raw), &out) != nil {
		return out, "", conflict("Retained repository source changed")
	}
	r := out.Receipt
	requestSHA, err := ri.RepositorySourceExportDigest(out.Export)
	if err != nil || requestSHA != out.RequestSHA256 || !storageBlobPath.MatchString(path) || !out.Origin.Valid() ||
		out.ApprovedBy == "" || !validReleaseTime(r.ApprovedAt) || r.ID != id || r.ProjectID != project || r.TaskID != task ||
		r.RunID != run || r.RunID != out.Origin.RunID || r.NodeID != out.Origin.NodeID || r.Attempt != out.Origin.Attempt ||
		out.Export.TenantID != tenant || out.Export.ProjectID != project || out.Export.ExpectedBase != r.BaseCommit ||
		out.Export.ExpectedHead != r.HeadCommit || !cc.ValidHash(r.BundleSHA256) || r.SizeBytes <= 0 || r.SizeBytes > cc.MaxBundleBytes {
		return out, "", conflict("Retained repository source changed")
	}
	return out, path, nil
}

func repositoryExecutionMatchesReceipt(source ri.RepositorySourceExecution, r TaskDeliveryRepositoryIntake) bool {
	return source.IntakeID == r.ID && source.TaskID == r.TaskID && source.TaskVersion == r.TaskVersion &&
		source.BaseCommit == r.BaseCommit && source.HeadCommit == r.HeadCommit && source.BundleSHA256 == r.BundleSHA256 && source.SizeBytes == r.SizeBytes
}

func (s *Service) repositorySourceNodeEligibleTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string, spec dag.Node) (bool, error) {
	raw, declared := spec.Config[ri.RepositorySourceExecutionKey]
	if !declared {
		return true, nil
	}
	source, err := s.repositorySourceExecutionTx(ctx, tx, tenant, project, run, node)
	if err != nil || source == nil {
		return false, err
	}
	parsed, err := ri.DecodeRepositorySourceExecution(raw)
	if err != nil || *parsed != *source || spec.Type != "command" || len(spec.Command) != 1 || spec.MaxRetries != 0 || spec.Idempotent {
		return false, nil
	}
	for _, name := range []string{"candidate_from", "candidate_composition", "candidate_conflict_repair", "code_baseline", "terminal_candidate_source", "development_continuation_id", "development_continuation", "model_execution", "codex_execution", "opencode_execution", "octop_execution", "execution_skills"} {
		if _, present := spec.Config[name]; present {
			return false, nil
		}
	}
	return true, nil
}

func (s *Service) currentRepositorySourceTaskTx(ctx context.Context, tx *sql.Tx, tenant, project, run string, source ri.RepositorySourceExecution) error {
	var bound string
	err := tx.QueryRowContext(ctx, s.q("SELECT run_id FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?"),
		tenant, project, source.TaskID).Scan(&bound)
	if errors.Is(err, sql.ErrNoRows) || err == nil && bound != run {
		return conflict("Repository candidate is no longer bound to the task")
	}
	return err
}

func validateRepositorySourceEvidence(proof map[string]any, expected *ri.RepositorySourceExecution) error {
	raw, declared := proof[ri.RepositorySourceExecutionKey]
	if expected == nil {
		if declared {
			return forbidden()
		}
		return nil
	}
	actual, err := ri.DecodeRepositorySourceExecution(raw)
	if err != nil || *actual != *expected || proof["head_commit"] != expected.HeadCommit ||
		proof["base_commit"] != expected.BaseCommit || proof["admin_base_commit"] != expected.BaseCommit {
		return conflict("Repository candidate evidence differs from the approved source")
	}
	return nil
}

func (s *Service) validateRepositorySourceResultTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) error {
	source, err := s.repositorySourceExecutionTx(ctx, tx, tenant, project, result.RunID, result.NodeID)
	if err != nil {
		return err
	}
	if source == nil {
		return validateRepositorySourceEvidence(result.WorkspaceEvidence, nil)
	}
	if result.OutcomeUnknown || result.Stopped || result.ExitCode != 0 || result.Error != "" {
		return nil
	}
	if err = s.currentRepositorySourceTaskTx(ctx, tx, tenant, project, result.RunID, *source); err != nil {
		return err
	}
	if err = validateRepositorySourceEvidence(result.WorkspaceEvidence, source); err != nil {
		return err
	}
	// Require the original approved bytes to have an actual sealed transfer.
	// A Runner's result payload cannot create this fact by naming a bundle.
	var reported cc.Binding
	if json.Unmarshal([]byte(stringify(result.WorkspaceEvidence["candidate_bundle"])), &reported) != nil || !reported.ValidSealed() ||
		reported.Source.RunID != result.RunID || reported.Source.NodeID != result.NodeID || reported.Source.Attempt != result.Attempt ||
		reported.Source.Fence != result.Fence || reported.Source.WorkerID != result.WorkerID ||
		reported.BaseCommit != source.BaseCommit || reported.BundleBase() != source.BaseCommit || reported.HeadCommit != source.HeadCommit ||
		reported.BundleSHA256 != source.BundleSHA256 || reported.SizeBytes != source.SizeBytes || reported.EvidenceSHA256 != result.WorkspaceEvidence["evidence_sha256"] {
		return conflict("Repository candidate has no sealed transfer for the approved source")
	}
	transfer, err := s.candidateTransferTx(ctx, tx, candidateSubject{tenant: tenant, project: project}, reported.TransferID)
	if err != nil {
		return err
	}
	if transfer.binding != reported || !storageBlobPath.MatchString(transfer.path) {
		return conflict("Repository candidate has no sealed transfer for the approved source")
	}
	return nil
}

type repositorySourceReadback struct {
	source ri.RepositorySourceExecution
	path   string
}

func (s *Service) repositorySourceBundleScope(ctx context.Context, subject candidateSubject, intake string, assignment cc.AssignmentRef) (out repositorySourceReadback, err error) {
	ctx, tx, cleanup, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cleanup()
	defer tx.Rollback()
	a, err := s.candidateAuthorityTx(ctx, tx, subject, assignment)
	if err != nil {
		return out, err
	}
	source, err := s.repositorySourceExecutionTx(ctx, tx, subject.tenant, subject.project, assignment.RunID, assignment.NodeID)
	if err != nil {
		return out, err
	}
	if source == nil || source.IntakeID != intake {
		return out, forbidden()
	}
	if eligible, err := s.repositorySourceNodeEligibleTx(ctx, tx, subject.tenant, subject.project, assignment.RunID, assignment.NodeID, a.spec); err != nil || !eligible {
		if err != nil {
			return out, err
		}
		return out, forbidden()
	}
	if err = s.currentRepositorySourceTaskTx(ctx, tx, subject.tenant, subject.project, assignment.RunID, *source); err != nil {
		return out, err
	}
	intakeRecord, path, err := s.loadRepositorySourceIntakeTx(ctx, tx, subject.tenant, subject.project, source.TaskID, intake)
	if err != nil {
		return out, err
	}
	if !repositoryExecutionMatchesReceipt(*source, intakeRecord.Receipt) {
		return out, conflict("Retained repository source changed")
	}
	return repositorySourceReadback{source: *source, path: path}, a.assert()
}

func (s *Server) repositorySourceBundleHTTP(w http.ResponseWriter, r *http.Request, subject candidateSubject, path, prefix string) bool {
	if path != prefix && !strings.HasPrefix(path, prefix+"/") {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	parts := strings.Split(strings.TrimPrefix(path, prefix+"/"), "/")
	if len(parts) != 2 || parts[1] != "bundle" || r.Method != http.MethodGet || !handoffID(parts[0], false) {
		writeError(w, missing())
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 1 || len(query["project_id"]) != 1 || !handoffID(query.Get("project_id"), false) {
		writeError(w, invalid("Invalid repository source scope"))
		return true
	}
	token, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !bearer || token == "" || !subject.pool && subject.actor.kind != actorProjectRunner {
		writeError(w, forbidden())
		return true
	}
	assignment, err := candidateAssignmentHeader(r)
	if err != nil {
		writeError(w, err)
		return true
	}
	project := query.Get("project_id")
	if subject.pool {
		subject.tenant, subject.project, subject.token = assignment.TenantID, assignment.ProjectID, token
	} else if subject.actor.credential.projectID != project {
		writeError(w, forbidden())
		return true
	}
	if err = candidateEnvelope(subject, project, assignment.TenantID, assignment.PoolRunnerID, assignment); err != nil {
		writeError(w, err)
		return true
	}
	ctx := r.Context()
	before, err := s.Service.repositorySourceBundleScope(ctx, subject, parts[0], assignment)
	if err != nil {
		writeError(w, err)
		return true
	}
	bytes, err := s.Service.readFixedCandidateBytes(before.path, before.source.BundleSHA256, before.source.SizeBytes)
	if err != nil {
		writeError(w, err)
		return true
	}
	after, err := s.Service.repositorySourceBundleScope(ctx, subject, parts[0], assignment)
	if err != nil {
		writeError(w, err)
	} else if after != before {
		writeError(w, conflict("Retained repository source changed"))
	} else {
		artifactBytes(w, bytes)
	}
	return true
}

func (s *Server) taskDeliveryRepositoryDispatchHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project, task, intake string, body map[string]any) {
	w.Header().Set("Cache-Control", "no-store")
	if !actor.IsBrowser() {
		writeError(w, forbidden())
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, missing())
		return
	}
	var req TaskDeliveryRepositoryDispatchPreviewRequest
	if r.URL.ForceQuery || r.URL.RawQuery != "" || repositoryDecodeBody(body, &req, "project_id", "expected_task_version", "producer_profile_id", "expected_producer_profile_version", "test_profile_id", "expected_test_profile_version", "idempotency_key") != nil {
		writeError(w, invalid("Invalid repository source dispatch preview"))
		return
	}
	out, err := s.Service.CreateTaskDeliveryRepositoryDispatchPlan(r.Context(), s.Engine, actor, project, task, intake, req)
	respond(w, http.StatusCreated, out, err)
}
