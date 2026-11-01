package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/displaycontract"
)

const taskDispatchOptionsInvalid = "Task dispatch options request is invalid"

// Options are current observations, not execution grants or reservations.
// Workflow/task versions are mandatory so an old screen cannot silently rank
// tools against a different engineering objective.
type TaskDispatchOptionsRequest struct {
	ProjectID           string `json:"project_id"`
	TaskID              string `json:"task_id"`
	ExpectedTaskVersion int    `json:"expected_task_version"`
	WorkflowID          string `json:"workflow_id"`
	WorkflowVersion     int    `json:"workflow_version"`
	AgentNodeID         string `json:"agent_node_id"`
	Limit               int    `json:"limit,omitempty"`
	Before              string `json:"before,omitempty"`
}

type TaskDispatchToolOption struct {
	Tool             ToolProfile                        `json:"tool"`
	Eligible         bool                               `json:"eligible"`
	ReasonCode       string                             `json:"reason_code"`
	Reason           string                             `json:"reason"`
	ReasonDiagnostic *displaycontract.DisplayDiagnostic `json:"reason_diagnostic,omitempty"`
	ModelState       string                             `json:"model_state"`
	CostState        string                             `json:"cost_state"`
	EligibleBasis    string                             `json:"eligible_basis"`
}

type TaskDispatchOptions struct {
	ProjectID                string                    `json:"project_id"`
	TaskID                   string                    `json:"task_id"`
	TaskVersion              int                       `json:"task_version"`
	WorkflowID               string                    `json:"workflow_id"`
	WorkflowVersion          int                       `json:"workflow_version"`
	AgentNodeID              string                    `json:"agent_node_id"`
	ObservedAt               string                    `json:"observed_at"`
	Context                  TaskContext               `json:"context"`
	ExecutionRoute           dag.ProjectExecutionRoute `json:"execution_route"`
	Items                    []TaskDispatchToolOption  `json:"items"`
	Total                    int                       `json:"total"`
	NextCursor               string                    `json:"next_cursor"`
	RecommendedToolProfileID string                    `json:"recommended_tool_profile_id"`
}

// A profile-page cursor cannot be reused after changing the engineering basis.
// The inner profile reader retains its existing authority/projection contract;
// only its stable last-profile position is re-enveloped in this exact scope.
func taskDispatchOptionsCursorScope(req TaskDispatchOptionsRequest) string {
	return "task_dispatch_options:" + digest(stringify(map[string]any{"task_id": req.TaskID, "task_version": req.ExpectedTaskVersion, "workflow_id": req.WorkflowID, "workflow_version": req.WorkflowVersion, "agent_node_id": req.AgentNodeID}))
}

// GetTaskDispatchOptions evaluates a single page in the same current-authority
// read transaction. It never creates a plan/run, changes a template, reserves
// budget, probes an executable, invokes a model, or claims a worker slot.
//
// The suggested ID is only the first eligible profile in this stable ID-ordered
// page. No quality/latency/cost score is invented. Writable profiles presently
// do not declare an independently selectable/priced model in toolcontract;
// model selection stays fixed by the administrator's exact tool profile.
func (s *Service) GetTaskDispatchOptions(ctx context.Context, engine *dag.Engine, actor Actor, project, taskID string, req TaskDispatchOptionsRequest) (out TaskDispatchOptions, err error) {
	out.Items = []TaskDispatchToolOption{}
	if req.Limit == 0 {
		req.Limit = 50
	}
	if engine == nil || req.ProjectID != project || req.TaskID != taskID || !handoffID(project, false) || !handoffID(taskID, false) || req.ExpectedTaskVersion < 1 || !diagnosisID(req.WorkflowID, 100) || req.WorkflowVersion < 1 || !diagnosisID(req.AgentNodeID, 150) || req.Limit < 1 || req.Limit > 200 || len(req.Before) > 4096 || !utf8.ValidString(req.Before) {
		return out, invalid(taskDispatchOptionsInvalid)
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: ""}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		bounded, tx, p := auth.ctx, auth.tx, auth.currentPrincipal
		if e := s.rejectReservedCandidateRepairTaskTx(bounded, tx, p.TenantID, project, taskID, ""); e != nil {
			return e
		}
		task, e := s.taskContextForDispatchTx(bounded, auth, project, taskID, nil)
		if e != nil {
			return e
		}
		if task.Task.Version != req.ExpectedTaskVersion {
			return conflict("任务已经变更，请刷新后重新预览")
		}
		// Match the original preview's fixed, non-managed template lookup. The
		// pure workflow builder then performs exactly the same capability,
		// protected-baseline, context-size and independent-test validation.
		var raw string
		e = tx.QueryRowContext(bounded, s.q(`SELECT w.spec FROM dag_workflows w WHERE w.tenant_id=? AND w.project_id=? AND w.id=? AND w.version=? AND NOT EXISTS(SELECT 1 FROM dag_managed_workflows m WHERE m.tenant_id=w.tenant_id AND m.project_id=w.project_id AND m.workflow_id=w.id)`), p.TenantID, project, req.WorkflowID, req.WorkflowVersion).Scan(&raw)
		if errors.Is(e, sql.ErrNoRows) {
			return missing()
		}
		if e != nil {
			return e
		}
		var workflow dag.Workflow
		if e = json.Unmarshal([]byte(raw), &workflow); e != nil {
			return e
		}
		route, e := engine.ReadProjectExecutionRouteTx(bounded, tx, p.TenantID, project)
		if e != nil {
			return e
		}
		cursorScope := taskDispatchOptionsCursorScope(req)
		position, e := automationDecodeCursor(p, project, cursorScope, req.Before)
		if e != nil {
			return e
		}
		profileQuery := ToolProfileListQuery{State: "active", Limit: req.Limit}
		profileCursorScope := toolProfileListCursorScope(p, profileQuery)
		innerCursor := ""
		if position != "" {
			innerCursor = automationEncodeCursor(p, project, profileCursorScope, position)
		}
		profiles, e := s.listToolProfilesCurrentTx(bounded, auth, project, ToolProfileListQuery{State: "active", Limit: req.Limit, Cursor: innerCursor})
		if e != nil {
			return e
		}
		nextCursor := ""
		if profiles.NextCursor != "" {
			last, cursorErr := automationDecodeCursor(p, project, profileCursorScope, profiles.NextCursor)
			if cursorErr != nil {
				return cursorErr
			}
			nextCursor = automationEncodeCursor(p, project, cursorScope, last)
		}
		clock, e := s.automationClock(bounded, tx)
		if e != nil {
			return e
		}
		out = TaskDispatchOptions{ProjectID: project, TaskID: taskID, TaskVersion: task.Task.Version, WorkflowID: req.WorkflowID, WorkflowVersion: req.WorkflowVersion, AgentNodeID: req.AgentNodeID, ObservedAt: automationStamp(clock), Context: task, ExecutionRoute: route, Total: profiles.Total, NextCursor: nextCursor, Items: []TaskDispatchToolOption{}}
		for _, profile := range profiles.Items {
			option := TaskDispatchToolOption{Tool: profile, ReasonCode: "eligible", ModelState: "fixed_by_tool", CostState: "unknown", EligibleBasis: "current_observation_and_compatibility"}
			switch {
			case !profile.Available:
				option.ReasonCode, option.Reason, option.ReasonDiagnostic = "tool_unavailable", profile.Reason, profile.ReasonDiagnostic
			default:
				if e = s.checkDispatchProfileRouteTx(bounded, tx, p.TenantID, project, profile, route); e != nil {
					var domain *APIError
					if !errors.As(e, &domain) {
						return e
					}
					option.ReasonCode, option.Reason = "route_incompatible", domain.Message
				} else if _, e = buildTaskDispatchWorkflow(workflow, req.AgentNodeID, profile.ID, profile.Profile, task); e != nil {
					var domain *APIError
					if !errors.As(e, &domain) {
						return e
					}
					option.ReasonCode, option.Reason = "workflow_incompatible", domain.Message
				} else {
					option.Eligible = true
					if out.RecommendedToolProfileID == "" {
						out.RecommendedToolProfileID = profile.ID
					}
				}
			}
			out.Items = append(out.Items, option)
		}
		return nil
	})
	return out, err
}
