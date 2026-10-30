package platform

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"
)

const taskConversationKnowledgeInvalid = "Invalid task conversation knowledge references"
const taskConversationKnowledgeSource = "The fixed task conversation knowledge source is invalid"
const taskConversationKnowledgeConflict = "Selected messages reference different revisions of the same knowledge"
const taskConversationKnowledgeLimit = "The selected conversation knowledge exceeds 16 unique references"

func validTaskConversationKnowledgeReferences(refs []ProjectKnowledgeReference) bool {
	if len(refs) > 16 {
		return false
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		if !handoffID(ref.ID, false) || ref.Revision < 1 || ref.Revision > 2147483647 || !contentDigest.MatchString(ref.SHA256) || seen[ref.ID] {
			return false
		}
		seen[ref.ID] = true
	}
	return true
}

func validTaskConversationKnowledgeMetadata(ref TaskConversationKnowledgeReference) bool {
	return validTaskConversationKnowledgeReferences([]ProjectKnowledgeReference{{ID: ref.ID, Revision: ref.Revision, SHA256: ref.SHA256}}) && knowledgeKind(ref.KindAtPost) && utf8.ValidString(ref.TitleAtPost) && strings.TrimSpace(ref.TitleAtPost) != "" && len(ref.TitleAtPost) <= 200
}

// Called only for a newly inserted human/API message after original receipt
// recovery. All publication resolution and relation writes share that message's
// authority transaction; a failed pin rolls back its sequence and receipt too.
func (s *Service) projectTaskConversationKnowledgeTx(ctx context.Context, auth *authorizedCommand, project, taskID string, message *TaskConversationMessage, refs []ProjectKnowledgeReference) error {
	if !validTaskConversationKnowledgeReferences(refs) {
		return invalid(taskConversationKnowledgeInvalid)
	}
	if len(refs) == 0 {
		return nil
	}
	if auth == nil || message == nil || !taskConversationActor(auth.actor) {
		return forbidden()
	}
	if err := assertProjectCommand(auth.tx, auth, project); err != nil {
		return err
	}
	if message.ActorKind != "human" && message.ActorKind != "api" || message.AuthorID != auth.currentPrincipal.ID || len(message.Knowledge) != 0 {
		return conflict(taskConversationKnowledgeSource)
	}
	saved, err := scanTaskConversation(auth.tx.QueryRowContext(ctx, s.q(`SELECT `+taskConversationColumns+` FROM suma_task_conversation_messages WHERE tenant_id=? AND project_id=? AND task_id=? AND id=?`), auth.target.tenantID, project, taskID, message.ID))
	if errors.Is(err, sql.ErrNoRows) {
		return conflict(taskConversationKnowledgeSource)
	}
	if err != nil {
		return err
	}
	// Mention metadata is an independent relation projected in the same
	// transaction. Compare the saved base message without either optional set.
	base := *message
	base.Mentions, base.Knowledge = nil, nil
	if stringify(saved) != stringify(base) {
		return conflict(taskConversationKnowledgeSource)
	}
	var version int
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT version FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, taskID).Scan(&version); errors.Is(err, sql.ErrNoRows) || err == nil && version != message.TaskVersion {
		return conflict(taskConversationKnowledgeSource)
	} else if err != nil {
		return err
	}
	snapshots, err := s.resolvePublishedKnowledgeTx(ctx, auth, project, refs)
	if err != nil {
		return err
	}
	metadata := make([]TaskConversationKnowledgeReference, 0, len(snapshots))
	for ordinal, snapshot := range snapshots {
		ref := TaskConversationKnowledgeReference{ID: snapshot.ID, Revision: snapshot.Revision, SHA256: snapshot.SHA256, TitleAtPost: snapshot.Title, KindAtPost: snapshot.Kind}
		if !validTaskConversationKnowledgeMetadata(ref) {
			return conflict(taskConversationKnowledgeSource)
		}
		if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_conversation_knowledge(tenant_id,project_id,task_id,message_id,ordinal,knowledge_id,revision,sha256,title_at_post,kind_at_post) VALUES(?,?,?,?,?,?,?,?,?,?)`), auth.target.tenantID, project, taskID, message.ID, ordinal, ref.ID, ref.Revision, ref.SHA256, ref.TitleAtPost, ref.KindAtPost); err != nil {
			return err
		}
		metadata = append(metadata, ref)
	}
	message.Knowledge = metadata
	return nil
}

// History uses only immutable relation metadata. Current publication eligibility
// is checked separately for each new model/development consumption, not here.
func (s *Service) taskConversationKnowledgeMetadataTx(ctx context.Context, tx *sql.Tx, tenant, project, task string, items []TaskConversationMessage) error {
	if len(items) == 0 {
		return nil
	}
	args := []any{tenant, project, task}
	markers := make([]string, len(items))
	positions := map[string]int{}
	seen := make([]map[string]bool, len(items))
	for i, item := range items {
		markers[i] = "?"
		args = append(args, item.ID)
		positions[item.ID] = i
		items[i].Knowledge = nil
		seen[i] = map[string]bool{}
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT message_id,ordinal,knowledge_id,revision,sha256,title_at_post,kind_at_post FROM suma_task_conversation_knowledge WHERE tenant_id=? AND project_id=? AND task_id=? AND message_id IN (`+strings.Join(markers, ",")+`) ORDER BY message_id,ordinal`), args...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var message string
		var ordinal int
		var ref TaskConversationKnowledgeReference
		if err = rows.Scan(&message, &ordinal, &ref.ID, &ref.Revision, &ref.SHA256, &ref.TitleAtPost, &ref.KindAtPost); err != nil {
			rows.Close()
			return err
		}
		position, ok := positions[message]
		if !ok || items[position].ActorKind != "human" && items[position].ActorKind != "api" || ordinal != len(items[position].Knowledge) || ordinal >= 16 || !validTaskConversationKnowledgeMetadata(ref) || seen[position][ref.ID] {
			rows.Close()
			return conflict(taskConversationKnowledgeSource)
		}
		seen[position][ref.ID] = true
		items[position].Knowledge = append(items[position].Knowledge, ref)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	// Close the bounded relation read before looking up any backing reply. This
	// also avoids nested SQLite reads on its single connection. No recursion
	// through Source.Messages is needed: each verified reply sealed its union.
	for i := range items {
		if items[i].ActorKind == "assistant" {
			if err = s.taskConversationAssistantKnowledgeTx(ctx, tx, tenant, project, task, &items[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) taskConversationAssistantKnowledgeTx(ctx context.Context, tx *sql.Tx, tenant, project, task string, message *TaskConversationMessage) error {
	var replyID, runID string
	var replyTo int64
	err := tx.QueryRowContext(ctx, s.q(`SELECT reply_id,reply_to_sequence,execution_run_id FROM suma_task_conversation_reply_messages WHERE tenant_id=? AND project_id=? AND task_id=? AND message_id=?`), tenant, project, task, message.ID).Scan(&replyID, &replyTo, &runID)
	if errors.Is(err, sql.ErrNoRows) {
		return conflict(taskConversationKnowledgeSource)
	}
	if err != nil {
		return err
	}
	if message.ReplyID != replyID || message.ReplyToSequence != replyTo || message.ExecutionRunID != runID {
		return conflict(taskConversationKnowledgeSource)
	}
	record, err := s.taskConversationReplyTx(ctx, tx, tenant, project, replyID)
	if err != nil {
		return err
	}
	reply := record.Reply
	source := reply.Source
	sealedHash := source.SHA256
	source.SHA256 = ""
	if reply.State != "completed" || reply.TaskID != task || reply.MessageID != message.ID || reply.MessageSequence != message.Sequence || reply.RunID != runID || reply.Source.ReplyToSequence != replyTo || reply.Source.Task.ID != task || reply.Source.Task.Version != message.TaskVersion || reply.Source.TaskRunID != message.RunID || !contentDigest.MatchString(sealedHash) || digest(stringify(source)) != sealedHash || !validTaskConversationKnowledgeReferences(knowledgeReferences(source.Knowledge)) {
		return conflict(taskConversationKnowledgeSource)
	}
	if err = checkTaskConversationKnowledgeSnapshot(source.Messages, source.Knowledge); err != nil {
		return err
	}
	for _, snapshot := range source.Knowledge {
		ref := TaskConversationKnowledgeReference{ID: snapshot.ID, Revision: snapshot.Revision, SHA256: snapshot.SHA256, TitleAtPost: snapshot.Title, KindAtPost: snapshot.Kind}
		if !validTaskConversationKnowledgeMetadata(ref) {
			return conflict(taskConversationKnowledgeSource)
		}
		message.Knowledge = append(message.Knowledge, ref)
	}
	return nil
}

func taskConversationKnowledgeReferences(messages []TaskConversationMessage) ([]ProjectKnowledgeReference, error) {
	var refs []ProjectKnowledgeReference
	seen := map[string]ProjectKnowledgeReference{}
	for _, message := range messages {
		if len(message.Knowledge) > 16 {
			return nil, invalid(taskConversationKnowledgeLimit)
		}
		local := map[string]bool{}
		for _, metadata := range message.Knowledge {
			if !validTaskConversationKnowledgeMetadata(metadata) || local[metadata.ID] {
				return nil, conflict(taskConversationKnowledgeSource)
			}
			local[metadata.ID] = true
			ref := ProjectKnowledgeReference{ID: metadata.ID, Revision: metadata.Revision, SHA256: metadata.SHA256}
			if previous, exists := seen[ref.ID]; exists {
				if previous != ref {
					return nil, conflict(taskConversationKnowledgeConflict)
				}
				continue
			}
			if len(refs) == 16 {
				return nil, invalid(taskConversationKnowledgeLimit)
			}
			seen[ref.ID] = ref
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

// A sealed hash proves the stored source bytes agree with that hash, but does
// not prove its knowledge list includes every dependency carried by messages.
// Compare the exact ordered pins without consulting current publication state:
// history stays immutable, while fresh consumption reauthorizes separately.
func checkTaskConversationKnowledgeSnapshot(messages []TaskConversationMessage, knowledge []ProjectKnowledgeContextItem) error {
	expected, err := taskConversationKnowledgeReferences(messages)
	if err != nil {
		return err
	}
	actual := knowledgeReferences(knowledge)
	if !validTaskConversationKnowledgeReferences(actual) || len(expected) != len(actual) {
		return conflict(taskConversationKnowledgeSource)
	}
	for i := range expected {
		if expected[i] != actual[i] {
			return conflict(taskConversationKnowledgeSource)
		}
	}
	return nil
}
