package main

// These DTOs describe map-backed HTTP envelopes. Domain structs are reflected
// directly from internal/platform and internal/dag; these maps are audited
// against their HTTP handlers rather than presented as generated handler code.
import (
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/toolcontract"
)

type SessionOrganization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type SessionResponse struct {
	User          platform.Principal  `json:"user"`
	Organization  SessionOrganization `json:"organization"`
	Projects      []platform.Record   `json:"projects"`
	SetupRequired bool                `json:"setup_required"`
}
type OrganizationSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}
type OrganizationMember struct {
	UserID  string `json:"user_id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Role    string `json:"role"`
	Version int    `json:"version"`
}
type OrganizationMemberUpdate struct {
	UserID  string `json:"user_id"`
	Role    string `json:"role"`
	Version int    `json:"version"`
	Status  string `json:"status"`
}
type InvitationGrant struct {
	ProjectID string `json:"project_id"`
	Role      string `json:"role"`
}
type InvitationMetadata struct {
	ID         string            `json:"id"`
	TenantID   string            `json:"tenant_id"`
	Version    int               `json:"version"`
	Email      string            `json:"email"`
	Role       string            `json:"role"`
	Grants     []InvitationGrant `json:"grants"`
	Status     string            `json:"status"`
	CreatedBy  string            `json:"created_by"`
	CreatedAt  string            `json:"created_at"`
	ExpiresAt  string            `json:"expires_at"`
	AcceptedBy string            `json:"accepted_by,omitempty"`
	AcceptedAt string            `json:"accepted_at,omitempty"`
}
type InvitationCreation struct {
	InvitationMetadata
	Token         string `json:"token,omitempty"`
	TokenRevealed bool   `json:"token_revealed"`
	Recovery      string `json:"recovery"`
}
type Team struct {
	ID          string `json:"id"`
	Version     int    `json:"version"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}
type TeamMember struct {
	ID           string `json:"id"`
	TeamID       string `json:"team_id"`
	ResourceKind string `json:"resource_kind"`
	ResourceID   string `json:"resource_id"`
	ProjectID    string `json:"project_id"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	Version      int    `json:"version"`
	Status       string `json:"status"`
}
type ProjectGrant struct {
	ProjectID string `json:"project_id"`
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Version   int    `json:"version"`
	Status    string `json:"status"`
}
type PlanningCoverage struct {
	LoadedTasks int    `json:"loaded_tasks"`
	TotalTasks  int    `json:"total_tasks"`
	TaskMetrics string `json:"task_metrics"`
	Timeline    string `json:"timeline"`
}
type PlanningMetrics struct {
	TotalTasks        int      `json:"total_tasks"`
	AcceptedTasks     int      `json:"accepted_tasks"`
	BlockedTasks      int      `json:"blocked_tasks"`
	ScheduledTasks    int      `json:"scheduled_tasks"`
	EstimatedTasks    int      `json:"estimated_tasks"`
	EstimatedHours    float64  `json:"estimated_hours"`
	OverdueTasks      int      `json:"overdue_tasks"`
	AcceptancePercent *float64 `json:"acceptance_percent"`
}
type PlanningResponse struct {
	Iterations []platform.PlanningPeriod  `json:"iterations"`
	Milestones []platform.PlanningPeriod  `json:"milestones"`
	Baselines  []platform.BaselineSummary `json:"baselines"`
	Tasks      []platform.PlannedTask     `json:"tasks"`
	Capacity   []platform.CapacityEntry   `json:"capacity"`
	Total      int                        `json:"total"`
	NextCursor string                     `json:"next_cursor"`
	Coverage   PlanningCoverage           `json:"coverage"`
	Metrics    PlanningMetrics            `json:"metrics"`
	AsOf       string                     `json:"as_of"`
}
type BaselineSnapshot struct {
	Tasks      []platform.PlannedTask `json:"tasks"`
	Total      int                    `json:"total"`
	NextCursor string                 `json:"next_cursor"`
}
type BaselineComparison struct {
	AddedTasks             int `json:"added_tasks"`
	RemovedTasks           int `json:"removed_tasks"`
	DefinitionChangedTasks int `json:"definition_changed_tasks"`
	PlanChangedTasks       int `json:"plan_changed_tasks"`
	AcceptedTasks          int `json:"accepted_tasks"`
	TotalTasks             int `json:"total_tasks"`
}
type BaselineDetail struct {
	platform.BaselineSummary
	Snapshot   BaselineSnapshot   `json:"snapshot"`
	Comparison BaselineComparison `json:"comparison"`
	AsOf       string             `json:"as_of"`
}
type Acknowledgement struct {
	Acknowledged bool `json:"acknowledged"`
}
type NodeProgressData struct {
	OperationID string `json:"operation_id"`
	Message     string `json:"message"`
	Attempt     int    `json:"attempt"`
	Fence       int64  `json:"fence"`
	WorkerID    string `json:"worker_id"`
}
type OrganizationCreateRequest struct {
	Name           string `json:"name"`
	IdempotencyKey string `json:"idempotency_key"`
}
type SwitchOrganizationRequest struct {
	OrganizationID string `json:"organization_id"`
	IdempotencyKey string `json:"idempotency_key"`
}
type OrganizationMemberWriteRequest struct {
	Role            string `json:"role,omitempty"`
	Action          string `json:"action,omitempty"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type InvitationCreateRequest struct {
	Email          string            `json:"email"`
	Role           string            `json:"role"`
	Grants         []InvitationGrant `json:"grants,omitempty"`
	IdempotencyKey string            `json:"idempotency_key"`
}
type InvitationAcceptRequest struct {
	Token          string `json:"token"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	Password       string `json:"password"`
	IdempotencyKey string `json:"idempotency_key"`
}
type VersionedOperation struct {
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type TeamWriteRequest struct {
	Name            string `json:"name,omitempty"`
	Description     string `json:"description,omitempty"`
	Status          string `json:"status,omitempty"`
	ExpectedVersion int    `json:"expected_version,omitempty"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type TeamMemberWriteRequest struct {
	ResourceKind    string `json:"resource_kind"`
	ResourceID      string `json:"resource_id"`
	ProjectID       string `json:"project_id,omitempty"`
	Role            string `json:"role,omitempty"`
	Action          string `json:"action,omitempty"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type ProjectAccessWriteRequest struct {
	ProjectID       string `json:"project_id"`
	UserID          string `json:"user_id"`
	Role            string `json:"role,omitempty"`
	Action          string `json:"action,omitempty"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type PlanningPeriodWriteRequest struct {
	ProjectID       string `json:"project_id"`
	Title           string `json:"title,omitempty"`
	Description     string `json:"description,omitempty"`
	Status          string `json:"status,omitempty"`
	StartDate       string `json:"start_date,omitempty"`
	EndDate         string `json:"end_date,omitempty"`
	TargetDate      string `json:"target_date,omitempty"`
	Goal            string `json:"goal,omitempty"`
	ExpectedVersion int    `json:"expected_version,omitempty"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type ScheduleChange struct {
	TaskID              string   `json:"task_id"`
	ExpectedTaskVersion int      `json:"expected_task_version"`
	ExpectedVersion     int      `json:"expected_version"`
	IterationID         string   `json:"iteration_id"`
	MilestoneID         string   `json:"milestone_id"`
	StartDate           string   `json:"start_date"`
	DueDate             string   `json:"due_date"`
	EstimatedHours      *float64 `json:"estimated_hours"`
}
type ScheduleRequest struct {
	ProjectID      string           `json:"project_id"`
	Changes        []ScheduleChange `json:"changes"`
	IdempotencyKey string           `json:"idempotency_key"`
}
type BaselineCaptureRequest struct {
	ProjectID      string `json:"project_id"`
	Title          string `json:"title"`
	IdempotencyKey string `json:"idempotency_key"`
}
type RunnerProgressRequest struct {
	ProjectID   string              `json:"project_id"`
	Assignment  dag.Assignment      `json:"assignment"`
	OperationID string              `json:"operation_id"`
	Message     string              `json:"message,omitempty"`
	ToolEvent   *toolcontract.Event `json:"tool_event,omitempty"`
}

func extraModels() []any {
	return []any{platform.PlanningPeriod{}, platform.TaskPlan{}, platform.PlannedTask{}, platform.CapacityEntry{}, platform.BaselineSummary{}, SessionResponse{}, OrganizationSummary{}, OrganizationMember{}, OrganizationMemberUpdate{}, InvitationMetadata{}, InvitationCreation{}, Team{}, TeamMember{}, ProjectGrant{}, PlanningResponse{}, BaselineDetail{}, Acknowledgement{}, NodeProgressData{}, OrganizationCreateRequest{}, SwitchOrganizationRequest{}, OrganizationMemberWriteRequest{}, InvitationCreateRequest{}, InvitationAcceptRequest{}, VersionedOperation{}, TeamWriteRequest{}, TeamMemberWriteRequest{}, ProjectAccessWriteRequest{}, PlanningPeriodWriteRequest{}, ScheduleRequest{}, platform.ResourceCalendar{}, platform.ResourceCalendarSaveRequest{}, BaselineCaptureRequest{}, RunnerProgressRequest{}}
}
