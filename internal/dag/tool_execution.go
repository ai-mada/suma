package dag

import (
	"fmt"

	"github.com/ai-mada/suma/internal/toolcontract"
)

// ToolBinding returns a complete immutable tool reference; partial bindings
// never silently fall back to generic command execution.
func ToolBinding(n Node) (id, sha string, bound bool, err error) {
	rawID, idOK := n.Config["tool_profile_id"]
	rawSHA, shaOK := n.Config["tool_profile_sha256"]
	if !idOK && !shaOK {
		return "", "", false, nil
	}
	id, idString := rawID.(string)
	sha, shaString := rawSHA.(string)
	if n.Type != "agent" || !idOK || !shaOK || !idString || !shaString || !toolcontract.ValidCommandID(id) || !toolcontract.ValidDigest(sha) || len(n.Command) != 1 || !toolcontract.ValidCommandID(n.Command[0]) {
		return "", "", false, dagDiagnostic(fmt.Errorf("%w: complete agent tool binding required", ErrInvalid), "dag.tool.binding", nil)
	}
	return id, sha, true, nil
}
func validateToolEvent(n Node, e toolcontract.Event) error {
	if err := toolcontract.ValidateEvent(e); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	_, sha, bound, err := ToolBinding(n)
	if err != nil {
		return err
	}
	if !bound || e.CommandID != n.Command[0] || e.ProfileSHA256 != sha {
		return dagDiagnostic(fmt.Errorf("%w: tool event differs from fixed node", ErrInvalid), "dag.tool.event_binding", nil)
	}
	return nil
}
func validateToolResult(n Node, r Result) error {
	_, sha, bound, err := ToolBinding(n)
	if err != nil {
		return err
	}
	success := r.ExitCode == 0 && r.Error == "" && !r.Stopped && !r.OutcomeUnknown
	if r.ToolEvidence == nil {
		if bound && success {
			return dagDiagnostic(fmt.Errorf("%w: bound tool success requires terminal evidence", ErrInvalid), "dag.tool.terminal_evidence", nil)
		}
		return nil
	}
	e := r.ToolEvidence
	if err := toolcontract.ValidateEvidence(*e); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if !bound || e.Profile.CommandID != n.Command[0] || e.Profile.ProfileSHA256 != sha {
		return dagDiagnostic(fmt.Errorf("%w: tool evidence differs from fixed node", ErrInvalid), "dag.tool.result_binding", nil)
	}
	if success && (e.Terminal != "completed" || e.Profile.Protocol != "text-v1" && e.EventCount == 0) {
		return dagDiagnostic(fmt.Errorf("%w: protocol completion does not prove success", ErrInvalid), "dag.tool.protocol_success", nil)
	}
	model, err := ModelBinding(n)
	if err != nil {
		return err
	}
	if model != nil && success {
		actual := e.ModelExecution
		if actual == nil || actual.BindingSHA256 != model.BindingSHA256 || actual.DeploymentSHA256 != model.Deployment.SHA256 || actual.ParametersSHA256 != model.ParametersSHA256 || actual.Model != model.Deployment.Model || len(actual.Requests) < 1 || len(actual.Requests) > model.Parameters.MaxSteps {
			return fmt.Errorf("%w: model execution evidence differs from fixed node", ErrInvalid)
		}
	}
	if err := validateCodexResult(n, r); err != nil {
		return err
	}
	if e.Terminal == "unknown" && !r.OutcomeUnknown && !r.Stopped {
		return dagDiagnostic(fmt.Errorf("%w: unknown tool evidence requires unknown outcome", ErrInvalid), "dag.tool.unknown_outcome", nil)
	}
	return nil
}
