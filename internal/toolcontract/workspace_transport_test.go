package toolcontract

import (
	"encoding/json"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
)

const testWorkspaceObservationProtocol = "workspace-observation-v1"
const testWorkspaceWriteProtocol = "workspace-write-v1"

// Decode through the inventory wire so this regression also catches fields
// being silently discarded before current transport capability validation.
func workspaceTransportObservation(t *testing.T, available bool, read, write string) Observation {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{
		"profile":   Profile{CommandID: "native", Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "owned-native-probe", WorkspaceToolsVersion: "attempt-workspace-v1", ModelExecutionProtocol: "suma-model-execution-v1"},
		"available": available, "reason": "local executable protocol probe; connection unverified",
		"workspace_observation_protocol": read, "workspace_write_protocol": write,
	})
	var result Observation
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestWorkspaceTransportObservationSeparatesCurrentBrokerFromApprovedProfile(t *testing.T) {
	native := workspaceTransportObservation(t, true, testWorkspaceObservationProtocol, testWorkspaceWriteProtocol)
	if err := ValidateObservation(native); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(native)
	var wire map[string]json.RawMessage
	_ = json.Unmarshal(raw, &wire)
	if string(wire["workspace_observation_protocol"]) != `"workspace-observation-v1"` || string(wire["workspace_write_protocol"]) != `"workspace-write-v1"` {
		t.Fatal("actual inventory discarded the current read/write broker protocols")
	}
	legacy := workspaceTransportObservation(t, true, "", "")
	if !EqualProfile(native.Profile, legacy.Profile) || ValidateObservation(legacy) != nil {
		t.Fatal("transport implementation changed the approved tool profile or rejected an explicitly unadvertised capability")
	}
	readonlyBroker := workspaceTransportObservation(t, true, testWorkspaceObservationProtocol, "")
	if ValidateObservation(readonlyBroker) != nil {
		t.Fatal("read-only broker inventory requires write support")
	}
}

func TestWorkspaceTransportObservationRejectsInvalidOrUnavailableDeclarations(t *testing.T) {
	for _, scenario := range []string{"unknown-read", "unknown-write", "write-without-read", "unavailable", "generic-cli", "no-native-tools", "readonly-purpose"} {
		t.Run(scenario, func(t *testing.T) {
			read, write, available := testWorkspaceObservationProtocol, testWorkspaceWriteProtocol, true
			switch scenario {
			case "unknown-read":
				read = "workspace-observation-v2"
			case "unknown-write":
				write = "workspace-write-v2"
			case "write-without-read":
				read = ""
			case "unavailable":
				available = false
			}
			observation := workspaceTransportObservation(t, available, read, write)
			switch scenario {
			case "generic-cli":
				observation.Profile.Adapter, observation.Profile.Protocol, observation.Profile.WorkspaceToolsVersion = "command", "text-v1", ""
			case "no-native-tools":
				observation.Profile.WorkspaceToolsVersion = ""
			case "readonly-purpose":
				observation.Profile.ReadOnly, observation.Profile.WorkspaceToolsVersion = true, ""
			}
			if err := ValidateObservation(observation); err == nil {
				t.Fatal("inventory advertised a transport that the current host cannot support")
			}
		})
	}
}

func TestWorkspaceTransportProfileKindsSeparateCodexReadFromNativeWrite(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		profile Profile
		read    bool
		write   bool
	}{
		{"native", Profile{Adapter: "suma", Protocol: "suma-jsonl-v1", WorkspaceToolsVersion: "attempt-workspace-v1", ModelExecutionProtocol: "suma-model-execution-v1"}, true, true},
		{"native-without-model-gateway", Profile{Adapter: "suma", Protocol: "suma-jsonl-v1", WorkspaceToolsVersion: "attempt-workspace-v1"}, false, false},
		{"codex-app-server", Profile{Adapter: "codex", Protocol: cc.Protocol}, true, false},
		{"codex-jsonl", Profile{Adapter: "codex", Protocol: "codex-jsonl-v1"}, false, false},
		{"command", Profile{Adapter: "command", Protocol: "text-v1"}, false, false},
		{"other-adapter", Profile{Adapter: "command", Protocol: cc.Protocol}, false, false},
		{"codex-native-claim", Profile{Adapter: "codex", Protocol: cc.Protocol, WorkspaceToolsVersion: "attempt-workspace-v1", ModelExecutionProtocol: "suma-model-execution-v1"}, false, false},
		{"codex-readonly-purpose", Profile{Adapter: "codex", Protocol: cc.Protocol, ReadOnly: true}, false, false},
		{"native-without-workspace", Profile{Adapter: "suma", Protocol: "suma-jsonl-v1"}, false, false},
		{"native-planning-purpose", Profile{Adapter: "suma", Protocol: "suma-jsonl-v1", ReadOnly: true, Planning: &PlanningCapability{}}, false, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if got := SupportsWorkspaceObservationProfile(scenario.profile); got != scenario.read {
				t.Fatal("workspace observation kind", got, scenario.read)
			}
			if got := SupportsWorkspaceWriteProfile(scenario.profile); got != scenario.write {
				t.Fatal("workspace write kind", got, scenario.write)
			}
		})
	}
}

func TestWorkspaceTransportCodexRequiresValidCurrentReadCatalog(t *testing.T) {
	catalog, err := cc.NewCatalog(strings.Repeat("c", 64), []cc.ModelOption{{Model: "model-a", ReasoningEfforts: []string{"low"}, DefaultReasoningEffort: "low"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"read", "no-catalog", "invalid-catalog", "unavailable", "write", "write-without-read", "native-claim", "readonly-purpose", "planning-purpose", "jsonl", "unknown-read"} {
		t.Run(scenario, func(t *testing.T) {
			observation := workspaceTransportObservation(t, true, testWorkspaceObservationProtocol, "")
			observation.Profile.Adapter, observation.Profile.Protocol, observation.Profile.WorkspaceToolsVersion = "codex", cc.Protocol, ""
			observation.Profile.ModelExecutionProtocol = ""
			observedCatalog := catalog
			observation.CodexCatalog = &observedCatalog
			switch scenario {
			case "no-catalog":
				observation.CodexCatalog = nil
			case "invalid-catalog":
				observedCatalog.CatalogSHA256 = strings.Repeat("d", 64)
			case "unavailable":
				observation.Available = false
			case "write":
				observation.WorkspaceWriteProtocol = testWorkspaceWriteProtocol
			case "write-without-read":
				observation.WorkspaceObservationProtocol, observation.WorkspaceWriteProtocol = "", testWorkspaceWriteProtocol
			case "native-claim":
				observation.Profile.WorkspaceToolsVersion = "attempt-workspace-v1"
			case "readonly-purpose":
				observation.Profile.ReadOnly = true
			case "planning-purpose":
				observation.Profile.Planning = &PlanningCapability{}
			case "jsonl":
				observation.Profile.Protocol = "codex-jsonl-v1"
			case "unknown-read":
				observation.WorkspaceObservationProtocol = "workspace-observation-v2"
			}
			if err := ValidateObservation(observation); (err == nil) != (scenario == "read") {
				t.Fatal("unexpected Codex broker declaration validation", err)
			}
		})
	}
}
