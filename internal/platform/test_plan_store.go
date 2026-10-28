package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ai-mada/suma/internal/store"
)

const (
	testPlanInvalid    = "Invalid test plan request"
	testPlanVersion    = "The test plan or selected task has changed; reload before deciding"
	testPlanSource     = "The test plan source has changed; review and save a current draft"
	testPlanIncomplete = "Complete the test cases and task acceptance coverage before publication"
	testPlanGate       = "The current test plan does not have complete passing evidence for this run"
	testPlanBinding    = "Published test plans require fixed report-capable test nodes without dynamic or child workflows"
	testPlanDefect     = "Only a current known failed or error test case can create a defect"
)

type testPlanAggregate struct {
	ID, TaskID, State, CreatedAt, UpdatedAt string
	Version, Latest, Published              int
	Governance                              int
}
type testPlanRunSnapshot struct {
	TaskID             string            `json:"task_id"`
	PlanID             string            `json:"plan_id"`
	Revision           *TestPlanRevision `json:"revision,omitempty"`
	GovernanceRevision int               `json:"governance_revision"`
	CreatedAt          string            `json:"created_at"`
	// Package tests consume a sealed artifact instead of producing JUnit cases.
	// Their authority is frozen before execution, never inferred from a result.
	PackageTests []testPlanPackageBinding `json:"package_tests"`
}

func (s *Service) InitializeTestPlans(ctx context.Context) error {
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS suma_test_plans(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,latest_revision INTEGER NOT NULL,published_revision INTEGER NOT NULL,governance_revision INTEGER NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL,UNIQUE(tenant_id,project_id,task_id))`,
		`CREATE TABLE IF NOT EXISTS suma_test_plan_revisions(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,plan_id VARCHAR(64) NOT NULL,revision INTEGER NOT NULL,snapshot TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,plan_id,revision))`,
		`CREATE TABLE IF NOT EXISTS suma_test_plan_runs(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,task_id VARCHAR(64) NOT NULL,plan_id VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,snapshot TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,run_id))`,
		`CREATE TABLE IF NOT EXISTS suma_test_plan_reports(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,plan_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,created_at VARCHAR(40) NOT NULL,projection TEXT NOT NULL,payload TEXT NOT NULL,UNIQUE(tenant_id,project_id,run_id,node_id,attempt,fence))`,
		`CREATE TABLE IF NOT EXISTS suma_test_plan_defects(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,plan_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,case_key VARCHAR(64) NOT NULL,issue_id VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,PRIMARY KEY(tenant_id,project_id,plan_id,run_id,case_key))`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
			return err
		}
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_test_plan_page", "suma_test_plans", "tenant_id", "project_id", "created_at", "id")
}
func (s *Service) testPlanTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out testPlanAggregate, err error) {
	err = tx.QueryRowContext(ctx, s.q(`SELECT id,task_id,version,state,latest_revision,published_revision,governance_revision,created_at,updated_at FROM suma_test_plans WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&out.ID, &out.TaskID, &out.Version, &out.State, &out.Latest, &out.Published, &out.Governance, &out.CreatedAt, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = missing()
	}
	return
}
func (s *Service) testPlanByTaskTx(ctx context.Context, tx *sql.Tx, tenant, project, task string) (out testPlanAggregate, found bool, err error) {
	var id string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_test_plans WHERE tenant_id=? AND project_id=? AND task_id=?`), tenant, project, task).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	out, err = s.testPlanTx(ctx, tx, tenant, project, id)
	return out, true, err
}
func testPlanRevisionHash(r TestPlanRevision) string {
	return digest(stringify(struct {
		Title, Description string
		Source             TestPlanSource
		Cases              []QualityTestCase
	}{r.Title, r.Description, r.Source, r.Cases}))
}
func (s *Service) testPlanRevisionTx(ctx context.Context, tx *sql.Tx, tenant, project, id string, revision int) (out TestPlanRevision, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT snapshot FROM suma_test_plan_revisions WHERE tenant_id=? AND project_id=? AND plan_id=? AND revision=?`), tenant, project, id, revision).Scan(&raw)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	if out.Revision != revision || out.SHA256 != testPlanRevisionHash(out) {
		err = fmt.Errorf("test plan revision is inconsistent")
	}
	return
}
func (s *Service) testPlanSourceTx(ctx context.Context, tx *sql.Tx, tenant, project, task string) (out TestPlanSource, err error) {
	r, err := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, task))
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if r.Status == "archived" || r.Status == "deleted" {
		return out, conflict(testPlanSource)
	}
	out.Task = designSnapshot(r)
	out.TaskBasisSHA256 = taskBasis(r)
	if r.RequirementID != "" {
		req, e := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_requirements WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, r.RequirementID))
		if e != nil {
			return out, e
		}
		if req.Status == "archived" || req.Status == "deleted" {
			return out, conflict(testPlanSource)
		}
		snapshot := designSnapshot(req)
		out.Requirement = &snapshot
	}
	out.SHA256 = digest(stringify(struct {
		Task        TaskContextRecord
		Basis       string
		Requirement *TaskContextRecord
	}{out.Task, out.TaskBasisSHA256, out.Requirement}))
	return out, nil
}
func (s *Service) testPlanSourceCurrentTx(ctx context.Context, tx *sql.Tx, tenant, project string, fixed TestPlanSource) (bool, error) {
	current, err := s.testPlanSourceTx(ctx, tx, tenant, project, fixed.Task.ID)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return false, nil
		}
		return false, err
	}
	// Run binding and review increment task.Version without changing its content.
	// Requirement revisions remain exact; they carry business meaning separately.
	return current.TaskBasisSHA256 == fixed.TaskBasisSHA256 && stringify(current.Requirement) == stringify(fixed.Requirement), nil
}
func (s *Service) saveTestPlanRevisionTx(ctx context.Context, auth *authorizedCommand, project string, current testPlanAggregate, revision TestPlanRevision, state string, published int) (testPlanAggregate, error) {
	if err := assertProjectCommand(auth.tx, auth, project); err != nil {
		return current, err
	}
	revision.SHA256 = testPlanRevisionHash(revision)
	_, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_test_plan_revisions(tenant_id,project_id,plan_id,revision,snapshot)VALUES(?,?,?,?,?)`), auth.target.tenantID, project, current.ID, revision.Revision, stringify(revision))
	if err != nil {
		return current, err
	}
	result, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_test_plans SET version=version+1,state=?,latest_revision=?,published_revision=?,governance_revision=?,updated_at=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), state, revision.Revision, published, current.Governance, revision.CreatedAt, auth.target.tenantID, project, current.ID, current.Version)
	if err != nil {
		return current, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return current, err
	}
	if n != 1 {
		return current, conflict(testPlanVersion)
	}
	current.Version++
	current.State = state
	current.Latest = revision.Revision
	current.Published = published
	current.UpdatedAt = revision.CreatedAt
	return current, nil
}
