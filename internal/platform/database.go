package platform

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
)

type Service struct {
	DB                         *sql.DB
	Dialect                    string
	DataDir                    string
	ReleaseProviders           []ReleaseProviderConfig
	RepositoryProviders        []RepositoryProviderConfig
	ApplicationSourceInspector ApplicationSourceInspectorConfig
	AutomationIngressKey       []byte
	DesktopWorkspaceKey        []byte   // Local desktop root approvals only; never projected to clients or tools.
	authorityBudgets           sync.Map // *sql.Tx -> *authorityTxBudget; never persisted authority
	authorityBudgetMu          sync.Mutex
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

// A declared 1.0.0 database must already have the current shape. Inspect before
// initialization writes anything; old user databases are never upgraded here.
func (s *Service) validateWorkspaceInitialSchema(ctx context.Context) error {
	workspacePresent, rejectionsPresent := false, false
	for _, table := range []struct{ name, columns string }{
		{"suma_workspace_bindings", "binding_id"},
		{"suma_workspace_file_operations", "context_sha256"},
		{"suma_workspace_file_rejections", "request_hash,path,expected_context_sha256,rejected_by,rejected_at"},
	} {
		var query string
		switch s.Dialect {
		case "sqlite":
			query = `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`
		case "postgres":
			query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=? AND table_type='BASE TABLE'`
		case "oceanbase":
			query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=? AND table_type='BASE TABLE'`
		default:
			return fmt.Errorf("unsupported initial schema dialect %q", s.Dialect)
		}
		var present int
		if err := s.DB.QueryRowContext(ctx, s.q(query), table.name).Scan(&present); err != nil {
			return err
		}
		if present == 0 {
			continue
		}
		if table.name == "suma_workspace_file_rejections" {
			rejectionsPresent = true
		} else {
			workspacePresent = true
		}
		rows, err := s.DB.QueryContext(ctx, "SELECT "+table.columns+" FROM "+table.name+" WHERE 1=0")
		if err != nil {
			return fmt.Errorf("%w: current workspace schema required for %s", store.ErrInitialSchema, table.name)
		}
		if err = rows.Close(); err != nil {
			return err
		}
	}
	if workspacePresent && !rejectionsPresent {
		return fmt.Errorf("%w: current workspace rejection receipts are required", store.ErrInitialSchema)
	}
	return nil
}

func (s *Service) Initialize(ctx context.Context) error {
	if err := s.validatePoolRunnerPairingsInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateToolProfileReplacementsInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateKnowledgeProjectionInitialSchema(ctx); err != nil {
		return err
	}
	if err := dag.ValidateExternalAssignmentsInitialSchema(ctx, s.DB, s.Dialect); err != nil {
		return err
	}
	if err := s.validateWorkspaceInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateResourceCalendarInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateWorklogInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateSharedTaskViewsInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateNotificationInboxInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateTaskSubscriptionsInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateProjectNotificationPreferencesInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateTaskConversationMentionsInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateTaskConversationKnowledgeInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateWorkBuddyConnectionsInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateWorkBuddyCloudInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateWorkBuddyTaskDispatchInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateWorkBuddyCloudCollaborationInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateWorkBuddyCloudTurnsInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateToolPermissionsInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateToolInspectionsInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateToolPreparationsInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateToolInstallationsInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateToolRecoveriesInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateToolInputsInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateTurnSteeringInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateWorkBuddyCloudHistorySharingInitialSchema(ctx); err != nil {
		return err
	}
	if err := s.validateTaskReviewRequestsInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	if err := s.initializeWorkspace(ctx); err != nil {
		return err
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS suma_human_resources(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,user_id VARCHAR(64) NOT NULL,resource_id VARCHAR(64) NOT NULL UNIQUE,PRIMARY KEY(tenant_id,project_id,user_id))`,
		`CREATE TABLE IF NOT EXISTS suma_token_scopes(token_hash VARCHAR(64) PRIMARY KEY,kind VARCHAR(30) NOT NULL,project_id VARCHAR(64) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_workspace_bindings(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,root_path TEXT NOT NULL,binding_id VARCHAR(32) NOT NULL,PRIMARY KEY(tenant_id,project_id))`,
		`CREATE TABLE IF NOT EXISTS suma_organizations (id VARCHAR(64) PRIMARY KEY,name VARCHAR(200) NOT NULL,created_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_users (id VARCHAR(64) PRIMARY KEY,name VARCHAR(200) NOT NULL,email VARCHAR(254) NOT NULL UNIQUE,password_hash TEXT NOT NULL,created_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_org_members (tenant_id VARCHAR(64) NOT NULL,user_id VARCHAR(64) NOT NULL,role VARCHAR(30) NOT NULL,PRIMARY KEY(tenant_id,user_id))`,
		`CREATE TABLE IF NOT EXISTS suma_sessions (token_hash VARCHAR(64) PRIMARY KEY,user_id VARCHAR(64) NOT NULL,tenant_id VARCHAR(64) NOT NULL,expires_at VARCHAR(40) NOT NULL,label VARCHAR(200) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_projects (id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,title VARCHAR(500) NOT NULL,description TEXT NOT NULL,status VARCHAR(40) NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_project_members (project_id VARCHAR(64) NOT NULL,user_id VARCHAR(64) NOT NULL,role VARCHAR(30) NOT NULL,PRIMARY KEY(project_id,user_id))`,
		`CREATE TABLE IF NOT EXISTS suma_operations (tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_hash VARCHAR(64) NOT NULL,response TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,operation_id))`,
		`CREATE TABLE IF NOT EXISTS suma_activity (id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,actor_id VARCHAR(64) NOT NULL,kind VARCHAR(60) NOT NULL,object_id VARCHAR(64) NOT NULL,title VARCHAR(500) NOT NULL,created_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_reviews (id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,task_version INTEGER NOT NULL,run_id VARCHAR(64) NOT NULL,reviewer_id VARCHAR(64) NOT NULL,decision VARCHAR(40) NOT NULL,reason TEXT NOT NULL,created_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_issue_repairs(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,issue_id VARCHAR(64) NOT NULL,cycle INTEGER NOT NULL,version INTEGER NOT NULL,task_id VARCHAR(64) NOT NULL,closed_run_id VARCHAR(100) NOT NULL,snapshot TEXT NOT NULL,UNIQUE(tenant_id,project_id,issue_id,cycle),UNIQUE(tenant_id,project_id,task_id))`,
		`CREATE TABLE IF NOT EXISTS suma_artifacts (id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,digest VARCHAR(64) NOT NULL,name VARCHAR(500) NOT NULL,media_type VARCHAR(200) NOT NULL,size_bytes BIGINT NOT NULL,created_at VARCHAR(40) NOT NULL)`,
		indexOutboxDDL,
	}
	for _, table := range []string{"requirements", "tasks", "members", "issues", "documents", "releases", "tests", "evidence"} {
		statements = append(statements, `CREATE TABLE IF NOT EXISTS suma_`+table+` (id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,title VARCHAR(500) NOT NULL,description TEXT NOT NULL,status VARCHAR(40) NOT NULL,priority VARCHAR(20) NOT NULL,owner_id VARCHAR(64) NOT NULL,requirement_id VARCHAR(64) NOT NULL,run_id VARCHAR(64) NOT NULL,depends_on TEXT NOT NULL,acceptance_criteria TEXT NOT NULL,kind VARCHAR(40) NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL,data TEXT NOT NULL)`)
	}
	for _, query := range statements {
		if _, err := s.DB.ExecContext(ctx, s.ddl(query)); err != nil {
			return fmt.Errorf("platform migration: %w", err)
		}
	}
	if err := s.InitializeWorkspaceObservations(ctx); err != nil {
		return err
	}
	if err := s.InitializeWorkspaceWrites(ctx); err != nil {
		return err
	}
	if err := s.InitializeProjections(ctx); err != nil {
		return err
	}
	if err := s.InitializePlanning(ctx); err != nil {
		return err
	}
	if err := s.InitializeWorklogs(ctx); err != nil {
		return err
	}
	if err := s.InitializeMyWork(ctx); err != nil {
		return err
	}
	if err := s.InitializeTaskCollections(ctx); err != nil {
		return err
	}
	if err := s.InitializeSharedTaskViews(ctx); err != nil {
		return err
	}
	if err := s.InitializeNotificationInbox(ctx); err != nil {
		return err
	}
	if err := s.InitializeIntelligentPlanning(ctx); err != nil {
		return err
	}
	if err := s.InitializeUsageAccounting(ctx); err != nil {
		return err
	}
	if err := s.InitializeGovernance(ctx); err != nil {
		return err
	}
	if err := s.InitializeCredentialAuthority(ctx); err != nil {
		return err
	}
	if err := s.InitializeLocalePreferences(ctx); err != nil {
		return err
	}
	if err := s.InitializeArtifacts(ctx); err != nil {
		return err
	}
	if err := s.InitializeRunnerPairings(ctx); err != nil {
		return err
	}
	if err := s.InitializeReleaseDeployment(ctx); err != nil {
		return err
	}
	if err := s.InitializeAutomations(ctx); err != nil {
		return err
	}
	if err := s.InitializeAutomationIngresses(ctx); err != nil {
		return err
	}
	if err := s.InitializeToolProfiles(ctx); err != nil {
		return err
	}
	if err := s.InitializeToolProfileReplacements(ctx); err != nil {
		return err
	}
	if err := s.InitializeToolInspections(ctx); err != nil {
		return err
	}
	if err := s.InitializeToolPreparations(ctx); err != nil {
		return err
	}
	if err := s.InitializeToolInstallations(ctx); err != nil {
		return err
	}
	if err := s.InitializeToolRecoveries(ctx); err != nil {
		return err
	}
	if err := s.InitializeTaskDispatch(ctx); err != nil {
		return err
	}
	if err := s.InitializeScheduling(ctx); err != nil {
		return err
	}
	if err := s.InitializeExecutionPools(ctx); err != nil {
		return err
	}
	if err := s.InitializePoolRunnerPairings(ctx); err != nil {
		return err
	}
	if err := s.InitializeProjectKnowledge(ctx); err != nil {
		return err
	}
	if err := s.InitializeIntelligentOperations(ctx); err != nil {
		return err
	}
	if err := s.InitializeRequirementDesign(ctx); err != nil {
		return err
	}
	if err := s.InitializeTestPlans(ctx); err != nil {
		return err
	}
	if err := s.InitializeProjectRisks(ctx); err != nil {
		return err
	}
	if err := s.InitializeProjectApplications(ctx); err != nil {
		return err
	}
	if err := s.InitializeApplicationReleaseSets(ctx); err != nil {
		return err
	}
	if err := s.InitializeApplicationReleaseRollouts(ctx); err != nil {
		return err
	}
	if err := s.InitializeDispatchRecommendations(ctx); err != nil {
		return err
	}
	if err := s.InitializeTaskDesignView(ctx); err != nil {
		return err
	}
	if err := s.InitializeTaskConversations(ctx); err != nil {
		return err
	}
	if err := s.InitializeTaskSubscriptions(ctx); err != nil {
		return err
	}
	if err := s.InitializeProjectNotificationPreferences(ctx); err != nil {
		return err
	}
	if err := s.InitializeTaskConversationMentions(ctx); err != nil {
		return err
	}
	if err := s.InitializeTaskConversationKnowledge(ctx); err != nil {
		return err
	}
	if err := s.InitializeWorkBuddyConnections(ctx); err != nil {
		return err
	}
	if err := s.InitializeWorkBuddyCloud(ctx); err != nil {
		return err
	}
	if err := s.InitializeWorkBuddyTaskDispatch(ctx); err != nil {
		return err
	}
	if err := s.InitializeWorkBuddyCloudCollaboration(ctx); err != nil {
		return err
	}
	if err := s.InitializeWorkBuddyCloudTurns(ctx); err != nil {
		return err
	}
	if err := s.InitializeToolPermissions(ctx); err != nil {
		return err
	}
	if err := s.InitializeToolInputs(ctx); err != nil {
		return err
	}
	if err := s.InitializeTurnSteering(ctx); err != nil {
		return err
	}
	if err := s.InitializeWorkBuddyCloudHistorySharing(ctx); err != nil {
		return err
	}
	if err := s.InitializeTaskReviewRequests(ctx); err != nil {
		return err
	}
	if err := s.InitializeTaskConversationReplies(ctx); err != nil {
		return err
	}
	if err := s.InitializeTaskDiscussionDevelopment(ctx); err != nil {
		return err
	}
	if err := s.InitializeWorkspaceTerminals(ctx); err != nil {
		return err
	}
	if err := s.InitializeWorkspaceTerminalCandidates(ctx); err != nil {
		return err
	}
	if err := s.InitializeTaskHandoffs(ctx); err != nil {
		return err
	}
	if err := s.InitializeCandidateTransfers(ctx); err != nil {
		return err
	}
	return s.InitializeRepositoryIntegration(ctx)
}
