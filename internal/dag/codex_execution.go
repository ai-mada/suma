package dag

import (
	"fmt"

	cc "github.com/ai-mada/suma/internal/codexcontract"
)

// CodexBinding is independent of native provider bindings. Partial/invalid
// configuration can never become an unbound legacy execution or a hard cap.
func CodexBinding(n Node) (*cc.Binding, error) {
	raw, exists := n.Config["codex_execution"]
	if skills, selected := n.Config["execution_skills"]; selected {
		if !exists || n.Type != "agent" {
			return nil, fmt.Errorf("%w: native skills require a fixed Codex agent", ErrInvalid)
		}
		if bundle, err := cc.DecodeExecutionSkillBundle(skills); err != nil || bundle == nil {
			return nil, fmt.Errorf("%w: invalid native skill bundle", ErrInvalid)
		}
	}
	if !exists {
		return nil, nil
	}
	if n.MaxRetries != 0 || cc.HasNodeOverrides(n.Config) {
		return nil, fmt.Errorf("%w: invalid Codex execution binding", ErrInvalid)
	}
	b, err := cc.DecodeBinding(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid Codex execution binding", ErrInvalid)
	}
	_, _, bound, err := ToolBinding(n)
	if err != nil || !bound || n.Type != "agent" {
		return nil, fmt.Errorf("%w: Codex execution requires a fixed agent tool", ErrInvalid)
	}
	return &b, nil
}

func validateCodexResult(n Node, r Result) error {
	b, err := CodexBinding(n)
	if err != nil || b == nil {
		return err
	}
	success := r.ExitCode == 0 && r.Error == "" && !r.Stopped && !r.OutcomeUnknown
	bundle, err := cc.DecodeExecutionSkillBundle(n.Config["execution_skills"])
	if err != nil {
		return fmt.Errorf("%w: invalid fixed Codex skills", ErrInvalid)
	}
	var skills *cc.ExecutionSkillEvidence
	if r.ToolEvidence != nil && r.ToolEvidence.CodexExecution != nil {
		skills = r.ToolEvidence.CodexExecution.Skills
	}
	if skills != nil && (bundle == nil || !cc.ExecutionSkillEvidenceMatchesBundle(*bundle, *skills)) {
		return fmt.Errorf("%w: Codex skill evidence differs from fixed node", ErrInvalid)
	}
	if success && bundle != nil && (skills == nil || !skills.Materialized || !skills.CatalogAcknowledged || !skills.InvocationRequested || !skills.TurnAcknowledged || !skills.FilesVerified || skills.FailureCode != "") {
		return fmt.Errorf("%w: successful Codex run lacks complete skill evidence", ErrInvalid)
	}
	if success && (r.ToolEvidence == nil || r.ToolEvidence.CodexExecution == nil || !cc.EvidenceMatchesBinding(*b, *r.ToolEvidence.CodexExecution)) {
		return fmt.Errorf("%w: Codex configuration evidence differs from fixed node", ErrInvalid)
	}
	return nil
}
