package platform

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/ai-mada/suma/internal/dag"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

type TaskDeliveryRepositorySourceProfile struct {
	ID        string `json:"id"`
	Version   int    `json:"version"`
	Name      string `json:"name"`
	WorkerID  string `json:"worker_id"`
	CommandID string `json:"command_id"`
}

// Current source observation and retained approvals have separate lifetimes.
// No credential, provider path, execution lease or Git byte is projected.
type TaskDeliveryRepositorySourceOptions struct {
	ProjectID          string                                `json:"project_id"`
	TaskID             string                                `json:"task_id"`
	TaskVersion        int                                   `json:"task_version"`
	RunID              string                                `json:"run_id"`
	NodeID             string                                `json:"node_id"`
	Attempt            int                                   `json:"attempt"`
	TargetID           string                                `json:"target_id"`
	TargetVersion      int                                   `json:"target_version"`
	HeadCommit         string                                `json:"head_commit"`
	BaseCommit         string                                `json:"base_commit"`
	BaselineRef        string                                `json:"baseline_ref"`
	LiveState          string                                `json:"live_state"`
	PublishedCaseCount int                                   `json:"published_case_count"`
	ActiveTerminal     bool                                  `json:"active_terminal"`
	ProfilesTruncated  bool                                  `json:"profiles_truncated"`
	RetainedIntakes    []TaskDeliveryRepositoryIntake        `json:"retained_intakes"`
	ProducerProfiles   []TaskDeliveryRepositorySourceProfile `json:"producer_profiles"`
	TestProfiles       []TaskDeliveryRepositorySourceProfile `json:"test_profiles"`
}

func (s *Service) taskDeliveryRepositoryOptions(ctx context.Context, engine *dag.Engine, actor Actor, query taskDeliveryRepositoryQuery) (out TaskDeliveryRepositorySourceOptions, err error) {
	if engine == nil {
		return out, invalid("Repository dispatch scheduler unavailable")
	}
	initialCtx, initial, closeInitial, err := s.openTaskDeliveryRepositoryScope(ctx, actor, query, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	before, provider, err := s.taskDeliveryRepositoryBasisTx(initialCtx, initial, query)
	closeInitial()
	if err != nil {
		return out, err
	}

	// Provider metadata is optional for retained approvals. Any current facts
	// from it are disclosed only after rechecking the original task and target.
	var pair ri.RepositorySourceExportBasis
	if len(provider.SourceExportToken) >= 32 {
		if observed, probeErr := fetchRepositorySourceExportBasis(ctx, provider, before.alias); probeErr == nil {
			pair = observed
		}
	}
	readCtx, auth, close, err := s.openTaskDeliveryRepositoryScope(ctx, actor, query, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	defer close()
	current, _, err := s.taskDeliveryRepositoryBasisTx(readCtx, auth, query)
	if err != nil {
		return out, err
	}
	if current != before {
		return out, conflict("Task delivery source changed; refresh the original task")
	}
	tenant := auth.target.tenantID
	out = TaskDeliveryRepositorySourceOptions{
		ProjectID: query.project, TaskID: query.task, TaskVersion: query.taskVersion,
		RunID: current.origin.RunID, NodeID: current.origin.NodeID, Attempt: current.origin.Attempt,
		TargetID: query.target, TargetVersion: query.targetVersion, LiveState: "unavailable",
		ProducerProfiles: []TaskDeliveryRepositorySourceProfile{}, TestProfiles: []TaskDeliveryRepositorySourceProfile{},
	}
	if pair.Source.Available && pair.Source.Ref == current.ref && pair.Source.ConfigSHA256 == current.configSHA &&
		pair.Source.Generation == current.generation && (current.objectFormat == "" ||
		pair.Source.ObjectFormat == current.objectFormat && pair.Source.FenceOID == current.fenceOID) {
		out.HeadCommit, out.BaseCommit, out.BaselineRef = pair.Source.HeadCommit, pair.Baseline.HeadCommit, pair.Baseline.Ref
		out.LiveState = "ready"
		if out.HeadCommit == out.BaseCommit {
			out.LiveState = "unchanged"
		}
	}
	out.RetainedIntakes, err = s.repositorySourceRetainedOptionsTx(readCtx, auth, query, current)
	if err != nil {
		return out, err
	}
	quality, err := s.taskTestPlanContextTx(readCtx, auth.tx, tenant, query.project, query.task)
	if err != nil {
		return out, err
	}
	if quality != nil {
		out.PublishedCaseCount = len(quality.Cases)
	}
	var terminalCount int
	if err = auth.tx.QueryRowContext(readCtx, s.q("SELECT COUNT(*) FROM suma_workspace_terminal_active WHERE tenant_id=? AND project_id=? AND task_id=?"), tenant, query.project, query.task).Scan(&terminalCount); err != nil {
		return out, err
	}
	out.ActiveTerminal = terminalCount > 0
	route, err := engine.ReadProjectExecutionRouteTx(readCtx, auth.tx, tenant, query.project)
	if err != nil {
		return out, err
	}
	profiles, err := s.listToolProfilesForProjectTx(readCtx, auth, query.project, ToolProfileListQuery{State: "enabled", Limit: 200})
	if err != nil {
		return out, err
	}
	out.ProfilesTruncated = profiles.NextCursor != ""
	eligible := make([]ToolProfile, 0, len(profiles.Items))
	for _, profile := range profiles.Items {
		if !profile.Available || !ingressVersion(profile.Version) || profile.Profile.Adapter != "command" || profile.Profile.Protocol != "text-v1" || profile.Profile.ReadOnly {
			continue
		}
		if err = s.checkDispatchProfileRouteTx(readCtx, auth.tx, tenant, query.project, profile, route); err != nil {
			var api *APIError
			if errors.As(err, &api) && api.Status == http.StatusConflict {
				continue
			}
			return out, err
		}
		eligible = append(eligible, profile)
	}
	out.ProducerProfiles, out.TestProfiles = repositorySourceIndependentProfiles(eligible)
	return out, auth.assertTx(auth.tx, auth.target, purposeBrowserProjectApprove)
}

func repositorySourceIntakeMatchesBasis(record repositorySourceIntakeRecord, current taskDeliveryRepositoryBasis) bool {
	export := record.Export
	return current.origin == record.Origin && current.provider == export.ProviderID && current.alias == export.TargetAlias &&
		current.ref == export.TargetRef && current.configSHA == export.TargetConfigSHA256 && current.generation == export.ExpectedGeneration &&
		(current.objectFormat == "" || current.fenceOID == export.ExpectedFenceOID)
}

func (s *Service) repositorySourceRetainedOptionsTx(ctx context.Context, auth *authorizedCommand, query taskDeliveryRepositoryQuery, current taskDeliveryRepositoryBasis) ([]TaskDeliveryRepositoryIntake, error) {
	tenant := auth.target.tenantID
	rows, err := auth.tx.QueryContext(ctx, s.q("SELECT id FROM suma_repository_source_intakes WHERE tenant_id=? AND project_id=? AND task_id=? AND run_id=? ORDER BY id LIMIT ?"), tenant, query.project, query.task, query.run, repositoryIntakeLimit)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, repositoryIntakeLimit)
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	receipts := make([]TaskDeliveryRepositoryIntake, 0, len(ids))
	for _, id := range ids {
		record, _, err := s.loadRepositorySourceIntakeTx(ctx, auth.tx, tenant, query.project, query.task, id)
		if err != nil {
			return nil, err
		}
		receipt := record.Receipt
		if receipt.TaskVersion == query.taskVersion && receipt.TargetID == query.target && receipt.TargetVersion == query.targetVersion && repositorySourceIntakeMatchesBasis(record, current) {
			receipts = append(receipts, receipt)
		}
	}
	// Approval stamps are canonical UTC RFC3339 seconds. Sort newest first;
	// ID provides a deterministic order for approvals in the same second.
	slices.SortFunc(receipts, func(a, b TaskDeliveryRepositoryIntake) int {
		if order := cmp.Compare(b.ApprovedAt, a.ApprovedAt); order != 0 {
			return order
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return receipts, nil
}

func repositorySourceIndependentProfiles(profiles []ToolProfile) (producers, tests []TaskDeliveryRepositorySourceProfile) {
	producers = []TaskDeliveryRepositorySourceProfile{}
	tests = []TaskDeliveryRepositorySourceProfile{}
	convert := func(p ToolProfile) TaskDeliveryRepositorySourceProfile {
		return TaskDeliveryRepositorySourceProfile{ID: p.ID, Version: p.Version, Name: p.Name, WorkerID: p.WorkerID, CommandID: p.Profile.CommandID}
	}
	producerProfiles := make([]ToolProfile, 0, len(profiles))
	for _, profile := range profiles {
		if workspaceTerminalCandidateProfile(profile) {
			producerProfiles = append(producerProfiles, profile)
		}
	}
	for _, profile := range profiles {
		// An independent candidate requires a different machine identity. One
		// compatible producer suffices; the UI also filters the selected pair.
		index := slices.IndexFunc(producerProfiles, func(producer ToolProfile) bool { return profile.WorkerID != producer.WorkerID })
		if index >= 0 && taskDispatchIndependentTestProfile(profile, producerProfiles[index]) {
			tests = append(tests, convert(profile))
		}
	}
	for _, profile := range producerProfiles {
		if slices.ContainsFunc(tests, func(test TaskDeliveryRepositorySourceProfile) bool { return profile.WorkerID != test.WorkerID }) {
			producers = append(producers, convert(profile))
		}
	}
	return producers, tests
}

func (s *Server) taskDeliveryRepositorySourceOptionsHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project, task string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		writeError(w, missing())
		return
	}
	if !actor.IsBrowser() {
		writeError(w, forbidden())
		return
	}
	query, err := readTaskDeliveryRepositoryQuery(r, project, task)
	if err != nil {
		writeError(w, err)
		return
	}
	out, err := s.Service.taskDeliveryRepositoryOptions(r.Context(), s.Engine, actor, query)
	respond(w, http.StatusOK, out, err)
}
