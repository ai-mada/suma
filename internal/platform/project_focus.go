package platform

import (
	"context"
	"strings"
)

const projectFocusItemLimit = 6

// ProjectFocus is a bounded read of current task and execution facts. Section
// totals are independent queues, never a delivery or quality denominator.
type ProjectFocus struct {
	ProjectID  string                `json:"project_id"`
	ObservedAt string                `json:"observed_at"`
	Sections   []ProjectFocusSection `json:"sections"`
}

type ProjectFocusSection struct {
	Key       string             `json:"key"`
	Total     int                `json:"total"`
	Items     []ProjectFocusItem `json:"items"`
	Truncated bool               `json:"truncated"`
}

type ProjectFocusItem struct {
	TaskID      string `json:"task_id"`
	Title       string `json:"title"`
	TaskVersion int    `json:"task_version"`
	TaskStatus  string `json:"task_status"`
	RunID       string `json:"run_id"`
	RunState    string `json:"run_state"`
	Reason      string `json:"reason"`
	UpdatedAt   string `json:"updated_at"`
	Priority    string `json:"priority"`
}

var projectFocusSections = []struct {
	key     string
	reasons []string
}{
	{"planning", []string{"task_unstarted"}},
	{"execution", []string{"run_queued", "run_active", "run_waiting", "run_paused"}},
	{"review", []string{"run_succeeded_review_pending"}},
	{"attention", []string{"task_blocked", "run_failed", "run_reconciling", "run_cancelled", "run_missing", "run_basis_changed", "run_basis_unavailable"}},
}

// The only JSON queried here is the existing protected run-binding receipt,
// not editable task data. A parse/database error fails the read closed.
func (s *Service) projectFocusBindingField(field string) string {
	path := "'$." + field + "'"
	switch s.Dialect {
	case "postgres":
		return "(CAST(p.payload AS JSONB)->>'" + field + "')"
	case "oceanbase":
		return "JSON_UNQUOTE(JSON_EXTRACT(p.payload," + path + "))"
	default:
		return "CAST(json_extract(p.payload," + path + ") AS VARCHAR(100))"
	}
}

func (s *Service) projectFocusBindingVersionType() string {
	switch s.Dialect {
	case "postgres":
		return "jsonb_typeof(CAST(p.payload AS JSONB)->'task_version')='number'"
	case "oceanbase":
		return "JSON_TYPE(JSON_EXTRACT(p.payload,'$.task_version'))='INTEGER'"
	default:
		return "json_type(p.payload,'$.task_version')='integer'"
	}
}

// task_version is written as task.Version+1 in the same BindRunTx which
// advances that task version. Equality proves only an unchanged bound task
// revision. Requirement/dependency, test and formal acceptance gates remain in
// their existing domains; this read never claims review eligibility.
func (s *Service) projectFocusQuery() string {
	field := s.projectFocusBindingField
	validBinding := "p.run_id IS NOT NULL AND " + field("binding_kind") + "='bound' AND " + field("task_id") + "=t.id AND " + field("run_id") + "=t.run_id AND " + s.projectFocusBindingVersionType() + " AND " + field("task_version") + " IS NOT NULL"
	currentBinding := field("task_version") + "=CAST(t.version AS VARCHAR(100))"
	return `SELECT t.id AS task_id,t.title,t.version AS task_version,t.status AS task_status,t.run_id,COALESCE(r.state,'') AS run_state,t.priority,
		CASE WHEN r.updated_at>t.updated_at THEN r.updated_at ELSE t.updated_at END AS updated_at,
		CASE
		 WHEN t.status='blocked' THEN 'task_blocked'
		 WHEN t.run_id<>'' AND r.id IS NULL THEN 'run_missing'
		 WHEN r.state='failed' THEN 'run_failed'
		 WHEN r.state='reconciling' OR EXISTS(SELECT 1 FROM dag_nodes n WHERE n.run_id=r.id AND n.state='reconciling') THEN 'run_reconciling'
		 WHEN r.state='cancelled' THEN 'run_cancelled'
		 WHEN r.state='succeeded' AND t.status='in_progress' THEN
		  CASE WHEN NOT COALESCE((` + validBinding + `),FALSE) THEN 'run_basis_unavailable'
		   WHEN NOT (` + currentBinding + `) THEN 'run_basis_changed'
		   ELSE 'run_succeeded_review_pending' END
		 WHEN t.run_id='' AND t.status IN ('todo','draft') THEN 'task_unstarted'
		 WHEN r.state IN ('queued','pending','ready') THEN 'run_queued'
		 WHEN r.state IN ('running','cancelling') THEN 'run_active'
		 WHEN r.state IN ('waiting','waiting_approval','waiting_external','waiting_timer','waiting_retry') THEN 'run_waiting'
		 WHEN r.state='paused' THEN 'run_paused'
		 ELSE '' END AS focus_reason
	 FROM suma_tasks t
	 LEFT JOIN dag_runs r ON r.id=t.run_id AND r.tenant_id=t.tenant_id AND r.project_id=t.project_id
	 LEFT JOIN suma_task_run_provenance p ON p.run_id=t.run_id AND p.tenant_id=t.tenant_id AND p.project_id=t.project_id AND p.task_id=t.id
	 WHERE t.tenant_id=? AND t.project_id=? AND t.status IN ('todo','draft','in_progress','blocked')`
}

// GetProjectFocus acquires the same current project authorization and scheduler
// guard as task delivery. SQL counts the complete authorized queue; at most six
// compact rows per section enter memory. Only the task's current run is joined.
func (s *Service) GetProjectFocus(ctx context.Context, actor Actor, project string) (out ProjectFocus, err error) {
	if actor.kind != actorBrowser && actor.kind != actorProjectAPI {
		return out, forbidden()
	}
	if !handoffID(project, false) {
		return out, invalid("Project focus request is invalid")
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	out = ProjectFocus{ProjectID: project, ObservedAt: automationStamp(auth.checkedAt), Sections: []ProjectFocusSection{}}
	query := s.projectFocusQuery()
	counts := map[string]int{}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT focus_reason,COUNT(*) FROM (`+query+`) focus GROUP BY focus_reason`), auth.target.tenantID, project)
	if err != nil {
		return ProjectFocus{}, err
	}
	for rows.Next() {
		var reason string
		var count int
		if err = rows.Scan(&reason, &count); err != nil {
			break
		}
		counts[reason] = count
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return ProjectFocus{}, err
	}
	for _, definition := range projectFocusSections {
		section := ProjectFocusSection{Key: definition.key, Items: []ProjectFocusItem{}}
		placeholders := make([]string, len(definition.reasons))
		args := []any{auth.target.tenantID, project}
		for i, reason := range definition.reasons {
			section.Total += counts[reason]
			placeholders[i] = "?"
			args = append(args, reason)
		}
		section.Truncated = section.Total > projectFocusItemLimit
		if section.Total > 0 {
			args = append(args, projectFocusItemLimit)
			rows, err = auth.tx.QueryContext(ctx, s.q(`SELECT task_id,title,task_version,task_status,run_id,run_state,focus_reason,updated_at,priority FROM (`+query+`) focus WHERE focus_reason IN (`+strings.Join(placeholders, ",")+`) ORDER BY updated_at DESC,task_id ASC LIMIT ?`), args...)
			if err != nil {
				return ProjectFocus{}, err
			}
			for rows.Next() {
				var item ProjectFocusItem
				if err = rows.Scan(&item.TaskID, &item.Title, &item.TaskVersion, &item.TaskStatus, &item.RunID, &item.RunState, &item.Reason, &item.UpdatedAt, &item.Priority); err != nil {
					break
				}
				section.Items = append(section.Items, item)
			}
			if err == nil {
				err = rows.Err()
			}
			rows.Close()
			if err != nil {
				return ProjectFocus{}, err
			}
		}
		out.Sections = append(out.Sections, section)
	}
	if err = auth.assertTx(auth.tx, auth.target, purposeProjectRead); err != nil {
		return ProjectFocus{}, err
	}
	return out, nil
}
