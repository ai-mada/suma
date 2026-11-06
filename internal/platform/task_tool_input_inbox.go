package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

const (
	taskToolInputInboxKind   = "task_tool_input_requested"
	taskToolInputEventPrefix = "tool-input:"
)

var taskToolInputInboxNode = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}(\[[0-9]+\])?$`)
var taskToolInputInboxID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Join only immutable source identifiers and binding metadata. Question and
// answer bodies remain in the protected tool-input read, including on replay.
const taskToolInputInboxJoins = `
 LEFT JOIN suma_tool_inputs ti ON n.kind='task_tool_input_requested' AND ti.id=SUBSTR(n.source_event_key,LENGTH('tool-input:')+1) AND ti.tenant_id=n.tenant_id AND ti.project_id=n.project_id
 LEFT JOIN dag_runs tir ON tir.id=ti.run_id AND tir.tenant_id=ti.tenant_id AND tir.project_id=ti.project_id
 LEFT JOIN suma_task_run_provenance tip ON tip.run_id=tir.id AND tip.tenant_id=tir.tenant_id AND tip.project_id=tir.project_id AND tip.task_id=n.task_id
 LEFT JOIN dag_nodes tin ON tin.run_id=tir.id AND tin.id=ti.node_id
 LEFT JOIN dag_attempts tia ON tia.run_id=tin.run_id AND tia.node_id=tin.id AND tia.attempt=ti.attempt AND tia.fence=ti.fence AND tia.worker_id=ti.worker_id`

const taskToolInputInboxColumns = `,COALESCE(ti.tenant_id,''),COALESCE(ti.project_id,''),COALESCE(ti.id,''),COALESCE(ti.run_id,''),COALESCE(ti.node_id,''),COALESCE(ti.attempt,0),COALESCE(ti.fence,0),COALESCE(ti.worker_id,''),COALESCE(ti.request_sha256,''),COALESCE(tip.payload,''),COALESCE(tir.id,''),COALESCE(tin.id,''),COALESCE(tia.worker_id,'')`

type taskToolInputInboxSource struct {
	tenant, project, task, at string
	id, run, node, worker     string
	requestSHA256             string
	attempt                   int
	fence                     int64
	taskVersion               int
}

func (source taskToolInputInboxSource) eventKey() string {
	return taskToolInputEventPrefix + source.id
}

func (source taskToolInputInboxSource) fingerprint(recipient string) string {
	return digest(stringify([]any{
		"task-tool-input-requested-v1", source.tenant, source.project, source.task, source.taskVersion,
		source.id, source.run, source.node, source.attempt, source.fence, source.worker,
		source.requestSHA256, source.at, recipient,
	}))
}

func (source taskToolInputInboxSource) boundVersion(payload string) int {
	var binding TaskRunProvenance
	if !taskSubscriptionID(source.task) || !taskToolInputInboxID.MatchString(source.id) ||
		!tc.ValidCommandID(source.run) || len(source.node) > 150 || !taskToolInputInboxNode.MatchString(source.node) ||
		source.attempt < 1 || int64(source.attempt) > cc.MaxSafeInteger || source.fence < 1 ||
		!tc.ValidDigest(source.requestSHA256) ||
		json.Unmarshal([]byte(payload), &binding) != nil || binding.BindingKind != "bound" ||
		binding.TaskID != source.task || binding.RunID != source.run || binding.TaskVersion == nil ||
		*binding.TaskVersion < 1 || binding.BoundAt == nil || binding.ObservedAt != *binding.BoundAt {
		return 0
	}
	bound, boundErr := time.Parse(time.RFC3339Nano, *binding.BoundAt)
	opened, openedErr := time.Parse(time.RFC3339Nano, source.at)
	if boundErr != nil || openedErr != nil || opened.UTC().Format(time.RFC3339Nano) != source.at || opened.Before(bound) {
		return 0
	}
	return *binding.TaskVersion
}

func (source taskToolInputInboxSource) available(item NotificationInboxItem, binding, eventKey, recipient, run, node, worker string) bool {
	return eventKey == source.eventKey() && source.run == run && source.node == node &&
		source.worker != "" && source.worker == worker && source.boundVersion(binding) == source.taskVersion &&
		item.SourceResourceID == source.fingerprint(recipient)
}

func toolInputNeedsInbox(request ic.ToolInputRequest) bool {
	if request.Codex != nil {
		return request.Codex.IsBlocking
	}
	return request.Cursor != nil || request.Claude != nil
}

// This runs only for a newly created input inside its original machine
// transaction, after execution, organization and project authority are held.
func (s *Service) projectTaskToolInputInboxTx(ctx context.Context, tx *sql.Tx, row toolInputRow) error {
	input := row.value
	if !toolInputNeedsInbox(input.Request) {
		return nil
	}
	source := taskToolInputInboxSource{
		tenant: row.tenant, project: input.ProjectID, at: input.CreatedAt,
		id: input.ID, run: input.RunID, node: input.NodeID, worker: input.WorkerID,
		requestSHA256: row.requestHash, attempt: input.Attempt, fence: input.Fence,
	}
	var binding, owner string
	var taskVersion int
	err := tx.QueryRowContext(ctx, s.q(`
		SELECT p.task_id,p.payload,t.owner_id,t.version
		FROM suma_task_run_provenance p
		JOIN suma_tasks t ON t.tenant_id=p.tenant_id AND t.project_id=p.project_id AND t.id=p.task_id
		WHERE p.tenant_id=? AND p.project_id=? AND p.run_id=?
	`), source.tenant, source.project, source.run).Scan(&source.task, &binding, &owner, &taskVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	source.taskVersion = source.boundVersion(binding)
	if source.taskVersion == 0 || source.taskVersion > taskVersion {
		return nil
	}

	ownerUser, err := s.notificationHumanOwnerTx(ctx, tx, source.tenant, source.project, owner)
	if err != nil {
		return err
	}
	recipients, err := s.taskToolInputInboxRecipientsTx(ctx, tx, source, ownerUser)
	if err != nil {
		return err
	}
	for _, recipient := range recipients {
		if _, err := s.insertNotificationInboxSourceTx(ctx, tx, source.tenant, source.project, source.at, notificationInboxSource{
			Kind: taskToolInputInboxKind, TaskID: source.task, TaskVersion: source.taskVersion,
			ResourceID: source.fingerprint(recipient), EventKey: source.eventKey(), RecipientUserID: recipient,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) taskToolInputInboxRecipientsTx(ctx context.Context, tx *sql.Tx, source taskToolInputInboxSource, owner string) ([]string, error) {
	var recipients []string
	if owner != "" {
		_, err := s.currentProjectMembershipLockedTx(ctx, tx, source.tenant, owner, source.project, true, false)
		if err == nil {
			recipients = append(recipients, owner)
		} else if !automationAuthorityFailure(err) {
			return nil, err
		}
	}
	rows, err := tx.QueryContext(ctx, s.q(`
		SELECT f.user_id,om.role,pm.role
		FROM suma_task_subscriptions f
		JOIN suma_users u ON u.id=f.user_id
		JOIN suma_org_members om ON om.tenant_id=f.tenant_id AND om.user_id=f.user_id
		LEFT JOIN suma_project_members pm ON pm.project_id=f.project_id AND pm.user_id=f.user_id
		WHERE f.tenant_id=? AND f.project_id=? AND f.task_id=? AND f.following=1
		ORDER BY f.user_id
	`), source.tenant, source.project, source.task)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var user, orgRole string
		var projectRole sql.NullString
		if err = rows.Scan(&user, &orgRole, &projectRole); err != nil {
			return nil, err
		}
		canWrite := orgRole == "owner" || orgRole == "admin" ||
			orgRole == "member" && projectRole.Valid && validProjectRole(projectRole.String) && projectRole.String != "viewer"
		if canWrite && user != owner && taskSubscriptionID(user) {
			recipients = append(recipients, user)
		}
	}
	return recipients, rows.Err()
}
