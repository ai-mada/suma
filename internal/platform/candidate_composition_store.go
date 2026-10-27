package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"github.com/ai-mada/suma/internal/store"
)

const (
	compositionInvalid          = "Invalid candidate composition request"
	compositionChanged          = "Candidate composition changed; refresh the fixed preview"
	compositionSource           = "Candidate composition source acceptance or candidate changed"
	compositionTemplate         = "Candidate composition requires one producer and independent candidate tests"
	compositionBase             = "Candidate composition sources do not share the administrator base"
	compositionExpired          = "Candidate composition preview expired"
	compositionAck              = "Explicit candidate composition acknowledgement required"
	compositionAuthority        = "Candidate composition initiating credential is no longer authorized"
	compositionBaseline         = "Code baseline changed or is no longer accepted"
	compositionStarted          = "Code baseline requires an unstarted task"
	compositionUnknown          = "Candidate composition requires recovery of the original run"
	compositionConflict         = "Candidate composition requires a known exact conflict with complete Git evidence"
	compositionRepairProfile    = "Conflict repair requires an explicitly enabled native workspace tool profile"
	compositionRepairTask       = "Conflict repair task must start through its fixed repair plan"
	compositionRepairHistorical = "Conflict repair plan was superseded; open its current reviewed revision"
	compositionRepairRevision   = "Conflict repair revision requires a current ready plan or certified startup failure"
	compositionRepairRecovery   = "Conflict repair recovery changed or requires reconciliation of the original run"
	compositionRepairLimit      = "Conflict repair revision family reached its limit of 32 plans"
)

func compositionError(status int, code, message string) error {
	return &APIError{Status: status, Code: "candidate_composition_" + code, Message: message}
}

type candidateCompositionRecord struct {
	Plan            CandidateCompositionPlan               `json:"plan"`
	Creator         expertTeamCredentialReference          `json:"creator"`
	Execution       *ccmp.CandidateCompositionExecution    `json:"execution,omitempty"`
	RepairExecution *ccmp.CandidateConflictRepairExecution `json:"repair_execution,omitempty"`
}

func (s *Service) MigrateCandidateCompositions(ctx context.Context) error {
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS suma_candidate_compositions(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(24) NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_candidate_composition_executions(run_id VARCHAR(100) NOT NULL,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,plan_id VARCHAR(64) NOT NULL,node_id VARCHAR(150) NOT NULL,input_kind VARCHAR(24) NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(run_id,node_id))`,
		`CREATE TABLE IF NOT EXISTS suma_task_code_baselines(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,binding TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,task_id))`,
		`CREATE TABLE IF NOT EXISTS suma_candidate_composition_repair_sources(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,composition_id VARCHAR(64) NOT NULL,conflict_sha256 VARCHAR(64) NOT NULL,repair_plan_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,PRIMARY KEY(tenant_id,project_id,composition_id,conflict_sha256),UNIQUE(tenant_id,project_id,task_id))`,
		`CREATE TABLE IF NOT EXISTS suma_candidate_composition_repair_revisions(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,root_plan_id VARCHAR(64) NOT NULL,plan_id VARCHAR(64) PRIMARY KEY,parent_plan_id VARCHAR(64),ordinal INTEGER NOT NULL,task_id VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL,UNIQUE(tenant_id,project_id,root_plan_id,ordinal),UNIQUE(tenant_id,project_id,parent_plan_id))`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
			return err
		}
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_composition_scope", "suma_candidate_compositions", "tenant_id", "project_id", "task_id", "created_at", "id")
}
func (s *Service) candidateCompositionTx(ctx context.Context, tx governanceQuery, tenant, project, id string) (r candidateCompositionRecord, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_candidate_compositions WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return r, missing()
	}
	if err != nil {
		return r, err
	}
	err = json.Unmarshal([]byte(raw), &r)
	return
}
func (s *Service) candidateCompositionExecutionRecordTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) (r candidateCompositionRecord, found bool, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_candidate_composition_executions WHERE tenant_id=? AND project_id=? AND run_id=? AND input_kind IN ('composition','conflict_repair')`), tenant, project, run).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	err = json.Unmarshal([]byte(raw), &r)
	return r, err == nil, err
}

func (s *Service) readCandidateCompositionExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string) (*ccmp.CandidateCompositionExecution, error) {
	r, found, err := s.candidateCompositionExecutionRecordTx(ctx, tx, tenant, project, run)
	if err != nil || !found {
		return nil, err
	}
	if r.Plan.ProducerNodeID != node {
		return nil, nil
	}
	if r.RepairExecution != nil {
		return nil, nil
	}
	if r.Plan.RunID != run || !validCandidateCompositionExecutionRecord(r) {
		return nil, compositionError(409, "changed", compositionChanged)
	}
	return r.Execution, nil
}
func (s *Service) saveCandidateCompositionTx(ctx context.Context, auth *authorizedCommand, old int, r candidateCompositionRecord) error {
	res, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_candidate_compositions SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), r.Plan.Version, r.Plan.State, stringify(r), auth.target.tenantID, r.Plan.ProjectID, r.Plan.ID, old)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return compositionError(409, "changed", compositionChanged)
	}
	return nil
}
func (s *Service) taskCodeBaselineTx(ctx context.Context, tx governanceQuery, tenant, project, task string) (binding *ccmp.CodeBaselineBinding, version int, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT version,binding FROM suma_task_code_baselines WHERE tenant_id=? AND project_id=? AND task_id=?`), tenant, project, task).Scan(&version, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	binding = &ccmp.CodeBaselineBinding{}
	err = json.Unmarshal([]byte(raw), binding)
	if err == nil {
		err = ccmp.ValidateCodeBaselineBinding(*binding)
	}
	return
}
