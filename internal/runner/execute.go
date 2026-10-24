package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ai-mada/suma/internal/releasecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
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
	Adapter        string            `json:"adapter,omitempty"`
	Protocol       string            `json:"protocol,omitempty"`
	Executable     string            `json:"executable"`
	Args           []string          `json:"args"`
	Env            map[string]string `json:"env,omitempty"`
	EnvFrom        []string          `json:"env_from,omitempty"`
	Input          string            `json:"input,omitempty"`
	ReadOnly       bool              `json:"read_only,omitempty"`
	TestEvidence   bool              `json:"test_evidence,omitempty"`
	ArtifactOutput string            `json:"artifact_output,omitempty"`
	ArtifactInput  bool              `json:"artifact_input,omitempty"`
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
	ToolInventory                        []tc.Observation
	Artifacts                            *Client
	Journal                              *Journal
	resolvedArtifact                     *releasecontract.ArtifactBinding
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
	fail := func(e string) Result { r.Error = e; return r }
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
		workspace, err = prepareWorkspace(ctx, a, cfg, spec)
		if err != nil {
			return fail(err.Error())
		}
		root = workspace.Source
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
	cmd := exec.Command(executable, spec.Args...)
	cmd.Dir = root
	if a.Type == "agent" {
		goal, _ := a.Config["goal"].(string)
		if strings.TrimSpace(goal) == "" || len(goal) > 32000 {
			return fail("agent config.goal must contain 1–32000 characters")
		}
		switch spec.Input {
		case "agent-jsonl":
			b, _ := json.Marshal(map[string]any{"goal": goal, "request_id": digest([]byte(key(a)))})
			cmd.Stdin = bytes.NewReader(append(b, '\n'))
		case "goal":
			cmd.Stdin = strings.NewReader(goal + "\n")
		default:
			return fail("agent command specification requires input agent-jsonl or goal")
		}
	}
	var environmentSecrets []string
	cmd.Env, environmentSecrets, err = commandEnvironment(spec)
	if err != nil {
		return fail(err.Error())
	}
	cfg.Secrets = append(cfg.Secrets, environmentSecrets...)
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
		}
		stream = newToolStream(profile, cfg.Secrets, emit, digest([]byte(key(a))))
		stream.abort = cancel
		cmd.Stdout = stream
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
	latestDigest, digestErr := commandProfileDigest(spec, latestSHA)
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
		if workspace != nil {
			r.WorkspaceEvidence, _ = workspace.capture(context.Background(), r)
		}
		return fail(fmt.Sprintf("process start: %v", err))
	}
	defer controller.Close()
	done := make(chan error, 1)
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
		stopErr := controller.Stop()
		waitErr := <-done
		r.Stopped = stopErr == nil
		r.OutcomeUnknown = stopErr != nil
		r.Error = ctx.Err().Error()
		if stopErr != nil {
			r.Error += "; process tree termination unverified"
		}
		_ = waitErr
	}
	if err = controller.Finish(); err != nil {
		r.ExitCode = -1
		r.OutcomeUnknown = true
		r.Error = "process tree final state unverified"
	}
	r.Output = out.String()
	if stream != nil {
		invalidBeforeFinish := stream.bad != nil
		evidence, streamErr := stream.finish()
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
		if pinned, _ := a.Config["tool_profile_sha256"].(string); pinned != "" {
			r.ToolEvidence = evidence
		}
		r.Output = stream.output.String() + r.Output
	} else if pinned, _ := a.Config["tool_profile_sha256"].(string); pinned != "" {
		terminal := "failed"
		if r.ExitCode == 0 && !r.Stopped && !r.OutcomeUnknown {
			terminal = "completed"
		}
		if r.OutcomeUnknown || r.Stopped {
			terminal = "unknown"
		}
		r.ToolEvidence = &tc.Evidence{Profile: profile, Terminal: terminal}
	}
	if err = out.flushProgress(); err != nil {
		r.ExitCode = -1
		r.Error = "durable progress unavailable: " + err.Error()
	}
	if workspace != nil {
		var captureErr error
		r.WorkspaceEvidence, captureErr = workspace.capture(context.Background(), r)
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
	}
	if r.ToolEvidence != nil {
		if r.OutcomeUnknown || r.Stopped {
			r.ToolEvidence.Terminal = "unknown"
		} else if r.ExitCode != 0 || r.Error != "" {
			r.ToolEvidence.Terminal = "failed"
		}
	}
	return r
}
