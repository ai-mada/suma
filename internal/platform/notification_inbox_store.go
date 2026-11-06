package platform

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/ai-mada/suma/internal/store"
)

const notificationInboxInvalid = "Invalid notification inbox request or cursor"
const notificationInboxChanged = "The notification has changed; reload before deciding"
const notificationInboxSourceChanged = "The task assignment source does not match the saved task"
const notificationInboxSequenceFailed = "The notification sequence is invalid or exhausted"
const notificationInboxRowsDDL = `CREATE TABLE IF NOT EXISTS suma_notification_inbox(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,recipient_user_id VARCHAR(64) NOT NULL,sequence BIGINT NOT NULL,kind VARCHAR(40) NOT NULL,task_id VARCHAR(64) NOT NULL,source_task_version INTEGER NOT NULL,source_resource_id VARCHAR(64) NOT NULL,source_event_key VARCHAR(64) NOT NULL,version INTEGER NOT NULL,created_at VARCHAR(40) NOT NULL,read_at VARCHAR(40) NOT NULL,archived_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL,UNIQUE(tenant_id,project_id,source_event_key,recipient_user_id),UNIQUE(tenant_id,project_id,sequence))`
const notificationInboxSequenceDDL = `CREATE TABLE IF NOT EXISTS suma_notification_inbox_sequences(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,last_sequence BIGINT NOT NULL,PRIMARY KEY(tenant_id,project_id))`

func (s *Service) InitializeNotificationInbox(ctx context.Context) error {
	if err := s.validateNotificationInboxInitialSchema(ctx); err != nil {
		return err
	}
	for _, ddl := range []string{notificationInboxRowsDDL, notificationInboxSequenceDDL} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
			return err
		}
	}
	if err := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_notification_inbox_page", "suma_notification_inbox", "tenant_id", "project_id", "recipient_user_id", "sequence"); err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_notification_inbox_unread", "suma_notification_inbox", "tenant_id", "project_id", "recipient_user_id", "archived_at", "read_at", "sequence")
}

// Called only from an original task write transaction. Recipient lookups are
// ordinary SELECTs under its organization authority serialization: taking a
// new recipient identity lock here would invert the established lock order.
func (s *Service) projectTaskAssignmentNotificationTx(ctx context.Context, auth *authorizedCommand, oldOwner string, nextTask Record) error {
	if auth == nil {
		return forbidden()
	}
	if err := assertProjectCommand(auth.tx, auth, nextTask.ProjectID); err != nil {
		return err
	}
	if nextTask.TenantID != auth.target.tenantID || !handoffID(nextTask.ID, false) || nextTask.Version < 1 {
		return forbidden()
	}
	var savedVersion int
	var savedOwner string
	err := auth.tx.QueryRowContext(ctx, s.q(`SELECT version,owner_id FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, nextTask.ProjectID, nextTask.ID).Scan(&savedVersion, &savedOwner)
	if errors.Is(err, sql.ErrNoRows) || err == nil && (savedVersion != nextTask.Version || savedOwner != nextTask.OwnerID) {
		return conflict(notificationInboxSourceChanged)
	}
	if err != nil {
		return err
	}
	if nextTask.OwnerID == "" || nextTask.OwnerID == oldOwner {
		return nil
	}
	user, err := s.notificationHumanOwnerTx(ctx, auth.tx, auth.target.tenantID, nextTask.ProjectID, nextTask.OwnerID)
	if err != nil || user == "" {
		return err
	}
	eventKey := digest(stringify(struct {
		Task     string
		Version  int
		Resource string
	}{nextTask.ID, nextTask.Version, nextTask.OwnerID}))
	_, err = s.insertNotificationInboxTx(ctx, auth, nextTask.ProjectID, notificationInboxSource{Kind: "task_assigned", TaskID: nextTask.ID, TaskVersion: nextTask.Version, ResourceID: nextTask.OwnerID, EventKey: eventKey, RecipientUserID: user})
	return err
}

func (s *Service) notificationHumanOwnerTx(ctx context.Context, tx *sql.Tx, tenant, project, owner string) (string, error) {
	if owner == "" {
		return "", nil
	}
	member, err := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_members WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, owner))
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	user, _ := member.Data["user_id"].(string)
	if member.Kind != "human" || member.Status != "active" || !handoffID(user, false) {
		return "", nil
	}
	var canonical, organizationRole string
	var projectRole sql.NullString
	err = tx.QueryRowContext(ctx, s.q(`SELECT h.resource_id,om.role,pm.role FROM suma_human_resources h JOIN suma_users u ON u.id=h.user_id JOIN suma_org_members om ON om.tenant_id=h.tenant_id AND om.user_id=h.user_id LEFT JOIN suma_project_members pm ON pm.project_id=h.project_id AND pm.user_id=h.user_id WHERE h.tenant_id=? AND h.project_id=? AND h.user_id=?`), tenant, project, user).Scan(&canonical, &organizationRole, &projectRole)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if canonical != member.ID || !notificationRecipientCanRead(organizationRole, projectRole) {
		return "", nil
	}
	return user, nil
}

func notificationRecipientCanRead(organizationRole string, projectRole sql.NullString) bool {
	return organizationRole == "owner" || organizationRole == "admin" || organizationRole == "member" && projectRole.Valid && validProjectRole(projectRole.String)
}

func validNotificationInboxKind(kind string) bool {
	switch kind {
	case "task_assigned", "task_mentioned", "task_review_requested", "task_handoff_requested", "task_followed", taskRunFailedInboxKind, taskToolInputInboxKind:
		return true
	default:
		return false
	}
}

// Source callers validate their persisted business event before reaching this
// sole sequence/row writer. It cannot be invoked through a public command.
type notificationInboxSource struct {
	Kind, TaskID, ResourceID, EventKey, RecipientUserID string
	TaskVersion                                         int
}

func (s *Service) insertNotificationInboxTx(ctx context.Context, auth *authorizedCommand, project string, source notificationInboxSource) (string, error) {
	if auth == nil {
		return "", forbidden()
	}
	if err := assertProjectCommand(auth.tx, auth, project); err != nil {
		return "", err
	}
	return s.insertNotificationInboxSourceTx(ctx, auth.tx, auth.target.tenantID, project, auth.checkedAt.UTC().Format(time.RFC3339Nano), source)
}

// Trusted domain projections share one sequence/row writer. The ordinary
// authority wrapper above stays mandatory for public actor commands; the
// completed-assistant path validates its persisted backing event separately
// under the original machine transaction before calling this private core.
func (s *Service) insertNotificationInboxSourceTx(ctx context.Context, tx *sql.Tx, tenant, project, at string, source notificationInboxSource) (string, error) {
	if tx == nil || tenant == "" || project == "" || at == "" {
		return "", forbidden()
	}
	if !validNotificationInboxKind(source.Kind) || !handoffID(source.TaskID, false) || !handoffID(source.ResourceID, false) || !handoffID(source.RecipientUserID, false) || !handoffID(source.EventKey, false) || source.TaskVersion < 1 {
		return "", conflict(notificationInboxSourceChanged)
	}
	var existing string
	err := tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_notification_inbox WHERE tenant_id=? AND project_id=? AND source_event_key=? AND recipient_user_id=? AND kind=? AND task_id=? AND source_task_version=? AND source_resource_id=?`), tenant, project, source.EventKey, source.RecipientUserID, source.Kind, source.TaskID, source.TaskVersion, source.ResourceID).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	insert := `INSERT INTO suma_notification_inbox_sequences(tenant_id,project_id,last_sequence) VALUES(?,?,0) ON CONFLICT(tenant_id,project_id) DO NOTHING`
	if s.Dialect == "oceanbase" {
		insert = `INSERT IGNORE INTO suma_notification_inbox_sequences(tenant_id,project_id,last_sequence) VALUES(?,?,0)`
	}
	if _, err = tx.ExecContext(ctx, s.q(insert), tenant, project); err != nil {
		return "", err
	}
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_notification_inbox_sequences SET last_sequence=last_sequence+1 WHERE tenant_id=? AND project_id=? AND last_sequence>=0 AND last_sequence<?`), tenant, project, int64(1<<63-1))
	if err != nil {
		return "", err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if count != 1 {
		return "", conflict(notificationInboxSequenceFailed)
	}
	var sequence int64
	if err = tx.QueryRowContext(ctx, s.q(`SELECT last_sequence FROM suma_notification_inbox_sequences WHERE tenant_id=? AND project_id=?`), tenant, project).Scan(&sequence); err != nil {
		return "", err
	}
	if sequence < 1 {
		return "", conflict(notificationInboxSequenceFailed)
	}
	id := newID()
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_notification_inbox(id,tenant_id,project_id,recipient_user_id,sequence,kind,task_id,source_task_version,source_resource_id,source_event_key,version,created_at,read_at,archived_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,1,?,'','',?)`), id, tenant, project, source.RecipientUserID, sequence, source.Kind, source.TaskID, source.TaskVersion, source.ResourceID, source.EventKey, at, at)
	return id, err
}

type notificationInboxStored struct {
	id                            string
	version                       int
	readAt, archivedAt, updatedAt string
}

func (s *Service) ownNotificationInboxTx(ctx context.Context, auth *authorizedCommand, project, id string) (out notificationInboxStored, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return out, err
	}
	err = auth.tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT id,version,read_at,archived_at,updated_at FROM suma_notification_inbox WHERE tenant_id=? AND project_id=? AND recipient_user_id=? AND id=?`)), auth.target.tenantID, project, auth.currentPrincipal.ID, id).Scan(&out.id, &out.version, &out.readAt, &out.archivedAt, &out.updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	return out, err
}

func notificationInboxSequence(value string) (int64, bool) {
	sequence, err := strconv.ParseInt(value, 10, 64)
	return sequence, err == nil && sequence > 0 && strconv.FormatInt(sequence, 10) == value
}
