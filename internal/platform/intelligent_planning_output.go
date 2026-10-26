package platform

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

const planningOutputLimit = 128 << 10

var planningRiskKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$`)

func planningText(value string, maximum int) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" && len(value) <= maximum && !strings.ContainsRune(value, 0)
}

func planningRequired(raw json.RawMessage, keys ...string) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return false
	}
	for _, key := range keys {
		value, ok := object[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	return true
}

func parsePlanningOutput(raw string, source PlanningSource) (out PlanningOutput, err error) {
	bad := func() (PlanningOutput, error) { return PlanningOutput{}, invalid(planningInvalidOutput) }
	if len(raw) > planningOutputLimit || !utf8.ValidString(raw) || validateIngressJSON([]byte(raw)) != nil || !planningRequired([]byte(raw), "schema_version", "summary", "rationale", "actions", "risks") {
		return bad()
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil {
		return bad()
	}
	if _, err = decoder.Token(); err != io.EOF {
		return bad()
	}
	var fields struct {
		Actions []json.RawMessage `json:"actions"`
		Risks   []json.RawMessage `json:"risks"`
	}
	if json.Unmarshal([]byte(raw), &fields) != nil {
		return bad()
	}
	if out.SchemaVersion != 1 || !planningText(out.Summary, 2000) || !planningText(out.Rationale, 8000) || len(out.Actions) < 1 || len(out.Actions) > 20 || len(out.Risks) > 20 {
		return bad()
	}
	selected := map[string]bool{}
	for _, task := range source.Tasks {
		selected[task.Task.ID] = true
	}
	seen := map[string]bool{}
	for i, action := range out.Actions {
		if !planningRequired(fields.Actions[i], "task_id", "iteration_id", "milestone_id", "start_date", "due_date") {
			return bad()
		}
		var actionFields map[string]json.RawMessage
		json.Unmarshal(fields.Actions[i], &actionFields)
		if _, ok := actionFields["estimated_hours"]; !ok {
			return bad()
		}
		if !selected[action.TaskID] || seen[action.TaskID] || !handoffID(action.IterationID, true) || !handoffID(action.MilestoneID, true) || !validDate(action.StartDate) || !validDate(action.DueDate) || action.StartDate != "" && action.DueDate != "" && action.StartDate > action.DueDate {
			return bad()
		}
		if action.EstimatedHours != nil && (math.IsNaN(*action.EstimatedHours) || math.IsInf(*action.EstimatedHours, 0) || *action.EstimatedHours < 0 || *action.EstimatedHours > 100000) {
			return bad()
		}
		seen[action.TaskID] = true
	}
	seen = map[string]bool{}
	for i, risk := range out.Risks {
		if !planningRequired(fields.Risks[i], "key", "severity", "title", "detail", "task_ids") || !planningRiskKey.MatchString(risk.Key) || seen[risk.Key] || risk.Severity != "info" && risk.Severity != "warning" && risk.Severity != "critical" || !planningText(risk.Title, 200) || !planningText(risk.Detail, 4000) || len(risk.TaskIDs) > 20 {
			return bad()
		}
		seen[risk.Key] = true
		ids := map[string]bool{}
		for _, id := range risk.TaskIDs {
			if !selected[id] || ids[id] {
				return bad()
			}
			ids[id] = true
		}
	}
	return out, nil
}

func planningScheduleChanges(source PlanningSource, proposal PlanningOutput) []any {
	versions := map[string]PlanningSourceTask{}
	for _, task := range source.Tasks {
		versions[task.Task.ID] = task
	}
	changes := make([]any, 0, len(proposal.Actions))
	for _, action := range proposal.Actions {
		task := versions[action.TaskID]
		var hours any
		if action.EstimatedHours != nil {
			hours = *action.EstimatedHours
		}
		changes = append(changes, map[string]any{"task_id": action.TaskID, "expected_task_version": task.Task.Version, "expected_version": task.Plan.Version, "iteration_id": action.IterationID, "milestone_id": action.MilestoneID, "start_date": action.StartDate, "due_date": action.DueDate, "estimated_hours": hours})
	}
	return changes
}
