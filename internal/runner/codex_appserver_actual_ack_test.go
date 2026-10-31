package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// The actual CLI receives no task or turn. Production configuration generation,
// thread parameters and acknowledgement validation are exercised independently
// of the driver's automatic turn-start transition.
func TestCodexAppServerActualNoTurnAcknowledgement(t *testing.T) {
	if os.Getenv("SUMA_CODEX_NO_TURN_ACK") != "1" {
		t.Skip("explicit installed-Codex no-turn acknowledgement opt-in is required")
	}
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	var pin struct {
		Executable string `json:"installed_executable"`
		SHA256     string `json:"installed_executable_sha256"`
	}
	raw, err := os.ReadFile(filepath.Join(root, ".cache", "e2e", "expert-model-stage-040", "codex-appserver-research.local.json"))
	if err != nil || json.Unmarshal(raw, &pin) != nil {
		t.Fatal("locked CLI receipt unavailable")
	}
	raw, err = os.ReadFile(pin.Executable)
	if err != nil || digest(raw) != pin.SHA256 {
		t.Fatal("locked CLI identity differs")
	}
	dir := filepath.Join(root, ".cache", "e2e", "codex-execution-stage-041", "discovery-debug", "ack-production-final")
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal("fresh ACK evidence directory required")
	}
	home, workspace, tmp := filepath.Join(dir, "home"), filepath.Join(dir, "workspace"), filepath.Join(dir, "tmp")
	for _, path := range []string{home, workspace, tmp} {
		if err = safeMkdir(path); err != nil {
			t.Fatal(err)
		}
	}
	var providerRequests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerRequests.Add(1)
		http.Error(w, "no model invocation is authorized", http.StatusForbidden)
	}))
	defer provider.Close()
	spec := CommandSpec{Adapter: "codex", Protocol: cc.Protocol, Executable: pin.Executable, Args: []string{"app-server", "--listen", "stdio://"}, Input: "codex-app-server", CodexAppServer: &CodexAppServerSpec{ProviderID: "owned041", BaseURL: provider.URL + "/v1", WindowsSandbox: "unelevated"}}
	if err = validateCodexAppServerSpec(spec); err != nil {
		t.Fatal(err)
	}
	if err = prepareCodexHome(spec, home, workspace); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, pin.Executable, spec.Args...)
	cmd.Dir = home
	cmd.Env, _, err = codexEnvironment(spec, home)
	if err != nil {
		t.Fatal(err)
	}
	for i, value := range cmd.Env {
		name, _, _ := strings.Cut(value, "=")
		if strings.EqualFold(name, "TEMP") || strings.EqualFold(name, "TMP") {
			cmd.Env[i] = name + "=" + tmp
		}
	}
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := newOutput(32<<10, []string{home, workspace, tmp, provider.URL})
	cmd.Stderr = diagnostic
	ctl, err := startProcess(cmd)
	if err != nil {
		t.Fatal("owned no-turn CLI could not start")
	}
	finished := false
	defer func() {
		_ = input.Close()
		if !finished {
			_ = ctl.Stop()
			_ = cmd.Wait()
		}
		ctl.Close()
	}()
	var methods, frames []string
	send := func(id int, method string, params any) {
		t.Helper()
		methods = append(methods, method)
		request := map[string]any{"method": method, "params": params}
		if id != 0 {
			request["id"] = id
		}
		if err := json.NewEncoder(input).Encode(request); err != nil {
			t.Fatal("owned RPC write failed")
		}
	}
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), toolLineLimit)
	readReply := func(id int) map[string]any {
		t.Helper()
		for count := 0; count < 32 && scanner.Scan(); count++ {
			frames = append(frames, scanner.Text())
			m, err := decodeToolObject(scanner.Bytes())
			if err != nil {
				t.Fatal("owned ACK frame malformed")
			}
			if method := textAt(m, "method"); method != "" {
				if _, request := m["id"]; request {
					t.Fatal("server requested an unauthorized capability")
				}
				continue
			}
			if number, ok := m["id"].(json.Number); ok && number.String() == fmt.Sprint(id) {
				if _, rejected := m["error"]; rejected {
					t.Fatal("owned no-turn RPC rejected")
				}
				return objectAt(m, "result")
			}
		}
		t.Fatal("owned ACK reply missing or bounded process expired")
		return nil
	}
	send(1, "initialize", map[string]any{"clientInfo": map[string]any{"name": "suma", "title": "suma Runner", "version": "1.0.0"}, "capabilities": map[string]any{"experimentalApi": false, "requestAttestation": false, "explicitGatewayOauth": true}})
	if !sameCodexPath(textAt(readReply(1), "codexHome"), home) {
		t.Fatal("isolated home was not acknowledged")
	}
	send(0, "initialized", map[string]any{})
	send(2, "model/list", map[string]any{"limit": 32, "includeHidden": false})
	driver := newCodexAppServer(tc.Profile{}, spec, home, workspace, "", nil, nil)
	defer driver.Close()
	driver.modelList(readReply(2))
	if driver.bad != nil || driver.catalog == nil {
		t.Fatal("owned public model catalog invalid")
	}
	binding, err := cc.NewBinding(*driver.catalog, cc.Selection{CatalogSHA256: driver.catalog.CatalogSHA256, Model: "gpt-5.5", ReasoningEffort: "medium"})
	if err != nil {
		t.Fatal("same failed-case model/effort unavailable")
	}
	driver.binding = &binding
	params := driver.threadStartParams()
	send(3, "thread/start", params)
	ack := readReply(3)
	mismatch := driver.configurationMismatch(ack, "sandbox", "reasoningEffort")
	_ = input.Close()
	for scanner.Scan() {
		frames = append(frames, scanner.Text())
	}
	processErr := cmd.Wait()
	finishErr := ctl.Finish()
	finished = true
	safe := func(text string) string {
		for _, path := range []struct{ value, label string }{{home, "[owned-home]"}, {workspace, "[owned-workspace]"}, {tmp, "[owned-tmp]"}, {provider.URL, "[owned-provider]"}, {root, "[workspace]"}} {
			text = strings.ReplaceAll(text, strings.ReplaceAll(path.value, `\`, `\\`), path.label)
			text = strings.ReplaceAll(text, path.value, path.label)
		}
		return text
	}
	expected := map[string]any{"model": binding.Model, "reasoningEffort": binding.ReasoningEffort, "modelProvider": spec.CodexAppServer.ProviderID, "cwd": workspace, "approvalPolicy": cc.ApprovalPolicy, "effectiveWritableRoots": []string{workspace}, "networkAccess": false, "excludeTmpdirEnvVar": true, "excludeSlashTmp": true, "windowsSandbox": spec.CodexAppServer.WindowsSandbox}
	receipt := map[string]any{"locked_executable_sha256": pin.SHA256, "configuration_sha256": binding.ConfigurationSHA256, "methods": methods, "request": params, "expected": expected, "ack": ack, "mismatch_field": mismatch, "provider_requests": providerRequests.Load(), "turn_requests": 0, "current_login_configuration_used": false, "process_exit_confirmed": processErr == nil && finishErr == nil}
	raw, _ = json.MarshalIndent(receipt, "", "  ")
	for name, data := range map[string]string{"public-expected-vs-ack.local.json": string(raw), "stdout.txt": strings.Join(frames, "\n"), "stderr.txt": diagnostic.String()} {
		if writeEvidence(filepath.Join(dir, name), []byte(safe(data))) != nil {
			t.Fatal("owned no-turn evidence could not be retained")
		}
	}
	if mismatch != "" || processErr != nil || finishErr != nil || scanner.Err() != nil || diagnostic.truncated || providerRequests.Load() != 0 || len(methods) != 4 {
		t.Fatalf("no-turn exact acknowledgement failed: field=%q processOK=%t", mismatch, processErr == nil && finishErr == nil)
	}
	t.Log("production generated independent home and exact effective scope acknowledged; provider/turn=0; process closed")
}
