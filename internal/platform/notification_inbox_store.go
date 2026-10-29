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
	member, err := scanRecord(auth.tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_members WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, nextTask.ProjectID, nextTask.OwnerID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	user, _ := member.Data["user_id"].(string)
	if member.Kind != "human" || member.Status != "active" || !handoffID(user, false) {
		return nil
	}
	var canonical, organizationRole string
	var projectRole sql.NullString
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT h.resource_id,om.role,pm.role FROM suma_human_resources h JOIN suma_users u ON u.id=h.user_id JOIN suma_org_members om ON om.tenant_id=h.tenant_id AND om.user_id=h.user_id LEFT JOIN suma_project_members pm ON pm.project_id=h.project_id AND pm.user_id=h.user_id WHERE h.tenant_id=? AND h.project_id=? AND h.user_id=?`), auth.target.tenantID, nextTask.ProjectID, user).Scan(&canonical, &organizationRole, &projectRole)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if canonical != member.ID || !validOrgRole(organizationRole) || organizationRole != "owner" && organizationRole != "admin" && (!projectRole.Valid || !validProjectRole(projectRole.String)) {
		return nil
	}
	eventKey := digest(stringify(struct {
		Task     string
		Version  int
		Resource string
	}{nextTask.ID, nextTask.Version, nextTask.OwnerID}))
	var existing string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_notification_inbox WHERE tenant_id=? AND project_id=? AND source_event_key=? AND recipient_user_id=?`), auth.target.tenantID, nextTask.ProjectID, eventKey, user).Scan(&existing)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	insert := `INSERT INTO suma_notification_inbox_sequences(tenant_id,project_id,last_sequence) VALUES(?,?,0) ON CONFLICT(tenant_id,project_id) DO NOTHING`
	if s.Dialect == "oceanbase" {
		insert = `INSERT IGNORE INTO suma_notification_inbox_sequences(tenant_id,project_id,last_sequence) VALUES(?,?,0)`
	}
	if _, err = auth.tx.ExecContext(ctx, s.q(insert), auth.target.tenantID, nextTask.ProjectID); err != nil {
		return err
	}
	result, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_notification_inbox_sequences SET last_sequence=last_sequence+1 WHERE tenant_id=? AND project_id=? AND last_sequence>=0 AND last_sequence<?`), auth.target.tenantID, nextTask.ProjectID, int64(1<<63-1))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return conflict(notificationInboxSequenceFailed)
	}
	var sequence int64
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT last_sequence FROM suma_notification_inbox_sequences WHERE tenant_id=? AND project_id=?`), auth.target.tenantID, nextTask.ProjectID).Scan(&sequence); err != nil {
		return err
	}
	if sequence < 1 {
		return conflict(notificationInboxSequenceFailed)
	}
	at := auth.checkedAt.UTC().Format(time.RFC3339Nano)
	_, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_notification_inbox(id,tenant_id,project_id,recipient_user_id,sequence,kind,task_id,source_task_version,source_resource_id,source_event_key,version,created_at,read_at,archived_at,updated_at) VALUES(?,?,?,?,?,'task_assigned',?,?,?,?,1,?,'','',?)`), newID(), auth.target.tenantID, nextTask.ProjectID, user, sequence, nextTask.ID, nextTask.Version, nextTask.OwnerID, eventKey, at, at)
	return err
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
