package runner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func normalizedCommand(spec CommandSpec) CommandSpec {
	if spec.Adapter == "" {
		spec.Adapter = "command"
	}
	if spec.Protocol == "" {
		spec.Protocol = "text-v1"
	}
	return spec
}
func DecodeCommands(raw []byte) (map[string]CommandSpec, error) {
	if len(raw) > 2<<20 {
		return nil, fmt.Errorf("local command configuration exceeds 2 MiB")
	}
	if _, err := decodeToolObject(raw); err != nil {
		return nil, fmt.Errorf("local command configuration must be bounded UTF-8 JSON without duplicate keys")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var specs map[string]CommandSpec
	if err := d.Decode(&specs); err != nil {
		return nil, fmt.Errorf("local command configuration has unknown or invalid fields")
	}
	if err := ValidateToolConfiguration(ExecConfig{Commands: specs}); err != nil {
		return nil, err
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
	exe, err = filepath.EvalSymlinks(exe)
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

// For an interpreted Suma runtime, pin the declared entry's bytes as well as
// the interpreter. This intentionally does not attest all imported modules.
func commandProfileDigest(spec CommandSpec, executableSHA string) (string, error) {
	if spec.Adapter != "suma" {
		return tc.ProfileDigest(spec, executableSHA)
	}
	if len(spec.Args) != 1 || !filepath.IsAbs(spec.Args[0]) || (strings.ToLower(filepath.Ext(spec.Args[0])) != ".js" && strings.ToLower(filepath.Ext(spec.Args[0])) != ".mjs") {
		return "", fmt.Errorf("Suma profile requires one absolute JavaScript entry")
	}
	f, err := os.Open(spec.Args[0])
	if err != nil {
		return "", fmt.Errorf("Suma JavaScript entry is unavailable")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > 16<<20 {
		return "", fmt.Errorf("Suma JavaScript entry is not a bounded regular file")
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", fmt.Errorf("Suma JavaScript entry digest unavailable")
	}
	return tc.ProfileDigest(struct {
		Command     CommandSpec `json:"command"`
		EntrySHA256 string      `json:"entry_sha256"`
	}{spec, hex.EncodeToString(h.Sum(nil))}, executableSHA)
}

func probeCommand(ctx context.Context, exe string, args []string, spec CommandSpec, secrets []string) (string, error) {
	if ctx.Err() != nil {
		return "", fmt.Errorf("read-only CLI probe cancelled")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.Command(exe, args...)
	env, hidden, err := commandEnvironment(spec)
	if err != nil {
		return "", err
	}
	cmd.Env = env
	out := newOutput(32<<10, append(append([]string{}, secrets...), hidden...))
	cmd.Stdout = out
	cmd.Stderr = out
	ctl, err := startProcess(cmd)
	if err != nil {
		return "", fmt.Errorf("read-only CLI probe could not start")
	}
	defer ctl.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = ctl.Stop()
		<-done
		return "", fmt.Errorf("read-only CLI probe timed out")
	}
	if err != nil {
		return "", fmt.Errorf("read-only CLI probe failed")
	}
	if err = ctl.Finish(); err != nil {
		return "", fmt.Errorf("read-only CLI probe descendants unverified")
	}
	if out.truncated {
		return "", fmt.Errorf("read-only CLI probe exceeded 32 KiB")
	}
	return out.String(), nil
}

func toolVersion(ctx context.Context, exe string, spec CommandSpec, secrets []string) (string, error) {
	if spec.Adapter == "command" {
		return "local-command", nil
	}
	if spec.Adapter == "opencode" {
		return "", fmt.Errorf("OpenCode whole-run JSONL terminal is not supported; step_finish is not a run completion")
	}
	var versionArgs, helpArgs []string
	switch spec.Adapter {
	case "codex":
		if err := validateCodexCommand(spec); err != nil {
			return "", err
		}
		versionArgs = []string{"--version"}
		helpArgs = []string{"exec", "--help"}
	case "suma":
		if spec.Input != "agent-jsonl" || len(spec.Args) != 1 || (strings.ToLower(filepath.Ext(spec.Args[0])) != ".js" && strings.ToLower(filepath.Ext(spec.Args[0])) != ".mjs") {
			return "", fmt.Errorf("Suma profile requires one runtime JavaScript entry and agent-jsonl stdin")
		}
		versionArgs = append(append([]string{}, spec.Args...), "--version")
		helpArgs = append(append([]string{}, spec.Args...), "--help")
	default:
		return "", fmt.Errorf("unsupported tool adapter")
	}
	version, err := probeCommand(ctx, exe, versionArgs, spec, secrets)
	if err != nil {
		return "", err
	}
	help, err := probeCommand(ctx, exe, helpArgs, spec, secrets)
	if err != nil {
		return "", err
	}
	if spec.Adapter == "codex" && (!strings.Contains(version, "codex-cli ") || !strings.Contains(help, "--json") || !strings.Contains(help, "--sandbox")) {
		return "", fmt.Errorf("installed Codex version/help does not match JSONL exec protocol")
	}
	if spec.Adapter == "suma" && (!strings.HasPrefix(version, "suma-agent-runtime ") || !strings.Contains(help, "JSONL") || !strings.Contains(help, "SUMA_AGENT_TOKEN")) {
		return "", fmt.Errorf("installed Suma version/help does not match JSONL agent protocol")
	}
	version = strings.TrimSpace(strings.SplitN(version, "\n", 2)[0])
	if version == "" || len(version) > 256 {
		return "", fmt.Errorf("CLI version is missing or oversized")
	}
	return version, nil
}

func validateCodexCommand(spec CommandSpec) error {
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
	if len(cfg.Commands) > 128 {
		return fmt.Errorf("at most 128 local command profiles are supported")
	}
	for id, spec := range cfg.Commands {
		spec = normalizedCommand(spec)
		if err := validateNativeWorkspacePolicy(cfg, id, spec); err != nil {
			return err
		}
		if tc.ValidateProfile(tc.Profile{CommandID: id, Adapter: spec.Adapter, Protocol: spec.Protocol}, false) != nil {
			return fmt.Errorf("local command identity or adapter/protocol is invalid")
		}
	}
	return nil
}
func DescribeTools(ctx context.Context, cfg ExecConfig) ([]tc.Observation, error) {
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
			p := tc.Profile{CommandID: id, Adapter: spec.Adapter, Protocol: spec.Protocol, ReadOnly: spec.ReadOnly}
			if spec.NativeWorkspace != nil {
				p.WorkspaceToolsVersion = NativeWorkspaceVersion
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
			if spec.Adapter == "opencode" {
				fail("OpenCode whole-run JSONL terminal is not supported; step_finish is not a run completion")
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
			version, err := toolVersion(ctx, exe, spec, cfg.Secrets)
			if err != nil {
				fail(err.Error())
				return
			}
			o.Profile.ToolVersion = version
			o.Available = true
			o.Reason = "installed executable and protocol probe; model authentication and task success unverified"
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
		version, e := toolVersion(ctx, exe, spec, cfg.Secrets)
		if e != nil {
			return p, "", e
		}
		p = tc.Profile{CommandID: a.Command[0], Adapter: spec.Adapter, Protocol: spec.Protocol, ProfileSHA256: digest, ExecutableSHA256: sha, ToolVersion: version, ReadOnly: spec.ReadOnly}
		if spec.NativeWorkspace != nil {
			p.WorkspaceToolsVersion = NativeWorkspaceVersion
		}
	}
	if err = tc.ValidateProfile(p, true); err != nil {
		return p, "", fmt.Errorf("invalid tool profile")
	}
	expectedWorkspace := ""
	if spec.NativeWorkspace != nil {
		expectedWorkspace = NativeWorkspaceVersion
	}
	if p.WorkspaceToolsVersion != expectedWorkspace {
		return p, "", fmt.Errorf("workspace tool declaration does not match local configuration")
	}
	if pinned, _ := a.Config["tool_profile_sha256"].(string); pinned != "" && pinned != p.ProfileSHA256 {
		return p, "", fmt.Errorf("assignment tool profile digest does not match local configuration")
	}
	return p, exe, nil
}
