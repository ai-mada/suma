package platform

// These are copies of the original explicit selections, not fresh catalogue
// lookups. A source rebuild must preserve both their order and the exact
// six-field frozen facts subsequently checked by the original dispatcher.
func taskDispatchSelectionsForBindings(plan TaskDispatchPlan) ([]TaskDispatchTestSelection, error) {
	if !taskDispatchTestBindingsConsistent(plan) {
		return nil, taskDispatchTestBindingChanged()
	}
	selections := make([]TaskDispatchTestSelection, len(plan.TestBindings))
	for i, binding := range plan.TestBindings {
		selections[i] = TaskDispatchTestSelection{NodeID: binding.NodeID, ToolProfileID: binding.ToolProfileID, ExpectedToolVersion: binding.ToolProfileVersion}
	}
	return selections, nil
}

func sameTaskDispatchTestBindings(a, b []TaskDispatchTestBinding) bool {
	if len(a) != len(b) || len(a) > 64 {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Task versions and code baselines can be reviewed in a new pending revision.
// They do not implicitly authorize copying tests to another development role,
// workflow or tool. A context change requires an explicit replacement/reset.
func expertTeamTestBindingsContext(item ExpertTeamPlanItem, role ExpertTeamRole, task, roleKey string) bool {
	p := item.Plan
	return task == item.TaskID && roleKey == item.RoleKey && role.Key == item.RoleKey && role.ResourceID == item.ResourceID &&
		role.WorkflowID == p.WorkflowID && role.WorkflowVersion == p.WorkflowVersion && role.AgentNodeID == p.AgentNodeID &&
		role.ToolProfileID == p.ToolProfileID && role.ToolProfileVersion == p.ToolProfileVersion && role.ToolProfileSHA256 == p.ToolProfileSHA256
}
