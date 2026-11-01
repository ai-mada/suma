package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCursorPrivateDiscoveryAndManagedConfigurationKeepClosedSource(t *testing.T) {
	spec := cursorConfigFixture(t)
	_, sha, err := qoderNativeTarget(context.Background(), spec.Executable)
	if err != nil {
		t.Fatal(err)
	}
	input := ACPModelDiscoveryInput{AdapterID: "cursor-cli", Executable: spec.Executable, ExecutableSHA256: sha, ReviewedVersion: spec.ACP.ReviewedVersion, CredentialEnvRef: spec.ACP.CredentialEnvRef}
	raw, _ := json.Marshal(input)
	if _, err = DecodeACPModelDiscoveryInput(raw); err != nil {
		t.Fatal("exact Cursor discovery source rejected", err)
	}
	for _, bad := range []string{strings.Replace(string(raw), `"credential_env_ref":"LOCAL_CURSOR_CREDENTIAL"`, `"credential_env_ref":""`, 1), strings.TrimSuffix(string(raw), "}") + `,"args":["--force"]}`, strings.Replace(string(raw), `"adapter_id":"cursor-cli"`, `"adapter_id":"cursor"`, 1)} {
		if _, err = DecodeACPModelDiscoveryInput([]byte(bad)); err == nil {
			t.Fatal("unapproved private discovery source accepted")
		}
	}
	managed := desktopManagedFixtureInput(t)
	managed.AdapterID = "cursor-cli"
	managed.ACPConfiguration = &DesktopManagedACPConfiguration{Executable: spec.Executable, ExecutableSHA256: sha, ReviewedVersion: spec.ACP.ReviewedVersion, Model: spec.ACP.Model, CredentialEnvRef: spec.ACP.CredentialEnvRef, ConfigurationSHA256: ACPConfigurationDigest(*spec.ACP)}
	raw, _ = json.Marshal(managed)
	if _, err = DecodeDesktopManagedRunnerInput(raw); err != nil {
		t.Fatal("closed Cursor desktop input rejected", err)
	}
	managed.AdapterID = "qoder-acp"
	raw, _ = json.Marshal(managed)
	if _, err = DecodeDesktopManagedRunnerInput(raw); err == nil {
		t.Fatal("Cursor package silently changed adapter")
	}
}

func TestCursorPackageDiscoveryKeepsConfigurationRequired(t *testing.T) {
	spec := cursorConfigFixture(t)
	root := spec.ACP.CursorPackage.PackageRoot
	item := discoverCursorPackage(context.Background(), []string{root}, func(context.Context, CommandSpec) (string, error) { return "Cursor exact fixture189", nil })
	if !item.Installed || item.LaunchSupported || item.Command != nil || item.ReasonCode != "acp_configuration_required" || item.Version != "Cursor exact fixture189" || item.ExecutableSHA256 == "" {
		t.Fatal("package observation manufactured executable configuration")
	}
	item = discoverCursorPackage(context.Background(), []string{root}, func(context.Context, CommandSpec) (string, error) {
		if err := os.WriteFile(filepath.Join(root, "index.js"), []byte("changed dependency"), 0600); err != nil {
			t.Fatal(err)
		}
		return "Cursor exact fixture189", nil
	})
	if item.ReasonCode != "probe_failed" || item.Version != "" || item.Command != nil {
		t.Fatal("package mutation retained reviewed version")
	}
}
