package acpcontract

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var configurationDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

// ValidFixedText does not normalize an agent option, model or version. Exact
// bytes remain part of administrator configuration and protocol comparison.
func ValidFixedText(value string, limit int) bool {
	if value == "" || strings.TrimSpace(value) != value || len(value) > limit || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func ValidateCapability(c Capability) error {
	validRuntime := c.Runtime == "qoder" && c.AuthStrategy == "existing_cli_login" || c.Runtime == "cursor" && c.AuthStrategy == "explicit_api_key_env"
	if !validRuntime || c.ProtocolVersion != 1 || c.SDKVersion != SDKVersion || !configurationDigest.MatchString(c.ConfigurationSHA256) || !ValidFixedText(c.Model, 256) || c.ModelSource != "administrator_session_config" || c.BudgetMode != "observe_only" || c.IdentityEvidence != "configuration_ack_only" || c.PermissionMode != "human_once" {
		return fmt.Errorf("invalid fixed ACP execution capability")
	}
	return nil
}

// Exit status is the actual host fact, including signalled Unix termination
// (-1) and Windows status values. It is independent of RPC operation success.
func ValidHostExitCode(code *int) bool {
	return code == nil || *code >= -1 && int64(*code) <= 4294967295
}
