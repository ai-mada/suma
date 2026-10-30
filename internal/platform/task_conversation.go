package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	taskConversationInvalid = "Invalid task conversation request"
	taskConversationVersion = "The task has changed; reload its context before posting"
	taskConversationLimit   = "The task conversation sequence limit has been reached"
	taskConversationMaxSeq  = int64(9007199254740991)
)

func (s *Service) InitializeTaskConversations(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS suma_task_conversation_heads(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,last_sequence BIGINT NOT NULL,PRIMARY KEY(tenant_id,project_id,task_id))`,
		`CREATE TABLE IF NOT EXISTS suma_task_conversation_messages(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,sequence BIGINT NOT NULL,task_version INTEGER NOT NULL,body TEXT NOT NULL,author_id VARCHAR(64) NOT NULL,author_name VARCHAR(200) NOT NULL,actor_kind VARCHAR(20) NOT NULL,created_at VARCHAR(40) NOT NULL,requirement_id VARCHAR(64) NOT NULL,requirement_version INTEGER NOT NULL,run_id VARCHAR(100) NOT NULL,UNIQUE(tenant_id,project_id,task_id,sequence))`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(statement)); err != nil {
			return err
		}
	}
	return nil
}

func taskConversationActor(actor Actor) bool {
	return actor.kind == actorBrowser || actor.kind == actorProjectAPI
}

func validTaskConversationAppend(req TaskConversationAppendRequest) bool {
	return handoffID(req.ProjectID, false) && handoffID(req.TaskID, false) && req.ExpectedTaskVersion > 0 && req.ExpectedTaskVersion <= 2147483646 &&
		utf8.ValidString(req.Body) && len(req.Body) <= 32000 && utf8.RuneCountInString(req.Body) <= 8000 &&
		strings.TrimSpace(req.Body) != "" && !strings.ContainsRune(req.Body, 0) && planningKey(req.IdempotencyKey) && validTaskConversationMentionIDs(req.MentionResourceIDs) && validTaskConversationKnowledgeReferences(req.Knowledge)
}

const taskConversationColumns = `id,sequence,task_version,body,author_id,author_name,actor_kind,created_at,requirement_id,requirement_version,run_id`

func scanTaskConversation(row scanner) (out TaskConversationMessage, err error) {
	err = row.Scan(&out.ID, &out.Sequence, &out.TaskVersion, &out.Body, &out.AuthorID, &out.AuthorName, &out.ActorKind, &out.CreatedAt, &out.RequirementID, &out.RequirementVersion, &out.RunID)
	return out, err
}

// ListTaskConversation is strictly observational: even a task's first visit
// cannot create a thread, activity, receipt or execution. Current write rights
// are only a UI hint; AppendTaskConversation authorizes independently.
func (s *Service) ListTaskConversation(ctx context.Context, actor Actor, project, taskID string, limit int, before int64) (out TaskConversationPage, err error) {
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || !handoffID(taskID, false) || limit < 1 || limit > 50 || before < 0 || before > taskConversationMaxSeq {
		return out, invalid(taskConversationInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, taskID, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	task, err := s.getRecordTx(ctx, auth, project, "tasks", taskID)
	if err != nil {
		return out, err
	}
	out = TaskConversationPage{ProjectID: project, TaskID: taskID, TaskVersion: task.Version, Items: []TaskConversationMessage{}, Limit: limit}
	_, e := s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, project, true, false)
	if e == nil {
		out.CanPost = true
	} else {
		var api *APIError
		if !errors.As(e, &api) || api.Status != 403 {
			return TaskConversationPage{}, e
		}
	}
	args := []any{auth.target.tenantID, project, taskID}
	base := ` FROM suma_task_conversation_messages WHERE tenant_id=? AND project_id=? AND task_id=?`
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+base), args...).Scan(&out.Total); err != nil {
		return TaskConversationPage{}, err
	}
	if before != 0 {
		base += ` AND sequence<?`
		args = append(args, before)
	}
	args = append(args, limit+1)
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT `+taskConversationColumns+base+` ORDER BY sequence DESC LIMIT ?`), args...)
	if err != nil {
		return TaskConversationPage{}, err
	}
	for rows.Next() {
		item, e := scanTaskConversation(rows)
		if e != nil {
			rows.Close()
			return TaskConversationPage{}, e
		}
		out.Items = append(out.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return TaskConversationPage{}, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextBefore = out.Items[len(out.Items)-1].Sequence
	}
	for left, right := 0, len(out.Items)-1; left < right; left, right = left+1, right-1 {
		out.Items[left], out.Items[right] = out.Items[right], out.Items[left]
	}
	if err = s.taskConversationMessageMetadataTx(ctx, auth.tx, auth.target.tenantID, project, taskID, out.Items); err != nil {
		return TaskConversationPage{}, err
	}
	return out, auth.assertTx(auth.tx, auth.target, purposeProjectRead)
}

// AppendTaskConversation changes only the collaboration ledger. The current
// authority/project lock serializes append sequences across service replicas;
// the unique constraint and CAS remain final database guards. A retry recovers
// the original immutable message before task-version checks, after current
// authorization, so a lost response is not an invitation to post twice.
func (s *Service) AppendTaskConversation(ctx context.Context, actor Actor, project string, req TaskConversationAppendRequest) (out TaskConversationMessage, err error) {
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if req.ProjectID != project || !validTaskConversationAppend(req) {
		return out, invalid(taskConversationInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, req.TaskID, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"task_conversation_append": req})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		return out, auth.assertTx(auth.tx, auth.target, purposeProjectWrite)
	}
	task, err := s.getRecordTx(ctx, auth, project, "tasks", req.TaskID)
	if err != nil {
		return out, err
	}
	if task.Version != req.ExpectedTaskVersion {
		return out, conflict(taskConversationVersion)
	}
	requirementVersion := 0
	if task.RequirementID != "" {
		requirement, e := s.getRecordTx(ctx, auth, project, "requirements", task.RequirementID)
		if e != nil {
			return out, e
		}
		requirementVersion = requirement.Version
	}
	var previous int64
	err = auth.tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT last_sequence FROM suma_task_conversation_heads WHERE tenant_id=? AND project_id=? AND task_id=?`)), auth.target.tenantID, project, task.ID).Scan(&previous)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_conversation_heads(tenant_id,project_id,task_id,last_sequence)VALUES(?,?,?,0)`), auth.target.tenantID, project, task.ID)
	}
	if err != nil {
		return out, err
	}
	if previous >= taskConversationMaxSeq {
		return out, conflict(taskConversationLimit)
	}
	changed, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_task_conversation_heads SET last_sequence=? WHERE tenant_id=? AND project_id=? AND task_id=? AND last_sequence=?`), previous+1, auth.target.tenantID, project, task.ID, previous)
	if err != nil {
		return out, err
	}
	if rows, e := changed.RowsAffected(); e != nil || rows != 1 {
		if e != nil {
			return out, e
		}
		return out, conflict(taskConversationInvalid)
	}
	kind := "human"
	if actor.kind == actorProjectAPI {
		kind = "api"
	}
	out = TaskConversationMessage{ID: newID(), Sequence: previous + 1, TaskVersion: task.Version, Body: req.Body,
		AuthorID: auth.currentPrincipal.ID, AuthorName: auth.currentPrincipal.Name, ActorKind: kind,
		CreatedAt: auth.checkedAt.Format(time.RFC3339Nano), RequirementID: task.RequirementID, RequirementVersion: requirementVersion, RunID: task.RunID}
	_, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_conversation_messages(tenant_id,project_id,task_id,`+taskConversationColumns+`)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		auth.target.tenantID, project, task.ID, out.ID, out.Sequence, out.TaskVersion, out.Body, out.AuthorID, out.AuthorName, out.ActorKind, out.CreatedAt, out.RequirementID, out.RequirementVersion, out.RunID)
	if err != nil {
		return TaskConversationMessage{}, err
	}
	if err = s.projectTaskConversationMentionsTx(ctx, auth, project, task.ID, &out, req.MentionResourceIDs); err != nil {
		return TaskConversationMessage{}, err
	}
	if err = s.projectTaskConversationKnowledgeTx(ctx, auth, project, task.ID, &out, req.Knowledge); err != nil {
		return TaskConversationMessage{}, err
	}
	if err = s.projectTaskSubscriptionMessageTx(ctx, auth, project, task.ID, out.ID); err != nil {
		return TaskConversationMessage{}, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return TaskConversationMessage{}, err
	}
	// A fixed activity title keeps user message text out of general activity and
	// telemetry streams; the task-authorized conversation is its sole read path.
	if err = s.commandActivity(ctx, auth, "task_conversation.posted", task.ID, "Task conversation message"); err != nil {
		return TaskConversationMessage{}, err
	}
	if err = auth.assertTx(auth.tx, auth.target, purposeProjectWrite); err != nil {
		return TaskConversationMessage{}, err
	}
	return out, auth.tx.Commit()
}
