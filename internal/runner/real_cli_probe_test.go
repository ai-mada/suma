package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A subprocess of the test binary observes only its own environment. It is
// neither a supplier CLI nor evidence of any supplier protocol or model.
func TestRealCLIProbeHelper226(t *testing.T) {
	if os.Getenv("SUMA_PROBE226_HELPER") != "1" {
		return
	}
	dir, _ := os.Getwd()
	marker, _ := os.ReadFile(filepath.Join(os.Getenv("HOME"), "account-marker"))
	features, _ := os.ReadFile(filepath.Join(os.Getenv("CODEX_HOME"), "config.toml"))
	_, _ = fmt.Fprintln(os.Stderr, "nonfatal startup warning is not a version")
	raw, _ := json.Marshal(map[string]string{"home": os.Getenv("HOME"), "codex_home": os.Getenv("CODEX_HOME"), "working": dir, "marker": string(marker), "features": string(features)})
	_, _ = fmt.Fprintln(os.Stdout, string(raw))
	os.Exit(0)
}

func TestReadOnlyProbePrivateHomeAndAuthoritativeStdout(t *testing.T) {
	original := t.TempDir()
	if err := os.WriteFile(filepath.Join(original, "account-marker"), []byte("existing-account-do-not-read"), 0600); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	raw, err := probeCommandStreams(context.Background(), exe, []string{"-test.run=^TestRealCLIProbeHelper226$"}, CommandSpec{Env: map[string]string{"SUMA_PROBE226_HELPER": "1", "HOME": original, "USERPROFILE": original, "CODEX_HOME": original}}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	var observed map[string]string
	if json.Unmarshal([]byte(raw), &observed) != nil || observed["home"] == original || observed["codex_home"] == original || observed["working"] == original || observed["marker"] != "" || strings.Contains(raw, "nonfatal startup") {
		t.Fatal("read-only startup consulted the original home or mixed diagnostics into stdout", raw)
	}
}

func TestTrustedProbeHomePreservesPreparedFeaturesUnderCollector(t *testing.T) {
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	client := &Client{BaseURL: "http://127.0.0.1:22600", ProjectID: "project226", Token: "fixture226"}
	cp, err := j.beginToolInspection(client, "worker226")
	if err != nil {
		t.Fatal(err)
	}
	i := inspectionFixture194(client.ProjectID, "worker226", cp.ClaimID)
	cp.Responded, cp.Inspection, cp.Started = true, &i, true
	if err = j.saveToolInspection(*cp); err != nil {
		t.Fatal(err)
	}
	collector, err := newToolProbeCollector(j, *cp)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	home := filepath.Join(root, "codex")
	if err = os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	const features = "[features]\nrequest_user_input = true\n"
	if err = os.WriteFile(filepath.Join(home, "config.toml"), []byte(features), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, closeScope, err := bindPrivateProbeDirectory(collector.context(context.Background()), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer closeScope()
	exe, _ := os.Executable()
	raw, err := probeCommandStreams(ctx, exe, []string{"-test.run=^TestRealCLIProbeHelper226$"}, CommandSpec{Env: map[string]string{"SUMA_PROBE226_HELPER": "1", "CODEX_HOME": home}}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	var observed map[string]string
	if json.Unmarshal([]byte(raw), &observed) != nil || observed["codex_home"] != home || observed["features"] != features {
		t.Fatal("collector replaced the explicitly prepared home", raw)
	}
	if err = collector.closeReturned(); err != nil {
		t.Fatal(err)
	}
	if receipt, _, e := j.readToolProbeReceipt(*cp); e != nil || receipt.NativeProcessCount != 1 {
		t.Fatal("original collector was bypassed", e)
	}
}

func TestPrivateProbeUnknownStopRetainsOwnedDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "owned")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, closeScope, err := bindPrivateProbeDirectory(context.Background(), root, func() { _ = os.RemoveAll(root) })
	if err != nil {
		t.Fatal(err)
	}
	scope := privateProbeScopeFrom(ctx)
	scope.begin()
	scope.end(false)
	closeScope()
	if _, err = os.Stat(root); err != nil {
		t.Fatal("unknown process stop discarded its private state", err)
	}
	if d := probeDiagnostic(scope.begin()); d == nil || d.Code != "stop_unverified" || d.NextAction != "none" {
		t.Fatal("unknown scope allowed another native start", d)
	}
	known := filepath.Join(t.TempDir(), "known")
	if err = os.Mkdir(known, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, closeScope, err = bindPrivateProbeDirectory(context.Background(), known, func() { _ = os.RemoveAll(known) })
	if err != nil {
		t.Fatal(err)
	}
	scope = privateProbeScopeFrom(ctx)
	scope.begin()
	scope.end(true)
	closeScope()
	if _, err = os.Stat(known); !os.IsNotExist(err) {
		t.Fatal("confirmed stopped scope was not released", err)
	}
}

func TestTypedProbeDiagnosticKeepsCauseAndProjectsOnlyClosedFacts(t *testing.T) {
	private := errors.Join(os.ErrPermission, errors.New("private-path credential=do-not-project"))
	failure := newProbeFailure("version", "launch_failed", private)
	if !errors.Is(failure, os.ErrPermission) || strings.Contains(failure.Error(), "private-path") {
		t.Fatal("cause was lost or exposed", failure)
	}
	root := t.TempDir()
	localDiscoveryNative(t, root, "codex")
	report, err := discoverInstalledTools(context.Background(), "codex-jsonl", []string{root}, func(context.Context, CommandSpec) (string, error) { return "", failure })
	if err != nil {
		t.Fatal(err)
	}
	items, err := installationItems(report, []string{"codex-jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(items)
	if len(items) != 1 || items[0].Diagnostic == nil || items[0].Diagnostic.Stage != "version" || items[0].Diagnostic.Code != "permission_denied" || items[0].Diagnostic.NextAction != "guide" || strings.Contains(string(raw), "private-path") || strings.Contains(string(raw), "do-not-project") {
		t.Fatal("diagnostic was guessed, dropped or leaked", string(raw))
	}
	d := probeDiagnostic(newProbeFailure("protocol", "stop_unverified", nil))
	if d == nil || d.NextAction != "none" {
		t.Fatal("unknown stop permitted a new probe", d)
	}
	d = probeDiagnostic(probeProcessFailure(context.Background(), "version", nil, os.ErrPermission, false))
	if d == nil || d.Code != "stop_unverified" || d.NextAction != "none" {
		t.Fatal("stop uncertainty was downgraded to a permission retry", d)
	}
	if probeDiagnostic(errors.New("permission denied at private-path")) != nil {
		t.Fatal("raw text was classified as a permission fact")
	}
	unixStop := probeDiagnostic(requireProbeTreeStop(true, nil, "unix_process_group"))
	if unixStop == nil || unixStop.Code != "stop_unverified" || unixStop.NextAction != "none" {
		t.Fatal("parent/group absence became complete tree-stop evidence", unixStop)
	}
	for _, containment := range []string{"windows_job", "linux_pid_namespace"} {
		if requireProbeTreeStop(true, nil, containment) != nil || requireProbeTreeStop(false, nil, containment) == nil {
			t.Fatal("expected both original wait and authoritative tree stop", containment)
		}
	}
	failedStop := requireProbeTreeStop(true, private, "windows_job")
	d = probeDiagnostic(failedStop)
	if !errors.Is(failedStop, os.ErrPermission) || d == nil || d.Code != "stop_unverified" || d.NextAction != "none" || strings.Contains(failedStop.Error(), "private-path") {
		t.Fatal("unconfirmed native stop lost its cause or became retryable", d)
	}
}
