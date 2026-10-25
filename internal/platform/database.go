package platform

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
)

type Service struct {
	DB                   *sql.DB
	Dialect              string
	DataDir              string
	ReleaseProviders     []ReleaseProviderConfig
	AutomationIngressKey []byte
	authorityBudgets     sync.Map // *sql.Tx -> *authorityTxBudget; never persisted authority
	authorityBudgetMu    sync.Mutex
}

func (s *Service) q(query string) string {
	if s.Dialect != "postgres" {
		return query
	}
	var b strings.Builder
	n := 0
	for _, c := range query {
		if c == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func (s *Service) ddl(query string) string {
	if s.Dialect == "oceanbase" {
		return strings.ReplaceAll(query, " TEXT", " LONGTEXT")
	}
	return query
}
func (s *Service) Migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS suma_human_resources(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,user_id VARCHAR(64) NOT NULL,resource_id VARCHAR(64) NOT NULL UNIQUE,PRIMARY KEY(tenant_id,project_id,user_id))`,
		`CREATE TABLE IF NOT EXISTS suma_token_scopes(token_hash VARCHAR(64) PRIMARY KEY,kind VARCHAR(30) NOT NULL,project_id VARCHAR(64) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_workspace_bindings(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,root_path TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id))`,
		`CREATE TABLE IF NOT EXISTS suma_organizations (id VARCHAR(64) PRIMARY KEY,name VARCHAR(200) NOT NULL,created_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_users (id VARCHAR(64) PRIMARY KEY,name VARCHAR(200) NOT NULL,email VARCHAR(254) NOT NULL UNIQUE,password_hash TEXT NOT NULL,created_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_org_members (tenant_id VARCHAR(64) NOT NULL,user_id VARCHAR(64) NOT NULL,role VARCHAR(30) NOT NULL,PRIMARY KEY(tenant_id,user_id))`,
		`CREATE TABLE IF NOT EXISTS suma_sessions (token_hash VARCHAR(64) PRIMARY KEY,user_id VARCHAR(64) NOT NULL,tenant_id VARCHAR(64) NOT NULL,expires_at VARCHAR(40) NOT NULL,label VARCHAR(200) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_projects (id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,title VARCHAR(500) NOT NULL,description TEXT NOT NULL,status VARCHAR(40) NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_project_members (project_id VARCHAR(64) NOT NULL,user_id VARCHAR(64) NOT NULL,role VARCHAR(30) NOT NULL,PRIMARY KEY(project_id,user_id))`,
		`CREATE TABLE IF NOT EXISTS suma_operations (tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_hash VARCHAR(64) NOT NULL,response TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,operation_id))`,
		`CREATE TABLE IF NOT EXISTS suma_activity (id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,actor_id VARCHAR(64) NOT NULL,kind VARCHAR(60) NOT NULL,object_id VARCHAR(64) NOT NULL,title VARCHAR(500) NOT NULL,created_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_reviews (id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,task_version INTEGER NOT NULL,run_id VARCHAR(64) NOT NULL,reviewer_id VARCHAR(64) NOT NULL,decision VARCHAR(40) NOT NULL,reason TEXT NOT NULL,created_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_artifacts (id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,digest VARCHAR(64) NOT NULL,name VARCHAR(500) NOT NULL,media_type VARCHAR(200) NOT NULL,size_bytes BIGINT NOT NULL,created_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_index_outbox (id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,source_id VARCHAR(64) NOT NULL,revision INTEGER NOT NULL,action VARCHAR(40) NOT NULL,payload TEXT NOT NULL,state VARCHAR(40) NOT NULL,created_at VARCHAR(40) NOT NULL)`,
	}
	for _, table := range []string{"requirements", "tasks", "members", "issues", "documents", "releases", "tests", "evidence"} {
		statements = append(statements, `CREATE TABLE IF NOT EXISTS suma_`+table+` (id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,title VARCHAR(500) NOT NULL,description TEXT NOT NULL,status VARCHAR(40) NOT NULL,priority VARCHAR(20) NOT NULL,owner_id VARCHAR(64) NOT NULL,requirement_id VARCHAR(64) NOT NULL,run_id VARCHAR(64) NOT NULL,depends_on TEXT NOT NULL,acceptance_criteria TEXT NOT NULL,kind VARCHAR(40) NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL,data TEXT NOT NULL)`)
	}
	for _, query := range statements {
		if _, err := s.DB.ExecContext(ctx, s.ddl(query)); err != nil {
			return fmt.Errorf("platform migration: %w", err)
		}
	}
	if err := s.MigrateWorkspace(ctx); err != nil {
		return err
	}
	if err := s.MigrateProjections(ctx); err != nil {
		return err
	}
	if err := s.MigratePlanning(ctx); err != nil {
		return err
	}
	if err := s.MigrateGovernance(ctx); err != nil {
		return err
	}
	if err := s.MigrateCredentialAuthority(ctx); err != nil {
		return err
	}
	if err := s.MigrateLocalePreferences(ctx); err != nil {
		return err
	}
	if err := s.MigrateArtifacts(ctx); err != nil {
		return err
	}
	if err := s.MigrateReleaseDeployment(ctx); err != nil {
		return err
	}
	if err := s.MigrateAutomations(ctx); err != nil {
		return err
	}
	if err := s.MigrateAutomationIngresses(ctx); err != nil {
		return err
	}
	if err := s.MigrateToolProfiles(ctx); err != nil {
		return err
	}
	if err := s.MigrateTaskDispatch(ctx); err != nil {
		return err
	}
	if err := s.MigrateScheduling(ctx); err != nil {
		return err
	}
	if err := s.MigrateExecutionPools(ctx); err != nil {
		return err
	}
	if err := s.MigrateProjectKnowledge(ctx); err != nil {
		return err
	}
	if err := s.MigrateRequirementDesign(ctx); err != nil {
		return err
	}
	if err := s.MigrateTaskDesignView(ctx); err != nil {
		return err
	}
	return s.MigrateCandidateTransfers(ctx)
}
