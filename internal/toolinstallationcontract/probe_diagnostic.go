package toolinstallationcontract

import (
	"fmt"
	"slices"
)

// ToolInstallationProbeDiagnostic contains only classified observation facts.
// Suggestions never authorize execution, expose a path, or replace stop proof.
type ToolInstallationProbeDiagnostic struct {
	Stage      string `json:"stage"`
	Code       string `json:"code"`
	NextAction string `json:"next_action"`
}

type ProbeDiagnostic = ToolInstallationProbeDiagnostic

func ProbeDiagnosticStages() []string {
	return []string{"resolve", "source", "version", "help", "protocol"}
}

func ProbeDiagnosticCodes() []string {
	return []string{"source_unavailable", "source_changed", "configuration_invalid", "permission_denied", "version_unsupported", "launch_failed", "probe_failed", "probe_timeout", "output_limit_exceeded", "help_contract_mismatch", "protocol_mismatch", "stop_unverified"}
}

func ProbeDiagnosticActions() []string { return []string{"guide", "scan", "none"} }

// ProbeDiagnosticStagesForCode is shared with schema generation so published
// combinations cannot drift from the server's runtime validator.
func ProbeDiagnosticStagesForCode(code string) []string {
	switch code {
	case "source_unavailable":
		return []string{"resolve", "source"}
	case "source_changed", "configuration_invalid":
		return []string{"source"}
	case "permission_denied":
		return ProbeDiagnosticStages()
	case "version_unsupported":
		return []string{"version"}
	case "help_contract_mismatch":
		return []string{"help"}
	case "protocol_mismatch":
		return []string{"protocol"}
	case "launch_failed", "probe_failed", "probe_timeout", "output_limit_exceeded", "stop_unverified":
		return []string{"version", "help", "protocol"}
	}
	return nil
}

func ProbeDiagnosticNextAction(code string) string {
	switch code {
	case "source_changed", "probe_timeout":
		return "scan"
	case "stop_unverified":
		return "none"
	default:
		if slices.Contains(ProbeDiagnosticCodes(), code) {
			return "guide"
		}
	}
	return ""
}

// ValidateProbeDiagnostic keeps the observed phase and the fixed suggested
// action inseparable. Unknown observations cannot become a new command.
func ValidateProbeDiagnostic(v ProbeDiagnostic) error {
	if !slices.Contains(ProbeDiagnosticStagesForCode(v.Code), v.Stage) || ProbeDiagnosticNextAction(v.Code) == "" || v.NextAction != ProbeDiagnosticNextAction(v.Code) {
		return fmt.Errorf("invalid installation probe diagnostic")
	}
	return nil
}

func validateItemDiagnostic(v Item) error {
	if v.Diagnostic == nil {
		return nil
	}
	if ValidateProbeDiagnostic(*v.Diagnostic) != nil || v.LaunchSupported {
		return fmt.Errorf("invalid failed installation observation")
	}
	switch v.ReasonCode {
	case "not_installed", "workspace_required", "configuration_required", "acp_configuration_required", "cursor_editor_only":
		return fmt.Errorf("installation observation contradicts its diagnostic")
	case "scan_incomplete":
		if v.Diagnostic.Stage != "resolve" {
			return fmt.Errorf("incomplete resolution requires its observed phase")
		}
	}
	return nil
}
