package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const DesktopManagedRunnerProtocol = "suma-desktop-managed-runner-v1"
const DesktopManagedInputLimit = 16 << 10

// DesktopManagedRunnerInput is a private parent/child stdin envelope. It is not
// an HTTP or renderer contract. The token is held in memory, never in arguments,
// inherited environment, commands files or a returned status frame.
type DesktopManagedRunnerInput struct {
	ACPConfiguration *DesktopManagedACPConfiguration `json:"acp_configuration,omitempty"`
	Protocol         string                          `json:"protocol"`
	ControlURL       string                          `json:"control_url"`
	ProjectID        string                          `json:"project_id"`
	WorkerID         string                          `json:"worker_id"`
	AdapterID        string                          `json:"adapter_id"`
	Repository       string                          `json:"repository"`
	BaseCommit       string                          `json:"base_commit"`
	StateDirectory   string                          `json:"state_directory"`
	Token            string                          `json:"token"`
}

func desktopManagedHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return false
		}
	}
	return true
}

func desktopManagedPath(value string) bool {
	if !filepath.IsAbs(value) || len(value) > 4096 || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || character == '\u2028' || character == '\u2029' {
			return false
		}
	}
	return true
}

func (input DesktopManagedRunnerInput) validate() error {
	invalid := fmt.Errorf("invalid private desktop Runner configuration")
	if input.Protocol != DesktopManagedRunnerProtocol || !desktopManagedHex(input.ProjectID, 32) || !strings.HasPrefix(input.WorkerID, "desktop-") || !desktopManagedHex(strings.TrimPrefix(input.WorkerID, "desktop-"), 32) || !desktopManagedHex(input.Token, 64) || !commitID.MatchString(input.BaseCommit) || !desktopManagedPath(input.Repository) || !desktopManagedPath(input.StateDirectory) {
		return invalid
	}
	if input.AdapterID != "codex-jsonl" && input.AdapterID != "codebuddy-cli" && input.AdapterID != "dsh-multica" && input.AdapterID != "claude-code-cli" && input.AdapterID != "qoder-acp" {
		return invalid
	}
	if input.AdapterID == "qoder-acp" {
		if !validManagedACPConfiguration(input.ACPConfiguration) {
			return invalid
		}
	} else if input.ACPConfiguration != nil {
		return invalid
	}
	u, err := url.Parse(input.ControlURL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.String() != input.ControlURL {
		return invalid
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1024 || port > 65535 || u.Host != "127.0.0.1:"+strconv.Itoa(port) {
		return invalid
	}
	return nil
}

func DecodeDesktopManagedRunnerInput(raw []byte) (DesktopManagedRunnerInput, error) {
	var input DesktopManagedRunnerInput
	invalid := fmt.Errorf("invalid private desktop Runner input")
	if len(raw) == 0 || len(raw) > DesktopManagedInputLimit {
		return input, invalid
	}
	fields, err := decodeToolObject(raw)
	if err != nil || len(fields) < 9 || len(fields) > 10 {
		return input, invalid
	}
	for _, key := range []string{"protocol", "control_url", "project_id", "worker_id", "adapter_id", "repository", "base_commit", "state_directory", "token"} {
		if _, ok := fields[key]; !ok {
			return input, invalid
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || input.validate() != nil {
		return DesktopManagedRunnerInput{}, invalid
	}
	if value, present := fields["acp_configuration"]; present {
		if input.AdapterID != "qoder-acp" || value == nil {
			return DesktopManagedRunnerInput{}, invalid
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return DesktopManagedRunnerInput{}, invalid
		}
		configuration, err := decodeManagedACPConfiguration(raw)
		if err != nil {
			return DesktopManagedRunnerInput{}, invalid
		}
		input.ACPConfiguration = configuration
	} else if input.AdapterID == "qoder-acp" {
		return DesktopManagedRunnerInput{}, invalid
	}
	return input, nil
}

func prepareDesktopManagedExecution(ctx context.Context, input DesktopManagedRunnerInput, discover func(context.Context, string) (InstalledToolDiscovery, error)) (ExecConfig, error) {
	if err := input.validate(); err != nil {
		return ExecConfig{}, err
	}
	if ctx.Err() != nil {
		return ExecConfig{}, ctx.Err()
	}
	cfg := ExecConfig{AllowExec: true, Workspace: input.Repository, Repository: input.Repository, BaseCommit: input.BaseCommit, WorktreeRoot: filepath.Join(input.Repository, ".cache", "runner", "worktrees"), Secrets: []string{input.Token}}
	// Validate the real immutable Git workspace before starting any CLI. This
	// reads repository facts only: it neither creates a checkout nor opens state.
	if err := inventoryWorkspace(ctx, cfg, true); err != nil {
		return ExecConfig{}, fmt.Errorf("desktop Runner needs the bound Git root, pinned commit and ignored worktree directory")
	}
	if input.AdapterID == "qoder-acp" {
		configuration := input.ACPConfiguration
		directory, cleanup, err := newACPProbeDirectory()
		if err != nil {
			return ExecConfig{}, err
		}
		defer cleanup()
		exe, err := checkedACPInstallation(ctx, ACPModelDiscoveryInput{AdapterID: "qoder-acp", Executable: configuration.Executable, ExecutableSHA256: configuration.ExecutableSHA256, ReviewedVersion: configuration.ReviewedVersion, CredentialEnvRef: configuration.CredentialEnvRef}, directory)
		if err != nil {
			return ExecConfig{}, err
		}
		spec := acpDiscoveryCommand(exe, configuration.ReviewedVersion, configuration.Model, configuration.CredentialEnvRef)
		if ACPConfigurationDigest(*spec.ACP) != configuration.ConfigurationSHA256 {
			return ExecConfig{}, fmt.Errorf("desktop ACP configuration differs from the confirmed model")
		}
		cfg.Commands = map[string]CommandSpec{"desktop-qoder-acp": spec}
		if ValidateToolConfiguration(cfg) != nil {
			return ExecConfig{}, fmt.Errorf("desktop ACP configuration unavailable")
		}
		return cfg, nil
	}
	report, err := discover(ctx, input.AdapterID)
	if err != nil || report.Protocol != InstalledToolDiscoveryProtocol || len(report.Items) != 1 {
		return ExecConfig{}, fmt.Errorf("desktop development tool could not be discovered")
	}
	item := report.Items[0]
	if item.AdapterID != input.AdapterID || !item.Installed || !item.LaunchSupported || item.Command == nil || item.Command.ReadOnly {
		return ExecConfig{}, fmt.Errorf("desktop development tool is not installed or cannot use its fixed launch policy")
	}
	cfg.Commands = map[string]CommandSpec{"desktop-" + input.AdapterID: *item.Command}
	if err := ValidateToolConfiguration(cfg); err != nil {
		return ExecConfig{}, fmt.Errorf("desktop development tool configuration is unavailable")
	}
	return cfg, nil
}

// RunDesktopManagedRunner uses the same Worker, durable claims, fencing,
// inventory, process containment and result journal as a separately deployed
// Runner. The desktop is a lifecycle owner, never a second scheduler.
func RunDesktopManagedRunner(ctx context.Context, input DesktopManagedRunnerInput) error {
	cfg, err := prepareDesktopManagedExecution(ctx, input, DiscoverInstalledTools)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	journal, err := OpenJournal(input.StateDirectory)
	if err != nil {
		return fmt.Errorf("desktop Runner state is unavailable; preserve it for reconciliation")
	}
	defer journal.Close()
	worker := Worker{Client: &Client{BaseURL: input.ControlURL, Token: input.Token, ProjectID: input.ProjectID}, Journal: journal, ID: input.WorkerID, Exec: cfg}
	return worker.Run(ctx)
}
