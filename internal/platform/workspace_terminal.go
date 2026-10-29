package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	term "github.com/ai-mada/suma/internal/terminalcontract"
)

func validWorkspaceTerminalCreate(r term.CreateRequest) bool {
	return handoffID(r.ProjectID, false) && handoffID(r.TaskID, false) && handoffID(r.ToolProfileID, false) && r.ExpectedTaskVersion > 0 && r.ExpectedTaskVersion <= 2147483646 && r.ExpectedToolVersion > 0 && r.ExpectedToolVersion <= 2147483646 && r.Columns >= 20 && r.Columns <= 500 && r.Rows >= 5 && r.Rows <= 200 && r.TimeoutSeconds >= 60 && r.TimeoutSeconds <= 3600 && planningKey(r.IdempotencyKey)
}

func (s *Service) workspaceTerminalBasisTx(ctx context.Context, auth *authorizedCommand, r workspaceTerminalRecord) error {
	task, err := s.getRecordTx(ctx, auth, r.Session.ProjectID, "tasks", r.Session.TaskID)
	if err != nil {
		return err
	}
	if task.Status == "accepted" || task.Version != r.Task.Version || task.RunID != r.TaskRunID || stringify(taskContextRecord(task)) != stringify(r.Task) {
		return conflict(workspaceTerminalChanged)
	}
	baseline, _, err := s.taskCodeBaselineTx(ctx, auth.tx, auth.target.tenantID, r.Session.ProjectID, r.Session.TaskID)
	if err != nil {
		return err
	}
	if stringify(baseline) != stringify(r.Session.CodeBaseline) {
		return conflict(workspaceTerminalChanged)
	}
	if baseline != nil {
		if baseline.TaskVersion != task.Version {
			return conflict(workspaceTerminalChanged)
		}
		if err = s.checkTaskCodeBaselineTx(ctx, auth.tx, auth.target.tenantID, r.Session.ProjectID, *baseline); err != nil {
			return err
		}
	}
	if r.Requirement == nil {
		if task.RequirementID != "" {
			return conflict(workspaceTerminalChanged)
		}
	} else {
		requirement, e := s.getRecordTx(ctx, auth, r.Session.ProjectID, "requirements", r.Requirement.ID)
		if e != nil {
			return e
		}
		if task.RequirementID != requirement.ID || stringify(taskContextRecord(requirement)) != stringify(r.Requirement) {
			return conflict(workspaceTerminalChanged)
		}
	}
	return nil
}

func (s *Service) workspaceTerminalActiveTx(ctx context.Context, tx *sql.Tx, tenant, project, task string) error {
	var count int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_workspace_terminal_active WHERE tenant_id=? AND project_id=? AND task_id=?`), tenant, project, task).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return conflict(workspaceTerminalActive)
	}
	return nil
}

func (s *Service) CreateWorkspaceTerminal(ctx context.Context, engine *dag.Engine, actor Actor, project string, request WorkspaceTerminalCreateRequest) (out WorkspaceTerminalSession, err error) {
	req := term.CreateRequest(request)
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if engine == nil || project != req.ProjectID || !validWorkspaceTerminalCreate(req) {
		return out, invalid(workspaceTerminalInvalid)
	}
	if !req.AcknowledgeShell {
		return out, invalid("Confirm real shell execution in the isolated attempt workspace")
	}
	first, auth, close, err := s.planningAuthority(ctx, actor, project, req.TaskID, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	body := map[string]any{"workspace_terminal_create": req}
	raw, replay, err := s.commandReceipt(first, auth, req.IdempotencyKey, body)
	if err != nil {
		close()
		return out, err
	}
	if replay {
		var receipt term.Session
		if err = json.Unmarshal([]byte(raw), &receipt); err == nil {
			var record workspaceTerminalRecord
			record, err = s.workspaceTerminalTx(first, auth.tx, auth.target.tenantID, project, receipt.ID)
			if err == nil {
				out, err = s.workspaceTerminalProjectionTx(first, auth, record)
			}
		}
		close()
		return out, err
	}
	if err = s.workspaceTerminalActiveTx(first, auth.tx, auth.target.tenantID, project, req.TaskID); err != nil {
		close()
		return out, err
	}
	task, err := s.getRecordTx(first, auth, project, "tasks", req.TaskID)
	if err == nil && (task.Version != req.ExpectedTaskVersion || task.Status == "accepted") {
		err = conflict(workspaceTerminalChanged)
	}
	record := workspaceTerminalRecord{Task: taskContextRecord(task), TaskRunID: task.RunID, Creator: auth.currentPrincipal.ID, CreatorCredentialHash: actor.credential.digest}
	var baseline *ccmp.CodeBaselineBinding
	if err == nil {
		var context TaskContext
		context, err = s.taskContextTx(first, auth.tx, auth.currentPrincipal, project, req.TaskID)
		baseline = context.CodeBaseline
	}
	if err == nil && task.RequirementID != "" {
		var requirement Record
		requirement, err = s.getRecordTx(first, auth, project, "requirements", task.RequirementID)
		if err == nil {
			r := taskContextRecord(requirement)
			record.Requirement = &r
		}
	}
	var profile ToolProfile
	if err == nil {
		profile, err = s.toolProfileTx(first, auth.tx, auth.currentPrincipal, project, req.ToolProfileID)
	}
	if err == nil && !workspaceTerminalProfile(profile) {
		err = invalid(workspaceTerminalProfileReason)
	}
	if err == nil {
		profile, err = s.validateToolDispatchTx(first, auth.tx, auth, project, profile.ID, req.ExpectedToolVersion, profile.Profile.ProfileSHA256)
	}
	if err == nil {
		record.Route, err = engine.ReadProjectExecutionRouteTx(first, auth.tx, auth.target.tenantID, project)
	}
	if err == nil {
		err = s.checkDispatchProfileRouteTx(first, auth.tx, auth.target.tenantID, project, profile, record.Route)
	}
	clock, clockErr := s.automationClock(first, auth.tx)
	close()
	if err != nil {
		return out, err
	}
	if clockErr != nil {
		return out, clockErr
	}
	id := digest(actor.credential.digest + ":" + project + ":" + req.IdempotencyKey + ":workspace-terminal")[:32]
	record.Session = term.Session{ID: id, ProjectID: project, TaskID: req.TaskID, TaskVersion: req.ExpectedTaskVersion, Version: 1, RunID: "terminal-" + id, State: "queued", ToolProfileID: profile.ID, ToolName: profile.Name, WorkerID: profile.WorkerID, Columns: req.Columns, Rows: req.Rows, ControlEpoch: 1, NextInputSequence: 1, ExpiresAt: automationStamp(clock.Add(time.Duration(req.TimeoutSeconds) * time.Second)), CreatedAt: automationStamp(clock), AllowedActions: []string{}}
	record.Session.CodeBaseline = baseline
	if baseline != nil {
		record.Session.BaseCommit, record.Session.AdminBaseCommit = baseline.Source.HeadCommit, baseline.Source.BundleBase()
	}
	record.ToolProfileVersion, record.ToolProfileSHA256 = profile.Version, profile.Profile.ProfileSHA256
	w := dag.Workflow{Name: "Interactive task terminal", Nodes: []dag.Node{{ID: "terminal", Name: "Interactive isolated workspace shell", Type: "terminal_session", Command: []string{profile.Profile.CommandID}, DependsOn: []string{}, TimeoutSeconds: req.TimeoutSeconds, MaxRetries: 0, Idempotent: false, Config: map[string]any{"terminal_session_id": id, "task_id": req.TaskID, "task_version": req.ExpectedTaskVersion, "columns": req.Columns, "rows": req.Rows, "tool_profile_id": profile.ID, "tool_profile_sha256": profile.Profile.ProfileSHA256}}}, Edges: []dag.Edge{}}
	if baseline != nil {
		w.Nodes[0].Config["code_baseline"] = baseline
	}
	authority := s.engineAuthority(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: req.TaskID}, purposeProjectWrite)
	defer authority.close()
	_, err = engine.CreateManagedRunBound(authority.ctx, actor.credential.tenantID, project, record.Session.RunID, "terminal-create-"+id, w, authority.before, func(tx *sql.Tx, actual string) error {
		a := authority.proof
		hash, replayed, e := s.commandReceipt(authority.ctx, a, req.IdempotencyKey, body)
		if e != nil {
			return e
		}
		if replayed {
			return conflict(workspaceTerminalChanged)
		}
		if e = s.workspaceTerminalActiveTx(authority.ctx, tx, a.target.tenantID, project, req.TaskID); e != nil {
			return e
		}
		if e = s.workspaceTerminalBasisTx(authority.ctx, a, record); e != nil {
			return e
		}
		p, e := s.validateToolDispatchTx(authority.ctx, tx, a, project, profile.ID, profile.Version, profile.Profile.ProfileSHA256)
		if e != nil {
			return e
		}
		if !workspaceTerminalProfile(p) {
			return invalid(workspaceTerminalProfileReason)
		}
		bound, e := dag.ReadRunExecutionRouteTx(authority.ctx, tx, s.Dialect, a.target.tenantID, project, actual)
		if e != nil {
			return e
		}
		if bound != record.Route {
			return conflict(workspaceTerminalChanged)
		}
		if e = s.checkDispatchProfileRouteTx(authority.ctx, tx, a.target.tenantID, project, p, bound); e != nil {
			return e
		}
		if _, e = tx.ExecContext(authority.ctx, s.q(`INSERT INTO suma_workspace_terminal_sessions(id,tenant_id,project_id,task_id,run_id,version,state,created_at,payload)VALUES(?,?,?,?,?,1,'queued',?,?)`), id, a.target.tenantID, project, req.TaskID, actual, record.Session.CreatedAt, stringify(record)); e != nil {
			return e
		}
		if baseline != nil {
			if e = s.bindCodeBaselineExecutionTx(authority.ctx, tx, a, project, actual, "terminal", *baseline); e != nil {
				return e
			}
		}
		if _, e = tx.ExecContext(authority.ctx, s.q(`INSERT INTO suma_workspace_terminal_active(tenant_id,project_id,task_id,session_id)VALUES(?,?,?,?)`), a.target.tenantID, project, req.TaskID, id); e != nil {
			return e
		}
		if e = s.commandActivity(authority.ctx, a, "workspace_terminal.created", req.TaskID, "Interactive terminal requested in an isolated workspace"); e != nil {
			return e
		}
		return s.saveCommandReceipt(authority.ctx, a, req.IdempotencyKey, hash, record.Session)
	})
	if err != nil {
		return out, err
	}
	return s.GetWorkspaceTerminal(ctx, actor, project, id)
}

func (s *Service) workspaceTerminalCreatorTx(ctx context.Context, tx *sql.Tx, tenant, project string, r workspaceTerminalRecord) (*authorizedCommand, error) {
	c, err := s.readCredential(ctx, tx, r.CreatorCredentialHash, true)
	if err != nil {
		return nil, err
	}
	at, err := s.authorityNow(ctx, tx)
	if err != nil {
		return nil, err
	}
	if c.kind != actorBrowser && c.kind != actorProjectAPI || c.ref.subjectID != r.Creator || c.ref.tenantID != tenant || c.kind == actorProjectAPI && c.ref.projectID != project || !c.expires.After(at) {
		return nil, forbidden()
	}
	p, err := s.currentProjectMembershipLockedTx(ctx, tx, tenant, c.ref.subjectID, project, true, false)
	if err != nil {
		return nil, err
	}
	b := s.txAuthorityBudget(ctx, tx)
	return &authorizedCommand{tx: tx, actor: Actor{kind: c.kind, credential: c.ref}, target: commandTarget{tenantID: tenant, projectID: project, resourceID: r.Session.TaskID}, purpose: purposeProjectWrite, currentPrincipal: p, checkedAt: at, deadline: b.deadline, ctx: b.ctx}, nil
}

// Claims and heartbeats never accept a generic terminal node. Input permission
// remains tied to the exact human source, tool and original execution route.
func (s *Service) checkWorkspaceTerminalExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run string, requireLive bool) error {
	r, found, err := s.workspaceTerminalForRunTx(ctx, tx, tenant, project, run)
	if err != nil || !found {
		return err
	}
	if r.Session.State == "exited" || r.Session.State == "stopped" || r.TreeStopped {
		return conflict(workspaceTerminalChanged)
	}
	auth, err := s.workspaceTerminalCreatorTx(ctx, tx, tenant, project, r)
	if err != nil {
		return err
	}
	if err = s.workspaceTerminalBasisTx(ctx, auth, r); err != nil {
		return err
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return err
	}
	if automationStamp(clock) >= r.Session.ExpiresAt {
		return conflict(workspaceTerminalAuthority)
	}
	p, err := s.toolProfileTx(ctx, tx, auth.currentPrincipal, project, r.Session.ToolProfileID)
	if err != nil {
		return err
	}
	if !workspaceTerminalProfile(p) || p.Version != r.ToolProfileVersion || p.WorkerID != r.Session.WorkerID || p.Profile.ProfileSHA256 != r.ToolProfileSHA256 {
		return conflict(workspaceTerminalProfileReason)
	}
	p, err = s.toolProfileObservationLockedTx(ctx, tx, tenant, project, p)
	if err != nil {
		return err
	}
	if !p.Available {
		return conflict(workspaceTerminalProfileReason)
	}
	route, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, run)
	if err != nil {
		return err
	}
	current, err := dag.ReadProjectExecutionRouteTx(ctx, tx, s.Dialect, tenant, project)
	if err != nil {
		return err
	}
	if route != r.Route || current != route {
		return conflict(workspaceTerminalChanged)
	}
	if err = s.checkDispatchProfileRouteTx(ctx, tx, tenant, project, p, route); err != nil {
		return err
	}
	if requireLive {
		var state string
		var attempt int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT state,attempt FROM dag_nodes WHERE run_id=? AND id='terminal'`), run).Scan(&state, &attempt); err != nil {
			return err
		}
		if attempt != 1 || state != "running" && state != "cancelling" && state != "reconciling" {
			return conflict(workspaceTerminalChanged)
		}
	}
	return nil
}

func (s *Service) workspaceTerminalProjectionTx(ctx context.Context, auth *authorizedCommand, r workspaceTerminalRecord) (out WorkspaceTerminalSession, err error) {
	if _, err = s.getRecordTx(ctx, auth, r.Session.ProjectID, "tasks", r.Session.TaskID); err != nil {
		return out, err
	}
	if r.Requirement != nil {
		if _, err = s.getRecordTx(ctx, auth, r.Session.ProjectID, "requirements", r.Requirement.ID); err != nil {
			return out, err
		}
	}
	out = WorkspaceTerminalSession(r.Session)
	out.AllowedActions = []string{}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT version FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, r.Session.ProjectID, r.Session.RunID).Scan(&out.RunVersion); err != nil {
		return WorkspaceTerminalSession{}, err
	}
	_, writeErr := s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, r.Session.ProjectID, true, false)
	if writeErr != nil {
		var api *APIError
		if !errors.As(writeErr, &api) || api.Status != 403 {
			return WorkspaceTerminalSession{}, writeErr
		}
		return out, nil
	}
	if !r.TreeStopped && out.State != "exited" && out.State != "stopped" {
		out.AllowedActions = append(out.AllowedActions, "stop")
	}
	if auth.currentPrincipal.ID == r.Creator && out.State == "running" && out.Reason == "" {
		if check := s.checkWorkspaceTerminalExecutionTx(ctx, auth.tx, auth.target.tenantID, out.ProjectID, out.RunID, true); check == nil {
			out.AllowedActions = append(out.AllowedActions, "input", "resize", "ack")
		} else {
			var api *APIError
			if !errors.As(check, &api) {
				return WorkspaceTerminalSession{}, check
			}
			out.Reason = workspaceTerminalChanged
		}
	}
	return out, nil
}

func (s *Service) GetWorkspaceTerminal(ctx context.Context, actor Actor, project, id string) (out WorkspaceTerminalSession, err error) {
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(id, false) {
		return out, invalid(workspaceTerminalInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	r, err := s.workspaceTerminalTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	return s.workspaceTerminalProjectionTx(ctx, auth, r)
}

func (s *Service) ListWorkspaceTerminals(ctx context.Context, actor Actor, project, task string) (out WorkspaceTerminalPage, err error) {
	out.Items, out.Profiles = []WorkspaceTerminalSession{}, []ToolProfile{}
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(task, false) {
		return out, invalid(workspaceTerminalInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, task, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	record, err := s.getRecordTx(ctx, auth, project, "tasks", task)
	if err != nil {
		return out, err
	}
	out.TaskVersion = record.Version
	out.BaselineCurrent = true
	out.CodeBaseline, _, err = s.taskCodeBaselineTx(ctx, auth.tx, auth.target.tenantID, project, task)
	if err != nil {
		return out, err
	}
	if out.CodeBaseline != nil {
		check := s.checkTaskCodeBaselineTx(ctx, auth.tx, auth.target.tenantID, project, *out.CodeBaseline)
		if out.CodeBaseline.TaskVersion != record.Version && check == nil {
			check = conflict(workspaceTerminalChanged)
		}
		if check != nil {
			var api *APIError
			if !errors.As(check, &api) {
				return out, check
			}
			out.BaselineCurrent, out.BaselineReason = false, check.Error()
		}
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT id FROM suma_workspace_terminal_sessions WHERE tenant_id=? AND project_id=? AND task_id=? ORDER BY created_at DESC,id DESC LIMIT 20`), auth.target.tenantID, project, task)
	if err != nil {
		return out, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		r, e := s.workspaceTerminalTx(ctx, auth.tx, auth.target.tenantID, project, id)
		if e != nil {
			return out, e
		}
		view, e := s.workspaceTerminalProjectionTx(ctx, auth, r)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, view)
	}
	profiles, err := s.listToolProfilesForProjectTx(ctx, auth, project, 200, "")
	if err != nil {
		return out, err
	}
	out.ProfilesComplete = profiles.NextCursor == ""
	for _, p := range profiles.Items {
		if workspaceTerminalProfile(p) {
			out.Profiles = append(out.Profiles, p)
		}
	}
	_, writeErr := s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, project, true, false)
	if writeErr == nil {
		out.CanCreate = record.Status != "accepted" && out.BaselineCurrent && s.workspaceTerminalActiveTx(ctx, auth.tx, auth.target.tenantID, project, task) == nil
	} else {
		var api *APIError
		if !errors.As(writeErr, &api) || api.Status != 403 {
			return out, writeErr
		}
	}
	return out, nil
}
