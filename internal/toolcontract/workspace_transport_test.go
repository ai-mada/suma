package toolcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

const testWorkspaceObservationProtocol = "workspace-observation-v1"
const testWorkspaceWriteProtocol = "workspace-write-v1"

// Decode through the inventory wire so this regression also catches fields
// being silently discarded before current transport capability validation.
func workspaceTransportObservation(t *testing.T, available bool, read, write string) Observation {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{
		"profile":   Profile{CommandID: "native", Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "owned-native-probe", WorkspaceToolsVersion: "attempt-workspace-v1"},
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
