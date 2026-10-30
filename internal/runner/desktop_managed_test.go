package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func desktopManagedFixtureInput(t *testing.T) DesktopManagedRunnerInput {
	t.Helper()
	return DesktopManagedRunnerInput{Protocol: DesktopManagedRunnerProtocol, ControlURL: "http://127.0.0.1:4319", ProjectID: strings.Repeat("1", 32), WorkerID: "desktop-" + strings.Repeat("2", 32), AdapterID: "codex-jsonl", Repository: t.TempDir(), BaseCommit: strings.Repeat("a", 40), StateDirectory: filepath.Join(t.TempDir(), "state"), Token: strings.Repeat("b", 64)}
}

func TestDesktopManagedInputClosedAndNeverEchoesCredential(t *testing.T) {
	input := desktopManagedFixtureInput(t)
	raw, _ := json.Marshal(input)
	decoded, err := DecodeDesktopManagedRunnerInput(raw)
	if err != nil || decoded.Token != input.Token {
		t.Fatal("valid private input was rejected")
	}
	for _, bad := range [][]byte{append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"token":"override"}`)...), []byte(strings.Replace(string(raw), `"protocol"`, `"Protocol"`, 1)), []byte(strings.Replace(string(raw), `"token":`, `"arbitrary":`, 1)), []byte(`null`), append(raw, []byte(`{}`)...), []byte(strings.Repeat("x", DesktopManagedInputLimit+1))} {
		if _, err := DecodeDesktopManagedRunnerInput(bad); err == nil || strings.Contains(err.Error(), input.Token) {
			t.Fatal("invalid input accepted or credential echoed")
		}
	}
}

func TestDesktopManagedInputRejectsRemoteControlAndInjectedPaths(t *testing.T) {
	for _, mutate := range []func(*DesktopManagedRunnerInput){
		func(c *DesktopManagedRunnerInput) { c.ControlURL = "https://remote.example" }, func(c *DesktopManagedRunnerInput) { c.ControlURL = "http://user:secret@127.0.0.1:4319" }, func(c *DesktopManagedRunnerInput) { c.ControlURL += "/api" }, func(c *DesktopManagedRunnerInput) { c.ControlURL += "?token=secret" }, func(c *DesktopManagedRunnerInput) { c.WorkerID = "ordinary-node" }, func(c *DesktopManagedRunnerInput) { c.AdapterID = "shell" }, func(c *DesktopManagedRunnerInput) { c.Repository = "relative" }, func(c *DesktopManagedRunnerInput) { c.StateDirectory += "\ninvalid" }, func(c *DesktopManagedRunnerInput) { c.Token = "short" },
	} {
		input := desktopManagedFixtureInput(t)
		mutate(&input)
		raw, _ := json.Marshal(input)
		if _, err := DecodeDesktopManagedRunnerInput(raw); err == nil {
			t.Fatal("unsafe configuration accepted")
		}
	}
}

func TestDesktopManagedExecutionKeepsPinnedWorktreePolicyWithoutJournalSideEffects(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo, base := gitFixture(t)
	input := desktopManagedFixtureInput(t)
	input.Repository = repo
	input.BaseCommit = base
	executable, _ := os.Executable()
	spec := localToolDirectory()[0].command
	spec.Executable = executable
	discover := func(context.Context, string) (InstalledToolDiscovery, error) {
		return InstalledToolDiscovery{Protocol: InstalledToolDiscoveryProtocol, Items: []InstalledToolDiscoveryItem{{AdapterID: input.AdapterID, Installed: true, LaunchSupported: true, Command: &spec}}}, nil
	}
	cfg, err := prepareDesktopManagedExecution(context.Background(), input, discover)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AllowExec || cfg.Repository != repo || cfg.BaseCommit != base || cfg.Workspace != repo || len(cfg.Commands) != 1 || cfg.Commands["desktop-codex-jsonl"].ReadOnly || len(cfg.Secrets) != 1 || cfg.Secrets[0] != input.Token {
		t.Fatal("lost pinned execution policy")
	}
	if _, err = os.Stat(input.StateDirectory); !os.IsNotExist(err) {
		t.Fatal("preparation must not open a durable journal")
	}
	if got := strings.TrimSpace(gitTest(t, repo, "status", "--porcelain")); got != "" {
		t.Fatal("preparation modified the checkout")
	}
}

func TestDesktopManagedWorkspacePrecedesAnyCLIProbe(t *testing.T) {
	input := desktopManagedFixtureInput(t)
	calls := 0
	_, err := prepareDesktopManagedExecution(context.Background(), input, func(context.Context, string) (InstalledToolDiscovery, error) {
		calls++
		return InstalledToolDiscovery{}, nil
	})
	if err == nil || calls != 0 {
		t.Fatal("unbound/non-Git directory reached tool execution")
	}
}
