package dag

import (
	"fmt"

	"github.com/ai-mada/suma/internal/modelcontract"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
	"github.com/ai-mada/suma/internal/toolcontract"
	"unicode/utf8"
)

// ToolBinding returns a complete immutable tool reference; partial bindings
// never silently fall back to generic command execution.
func ToolBinding(n Node) (id, sha string, bound bool, err error) {
	rawID, idOK := n.Config["tool_profile_id"]
	rawSHA, shaOK := n.Config["tool_profile_sha256"]
	fixedOpenCode, openCodeErr := toolcontract.DecodeOpenCodeNodeConfiguration(n.Config)
	_, oldOctop := n.Config["octop_execution"]
	if openCodeErr != nil || fixedOpenCode != nil && n.MaxRetries != 0 || oldOctop {
		return "", "", false, dagDiagnostic(fmt.Errorf("%w: complete agent tool binding required", ErrInvalid), "dag.tool.binding", nil)
	}
	if !idOK && !shaOK {
		if fixedOpenCode != nil {
			return "", "", false, dagDiagnostic(fmt.Errorf("%w: complete agent tool binding required", ErrInvalid), "dag.tool.binding", nil)
		}
		return "", "", false, nil
	}
	id, idString := rawID.(string)
	sha, shaString := rawSHA.(string)
	if (n.Type != "agent" && n.Type != "test" && n.Type != "command") || n.Type == "command" && fixedOpenCode != nil || !idOK || !shaOK || !idString || !shaString || !toolcontract.ValidCommandID(id) || !toolcontract.ValidDigest(sha) || len(n.Command) != 1 || !toolcontract.ValidCommandID(n.Command[0]) {
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
	if !bound || e.CommandID != n.Command[0] || e.ProfileSHA256 != sha || n.Type == "command" && e.Protocol != "text-v1" {
		return dagDiagnostic(fmt.Errorf("%w: tool event differs from fixed node", ErrInvalid), "dag.tool.event_binding", nil)
	}
	return nil
}
func validateToolResult(n Node, r Result) error {
	// The domain owns parsed case facts. The scheduler only admits bounded
	// original report bytes from a fixed, report-capable test command; even a
	// malformed XML report cannot grant another node or a model this capability.
	if r.TestReport != nil && (n.Type != "test" || r.ToolEvidence == nil || r.ToolEvidence.Profile.TestReportProtocol != tr.Protocol || r.TestReport.Protocol != tr.Protocol || len(r.TestReport.XML) > 256*1024 || !utf8.ValidString(r.TestReport.XML) || r.TestReport.SHA256 != hashText(r.TestReport.XML)) {
		return dagDiagnostic(fmt.Errorf("%w: test report differs from fixed test capability", ErrInvalid), "dag.tool.test_report", nil)
	}
	_, sha, bound, err := ToolBinding(n)
	if err != nil {
		return err
	}
	fixedOpenCode, err := toolcontract.DecodeOpenCodeNodeConfiguration(n.Config)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	success := r.ExitCode == 0 && r.Error == "" && !r.Stopped && !r.OutcomeUnknown
	if r.ToolEvidence == nil {
		if fixedOpenCode != nil && r.Stopped {
			return dagDiagnostic(fmt.Errorf("%w: tool evidence differs from fixed node", ErrInvalid), "dag.tool.result_binding", nil)
		}
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
	// A fixed shell command is an administrator-defined executable, never a
	// structured agent hidden behind the more permissive command node type.
	if n.Type == "command" && (e.Profile.Adapter != "command" || e.Profile.Protocol != "text-v1") {
		return dagDiagnostic(fmt.Errorf("%w: tool evidence differs from fixed node", ErrInvalid), "dag.tool.result_binding", nil)
	}
	sdkEvidence := e.Profile.Protocol == toolcontract.OpenCodeSDKProtocol || e.Profile.OpenCodeExecution != nil || e.OpenCodeExecution != nil
	if (fixedOpenCode != nil) != sdkEvidence || fixedOpenCode != nil && (e.Profile.OpenCodeExecution == nil || *fixedOpenCode != *e.Profile.OpenCodeExecution) {
		return dagDiagnostic(fmt.Errorf("%w: tool evidence differs from fixed node", ErrInvalid), "dag.tool.result_binding", nil)
	}
	octopEvidence := e.Profile.Protocol == toolcontract.OctopProtocol || e.Profile.OctopExecution != nil || e.OctopExecution != nil
	if octopEvidence && (toolcontract.ValidateOctopNodeConfiguration(n.Config) != nil || e.Terminal == "completed" && !success) {
		return dagDiagnostic(fmt.Errorf("%w: tool evidence differs from fixed node", ErrInvalid), "dag.tool.result_binding", nil)
	}
	if err := toolcontract.ValidateOctopResultStop(*e, r.Stopped, r.OutcomeUnknown); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := toolcontract.ValidateOpenCodeResultStop(*e, r.Stopped, r.OutcomeUnknown); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if success && (e.Terminal != "completed" || e.Profile.Protocol != "text-v1" && e.EventCount == 0) {
		return dagDiagnostic(fmt.Errorf("%w: protocol completion does not prove success", ErrInvalid), "dag.tool.protocol_success", nil)
	}
	model, err := ModelBinding(n)
	if err != nil {
		return err
	}
	if e.ModelExecution != nil && model == nil || octopEvidence && model == nil {
		return fmt.Errorf("%w: model gateway evidence requires its reviewed node binding", ErrInvalid)
	}
	if model != nil && success {
		actual := e.ModelExecution
		if actual == nil || !modelcontract.EvidenceResponsesComplete(*model, *actual) {
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
