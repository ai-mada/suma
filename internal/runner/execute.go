package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	ec "github.com/ai-mada/suma/internal/executioncontract"
	mc "github.com/ai-mada/suma/internal/modelcontract"
	mg "github.com/ai-mada/suma/internal/modelgateway"
	"github.com/ai-mada/suma/internal/releasecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	wc "github.com/ai-mada/suma/internal/workspaceoperationcontract"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type CommandSpec struct {
	Adapter                string                 `json:"adapter,omitempty"`
	Protocol               string                 `json:"protocol,omitempty"`
	Executable             string                 `json:"executable"`
	Args                   []string               `json:"args"`
	Env                    map[string]string      `json:"env,omitempty"`
	EnvFrom                []string               `json:"env_from,omitempty"`
	Input                  string                 `json:"input,omitempty"`
	ReadOnly               bool                   `json:"read_only,omitempty"`
	Planning               bool                   `json:"planning,omitempty"`
	KnowledgeExtraction    bool                   `json:"knowledge_extraction,omitempty"`
	FailureDiagnosis       bool                   `json:"failure_diagnosis,omitempty"`
	IntelligentOperations  bool                   `json:"intelligent_operations,omitempty"`
	DispatchRecommendation bool                   `json:"dispatch_recommendation,omitempty"`
	TaskConversationReply  bool                   `json:"task_conversation_reply,omitempty"`
	CandidateComposition   bool                   `json:"candidate_composition,omitempty"`
	TestEvidence           bool                   `json:"test_evidence,omitempty"`
	TestReport             bool                   `json:"test_report,omitempty"`
	ArtifactOutput         string                 `json:"artifact_output,omitempty"`
	ArtifactInput          bool                   `json:"artifact_input,omitempty"`
	NativeWorkspace        *NativeWorkspacePolicy `json:"native_workspace,omitempty"`
	ModelExecutionProtocol string                 `json:"model_execution_protocol,omitempty"`
	CodexAppServer         *CodexAppServerSpec    `json:"codex_app_server,omitempty"`
	OpenCode               *OpenCodeSpec          `json:"opencode,omitempty"`
}

type nativeAgentLimits struct {
	MaxSteps  int `json:"max_steps"`
	MaxTokens int `json:"max_tokens"`
}

// Native workspace requests have their own stdin envelope. Validate its bounds
// after the exact tool/purpose checks and before entering any workspace, then
// carry the same limits into that envelope instead of falling back in the SDK.
// Repair limits require the typed repair execution, never its string marker.
func nativeExecutionLimits(a Assignment) (nativeAgentLimits, error) {
	limits := nativeAgentLimits{MaxSteps: 8, MaxTokens: 4096}
	maxSteps, maxTokens := mc.MaxSteps, int(mc.MaxOutputTokens)
	if a.CandidateConflictRepair != nil {
		limits.MaxSteps, maxSteps, maxTokens = 64, 128, 131072
	}
	if value, present := a.Config["model_execution"]; present {
		binding, err := mc.DecodeBinding(value)
		if err != nil {
			return nativeAgentLimits{}, fmt.Errorf("model execution binding is invalid")
		}
		if binding.Parameters.MaxSteps > maxSteps || binding.Parameters.MaxOutputTokens > int64(maxTokens) {
			return nativeAgentLimits{}, fmt.Errorf("Native agent execution limits are invalid")
		}
		limits.MaxSteps, limits.MaxTokens = binding.Parameters.MaxSteps, int(binding.Parameters.MaxOutputTokens)
	}
	for _, field := range []struct {
		name     string
		fallback int
		maximum  int
		target   *int
	}{
		{"max_steps", limits.MaxSteps, maxSteps, &limits.MaxSteps},
		{"max_output_tokens", limits.MaxTokens, maxTokens, &limits.MaxTokens},
	} {
		value, present := a.Config[field.name]
		if present && value == nil {
			return nativeAgentLimits{}, fmt.Errorf("Native agent execution limits are invalid")
		}
		if number, ok := value.(json.Number); ok {
			n, err := number.Int64()
			if err != nil || number.String() != fmt.Sprint(n) {
				return nativeAgentLimits{}, fmt.Errorf("Native agent execution limits are invalid")
			}
		}
		n, err := planningLimit(value, field.fallback, field.maximum)
		if err != nil {
			return nativeAgentLimits{}, fmt.Errorf("Native agent execution limits are invalid")
		}
		*field.target = n
	}
	if value, present := a.Config["model_execution"]; present {
		binding, _ := mc.DecodeBinding(value)
		if limits.MaxSteps != binding.Parameters.MaxSteps || int64(limits.MaxTokens) != binding.Parameters.MaxOutputTokens {
			return nativeAgentLimits{}, fmt.Errorf("model execution limits differ from the immutable binding")
		}
	}
	return limits, nil
}

func nativeAgentInput(a Assignment, limits nativeAgentLimits, bridge NativeWorkspaceBridge) ([]byte, error) {
	goal, ok := a.Config["goal"].(string)
	if !ok || strings.TrimSpace(goal) == "" || len(goal) > 32000 {
		return nil, fmt.Errorf("agent config.goal must contain 1–32000 characters")
	}
	return json.Marshal(struct {
		Goal            string                `json:"goal"`
		RequestID       string                `json:"request_id"`
		MaxSteps        int                   `json:"max_steps"`
		MaxTokens       int                   `json:"max_tokens"`
		WorkspaceBridge NativeWorkspaceBridge `json:"workspace_bridge"`
		ModelExecution  any                   `json:"model_execution,omitempty"`
	}{Goal: goal, RequestID: digest([]byte(key(a))), MaxSteps: limits.MaxSteps, MaxTokens: limits.MaxTokens, WorkspaceBridge: bridge, ModelExecution: a.Config["model_execution"]})
}

type ExecConfig struct {
	AllowExec                            bool
	Workspace                            string
	Commands                             map[string]CommandSpec
	Secrets                              []string
	OutputLimit                          int
	External                             map[string]ExternalSpec
	Repository, BaseCommit, WorktreeRoot string
	Progress                             func(string) error
	ToolProgress                         func(tc.Event) error
	WorkspaceProgress                    func(wc.Event) error
	ToolInventory                        []tc.Observation
	ModelDeployments                     []ModelDeploymentSpec
	WorkspaceObservations                *WorkspaceObservationRegistry
	Artifacts                            *Client
	Journal                              *Journal
	resolvedArtifact                     *releasecontract.ArtifactBinding
	candidateImport                      *cc.ImportReceipt
}
type output struct {
	mu           sync.Mutex
	b            bytes.Buffer
	limit        int
	secrets      []string
	truncated    bool
	progress     func(string) error
	progressSent int
	progressErr  error
}

func newOutput(limit int, secrets []string) *output {
	if limit <= 0 {
		limit = 128 << 10
	}
	return &output{limit: limit, secrets: secrets}
}
func (b *output) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	remaining := b.limit - b.b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.b.Write(p)
	}
	if n > remaining {
		b.truncated = true
	}
	if b.progress != nil && b.progressErr == nil && b.b.Len() > b.progressSent && (bytes.Contains(p, []byte{'\n'}) || b.b.Len()-b.progressSent >= 4096) {
		b.progressSent = b.b.Len()
		b.progressErr = b.progress(progressText(b.stringLocked(), b.secrets))
	}
	if b.progressErr != nil {
		return n, b.progressErr
	}
	return n, nil
}

var ansi = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var credential = regexp.MustCompile(`(?i)(authorization["']?\s*[:=]\s*["']?(?:bearer\s+)?|(?:api[_-]?key|private[_-]?key|master[_-]?key|access[_-]?key|password|secret|token|credential)["']?\s*[:=]\s*["']?)[^\s"',}]+`)

func (b *output) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stringLocked()
}
func (b *output) stringLocked() string {
	s := b.b.String()
	for _, secret := range b.secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, "[REDACTED]")
			for n := min(len(secret)-1, len(s)); n > 0; n-- {
				if strings.HasSuffix(s, secret[:n]) {
					s = strings.TrimSuffix(s, secret[:n]) + "[REDACTED]"
					break
				}
			}
		}
	}
	s = credential.ReplaceAllString(s, "[REDACTED]")
	s = ansi.ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' && r != '\r' {
			return -1
		}
		return r
	}, s)
	if b.truncated {
		s += "\n[output truncated]"
	}
	return s
}
func progressText(message string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
			for n := min(len(secret)-1, len(message)); n > 0; n-- {
				if strings.HasSuffix(message, secret[:n]) {
					message = strings.TrimSuffix(message, secret[:n]) + "[REDACTED]"
					break
				}
			}
		}
	}
	message = ansi.ReplaceAllString(message, "")
	message = credential.ReplaceAllString(message, "[REDACTED]")
	message = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' && r != '\r' {
			return -1
		}
		return r
	}, message)
	if len(message) > 8192 {
		message = message[len(message)-8192:]
		for !utf8.ValidString(message) && len(message) > 0 {
			message = message[1:]
		}
	}
	return message
}
func (b *output) flushProgress() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.progressErr == nil && b.progress != nil && b.b.Len() > b.progressSent {
		b.progressSent = b.b.Len()
		b.progressErr = b.progress(progressText(b.stringLocked(), b.secrets))
	}
	return b.progressErr
}
func contains(a, b string) bool { return strings.Contains(a, b) }
func Execute(ctx context.Context, a Assignment, cfg ExecConfig) Result {
	// Freeze nested reviewed bindings in the same wire form used by the
	// journal and durable attempt. Typed in-process callers must not change
	// identity merely because a saved attempt is decoded into JSON objects.
	a = cloneModelAssignment(a)
	if a.CandidateConflictRepair != nil && (a.Type != "agent" || a.CandidateComposition != nil || a.CodeBaseline != nil || a.RepositoryIntegration != nil || a.ReleaseExecution != nil || hasCandidate(a)) {
		return Result{Assignment: a, ExitCode: -1, Error: "conflict repair requires a dedicated native agent input"}
	}
	if a.CandidateComposition != nil && a.Type != "command" {
		return Result{Assignment: a, ExitCode: -1, Error: "candidate composition requires an isolated command node"}
	}
	if a.CodeBaseline != nil && a.Type != "agent" {
		return Result{Assignment: a, ExitCode: -1, Error: "code baseline requires isolated development"}
	}
	if a.CandidateComposition != nil && (a.CodeBaseline != nil || a.RepositoryIntegration != nil || a.ReleaseExecution != nil || hasCandidate(a)) {
		return Result{Assignment: a, ExitCode: -1, Error: "candidate composition cannot be mixed with another input mode"}
	}
	if a.CodeBaseline != nil && (a.CandidateComposition != nil || a.RepositoryIntegration != nil || a.ReleaseExecution != nil || hasCandidate(a)) {
		return Result{Assignment: a, ExitCode: -1, Error: "code baseline cannot be mixed with another input mode"}
	}
	if a.RepositoryIntegration != nil {
		return executeRepositoryIntegration(ctx, a, cfg)
	}
	if a.ReleaseExecution != nil {
		return executeRelease(ctx, a, cfg)
	}
	if hasCandidate(a) && a.Type != "test" && a.Type != "command" && a.Type != "agent" {
		return Result{Assignment: a, ExitCode: -1, Error: "candidate envelopes require a test or administrator build profile"}
	}
	if a.Type == "external_workflow" {
		return executeExternal(ctx, a, cfg)
	}
	r := Result{Assignment: a, ExitCode: -1}
	var startupProfile *tc.Profile
	beforeWorkspace := true
	fail := func(e string) Result {
		r.Error = e
		if startupProfile != nil && beforeWorkspace {
			r.StartupFailure = &ec.StartupFailureProof{Protocol: ec.StartupFailureProtocol, Stage: "before_workspace", Assignment: candidateRef(a), Profile: *startupProfile, Purpose: "candidate_conflict_repair"}
		}
		return r
	}
	if !cfg.AllowExec {
		return fail("execution disabled: set --allow-exec with a command allowlist")
	}
	if a.Type != "command" && a.Type != "test" && a.Type != "agent" {
		return fail("unsupported node type")
	}
	if len(a.Command) != 1 {
		return fail("command must name one installed allowlist entry; arguments belong to local specification")
	}
	spec, ok := cfg.Commands[a.Command[0]]
	if !ok {
		return fail("command is not in the local allowlist")
	}
	if err := validateTestReportSpec(spec); err != nil {
		return fail(err.Error())
	}
	if spec.TestReport && a.Type != "test" {
		return fail("test reports require an isolated administrator-designated command test")
	}
	if a.CandidateComposition != nil || spec.CandidateComposition || a.Command[0] == "suma-compose" {
		return executeCandidateComposition(ctx, a, cfg, spec)
	}
	if a.CodeBaseline != nil {
		if a.Type != "agent" || spec.ReadOnly || spec.ArtifactInput || spec.ArtifactOutput != "" {
			return fail("code baseline requires isolated development")
		}
		ctx = context.WithValue(ctx, nativeGitScope{}, true)
		baselineDuration := time.Duration(a.TimeoutSeconds) * time.Second
		if baselineDuration <= 0 {
			baselineDuration = 10 * time.Minute
		}
		if baselineDuration > 24*time.Hour {
			baselineDuration = 24 * time.Hour
		}
		var baselineCancel context.CancelFunc
		ctx, baselineCancel = context.WithTimeout(ctx, baselineDuration)
		defer baselineCancel()
	}
	if spec.NativeWorkspace != nil {
		if a.Type != "agent" {
			return fail("native workspace requires an isolated agent node")
		}
		ctx = context.WithValue(ctx, nativeGitScope{}, true)
	}
	if a.CandidateConflictRepair != nil && (spec.NativeWorkspace == nil || !spec.NativeWorkspace.ConflictRepair) {
		return fail("native conflict repair capability is not enabled")
	}
	if spec.ArtifactOutput != "" || spec.ArtifactInput {
		if _, _, err := artifactProfile(a.Command[0], spec); err != nil {
			return fail("artifact profile: " + err.Error())
		}
		if spec.ArtifactInput && a.Type != "test" {
			return fail("artifact input requires a test node")
		}
		if cfg.Artifacts == nil {
			return fail("artifact control client is required")
		}
	}
	if hasCandidate(a) && spec.ReadOnly {
		return fail("candidate tests require an isolated command; shared read_only mode is forbidden")
	}
	if hasCandidate(a) && a.Type != "test" && spec.ArtifactOutput == "" {
		return fail("candidate commands require an administrator artifact build profile")
	}
	if hasCandidate(a) && a.Type == "test" && !spec.TestEvidence {
		return fail("candidate tests require an administrator-designated test command")
	}
	if ctx.Err() != nil {
		return fail(ctx.Err().Error())
	}
	spec = normalizedCommand(spec)
	profile, executable, profileErr := launchProfile(ctx, a, cfg, spec)
	if profileErr != nil {
		return fail(profileErr.Error())
	}
	if err := openCodeExecutionRequest(a, spec, profile); err != nil {
		return fail(err.Error())
	}
	if err := octopExecutionRequest(a, spec, profile); err != nil {
		return fail(err.Error())
	}
	purposeInput, purposeErr := modelPurposeRequest(a, spec, profile)
	if purposeErr != nil {
		return fail(purposeErr.Error())
	}
	if a.CandidateConflictRepair != nil {
		startupProfile = &profile
	}
	modelExecution, modelErr := modelExecutionRequest(a, cfg, spec, profile)
	if modelErr != nil {
		return fail(modelErr.Error())
	}
	codexBinding, codexErr := codexExecutionRequest(a, cfg, spec)
	if codexErr != nil {
		return fail(codexErr.Error())
	}
	var nativeLimits nativeAgentLimits
	if spec.NativeWorkspace != nil && spec.OpenCode == nil && spec.Adapter != "octop" {
		limits, limitsErr := nativeExecutionLimits(a)
		if limitsErr != nil {
			return fail(limitsErr.Error())
		}
		nativeLimits = limits
	}
	if spec.ArtifactInput {
		binding, resolveErr := resolveArtifact(ctx, a, cfg)
		if resolveErr != nil {
			return fail("artifact input: " + resolveErr.Error())
		}
		cfg.resolvedArtifact = &binding
	}
	root, err := filepath.Abs(cfg.Workspace)
	if err != nil {
		return fail("invalid workspace")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return fail(fmt.Sprintf("workspace unavailable: %v", err))
	}
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return fail("workspace must be a directory")
	}
	var workspace *attemptWorkspace
	if !spec.ReadOnly {
		// Entering preparation can create private Git/filesystem state even
		// when it returns an error. Only failures before this call certify it.
		beforeWorkspace = false
		workspace, err = prepareWorkspace(ctx, a, cfg, spec)
		if err != nil {
			return fail(err.Error())
		}
		root = workspace.Source
	}
	modelAttempt := workspace
	if spec.DispatchRecommendation || spec.TaskConversationReply {
		modelAttempt, err = prepareReadOnlyModelAttempt(a, cfg)
		if err != nil {
			return fail(err.Error())
		}
		root = modelAttempt.Source
	}
	var reportCapture *testReportCapture
	if spec.TestReport {
		reportCapture, err = prepareTestReport(workspace)
		if err != nil {
			return fail(err.Error())
		}
	}
	artifactDir := ""
	if spec.ArtifactInput {
		artifactDir = filepath.Join(root, ".cache", "runner", "artifact-input")
		if err = materializeArtifact(ctx, cfg.Artifacts, a, *cfg.resolvedArtifact, artifactDir); err != nil {
			return fail("artifact input: " + err.Error())
		}
	}
	duration := time.Duration(a.TimeoutSeconds) * time.Second
	if duration <= 0 {
		duration = 10 * time.Minute
	}
	if duration > 24*time.Hour {
		duration = 24 * time.Hour
	}
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	// Expand only the administrator's literal template into an argument copy.
	// The profile digest keeps the original template; no shell or remote value
	// participates, so absolute paths containing spaces stay a single argument.
	arguments := append([]string(nil), spec.Args...)
	if reportCapture != nil {
		for index, argument := range arguments {
			arguments[index] = strings.ReplaceAll(argument, "{{test_report_path}}", reportCapture.path)
		}
	}
	cmd := exec.Command(executable, arguments...)
	cmd.Dir = root
	var dshStdin *dshInput
	var dshPipe io.WriteCloser
	var codexPipe io.WriteCloser
	var codexDriver *codexAppServer
	if (spec.Adapter == "harness" || spec.Adapter == "codebuddy") && a.Type != "agent" {
		return fail("team CLI profiles require an agent node")
	}
	if a.Type == "agent" {
		goal, _ := a.Config["goal"].(string)
		goalLimit := 32000
		if spec.DispatchRecommendation || spec.TaskConversationReply {
			goalLimit = 64000
		}
		if strings.TrimSpace(goal) == "" || len(goal) > goalLimit {
			return fail(fmt.Sprintf("agent config.goal must contain 1–%d UTF-8 bytes", goalLimit))
		}
		switch spec.Input {
		case "agent-jsonl":
			input := map[string]any{"goal": goal, "request_id": digest([]byte(key(a)))}
			if (spec.DispatchRecommendation || spec.TaskConversationReply) && modelExecution != nil {
				input["model_execution"] = modelExecution.Binding
			}
			for name, value := range purposeInput {
				input[name] = value
			}
			b, _ := json.Marshal(input)
			cmd.Stdin = bytes.NewReader(append(b, '\n'))
		case "goal":
			cmd.Stdin = strings.NewReader(goal + "\n")
		case "dsh-jsonl":
			if spec.Adapter != "harness" {
				return fail("dsh-jsonl input requires the Harness adapter")
			}
			dshStdin, err = newDshInput(digest([]byte(key(a))), root, goal)
			if err != nil {
				return fail("Harness input could not be prepared")
			}
			defer dshStdin.Close()
			// Own this copy loop explicitly. Giving exec.Cmd a blocking Reader
			// would make Wait await stdin even when a child dies without a result.
			dshPipe, err = cmd.StdinPipe()
			if err != nil {
				return fail("Harness stdin pipe could not be opened")
			}
			defer dshPipe.Close()
		case "codex-app-server":
			if codexBinding == nil {
				return fail("Codex app-server requires its fixed execution binding")
			}
			codexPipe, err = cmd.StdinPipe()
			if err != nil {
				return fail("Codex app-server stdin pipe could not be opened")
			}
			defer codexPipe.Close()
		case "octop-acp-jsonl":
			if spec.Adapter != "octop" {
				return fail("Octop requires its fixed local configuration")
			}
			dshPipe, err = cmd.StdinPipe()
			if err != nil {
				return fail("Octop stdin pipe could not be opened")
			}
			defer dshPipe.Close()
		case "opencode-sdk-jsonl":
			if spec.OpenCode == nil {
				return fail("OpenCode SDK requires its fixed local configuration")
			}
			dshPipe, err = cmd.StdinPipe()
			if err != nil {
				return fail("OpenCode SDK stdin pipe could not be opened")
			}
			defer dshPipe.Close()
		default:
			return fail("agent command specification requires a supported fixed stdin format")
		}
	}
	var environmentSecrets []string
	cmd.Env, environmentSecrets, err = commandEnvironment(spec)
	if codexBinding != nil {
		// Replace the inherited home and authentication context before the
		// child starts. A separate generated home contains no login state.
		codexHome := filepath.Join(workspace.Directory, "codex-home")
		if err = prepareCodexHome(spec, codexHome, root); err != nil {
			return fail("Codex isolated configuration could not be prepared")
		}
		cmd.Dir = codexHome
		cmd.Env, environmentSecrets, err = codexEnvironment(spec, codexHome)
		cfg.Secrets = append(cfg.Secrets, codexHome, root, spec.CodexAppServer.BaseURL)
	}
	if spec.OpenCode != nil {
		home := filepath.Join(workspace.Directory, "opencode-home")
		cmd.Env, environmentSecrets, err = openCodeEnvironment(spec, home, root, true)
		cmd.Dir = home
		cfg.Secrets = append(cfg.Secrets, home, root, spec.OpenCode.BaseURL)
	}
	if err != nil {
		return fail(err.Error())
	}
	if spec.Adapter == "octop" {
		home := filepath.Join(workspace.Directory, "octop-home")
		cmd.Env, environmentSecrets, err = octopEnvironment(spec, home)
		cmd.Dir = home
		cfg.Secrets = append(cfg.Secrets, home, root)
		if err != nil {
			return fail(err.Error())
		}
	}
	if modelExecution != nil {
		// The provider key stays in this Go process. The loopback connection is
		// injected only after its durable model gateway has been opened below.
		cfg.Secrets = append(cfg.Secrets, modelExecution.APIKey)
	}
	if spec.Adapter == "codebuddy" {
		// One-shot headless tasks must not leave background agents or tools
		// outside the result interval. The decoder also rejects such events.
		kept := cmd.Env[:0]
		for _, value := range cmd.Env {
			if !strings.HasPrefix(strings.ToUpper(value), "CODEBUDDY_CODE_DISABLE_BACKGROUND_TASKS=") {
				kept = append(kept, value)
			}
		}
		cmd.Env = append(kept, "CODEBUDDY_CODE_DISABLE_BACKGROUND_TASKS=1")
	}
	cfg.Secrets = append(cfg.Secrets, environmentSecrets...)
	var native *nativeWorkspace
	if spec.NativeWorkspace != nil {
		if err = validateNativeWorkspacePolicy(cfg, a.Command[0], spec); err != nil {
			return fail(err.Error())
		}
		native, err = openNativeWorkspace(ctx, a, cfg, workspace, spec, profile.ProfileSHA256, func(checkCtx context.Context, id string) (NativeWorkspaceCheckResult, error) {
			return runPinnedNativeCheck(checkCtx, cfg, workspace, spec, profile.ProfileSHA256, id)
		})
		if err != nil {
			return fail("native workspace could not be opened")
		}
		defer native.Close()
		bridge, bridgeErr := native.StartMCP()
		if bridgeErr != nil {
			return fail("native workspace connection could not start")
		}
		cfg.Secrets = append(cfg.Secrets, bridge.Token)
		if cfg.WorkspaceObservations != nil {
			if profileID, _ := a.Config["tool_profile_id"].(string); profileID != "" {
				unregister, registerErr := cfg.WorkspaceObservations.register(native)
				if registerErr != nil {
					return fail("native workspace observation registration failed")
				}
				defer unregister()
			}
		}
		if spec.OpenCode != nil || spec.Adapter == "octop" {
			dshStdin, err = newHostAgentInput(digest([]byte(key(a))), a.Config["goal"].(string), bridge, a.Config["model_execution"])
			if err != nil {
				return fail(err.Error())
			}
			defer dshStdin.Close()
		} else {
			payload, inputErr := nativeAgentInput(a, nativeLimits, bridge)
			if inputErr != nil {
				return fail(inputErr.Error())
			}
			cmd.Stdin = bytes.NewReader(append(payload, '\n'))
		}
	}
	var gatewaySnapshot mg.Snapshot
	var gatewayCloseError error
	closeModelGateway := func() {}
	if modelExecution != nil {
		gateway, gatewayErr := openAttemptModelGateway(ctx, cancel, a, cfg, modelAttempt, profile, modelExecution)
		if gatewayErr != nil {
			return fail("model gateway could not record its original attempt")
		}
		var once sync.Once
		closeModelGateway = func() { once.Do(func() { gatewaySnapshot, gatewayCloseError = gateway.Close() }) }
		defer closeModelGateway()
		cmd.Env = applyModelExecutionEnvironment(cmd.Env, modelExecution.Binding, gateway.Connection())
		cfg.Secrets = append(cfg.Secrets, gateway.Connection().Token, modelExecution.BaseURL)
	}
	var codexRead *liveWorkspaceRead
	var observationCloseErr error
	closeObservation := func() {}
	if codexBinding != nil && cfg.WorkspaceObservations != nil {
		if profileID, _ := a.Config["tool_profile_id"].(string); profileID != "" {
			codexRead, err = openLiveWorkspaceRead(ctx, a, cfg, workspace, profile.ProfileSHA256)
			if err != nil {
				return fail("Codex workspace observation could not be opened")
			}
			unregister, registerErr := cfg.WorkspaceObservations.registerRead(codexRead, nil)
			if registerErr != nil {
				_ = codexRead.Close()
				return fail("Codex workspace observation registration failed")
			}
			var once sync.Once
			closeObservation = func() {
				once.Do(func() {
					codexRead.cancel()
					unregister()
					observationCloseErr = codexRead.Close()
				})
			}
			defer closeObservation()
		}
	}
	tempRoot := filepath.Join(root, ".cache", "runner", "tmp", attemptKey(a))
	if workspace != nil {
		tempRoot = filepath.Join(workspace.Directory, "tmp")
	}
	if err = safeMkdir(tempRoot); err != nil {
		return fail("execution temporary directory unavailable")
	}
	// Overrides are process-local and cannot be supplied by the remote workflow.
	filtered := cmd.Env[:0]
	for _, env := range cmd.Env {
		upper := strings.ToUpper(env)
		if !strings.HasPrefix(upper, "TEMP=") && !strings.HasPrefix(upper, "TMP=") && !strings.HasPrefix(upper, "TMPDIR=") {
			filtered = append(filtered, env)
		}
	}
	cmd.Env = append(filtered, "TEMP="+tempRoot, "TMP="+tempRoot, "TMPDIR="+tempRoot)
	if reportCapture != nil {
		// The dedicated absolute path is Runner-owned and never read from the
		// remote node config. Drop any inherited spelling before injecting it.
		kept := cmd.Env[:0]
		for _, value := range cmd.Env {
			if !strings.HasPrefix(strings.ToUpper(value), "SUMA_TEST_REPORT_PATH=") {
				kept = append(kept, value)
			}
		}
		cmd.Env = append(kept, "SUMA_TEST_REPORT_PATH="+reportCapture.path)
	}
	if artifactDir != "" {
		filtered = cmd.Env[:0]
		for _, value := range cmd.Env {
			if !strings.HasPrefix(strings.ToUpper(value), "SUMA_ARTIFACT_DIR=") {
				filtered = append(filtered, value)
			}
		}
		cmd.Env = append(filtered, "SUMA_ARTIFACT_DIR="+artifactDir)
	}
	// Go requires an explicit cache on minimal Windows environments. Keep its
	// default process-local too; administrators may supply a shared cache.
	for name, value := range map[string]string{"GOCACHE": filepath.Join(tempRoot, "go-build"), "GOTMPDIR": tempRoot} {
		present := false
		for _, env := range cmd.Env {
			if strings.HasPrefix(strings.ToUpper(env), name+"=") {
				present = true
				break
			}
		}
		if !present {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	out := newOutput(cfg.OutputLimit, cfg.Secrets)
	if cfg.Progress != nil {
		out.progress = func(message string) error {
			err := cfg.Progress(message)
			if err != nil {
				cancel()
			}
			return err
		}
	}
	var stream *toolStream
	if profile.Protocol != "text-v1" {
		var emit func(tc.Event) error
		if pinned, _ := a.Config["tool_profile_sha256"].(string); pinned != "" {
			emit = cfg.ToolProgress
			if modelExecution != nil && emit != nil {
				forward := emit
				emit = func(event tc.Event) error {
					// SDK usage is not an accounting authority. Final host receipts
					// replace it; ordinary model/tool activity is still forwarded.
					if event.Kind == "usage" {
						return nil
					}
					return forward(event)
				}
			}
		}
		stream = newToolStream(profile, cfg.Secrets, emit, digest([]byte(key(a))))
		if modelExecution != nil {
			stream.modelBinding = &modelExecution.Binding
		}
		stream.abort = cancel
		if dshStdin != nil {
			stream.onTerminal = func() { _ = dshStdin.Close() }
		}
		if codexBinding != nil {
			if codexRead != nil {
				// Revoke before publishing a terminal or capturing Git evidence.
				stream.onTerminal = closeObservation
			}
			codexDriver = newCodexAppServer(profile, spec, cmd.Dir, root, a.Config["goal"].(string), codexBinding, stream)
			defer codexDriver.Close()
			cmd.Stdout = codexDriver
		} else {
			cmd.Stdout = stream
		}
	} else {
		cmd.Stdout = out
	}
	cmd.Stderr = out
	if ctx.Err() != nil {
		return fail(ctx.Err().Error())
	}
	latestExecutable, latestSHA, digestErr := commandExecutable(spec)
	if digestErr != nil {
		return fail("tool profile executable became unavailable before launch")
	}
	latestDigest, digestErr := effectiveCommandProfileDigest(cfg, spec, latestSHA)
	if digestErr != nil || latestDigest != profile.ProfileSHA256 || latestExecutable != executable {
		return fail("tool profile changed before process launch")
	}
	if workspace != nil {
		workspace.State = "running"
		if err = workspace.save(); err != nil {
			return fail("durable workspace start unavailable")
		}
	}
	controller, err := startProcess(cmd)
	if err != nil {
		closeObservation()
		if workspace != nil {
			r.WorkspaceEvidence, _ = workspace.capture(context.Background(), r)
		}
		return fail(fmt.Sprintf("process start: %v", err))
	}
	defer controller.Close()
	if dshStdin != nil {
		stdinDone := make(chan struct{})
		go func() {
			defer close(stdinDone)
			_, _ = io.Copy(dshPipe, dshStdin)
			_ = dshPipe.Close()
		}()
		defer func() {
			_ = dshStdin.Close()
			_ = dshPipe.Close()
			<-stdinDone
		}()
	}
	if codexDriver != nil {
		stdinDone := make(chan struct{})
		go func() {
			defer close(stdinDone)
			_, _ = codexDriver.CopyInput(codexPipe)
			_ = codexPipe.Close()
		}()
		defer func() { _ = codexDriver.Close(); _ = codexPipe.Close(); <-stdinDone }()
	}
	done := make(chan error, 1)
	processContainmentVerified := true
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
		if err == nil {
			r.ExitCode = 0
		} else if cmd.ProcessState != nil {
			r.ExitCode = cmd.ProcessState.ExitCode()
			r.Error = "process exited unsuccessfully"
		}
	case <-ctx.Done():
		waited := false
		if dshStdin != nil || codexDriver != nil {
			// Give the versioned cancellation request a bounded chance to settle;
			// it does not prove that the agent or its descendants actually stopped.
			if dshStdin != nil {
				_ = dshStdin.Cancel()
			} else {
				_ = codexDriver.Cancel()
			}
			graceDuration := 1500 * time.Millisecond
			if spec.OpenCode != nil || spec.Adapter == "octop" {
				graceDuration = 10 * time.Second
			}
			grace := time.NewTimer(graceDuration)
			select {
			case <-done:
				waited = true
			case <-grace.C:
			}
			grace.Stop()
			if dshStdin != nil {
				_ = dshStdin.Close()
			} else {
				_ = codexDriver.Close()
			}
		}
		stopErr := controller.Stop()
		if !waited {
			<-done
		}
		r.Stopped = stopErr == nil
		r.OutcomeUnknown = stopErr != nil
		r.Error = ctx.Err().Error()
		if stopErr != nil {
			processContainmentVerified = false
			r.Error += "; process tree termination unverified"
		}
	}
	closeObservation()
	if observationCloseErr != nil {
		r.ExitCode = -1
		r.OutcomeUnknown = true
		r.Error = "Codex workspace observation close requires reconciliation"
	}
	if err = controller.Finish(); err != nil {
		processContainmentVerified = false
		r.ExitCode = -1
		r.OutcomeUnknown = true
		r.Error = "process tree final state unverified"
	}
	closeModelGateway()
	r.Output = out.String()
	if stream != nil {
		if codexDriver != nil {
			codexDriver.Finish()
		}
		invalidBeforeFinish := stream.bad != nil
		evidence, streamErr := stream.finish()
		if modelExecution != nil {
			if gatewayCloseError != nil {
				gatewaySnapshot.Closed = false
			}
			complete := applyModelGatewayEvidence(evidence, gatewaySnapshot)
			if gatewayCloseError != nil || evidence.Terminal == "completed" && !complete {
				evidence.Terminal = "unknown"
				streamErr = fmt.Errorf("model gateway response coverage requires reconciliation")
			}
		}
		parentStopped := stream.openCodeState != nil && stream.openCodeState.parentStopped
		applyOpenCodeProcessContainment(evidence, parentStopped, processContainmentVerified)
		octopParentStopped := stream.octopState != nil && stream.octopState.parentStopped
		applyOctopProcessContainment(evidence, octopParentStopped, processContainmentVerified)
		if r.Stopped || r.OutcomeUnknown {
			evidence.Terminal = "unknown"
		}
		if streamErr != nil && !(r.Stopped && !r.OutcomeUnknown && !invalidBeforeFinish) {
			r.ExitCode = -1
			r.OutcomeUnknown = true
			r.Error = streamErr.Error()
		} else if evidence.Terminal == "failed" {
			r.ExitCode = 1
			r.Error = "external tool reported whole-run failure"
		} else if evidence.Terminal != "completed" && !r.Stopped {
			r.ExitCode = -1
			r.OutcomeUnknown = true
			r.Error = "external tool completion is unverified"
		}
		if r.ExitCode != 0 && !r.Stopped && !r.OutcomeUnknown && evidence.Terminal == "completed" {
			evidence.Terminal = "failed"
		}
		if evidence.Terminal == "failed" && !r.Stopped && !r.OutcomeUnknown {
			r.FailureClass = stream.failureClass
		}
		if pinned, _ := a.Config["tool_profile_sha256"].(string); pinned != "" {
			r.ToolEvidence = evidence
		}
		if profile.Protocol == tc.OpenCodeSDKProtocol {
			if stopErr := tc.ValidateOpenCodeResultStop(*evidence, r.Stopped, r.OutcomeUnknown); stopErr != nil {
				r.Stopped = false
				r.ExitCode = -1
				r.OutcomeUnknown = true
				evidence.Terminal = "unknown"
				r.Error = "OpenCode SDK stop acknowledgement requires reconciliation"
			}
		}
		if profile.Protocol == tc.OctopProtocol {
			if stopErr := tc.ValidateOctopResultStop(*evidence, r.Stopped, r.OutcomeUnknown); stopErr != nil {
				r.Stopped, r.OutcomeUnknown, r.ExitCode = false, true, -1
				evidence.Terminal = "unknown"
				r.Error = "Octop process-stop evidence requires reconciliation"
			}
		}
		if (profile.Planning != nil || profile.KnowledgeExtraction != nil || profile.FailureDiagnosis != nil || profile.IntelligentOperations != nil || profile.DispatchRecommendation != nil || profile.TaskConversationReply != nil) && stream.terminal == "completed" && stream.bad == nil {
			r.Output = stream.completionOutput
		} else {
			r.Output = stream.output.String() + r.Output
		}
	} else if pinned, _ := a.Config["tool_profile_sha256"].(string); pinned != "" {
		terminal := "failed"
		if r.ExitCode == 0 && !r.Stopped && !r.OutcomeUnknown {
			terminal = "completed"
		}
		if r.OutcomeUnknown || r.Stopped {
			terminal = "unknown"
		}
		r.ToolEvidence = &tc.Evidence{Profile: profile, Terminal: terminal, UsageState: tc.UsageUnreported}
	}
	if err = out.flushProgress(); err != nil {
		r.ExitCode = -1
		r.Error = "durable progress unavailable: " + err.Error()
	}
	reportDiagnostic := ""
	if reportCapture != nil && processContainmentVerified && !r.Stopped && !r.OutcomeUnknown {
		// Known failed commands still carry their genuine failure/error cases.
		// An uncertain or still-running producer cannot certify case outcomes.
		r.TestReport, err = reportCapture.collect(cfg.Secrets)
		if err != nil {
			if r.ExitCode == 0 && r.Error == "" {
				reportDiagnostic = err.Error()
			}
			r.Error = err.Error()
		} else if reportCapture.failedCases && r.ExitCode == 0 && r.Error == "" {
			// Keep the real process exit and protocol terminal. Parsed failed
			// cases independently make the DAG test node unsuccessful.
			reportDiagnostic = "test report includes failed or error cases"
			r.Error = reportDiagnostic
		}
	}
	if workspace != nil {
		var nativeEvidence *NativeWorkspaceEvidence
		if native != nil {
			// A successful child exit does not override a lease cancellation that
			// arrived at the finalization boundary. Unsealed code is never success.
			if ctx.Err() != nil {
				r.ExitCode = -1
				r.Error = "native workspace execution was cancelled"
			}
			if closeErr := native.Close(); closeErr != nil {
				r.ExitCode = -1
				r.OutcomeUnknown = true
				r.Error = "native workspace close requires reconciliation"
			}
			evidence := native.Evidence()
			nativeEvidence = &evidence
			if evidence.State == "unknown" {
				r.ExitCode = -1
				r.OutcomeUnknown = true
				r.Error = "native workspace effects require reconciliation"
			}
			if r.ExitCode == 0 && r.Error == "" && !r.Stopped && !r.OutcomeUnknown && ctx.Err() == nil {
				seal, sealErr := native.seal(ctx, evidence)
				nativeEvidence.Seal = &seal
				if sealErr != nil {
					r.ExitCode = -1
					r.Error = sealErr.Error()
					if seal.State == "unknown" || seal.State == "prepared" {
						r.OutcomeUnknown = true
						nativeEvidence.State = "unknown"
					}
				} else if seal.State != "sealed" && seal.State != "no_change" {
					r.ExitCode = -1
					r.OutcomeUnknown = true
					r.Error = "native workspace sealing requires reconciliation"
					nativeEvidence.State = "unknown"
				}
			}
			nativeEvidence.OperationTrace = native.traceCoverage()
		}
		var captureErr error
		workspace.nativeEvidence = nativeEvidence
		captureCtx := context.Background()
		if native != nil || a.CodeBaseline != nil {
			captureCtx = context.WithValue(captureCtx, nativeGitScope{}, true)
		}
		r.WorkspaceEvidence, captureErr = workspace.capture(captureCtx, r)
		if captureErr != nil {
			r.ExitCode = -1
			r.OutcomeUnknown = true
			r.Error = "workspace evidence unavailable: " + captureErr.Error()
		}
	}
	if r.ExitCode == 0 && !r.Stopped && !r.OutcomeUnknown && r.Error == "" && (spec.ArtifactInput || spec.ArtifactOutput != "") {
		if err = finishArtifact(ctx, a, cfg, spec, workspace, artifactDir, &r); err != nil {
			r.ExitCode = -1
			r.Error = "artifact evidence: " + err.Error()
		}
	} else if workspace != nil && cfg.Artifacts != nil && (a.Type == "command" || a.Type == "agent") && workspace.HeadCommit != workspace.AdminBaseCommit && r.ExitCode == 0 && !r.Stopped && !r.OutcomeUnknown && r.Error == "" && r.WorkspaceEvidence["clean"] == true {
		if err = finishCandidate(ctx, a, cfg, workspace, &r); err != nil {
			r.ExitCode = -1
			r.Error = "candidate transfer: " + err.Error()
		}
	}
	if r.ToolEvidence != nil {
		if r.OutcomeUnknown || r.Stopped {
			r.ToolEvidence.Terminal = "unknown"
		} else if (r.ExitCode != 0 || r.Error != "") && !(r.ExitCode == 0 && reportDiagnostic != "" && r.Error == reportDiagnostic) {
			r.ToolEvidence.Terminal = "failed"
		}
	}
	if r.OutcomeUnknown || r.Stopped {
		r.TestReport = nil
	}
	return r
}
