package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
)

const (
	conversationReplyInvalid   = "Invalid task conversation reply request"
	conversationReplyChanged   = "The conversation reply basis changed; refresh the task and messages"
	conversationReplyActive    = "This task already has an active or uncertain assistant reply"
	conversationReplyAnswered  = "This message already has a completed assistant reply; add a new message to continue"
	conversationReplyProfile   = "Select a current enabled read-only task conversation reply profile"
	conversationReplyConsent   = "Confirm that the fixed task and conversation context will be sent to the selected model"
	conversationReplyContext   = "The conversation context exceeds its bounded model request; use a smaller task or message context"
	conversationReplyAuthority = "The original reply creator no longer has current execution authority"
	conversationReplyUnknown   = "The assistant reply outcome is unknown; inspect the original execution without sending it again"
	conversationReplyFailed    = "The assistant reply did not complete successfully"
	conversationReplyOutput    = "The model response does not satisfy the task conversation reply contract"
	conversationReplyVersion   = "The assistant reply changed; reload before deciding"
	conversationReplyControls  = "Conversation replies must use their dedicated controls"
)

type taskConversationReplyRecord struct {
	Reply                 TaskConversationReply `json:"reply"`
	CreatorCredentialHash string                `json:"creator_credential_hash"`
	WorkerID              string                `json:"worker_id"`
}

func (s *Service) InitializeTaskConversationReplies(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS suma_task_conversation_replies(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,run_id VARCHAR(100) NOT NULL UNIQUE,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_task_conversation_reply_active(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,reply_id VARCHAR(64) NOT NULL UNIQUE,PRIMARY KEY(tenant_id,project_id,task_id))`,
		`CREATE TABLE IF NOT EXISTS suma_task_conversation_reply_outcomes(reply_id VARCHAR(64) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,result_sha256 VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,result TEXT NOT NULL,PRIMARY KEY(reply_id,attempt,fence,result_sha256))`,
		`CREATE TABLE IF NOT EXISTS suma_task_conversation_reply_messages(message_id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,reply_id VARCHAR(64) NOT NULL UNIQUE,reply_to_sequence BIGINT NOT NULL,execution_run_id VARCHAR(100) NOT NULL)`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(statement)); err != nil {
			return err
		}
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_task_conversation_reply_page", "suma_task_conversation_replies", "tenant_id", "project_id", "task_id", "created_at", "id")
}

func (s *Service) taskConversationReplyTx(ctx context.Context, tx governanceQuery, tenant, project, id string) (out taskConversationReplyRecord, err error) {
	var raw, task, state, run string
	var version int
	err = tx.QueryRowContext(ctx, s.q(`SELECT version,task_id,state,run_id,payload FROM suma_task_conversation_replies WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&version, &task, &state, &run, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	a := out.Reply
	if a.ID != id || a.ProjectID != project || a.TaskID != task || a.Version != version || a.State != state || a.RunID != run {
		return out, conflict(conversationReplyVersion)
	}
	return out, nil
}

func (s *Service) saveTaskConversationReplyTx(ctx context.Context, tx *sql.Tx, tenant string, r taskConversationReplyRecord, previous int) error {
	a := r.Reply
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_task_conversation_replies SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), a.Version, a.State, stringify(r), tenant, a.ProjectID, a.ID, previous)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err == nil && rows != 1 {
		return conflict(conversationReplyVersion)
	}
	return err
}

func (s *Service) taskConversationReplyForRunTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) (out taskConversationReplyRecord, found bool, err error) {
	var id string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_task_conversation_replies WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, run).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		// A declared purpose without the protected platform relation is never a
		// generic agent execution, even if a caller supplied a matching config.
		rows, e := tx.QueryContext(ctx, s.q(`SELECT n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND r.id=?`), tenant, project, run)
		if e != nil {
			return out, false, e
		}
		defer rows.Close()
		for rows.Next() {
			var raw string
			if e = rows.Scan(&raw); e != nil {
				return out, false, e
			}
			var node dag.Node
			if json.Unmarshal([]byte(raw), &node) != nil {
				return out, false, conflict(conversationReplyInvalid)
			}
			if node.Config["purpose"] == "task_conversation_reply" {
				return out, false, forbidden()
			}
		}
		return out, false, rows.Err()
	}
	if err != nil {
		return out, false, err
	}
	out, err = s.taskConversationReplyTx(ctx, tx, tenant, project, id)
	return out, true, err
}

// Associate assistant attribution only through the new protected relation.
// Human/API rows have no reply metadata by definition; no table alteration or
// historical row backfill is needed, and an unbacked assistant is rejected.
func (s *Service) taskConversationMessageMetadataTx(ctx context.Context, tx *sql.Tx, tenant, project, task string, items []TaskConversationMessage) error {
	if len(items) == 0 {
		return nil
	}
	args := []any{tenant, project, task}
	markers := make([]string, len(items))
	positions := map[string]int{}
	for i, item := range items {
		markers[i] = "?"
		args = append(args, item.ID)
		positions[item.ID] = i
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT message_id,reply_id,reply_to_sequence,execution_run_id FROM suma_task_conversation_reply_messages WHERE tenant_id=? AND project_id=? AND task_id=? AND message_id IN (`+strings.Join(markers, ",")+`)`), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, reply, run string
		var target int64
		if err = rows.Scan(&id, &reply, &target, &run); err != nil {
			return err
		}
		index, ok := positions[id]
		if !ok || items[index].ActorKind != "assistant" || !handoffID(reply, false) || target < 1 || target > taskConversationMaxSeq || !diagnosisID(run, 100) {
			return conflict(conversationReplyInvalid)
		}
		items[index].ReplyID, items[index].ReplyToSequence, items[index].ExecutionRunID = reply, target, run
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, item := range items {
		if item.ActorKind == "assistant" && item.ReplyID == "" || item.ActorKind != "assistant" && item.ActorKind != "human" && item.ActorKind != "api" {
			return conflict(conversationReplyInvalid)
		}
	}
	if err = s.taskConversationMentionsMetadataTx(ctx, tx, tenant, project, task, items); err != nil {
		return err
	}
	if err = s.taskConversationKnowledgeMetadataTx(ctx, tx, tenant, project, task, items); err != nil {
		return err
	}
	return s.taskConversationCloudObservationMetadataTx(ctx, tx, tenant, project, task, items)
}
