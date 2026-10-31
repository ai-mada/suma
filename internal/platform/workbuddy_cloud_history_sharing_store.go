package platform

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

const historyShareColumns = "id,tenant_id,project_id,task_id,message_id,owner_id,load_id,load_version,task_version,operation_id,request_sha256,payload"
const historyShareDDL = `CREATE TABLE IF NOT EXISTS suma_workbuddy_cloud_history_shares(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,message_id VARCHAR(64) NOT NULL UNIQUE,owner_id VARCHAR(64) NOT NULL,load_id VARCHAR(64) NOT NULL,load_version INTEGER NOT NULL,task_version INTEGER NOT NULL,operation_id VARCHAR(128) NOT NULL,request_sha256 VARCHAR(64) NOT NULL,payload TEXT NOT NULL,UNIQUE(tenant_id,project_id,operation_id))`

func (s *Service) validateWorkBuddyCloudHistorySharingInitialSchema(ctx context.Context) error {
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=?`
	switch s.Dialect {
	case "sqlite":
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
	default:
		return fmt.Errorf("%w: cloud history share dialect", store.ErrInitialSchema)
	}
	const table = "suma_workbuddy_cloud_history_shares"
	var count int
	if err := s.DB.QueryRowContext(ctx, s.q(query), table).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	var baseline string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT version FROM suma_schema_baselines WHERE component='platform'`)).Scan(&baseline); err != nil || baseline != store.InitialDataContract {
		return fmt.Errorf("%w: registered cloud history share schema required", store.ErrInitialSchema)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT * FROM `+table+` WHERE 1=0`)
	if err != nil {
		return fmt.Errorf("%w: cloud history share columns", store.ErrInitialSchema)
	}
	columns, err := rows.Columns()
	rows.Close()
	if err != nil {
		return err
	}
	wanted := strings.Split(historyShareColumns, ",")
	seen := map[string]bool{}
	for _, column := range columns {
		seen[column] = true
	}
	if len(columns) != len(wanted) {
		return fmt.Errorf("%w: exact cloud history share columns required", store.ErrInitialSchema)
	}
	for _, column := range wanted {
		if !seen[column] {
			return fmt.Errorf("%w: cloud history share column required", store.ErrInitialSchema)
		}
	}
	primary, unique, err := s.notificationInboxSchemaKeys(ctx, table)
	if err != nil {
		return err
	}
	if primary != "id" || !unique["message_id"] || !unique["tenant_id,project_id,operation_id"] {
		return fmt.Errorf("%w: cloud history share identity required", store.ErrInitialSchema)
	}
	return nil
}
func (s *Service) InitializeWorkBuddyCloudHistorySharing(ctx context.Context) error {
	if err := s.validateWorkBuddyCloudHistorySharingInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, s.ddl(historyShareDDL))
	return err
}

func (w *WorkBuddyConnections) saveCloudHistoryShareTx(ctx context.Context, a *authorizedCommand, message TaskConversationMessage, req wc.CloudHistoryShareRequest, hash string, observation wc.CloudHistorySharedObservation) error {
	if observation.Validate() != nil || message.AuthorID != a.currentPrincipal.ID || message.ActorKind != "human" || message.CloudObservation != nil || message.Body == "" || observation.BodySHA256 != wc.CloudHistoryTextSHA256(message.Body) {
		return historyShareChanged()
	}
	if err := assertProjectCommand(a.tx, a, a.target.projectID); err != nil {
		return err
	}
	_, err := a.tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_workbuddy_cloud_history_shares(`+historyShareColumns+`)VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`), newID(), a.target.tenantID, a.target.projectID, req.TaskID, message.ID, a.currentPrincipal.ID, observation.LoadID, observation.LoadVersion, message.TaskVersion, req.IdempotencyKey, hash, stringify(observation))
	return err
}

// Called only after the discussion's own current task read authority. The
// immutable shared copy does not reopen or reauthorize any private load.
func (s *Service) taskConversationCloudObservationMetadataTx(ctx context.Context, tx *sql.Tx, tenant, project, task string, items []TaskConversationMessage) error {
	if len(items) == 0 {
		return nil
	}
	args := []any{tenant, project, task}
	marks := make([]string, len(items))
	positions := map[string]int{}
	for i, item := range items {
		marks[i] = "?"
		args = append(args, item.ID)
		positions[item.ID] = i
		items[i].CloudObservation = nil
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT message_id,owner_id,load_id,load_version,task_version,payload FROM suma_workbuddy_cloud_history_shares WHERE tenant_id=? AND project_id=? AND task_id=? AND message_id IN (`+strings.Join(marks, ",")+`)`), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var message, owner, load, raw string
		var loadVersion, taskVersion int
		if err = rows.Scan(&message, &owner, &load, &loadVersion, &taskVersion, &raw); err != nil {
			return err
		}
		position, ok := positions[message]
		if !ok {
			return historyShareChanged()
		}
		var observation wc.CloudHistorySharedObservation
		if wc.Decode([]byte(raw), &observation) != nil || observation.Validate() != nil || observation.LoadID != load || observation.LoadVersion != loadVersion || items[position].CloudObservation != nil || items[position].ActorKind != "human" || items[position].AuthorID != owner || items[position].TaskVersion != taskVersion || observation.BodySHA256 != wc.CloudHistoryTextSHA256(items[position].Body) {
			return historyShareChanged()
		}
		items[position].CloudObservation = &observation
	}
	return rows.Err()
}
