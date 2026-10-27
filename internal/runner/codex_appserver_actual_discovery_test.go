package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
)

// This opt-in test is deliberately discovery-only. It never supplies a binding,
// task text, thread/start, or turn/start, and rejects every provider request.
func TestCodexAppServerActualReadOnlyDiscovery(t *testing.T) {
	if os.Getenv("SUMA_CODEX_READONLY_DISCOVERY") != "1" {
		t.Skip("explicit installed-Codex read-only discovery opt-in is required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var pin struct {
		Executable string `json:"installed_executable"`
		SHA256     string `json:"installed_executable_sha256"`
	}
	raw, err := os.ReadFile(filepath.Join(root, ".cache", "e2e", "expert-model-stage-040", "codex-appserver-research.local.json"))
	if err != nil || json.Unmarshal(raw, &pin) != nil || pin.Executable == "" || pin.SHA256 == "" {
		t.Fatal("locked CLI research receipt unavailable")
	}
	executable, err := os.ReadFile(pin.Executable)
	if err != nil || digest(executable) != pin.SHA256 {
		t.Fatal("locked installed CLI identity differs")
	}
	dir := filepath.Join(root, ".cache", "e2e", "codex-execution-stage-041", "discovery-debug")
	if err = safeMkdir(dir); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "discovery cannot invoke a model", http.StatusForbidden)
	}))
	defer provider.Close()
	repo, base := gitFixture(t)
	spec := CommandSpec{Adapter: "codex", Protocol: cc.Protocol, Executable: pin.Executable, Args: []string{"app-server", "--listen", "stdio://"}, Input: "codex-app-server", CodexAppServer: &CodexAppServerSpec{WindowsSandbox: "unelevated", ProviderID: "owned041", BaseURL: provider.URL + "/v1"}}
	inventory, probeErr := DescribeTools(context.Background(), ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]CommandSpec{"codex-actual-discovery-041": spec}})
	receipt := map[string]any{"inventory": inventory, "locked_executable_sha256": pin.SHA256, "provider_requests": requests.Load(), "thread_requests": 0, "turn_requests": 0, "current_login_read": false}
	if probeErr != nil {
		receipt["error"] = probeErr.Error()
	}
	raw, err = json.MarshalIndent(receipt, "", "  ")
	if err != nil || writeEvidence(filepath.Join(dir, "inventory-receipt.local.json"), raw) != nil {
		t.Fatal("read-only discovery receipt could not be retained")
	}
	if probeErr != nil || len(inventory) != 1 || !inventory[0].Available || inventory[0].CodexCatalog == nil {
		t.Fatalf("read-only installed discovery rejected: error=%v observations=%+v", probeErr, inventory)
	}
	if requests.Load() != 0 {
		t.Fatal("read-only discovery invoked the provider")
	}
	t.Logf("locked installed CLI read-only discovery passed: models=%d; provider/thread/turn requests=0", len(inventory[0].CodexCatalog.Models))
}
