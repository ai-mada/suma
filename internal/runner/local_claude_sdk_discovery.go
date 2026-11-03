package runner

import (
	"context"
	"strings"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// SDK installation discovery observes the native program, never SDK assets,
// model authentication, a launch command, or dispatch permission.
func discoverLocalClaudeSDK(ctx context.Context, entry localToolDirectoryEntry, paths []string, probe func(context.Context, CommandSpec) (string, error)) InstalledToolDiscoveryItem {
	item := InstalledToolDiscoveryItem{AdapterID: entry.id, Product: entry.product, ReasonCode: "not_installed"}
	spec, _, installed, reason, diagnostic := resolveLocalToolObserved(entry, paths)
	item.Installed, item.ReasonCode = installed, reason
	item.Diagnostic = diagnostic
	if reason != "resolved" {
		return item
	}
	exe, sha, err := qoderNativeTarget(ctx, spec.Executable)
	if err != nil {
		item.ReasonCode = "unsupported_launcher"
		item.Diagnostic = probeDiagnostic(newProbeFailure("source", "source_unavailable", err))
		return item
	}
	version, err := probe(ctx, CommandSpec{Adapter: "claude", Protocol: tc.ClaudeSDKProtocol, Executable: exe})
	version = strings.TrimSuffix(version, " (Claude Code)")
	if err != nil || !localToolSafeText(version, 256) || strings.TrimSpace(version) != version {
		item.ReasonCode = "probe_failed"
		if err == nil {
			err = newProbeFailure("version", "probe_failed", nil)
		}
		item.Diagnostic = probeDiagnostic(err)
		return item
	}
	if !sameACPProbeInstallation(ctx, exe, sha) {
		item.ReasonCode = "probe_failed"
		item.Diagnostic = probeDiagnostic(newProbeFailure("source", "source_changed", nil))
		return item
	}
	item.Executable, item.ExecutableSHA256, item.Version, item.ReasonCode = exe, sha, version, "configuration_required"
	if version != tc.ClaudeSDKCLIVersion {
		item.ReasonCode = "version_unsupported"
		item.Diagnostic = probeDiagnostic(newProbeFailure("version", "version_unsupported", nil))
	}
	return item
}

func passiveClaudeSDKVersion(ctx context.Context, exe string) (string, error) {
	return passiveNativeToolVersion(ctx, exe, []string{"DISABLE_AUTOUPDATER=1", "CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1", "CLAUDE_CODE_DISABLE_AGENT_VIEW=1"})
}
