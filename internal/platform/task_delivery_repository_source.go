package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/ai-mada/suma/internal/dag"
	term "github.com/ai-mada/suma/internal/terminalcontract"
)

// TaskDeliveryRepositorySource is a current registered Git ref observation.
// It grants no candidate intake, test, acceptance, or write authority.
type TaskDeliveryRepositorySource struct {
	TaskID        string `json:"task_id"`
	TaskVersion   int    `json:"task_version"`
	RunID         string `json:"run_id"`
	NodeID        string `json:"node_id"`
	Attempt       int    `json:"attempt"`
	TargetID      string `json:"target_id"`
	TargetVersion int    `json:"target_version"`
	Ref           string `json:"ref"`
	HeadCommit    string `json:"head_commit"`
	ObjectFormat  string `json:"object_format"`
	ObservedAt    string `json:"observed_at"`
}

type taskDeliveryRepositoryQuery struct {
	project, task, run, target string
	taskVersion, targetVersion int
}

// Only fixed values from the locked scope participate in comparison across
// the provider read. Credential digests stay private to this transaction pair.
type taskDeliveryRepositoryBasis struct {
	origin                                                  term.TerminalCandidateWorkBuddyOrigin
	provider, alias, ref, configSHA, objectFormat, fenceOID string
	generation                                              int64
	providerOrigin, providerTokenSHA, reconcileTokenSHA     string
	exportTokenSHA                                          string
}

func (s *Service) openTaskDeliveryRepositoryScope(ctx context.Context, actor Actor, query taskDeliveryRepositoryQuery, purpose commandPurpose) (context.Context, *authorizedCommand, func(), error) {
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return ctx, nil, nil, err
	}
	close := func() { tx.Rollback(); cancel() }
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: query.project, resourceID: query.task}, purpose)
	if err == nil {
		err = dag.GuardExecutionScopeTx(ctx, tx, s.Dialect, auth.target.tenantID, query.project, query.run)
	}
	if err != nil {
		close()
		return ctx, nil, nil, err
	}
	return ctx, auth, close, nil
}

func (s *Service) taskDeliveryRepositoryBasis(ctx context.Context, actor Actor, query taskDeliveryRepositoryQuery) (basis taskDeliveryRepositoryBasis, provider RepositoryProviderConfig, err error) {
	ctx, auth, close, err := s.openTaskDeliveryRepositoryScope(ctx, actor, query, purposeBrowserProjectWrite)
	if err != nil {
		return basis, provider, err
	}
	defer close()
	return s.taskDeliveryRepositoryBasisTx(ctx, auth, query)
}

func (s *Service) taskDeliveryRepositoryBasisTx(ctx context.Context, auth *authorizedCommand, query taskDeliveryRepositoryQuery) (basis taskDeliveryRepositoryBasis, provider RepositoryProviderConfig, err error) {
	tx := auth.tx
	tenant := auth.target.tenantID
	task, err := s.getRecordTx(ctx, auth, query.project, "tasks", query.task)
	if err != nil {
		return basis, provider, err
	}
	if task.Version != query.taskVersion || task.RunID != query.run || task.Data["run_task_basis"] != taskBasis(task) {
		return basis, provider, conflict("Task delivery source changed; refresh the original task")
	}
	origin, err := s.workspaceTerminalWorkBuddyOriginTx(ctx, tx, tenant, query.project, query.task, query.run)
	if err != nil {
		return basis, provider, err
	}
	if origin == nil {
		return basis, provider, conflict("Task delivery source changed; refresh the original task")
	}

	// The project guard serializes target and run operations. A current locked
	// read also avoids stale target data under the distributed SQL dialects.
	var raw string
	var version int
	err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT version,data FROM suma_repository_targets WHERE tenant_id=? AND project_id=? AND id=?`)), tenant, query.project, query.target).Scan(&version, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return basis, provider, missing()
	}
	if err != nil {
		return basis, provider, err
	}
	var target RepositoryIntegrationTarget
	if err = json.Unmarshal([]byte(raw), &target); err != nil {
		return basis, provider, err
	}
	if target.ID != query.target || target.ProjectID != query.project || version != query.targetVersion || target.Version != version || target.Status != "active" {
		return basis, provider, repositoryConflict("target", repositoryTarget)
	}
	if target.ActiveOperationID != "" {
		return basis, provider, repositoryConflict("busy", repositoryBusy)
	}
	provider, err = s.repositoryProvider(tenant, query.project, target.ProviderID, target.TargetAlias)
	if err != nil {
		return basis, provider, err
	}
	basis = taskDeliveryRepositoryBasis{
		origin: *origin, provider: target.ProviderID, alias: target.TargetAlias, ref: target.Ref,
		configSHA: target.ConfigSHA256, generation: target.Generation,
		providerOrigin: provider.Origin, providerTokenSHA: provider.TokenHash, reconcileTokenSHA: digest(provider.ReconcileToken), exportTokenSHA: digest(provider.SourceExportToken),
	}
	// A reconciled apply clears the old observation. The persisted target
	// configuration digest still binds Git format, ref and repository identity.
	if target.Observation != nil {
		basis.objectFormat, basis.fenceOID = target.Observation.ObjectFormat, target.Observation.FenceOID
	}
	return basis, provider, auth.assertTx(tx, auth.target, auth.purpose)
}

func (s *Service) taskDeliveryRepositorySource(ctx context.Context, actor Actor, query taskDeliveryRepositoryQuery) (out TaskDeliveryRepositorySource, err error) {
	before, provider, err := s.taskDeliveryRepositoryBasis(ctx, actor, query)
	if err != nil {
		return out, err
	}
	// Keep the provider request outside SQL transactions. Recheck authority and
	// the exact original execution and target before disclosing its response.
	observed, probeErr := repositoryProbe(ctx, provider, before.alias)
	after, _, err := s.taskDeliveryRepositoryBasis(ctx, actor, query)
	if err != nil {
		return out, err
	}
	if before != after {
		return out, conflict("Task delivery source changed; refresh the original task")
	}
	if probeErr != nil {
		return out, probeErr
	}
	commitLength := 40
	if observed.ObjectFormat == "sha256" {
		commitLength = 64
	}
	if observed.Ref != before.ref || observed.ConfigSHA256 != before.configSHA || observed.Generation != before.generation ||
		before.objectFormat != "" && (observed.ObjectFormat != before.objectFormat || observed.FenceOID != before.fenceOID) ||
		len(observed.HeadCommit) != commitLength || observed.FenceOID != "" && len(observed.FenceOID) != commitLength {
		return out, repositoryConflict("target", repositoryTarget)
	}
	return TaskDeliveryRepositorySource{
		TaskID: query.task, TaskVersion: query.taskVersion, RunID: before.origin.RunID,
		NodeID: before.origin.NodeID, Attempt: before.origin.Attempt, TargetID: query.target,
		TargetVersion: query.targetVersion, Ref: observed.Ref, HeadCommit: observed.HeadCommit,
		ObjectFormat: observed.ObjectFormat, ObservedAt: observed.ObservedAt,
	}, nil
}

func (s *Server) taskDeliveryRepositorySourceHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project, task string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		writeError(w, missing())
		return
	}
	if !actor.IsBrowser() {
		writeError(w, forbidden())
		return
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	const invalidQuery = "Invalid task delivery repository source query"
	if err != nil || len(r.URL.RawQuery) > 8192 || len(values) != 5 {
		writeError(w, invalid(invalidQuery))
		return
	}
	for _, key := range []string{"project_id", "task_version", "run_id", "target_id", "target_version"} {
		if len(values[key]) != 1 {
			writeError(w, invalid(invalidQuery))
			return
		}
	}
	run, target := values.Get("run_id"), values.Get("target_id")
	if project != values.Get("project_id") || !handoffID(project, false) || !handoffID(task, false) || !handoffID(run, false) || !handoffID(target, false) {
		writeError(w, invalid(invalidQuery))
		return
	}
	version := func(key string) int {
		value := values.Get(key)
		parsed, err := strconv.ParseUint(value, 10, 31)
		if err != nil || strconv.FormatUint(parsed, 10) != value {
			return 0
		}
		return int(parsed)
	}
	taskVersion, targetVersion := version("task_version"), version("target_version")
	if taskVersion == 0 || targetVersion == 0 {
		writeError(w, invalid(invalidQuery))
		return
	}
	out, err := s.Service.taskDeliveryRepositorySource(r.Context(), actor, taskDeliveryRepositoryQuery{project: project, task: task, run: run, target: target, taskVersion: taskVersion, targetVersion: targetVersion})
	respond(w, http.StatusOK, out, err)
}
