package runner

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestCodexAppServerStopBeforeStartAcknowledgementSubmitsNothingNew(t *testing.T) {
	for _, phase := range []string{"initialize", "model/list", "thread/start", "session-progress-failure"} {
		t.Run(phase, func(t *testing.T) {
			root, home := t.TempDir(), t.TempDir()
			spec := CommandSpec{CodexAppServer: &CodexAppServerSpec{WindowsSandbox: "unelevated", ProviderID: "owned041", BaseURL: "http://127.0.0.1:49141/v1"}}
			catalog, err := cc.NewCatalog(CodexConfigurationDigest(*spec.CodexAppServer), []cc.ModelOption{{Model: "owned-model-041", ReasoningEfforts: []string{"high"}, DefaultReasoningEffort: "high"}})
			if err != nil {
				t.Fatal(err)
			}
			binding, err := cc.NewBinding(catalog, cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "owned-model-041", ReasoningEffort: "high"})
			if err != nil {
				t.Fatal(err)
			}
			callbackFailed := false
			profile := tc.Profile{CommandID: "stop-041", Adapter: "codex", Protocol: cc.Protocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "fixture"}
			stream := newToolStream(profile, nil, func(event tc.Event) error {
				if phase == "session-progress-failure" && event.Kind == "session_started" {
					callbackFailed = true
					return fmt.Errorf("owned durable progress failure")
				}
				return nil
			})
			d := newCodexAppServer(tc.Profile{}, spec, home, root, "must not submit after stop", &binding, stream)
			defer d.Close()
			drain := func() []string {
				d.input.mu.Lock()
				defer d.input.mu.Unlock()
				var methods []string
				for _, line := range strings.Split(strings.TrimSpace(string(d.input.pending)), "\n") {
					if line == "" {
						continue
					}
					var request map[string]any
					if json.Unmarshal([]byte(line), &request) != nil {
						t.Fatal("host RPC malformed")
					}
					methods = append(methods, textAt(request, "method"))
				}
				d.input.pending = nil
				return methods
			}
			reply := func(id int, result any) {
				raw, _ := json.Marshal(map[string]any{"id": id, "result": result})
				_, _ = d.Write(append(raw, '\n'))
			}
			stop := func() {
				if err := d.Cancel(); err != nil {
					t.Fatal(err)
				}
			}
			if got := strings.Join(drain(), ","); got != "initialize" {
				t.Fatalf("initial request: %s", got)
			}
			if phase == "initialize" {
				stop()
			}
			reply(1, map[string]any{"codexHome": home, "userAgent": "owned-041"})
			if phase != "initialize" {
				if got := strings.Join(drain(), ","); got != "initialized,model/list" {
					t.Fatalf("catalog request: %s", got)
				}
				if phase == "model/list" {
					stop()
				}
				reply(2, map[string]any{"data": []any{map[string]any{"model": "owned-model-041", "defaultReasoningEffort": "high", "supportedReasoningEfforts": []any{map[string]any{"reasoningEffort": "high"}}}}, "nextCursor": nil})
				if phase != "model/list" {
					if got := strings.Join(drain(), ","); got != "thread/start" {
						t.Fatalf("thread request: %s", got)
					}
					if phase == "thread/start" {
						stop()
					}
					reply(3, map[string]any{"thread": map[string]any{"id": "stopped-thread-041"}, "model": binding.Model, "reasoningEffort": binding.ReasoningEffort, "modelProvider": "owned041", "cwd": root, "approvalPolicy": "never", "sandbox": map[string]any{"type": "workspaceWrite", "writableRoots": []any{root}, "networkAccess": false, "excludeTmpdirEnvVar": true, "excludeSlashTmp": true}})
				}
			}
			if got := drain(); len(got) != 0 {
				t.Fatalf("stopped attempt submitted new RPCs: %v", got)
			}
			if !d.input.closed || d.turn != "" {
				t.Fatal("stop left request input open or acquired a new turn")
			}
			if phase == "session-progress-failure" && (!callbackFailed || stream.bad == nil || !strings.Contains(stream.bad.Error(), "durable tool progress unavailable")) {
				t.Fatal("durable progress failure was discarded")
			}
		})
	}
}

func TestCodexAppServerThreadAcknowledgementExactEffectiveScope(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(root, "unapproved")
	tests := []struct {
		name     string
		change   func(map[string]any)
		mismatch string
	}{
		{"explicit_exact_root", func(m map[string]any) {}, ""},
		{"runtime_exact_root", func(m map[string]any) {
			m["runtimeWorkspaceRoots"] = []any{root}
			objectAt(m, "sandbox")["writableRoots"] = []any{}
		}, ""},
		{"missing_effective_root", func(m map[string]any) { objectAt(m, "sandbox")["writableRoots"] = []any{} }, "sandbox.writableRoots"},
		{"runtime_extra_root", func(m map[string]any) { m["runtimeWorkspaceRoots"] = []any{root, other} }, "runtimeWorkspaceRoots"},
		{"runtime_wrong_root", func(m map[string]any) { m["runtimeWorkspaceRoots"] = []any{other} }, "runtimeWorkspaceRoots"},
		{"runtime_null", func(m map[string]any) { m["runtimeWorkspaceRoots"] = nil }, "runtimeWorkspaceRoots"},
		{"additional_wrong_root", func(m map[string]any) {
			m["runtimeWorkspaceRoots"] = []any{root}
			objectAt(m, "sandbox")["writableRoots"] = []any{other}
		}, "sandbox.writableRoots"},
		{"readonly", func(m map[string]any) { objectAt(m, "sandbox")["type"] = "readOnly" }, "sandbox.type"},
		{"network", func(m map[string]any) { objectAt(m, "sandbox")["networkAccess"] = true }, "sandbox.networkAccess"},
		{"temporary_root", func(m map[string]any) { objectAt(m, "sandbox")["excludeTmpdirEnvVar"] = false }, "sandbox.excludeTmpdirEnvVar"},
		{"slash_temporary_root", func(m map[string]any) { objectAt(m, "sandbox")["excludeSlashTmp"] = false }, "sandbox.excludeSlashTmp"},
		{"cwd", func(m map[string]any) { m["cwd"] = other }, "cwd"},
		{"approval", func(m map[string]any) { m["approvalPolicy"] = "on-request" }, "approvalPolicy"},
		{"model", func(m map[string]any) { m["model"] = "observed-other-model" }, "model"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := tc.Profile{CommandID: "ack-test", Adapter: "codex", Protocol: cc.Protocol}
			stream := newToolStream(profile, nil, nil)
			d := newCodexAppServer(profile, CommandSpec{CodexAppServer: &CodexAppServerSpec{WindowsSandbox: "unelevated", ProviderID: "owned041"}}, t.TempDir(), root, "fixture only", &cc.Binding{Model: "owned-model-041", ReasoningEffort: "high"}, stream)
			m := map[string]any{"thread": map[string]any{"id": "ack-thread"}, "model": "owned-model-041", "modelProvider": "owned041", "reasoningEffort": "high", "cwd": root, "approvalPolicy": "never", "sandbox": map[string]any{"type": "workspaceWrite", "writableRoots": []any{root}, "networkAccess": false, "excludeTmpdirEnvVar": true, "excludeSlashTmp": true}}
			test.change(m)
			d.threadStart(m)
			if test.mismatch == "" {
				if d.bad != nil || !d.evidence.ConfigurationAcknowledged {
					t.Fatalf("exact effective scope rejected: %v", d.bad)
				}
			} else {
				if d.bad == nil || !strings.Contains(d.bad.Error(), test.mismatch) || d.evidence.ConfigurationAcknowledged || strings.Contains(string(d.input.pending), `"method":"turn/start"`) {
					t.Fatalf("unsafe scope crossed ACK boundary: %v %+v", d.bad, d.evidence)
				}
				if d.evidence.AcknowledgedModel != textAt(m, "model") || d.evidence.AcknowledgedProvider != "owned041" {
					t.Fatal("safe actual acknowledgement fields were discarded")
				}
			}
		})
	}
}

func TestCodexAppServerConfigurationPinsRestrictedWindowsBackend(t *testing.T) {
	provider := CodexAppServerSpec{WindowsSandbox: "unelevated", ProviderID: "owned041", BaseURL: "http://127.0.0.1:49141/v1"}
	config := codexConfiguration(provider, t.TempDir())
	if runtime.GOOS == "windows" && !strings.Contains(config, "[windows]\nsandbox = \"unelevated\"") {
		t.Fatal("Windows isolated home does not select the restricted backend")
	}
	for _, setting := range []string{"network_access = false", "exclude_tmpdir_env_var = true", "exclude_slash_tmp = true", "trust_level = \"untrusted\"", "approval_policy = \"never\""} {
		if !strings.Contains(config, setting) {
			t.Fatalf("required restricted setting absent: %s", setting)
		}
	}
	spec := CommandSpec{Adapter: "codex", Protocol: cc.Protocol, Args: []string{"app-server", "--listen", "stdio://"}, Input: "codex-app-server", CodexAppServer: &provider}
	seen := map[string]bool{}
	for _, mode := range []string{"unelevated", "elevated", "mxc"} {
		provider.WindowsSandbox = mode
		if err := validateCodexAppServerSpec(spec); err != nil {
			t.Fatalf("declared Windows mode rejected: %v", err)
		}
		digest := CodexConfigurationDigest(provider)
		if seen[digest] {
			t.Fatal("Windows backend not bound into configuration identity")
		}
		seen[digest] = true
	}
	provider.WindowsSandbox = "disabled"
	if validateCodexAppServerSpec(spec) == nil {
		t.Fatal("unsupported Windows backend accepted")
	}
	provider.WindowsSandbox = ""
	if runtime.GOOS == "windows" && validateCodexAppServerSpec(spec) == nil {
		t.Fatal("Windows backend was selected implicitly")
	}
}
