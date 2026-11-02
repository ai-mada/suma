package platform

import (
	"context"
	"database/sql"

	"github.com/ai-mada/suma/internal/dag"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const (
	dispatchTestSelectionInvalid = "请选择所选开发节点关联的静态测试节点及准确工具版本，不得改变额外能力约束"
	dispatchTestProfileInvalid   = "独立测试需要另一执行节点上可写的固定命令工具及完整JUnit报告协议"
	dispatchTestBindingChanged   = "独立测试工具或计划固定依据已变化，请重新预览派发"
)

func taskDispatchTestBindingChanged() error {
	return &diagnosticAPIError{cause: &APIError{409, "task_dispatch_test_binding_changed", dispatchTestBindingChanged, nil}, key: "execution.task_dispatch.test_binding_changed"}
}

// Only the command and its exact binding constraint/reference may change.
// An unrelated capability is never removed or broadened to admit a tool.
func taskDispatchCandidateTestNode(node dag.Node, agent, command string) bool {
	if node.Type != "test" || node.Config["candidate_from"] != agent || agent == "" || len(node.Command) != 1 {
		return false
	}
	if _, present := node.Config["artifact_from"]; present {
		return false
	}
	if raw, present := node.Config["capability"]; present {
		capability, valid := raw.(string)
		if !valid || capability != "" && capability != "command:"+command && capability != "command:"+node.Command[0] {
			return false
		}
	}
	return true
}

func taskDispatchIndependentTestProfile(profile, development ToolProfile) bool {
	return profile.WorkerID != "" && development.WorkerID != "" && profile.WorkerID != development.WorkerID &&
		profile.Profile.Adapter == "command" && profile.Profile.Protocol == "text-v1" && !profile.Profile.ReadOnly &&
		profile.Profile.TestReportProtocol == tr.Protocol && tc.ValidateProfile(profile.Profile, true) == nil
}

// Resolve explicit selections inside the original command transaction. The
// workflow is already a private copy of one published definition; no source
// node, edge, condition, input, retry or timeout is synthesized here.
func (s *Service) freezeTaskDispatchTestBindingsTx(ctx context.Context, auth *authorizedCommand, project string, workflow *dag.Workflow, agent string, development ToolProfile, route dag.ProjectExecutionRoute, selections []TaskDispatchTestSelection) ([]TaskDispatchTestBinding, error) {
	if err := assertProjectCommand(auth.tx, auth, project); err != nil {
		return nil, err
	}
	if len(selections) > 64 {
		return nil, invalidDiagnostic("validation.task_dispatch_test_binding", dispatchTestSelectionInvalid, nil)
	}
	if len(selections) == 0 {
		return nil, nil
	}
	bindings := make([]TaskDispatchTestBinding, 0, len(selections))
	seen := make(map[string]bool, len(selections))
	for _, selection := range selections {
		if !diagnosisID(selection.NodeID, 150) || !handoffID(selection.ToolProfileID, false) || !ingressVersion(selection.ExpectedToolVersion) || seen[selection.NodeID] {
			return nil, invalidDiagnostic("validation.task_dispatch_test_binding", dispatchTestSelectionInvalid, nil)
		}
		seen[selection.NodeID] = true
		index := -1
		for i, node := range workflow.Nodes {
			if node.ID == selection.NodeID {
				index = i
				break
			}
		}
		if index < 0 {
			return nil, invalidDiagnostic("validation.task_dispatch_test_binding", dispatchTestSelectionInvalid, nil)
		}
		profile, err := s.toolProfileTx(ctx, auth.tx, auth.currentPrincipal, project, selection.ToolProfileID)
		if err != nil {
			return nil, err
		}
		if !taskDispatchIndependentTestProfile(profile, development) {
			return nil, invalidDiagnostic("validation.task_dispatch_test_profile", dispatchTestProfileInvalid, nil)
		}
		if !taskDispatchCandidateTestNode(workflow.Nodes[index], agent, profile.Profile.CommandID) {
			return nil, invalidDiagnostic("validation.task_dispatch_test_binding", dispatchTestSelectionInvalid, nil)
		}
		profile, err = s.validateToolDispatchTx(ctx, auth.tx, auth, project, profile.ID, selection.ExpectedToolVersion, profile.Profile.ProfileSHA256)
		if err != nil {
			return nil, err
		}
		if err = s.checkDispatchProfileRouteTx(ctx, auth.tx, auth.currentPrincipal.TenantID, project, profile, route); err != nil {
			return nil, err
		}
		node := &workflow.Nodes[index]
		if node.Config["capability"] == "command:"+node.Command[0] {
			// This is the old command's own exact constraint, not an unrelated
			// execution privilege. Replace it together with the reviewed command.
			node.Config["capability"] = "command:" + profile.Profile.CommandID
		}
		node.Command = []string{profile.Profile.CommandID}
		// candidate_from was already required, so Config is non-nil.
		node.Config["tool_profile_id"] = profile.ID
		node.Config["tool_profile_sha256"] = profile.Profile.ProfileSHA256
		bindings = append(bindings, TaskDispatchTestBinding{NodeID: node.ID, ToolProfileID: profile.ID, ToolProfileVersion: profile.Version, ToolProfileSHA256: profile.Profile.ProfileSHA256, WorkerID: profile.WorkerID, CommandID: profile.Profile.CommandID})
	}
	if err := dag.Compile(*workflow); err != nil {
		return nil, err
	}
	return bindings, nil
}

// Frozen public metadata and the actual node admitted by Core must agree.
// Checking only metadata would permit a displayed independent tool to differ
// from the command/reference in the copied workflow that will really execute.
func taskDispatchTestBindingsConsistent(plan TaskDispatchPlan) bool {
	if len(plan.TestBindings) > 64 {
		return false
	}
	seen := make(map[string]bool, len(plan.TestBindings))
	for _, binding := range plan.TestBindings {
		if !diagnosisID(binding.NodeID, 150) || !handoffID(binding.ToolProfileID, false) || !ingressVersion(binding.ToolProfileVersion) || !tc.ValidDigest(binding.ToolProfileSHA256) || binding.WorkerID == "" || !tc.ValidCommandID(binding.CommandID) || seen[binding.NodeID] {
			return false
		}
		seen[binding.NodeID] = true
		count := 0
		for _, node := range plan.Workflow.Nodes {
			if node.ID != binding.NodeID {
				continue
			}
			count++
			id, sha, bound, err := dag.ToolBinding(node)
			if !taskDispatchCandidateTestNode(node, plan.AgentNodeID, binding.CommandID) || err != nil || !bound || id != binding.ToolProfileID || sha != binding.ToolProfileSHA256 || node.Command[0] != binding.CommandID {
				return false
			}
		}
		if count != 1 {
			return false
		}
	}
	return true
}

func taskDispatchTestBindingMatchesProfile(binding TaskDispatchTestBinding, profile, development ToolProfile) bool {
	return profile.ID == binding.ToolProfileID && profile.Version == binding.ToolProfileVersion && profile.Profile.ProfileSHA256 == binding.ToolProfileSHA256 && profile.WorkerID == binding.WorkerID && profile.Profile.CommandID == binding.CommandID && taskDispatchIndependentTestProfile(profile, development)
}

func (s *Service) validateTaskDispatchTestBindingsTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project string, plan TaskDispatchPlan, development ToolProfile) error {
	if !taskDispatchTestBindingsConsistent(plan) {
		return taskDispatchTestBindingChanged()
	}
	for _, binding := range plan.TestBindings {
		profile, err := s.validateToolDispatchTx(ctx, tx, auth, project, binding.ToolProfileID, binding.ToolProfileVersion, binding.ToolProfileSHA256)
		if err != nil {
			return err
		}
		if !taskDispatchTestBindingMatchesProfile(binding, profile, development) {
			return taskDispatchTestBindingChanged()
		}
	}
	return nil
}

// Both preflight and final run-route hooks use this same exact-source check.
// The caller already owns original current command authority in this tx; this
// helper grants no access from the public plan or its display route snapshot.
func (s *Service) checkTaskDispatchTestBindingsRouteTx(ctx context.Context, tx *sql.Tx, p Principal, project string, plan TaskDispatchPlan, development ToolProfile, route dag.ProjectExecutionRoute) error {
	if !taskDispatchTestBindingsConsistent(plan) {
		return taskDispatchTestBindingChanged()
	}
	for _, binding := range plan.TestBindings {
		profile, err := s.toolProfileTx(ctx, tx, p, project, binding.ToolProfileID)
		if err != nil {
			return err
		}
		if !taskDispatchTestBindingMatchesProfile(binding, profile, development) {
			return taskDispatchTestBindingChanged()
		}
		profile, err = s.toolProfileObservationTx(ctx, tx, p, project, profile)
		if err != nil {
			return err
		}
		if !profile.Available {
			return taskDispatchTestBindingChanged()
		}
		if err = s.checkDispatchProfileRouteTx(ctx, tx, p.TenantID, project, profile, route); err != nil {
			return err
		}
	}
	return nil
}
