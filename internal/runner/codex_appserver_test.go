package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// The test executable implements only our selected public RPC shapes. This is
// a real child process for transport/lifecycle evidence, not a Codex invocation.
func codexAppServerFixtureProcess() bool {
	if os.Getenv("SUMA_CODEX_RPC_FIXTURE") != "1" {
		return false
	}
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		if os.Getenv("SUMA_CODEX_RPC_MODE") == "version-warning" {
			fmt.Fprintln(os.Stderr, "WARNING: "+strings.Repeat("owned startup diagnostic ", 16))
			time.Sleep(20 * time.Millisecond)
		}
		fmt.Println("codex-cli fixture-app-server-only")
		os.Exit(0)
	}
	if len(os.Args) == 3 && os.Args[1] == "app-server" && os.Args[2] == "--help" {
		fmt.Println("app-server --listen stdio://")
		os.Exit(0)
	}
	if len(os.Args) != 4 || strings.Join(os.Args[1:], " ") != "app-server --listen stdio://" {
		os.Exit(71)
	}
	mode := os.Getenv("SUMA_CODEX_RPC_MODE")
	emit := func(v any) { raw, _ := json.Marshal(v); fmt.Println(string(raw)) }
	notify := func(method string, params any) { emit(map[string]any{"method": method, "params": params}) }
	turn := map[string]any{"id": "fixture-turn-041", "status": "inProgress"}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		m, err := decodeToolObject(scanner.Bytes())
		if err != nil {
			os.Exit(72)
		}
		id, method := m["id"], textAt(m, "method")
		p := objectAt(m, "params")
		reply := func(result any) { emit(map[string]any{"id": id, "result": result}) }
		switch method {
		case "initialize":
			if os.Getenv("CODEX_HOME") == "" || os.Getenv("CODEX_HOME") == os.Getenv("SUMA_FOREIGN_HOME") || os.Getenv("SUMA_RUNNER_TOKEN") != "" || os.Getenv("SUMA_AGENT_TOKEN") != "" || os.Getenv("CODEX_API_KEY") != "" {
				os.Exit(73)
			}
			reply(map[string]any{"userAgent": "codex-fixture-041", "codexHome": os.Getenv("CODEX_HOME"), "platformFamily": "fixture", "platformOs": "fixture"})
		case "initialized":
		case "model/list":
			reply(map[string]any{"data": []any{map[string]any{"id": "fixture-catalog-041", "model": "owned-model-041", "supportedReasoningEfforts": []any{map[string]any{"reasoningEffort": "low"}, map[string]any{"reasoningEffort": "high"}}, "defaultReasoningEffort": "low"}}, "nextCursor": nil})
		case "thread/start":
			if textAt(p, "model") != "owned-model-041" || textAt(p, "modelProvider") != "owned041" || textAt(objectAt(p, "config"), "model_reasoning_effort") != "high" || textAt(p, "approvalPolicy") != cc.ApprovalPolicy {
				os.Exit(74)
			}
			model := "owned-model-041"
			if mode == "wrong-ack" {
				model = "another-model"
			}
			reply(map[string]any{"thread": map[string]any{"id": "fixture-thread-041"}, "model": model, "modelProvider": "owned041", "reasoningEffort": "high", "cwd": p["cwd"], "approvalPolicy": cc.ApprovalPolicy, "sandbox": map[string]any{"type": "workspaceWrite", "writableRoots": []any{p["cwd"]}, "networkAccess": false, "excludeTmpdirEnvVar": true, "excludeSlashTmp": true}})
		case "turn/start":
			if textAt(p, "threadId") != "fixture-thread-041" || textAt(p, "model") != "owned-model-041" || textAt(p, "effort") != "high" {
				os.Exit(75)
			}
			if mode == "lost-start" {
				os.Exit(0)
			}
			notify("turn/started", map[string]any{"threadId": "fixture-thread-041", "turn": turn})
			reply(map[string]any{"turn": turn})
			if mode == "blocked" {
				continue
			}
			if mode == "approval" {
				emit(map[string]any{"id": "server-approval", "method": "item/commandExecution/requestApproval", "params": map[string]any{"threadId": "fixture-thread-041", "turnId": "fixture-turn-041"}})
				continue
			}
			usage := map[string]any{"totalTokens": 40, "inputTokens": 23, "cachedInputTokens": 0, "cacheWriteInputTokens": 0, "outputTokens": 17, "reasoningOutputTokens": 2}
			for i := 0; i < 2; i++ {
				notify("thread/tokenUsage/updated", map[string]any{"threadId": "fixture-thread-041", "turnId": "fixture-turn-041", "tokenUsage": map[string]any{"total": usage, "last": usage, "modelContextWindow": 8192}})
			}
			notify("item/completed", map[string]any{"threadId": "fixture-thread-041", "turnId": "fixture-turn-041", "item": map[string]any{"type": "agentMessage", "id": "message-041", "text": "owned RPC fixture completed"}})
			turn["status"] = "completed"
			notify("turn/completed", map[string]any{"threadId": "fixture-thread-041", "turn": turn})
		case "turn/interrupt":
			if textAt(p, "threadId") != "fixture-thread-041" || textAt(p, "turnId") != "fixture-turn-041" {
				os.Exit(76)
			}
			reply(map[string]any{})
			turn["status"] = "interrupted"
			notify("turn/completed", map[string]any{"threadId": "fixture-thread-041", "turn": turn})
		}
	}
	os.Exit(0)
	return true
}

func TestCodexAppServerVersionIgnoresStderrWarning(t *testing.T) {
	exe, _ := os.Executable()
	spec := CommandSpec{Adapter: "codex", Protocol: cc.Protocol, Executable: exe, Args: []string{"app-server", "--listen", "stdio://"}, Input: "codex-app-server", CodexAppServer: &CodexAppServerSpec{WindowsSandbox: "unelevated", ProviderID: "owned041", BaseURL: "http://127.0.0.1:49141/v1"}, Env: map[string]string{"SUMA_CODEX_RPC_FIXTURE": "1", "SUMA_CODEX_RPC_MODE": "version-warning"}}
	version, err := probeCodexVersion(context.Background(), exe, spec, nil, t.TempDir())
	if err != nil || version != "codex-cli fixture-app-server-only" {
		t.Fatalf("stderr warning corrupted authoritative version: %q %v", version, err)
	}
}

func codexRPCFixture(t *testing.T, mode string) (ExecConfig, Assignment) {
	t.Helper()
	repo, base := gitFixture(t)
	exe, _ := os.Executable()
	spec := CommandSpec{Adapter: "codex", Protocol: cc.Protocol, Executable: exe, Args: []string{"app-server", "--listen", "stdio://"}, Input: "codex-app-server", CodexAppServer: &CodexAppServerSpec{WindowsSandbox: "unelevated", ProviderID: "owned041", BaseURL: "http://127.0.0.1:49141/v1"}, Env: map[string]string{"SUMA_CODEX_RPC_FIXTURE": "1", "SUMA_CODEX_RPC_MODE": mode}}
	cfg := ExecConfig{AllowExec: true, Repository: repo, BaseCommit: base, Workspace: repo, Commands: map[string]CommandSpec{"codex041": spec}}
	cfg.ToolInventory = describeForTest(t, cfg)
	if !cfg.ToolInventory[0].Available || cfg.ToolInventory[0].CodexCatalog == nil {
		t.Fatalf("app-server fixture unavailable: %+v", cfg.ToolInventory)
	}
	limit := int64(30)
	catalog := *cfg.ToolInventory[0].CodexCatalog
	binding, err := cc.NewBinding(catalog, cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "owned-model-041", ReasoningEffort: "high", TokenNoticeLimit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	a := Assignment{RunID: "run-041", NodeID: "agent", Attempt: 1, Fence: 1, WorkerID: "worker-041", Type: "agent", Command: []string{"codex041"}, Config: map[string]any{"goal": "owned RPC fixture", "tool_profile_id": "profile-041", "tool_profile_sha256": cfg.ToolInventory[0].Profile.ProfileSHA256, "codex_execution": binding}}
	return cfg, a
}

func TestCodexAppServerSelectedConfigurationAndSnapshotProcess(t *testing.T) {
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "foreign-login"))
	t.Setenv("SUMA_RUNNER_TOKEN", "must-not-forward")
	t.Setenv("CODEX_API_KEY", "must-not-forward-either")
	cfg, a := codexRPCFixture(t, "completed")
	var events []tc.Event
	cfg.ToolProgress = func(e tc.Event) error { events = append(events, e); return nil }
	r := Execute(context.Background(), a, cfg)
	if r.ExitCode != 0 || r.Error != "" || r.OutcomeUnknown || r.Stopped || r.ToolEvidence == nil || r.ToolEvidence.CodexExecution == nil {
		t.Fatalf("actual protocol child: %+v", r)
	}
	e := r.ToolEvidence
	if e.Terminal != "completed" || e.SessionID != "fixture-thread-041" || e.UsageState != tc.UsagePartial || e.Usage == nil || *e.Usage.InputTokens != 23 || *e.Usage.OutputTokens != 17 || !e.CodexExecution.TokenNoticeExceeded || e.CodexExecution.TurnID != "fixture-turn-041" {
		t.Fatalf("snapshot/config evidence: %+v", e)
	}
	notices := 0
	for _, event := range events {
		if strings.Contains(event.Message, "notice") {
			notices++
		}
	}
	if notices != 1 {
		t.Fatalf("notice was not emitted exactly once: %d", notices)
	}
	if got := strings.TrimSpace(gitTest(t, cfg.Repository, "rev-parse", "HEAD")); got != cfg.BaseCommit {
		t.Fatal("source HEAD changed")
	}
	if tc.ValidateEvidence(*e) != nil {
		t.Fatalf("invalid public evidence: %+v", e)
	}
}

func TestCodexAppServerUnknownAndDeniedRequestsNeverRetry(t *testing.T) {
	for _, mode := range []string{"wrong-ack", "lost-start", "approval"} {
		t.Run(mode, func(t *testing.T) {
			cfg, a := codexRPCFixture(t, mode)
			r := Execute(context.Background(), a, cfg)
			if r.ExitCode == 0 || r.ToolEvidence == nil || r.ToolEvidence.Terminal == "completed" {
				t.Fatalf("invented success: %+v", r)
			}
			if r.ToolEvidence.CodexExecution == nil || r.ToolEvidence.CodexExecution.InterruptAcknowledged {
				t.Fatalf("invented permission/cancel acknowledgement: %+v", r)
			}
		})
	}
}

func TestCodexAppServerCancellationUsesExactTurnAndConfirmsProcess(t *testing.T) {
	cfg, a := codexRPCFixture(t, "blocked")
	ctx, cancel := context.WithCancel(context.Background())
	cfg.ToolProgress = func(e tc.Event) error {
		if e.Kind == "turn_started" {
			cancel()
		}
		return nil
	}
	started := time.Now()
	r := Execute(ctx, a, cfg)
	if time.Since(started) > 8*time.Second || !r.Stopped || r.OutcomeUnknown || r.ToolEvidence == nil || r.ToolEvidence.Terminal != "unknown" || r.ToolEvidence.CodexExecution == nil || !r.ToolEvidence.CodexExecution.InterruptRequested || !r.ToolEvidence.CodexExecution.InterruptAcknowledged {
		t.Fatalf("exact protocol cancel/process result: %+v", r)
	}
}

func TestCodexAppServerLocalPolicyAndDurableReceiptIsolation(t *testing.T) {
	spec := CommandSpec{Adapter: "codex", Protocol: cc.Protocol, Executable: "codex", Args: []string{"app-server", "--listen", "stdio://"}, Input: "codex-app-server", CodexAppServer: &CodexAppServerSpec{WindowsSandbox: "unelevated", ProviderID: "owned041", BaseURL: "http://127.0.0.1:49141/v1", APIKeyEnvRef: "SUMA_OWNED_CODEX_041"}}
	t.Setenv("SUMA_OWNED_CODEX_041", "owned-ref-secret")
	t.Setenv("CODEX_HOME", "foreign-account-home")
	t.Setenv("OPENAI_API_KEY", "implicit-key-must-not-forward")
	if err := validateCodexAppServerSpec(spec); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	env, secrets, err := codexEnvironment(spec, home)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "foreign-account-home") || strings.Contains(joined, "implicit-key-must-not-forward") || !strings.Contains(joined, "CODEX_HOME="+home) || !strings.Contains(joined, "SUMA_OWNED_CODEX_041=owned-ref-secret") || !strings.Contains(strings.Join(secrets, "\n"), "owned-ref-secret") {
		t.Fatal("account context was inherited or explicit credential was lost")
	}
	configSHA := CodexConfigurationDigest(*spec.CodexAppServer)
	policyBytes, _ := json.Marshal(struct {
		Version        int                `json:"version"`
		ApprovalPolicy string             `json:"approval_policy"`
		Provider       CodexAppServerSpec `json:"provider"`
	}{4, cc.ApprovalPolicy, *spec.CodexAppServer})
	if configSHA != digest(policyBytes) || !strings.Contains(codexConfiguration(*spec.CodexAppServer, home), "approval_policy = \"on-request\"") || strings.Contains(codexConfiguration(*spec.CodexAppServer, home), "never") {
		t.Fatal("native approval policy was absent from the immutable configuration")
	}
	copy := *spec.CodexAppServer
	copy.BaseURL += "/different"
	if configSHA == CodexConfigurationDigest(copy) || strings.Contains(codexConfiguration(*spec.CodexAppServer, home), "owned-ref-secret") {
		t.Fatal("configuration digest/secret boundary failed")
	}
	bad := spec
	bad.Env = map[string]string{"CODEX_HOME": "foreign"}
	if validateCodexAppServerSpec(bad) == nil {
		t.Fatal("home override accepted")
	}
	copy = *spec.CodexAppServer
	copy.BaseURL = "https://user:password@example.invalid/v1"
	bad = spec
	bad.CodexAppServer = &copy
	if validateCodexAppServerSpec(bad) == nil {
		t.Fatal("URL credential accepted")
	}
	catalog, err := cc.NewCatalog(configSHA, []cc.ModelOption{{Model: "owned-model-041", ReasoningEfforts: []string{"high"}, DefaultReasoningEffort: "high"}})
	if err != nil {
		t.Fatal(err)
	}
	limit := int64(30)
	binding, err := cc.NewBinding(catalog, cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "owned-model-041", ReasoningEffort: "high", TokenNoticeLimit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	a := Assignment{RunID: "journal-041", NodeID: "agent", Attempt: 1, Fence: 9, WorkerID: "worker-041", Config: map[string]any{"codex_execution": binding}}
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	*binding.TokenNoticeLimit = 999
	stored, err := cc.DecodeBinding(j.entries[key(a)].Assignment.Config["codex_execution"])
	if err != nil || stored.TokenNoticeLimit == nil || *stored.TokenNoticeLimit != 30 {
		t.Fatal("accepted binding aliases caller memory")
	}
	evidence := &cc.Evidence{BindingSHA256: strings.Repeat("a", 64), RequestedModel: "owned-model-041", RequestedReasoningEffort: "high", ReroutedModels: []string{"observed-reroute"}}
	result := Result{Assignment: a, ExitCode: -1, OutcomeUnknown: true, ToolEvidence: &tc.Evidence{CodexExecution: evidence}}
	if err = j.Finish(result); err != nil {
		t.Fatal(err)
	}
	evidence.ReroutedModels[0] = "caller-mutated"
	pending := j.Pending()
	if len(pending) != 1 || pending[0].ToolEvidence.CodexExecution.ReroutedModels[0] != "observed-reroute" {
		t.Fatal("durable Codex evidence aliases caller memory")
	}
	pending[0].ToolEvidence.CodexExecution.ReroutedModels[0] = "reader-mutated"
	if j.Pending()[0].ToolEvidence.CodexExecution.ReroutedModels[0] != "observed-reroute" {
		t.Fatal("pending evidence aliases journal memory")
	}
	if err = j.Ack(pending[0].OperationID); err != nil || len(j.Pending()) != 0 {
		t.Fatal("exact result ACK did not drain")
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if len(j.Pending()) != 0 {
		t.Fatal("acknowledged effect was reopened")
	}
}

func TestCodexAppServerFinalFrameAndUsageIntegrity(t *testing.T) {
	profile := tc.Profile{CommandID: "codex041", Adapter: "codex", Protocol: cc.Protocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "fixture"}
	newDriver := func() *codexAppServer {
		spec := CommandSpec{CodexAppServer: &CodexAppServerSpec{WindowsSandbox: "unelevated", ProviderID: "owned041", BaseURL: "http://127.0.0.1:49141/v1"}}
		stream := newToolStream(profile, nil, nil)
		d := newCodexAppServer(profile, spec, t.TempDir(), t.TempDir(), "fixture", &cc.Binding{BindingSHA256: strings.Repeat("c", 64), Model: "owned-model-041", ReasoningEffort: "high"}, stream)
		d.thread, d.turn = "thread-041", "turn-041"
		d.evidence.ThreadID, d.evidence.TurnID = d.thread, d.turn
		d.evidence.ConfigurationAcknowledged = true
		d.turnStarted, d.turnAcknowledged = true, true
		return d
	}
	d := newDriver()
	terminal := []byte(`{"method":"turn/completed","params":{"threadId":"thread-041","turn":{"id":"turn-041","status":"completed"}}}`)
	_, _ = d.Write(terminal)
	d.Finish()
	if d.bad != nil || d.stream.terminal != "completed" {
		t.Fatal("complete final frame without newline was lost")
	}
	_, _ = d.Write([]byte("{"))
	d.Finish()
	if d.bad == nil {
		t.Fatal("truncated final stdout was accepted after completion")
	}
	d = newDriver()
	usage := func(total int64) map[string]any {
		return map[string]any{"total": map[string]any{"totalTokens": json.Number(fmt.Sprint(total)), "inputTokens": json.Number("23"), "outputTokens": json.Number("17"), "cachedInputTokens": json.Number("0"), "cacheWriteInputTokens": json.Number("0"), "reasoningOutputTokens": json.Number("2")}}
	}
	d.usage(usage(40))
	if d.bad != nil {
		t.Fatal(d.bad)
	}
	d.usage(usage(0))
	if d.bad == nil || *d.stream.usage.InputTokens != 23 || *d.stream.usage.OutputTokens != 17 {
		t.Fatal("inconsistent total replaced known usage")
	}
}
