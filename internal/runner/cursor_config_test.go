package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func cursorConfigFixture(t *testing.T) CommandSpec {
	t.Helper()
	root := t.TempDir()
	name := "node"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	localDiscoveryFile(t, filepath.Join(root, name), "MZowned native fixture; not executed")
	localDiscoveryFile(t, filepath.Join(root, "index.js"), "// owned entry fixture")
	spec, err := BuildCursorACPCommand(root, "Cursor exact fixture189", "fixed-model189", "LOCAL_CURSOR_CREDENTIAL")
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestCursorConfigurationBindsPackageAndFixedSafeArguments(t *testing.T) {
	spec := cursorConfigFixture(t)
	capability, err := localACPCapability(spec)
	if err != nil || capability == nil || capability.Runtime != "cursor" || capability.AuthStrategy != "explicit_api_key_env" {
		t.Fatal("Cursor capability unavailable", err)
	}
	for _, mutate := range []func(*CommandSpec){
		func(s *CommandSpec) { s.Args = append(s.Args, "--force") },
		func(s *CommandSpec) { s.Args[3] = "disabled" },
		func(s *CommandSpec) { s.ACP.CredentialEnvRef = "" },
		func(s *CommandSpec) { s.Adapter = "qoder" },
		func(s *CommandSpec) { s.ACP.CursorPackage = nil },
		func(s *CommandSpec) { s.EnvFrom = []string{"CURSOR_API_KEY"} },
	} {
		changed := cursorConfigFixture(t)
		mutate(&changed)
		if validateACPExecutionSpec(changed) == nil {
			t.Fatal("unsafe or mixed Cursor configuration accepted")
		}
	}
	changed := *spec.ACP
	packageCopy := *changed.CursorPackage
	changed.CursorPackage = &packageCopy
	changed.CursorPackage.SHA256 = strings.Repeat("b", 64)
	if ACPConfigurationDigest(changed) == capability.ConfigurationSHA256 {
		t.Fatal("closure digest did not bind profile")
	}
	raw, _ := json.Marshal(capability)
	if strings.Contains(string(raw), spec.ACP.CursorPackage.PackageRoot) || strings.Contains(string(raw), "credential_env_ref") {
		t.Fatal("private package metadata leaked")
	}
}

func TestCursorEnvironmentRequiresFreshPrivateHomeAndOnlyExplicitKey(t *testing.T) {
	spec := cursorConfigFixture(t)
	t.Setenv("LOCAL_CURSOR_CREDENTIAL", "owned-synthetic-key189")
	t.Setenv("CURSOR_API_KEY", "original-cursor-key-must-not-pass")
	t.Setenv("CURSOR_AUTH_TOKEN", "original-login-must-not-pass")
	t.Setenv("NODE_OPTIONS", "--require=untrusted")
	t.Setenv("SUMA_RUNNER_TOKEN", "platform-must-not-pass")
	root := t.TempDir()
	home := filepath.Join(root, "home")
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	env, secrets, err := cursorEnvironment(spec, home, source, true)
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(env, "\n")
	if !strings.Contains(all, "CURSOR_API_KEY=owned-synthetic-key189") || !strings.Contains(all, "CURSOR_CONFIG_DIR="+filepath.Join(home, "cursor-config")) || !strings.Contains(all, "HOME="+home) || len(secrets) != 1 {
		t.Fatal("private Cursor environment missing exact credential")
	}
	for _, bad := range []string{"original-cursor-key", "original-login", "NODE_OPTIONS", "platform-must-not-pass", "LOCAL_CURSOR_CREDENTIAL="} {
		if strings.Contains(all, bad) {
			t.Fatal("inherited unsafe authentication or runtime setting")
		}
	}
	if _, _, err = cursorEnvironment(spec, home, source, true); err == nil {
		t.Fatal("reused home imported prior state")
	}
}

func TestCursorProfileFinalDigestRejectsChangedDependency(t *testing.T) {
	spec := cursorConfigFixture(t)
	_, sha, err := qoderNativeTarget(context.Background(), spec.Executable)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = commandProfileDigest(spec, sha); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(spec.ACP.CursorPackage.PackageRoot, "index.js"), []byte("unreviewed replacement after version/workspace"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = commandProfileDigest(spec, sha); err == nil {
		t.Fatal("final profile gate trusted changed dependency with unchanged native host")
	}
}

func TestCursorDevelopmentContinuationUsesOrdinaryGuard(t *testing.T) {
	a := continuationGuardAssignment(t)
	spec := cursorConfigFixture(t)
	if err := developmentContinuationSpec(a, spec); err != nil {
		t.Fatal("reviewed Cursor continuation rejected", err)
	}
	a.Config["purpose"] = ""
	if developmentContinuationSpec(a, spec) == nil {
		t.Fatal("Cursor continuation accepted a dedicated purpose")
	}
}
