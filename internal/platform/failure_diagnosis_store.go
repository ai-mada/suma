package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
)

const (
	diagnosisInvalid   = "Invalid failure diagnosis request"
	diagnosisSource    = "Failure evidence has changed; create a new diagnosis"
	diagnosisUncertain = "A current known failed node is required; reconcile the original run first"
	diagnosisIssue     = "The failure issue is not yet available; wait for issue projection"
	diagnosisProfile   = "Select a current enabled read-only Suma failure diagnosis profile"
	diagnosisConsent   = "Confirm that the selected failure evidence will be sent to the configured model"
	diagnosisOutput    = "The model output does not satisfy the fixed failure diagnosis contract"
	diagnosisVersion   = "The failure diagnosis has changed; reload before deciding"
	diagnosisDraft     = "Confirm adoption as a draft repair requirement requiring normal delivery review"
	diagnosisFailed    = "The failure diagnosis execution did not complete successfully"
	diagnosisUnknown   = "The failure diagnosis outcome is unknown; inspect the original diagnosis run"
	diagnosisPrompt    = "Failure diagnosis context exceeds 32000 UTF-8 bytes"
	diagnosisAuthority = "The diagnosis creator no longer has current execution authority"
	diagnosisExisting  = "This issue already has a repair requirement; open the current requirement"
)

type failureDiagnosisRecord struct {
	Analysis              FailureDiagnosisAnalysis  `json:"analysis"`
	CreatorCredentialHash string                    `json:"creator_credential_hash"`
	WorkerID              string                    `json:"worker_id"`
	Route                 dag.ProjectExecutionRoute `json:"route"`
}

func (s *Service) InitializeFailureDiagnosis(ctx context.Context) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS suma_failure_diagnoses(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,run_id VARCHAR(100) NOT NULL UNIQUE,source_run_id VARCHAR(100) NOT NULL,source_node_id VARCHAR(150) NOT NULL,source_attempt INTEGER NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_failure_diagnosis_executions(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,analysis_id VARCHAR(64) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,result_sha256 VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,result TEXT NOT NULL,PRIMARY KEY(analysis_id,attempt,fence,result_sha256))`,
		`CREATE TABLE IF NOT EXISTS suma_failure_diagnosis_adoptions(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,issue_id VARCHAR(64) NOT NULL,requirement_id VARCHAR(64) NOT NULL,analysis_id VARCHAR(64) NOT NULL UNIQUE,created_at VARCHAR(40) NOT NULL,snapshot TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,issue_id),UNIQUE(tenant_id,project_id,requirement_id))`,
	} {
		if _, e := s.DB.ExecContext(ctx, s.ddl(q)); e != nil {
			return e
		}
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_failure_diagnosis_page", "suma_failure_diagnoses", "tenant_id", "project_id", "source_run_id", "source_node_id", "source_attempt", "created_at", "id")
}

func (s *Service) failureDiagnosisTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out failureDiagnosisRecord, err error) {
	var raw, state string
	var version int
	err = tx.QueryRowContext(ctx, s.q(`SELECT version,state,payload FROM suma_failure_diagnoses WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&version, &state, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	if out.Analysis.ID != id || out.Analysis.ProjectID != project || out.Analysis.Version != version || out.Analysis.State != state {
		return out, conflict(diagnosisVersion)
	}
	return out, nil
}
func (s *Service) saveFailureDiagnosisTx(ctx context.Context, tx *sql.Tx, tenant string, r failureDiagnosisRecord, old int) error {
	a := r.Analysis
	res, e := tx.ExecContext(ctx, s.q(`UPDATE suma_failure_diagnoses SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), a.Version, a.State, stringify(r), tenant, a.ProjectID, a.ID, old)
	if e != nil {
		return e
	}
	n, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return conflict(diagnosisVersion)
	}
	return nil
}
func (s *Service) failureDiagnosisExecutionRecordTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) (r failureDiagnosisRecord, found bool, err error) {
	var id string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_failure_diagnoses WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, run).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	r, err = s.failureDiagnosisTx(ctx, tx, tenant, project, id)
	return r, err == nil, err
}
func (s *Service) failureDiagnosisAdoptionTx(ctx context.Context, tx *sql.Tx, tenant, project, requirement string) (out *FailureDiagnosisAdoption, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT snapshot FROM suma_failure_diagnosis_adoptions WHERE tenant_id=? AND project_id=? AND requirement_id=?`), tenant, project, requirement).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out = &FailureDiagnosisAdoption{}
	err = json.Unmarshal([]byte(raw), out)
	return out, err
}

// The stable intake receipt identifies one actual requirement. Its original
// response is not a current authorization or a substitute for the current row.
func (s *Service) lookupIssueRequirementTx(ctx context.Context, auth *authorizedCommand, project, issue string) (out Record, found bool, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return out, false, err
	}
	var raw string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), auth.target.tenantID, project, "issue-intake-"+issue).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	var identity struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(raw), &identity) != nil || !handoffID(identity.ID, false) {
		return out, false, conflict(diagnosisExisting)
	}
	out, err = s.getRecordTx(ctx, auth, project, "requirements", identity.ID)
	if err != nil {
		return out, false, err
	}
	if out.Data["source_issue_id"] != issue {
		return out, false, conflict(diagnosisExisting)
	}
	return out, true, nil
}
