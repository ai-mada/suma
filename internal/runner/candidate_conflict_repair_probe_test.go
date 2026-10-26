package runner

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
)

func TestCandidateConflictRepairCapabilityRequiresActualNativeProbe(t *testing.T) {
	cfg, entry := nativeHostFixture(t)
	spec := cfg.Commands["native"]
	spec.NativeWorkspace.ConflictRepair = true
	cfg.Commands["native"] = spec
	if _, _, err := launchProfile(context.Background(), Assignment{Command: []string{"native"}}, cfg, spec); err == nil {
		t.Fatal("an old runtime advertised unsupported conflict repair")
	}
	raw, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	report := `{"protocol":"suma.candidate-conflict-repair.v1","source_limit":16,"workspace_tools":["workspace.check","workspace.edit","workspace.list","workspace.merge_continue","workspace.merge_status","workspace.read","workspace.remove","workspace.write"],"max_steps":128,"max_tokens":131072}`
	encoded, _ := json.Marshal(report)
	script := []byte("if(process.argv.includes('--conflict-repair-probe')){console.log(" + string(encoded) + ");process.exit(0)}\n" + string(raw))
	if err = os.WriteFile(entry, script, 0600); err != nil {
		t.Fatal(err)
	}
	manifestRaw, err := os.ReadFile(spec.NativeWorkspace.RuntimeManifest)
	if err != nil {
		t.Fatal(err)
	}
	var manifest nativeRuntimeManifest
	if err = json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.EntrySHA256 = digest(script)
	manifestRaw, _ = json.Marshal(manifest)
	if err = os.WriteFile(spec.NativeWorkspace.RuntimeManifest, manifestRaw, 0600); err != nil {
		t.Fatal(err)
	}
	profile, _, err := launchProfile(context.Background(), Assignment{Command: []string{"native"}}, cfg, spec)
	if err != nil || profile.ConflictRepairProtocol != ccc.CandidateConflictRepairProtocol {
		t.Fatalf("actual fixed capability probe not retained: %+v %v", profile, err)
	}
}
