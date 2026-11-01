package acpcontract

import "errors"

var ErrModelDiscoveryInvalid = errors.New("invalid ACP model discovery result")

func ValidateModelCatalogue(c ACPModelCatalogue) error {
	if c.Source != "acp_session_config" || c.ProtocolVersion != 1 || !ValidFixedText(c.ConfigID, 128) || c.Models == nil || len(c.Models) < 1 || len(c.Models) > 256 {
		return ErrModelDiscoveryInvalid
	}
	seen := map[string]bool{}
	for _, m := range c.Models {
		if !ValidFixedText(m.ID, 256) || !ValidFixedText(m.Name, 4096) || seen[m.ID] || m.ConfigurationSHA256 != "" && !configurationDigest.MatchString(m.ConfigurationSHA256) {
			return ErrModelDiscoveryInvalid
		}
		seen[m.ID] = true
	}
	if c.CurrentValue != "" && !ValidFixedText(c.CurrentValue, 256) {
		return ErrModelDiscoveryInvalid
	}
	return nil
}
func ValidateModelDiscoveryResult(r ACPModelDiscoveryResult) error {
	e := r.Evidence
	if e.HostProcessExitCode != nil && !e.HostProcessStopped {
		return ErrModelDiscoveryInvalid
	}
	if r.Protocol != ModelDiscoveryProtocol || (r.Runtime != "qoder" && r.Runtime != "cursor") || !configurationDigest.MatchString(r.ExecutableSHA256) || !ValidFixedText(r.ToolVersion, 256) || !permissionText(r.AgentName, 256, false, false) || !permissionText(r.AgentVersion, 256, false, false) || e.PromptRequests < 0 || e.ConfigurationWrites < 0 || e.PermissionRequests < 0 || e.UnexpectedActivity < 0 || e.PromptRequests > 4096 || e.ConfigurationWrites > 4096 || e.PermissionRequests > 4096 || e.UnexpectedActivity > 4096 || e.HostProcessTreeStopped && !e.HostProcessStopped || e.SessionCancelWritten && !e.SessionCancelRequested || e.SessionCancelRequested && !e.SessionCreated || !ValidHostExitCode(e.HostProcessExitCode) {
		return ErrModelDiscoveryInvalid
	}
	if e.HostStoppedAfterRPC && (r.Runtime != "cursor" || !e.SessionCreated || !e.SessionCancelRequested || !e.SessionCancelWritten || !e.HostProcessStopped || !e.HostProcessTreeStopped || e.HostProcessExitCode == nil || e.PromptRequests != 0 || e.ConfigurationWrites != 0 || e.PermissionRequests != 0 || e.UnexpectedActivity != 0) {
		return ErrModelDiscoveryInvalid
	}
	switch r.State {
	case "ready":
		if r.ErrorCode != "" || r.Catalogue == nil || ValidateModelCatalogue(*r.Catalogue) != nil || !e.SessionCreated || !e.HostProcessStopped || !e.HostProcessTreeStopped || e.PromptRequests != 0 || e.ConfigurationWrites != 0 || e.PermissionRequests != 0 || e.UnexpectedActivity != 0 || e.SessionCancelRequested && !e.SessionCancelWritten || e.HostProcessExitCode != nil && *e.HostProcessExitCode != 0 && !e.HostStoppedAfterRPC {
			return ErrModelDiscoveryInvalid
		}
	case "failed", "unknown":
		if r.Catalogue != nil {
			return ErrModelDiscoveryInvalid
		}
		switch r.ErrorCode {
		case "acp_discovery_invalid", "acp_discovery_unknown", "acp_discovery_unexpected_activity", "acp_discovery_model_unavailable", "acp_discovery_not_stopped", "acp_discovery_failed":
		default:
			return ErrModelDiscoveryInvalid
		}
	default:
		return ErrModelDiscoveryInvalid
	}
	return nil
}
