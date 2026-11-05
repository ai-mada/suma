package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

const (
	taskRunFailedInboxKind   = "task_run_failed"
	taskRunFailedEventPrefix = "run-failed:"
)

// The first run_failed fact is also the source of the once-per-run issue
// receipt. Limit source validation to the selected page and keep joins
// singular even if the same run later fails again.
const taskRunFailureInboxJoins = `
 LEFT JOIN dag_runs fr ON n.kind='task_run_failed' AND fr.id=SUBSTR(n.source_event_key,LENGTH('run-failed:')+1) AND fr.tenant_id=n.tenant_id AND fr.project_id=n.project_id
 LEFT JOIN suma_task_run_provenance fp ON fp.run_id=fr.id AND fp.tenant_id=fr.tenant_id AND fp.project_id=fr.project_id AND fp.task_id=n.task_id
 LEFT JOIN dag_events fe ON fe.sequence=(SELECT MIN(e.sequence) FROM dag_events e WHERE e.tenant_id=fr.tenant_id AND e.project_id=fr.project_id AND e.run_id=fr.id AND e.event_type='run_failed')`

type taskRunFailureInboxSource struct {
	tenant, project, task, run, at string
	sequence                       int64
	taskVersion                    int
}

func (source taskRunFailureInboxSource) eventKey() string {
	return taskRunFailedEventPrefix + source.run
}

// The wire-visible source identifier is opaque even when the bound run
// becomes unavailable and the link must be withheld.
func (source taskRunFailureInboxSource) fingerprint(recipient string) string {
	return digest(stringify([]any{"task-run-failed-v1", source.tenant, source.project, source.task, source.run, source.at, source.sequence, source.taskVersion, recipient}))
}

func (source taskRunFailureInboxSource) boundVersion(payload string) int {
	var binding TaskRunProvenance
	if !taskSubscriptionID(source.task) || !taskSubscriptionID(source.run) || source.sequence < 1 ||
		json.Unmarshal([]byte(payload), &binding) != nil || binding.BindingKind != "bound" ||
		binding.TaskID != source.task || binding.RunID != source.run ||
		binding.TaskVersion == nil || *binding.TaskVersion < 1 || binding.BoundAt == nil || binding.ObservedAt != *binding.BoundAt {
		return 0
	}
	bound, boundErr := time.Parse(time.RFC3339Nano, *binding.BoundAt)
	failed, failedErr := time.Parse(time.RFC3339Nano, source.at)
	if boundErr != nil || failedErr != nil || failed.UTC().Format(time.RFC3339Nano) != source.at || failed.Before(bound) {
		return 0
	}
	return *binding.TaskVersion
}

func (source taskRunFailureInboxSource) available(item NotificationInboxItem, binding, eventKey, recipient string) bool {
	return eventKey == source.eventKey() && source.at == item.CreatedAt &&
		source.boundVersion(binding) == source.taskVersion && item.SourceResourceID == source.fingerprint(recipient)
}

// Called only while writing the original issue receipt under the same
// organization lock. Resolve recipients at projection time, then seal them
// with the receipt so a delayed retry cannot notify newly added recipients.
func (s *Service) projectTaskRunFailureInboxTx(ctx context.Context, tx *sql.Tx, proof *failureProjectionProof) error {
	if err := proof.assertTx(tx); err != nil {
		return err
	}
	source := taskRunFailureInboxSource{tenant: proof.tenant, project: proof.project, run: proof.run, at: proof.occurredAt, sequence: proof.sequence}
	var binding, owner string
	var currentVersion int
	query := `SELECT p.task_id,p.payload,t.owner_id,t.version FROM suma_task_run_provenance p JOIN suma_tasks t ON t.tenant_id=p.tenant_id AND t.project_id=p.project_id AND t.id=p.task_id WHERE p.tenant_id=? AND p.project_id=? AND p.run_id=?`
	err := tx.QueryRowContext(ctx, s.q(query), source.tenant, source.project, source.run).Scan(&source.task, &binding, &owner, &currentVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	source.taskVersion = source.boundVersion(binding)
	if source.taskVersion == 0 || source.taskVersion > currentVersion || !handoffID(source.eventKey(), false) {
		return nil
	}
	ownerUser, err := s.notificationHumanOwnerTx(ctx, tx, source.tenant, source.project, owner)
	if err != nil {
		return err
	}
	recipients, err := s.taskRunFailureInboxRecipientsTx(ctx, tx, source, ownerUser)
	if err != nil {
		return err
	}
	for _, recipient := range recipients {
		if _, err = s.insertNotificationInboxSourceTx(ctx, tx, source.tenant, source.project, source.at, notificationInboxSource{
			Kind: taskRunFailedInboxKind, TaskID: source.task, TaskVersion: source.taskVersion,
			ResourceID: source.fingerprint(recipient), EventKey: source.eventKey(), RecipientUserID: recipient,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) taskRunFailureInboxRecipientsTx(ctx context.Context, tx *sql.Tx, source taskRunFailureInboxSource, owner string) ([]string, error) {
	// The follower relation is unique per user and task. Check the actual
	// roles in Go so database collation cannot expand membership eligibility.
	query := `SELECT f.user_id,om.role,pm.role FROM suma_task_subscriptions f
 JOIN suma_users u ON u.id=f.user_id
 JOIN suma_org_members om ON om.tenant_id=f.tenant_id AND om.user_id=f.user_id
 LEFT JOIN suma_project_members pm ON pm.project_id=f.project_id AND pm.user_id=f.user_id
 WHERE f.tenant_id=? AND f.project_id=? AND f.task_id=? AND f.following=1 ORDER BY f.user_id`
	rows, err := tx.QueryContext(ctx, s.q(query), source.tenant, source.project, source.task)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	recipients := []string{}
	if owner != "" {
		recipients = append(recipients, owner)
	}
	for rows.Next() {
		var user, organizationRole string
		var projectRole sql.NullString
		if err = rows.Scan(&user, &organizationRole, &projectRole); err != nil {
			return nil, err
		}
		if user != owner && taskSubscriptionID(user) && notificationRecipientCanRead(organizationRole, projectRole) {
			recipients = append(recipients, user)
		}
	}
	return recipients, rows.Err()
}
