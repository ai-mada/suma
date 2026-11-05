package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	cl "github.com/ai-mada/suma/internal/claudecontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	mc "github.com/ai-mada/suma/internal/modelcontract"
	"github.com/ai-mada/suma/internal/strictjson"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
	wwc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

func normalizedCommand(spec CommandSpec) CommandSpec {
	if spec.Adapter == "claude" && spec.ClaudeSDK == nil {
		return normalizeClaudeCommand(spec)
	}
	if spec.Adapter == "" {
		spec.Adapter = "command"
	}
	if spec.Protocol == "" {
		spec.Protocol = "text-v1"
	}
	if spec.Adapter == "codebuddy" {
		// Normalize the enforced headless policy before hashing or probing it.
		// Copy the caller's map so inventory cannot mutate shared configuration.
		env := make(map[string]string, len(spec.Env)+1)
		for name, value := range spec.Env {
			if !strings.EqualFold(name, "CODEBUDDY_CODE_DISABLE_BACKGROUND_TASKS") {
				env[name] = value
			}
		}
		env["CODEBUDDY_CODE_DISABLE_BACKGROUND_TASKS"] = "1"
		spec.Env = env
	}
	return spec
}
func DecodeCommands(raw []byte) (map[string]CommandSpec, error) {
	specs, err := decodeCommandProfiles(raw)
	if err != nil {
		return nil, err
	}
	if err = ValidateToolConfiguration(ExecConfig{Commands: specs}); err != nil {
		return nil, err
	}
	return specs, nil
}

// Decode source structure without granting executable configuration authority.
// Composition needs this one seam because an otherwise valid fragment may
// reference checks in another fragment. DecodeCommands remains the final gate
// for every complete allowlist, including the composed output.
func decodeCommandProfiles(raw []byte) (map[string]CommandSpec, error) {
	if len(raw) > 2<<20 {
		return nil, fmt.Errorf("local command configuration exceeds 2 MiB")
	}
	object, err := decodeToolObject(raw)
	if err != nil {
		return nil, fmt.Errorf("local command configuration must be bounded UTF-8 JSON without duplicate keys")
	}
	for _, value := range object {
		if _, ok := value.(map[string]any); !ok {
			return nil, fmt.Errorf("local command profiles must be non-null JSON objects")
		}
	}
	var specs map[string]CommandSpec
	if err = strictjson.Decode(raw, &specs, 2<<20); err != nil {
		return nil, fmt.Errorf("local command configuration has unknown or invalid fields")
	}
	return specs, nil
}

func commandExecutable(spec CommandSpec) (string, string, error) {
	exe, err := exec.LookPath(spec.Executable)
	if err != nil {
		return "", "", fmt.Errorf("configured executable is not installed")
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return "", "", fmt.Errorf("executable path unavailable")
	}
	exe, err = resolveExecutableTarget(exe)
	if err != nil {
		return "", "", fmt.Errorf("executable target unavailable")
	}
	if strings.EqualFold(filepath.Ext(exe), ".cmd") || strings.EqualFold(filepath.Ext(exe), ".bat") {
		return "", "", fmt.Errorf("shell wrapper is not enabled as a native executable")
	}
	f, err := os.Open(exe)
	if err != nil {
		return "", "", fmt.Errorf("executable cannot be read")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > 1<<30 {
		return "", "", fmt.Errorf("executable is not a bounded regular file")
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", "", fmt.Errorf("executable digest unavailable")
	}
	return exe, hex.EncodeToString(h.Sum(nil)), nil
}

// Pin each interpreter and its reviewed entry. Cursor additionally checks its
// complete package closure; the other existing entry policies remain scoped.
func commandProfileDigest(spec CommandSpec, executableSHA string) (string, error) {
	if err := validateInstalledCommandPins(context.Background(), spec); err != nil {
		return "", err
	}
	if spec.ClaudeSDK != nil || spec.Protocol == tc.ClaudeSDKProtocol {
		configurationSHA, err := ClaudeSDKConfigurationDigest(spec)
		if err != nil {
			return "", err
		}
		return tc.ProfileDigest(struct {
			Command             CommandSpec `json:"command"`
			ConfigurationSHA256 string      `json:"configuration_sha256"`
		}{spec, configurationSHA}, executableSHA)
	}
	if spec.ACP != nil && spec.ACP.Runtime == "cursor" {
		// This function is also the final pre-spawn gate after workspace and
		// network preparation. Node alone cannot attest its imported modules.
		if err := validateACPExecutionSpec(spec); err != nil {
			return "", err
		}
		if _, err := validateCursorPackage(*spec.ACP.CursorPackage); err != nil {
			return "", err
		}
	}
	if spec.CodexAppServer != nil {
		return tc.ProfileDigest(struct {
			Command             CommandSpec `json:"command"`
			ConfigurationSHA256 string      `json:"configuration_sha256"`
		}{spec, CodexConfigurationDigest(*spec.CodexAppServer)}, executableSHA)
	}
	entry, err := teamCLIEntry(spec)
	if err != nil {
		return "", err
	}
	if spec.Adapter == "suma" || spec.Adapter == "opencode" {
		if len(spec.Args) != 1 || !filepath.IsAbs(spec.Args[0]) || (strings.ToLower(filepath.Ext(spec.Args[0])) != ".js" && strings.ToLower(filepath.Ext(spec.Args[0])) != ".mjs") {
			return "", fmt.Errorf("Suma profile requires one absolute JavaScript entry")
		}
		entry = spec.Args[0]
	}
	if spec.Adapter == "octop" {
		if err := validateOctopSpec(spec); err != nil {
			return "", err
		}
		entry = spec.Args[0]
	}
	if entry == "" {
		return tc.ProfileDigest(spec, executableSHA)
	}
	f, err := os.Open(entry)
	if err != nil {
		return "", fmt.Errorf("runtime JavaScript entry is unavailable")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > 16<<20 {
		return "", fmt.Errorf("runtime JavaScript entry is not a bounded regular file")
	}
	h := sha256.New()
	// Bound the read too: a locally changing entry must not turn inventory into
	// an unbounded read after the initial size check.
	if count, err := io.Copy(h, io.LimitReader(f, (16<<20)+1)); err != nil || count > 16<<20 {
		return "", fmt.Errorf("runtime JavaScript entry digest unavailable")
	}
	if spec.OpenCode != nil {
		_, serverSHA, err := openCodeExecutable(spec)
		if err != nil {
			return "", err
		}
		return tc.ProfileDigest(struct {
			Command             CommandSpec `json:"command"`
			EntrySHA256         string      `json:"entry_sha256"`
			ServerSHA256        string      `json:"server_sha256"`
			ConfigurationSHA256 string      `json:"configuration_sha256"`
		}{spec, hex.EncodeToString(h.Sum(nil)), serverSHA, OpenCodeConfigurationDigest(*spec.OpenCode)}, executableSHA)
	}
	return tc.ProfileDigest(struct {
		Command     CommandSpec `json:"command"`
		EntrySHA256 string      `json:"entry_sha256"`
	}{spec, hex.EncodeToString(h.Sum(nil))}, executableSHA)
}

func probeCommand(ctx context.Context, exe string, args []string, spec CommandSpec, secrets []string) (string, error) {
	return probeCommandStreams(ctx, exe, args, spec, secrets, false)
}

// Text help may use either stream. A versioned JSON probe must keep ordinary
// stderr diagnostics separate from its authoritative stdout frame.
func probeCommandStreams(ctx context.Context, exe string, args []string, spec CommandSpec, secrets []string, stdoutOnly bool) (string, error) {
	stage := probeStage(args)
	if ctx.Err() != nil {
		return "", newProbeFailure(stage, "probe_timeout", ctx.Err())
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.Command(exe, args...)
	env, hidden, err := commandEnvironment(spec)
	if err != nil {
		return "", newProbeFailure("source", "configuration_invalid", err)
	}
	cmd.Env = env
	out := newOutput(32<<10, append(append([]string{}, secrets...), hidden...))
	cmd.Stdout = out
	cmd.Stderr = out
	var diagnostic *output
	if stdoutOnly {
		diagnostic = newOutput(32<<10, append(append([]string{}, secrets...), hidden...))
		cmd.Stderr = diagnostic
	}
	err, stopErr := runProbeProcess(ctx, cmd)
	if failure := probeProcessFailure(ctx, stage, err, stopErr, out.truncated || diagnostic != nil && diagnostic.truncated); failure != nil {
		return "", failure
	}
	return out.String(), nil
}

func toolVersion(ctx context.Context, exe string, spec CommandSpec, secrets []string, workspace string) (string, error) {
	if spec.ClaudeSDK != nil {
		return claudeSDKVersion(ctx, spec)
	}
	if spec.ACP != nil {
		return acpVersion(ctx, exe, spec, secrets)
	}
	if spec.Protocol == cc.Protocol {
		return probeCodexVersion(ctx, exe, spec, secrets, workspace)
	}
	if spec.CandidateComposition {
		return probeCompositionGit(ctx, exe)
	}
	if spec.Adapter == "claude" {
		return claudeVersion(ctx, exe, spec, secrets)
	}
	if spec.Adapter == "harness" || spec.Adapter == "codebuddy" {
		return teamCLIVersion(ctx, exe, spec, secrets)
	}
	if spec.Adapter == "command" {
		return "local-command", nil
	}
	if spec.Adapter == "octop" {
		return probeOctop(ctx, exe, spec, secrets, workspace)
	}
	if spec.Adapter == "opencode" {
		return probeOpenCode(ctx, exe, spec, secrets, workspace)
	}
	var versionArgs, helpArgs []string
	switch spec.Adapter {
	case "codex":
		if err := validateCodexCommand(spec); err != nil {
			return "", err
		}
		versionArgs = []string{"--version"}
		helpArgs = []string{"exec", "--help"}
		if spec.Protocol == cc.Protocol {
			helpArgs = []string{"app-server", "--help"}
		}
	case "suma":
		if spec.Input != "agent-jsonl" || len(spec.Args) != 1 || (strings.ToLower(filepath.Ext(spec.Args[0])) != ".js" && strings.ToLower(filepath.Ext(spec.Args[0])) != ".mjs") {
			return "", fmt.Errorf("Suma profile requires one runtime JavaScript entry and agent-jsonl stdin")
		}
		versionArgs = append(append([]string{}, spec.Args...), "--version")
		helpArgs = append(append([]string{}, spec.Args...), "--help")
	default:
		return "", fmt.Errorf("unsupported tool adapter")
	}
	version, err := probeCommandStreams(ctx, exe, versionArgs, spec, secrets, true)
	if err != nil {
		return "", err
	}
	help, err := probeCommand(ctx, exe, helpArgs, spec, secrets)
	if err != nil {
		return "", err
	}
	if spec.Adapter == "codex" && !strings.Contains(version, "codex-cli ") {
		return "", newProbeFailure("version", "version_unsupported", nil)
	}
	if spec.Adapter == "codex" && spec.Protocol != cc.Protocol && (!strings.Contains(help, "--json") || !strings.Contains(help, "--sandbox")) {
		return "", newProbeFailure("help", "help_contract_mismatch", nil)
	}
	if spec.Protocol == cc.Protocol && (!strings.Contains(version, "codex-cli ") || !strings.Contains(help, "--listen")) {
		return "", newProbeFailure("help", "help_contract_mismatch", nil)
	}
	if spec.Adapter == "suma" && (!strings.HasPrefix(version, "suma-agent-runtime ") || !strings.Contains(help, "JSONL") || !strings.Contains(help, "SUMA_AGENT_TOKEN")) {
		return "", newProbeFailure("help", "help_contract_mismatch", nil)
	}
	if spec.NativeWorkspace != nil && spec.NativeWorkspace.ConflictRepair {
		if err = probeNativeConflictRepair(ctx, exe, spec, secrets); err != nil {
			return "", err
		}
	}
	if spec.Planning {
		if err = probePlanning(ctx, exe, spec, secrets); err != nil {
			return "", err
		}
	}
	if spec.KnowledgeExtraction {
		if err = probeKnowledgeExtraction(ctx, exe, spec, secrets); err != nil {
			return "", err
		}
	}
	if spec.FailureDiagnosis {
		if err = probeFailureDiagnosis(ctx, exe, spec, secrets); err != nil {
			return "", err
		}
	}
	if spec.IntelligentOperations {
		if err = probeIntelligentOperations(ctx, exe, spec, secrets); err != nil {
			return "", err
		}
	}
	if spec.TaskConversationReply {
		if err = probeTaskConversationReply(ctx, exe, spec); err != nil {
			return "", err
		}
	}
	if spec.DispatchRecommendation {
		if err = probeDispatchRecommendation(ctx, exe, spec); err != nil {
			return "", err
		}
	}
	version = strings.TrimSpace(strings.SplitN(version, "\n", 2)[0])
	if version == "" || len(version) > 256 {
		return "", newProbeFailure("version", "probe_failed", nil)
	}
	return version, nil
}

func validateCodexCommand(spec CommandSpec) error {
	if spec.Protocol == cc.Protocol {
		return validateCodexAppServerSpec(spec)
	}
	fail := func() error {
		return fmt.Errorf("Codex requires the supported fixed-stdin exec --json preset with matching explicit sandbox; prompt/resume/cwd/config overrides are unavailable")
	}
	if spec.Input != "goal" || len(spec.Args) < 5 || spec.Args[0] != "exec" || spec.Args[len(spec.Args)-1] != "-" {
		return fail()
	}
	seen := map[string]bool{}
	for i := 1; i < len(spec.Args); i++ {
		arg := spec.Args[i]
		if arg == "-" {
			if i != len(spec.Args)-1 {
				return fail()
			}
			continue
		}
		name, value, equal := strings.Cut(arg, "=")
		if name == "-m" {
			name = "--model"
		}
		if seen[name] {
			return fail()
		}
		seen[name] = true
		switch name {
		case "--json", "--ephemeral":
			if equal {
				return fail()
			}
		case "--sandbox", "--color", "--model":
			if !equal {
				i++
				if i >= len(spec.Args)-1 {
					return fail()
				}
				value = spec.Args[i]
			}
			if value == "" || strings.HasPrefix(value, "-") || len(value) > 128 || strings.ContainsAny(value, "\x00\r\n\t ") {
				return fail()
			}
			if name == "--sandbox" {
				want := "workspace-write"
				if spec.ReadOnly {
					want = "read-only"
				}
				if value != want {
					return fail()
				}
			}
			if name == "--color" && value != "never" && value != "always" && value != "auto" {
				return fail()
			}
		default:
			return fail()
		}
	}
	if !seen["--json"] || !seen["--sandbox"] {
		return fail()
	}
	return nil
}
func hasArg(args []string, want string) bool {
	for _, v := range args {
		if v == want {
			return true
		}
	}
	return false
}

// Inventory checks isolation without preparing or creating any workspace.
func inventoryWorkspace(ctx context.Context, cfg ExecConfig, write bool) error {
	workspace := cfg.Workspace
	if workspace == "" {
		workspace = "."
	}
	if _, err := realDirectory(workspace); err != nil {
		return fmt.Errorf("execution workspace is unavailable")
	}
	if !write {
		return nil
	}
	if cfg.Repository == "" || !commitID.MatchString(cfg.BaseCommit) {
		return fmt.Errorf("write commands require an administrator repository and full pinned base commit")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	repo, err := realDirectory(cfg.Repository)
	if err != nil {
		return fmt.Errorf("administrator repository is unavailable")
	}
	top, err := gitText(ctx, repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("administrator repository is not a Git checkout")
	}
	top, err = realDirectory(top)
	if err != nil || !strings.EqualFold(top, repo) {
		return fmt.Errorf("administrator repository must be the checkout root")
	}
	base, err := gitText(ctx, repo, "rev-parse", "--verify", cfg.BaseCommit+"^{commit}")
	if err != nil || base != cfg.BaseCommit {
		return fmt.Errorf("pinned repository base commit is unavailable")
	}
	root := cfg.WorktreeRoot
	if root == "" {
		root = filepath.Join(repo, ".cache", "runner", "worktrees")
	}
	root, err = filepath.Abs(root)
	if err != nil || !within(filepath.Join(repo, ".cache"), root) || strings.EqualFold(root, filepath.Join(repo, ".cache")) {
		return fmt.Errorf("worktree path must be a dedicated repository cache child")
	}
	for p := root; ; p = filepath.Dir(p) {
		st, e := os.Lstat(p)
		if e == nil {
			resolved, e := filepath.EvalSymlinks(p)
			if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(p)) {
				return fmt.Errorf("worktree cache traverses a link or non-directory")
			}
			break
		}
		if !os.IsNotExist(e) || filepath.Dir(p) == p {
			return fmt.Errorf("worktree cache is unavailable")
		}
	}
	if _, err = gitRun(ctx, repo, "check-ignore", "--quiet", root); err != nil {
		return fmt.Errorf("worktree cache must be ignored by Git")
	}
	return nil
}

// DescribeTools performs only local installation and read-only protocol probes.
// Availability never claims model authentication or successful task execution.
func ValidateToolConfiguration(cfg ExecConfig) error {
	if err := validateModelDeployments(cfg.ModelDeployments); err != nil {
		return err
	}
	if len(cfg.Commands) > 128 {
		return fmt.Errorf("at most 128 local command profiles are supported")
	}
	for id, spec := range cfg.Commands {
		spec = normalizedCommand(spec)
		if err := validateInstalledCommandSpec(spec); err != nil {
			return err
		}
		if err := validateTerminalSessionSpec(spec); err != nil {
			return err
		}
		if _, err := localTaskConversationReplyCapability(spec); err != nil {
			return err
		}
		if _, err := localDispatchRecommendationCapability(spec); err != nil {
			return err
		}
		if err := validateTestReportSpec(spec); err != nil {
			return err
		}
		if _, err := localIntelligentOperationsCapability(spec); err != nil {
			return err
		}
		if err := validateOpenCodeSpec(spec); err != nil {
			return err
		}
		if err := validateOctopSpec(spec); err != nil {
			return err
		}
		if err := validateCodexAppServerSpec(spec); err != nil {
			return err
		}
		if err := validateACPExecutionSpec(spec); err != nil {
			return err
		}
		if err := validateModelExecutionSpec(spec); err != nil {
			return err
		}
		if err := validateCompositionCommand(id, spec); err != nil {
			return err
		}
		if _, err := localPlanningCapability(spec); err != nil {
			return err
		}
		if _, err := localKnowledgeExtractionCapability(spec); err != nil {
			return err
		}
		if _, err := localFailureDiagnosisCapability(spec); err != nil {
			return err
		}
		if err := validateTeamCLICommand(spec); err != nil {
			return err
		}
		if err := validateClaudeCommand(spec); err != nil {
			return err
		}
		if err := validateClaudeSDKSpec(spec); err != nil {
			return err
		}
		if err := validateNativeWorkspacePolicy(cfg, id, spec); err != nil {
			return err
		}
		if !tc.ValidCommandIdentity(id, spec.Adapter, spec.Protocol) {
			return fmt.Errorf("local command identity or adapter/protocol is invalid")
		}
	}
	return nil
}
func DescribeTools(ctx context.Context, cfg ExecConfig) ([]tc.Observation, error) {
	return describeTools(ctx, cfg, false)
}

func describeTools(ctx context.Context, cfg ExecConfig, poolArtifacts bool) ([]tc.Observation, error) {
	if err := ValidateToolConfiguration(cfg); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(cfg.Commands))
	for id := range cfg.Commands {
		names = append(names, id)
	}
	sort.Strings(names)
	results := make([]tc.Observation, len(names))
	var readErr error
	var writeErr error
	if cfg.AllowExec {
		readErr = inventoryWorkspace(ctx, cfg, false)
		for _, id := range names {
			if !cfg.Commands[id].ReadOnly {
				writeErr = inventoryWorkspace(ctx, cfg, true)
				break
			}
		}
	}
	sem := make(chan struct{}, 4)
	done := make(chan int, len(names))
	for i, id := range names {
		go func(i int, id string) {
			sem <- struct{}{}
			defer func() { <-sem; done <- i }()
			spec := normalizedCommand(cfg.Commands[id])
			p := tc.Profile{CommandID: id, Adapter: spec.Adapter, Protocol: spec.Protocol, ReadOnly: spec.ReadOnly, ModelExecutionProtocol: spec.ModelExecutionProtocol, TestReportProtocol: testReportProtocol(spec)}
			if spec.ACP != nil && spec.ACP.Runtime == "cursor" {
				p.UserInputProtocol = cu.InputProtocol
			}
			if spec.ClaudeSDK != nil {
				p.UserInputProtocol = cl.InputProtocol
			}
			p.TerminalSessionProtocol = terminalSessionProtocol(spec)
			if spec.TaskConversationReply {
				p.TaskConversationReply = &tc.TaskConversationReplyCapability{Protocol: tc.TaskConversationReplyProtocol}
			}
			if spec.DispatchRecommendation {
				p.DispatchRecommendation = &tc.DispatchRecommendationCapability{Protocol: tc.DispatchRecommendationProtocol}
			}
			if spec.NativeWorkspace != nil && spec.Adapter != "opencode" && spec.Adapter != "octop" {
				p.WorkspaceToolsVersion = NativeWorkspaceVersion
				if spec.NativeWorkspace.ConflictRepair {
					p.ConflictRepairProtocol = "suma.candidate-conflict-repair.v1"
				}
			}
			o := tc.Observation{Profile: p}
			fail := func(reason string) { o.Reason = reason; results[i] = o }
			if !tc.ValidCommandID(id) || tc.ValidateProfile(p, false) != nil {
				fail("command identity or adapter/protocol is invalid")
				return
			}
			if !cfg.AllowExec {
				fail("execution is disabled by the local administrator")
				return
			}
			if readErr != nil {
				fail(readErr.Error())
				return
			}
			if !spec.ReadOnly && writeErr != nil {
				fail(writeErr.Error())
				return
			}
			exe, sha, err := commandExecutable(spec)
			if err != nil {
				fail(err.Error())
				return
			}
			o.Profile.ExecutableSHA256 = sha
			digest, err := effectiveCommandProfileDigest(cfg, spec, sha)
			if err != nil {
				fail(err.Error())
				return
			}
			o.Profile.ProfileSHA256 = digest
			if poolArtifacts {
				o.Profile.ArtifactKind, o.Profile.ArtifactProfileSHA256, err = localArtifactCapability(id, spec)
				if err != nil {
					fail(err.Error())
					return
				}
			}
			if err := probeTerminalSessionSpec(spec); err != nil {
				fail(err.Error())
				return
			}
			version, err := toolVersion(ctx, exe, spec, cfg.Secrets, cfg.Workspace)
			if err != nil {
				fail(err.Error())
				return
			}
			o.Profile.ToolVersion = version
			o.Profile.ACPExecution, err = localACPCapability(spec)
			if err != nil {
				fail(err.Error())
				return
			}
			if spec.ACP != nil && spec.ACP.Runtime == "cursor" {
				o.Profile.UserInputProtocol = cu.InputProtocol
			}
			o.Profile.ClaudeExecution, err = localClaudeCapability(spec)
			if err != nil {
				fail(err.Error())
				return
			}
			o.Profile.ClaudeSDKExecution, err = localClaudeSDKCapability(spec)
			if err != nil {
				fail(err.Error())
				return
			}
			if spec.Adapter == "opencode" {
				o.Profile.OpenCodeExecution, err = localOpenCodeCapability(spec)
				if err != nil {
					fail(err.Error())
					return
				}
				o.Profile.WorkspaceToolsVersion = NativeWorkspaceVersion
			}
			if spec.Adapter == "octop" {
				o.Profile.OctopExecution, err = localOctopCapability(spec)
				if err != nil {
					fail(err.Error())
					return
				}
				o.Profile.WorkspaceToolsVersion = NativeWorkspaceVersion
			}
			if spec.Protocol == cc.Protocol {
				o.CodexCatalog, err = probeCodexCatalog(ctx, exe, spec, cfg)
				if err != nil {
					fail(err.Error())
					return
				}
				if spec.CodexAppServer.EnableUserInput {
					// toolVersion has already proved the enabled native feature
					// through its credential-free, isolated offline listing.
					o.Profile.UserInputProtocol = cc.UserInputProtocol
				}
			}
			if spec.ModelExecutionProtocol == mc.Protocol {
				if err = probeModelExecution(ctx, exe, spec); err != nil {
					fail(err.Error())
					return
				}
				for _, local := range cfg.ModelDeployments {
					o.ModelDeployments = append(o.ModelDeployments, local.Deployment)
				}
			}
			o.Profile.Planning, err = localPlanningCapability(spec)
			if err != nil {
				fail(err.Error())
				return
			}
			o.Profile.KnowledgeExtraction, err = localKnowledgeExtractionCapability(spec)
			if err != nil {
				fail(err.Error())
				return
			}
			o.Profile.FailureDiagnosis, err = localFailureDiagnosisCapability(spec)
			if err != nil {
				fail(err.Error())
				return
			}
			o.Profile.IntelligentOperations, err = localIntelligentOperationsCapability(spec)
			if err != nil {
				fail(err.Error())
				return
			}
			o.Profile.TaskConversationReply, err = localTaskConversationReplyCapability(spec)
			if err != nil {
				fail(err.Error())
				return
			}
			o.Profile.DispatchRecommendation, err = localDispatchRecommendationCapability(spec)
			if err != nil {
				fail(err.Error())
				return
			}
			o.Available = true
			// These declarations follow successful local probes and do not
			// probe a remote connection or change the approved profile.
			if tc.SupportsWorkspaceObservationProfile(o.Profile) {
				o.WorkspaceObservationProtocol = woc.Protocol
			}
			if tc.SupportsWorkspaceWriteProfile(o.Profile) {
				o.WorkspaceWriteProtocol = wwc.Protocol
			}
			o.Reason = "installed executable and protocol probe; model authentication and task success unverified"
			if spec.ACP != nil {
				o.Reason = "reviewed native executable version; ACP session, model selection, login and task success unverified"
			}
			results[i] = o
		}(i, id)
	}
	for range names {
		<-done
	}
	return results, nil
}
func ToolCapabilities(items []tc.Observation) []string {
	caps := []string{}
	for _, o := range items {
		if o.Available && tc.ValidateProfile(o.Profile, true) == nil {
			caps = append(caps, "command:"+o.Profile.CommandID)
		}
	}
	sort.Strings(caps)
	return caps
}

func launchProfile(ctx context.Context, a Assignment, cfg ExecConfig, spec CommandSpec) (tc.Profile, string, error) {
	spec = normalizedCommand(spec)
	if err := validateClaudeSDKSpec(spec); err != nil {
		return tc.Profile{}, "", err
	}
	if err := validateACPExecutionSpec(spec); err != nil {
		return tc.Profile{}, "", err
	}
	if err := probeTerminalSessionSpec(spec); err != nil {
		return tc.Profile{}, "", err
	}
	if _, err := localTaskConversationReplyCapability(spec); err != nil {
		return tc.Profile{}, "", err
	}
	if _, err := localDispatchRecommendationCapability(spec); err != nil {
		return tc.Profile{}, "", err
	}
	if err := validateTestReportSpec(spec); err != nil {
		return tc.Profile{}, "", err
	}
	if _, err := localIntelligentOperationsCapability(spec); err != nil {
		return tc.Profile{}, "", err
	}
	if err := validateOpenCodeSpec(spec); err != nil {
		return tc.Profile{}, "", err
	}
	if err := validateOctopSpec(spec); err != nil {
		return tc.Profile{}, "", err
	}
	if spec.Adapter == "codex" {
		if err := validateCodexCommand(spec); err != nil {
			return tc.Profile{}, "", err
		}
	}
	exe, sha, err := commandExecutable(spec)
	if err != nil {
		return tc.Profile{}, "", err
	}
	digest, err := effectiveCommandProfileDigest(cfg, spec, sha)
	if err != nil {
		return tc.Profile{}, "", fmt.Errorf("profile digest unavailable")
	}
	var artifactKind, artifactHash string
	if a.PoolID != "" {
		artifactKind, artifactHash, err = localArtifactCapability(a.Command[0], spec)
		if err != nil {
			return tc.Profile{}, "", err
		}
	}
	var p tc.Profile
	if cfg.ToolInventory != nil {
		for _, o := range cfg.ToolInventory {
			if o.Profile.CommandID == a.Command[0] && o.Available {
				p = o.Profile
				break
			}
		}
		if p.CommandID == "" || p.ProfileSHA256 != digest || p.ExecutableSHA256 != sha {
			return p, "", fmt.Errorf("local tool profile changed or is unavailable; refresh inventory before execution")
		}
	} else {
		version, e := toolVersion(ctx, exe, spec, cfg.Secrets, cfg.Workspace)
		if e != nil {
			return p, "", e
		}
		p = tc.Profile{CommandID: a.Command[0], Adapter: spec.Adapter, Protocol: spec.Protocol, ProfileSHA256: digest, ExecutableSHA256: sha, ToolVersion: version, ReadOnly: spec.ReadOnly, ModelExecutionProtocol: spec.ModelExecutionProtocol, TestReportProtocol: testReportProtocol(spec), ArtifactKind: artifactKind, ArtifactProfileSHA256: artifactHash}
		p.ACPExecution, err = localACPCapability(spec)
		if err != nil {
			return p, "", err
		}
		if spec.ACP != nil && spec.ACP.Runtime == "cursor" {
			p.UserInputProtocol = cu.InputProtocol
		}
		if spec.ClaudeSDK != nil {
			p.UserInputProtocol = cl.InputProtocol
		}
		p.TerminalSessionProtocol = terminalSessionProtocol(spec)
		p.ClaudeExecution, err = localClaudeCapability(spec)
		if err != nil {
			return p, "", err
		}
		p.ClaudeSDKExecution, err = localClaudeSDKCapability(spec)
		if err != nil {
			return p, "", err
		}
		p.OpenCodeExecution, err = localOpenCodeCapability(spec)
		if err != nil {
			return p, "", err
		}
		p.OctopExecution, err = localOctopCapability(spec)
		if err != nil {
			return p, "", err
		}
		if spec.ModelExecutionProtocol == mc.Protocol {
			if err = probeModelExecution(ctx, exe, spec); err != nil {
				return p, "", err
			}
		}
		if spec.NativeWorkspace != nil {
			p.WorkspaceToolsVersion = NativeWorkspaceVersion
			if spec.NativeWorkspace.ConflictRepair {
				p.ConflictRepairProtocol = "suma.candidate-conflict-repair.v1"
			}
		}
		p.Planning, err = localPlanningCapability(spec)
		if err != nil {
			return p, "", err
		}
		p.KnowledgeExtraction, err = localKnowledgeExtractionCapability(spec)
		if err != nil {
			return p, "", err
		}
		p.FailureDiagnosis, err = localFailureDiagnosisCapability(spec)
		if err != nil {
			return p, "", err
		}
		p.IntelligentOperations, err = localIntelligentOperationsCapability(spec)
		if err != nil {
			return p, "", err
		}
		p.TaskConversationReply, err = localTaskConversationReplyCapability(spec)
		if err != nil {
			return p, "", err
		}
		p.DispatchRecommendation, err = localDispatchRecommendationCapability(spec)
		if err != nil {
			return p, "", err
		}
	}
	if err = tc.ValidateProfile(p, true); err != nil {
		return p, "", fmt.Errorf("invalid tool profile")
	}
	if p.ArtifactKind != artifactKind || p.ArtifactProfileSHA256 != artifactHash {
		return p, "", fmt.Errorf("artifact capability does not match local configuration")
	}
	if p.TestReportProtocol != testReportProtocol(spec) {
		return p, "", fmt.Errorf("test report capability does not match local configuration")
	}
	expectedOpenCode, err := localOpenCodeCapability(spec)
	expectedClaudeSDK, sdkErr := localClaudeSDKCapability(spec)
	if sdkErr != nil || (p.ClaudeSDKExecution == nil) != (expectedClaudeSDK == nil) || p.ClaudeSDKExecution != nil && *p.ClaudeSDKExecution != *expectedClaudeSDK {
		return p, "", fmt.Errorf("Claude SDK fixed configuration differs from local inventory")
	}
	expectedClaude, claudeErr := localClaudeCapability(spec)
	if claudeErr != nil || (p.ClaudeExecution == nil) != (expectedClaude == nil) || p.ClaudeExecution != nil && *p.ClaudeExecution != *expectedClaude {
		return p, "", fmt.Errorf("Claude fixed configuration differs from local inventory")
	}
	if err != nil || (p.OpenCodeExecution == nil) != (expectedOpenCode == nil) || p.OpenCodeExecution != nil && *p.OpenCodeExecution != *expectedOpenCode {
		return p, "", fmt.Errorf("OpenCode fixed configuration differs from local inventory")
	}
	expectedOctop, err := localOctopCapability(spec)
	if err != nil || (p.OctopExecution == nil) != (expectedOctop == nil) || p.OctopExecution != nil && *p.OctopExecution != *expectedOctop {
		return p, "", fmt.Errorf("Octop fixed configuration differs from local inventory")
	}
	expectedACP, err := localACPCapability(spec)
	if err != nil || (p.ACPExecution == nil) != (expectedACP == nil) || p.ACPExecution != nil && *p.ACPExecution != *expectedACP {
		return p, "", fmt.Errorf("ACP fixed configuration differs from local inventory")
	}
	if spec.ACP != nil {
		if spec.ACP.Runtime == "cursor" && p.UserInputProtocol != cu.InputProtocol || spec.ACP.Runtime == "qoder" && p.UserInputProtocol != "" {
			return p, "", fmt.Errorf("ACP input protocol differs from current runtime")
		}
		version, versionErr := acpVersion(ctx, exe, spec, cfg.Secrets)
		if versionErr != nil || version != p.ToolVersion {
			return p, "", fmt.Errorf("ACP executable version changed before launch")
		}
	}
	if p.ModelExecutionProtocol != spec.ModelExecutionProtocol || validateModelExecutionSpec(spec) != nil {
		return p, "", fmt.Errorf("model execution capability does not match local configuration")
	}
	expectedWorkspace := ""
	if spec.NativeWorkspace != nil {
		expectedWorkspace = NativeWorkspaceVersion
	}
	if p.WorkspaceToolsVersion != expectedWorkspace {
		return p, "", fmt.Errorf("workspace tool declaration does not match local configuration")
	}
	expectedRepair := ""
	if spec.NativeWorkspace != nil && spec.NativeWorkspace.ConflictRepair {
		expectedRepair = "suma.candidate-conflict-repair.v1"
	}
	if p.ConflictRepairProtocol != expectedRepair {
		return p, "", fmt.Errorf("conflict repair capability does not match local configuration")
	}
	if p.TerminalSessionProtocol != terminalSessionProtocol(spec) {
		return p, "", fmt.Errorf("Terminal capability differs from local configuration")
	}
	expectedPlanning, err := localPlanningCapability(spec)
	if err != nil || (p.Planning == nil) != (expectedPlanning == nil) || p.Planning != nil && *p.Planning != *expectedPlanning {
		return p, "", fmt.Errorf("planning capability does not match local configuration")
	}
	expectedExtraction, err := localKnowledgeExtractionCapability(spec)
	if err != nil || (p.KnowledgeExtraction == nil) != (expectedExtraction == nil) || p.KnowledgeExtraction != nil && *p.KnowledgeExtraction != *expectedExtraction {
		return p, "", fmt.Errorf("knowledge extraction capability does not match local configuration")
	}
	expectedDiagnosis, err := localFailureDiagnosisCapability(spec)
	if err != nil || (p.FailureDiagnosis == nil) != (expectedDiagnosis == nil) || p.FailureDiagnosis != nil && *p.FailureDiagnosis != *expectedDiagnosis {
		return p, "", fmt.Errorf("failure diagnosis capability does not match local configuration")
	}
	expectedOperations, err := localIntelligentOperationsCapability(spec)
	if err != nil || (p.IntelligentOperations == nil) != (expectedOperations == nil) || p.IntelligentOperations != nil && *p.IntelligentOperations != *expectedOperations {
		return p, "", fmt.Errorf("intelligent operations capability does not match local configuration")
	}
	expectedReply, err := localTaskConversationReplyCapability(spec)
	if err != nil || (p.TaskConversationReply == nil) != (expectedReply == nil) || p.TaskConversationReply != nil && *p.TaskConversationReply != *expectedReply {
		return p, "", fmt.Errorf("task conversation reply capability does not match local configuration")
	}
	expectedRecommendation, err := localDispatchRecommendationCapability(spec)
	if err != nil || (p.DispatchRecommendation == nil) != (expectedRecommendation == nil) || p.DispatchRecommendation != nil && *p.DispatchRecommendation != *expectedRecommendation {
		return p, "", fmt.Errorf("dispatch recommendation capability does not match local configuration")
	}
	if pinned, _ := a.Config["tool_profile_sha256"].(string); pinned != "" && pinned != p.ProfileSHA256 {
		return p, "", fmt.Errorf("assignment tool profile digest does not match local configuration")
	}
	return p, exe, nil
}
