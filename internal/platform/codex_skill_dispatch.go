package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const codexSkillsInvalid = "Invalid Codex native skill binding"
const codexSkillsChanged = "Codex native skill binding changed; prepare a new dispatch preview"

func taskExecutionSkills(profile tc.Profile, task TaskContext) (*cc.ExecutionSkillBundle, error) {
	if !supportsCodexExecution(ToolProfile{Profile: profile}) {
		return nil, nil
	}
	documents := []cc.ExecutionSkillDocument{}
	for _, item := range task.Knowledge {
		if item.Kind == "skill" {
			documents = append(documents, cc.ExecutionSkillDocument{ID: item.ID, Revision: item.Revision, SHA256: item.SHA256, Title: item.Title, Description: item.Summary, Content: item.Content})
		}
	}
	bundle, err := cc.BuildExecutionSkillBundle(documents)
	if err != nil {
		return nil, invalid(codexSkillsInvalid)
	}
	return bundle, nil
}

// Only model text changes. The protected Context, revision/source SHA and
// original complete-input size gate keep their existing meanings.
func taskNativeSkillPromptContext(task TaskContext, bundle *cc.ExecutionSkillBundle) any {
	if bundle == nil {
		return taskKnowledgePromptContext(task)
	}
	projected := task
	projected.Knowledge = append([]ProjectKnowledgeContextItem(nil), task.Knowledge...)
	index := 0
	for i := range projected.Knowledge {
		if projected.Knowledge[i].Kind == "skill" {
			d := bundle.Documents[index]
			projected.Knowledge[i].Content = fmt.Sprintf("Approved native skill attachment $%s (id=%s; revision=%d; sha256=%s; document_sha256=%s). The original approved content is supplied through this native attachment.", d.Name, d.ID, d.Revision, d.SHA256, d.DocumentSHA256)
			index++
		}
	}
	return taskKnowledgePromptContext(projected)
}

func bindTaskExecutionSkills(w *dag.Workflow, agent string, bundle *cc.ExecutionSkillBundle) error {
	found := false
	for i := range w.Nodes {
		n := &w.Nodes[i]
		delete(n.Config, "execution_skills")
		if n.ID == agent && bundle != nil {
			found = true
			var value map[string]any
			if err := json.Unmarshal([]byte(stringify(bundle)), &value); err != nil {
				return err
			}
			if n.Config == nil {
				n.Config = map[string]any{}
			}
			n.Config["execution_skills"] = value
		}
	}
	if bundle != nil {
		if !found {
			return invalid(codexSkillsInvalid)
		}
		return dag.Compile(*w)
	}
	return nil
}

func validateTaskExecutionSkills(plan TaskDispatchPlan, profile tc.Profile) error {
	expected, err := taskExecutionSkills(profile, plan.Context)
	if err != nil {
		return err
	}
	if stringify(expected) != stringify(plan.ExecutionSkills) {
		return conflict(codexSkillsChanged)
	}
	return executionSkillWorkflowMatches(plan)
}

func executionSkillWorkflowMatches(plan TaskDispatchPlan) error {
	found := false
	for _, n := range plan.Workflow.Nodes {
		value, present := n.Config["execution_skills"]
		if n.ID != plan.AgentNodeID {
			if present {
				return conflict(codexSkillsChanged)
			}
			continue
		}
		found = true
		bundle, err := cc.DecodeExecutionSkillBundle(value)
		if err != nil || present && bundle == nil || stringify(bundle) != stringify(plan.ExecutionSkills) {
			return conflict(codexSkillsChanged)
		}
		if bundle != nil && (plan.CodexExecution == nil || n.Type != "agent" || len(n.Command) != 1 || n.Config["tool_profile_id"] != plan.ToolProfileID || n.Config["tool_profile_sha256"] != plan.ToolProfileSHA256) {
			return conflict(codexSkillsChanged)
		}
	}
	if !found {
		return conflict(codexSkillsChanged)
	}
	return nil
}

// A workflow value alone never grants file authority. A started task plan is
// the durable source; current publication is deliberately not re-resolved here.
func (s *Service) executionSkillsForNodeTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string, spec dag.Node) (*cc.ExecutionSkillBundle, error) {
	value, present := spec.Config["execution_skills"]
	actual, err := cc.DecodeExecutionSkillBundle(value)
	if err != nil || present && actual == nil {
		return nil, conflict(codexSkillsChanged)
	}
	var raw, state string
	err = tx.QueryRowContext(ctx, s.q(`SELECT state,payload FROM suma_task_dispatch_plans WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, run).Scan(&state, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		if present {
			return nil, forbidden()
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var plan TaskDispatchPlan
	if json.Unmarshal([]byte(raw), &plan) != nil || plan.ProjectID != project || plan.RunID != run || state != "started" {
		return nil, conflict(codexSkillsChanged)
	}
	if plan.AgentNodeID != node {
		if present {
			return nil, forbidden()
		}
		return nil, nil
	}
	if err = executionSkillWorkflowMatches(plan); err != nil {
		return nil, err
	}
	if stringify(actual) != stringify(plan.ExecutionSkills) {
		return nil, conflict(codexSkillsChanged)
	}
	var profile tc.Profile
	if plan.CodexExecution != nil {
		profile = tc.Profile{Adapter: "codex", Protocol: cc.Protocol}
	}
	expected, err := taskExecutionSkills(profile, plan.Context)
	if err != nil || stringify(expected) != stringify(plan.ExecutionSkills) {
		return nil, conflict(codexSkillsChanged)
	}
	if expected != nil && (spec.ID != plan.AgentNodeID || spec.Type != "agent" || spec.Config["tool_profile_id"] != plan.ToolProfileID || spec.Config["tool_profile_sha256"] != plan.ToolProfileSHA256) {
		return nil, conflict(codexSkillsChanged)
	}
	return actual, nil
}

func (s *Service) validateExecutionSkillResultTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) error {
	var raw string
	if err := tx.QueryRowContext(ctx, s.q(`SELECT n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`), tenant, project, result.RunID, result.NodeID).Scan(&raw); err != nil {
		return err
	}
	var node dag.Node
	if json.Unmarshal([]byte(raw), &node) != nil {
		return invalid(codexSkillsInvalid)
	}
	bundle, err := s.executionSkillsForNodeTx(ctx, tx, tenant, project, result.RunID, result.NodeID, node)
	if err != nil {
		return err
	}
	var proof *cc.ExecutionSkillEvidence
	if result.ToolEvidence != nil && result.ToolEvidence.CodexExecution != nil {
		proof = result.ToolEvidence.CodexExecution.Skills
	}
	if proof != nil && (bundle == nil || !cc.ExecutionSkillEvidenceMatchesBundle(*bundle, *proof)) {
		return invalid(codexSkillsInvalid)
	}
	return nil
}
