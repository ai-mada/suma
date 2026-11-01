package toolcontract

import (
	ac "github.com/ai-mada/suma/internal/acpcontract"
	"strings"
	"testing"
)

func catalogEntry(t *testing.T, id string) AdapterDescriptor {
	t.Helper()
	for _, d := range AdapterCatalog() {
		if d.ID == id {
			return d
		}
	}
	t.Fatal("missing adapter", id)
	return AdapterDescriptor{}
}
func catalogCapability(t *testing.T, d AdapterDescriptor, key string) string {
	t.Helper()
	for _, c := range d.Capabilities {
		if c.Key == key {
			return c.Support
		}
	}
	t.Fatal("missing capability", key)
	return ""
}
func TestAdapterCatalogDistinctDirectionsAndProtectedCloudStart(t *testing.T) {
	cloud := catalogEntry(t, "workbuddy-cloud")
	cli := catalogEntry(t, "codebuddy-cli")
	if cloud.Product == cli.Product || cloud.Adapter != "" || cloud.Protocol != "" || cloud.Implementation != "implemented" || cloud.SetupKind != "manual_guide" || cli.Implementation != "implemented" {
		t.Fatal(cloud, cli)
	}
	for _, c := range cloud.Capabilities {
		want := "unsupported"
		if c.Key == "task_start" {
			want = "supported"
		}
		if c.Support != want || len(c.SourceRefs) == 0 {
			t.Fatal("cloud advertised more than protected task start", c)
		}
	}
	for _, limit := range cloud.Limitations {
		if limit == "no_execution_adapter" || strings.HasPrefix(limit, "host_stop_") || limit == "candidate_requires_workspace_artifacts" {
			t.Fatal("cloud inherited local CLI admission", limit)
		}
	}
	if !strings.Contains(strings.Join(cloud.SourceRefs, " "), "contracts/workbuddy-cloud-runner.md") {
		t.Fatal("missing current protected cloud contract")
	}
	inbound := catalogEntry(t, "codex-client")
	outbound := catalogEntry(t, "codex-app-server")
	if inbound.Direction != "tool_to_suma" || outbound.Direction != "suma_to_tool" || inbound.Protocol != "" {
		t.Fatal(inbound, outbound)
	}
	if catalogCapability(t, inbound, "task_start") != "unsupported" || catalogCapability(t, inbound, "configuration_export") != "supported" || catalogCapability(t, inbound, "project_access") != "unverified" {
		t.Fatal("config export confused with granted access")
	}
}
func TestAdapterCatalogClosedUniqueAndFresh(t *testing.T) {
	seen := map[string]bool{}
	rows := AdapterCatalog()
	if len(rows) > 64 {
		t.Fatal("unbounded catalog")
	}
	for _, d := range rows {
		if d.ID == "" || seen[d.ID] || len(d.SourceRefs) == 0 {
			t.Fatal("invalid identity", d)
		}
		seen[d.ID] = true
		if d.Adapter != "" && !ValidCommandIdentity("catalog", d.Adapter, d.Protocol) {
			t.Fatal("invented executable protocol", d)
		}
		keys := map[string]bool{}
		for _, c := range d.Capabilities {
			if keys[c.Key] || len(c.SourceRefs) == 0 || c.Support != "supported" && c.Support != "unsupported" && c.Support != "unverified" {
				t.Fatal(c)
			}
			keys[c.Key] = true
		}
		if len(keys) != len(adapterCapabilityKeys) {
			t.Fatal("missing unknown states", d.ID)
		}
	}
	rows[0].Capabilities[0].Support = "hacked"
	rows[0].SourceRefs[0] = "hacked"
	if AdapterCatalog()[0].Capabilities[0].Support == "hacked" || AdapterCatalog()[0].SourceRefs[0] == "hacked" {
		t.Fatal("mutable global catalog")
	}
}
func TestAdapterCatalogDoesNotInventResumeBillingOrInteractiveCLI(t *testing.T) {
	for _, id := range []string{"dsh-multica", "codebuddy-cli", "codex-jsonl", "codex-app-server", "opencode-sdk", "octop-acp"} {
		d := catalogEntry(t, id)
		for _, key := range []string{"session_load", "interrupted_resume", "provider_billing", "independent_test", "human_input"} {
			if catalogCapability(t, d, key) != "unsupported" {
				t.Fatalf("%s invents %s", id, key)
			}
		}
	}
	for _, id := range []string{"codex-app-server", "opencode-sdk"} {
		if catalogCapability(t, catalogEntry(t, id), "gateway_budget") != "unsupported" {
			t.Fatal("observe-only promoted to hard gateway", id)
		}
	}
}

func TestCursorCatalogPublishesACPWithoutPromotingPassiveDetection(t *testing.T) {
	d := catalogEntry(t, "cursor-cli")
	if d.Implementation != "implemented" || d.SetupKind != "runner_profile" || d.Adapter != "cursor" || d.Protocol != ac.Protocol || len(d.SourceRefs) != 2 || d.SourceRefs[0] != "contracts/cursor-acp-execution.md" || d.SourceRefs[1] != "internal/toolcontract/validation.go#protocolPair" {
		t.Fatal("Cursor source support did not use the reviewed ACP contract", d)
	}
	if catalogCapability(t, d, "gateway_budget") != "unsupported" {
		t.Fatal("a configured CLI acquired a hard gateway budget")
	}
	if _, err := DescribeAdapterProfile(Profile{CommandID: "cursor", Adapter: "cursor", Protocol: "cursor-jsonl-v1"}); err == nil {
		t.Fatal("unreviewed Cursor profile became executable")
	}
}
func TestAdapterProfileProjectionRejectsUnknownAndNarrowsCapabilities(t *testing.T) {
	_, e := DescribeAdapterProfile(Profile{CommandID: "x", Adapter: "workbuddy", Protocol: "codebuddy-jsonl-v1"})
	if e == nil {
		t.Fatal("WorkBuddy impersonated CodeBuddy")
	}
	d, e := DescribeAdapterProfile(Profile{CommandID: "x", Adapter: "suma", Protocol: "suma-jsonl-v1", ReadOnly: true})
	if e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"workspace_read", "workspace_write", "gateway_budget", "model_selection", "candidate_capture"} {
		if catalogCapability(t, d, key) != "unsupported" {
			t.Fatal("ordinary profile acquired capability", key)
		}
	}
	d, e = DescribeAdapterProfile(Profile{CommandID: "x", Adapter: "command", Protocol: "text-v1", TerminalSessionProtocol: TerminalSessionProtocol})
	if e != nil || catalogCapability(t, d, "human_input") != "supported" || catalogCapability(t, d, "independent_test") != "unsupported" {
		t.Fatal(d, e)
	}
	bad := Profile{CommandID: "x", Adapter: "codebuddy", Protocol: "dsh-multica-jsonl-v1", ProfileSHA256: strings.Repeat("a", 64)}
	if _, e = DescribeAdapterProfile(bad); e == nil {
		t.Fatal("wrong adapter protocol accepted")
	}
}
func TestAdapterCatalogConditionalHostEvidenceIsNotProfileAuthority(t *testing.T) {
	for _, id := range []string{"command-cli", "codex-jsonl", "codebuddy-cli", "dsh-multica"} {
		d := catalogEntry(t, id)
		if catalogCapability(t, d, "host_stop_proof") != "unverified" {
			t.Fatal("platform-less descriptor asserted host stop", id)
		}
		found := false
		for _, c := range d.Capabilities {
			if c.Key == "candidate_capture" {
				for _, limit := range c.Limitations {
					if limit == "candidate_requires_workspace_artifacts" {
						found = true
					}
				}
			}
		}
		if !found {
			t.Fatal("candidate infrastructure conditions omitted", id)
		}
		p := Profile{CommandID: "configured", Adapter: d.Adapter, Protocol: d.Protocol}
		projected, e := DescribeAdapterProfile(p)
		if e != nil {
			t.Fatal(e)
		}
		if catalogCapability(t, projected, "candidate_capture") != "unverified" {
			t.Fatal("writable profile alone asserted candidate eligibility", id)
		}
		if catalogCapability(t, projected, "host_stop_proof") != "unverified" {
			t.Fatal("profile erased OS evidence requirement", id)
		}
	}
}
func TestAdapterCatalogRuntimeGuideDoesNotClaimClientSetup(t *testing.T) {
	d := catalogEntry(t, "harness-guide")
	found := false
	for _, limit := range d.Limitations {
		if limit == "manual_client_verification" {
			t.Fatal("runtime guide reuses reverse-direction client instructions")
		}
		if limit == "manual_runtime_verification" {
			found = true
		}
	}
	if !found || d.Direction != "suma_to_tool" || d.EntrypointKind != "sdk" {
		t.Fatal(d)
	}
	client := catalogEntry(t, "workbuddy-connector")
	for _, limit := range client.Limitations {
		if limit == "manual_runtime_verification" {
			t.Fatal("connector confused with runtime")
		}
	}
}
