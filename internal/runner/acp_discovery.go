package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	"github.com/ai-mada/suma/internal/acpstdio"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const ACPModelDiscoveryInputLimit = 16 << 10

// Private main-process stdin only. The renderer never selects these paths.
type ACPModelDiscoveryInput struct {
	AdapterID        string `json:"adapter_id"`
	Executable       string `json:"executable"`
	ExecutableSHA256 string `json:"executable_sha256"`
	ReviewedVersion  string `json:"reviewed_version"`
	CredentialEnvRef string `json:"credential_env_ref,omitempty"`
}
type DesktopManagedACPConfiguration struct {
	Executable          string `json:"executable"`
	ExecutableSHA256    string `json:"executable_sha256"`
	ReviewedVersion     string `json:"reviewed_version"`
	Model               string `json:"model"`
	CredentialEnvRef    string `json:"credential_env_ref,omitempty"`
	ConfigurationSHA256 string `json:"configuration_sha256"`
}

func acpDiscoveryCommand(executable, version, model, credential string) CommandSpec {
	return CommandSpec{Adapter: "qoder", Protocol: ac.Protocol, Executable: executable, Args: []string{"--acp"}, Input: "acp-stdio", ACP: &ACPExecutionSpec{Runtime: "qoder", ReviewedVersion: version, Model: model, CredentialEnvRef: credential}}
}
func acpInstallationInput(executable, sha, version, credential string) bool {
	return desktopManagedPath(executable) && tc.ValidDigest(sha) && validateACPExecutionSpec(acpDiscoveryCommand(executable, version, "discovery-only", credential)) == nil
}
func strictACPStrings(raw []byte, required, optional []string) bool {
	if len(raw) == 0 || len(raw) > ACPModelDiscoveryInputLimit {
		return false
	}
	fields, err := decodeToolObject(raw)
	if err != nil {
		return false
	}
	for _, key := range required {
		value, ok := fields[key].(string)
		if !ok || value == "" {
			return false
		}
	}
	for key, value := range fields {
		allowed := false
		for _, name := range append(append([]string{}, required...), optional...) {
			allowed = allowed || key == name
		}
		if !allowed {
			return false
		}
		if text, ok := value.(string); !ok || text == "" {
			return false
		}
	}
	return true
}
func DecodeACPModelDiscoveryInput(raw []byte) (ACPModelDiscoveryInput, error) {
	var input ACPModelDiscoveryInput
	invalid := fmt.Errorf("invalid private ACP model discovery input")
	if !strictACPStrings(raw, []string{"adapter_id", "executable", "executable_sha256", "reviewed_version"}, []string{"credential_env_ref"}) || json.Unmarshal(raw, &input) != nil || input.AdapterID != "qoder-acp" || !acpInstallationInput(input.Executable, input.ExecutableSHA256, input.ReviewedVersion, input.CredentialEnvRef) {
		return ACPModelDiscoveryInput{}, invalid
	}
	return input, nil
}
func validManagedACPConfiguration(value *DesktopManagedACPConfiguration) bool {
	if value == nil || !acpInstallationInput(value.Executable, value.ExecutableSHA256, value.ReviewedVersion, value.CredentialEnvRef) || !acpLocalText(value.Model, 256) {
		return false
	}
	spec := acpDiscoveryCommand(value.Executable, value.ReviewedVersion, value.Model, value.CredentialEnvRef)
	return tc.ValidDigest(value.ConfigurationSHA256) && value.ConfigurationSHA256 == ACPConfigurationDigest(*spec.ACP)
}

// Native Qoder candidates are local, regular files with no reparse/symlink
// ancestor. A shell shim is merely an observation, never executable authority.
func qoderNativeTarget(ctx context.Context, candidate string) (string, string, error) {
	if !desktopManagedPath(candidate) || ctx.Err() != nil {
		return "", "", fmt.Errorf("native ACP installation unavailable")
	}
	if runtime.GOOS == "windows" && (!cursorDiscoveryWindowsDrive(candidate) || cursorDiscoveryWindowsNamespace(candidate)) {
		return "", "", fmt.Errorf("native ACP installation must be local")
	}
	resolved, err := resolveExecutableTarget(candidate)
	if err != nil || !desktopManagedPath(resolved) || runtime.GOOS == "windows" && (!cursorDiscoveryWindowsDrive(resolved) || cursorDiscoveryWindowsNamespace(resolved)) {
		return "", "", fmt.Errorf("native ACP installation target unavailable")
	}
	candidate = resolved
	for current := candidate; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return "", "", fmt.Errorf("native ACP installation path is unavailable")
		}
		if current == candidate {
			if !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 1<<30 || runtime.GOOS == "windows" && !strings.EqualFold(filepath.Ext(current), ".exe") || runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
				return "", "", fmt.Errorf("native ACP entry is unavailable")
			}
		} else if !info.IsDir() {
			return "", "", fmt.Errorf("native ACP parent is unavailable")
		}
		if filepath.Dir(current) == current {
			break
		}
		if ctx.Err() != nil {
			return "", "", ctx.Err()
		}
	}
	file, err := os.Open(candidate)
	if err != nil {
		return "", "", fmt.Errorf("native ACP entry unavailable")
	}
	var magic [4]byte
	_, readErr := io.ReadFull(file, magic[:])
	_ = file.Close()
	native := magic[0] == 'M' && magic[1] == 'Z'
	if runtime.GOOS != "windows" {
		native = native || bytes.Equal(magic[:], []byte{0x7f, 'E', 'L', 'F'}) || bytes.Equal(magic[:], []byte{0xfe, 0xed, 0xfa, 0xce}) || bytes.Equal(magic[:], []byte{0xce, 0xfa, 0xed, 0xfe}) || bytes.Equal(magic[:], []byte{0xfe, 0xed, 0xfa, 0xcf}) || bytes.Equal(magic[:], []byte{0xcf, 0xfa, 0xed, 0xfe}) || bytes.Equal(magic[:], []byte{0xca, 0xfe, 0xba, 0xbe}) || bytes.Equal(magic[:], []byte{0xbe, 0xba, 0xfe, 0xca})
	}
	if readErr != nil || !native {
		return "", "", fmt.Errorf("native ACP entry format unavailable")
	}
	exe, sha, err := commandExecutable(CommandSpec{Executable: candidate})
	if err != nil || !sameACPPath(exe, candidate) {
		return "", "", fmt.Errorf("native ACP installation target changed")
	}
	return exe, sha, nil
}
func sameACPPath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
func sameACPProbeInstallation(ctx context.Context, exe, sha string) bool {
	current, currentSHA, err := qoderNativeTarget(ctx, exe)
	return err == nil && sameACPPath(current, exe) && currentSHA == sha
}
func newACPProbeDirectory() (string, func(), error) {
	base, err := filepath.Abs(os.TempDir())
	if err != nil || filepath.Dir(base) == base {
		return "", nil, fmt.Errorf("ACP probe directory unavailable")
	}
	directory, err := os.MkdirTemp(base, "suma-acp-probe-")
	if err != nil {
		return "", nil, fmt.Errorf("ACP probe directory unavailable")
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return "", nil, fmt.Errorf("ACP probe directory unavailable")
	}
	return directory, func() {
		current, e := os.Lstat(directory)
		if e == nil && os.SameFile(info, current) && current.IsDir() && current.Mode()&os.ModeSymlink == 0 && filepath.Dir(directory) == base {
			_ = os.RemoveAll(directory)
		}
	}, nil
}
func probeQoderNativeVersion(ctx context.Context, exe, directory string, secrets []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.Command(exe, "--version")
	cmd.Dir = directory
	env, _, err := commandEnvironment(CommandSpec{})
	if err != nil {
		return "", fmt.Errorf("ACP version environment unavailable")
	}
	cmd.Env = env
	out, diagnostic := newOutput(32<<10, secrets), newOutput(32<<10, secrets)
	cmd.Stdout, cmd.Stderr = out, diagnostic
	controller, err := startProcess(cmd)
	if err != nil {
		return "", fmt.Errorf("ACP version probe unavailable")
	}
	defer controller.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		_ = controller.Stop()
		waitErr = <-done
	}
	stopErr := controller.Finish()
	out.mu.Lock()
	raw, truncated := out.b.String(), out.truncated
	out.mu.Unlock()
	diagnostic.mu.Lock()
	truncated = truncated || diagnostic.truncated
	diagnostic.mu.Unlock()
	version := strings.TrimSpace(raw)
	if waitErr != nil || stopErr != nil || ctx.Err() != nil || truncated || !acpLocalText(version, 256) || strings.TrimSpace(out.String()) != version {
		return "", fmt.Errorf("ACP version probe was not confirmed")
	}
	return version, nil
}
func checkedACPInstallation(ctx context.Context, input ACPModelDiscoveryInput, directory string) (string, error) {
	exe, sha, err := qoderNativeTarget(ctx, input.Executable)
	if err != nil || sha != input.ExecutableSHA256 {
		return "", fmt.Errorf("ACP installation differs from its reviewed bytes")
	}
	version, err := probeQoderNativeVersion(ctx, exe, directory, nil)
	if err != nil || version != input.ReviewedVersion {
		return "", fmt.Errorf("ACP installation differs from its complete reviewed version")
	}
	current, currentSHA, err := qoderNativeTarget(ctx, exe)
	if err != nil || current != exe || currentSHA != sha {
		return "", fmt.Errorf("ACP installation changed during version verification")
	}
	return exe, nil
}

func discoverLocalQoder(ctx context.Context, paths []string, probe func(context.Context, CommandSpec) (string, error)) InstalledToolDiscoveryItem {
	item := InstalledToolDiscoveryItem{AdapterID: "qoder-acp", Product: "Qoder CLI", ReasonCode: "not_installed"}
	for _, directory := range paths {
		name := "qoder"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		candidate := filepath.Join(directory, name)
		if _, err := os.Lstat(candidate); err == nil {
			item.Installed = true
			item.ReasonCode = "unsupported_launcher"
			exe, sha, err := qoderNativeTarget(ctx, candidate)
			if err != nil {
				continue
			}
			item.ReasonCode = "probe_failed"
			version, err := probe(ctx, CommandSpec{Adapter: "qoder", Protocol: ac.Protocol, Executable: exe})
			if err != nil || !acpLocalText(version, 256) {
				continue
			}
			latest, latestSHA, err := qoderNativeTarget(ctx, exe)
			if err != nil || latest != exe || latestSHA != sha {
				continue
			}
			item.Executable, item.ExecutableSHA256, item.Version, item.ReasonCode = exe, sha, version, "acp_configuration_required"
			return item
		}
		for _, extension := range []string{".cmd", ".bat", ".ps1"} {
			if _, err := os.Lstat(filepath.Join(directory, "qoder"+extension)); err == nil {
				item.Installed = true
				item.ReasonCode = "unsupported_launcher"
			}
		}
	}
	return item
}

// This entry never opens a Journal, Worker or project repository. A catalogue
// becomes adoptable only after the real process and its contained tree stop.
func DiscoverACPModels(parent context.Context, input ACPModelDiscoveryInput) (ac.ACPModelDiscoveryResult, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return ac.ACPModelDiscoveryResult{}, fmt.Errorf("invalid private ACP model discovery input")
	}
	if _, err = DecodeACPModelDiscoveryInput(raw); err != nil {
		return ac.ACPModelDiscoveryResult{}, err
	}
	directory, cleanup, err := newACPProbeDirectory()
	if err != nil {
		return ac.ACPModelDiscoveryResult{}, err
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	exe, err := checkedACPInstallation(ctx, input, directory)
	if err != nil {
		return ac.ACPModelDiscoveryResult{}, err
	}
	spec := acpDiscoveryCommand(exe, input.ReviewedVersion, "discovery-only", input.CredentialEnvRef)
	environment, secrets, err := acpExecutionEnvironment(spec)
	if err != nil {
		return ac.ACPModelDiscoveryResult{}, fmt.Errorf("ACP discovery credential unavailable")
	}
	result := ac.ACPModelDiscoveryResult{Protocol: ac.ModelDiscoveryProtocol, Runtime: "qoder", State: "unknown", ExecutableSHA256: input.ExecutableSHA256, ToolVersion: input.ReviewedVersion, ErrorCode: "acp_discovery_unknown"}
	driver, err := acpstdio.NewModelDiscovery(ctx, acpstdio.DiscoveryOptions{Cwd: directory, Secrets: secrets})
	if err != nil {
		return result, nil
	}
	defer driver.Close()
	cmd := exec.Command(exe, "--acp")
	cmd.Dir, cmd.Env = directory, environment
	cmd.Stdout = driver
	diagnostic := newOutput(64<<10, secrets)
	cmd.Stderr = diagnostic
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return result, nil
	}
	defer stdin.Close()
	controller, err := startProcess(cmd)
	if err != nil {
		return result, nil
	}
	defer controller.Close()
	inputDone := make(chan struct{})
	go func() { defer close(inputDone); _, _ = driver.CopyInput(stdin); _ = stdin.Close() }()
	defer func() { _ = driver.Close(); _ = stdin.Close(); <-inputDone }()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var waitErr error
	interrupted := false
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		interrupted = true
		_ = driver.Close()
		_ = stdin.Close()
		_ = controller.Stop()
		waitErr = <-done
	}
	stopErr := controller.Finish()
	observation := driver.Finish()
	result.AgentName, result.AgentVersion, result.Evidence = observation.AgentName, observation.AgentVersion, observation.Evidence
	result.Evidence.HostProcessStopped = cmd.ProcessState != nil
	result.Evidence.HostProcessTreeStopped = result.Evidence.HostProcessStopped && stopErr == nil
	diagnostic.mu.Lock()
	truncated := diagnostic.truncated
	diagnostic.mu.Unlock()
	if !result.Evidence.HostProcessStopped || !result.Evidence.HostProcessTreeStopped {
		result.ErrorCode = "acp_discovery_not_stopped"
	} else if interrupted || ctx.Err() != nil {
		result.ErrorCode = "acp_discovery_unknown"
	} else if observation.ErrorCode != "" {
		result.ErrorCode = observation.ErrorCode
		if result.ErrorCode != "acp_discovery_unknown" {
			result.State = "failed"
		}
	} else if waitErr != nil || truncated {
		result.State, result.ErrorCode = "failed", "acp_discovery_failed"
	} else if !sameACPProbeInstallation(ctx, exe, input.ExecutableSHA256) {
		result.ErrorCode = "acp_discovery_unknown"
	} else if observation.Catalogue != nil {
		catalogue := *observation.Catalogue
		catalogue.Models = append([]ac.ACPModelChoice{}, catalogue.Models...)
		for index := range catalogue.Models {
			fixed := *spec.ACP
			fixed.Model = catalogue.Models[index].ID
			catalogue.Models[index].ConfigurationSHA256 = ACPConfigurationDigest(fixed)
		}
		result.Catalogue, result.State, result.ErrorCode = &catalogue, "ready", ""
	}
	if result.State != "ready" {
		result.Catalogue = nil
	}
	if ac.ValidateModelDiscoveryResult(result) != nil {
		result.Catalogue = nil
		result.State, result.ErrorCode = "unknown", "acp_discovery_unknown"
		if ac.ValidateModelDiscoveryResult(result) != nil {
			return ac.ACPModelDiscoveryResult{}, fmt.Errorf("ACP discovery result could not be confirmed")
		}
	}
	return result, nil
}

// decode nested private configuration without accepting null, duplicated or
// incorrectly cased fields through permissive encoding/json defaults.
func decodeManagedACPConfiguration(raw []byte) (*DesktopManagedACPConfiguration, error) {
	if !strictACPStrings(raw, []string{"executable", "executable_sha256", "reviewed_version", "model", "configuration_sha256"}, []string{"credential_env_ref"}) {
		return nil, fmt.Errorf("invalid private ACP configuration")
	}
	var value DesktopManagedACPConfiguration
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF || !validManagedACPConfiguration(&value) {
		return nil, fmt.Errorf("invalid private ACP configuration")
	}
	return &value, nil
}
