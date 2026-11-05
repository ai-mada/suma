package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	term "github.com/ai-mada/suma/internal/terminalcontract"
)

type TaskDeliveryRepositoryIntakeRequest struct {
	ProjectID      string `json:"project_id"`
	TaskVersion    int    `json:"task_version"`
	RunID          string `json:"run_id"`
	TargetID       string `json:"target_id"`
	TargetVersion  int    `json:"target_version"`
	ExpectedHead   string `json:"expected_head"`
	ExpectedBase   string `json:"expected_base"`
	IdempotencyKey string `json:"idempotency_key"`
}

// TaskDeliveryRepositoryIntake is approval and retained bytes for a source,
// separate from a code candidate, test verdict or acceptance.
type TaskDeliveryRepositoryIntake struct {
	ID            string `json:"id"`
	ProjectID     string `json:"project_id"`
	TaskID        string `json:"task_id"`
	TaskVersion   int    `json:"task_version"`
	RunID         string `json:"run_id"`
	NodeID        string `json:"node_id"`
	Attempt       int    `json:"attempt"`
	TargetID      string `json:"target_id"`
	TargetVersion int    `json:"target_version"`
	BaseCommit    string `json:"base_commit"`
	HeadCommit    string `json:"head_commit"`
	BundleSHA256  string `json:"bundle_sha256"`
	SizeBytes     int64  `json:"size_bytes"`
	ApprovedAt    string `json:"approved_at"`
}

type repositorySourceIntakeRecord struct {
	Receipt       TaskDeliveryRepositoryIntake          `json:"receipt"`
	Origin        term.TerminalCandidateWorkBuddyOrigin `json:"origin"`
	Export        ri.RepositorySourceExportRequest      `json:"export"`
	RequestSHA256 string                                `json:"request_sha256"`
	ApprovedBy    string                                `json:"approved_by"`
}

const repositoryIntakeLimit = 8

func (s *Service) retainTaskDeliveryRepositorySource(ctx context.Context, actor Actor, project, task string, req TaskDeliveryRepositoryIntakeRequest) (out TaskDeliveryRepositoryIntake, err error) {
	if req.ProjectID != project || !handoffID(project, false) || !handoffID(task, false) || !handoffID(req.RunID, false) ||
		!handoffID(req.TargetID, false) || req.TaskVersion < 1 || req.TaskVersion > 2147483647 || req.TargetVersion < 1 || req.TargetVersion > 2147483647 ||
		!cc.ValidCommit(req.ExpectedHead) || !cc.ValidCommit(req.ExpectedBase) || len(req.ExpectedHead) != len(req.ExpectedBase) || req.ExpectedBase == req.ExpectedHead ||
		!planningKey(req.IdempotencyKey) {
		return out, invalid("Invalid repository source intake")
	}
	query := taskDeliveryRepositoryQuery{project: project, task: task, run: req.RunID, target: req.TargetID, taskVersion: req.TaskVersion, targetVersion: req.TargetVersion}
	command := struct {
		Task    string
		Request TaskDeliveryRepositoryIntakeRequest
	}{Task: task, Request: req}
	body := map[string]any{"repository_source_intake": command}
	load := func(ctx context.Context, auth *authorizedCommand) (fingerprint string, replay bool, basis taskDeliveryRepositoryBasis, provider RepositoryProviderConfig, err error) {
		fingerprint, replay, err = s.commandReceipt(ctx, auth, req.IdempotencyKey, body)
		if err != nil {
			return
		}
		if replay {
			err = json.Unmarshal([]byte(fingerprint), &out)
			return
		}
		basis, provider, err = s.taskDeliveryRepositoryBasisTx(ctx, auth, query)
		if err != nil {
			return
		}
		var count int
		err = auth.tx.QueryRowContext(ctx, s.q("SELECT COUNT(*) FROM suma_repository_source_intakes WHERE tenant_id=? AND project_id=? AND task_id=? AND run_id=?"),
			auth.target.tenantID, project, task, req.RunID).Scan(&count)
		if err == nil && count >= repositoryIntakeLimit {
			err = conflict("This delivery attempt has reached its repository source intake limit")
		}
		return
	}
	readCtx, auth, close, err := s.openTaskDeliveryRepositoryScope(ctx, actor, query, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	tenant := auth.target.tenantID
	_, replay, before, provider, err := load(readCtx, auth)
	close()
	if err != nil || replay {
		return out, err
	}
	if len(provider.SourceExportToken) < 32 {
		return out, repositoryError(http.StatusServiceUnavailable, "provider", repositoryProvider)
	}
	// The registered baseline is independently chosen by the provider. Browser
	// input asserts the two reviewed commits and cannot select the baseline ref.
	pair, err := fetchRepositorySourceExportBasis(ctx, provider, before.alias)
	if err != nil {
		return out, err
	}
	if pair.Source.HeadCommit != req.ExpectedHead || pair.Baseline.HeadCommit != req.ExpectedBase ||
		pair.Source.Ref != before.ref || pair.Source.ConfigSHA256 != before.configSHA || pair.Source.Generation != before.generation ||
		before.objectFormat != "" && (pair.Source.ObjectFormat != before.objectFormat || pair.Source.FenceOID != before.fenceOID) {
		return out, repositoryConflict("target", repositoryTarget)
	}
	export := ri.RepositorySourceExportRequest{
		Protocol: ri.RepositorySourceExportProtocol, TenantID: tenant, ProjectID: project, ProviderID: before.provider,
		TargetAlias: before.alias, TargetRef: before.ref, TargetConfigSHA256: before.configSHA,
		ExpectedHead: req.ExpectedHead, ExpectedBase: req.ExpectedBase, ExpectedFenceOID: pair.Source.FenceOID, ExpectedGeneration: pair.Source.Generation,
	}
	source, err := fetchRepositorySourceExport(ctx, provider, export)
	if err != nil {
		return out, err
	}

	// Keep provider I/O and blob fsync outside scheduler/authority locks. The
	// scoped SQL row owns the blob only when the receipt transaction commits.
	root, err := s.artifactRoot()
	if err != nil {
		return out, err
	}
	defer root.Close()
	id := newID()
	storagePath := "blobs/" + id
	file, err := root.OpenFile(storagePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return out, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = root.Remove(storagePath)
		}
	}()
	_, writeErr := file.Write(source.Bundle)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil {
		return out, writeErr
	}
	if syncErr != nil {
		return out, syncErr
	}
	if closeErr != nil {
		return out, closeErr
	}
	writeCtx, auth, close, err := s.openTaskDeliveryRepositoryScope(ctx, actor, query, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	defer close()
	fingerprint, replay, current, _, err := load(writeCtx, auth)
	if err != nil || replay {
		return out, err
	}
	if current != before {
		return out, repositoryConflict("target", repositoryTarget)
	}
	clock, err := s.authorityNow(writeCtx, auth.tx)
	if err != nil {
		return out, err
	}
	out = TaskDeliveryRepositoryIntake{
		ID: id, ProjectID: project, TaskID: task, TaskVersion: req.TaskVersion,
		RunID: before.origin.RunID, NodeID: before.origin.NodeID, Attempt: before.origin.Attempt,
		TargetID: req.TargetID, TargetVersion: req.TargetVersion, BaseCommit: req.ExpectedBase, HeadCommit: req.ExpectedHead,
		BundleSHA256: source.BundleSHA256, SizeBytes: int64(len(source.Bundle)), ApprovedAt: automationStamp(clock),
	}
	record := repositorySourceIntakeRecord{Receipt: out, Origin: before.origin, Export: export, RequestSHA256: source.RequestSHA256, ApprovedBy: auth.currentPrincipal.ID}
	if _, err = auth.tx.ExecContext(writeCtx, s.q("INSERT INTO suma_repository_source_intakes(id,tenant_id,project_id,task_id,run_id,storage_path,data)VALUES(?,?,?,?,?,?,?)"),
		id, tenant, project, task, req.RunID, storagePath, stringify(record)); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(writeCtx, auth, req.IdempotencyKey, fingerprint, out); err != nil {
		return out, err
	}
	if err = s.commandActivity(writeCtx, auth, "repository.source_retained", task, "Repository source retained"); err != nil {
		return out, err
	}
	// Preserve possible committed bytes if the database loses COMMIT's reply.
	keep = true
	return out, auth.tx.Commit()
}

func (s *Server) taskDeliveryRepositoryIntakeHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project, task string, body map[string]any) {
	w.Header().Set("Cache-Control", "no-store")
	if !actor.IsBrowser() {
		writeError(w, forbidden())
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, missing())
		return
	}
	var req TaskDeliveryRepositoryIntakeRequest
	if r.URL.ForceQuery || r.URL.RawQuery != "" || repositoryDecodeBody(body, &req, "project_id", "task_version", "run_id", "target_id", "target_version", "expected_head", "expected_base", "idempotency_key") != nil {
		writeError(w, invalid("Invalid repository source intake"))
		return
	}
	out, err := s.Service.retainTaskDeliveryRepositorySource(r.Context(), actor, project, task, req)
	respond(w, http.StatusCreated, out, err)
}
