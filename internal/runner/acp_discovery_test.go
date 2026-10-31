package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestQoderNativeDiscoveryRequiresExactVersionAndStableBytes(t *testing.T) {
	root := t.TempDir()
	name := "qoder"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	file := localDiscoveryFile(t, filepath.Join(root, name), "MZowned native fixture; injected probe, never executed")
	calls := 0
	result, err := discoverInstalledTools(context.Background(), "qoder-acp", []string{root}, func(context.Context, CommandSpec) (string, error) { calls++; return "Qoder full exact version171", nil })
	if err != nil || len(result.Items) != 1 || !result.Items[0].Installed || result.Items[0].LaunchSupported || result.Items[0].Command != nil || result.Items[0].ReasonCode != "acp_configuration_required" || result.Items[0].Version != "Qoder full exact version171" || calls != 1 {
		t.Fatal("native observation did not remain configuration-only", err)
	}
	changed, err := discoverInstalledTools(context.Background(), "qoder-acp", []string{root}, func(context.Context, CommandSpec) (string, error) {
		if e := os.WriteFile(file, []byte("changed during probe"), 0700); e != nil {
			t.Fatal(e)
		}
		return "Qoder full exact version171", nil
	})
	if err != nil || changed.Items[0].ReasonCode != "probe_failed" || changed.Items[0].Version != "" {
		t.Fatal("changed executable was retained as reviewed installation", err)
	}
}

func TestQoderResolvedNativeTargetAndPostProbePinRejectReplacement(t *testing.T) {
	root := t.TempDir()
	name := "native"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	target := localDiscoveryFile(t, filepath.Join(root, name), "MZowned native target; structural test never executes")
	resolved, sha, err := qoderNativeTarget(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if !sameACPProbeInstallation(context.Background(), resolved, sha) {
		t.Fatal("stable native target rejected")
	}
	link := filepath.Join(root, "current")
	if runtime.GOOS == "windows" {
		link += ".exe"
	}
	if err = os.Symlink(target, link); err == nil {
		actual, hash, e := qoderNativeTarget(context.Background(), link)
		if e != nil || !sameACPPath(actual, resolved) || hash != sha {
			t.Fatal("versioned native link did not resolve to pinned bytes", e)
		}
	} else {
		t.Log("symlink creation unavailable; remaining source-pin checks still execute")
	}
	if err = os.WriteFile(target, []byte("MZreplacement native target"), 0700); err != nil {
		t.Fatal(err)
	}
	if sameACPProbeInstallation(context.Background(), resolved, sha) {
		t.Fatal("post-probe changed installation retained a ready catalogue")
	}
	script := localDiscoveryFile(t, filepath.Join(root, "shell"), "#!/bin/sh\nexit 0\n")
	if _, _, err = qoderNativeTarget(context.Background(), script); err == nil {
		t.Fatal("shell launcher accepted as native")
	}
	if runtime.GOOS != "windows" && sameACPPath("/A", "/a") {
		t.Fatal("POSIX source comparison became case-insensitive")
	}
}

func discovery171Input(t *testing.T) ACPModelDiscoveryInput {
	t.Helper()
	exe, _ := os.Executable()
	resolved, sha, err := commandExecutable(CommandSpec{Executable: exe})
	if err != nil {
		t.Fatal(err)
	}
	return ACPModelDiscoveryInput{AdapterID: "qoder-acp", Executable: resolved, ExecutableSHA256: sha, ReviewedVersion: "Qoder fixture protocol only 170"}
}

func TestACPDiscoveryPrivateInputRejectsUnknownNullDuplicateAndProtectedSecrets(t *testing.T) {
	input := discovery171Input(t)
	raw, _ := json.Marshal(input)
	if _, err := DecodeACPModelDiscoveryInput(raw); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"adapter_id":"qoder-acp"}`)...), append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"args":["--yolo"]}`)...), append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"credential_env_ref":null}`)...), []byte(strings.Replace(string(raw), `"adapter_id"`, `"Adapter_ID"`, 1)), []byte(`null`), []byte(strings.Repeat("x", ACPModelDiscoveryInputLimit+1))} {
		if _, err := DecodeACPModelDiscoveryInput(bad); err == nil {
			t.Fatal("private discovery accepted altered input")
		}
	}
	for _, name := range []string{"SUMA_RUNNER_TOKEN", "CODEX_API_KEY", "OPENAI_API_KEY", "HOME", "PATH", "lowercase"} {
		changed := input
		changed.CredentialEnvRef = name
		raw, _ := json.Marshal(changed)
		if _, err := DecodeACPModelDiscoveryInput(raw); err == nil {
			t.Fatal("protected credential reference accepted")
		}
	}
}

func managed171Input(t *testing.T) DesktopManagedRunnerInput {
	input := desktopManagedFixtureInput(t)
	installation := discovery171Input(t)
	input.AdapterID = "qoder-acp"
	spec := acpDiscoveryCommand(installation.Executable, installation.ReviewedVersion, "primary171", "")
	input.ACPConfiguration = &DesktopManagedACPConfiguration{Executable: installation.Executable, ExecutableSHA256: installation.ExecutableSHA256, ReviewedVersion: installation.ReviewedVersion, Model: "primary171", ConfigurationSHA256: ACPConfigurationDigest(*spec.ACP)}
	return input
}

func TestDesktopManagedACPInputBindsOnlyExactSelectedConfiguration(t *testing.T) {
	input := managed171Input(t)
	raw, _ := json.Marshal(input)
	decoded, err := DecodeDesktopManagedRunnerInput(raw)
	if err != nil || decoded.ACPConfiguration.Model != "primary171" {
		t.Fatal("confirmed private ACP configuration rejected", err)
	}
	for _, change := range []func(*DesktopManagedRunnerInput){func(v *DesktopManagedRunnerInput) { v.AdapterID = "codex-jsonl" }, func(v *DesktopManagedRunnerInput) { v.ACPConfiguration.Model = "another" }, func(v *DesktopManagedRunnerInput) { v.ACPConfiguration = nil }, func(v *DesktopManagedRunnerInput) { v.ACPConfiguration.Executable = "relative" }} {
		value := managed171Input(t)
		change(&value)
		raw, _ := json.Marshal(value)
		if _, err := DecodeDesktopManagedRunnerInput(raw); err == nil {
			t.Fatal("managed configuration lost its exact selected source")
		}
	}
	raw, _ = json.Marshal(desktopManagedFixtureInput(t))
	raw = append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"acp_configuration":null}`)...)
	if _, err := DecodeDesktopManagedRunnerInput(raw); err == nil {
		t.Fatal("another adapter accepted a null ACP extension")
	}
}

func TestDesktopManagedACPRechecksInstallationWithoutRediscoveringOrOpeningJournal(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo, base := gitFixture(t)
	input := managed171Input(t)
	input.Repository, input.BaseCommit = repo, base
	discover := func(context.Context, string) (InstalledToolDiscovery, error) {
		t.Fatal("confirmed model caused a second discovery session")
		return InstalledToolDiscovery{}, nil
	}
	cfg, err := prepareDesktopManagedExecution(context.Background(), input, discover)
	if err != nil {
		t.Fatal(err)
	}
	spec := cfg.Commands["desktop-qoder-acp"]
	if spec.ACP == nil || spec.ACP.Model != "primary171" || ACPConfigurationDigest(*spec.ACP) != input.ACPConfiguration.ConfigurationSHA256 {
		t.Fatal("confirmed fixed model changed")
	}
	if _, err = os.Stat(input.StateDirectory); !os.IsNotExist(err) {
		t.Fatal("configuration preparation opened worker journal")
	}
	input.ACPConfiguration.ExecutableSHA256 = strings.Repeat("a", 64)
	if _, err = prepareDesktopManagedExecution(context.Background(), input, discover); err == nil {
		t.Fatal("changed executable identity reached a worker")
	}
}

func TestACPModelDiscoveryProcessHasNoWorkAndBindsEachChoice(t *testing.T) {
	input := discovery171Input(t)
	input.CredentialEnvRef = "LOCAL_QODER_DISCOVERY171"
	t.Setenv(input.CredentialEnvRef, "controlled-discovery171-marker")
	t.Setenv("SUMA_RUNNER_TOKEN", "platform-token-must-not-reach-child")
	result, err := DiscoverACPModels(context.Background(), input)
	if err != nil || result.State != "ready" || result.Catalogue == nil {
		t.Fatalf("controlled zero-work process discovery failed: %v state=%s code=%s", err, result.State, result.ErrorCode)
	}
	if !result.Evidence.HostProcessStopped || !result.Evidence.HostProcessTreeStopped || !result.Evidence.SessionCreated || result.Evidence.PromptRequests != 0 || result.Evidence.ConfigurationWrites != 0 || result.Evidence.PermissionRequests != 0 || result.Evidence.UnexpectedActivity != 0 || result.Evidence.SessionCloseRequested {
		t.Fatal("discovery confused catalogue with work or host stop")
	}
	if len(result.Catalogue.Models) != 2 || result.Catalogue.Models[1].ID != "secondary171" {
		t.Fatal("exact catalogue choices were lost")
	}
	for _, model := range result.Catalogue.Models {
		spec := acpDiscoveryCommand(input.Executable, input.ReviewedVersion, model.ID, input.CredentialEnvRef)
		if model.ConfigurationSHA256 != ACPConfigurationDigest(*spec.ACP) {
			t.Fatal("model is not bound to the actual reviewed configuration")
		}
	}
	raw, _ := json.Marshal(result)
	for _, secret := range []string{input.Executable, input.CredentialEnvRef, "controlled-discovery171-marker", "private-discovery171-session"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("private process/session/credential escaped public discovery")
		}
	}
}

// Selected only by an explicit fake credential reference in this new test.
// Every forbidden work/auth RPC exits unsuccessfully, rather than acknowledging it.
func acpModelDiscovery171FixtureProcess() bool {
	if len(os.Args) != 2 || os.Args[1] != "--acp" || os.Getenv("QODER_PERSONAL_ACCESS_TOKEN") != "controlled-discovery171-marker" {
		return false
	}
	if os.Getenv("SUMA_RUNNER_TOKEN") != "" {
		os.Exit(83)
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var packet struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &packet) != nil {
			os.Exit(84)
		}
		var result any
		switch packet.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": 1, "agentInfo": map[string]any{"name": "owned model discovery fixture", "version": "171"}, "agentCapabilities": map[string]any{}, "authMethods": []any{}}
		case "session/new":
			result = map[string]any{"sessionId": "private-discovery171-session", "configOptions": []any{map[string]any{"id": "model", "name": "Model", "category": "model", "type": "select", "currentValue": "primary171", "options": []any{map[string]any{"value": "primary171", "name": "Primary"}, map[string]any{"value": "secondary171", "name": "模型二"}}}}}
		default:
			os.Exit(85)
		}
		if json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc": "2.0", "id": packet.ID, "result": result}) != nil {
			os.Exit(86)
		}
	}
	if scanner.Err() != nil {
		os.Exit(87)
	}
	fmt.Fprint(os.Stderr, "")
	os.Exit(0)
	return true
}
