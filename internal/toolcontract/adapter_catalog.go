package toolcontract

import (
	"fmt"
	ac "github.com/ai-mada/suma/internal/acpcontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
)

// AdapterDescriptor is an implementation catalog, never an installation,
// authorization, health, billing or execution-result claim. References identify
// repository implementation/contracts, not fetched third-party instructions.
type AdapterCapability struct {
	Key         string   `json:"key"`
	Support     string   `json:"support"`
	SourceRefs  []string `json:"source_refs"`
	Limitations []string `json:"limitations"`
}
type AdapterDescriptor struct {
	ID             string              `json:"id"`
	Product        string              `json:"product"`
	EntrypointKind string              `json:"entrypoint_kind"`
	Direction      string              `json:"direction"`
	Adapter        string              `json:"adapter,omitempty"`
	Protocol       string              `json:"protocol,omitempty"`
	Implementation string              `json:"implementation"`
	SetupKind      string              `json:"setup_kind"`
	Capabilities   []AdapterCapability `json:"capabilities"`
	SourceRefs     []string            `json:"source_refs"`
	Limitations    []string            `json:"limitations"`
}

var adapterCapabilityKeys = []string{"task_start", "history_read", "session_load", "new_turn", "interrupted_resume", "cancel_request", "terminal_observation", "host_stop_proof", "usage_observation", "provider_billing", "candidate_capture", "independent_test", "model_selection", "gateway_budget", "workspace_read", "workspace_write", "human_input", "configuration_export", "project_access"}

func adapterDescriptor(id, product, kind, direction, implementation, setup, adapter, protocol, source string, supported, limits []string) AdapterDescriptor {
	d := AdapterDescriptor{ID: id, Product: product, EntrypointKind: kind, Direction: direction, Implementation: implementation, SetupKind: setup, Adapter: adapter, Protocol: protocol, SourceRefs: []string{source}, Limitations: append([]string{"implementation_not_connection", "unsupported_is_suma_contract"}, limits...), Capabilities: []AdapterCapability{}}
	for _, key := range adapterCapabilityKeys {
		state := "unsupported"
		if implementation == "planned" || implementation == "guide_only" {
			state = "unverified"
		}
		for _, present := range supported {
			if key == present {
				state = "supported"
			}
		}
		d.Capabilities = append(d.Capabilities, AdapterCapability{Key: key, Support: state, SourceRefs: []string{source}, Limitations: []string{}})
	}
	return d
}

// AdapterCatalog builds fresh objects on each call. There is no mutable plugin
// registry and no user-submitted manifest can enable a Runner protocol.
func AdapterCatalog() []AdapterDescriptor {
	run := func(id, product, kind, adapter, protocol, source string, caps, limits []string) AdapterDescriptor {
		return adapterDescriptor(id, product, kind, "suma_to_tool", "implemented", "runner_profile", adapter, protocol, source, caps, append([]string{"profile_and_current_authorization_required", "independent_quality_gate", "host_stop_requires_evidence"}, limits...))
	}
	common := []string{"task_start", "cancel_request", "terminal_observation", "host_stop_proof", "candidate_capture"}
	with := func(extra ...string) []string { return append(append([]string{}, common...), extra...) }
	rows := []AdapterDescriptor{
		run("suma-native", "suma", "cli", "suma", "suma-jsonl-v1", "internal/toolcontract/validation.go", with("usage_observation", "model_selection", "gateway_budget", "workspace_read", "workspace_write"), []string{"profile_specific_capabilities", "provider_usage_may_be_partial"}),
		run("command-cli", "Command", "cli", "command", "text-v1", "internal/runner/execute.go", with("human_input", "independent_test"), []string{"profile_specific_capabilities"}),
		run("codex-jsonl", "Codex", "cli", "codex", "codex-jsonl-v1", "internal/runner/tool_stream.go", with("usage_observation"), []string{"foreground_only_no_resume", "provider_usage_may_be_partial"}),
		run("codex-app-server", "Codex", "app_server", "codex", cc.Protocol, "contracts/codex-app-server.md", with("usage_observation", "model_selection", "workspace_read"), []string{"observe_only_budget", "provider_usage_may_be_partial"}),
		run("opencode-sdk", "OpenCode", "sdk", "opencode", OpenCodeSDKProtocol, "contracts/opencode-sdk.md", with("usage_observation", "workspace_read", "workspace_write"), []string{"observe_only_budget", "provider_usage_may_be_partial"}),
		run("octop-acp", "Octop Harness", "sdk", "octop", OctopProtocol, "contracts/octop-harness-acp.md", with("usage_observation", "model_selection", "gateway_budget", "workspace_read", "workspace_write"), []string{"provider_usage_may_be_partial"}),
		run("dsh-multica", "DeepSeek Harness / DSH", "cli", "harness", "dsh-multica-jsonl-v1", "contracts/team-cli-runtime.md", with("usage_observation"), []string{"fixed_dsh_multica_profile", "foreground_only_no_resume", "provider_usage_may_be_partial"}),
		run("codebuddy-cli", "CodeBuddy", "cli", "codebuddy", "codebuddy-jsonl-v1", "contracts/team-cli-runtime.md", with("usage_observation"), []string{"foreground_only_no_resume", "provider_usage_may_be_partial", "workbuddy_not_codebuddy"}),
		run("claude-code-cli", "Claude Code", "cli", "claude", ClaudeProtocol, "contracts/claude-cli.md", with("usage_observation"), []string{"foreground_only_no_resume", "observe_only_budget", "provider_usage_may_be_partial", "fixed_claude_cli_configuration", "claude_cli_not_sdk", "claude_permissions_require_admin_policy"}),
		run("qoder-acp", "Qoder", "cli", "qoder", ac.Protocol, "contracts/acp-cli-execution.md", with("usage_observation", "human_input"), []string{"foreground_only_no_resume", "observe_only_budget", "provider_usage_may_be_partial", "fixed_acp_cli_configuration", "acp_human_once_permissions", "trusted_cli_not_os_sandbox"}),
		run("cursor-cli", "Cursor", "cli", "cursor", ac.Protocol, "contracts/cursor-acp-execution.md", with("usage_observation", "human_input"), []string{"foreground_only_no_resume", "observe_only_budget", "provider_usage_may_be_partial", "fixed_acp_cli_configuration", "acp_human_once_permissions", "trusted_cli_not_os_sandbox"}),
	}
	for _, client := range []struct {
		id, product string
		export      bool
	}{{"cursor", "Cursor", true}, {"claude-code", "Claude Code", true}, {"codex", "Codex", true}, {"opencode", "OpenCode", true}, {"qoder", "Qoder", false}, {"zed", "Zed", false}, {"workbuddy", "WorkBuddy", false}} {
		id := client.id + "-client"
		if client.id == "workbuddy" {
			id = "workbuddy-connector"
		}
		implementation, setup := "guide_only", "manual_guide"
		caps := []string{}
		limits := []string{"manual_client_verification"}
		if client.export {
			implementation, setup = "implemented", "client_export"
			caps = []string{"configuration_export"}
			limits = append(limits, "configuration_export_only")
		}
		row := adapterDescriptor(id, client.product, "mcp_client", "tool_to_suma", implementation, setup, "", "", "docs/integrations/developer-clients.md", caps, limits)
		// No client export proves the client has connected or possesses project access.
		for i := range row.Capabilities {
			if row.Capabilities[i].Key == "project_access" {
				row.Capabilities[i].Support = "unverified"
			}
		}
		rows = append(rows, row)
	}
	rows = append(rows, adapterDescriptor("harness-guide", "Harness", "sdk", "suma_to_tool", "guide_only", "manual_guide", "", "", "docs/integrations/development-tool-matrix.md", nil, []string{"manual_runtime_verification"}))
	rows = append(rows, adapterDescriptor("workbuddy-cloud", "WorkBuddy", "cloud_api", "suma_to_tool", "implemented", "manual_guide", "", "", "contracts/workbuddy-cloud-runner.md", []string{"task_start"}, []string{"workbuddy_not_codebuddy", "cloud_fixed_delegation_required", "cloud_start_only", "cloud_delivery_not_verified", "cloud_budget_not_enforced"}))
	for _, entry := range []struct{ id, product, kind string }{{"claude-code-sdk", "Claude Code", "sdk"}} {
		limits := []string{"no_execution_adapter"}
		source := "docs/integrations/development-tool-matrix.md"
		rows = append(rows, adapterDescriptor(entry.id, entry.product, entry.kind, "suma_to_tool", "planned", "manual_guide", "", "", source, nil, limits))
	}
	for i := range rows {
		refineAdapterSources(&rows[i])
	}
	return rows
}

// DescribeAdapterProfile narrows the implemented descriptor to the fixed local
// profile. It does not consult availability or transform a plan into authority.
func DescribeAdapterProfile(p Profile) (AdapterDescriptor, error) {
	if err := ValidateProfile(p, false); err != nil {
		return AdapterDescriptor{}, err
	}
	for _, d := range AdapterCatalog() {
		if d.Adapter != p.Adapter || d.Protocol != p.Protocol || d.Implementation != "implemented" || d.Direction != "suma_to_tool" {
			continue
		}
		set := func(key string, enabled bool) {
			for i := range d.Capabilities {
				if d.Capabilities[i].Key == key {
					d.Capabilities[i].Support = "unsupported"
					if enabled {
						d.Capabilities[i].Support = "supported"
					}
				}
			}
		}
		set("candidate_capture", false)
		set("workspace_read", SupportsWorkspaceObservationProfile(p))
		set("workspace_write", SupportsWorkspaceWriteProfile(p))
		if p.Adapter == "suma" {
			set("gateway_budget", p.ModelExecutionProtocol != "")
			set("model_selection", p.ModelExecutionProtocol != "")
			set("candidate_capture", false)
			if p.ReadOnly {
				set("workspace_write", false)
			}
		}
		if p.Adapter == "command" {
			set("human_input", SupportsTerminalSessionProfile(p))
			set("independent_test", p.TestReportProtocol != "")
			set("candidate_capture", false)
		}
		// Profile does not include the actual assignment workspace/artifact transport.
		if !p.ReadOnly {
			for i := range d.Capabilities {
				if d.Capabilities[i].Key == "candidate_capture" {
					d.Capabilities[i].Support = "unverified"
				}
			}
		}
		return d, nil
	}
	return AdapterDescriptor{}, fmt.Errorf("profile has no implemented adapter descriptor")
}

// References describe the Suma implementation, not the vendor's whole product.
// Conditional execution infrastructure is deliberately not inferred from a CLI
// name: candidates need an assignment and artifact channel; process containment
// needs a platform-specific controller and actual per-attempt evidence.
func refineAdapterSources(d *AdapterDescriptor) {
	if d.ID == "workbuddy-cloud" {
		d.SourceRefs = append(d.SourceRefs, "internal/runner/workbuddy_cloud_execution.go", "internal/platform/workbuddy_cloud_claim.go", "internal/platform/workbuddy_cloud_execution.go")
		for i := range d.Capabilities {
			if d.Capabilities[i].Key == "task_start" {
				d.Capabilities[i].SourceRefs = append([]string(nil), d.SourceRefs...)
			}
		}
		return
	}
	if d.Direction == "tool_to_suma" {
		for i := range d.Capabilities {
			if d.Capabilities[i].Key == "configuration_export" && d.Capabilities[i].Support == "supported" {
				d.Capabilities[i].SourceRefs = []string{"apps/web/src/developer-client-connections-model.ts#developerClientArtifact"}
			}
		}
		return
	}
	if d.Implementation != "implemented" {
		return
	}
	d.SourceRefs = append(d.SourceRefs, "internal/toolcontract/validation.go#protocolPair")
	if d.ID == "cursor-cli" {
		d.SourceRefs = append(d.SourceRefs, "contracts/cursor-inputs.md")
	}
	protocolSource := map[string]string{
		"suma-native":      "internal/runner/tool_stream.go#toolStream.suma",
		"command-cli":      "internal/runner/execute.go#Execute",
		"codex-jsonl":      "internal/runner/tool_stream.go#toolStream.codex",
		"codex-app-server": "internal/runner/codex_appserver.go",
		"opencode-sdk":     "internal/runner/tool_opencode.go",
		"octop-acp":        "internal/runner/tool_octop.go",
		"dsh-multica":      "internal/runner/tool_dsh.go#toolStream.dsh",
		"codebuddy-cli":    "internal/runner/tool_codebuddy.go#toolStream.codebuddy",
		"claude-code-cli":  "internal/runner/tool_claude.go#toolStream.claude",
		"qoder-acp":        "internal/runner/acp_execution.go",
		"cursor-cli":       "internal/runner/acp_execution.go",
	}[d.ID]
	for i := range d.Capabilities {
		c := &d.Capabilities[i]
		if c.Support == "supported" {
			c.SourceRefs = []string{protocolSource}
		}
		switch c.Key {
		case "task_start", "cancel_request":
			c.SourceRefs = []string{"internal/runner/execute.go#Execute", protocolSource}
		case "host_stop_proof":
			c.Support = "unverified"
			c.SourceRefs = []string{"internal/runner/execute.go#Execute", "internal/runner/process_windows.go#processController.Stop", "internal/runner/process_unix.go#processController.Stop"}
			c.Limitations = []string{"host_stop_platform_dependent", "host_stop_requires_evidence"}
			d.Limitations = append(d.Limitations, "host_stop_platform_dependent")
		case "candidate_capture":
			c.SourceRefs = []string{"internal/runner/execute.go#Execute", "internal/runner/candidate_transfer.go#finishCandidate"}
			c.Limitations = []string{"candidate_requires_workspace_artifacts", "independent_quality_gate"}
			d.Limitations = append(d.Limitations, "candidate_requires_workspace_artifacts")
		case "workspace_read":
			c.SourceRefs = []string{"internal/toolcontract/validation.go#SupportsWorkspaceObservationProfile"}
		case "workspace_write":
			c.SourceRefs = []string{"internal/toolcontract/validation.go#SupportsWorkspaceWriteProfile"}
		case "gateway_budget":
			if c.Support == "supported" {
				c.SourceRefs = []string{"internal/modelgateway/gateway.go", "internal/runner/execute.go#nativeExecutionLimits"}
			}
		case "human_input":
			if c.Support == "supported" {
				c.SourceRefs = []string{"internal/toolcontract/terminal.go#SupportsTerminalSessionProfile"}
				c.Limitations = []string{"profile_specific_capabilities"}
				if d.ID == "qoder-acp" || d.ID == "cursor-cli" {
					c.SourceRefs = []string{"internal/platform/tool_permission.go", "internal/runner/acp_execution.go"}
					c.Limitations = []string{"acp_human_once_permissions"}
					if d.ID == "cursor-cli" {
						c.SourceRefs = append(c.SourceRefs, "internal/platform/tool_input.go", "internal/runner/cursor_tool_inputs.go")
					}
				}
			}
		case "independent_test":
			if c.Support == "supported" {
				c.SourceRefs = []string{"internal/toolcontract/validation.go#ValidateProfile", "internal/runner/execute.go#Execute"}
				c.Limitations = []string{"profile_specific_capabilities", "independent_quality_gate"}
			}
		}
	}
}
