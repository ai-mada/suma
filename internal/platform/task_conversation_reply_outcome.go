package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	mc "github.com/ai-mada/suma/internal/modelcontract"
	rc "github.com/ai-mada/suma/internal/taskconversationreplycontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func conversationReplyAdmission(message string) error {
	return &APIError{Status: 409, Code: "task_conversation_reply_basis_changed", Message: message}
}

// Callback authority comes from the original live credential, never the stored
// user ID alone. The existing Runner/Pool transaction already holds this exact
// tenant's governance lock; do not widen lock order to other organizations.
func (s *Service) taskConversationReplyCreatorTx(ctx context.Context, tx *sql.Tx, tenant, project string, r taskConversationReplyRecord) (*authorizedCommand, error) {
	budget, ok := s.authorityBudgets.Load(tx)
	if !ok {
		return nil, forbidden()
	}
	b := budget.(*authorityTxBudget)
	c, err := s.readCredential(ctx, tx, r.CreatorCredentialHash, true)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return nil, conversationReplyAdmission(conversationReplyAuthority)
		}
		return nil, err
	}
	at, err := s.authorityNow(ctx, tx)
	if err != nil {
		return nil, err
	}
	if c.kind != actorBrowser && c.kind != actorProjectAPI || c.ref.subjectID != r.Reply.CreatedBy || c.ref.tenantID != tenant || c.kind == actorProjectAPI && c.ref.projectID != project || !c.expires.After(at) {
		return nil, conversationReplyAdmission(conversationReplyAuthority)
	}
	p, err := s.currentProjectMembershipLockedTx(ctx, tx, tenant, c.ref.subjectID, project, true, false)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return nil, conversationReplyAdmission(conversationReplyAuthority)
		}
		return nil, err
	}
	target := commandTarget{tenantID: tenant, projectID: project, resourceID: r.Reply.TaskID}
	return &authorizedCommand{tx: tx, actor: Actor{kind: c.kind, credential: c.ref}, target: target, purpose: purposeProjectWrite, currentPrincipal: p, checkedAt: at, deadline: b.deadline, ctx: b.ctx}, nil
}

func (s *Service) checkTaskConversationReplyExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run string, allowStaleSource ...bool) error {
	r, found, err := s.taskConversationReplyForRunTx(ctx, tx, tenant, project, run)
	if err != nil || !found {
		return err
	}
	a := r.Reply
	if a.State != "running" && a.State != "reconciling" && a.State != "cancelling" {
		return conversationReplyAdmission(conversationReplyVersion)
	}
	var attempt int
	var state string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT attempt,state FROM dag_nodes WHERE run_id=? AND id='reply'`), run).Scan(&attempt, &state); err != nil {
		return err
	}
	if attempt > 1 || attempt > 0 && state != "running" && state != "cancelling" && state != "reconciling" && state != "succeeded" {
		return conversationReplyAdmission(conversationReplyVersion)
	}
	auth, err := s.taskConversationReplyCreatorTx(ctx, tx, tenant, project, r)
	if err != nil {
		return err
	}
	// Claims and heartbeats require current fixed inputs. Original terminal
	// facts enter through the separate result hook, even after source drift.
	if len(allowStaleSource) == 0 || !allowStaleSource[0] {
		if err = s.checkTaskConversationReplySourceTx(ctx, auth, a); err != nil {
			return conversationReplyAdmission(conversationReplyChanged)
		}
	}
	profile, err := s.toolProfileTx(ctx, tx, auth.currentPrincipal, project, a.ToolProfileID)
	if err != nil {
		return err
	}
	if profile.Version != a.ToolProfileVersion || profile.WorkerID != r.WorkerID || profile.Profile.ProfileSHA256 != a.ToolProfileSHA256 || !taskConversationReplyProfile(profile) || a.ModelExecution == nil {
		return conversationReplyAdmission(conversationReplyProfile)
	}
	profile, err = s.toolProfileObservationLockedTx(ctx, tx, tenant, project, profile)
	if err != nil {
		return err
	}
	if !profile.Available {
		return conversationReplyAdmission(conversationReplyProfile)
	}
	route, err := dag.ReadProjectExecutionRouteTx(ctx, tx, s.Dialect, tenant, project)
	if err != nil {
		return err
	}
	bound, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, run)
	if err != nil {
		return err
	}
	if route != a.Source.ExecutionRoute || bound != route {
		return conversationReplyAdmission(conversationReplyChanged)
	}
	selection := mc.Selection{DeploymentID: a.ModelExecution.Deployment.ID, DeploymentRevision: a.ModelExecution.Deployment.Revision, DeploymentSHA256: a.ModelExecution.Deployment.SHA256, Parameters: a.ModelExecution.Parameters, Budget: a.ModelExecution.Budget}
	current, err := s.buildModelExecutionTx(ctx, tx, tenant, project, profile, &selection)
	if err != nil {
		return err
	}
	if stringify(current) != stringify(a.ModelExecution) {
		return conversationReplyAdmission(conversationReplyChanged)
	}
	return s.checkDispatchProfileRouteTx(ctx, tx, tenant, project, profile, route)
}

func taskConversationReplyModelCompleted(result dag.Result, a TaskConversationReply) bool {
	tool := result.ToolEvidence
	if tool == nil || a.ModelExecution == nil || tool.ModelExecution == nil || tool.Terminal != "completed" || tool.Profile.ProfileSHA256 != a.ToolProfileSHA256 || !tc.SupportsTaskConversationReplyProfile(tool.Profile) || tool.UsageState != tc.UsageComplete {
		return false
	}
	proof, b := tool.ModelExecution, a.ModelExecution
	if !mc.EvidenceMatchesBinding(*b, *proof) || proof.GatewayProtocol != mc.GatewayProtocol || !proof.GatewayClosed || len(proof.Requests) != 1 {
		return false
	}
	request := proof.Requests[0]
	return request.State == "completed" && request.Step == 0 && request.ProviderModel == b.Deployment.Model && request.ResponseSHA256 != "" && request.UsageState == tc.UsageComplete
}

func (s *Service) appendTaskConversationReplyMessageTx(ctx context.Context, tx *sql.Tx, tenant string, a TaskConversationReply, body string) (TaskConversationMessage, error) {
	var previous int64
	if err := tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT last_sequence FROM suma_task_conversation_heads WHERE tenant_id=? AND project_id=? AND task_id=?`)), tenant, a.ProjectID, a.TaskID).Scan(&previous); err != nil {
		return TaskConversationMessage{}, err
	}
	if previous >= taskConversationMaxSeq {
		return TaskConversationMessage{}, conflict(taskConversationLimit)
	}
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_task_conversation_heads SET last_sequence=? WHERE tenant_id=? AND project_id=? AND task_id=? AND last_sequence=?`), previous+1, tenant, a.ProjectID, a.TaskID, previous)
	if err != nil {
		return TaskConversationMessage{}, err
	}
	if rows, e := result.RowsAffected(); e != nil || rows != 1 {
		if e != nil {
			return TaskConversationMessage{}, e
		}
		return TaskConversationMessage{}, conflict(conversationReplyVersion)
	}
	message := TaskConversationMessage{ID: digest(a.ID + ":assistant-message")[:32], Sequence: previous + 1, TaskVersion: a.Source.Task.Version, Body: body, AuthorID: a.ToolProfileID, AuthorName: a.AssistantName, ActorKind: "assistant", CreatedAt: a.UpdatedAt, RunID: a.Source.TaskRunID, ReplyID: a.ID, ReplyToSequence: a.Source.ReplyToSequence, ExecutionRunID: a.RunID}
	if a.Source.Requirement != nil {
		message.RequirementID, message.RequirementVersion = a.Source.Requirement.ID, a.Source.Requirement.Version
	}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_conversation_messages(tenant_id,project_id,task_id,`+taskConversationColumns+`)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), tenant, a.ProjectID, a.TaskID, message.ID, message.Sequence, message.TaskVersion, message.Body, message.AuthorID, message.AuthorName, message.ActorKind, message.CreatedAt, message.RequirementID, message.RequirementVersion, message.RunID)
	if err != nil {
		return TaskConversationMessage{}, err
	}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_conversation_reply_messages(message_id,tenant_id,project_id,task_id,reply_id,reply_to_sequence,execution_run_id)VALUES(?,?,?,?,?,?,?)`), message.ID, tenant, a.ProjectID, a.TaskID, a.ID, message.ReplyToSequence, message.ExecutionRunID)
	return message, err
}

func (s *Service) recordTaskConversationReplyOutcomeTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result, authorize func(*sql.Tx) error) error {
	record, found, err := s.taskConversationReplyForRunTx(ctx, tx, tenant, project, result.RunID)
	if err != nil || !found {
		return err
	}
	if authorize == nil {
		return forbidden()
	}
	if err = authorize(tx); err != nil {
		return err
	}
	a := record.Reply
	var attempt int
	var fence int64
	var worker, raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT n.attempt,n.fence,n.worker_id,n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`), tenant, project, result.RunID, result.NodeID).Scan(&attempt, &fence, &worker, &raw)
	if err != nil {
		return err
	}
	if result.NodeID != "reply" || result.Attempt != 1 || result.Attempt != attempt || result.Fence != fence || result.WorkerID != worker || worker != record.WorkerID {
		return forbidden()
	}
	var node dag.Node
	if json.Unmarshal([]byte(raw), &node) != nil || node.Config["purpose"] != "task_conversation_reply" || node.Config["conversation_source_sha256"] != a.Source.SHA256 || node.Config["tool_profile_id"] != a.ToolProfileID || node.Config["tool_profile_sha256"] != a.ToolProfileSHA256 {
		return forbidden()
	}
	bound, err := dag.ModelBinding(node)
	if err != nil || stringify(bound) != stringify(a.ModelExecution) {
		return forbidden()
	}
	resultSHA := digest(stringify(result))
	var count int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_task_conversation_reply_outcomes WHERE reply_id=? AND attempt=? AND fence=? AND result_sha256=?`), a.ID, attempt, fence, resultSHA).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return nil
	}
	if a.State != "running" && a.State != "reconciling" && a.State != "cancelling" {
		return conflict(conversationReplyVersion)
	}
	previous := a.Version
	a.Version++
	a.UpdatedAt, a.AllowedActions = now(), []string{}
	switch {
	case result.OutcomeUnknown:
		a.State, a.Reason = "reconciling", conversationReplyUnknown
	case result.Stopped:
		a.State, a.Reason = "cancelled", ""
	case a.State == "cancelling" || result.ExitCode != 0 || result.Error != "" || !taskConversationReplyModelCompleted(result, a):
		a.State, a.Reason = "failed", conversationReplyFailed
	default:
		output, e := rc.Parse(result.Output)
		if e != nil {
			a.State, a.Reason = "failed", conversationReplyOutput
			break
		}
		auth, e := s.taskConversationReplyCreatorTx(ctx, tx, tenant, project, record)
		if e == nil {
			e = s.checkTaskConversationReplySourceTx(ctx, auth, a)
		}
		if e != nil {
			var api *APIError
			if !errors.As(e, &api) {
				return e
			}
			a.State, a.Reason, a.BasisCurrent = "failed", api.Message, false
			break
		}
		message, e := s.appendTaskConversationReplyMessageTx(ctx, tx, tenant, a, output.Reply)
		if e != nil {
			return e
		}
		a.State, a.Reason, a.MessageID, a.MessageSequence = "completed", "", message.ID, message.Sequence
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_conversation_reply_outcomes(reply_id,attempt,fence,result_sha256,created_at,result)VALUES(?,?,?,?,?,?)`), a.ID, attempt, fence, resultSHA, a.UpdatedAt, stringify(result)); err != nil {
		return err
	}
	if a.State == "completed" || a.State == "failed" || a.State == "cancelled" {
		if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM suma_task_conversation_reply_active WHERE tenant_id=? AND project_id=? AND task_id=? AND reply_id=?`), tenant, project, a.TaskID, a.ID); err != nil {
			return err
		}
	}
	record.Reply = a
	return s.saveTaskConversationReplyTx(ctx, tx, tenant, record, previous)
}
