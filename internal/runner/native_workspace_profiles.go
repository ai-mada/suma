package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type nativeRuntimeManifest struct {
	SchemaVersion   int               `json:"schema_version"`
	Entry           string            `json:"entry"`
	EntrySHA256     string            `json:"entry_sha256"`
	ExternalImports []string          `json:"external_imports"`
	Dependencies    map[string]string `json:"dependencies"`
}

func validateNativeWorkspacePolicy(cfg ExecConfig, id string, spec CommandSpec) error {
	if spec.NativeWorkspace == nil {
		return nil
	}
	policy := spec.NativeWorkspace
	if spec.Adapter != "suma" || spec.Protocol != "suma-jsonl-v1" || spec.ReadOnly || spec.Input != "agent-jsonl" || spec.ArtifactInput || spec.ArtifactOutput != "" || spec.TestEvidence || len(policy.Checks) > 16 || !filepath.IsAbs(policy.RuntimeManifest) {
		return fmt.Errorf("native workspace requires an isolated Suma agent, a runtime manifest and bounded fixed checks")
	}
	seen := map[string]bool{}
	for _, checkID := range policy.Checks {
		check, ok := cfg.Commands[checkID]
		check = normalizedCommand(check)
		if !tc.ValidCommandID(checkID) || checkID == id || seen[checkID] || !ok || check.Adapter != "command" || check.Protocol != "text-v1" || check.NativeWorkspace != nil || check.Input != "" || check.ArtifactInput || check.ArtifactOutput != "" {
			return fmt.Errorf("native workspace checks must reference distinct fixed text command profiles")
		}
		seen[checkID] = true
	}
	return nil
}

// The bundled entry closes the JS/SDK dependency graph. A module entry alone
// cannot prove that a later dependency import still matches the inventory.
func nativeManifestDigest(spec CommandSpec) (string, error) {
	if len(spec.Args) != 1 || strings.ToLower(filepath.Ext(spec.Args[0])) != ".mjs" || !filepath.IsAbs(spec.Args[0]) {
		return "", fmt.Errorf("native workspace requires one bundled ESM entry")
	}
	path := spec.NativeWorkspace.RuntimeManifest
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || st.Size() > 256<<10 {
		return "", fmt.Errorf("native runtime manifest is unavailable or invalid")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("native runtime manifest is unavailable")
	}
	if _, err = decodeToolObject(raw); err != nil {
		return "", fmt.Errorf("native runtime manifest is invalid")
	}
	var manifest nativeRuntimeManifest
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&manifest) != nil || manifest.SchemaVersion != 1 || manifest.Entry != filepath.Base(spec.Args[0]) || filepath.Dir(path) != filepath.Dir(spec.Args[0]) || !evidenceHash.MatchString(manifest.EntrySHA256) || len(manifest.ExternalImports) > 128 || len(manifest.Dependencies) > 128 {
		return "", fmt.Errorf("native runtime manifest does not bind the configured entry")
	}
	for _, name := range manifest.ExternalImports {
		if !strings.HasPrefix(name, "node:") || len(name) > 128 || strings.ContainsAny(name, "\x00\r\n") {
			return "", fmt.Errorf("native runtime bundle contains a non-Node external dependency")
		}
	}
	if manifest.Dependencies["@modelcontextprotocol/client"] == "" {
		return "", fmt.Errorf("native runtime manifest is missing its official MCP client version")
	}
	for name, version := range manifest.Dependencies {
		if name == "" || version == "" || len(name) > 256 || len(version) > 100 || strings.ContainsAny(name+version, "\x00\r\n") {
			return "", fmt.Errorf("native runtime dependency declaration is invalid")
		}
	}
	entry, err := os.Lstat(spec.Args[0])
	if err != nil || !entry.Mode().IsRegular() || entry.Mode()&os.ModeSymlink != 0 || entry.Size() > 16<<20 {
		return "", fmt.Errorf("native runtime bundle is unavailable or invalid")
	}
	data, err := os.ReadFile(spec.Args[0])
	if err != nil || digest(data) != manifest.EntrySHA256 {
		return "", fmt.Errorf("native runtime bundle does not match its manifest")
	}
	return digest(raw), nil
}

func effectiveCommandProfileDigest(cfg ExecConfig, spec CommandSpec, executableSHA string) (string, error) {
	base, err := commandProfileDigest(spec, executableSHA)
	if err != nil || spec.NativeWorkspace == nil {
		return base, err
	}
	if err = validateNativeWorkspacePolicy(cfg, "", spec); err != nil {
		return "", err
	}
	manifest, err := nativeManifestDigest(spec)
	if err != nil {
		return "", err
	}
	_, gitSHA, err := commandExecutable(CommandSpec{Executable: "git"})
	if err != nil {
		return "", fmt.Errorf("native workspace Git executable is unavailable")
	}
	checks := map[string]string{}
	for _, id := range spec.NativeWorkspace.Checks {
		check := normalizedCommand(cfg.Commands[id])
		_, sha, e := commandExecutable(check)
		if e != nil {
			return "", fmt.Errorf("native check executable is unavailable")
		}
		pin, e := commandProfileDigest(check, sha)
		if e != nil {
			return "", fmt.Errorf("native check profile cannot be pinned")
		}
		checks[id] = pin
	}
	data, err := json.Marshal(struct {
		Version, Base, Manifest, Git string
		Checks                       map[string]string
	}{NativeWorkspaceVersion, base, manifest, gitSHA, checks})
	if err != nil {
		return "", err
	}
	return digest(data), nil
}

// Checks run in the existing attempt, with the same process-tree controller
// and secret filtering. They never call Execute or manufacture formal evidence.
func runPinnedNativeCheck(ctx context.Context, cfg ExecConfig, workspace *attemptWorkspace, parent CommandSpec, parentSHA, id string) (NativeWorkspaceCheckResult, error) {
	result := NativeWorkspaceCheckResult{CheckID: id, ExitCode: -1}
	if workspace == nil || parent.NativeWorkspace == nil || ctx.Err() != nil {
		return result, fmt.Errorf("native check scope is unavailable")
	}
	allowed := false
	for _, name := range parent.NativeWorkspace.Checks {
		if name == id {
			allowed = true
		}
	}
	if !allowed {
		return result, fmt.Errorf("native check is not allowed")
	}
	_, parentExecutableSHA, err := commandExecutable(parent)
	if err != nil {
		return result, fmt.Errorf("native parent executable is unavailable")
	}
	current, err := effectiveCommandProfileDigest(cfg, parent, parentExecutableSHA)
	if err != nil || current != parentSHA {
		return result, fmt.Errorf("native profile changed before check")
	}
	spec := normalizedCommand(cfg.Commands[id])
	exe, sha, err := commandExecutable(spec)
	if err != nil {
		return result, fmt.Errorf("native check executable is unavailable")
	}
	result.ProfileSHA256, err = commandProfileDigest(spec, sha)
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.Command(exe, spec.Args...)
	cmd.Dir = workspace.Source
	var secrets []string
	cmd.Env, secrets, err = commandEnvironment(spec)
	if err != nil {
		return result, fmt.Errorf("native check environment is unavailable")
	}
	temp := filepath.Join(workspace.Directory, "tmp", "native-checks", id)
	if err = safeMkdir(temp); err != nil {
		return result, fmt.Errorf("native check temporary directory is unavailable")
	}
	filtered := cmd.Env[:0]
	for _, value := range cmd.Env {
		upper := strings.ToUpper(value)
		if !strings.HasPrefix(upper, "TEMP=") && !strings.HasPrefix(upper, "TMP=") && !strings.HasPrefix(upper, "TMPDIR=") && !strings.HasPrefix(upper, "GOCACHE=") && !strings.HasPrefix(upper, "GOTMPDIR=") {
			filtered = append(filtered, value)
		}
	}
	cmd.Env = append(filtered, "TEMP="+temp, "TMP="+temp, "TMPDIR="+temp, "GOCACHE="+filepath.Join(temp, "go-build"), "GOTMPDIR="+temp)
	output := newOutput(32<<10, append(append([]string{}, cfg.Secrets...), secrets...))
	cmd.Stdout = output
	cmd.Stderr = output
	controller, err := startProcess(cmd)
	if err != nil {
		return result, fmt.Errorf("native check process could not start")
	}
	defer controller.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
		if err == nil {
			result.ExitCode = 0
		} else if cmd.ProcessState != nil {
			result.ExitCode = cmd.ProcessState.ExitCode()
		}
	case <-ctx.Done():
		stopErr := controller.Stop()
		<-done
		result.Stopped = stopErr == nil
		result.OutcomeUnknown = stopErr != nil
	}
	if controller.Finish() != nil {
		result.OutcomeUnknown = true
		result.ExitCode = -1
	}
	result.Output = output.String()
	result.OutputTruncated = output.truncated
	return result, nil
}

func nativeCheckIDs(policy *NativeWorkspacePolicy) []string {
	if policy == nil {
		return nil
	}
	ids := append([]string{}, policy.Checks...)
	sort.Strings(ids)
	return ids
}
