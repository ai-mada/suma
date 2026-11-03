package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

func inertInstalledHost225(t *testing.T, dir, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	file := filepath.Join(dir, name)
	if err := os.WriteFile(file, []byte("MZ\x00\x00owned-inert-host-225"), 0700); err != nil {
		t.Fatal(err)
	}
	return file
}
func installedFixture225(t *testing.T, w *Worker, id string) (tp.Operation, installationCheckpoint, InstalledToolDiscovery) {
	t.Helper()
	entry, ok := installedDirectoryEntry(id)
	if !ok {
		t.Fatal(id)
	}
	entry.command.Executable = inertInstalledHost225(t, t.TempDir(), entry.binary)
	pinned, err := pinResolvedInstalledCommand(context.Background(), id, entry.command, "")
	if err != nil {
		t.Fatal(err)
	}
	version := "owned-fixture225"
	configuration := tp.Configuration{}
	if id == "claude-code-cli" {
		version = "2.1.314"
		configuration.Model = "claude-explicit-model225"
	}
	row := InstalledToolDiscoveryItem{AdapterID: id, Installed: true, LaunchSupported: true, Executable: pinned.Executable, ExecutableSHA256: pinned.InstalledCommand.HostSHA256, Version: version, ReasonCode: "workspace_required", Command: &entry.command, PinnedCommand: &pinned}
	items, err := installationItems(InstalledToolDiscovery{Protocol: InstalledToolDiscoveryProtocol, Items: []InstalledToolDiscoveryItem{row}}, []string{id})
	if err != nil {
		t.Fatal(err)
	}
	op := preparationFixture224(t, w)
	op.AdapterID = id
	op.Configuration = configuration
	op.InstallationItem = items[0]
	scan := installationFixture205(w.Client.ProjectID, w.ID, op.InstallationReference.ClaimID, w.maintenance.InstanceID)
	scan.ID = op.InstallationScanID
	scan.AdapterIDs = []string{id}
	scan.SourceSHA256 = ii.ScanSourceSHA256(scan)
	op.InstallationSourceSHA256 = scan.SourceSHA256
	op.InstallationStopProof.SourceSHA256 = scan.SourceSHA256
	op.SourceSHA256 = tp.SourceSHA256(op)
	if err = tp.ValidateOperation(op); err != nil {
		t.Fatal(err)
	}
	scope, _ := InventoryScopeSHA256(w.Client.BaseURL, w.Client.ProjectID, w.ID)
	p := installationCheckpoint{ScopeSHA256: scope, WorkerID: w.ID, ClaimID: scan.ClaimID, Responded: true, Scan: &scan, Started: true}
	return op, p, InstalledToolDiscovery{Protocol: InstalledToolDiscoveryProtocol, Items: []InstalledToolDiscoveryItem{row}}
}
func staticInstalledProfile225(t *testing.T, op tp.Operation, spec CommandSpec) tc.Profile {
	t.Helper()
	sha, err := commandProfileDigest(spec, spec.InstalledCommand.HostSHA256)
	if err != nil {
		t.Fatal(err)
	}
	p := tc.Profile{CommandID: tp.CommandID(op.ID), Adapter: spec.Adapter, Protocol: spec.Protocol, ToolVersion: op.InstallationItem.Version, ExecutableSHA256: spec.InstalledCommand.HostSHA256, ProfileSHA256: sha}
	p.ClaudeExecution, err = localClaudeCapability(spec)
	if err != nil || !tp.ProfileMatchesConfiguration(op.AdapterID, op.Configuration, p) {
		t.Fatal("fixed profile configuration mismatch", err)
	}
	return p
}

func TestManagedInstalledFourFactoriesConsumeOnlyOriginalPrivateScan(t *testing.T) {
	for _, id := range []string{"codex-jsonl", "codebuddy-cli", "dsh-multica", "claude-code-cli"} {
		t.Run(id, func(t *testing.T) {
			w := preparationWorker224(t, "http://127.0.0.1:4310")
			op, scan, report := installedFixture225(t, w, id)
			items, err := installationItems(report, []string{id})
			if err != nil {
				t.Fatal(err)
			}
			if err = w.Journal.saveInstallationObservation(context.Background(), scan, report, items); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", strings.Repeat("invalid-path-", 4000)) // Any second discovery would fail.
			source, err := w.Journal.preparationNative(context.Background(), scan.ScopeSHA256, op)
			if err != nil {
				t.Fatal(err)
			}
			intent, err := installedPreparationIntent(op, source)
			if err != nil {
				t.Fatal(err)
			}
			if id != "claude-code-cli" && !reflect.DeepEqual(intent.Command.Args, report.Items[0].PinnedCommand.Args) {
				t.Fatal("generic preparation invented argv")
			}
			if id == "claude-code-cli" && intent.Command.Args[len(intent.Command.Args)-1] != op.Configuration.Model {
				t.Fatal("Claude reused the scan default model")
			}
			if id == "claude-code-cli" {
				boundary := op
				boundary.Configuration.Model = "claude-" + strings.Repeat("x", 248) + "1"
				if !tc.ValidClaudeCLIModel(boundary.Configuration.Model) {
					t.Fatal("boundary fixture is not a shared valid model token")
				}
				if _, e := installedPreparationIntent(boundary, source); e != nil {
					t.Fatal("shared valid model token diverged in the local factory", e)
				}
			}
			file := filepath.Join(t.TempDir(), "prepared.json")
			if err = writeInstalledToolPreparation(context.Background(), file, *intent); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(file)
			commands, err := DecodeCommands(data)
			if err != nil || len(commands) != 1 {
				t.Fatal(err)
			}
			spec := commands["local-"+id]
			_ = staticInstalledProfile225(t, op, spec)
			if err = writeInstalledToolPreparation(context.Background(), file, *intent); err == nil {
				t.Fatal("factory overwrote existing output")
			}
			changed := op
			changed.Configuration.CredentialEnvRef = "IGNORED_REF"
			if _, err = installedPreparationIntent(changed, source); err == nil {
				t.Fatal("unsupported credential reference was silently accepted")
			}
			changed = op
			if id != "claude-code-cli" {
				changed.Configuration.Model = "ignored-model"
			} else {
				changed.Configuration.Model = ""
			}
			if _, err = installedPreparationIntent(changed, source); err == nil {
				t.Fatal("unsupported or missing model was silently accepted")
			}
			raw, _ := json.Marshal(report)
			if strings.Contains(string(raw), `"resolver_root"`) {
				t.Fatal("private closure escaped through scan wire")
			}
		})
	}
}

func TestInstalledNodeClosurePinsHoistedDependenciesAndRejectsExternalResolution(t *testing.T) {
	// Node's resolver must not inherit this repository's unrelated node_modules.
	// Use a tiny task-owned tree beneath the standard temporary directory.
	base := os.Getenv("SUMA_INSTALLED_NODE_TEST_TMP")
	if base == "" {
		base = os.TempDir()
	}
	if !filepath.IsAbs(base) {
		t.Fatal("explicit or standard temporary directory unavailable")
	}
	// Go 1.26 testing.TempDir prioritizes GOTMPDIR. The optional process-local
	// root lets a restricted host choose an explicitly owned, closed resolver
	// scope without weakening production pins or changing a user's environment.
	t.Setenv("TEMP", base)
	t.Setenv("TMP", base)
	t.Setenv("GOTMPDIR", base)
	directory := t.TempDir()
	t.Setenv("HOME", directory)
	t.Setenv("USERPROFILE", directory)
	modules := filepath.Join(directory, "install", "node_modules")
	pkg := filepath.Join(modules, "@tencent-ai", "codebuddy-code")
	if err := os.MkdirAll(pkg, 0700); err != nil {
		t.Fatal(err)
	}
	hostDirectory := filepath.Join(directory, "host")
	if err := os.Mkdir(hostDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	host := inertInstalledHost225(t, hostDirectory, "node")
	entryFile := filepath.Join(pkg, "cli.cjs")
	if err := os.WriteFile(entryFile, []byte("require('owned-shared');\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"name":"@tencent-ai/codebuddy-code","version":"1.0.0","bin":{"codebuddy":"cli.cjs"},"dependencies":{"owned-shared":"1.0.0"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(modules, "owned-shared")
	if err := os.Mkdir(sibling, 0700); err != nil {
		t.Fatal(err)
	}
	dependency := filepath.Join(sibling, "index.js")
	if err := os.WriteFile(dependency, []byte("exports.value=1;\n"), 0600); err != nil {
		t.Fatal(err)
	}
	template, _ := installedDirectoryEntry("codebuddy-cli")
	spec := template.command
	spec.Executable = host
	spec.Args = append([]string{entryFile}, spec.Args...)
	pinned, err := pinResolvedInstalledCommand(context.Background(), "codebuddy-cli", spec, modules)
	if err != nil {
		_, parents, parentErr := cursorPackageRoot(modules)
		opened, openErr := os.OpenRoot(modules)
		rootSame := false
		var statErr error
		if opened != nil {
			info, e := opened.Stat(".")
			statErr = e
			if e == nil && len(parents) > 0 {
				rootSame = os.SameFile(parents[0].info, info)
			}
			_ = opened.Close()
		}
		unstable := []string{}
		for _, parent := range parents {
			info, e := directoryIdentity(parent.path)
			if e != nil || !os.SameFile(parent.info, info) {
				unstable = append(unstable, filepath.Base(parent.path))
			}
		}
		identityError := ""
		for parent := modules; ; parent = filepath.Dir(parent) {
			if _, e := directoryIdentity(parent); e != nil {
				identityError = filepath.Base(parent) + ": " + e.Error()
				break
			}
			if filepath.Dir(parent) == parent {
				break
			}
		}
		t.Fatal("bounded original Node installation rejected", err, "parent_read", parentErr, "root_open", openErr, "root_stat", statErr, "root_same", rootSame, "unstable_ancestors", unstable, "identity_error", identityError)
	}
	originalSHA, err := commandProfileDigest(pinned, pinned.InstalledCommand.HostSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(dependency, []byte("exports.value=2;\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = commandProfileDigest(pinned, pinned.InstalledCommand.HostSHA256); err == nil {
		t.Fatal("a hoisted dependency changed but retained its formal profile")
	}
	repinned, err := pinResolvedInstalledCommand(context.Background(), "codebuddy-cli", spec, modules)
	if err != nil {
		t.Fatal(err)
	}
	changedSHA, err := commandProfileDigest(repinned, repinned.InstalledCommand.HostSHA256)
	if err != nil || changedSHA == originalSHA {
		t.Fatal("dependency content was excluded from the new profile", err)
	}
	outside := filepath.Join(directory, "node_modules")
	if err = os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	if err = validateInstalledCommandPins(context.Background(), repinned); err == nil {
		t.Fatal("an uncaptured ancestor resolver became loadable")
	}
	if err = os.Remove(outside); err != nil {
		t.Fatal(err)
	}
	if err = validateInstalledCommandPins(context.Background(), repinned); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(host, []byte("MZ\x00\x00different-owned-host225"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = validateInstalledCommandPins(context.Background(), repinned); err == nil {
		t.Fatal("Node host drift was accepted")
	}
}

func TestManagedCursorPrivateReceiptUsesNodePathNotVersion(t *testing.T) {
	w := preparationWorker224(t, "http://127.0.0.1:4310")
	op := preparationFixture224(t, w)
	pkg := t.TempDir()
	hostName := "node"
	if runtime.GOOS == "windows" {
		hostName += ".exe"
	}
	host := filepath.Join(pkg, hostName)
	if err := os.WriteFile(host, []byte("MZ\x00\x00owned-cursor-node225"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "index.js"), []byte("throw Error('never run');\n"), 0600); err != nil {
		t.Fatal(err)
	}
	closure, node, err := inspectCursorPackage(pkg)
	if err != nil || node != host {
		t.Fatal(err)
	}
	_, nativeSHA, err := qoderNativeTarget(context.Background(), host)
	if err != nil {
		t.Fatal(err)
	}
	row := InstalledToolDiscoveryItem{AdapterID: "cursor-cli", Installed: true, Executable: host, ExecutableSHA256: nativeSHA, Version: "real-scanned-version-225", ReasonCode: "acp_configuration_required", CursorPackage: &closure}
	report := InstalledToolDiscovery{Protocol: InstalledToolDiscoveryProtocol, Items: []InstalledToolDiscoveryItem{row}}
	items, err := installationItems(report, []string{"cursor-cli"})
	if err != nil {
		t.Fatal(err)
	}
	scan := installationFixture205(w.Client.ProjectID, w.ID, op.InstallationReference.ClaimID, w.maintenance.InstanceID)
	scan.ID = op.InstallationScanID
	scan.AdapterIDs = []string{"cursor-cli"}
	scan.SourceSHA256 = ii.ScanSourceSHA256(scan)
	scope, _ := InventoryScopeSHA256(w.Client.BaseURL, w.Client.ProjectID, w.ID)
	checkpoint := installationCheckpoint{ScopeSHA256: scope, WorkerID: w.ID, ClaimID: scan.ClaimID, Responded: true, Scan: &scan, Started: true}
	if err = w.Journal.saveInstallationObservation(context.Background(), checkpoint, report, items); err != nil {
		t.Fatal("Node path was wrongly compared with the real scanned version", err)
	}
	op.AdapterID = "cursor-cli"
	op.Configuration = tp.Configuration{Model: "cursor-explicit225", CredentialEnvRef: "CURSOR_API_KEY"}
	op.InstallationItem = items[0]
	op.InstallationSourceSHA256 = scan.SourceSHA256
	op.InstallationStopProof.SourceSHA256 = scan.SourceSHA256
	op.SourceSHA256 = tp.SourceSHA256(op)
	if _, err = w.Journal.preparationNative(context.Background(), scope, op); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(pkg, "new-dependency.js"), []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Journal.preparationNative(context.Background(), scope, op); err == nil {
		t.Fatal("Cursor dependency drift retained the private scan source")
	}
}

func TestManagedGenericGenerationRestoresExactClaimAndPreservesUnknownIntent(t *testing.T) {
	var operation tp.Operation
	var nonces []string
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runner/tool-preparations/claim" {
			t.Error("unexpected operation on restore", r.URL.Path)
			out.WriteHeader(404)
			return
		}
		var request tp.ClaimRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		nonces = append(nonces, request.ClaimID)
		_ = json.NewEncoder(out).Encode(tp.Claim{TenantID: operation.TenantID, ProjectID: operation.ProjectID, WorkerID: operation.WorkerID, Preparation: &operation})
	}))
	defer server.Close()
	w := preparationWorker224(t, server.URL)
	op, scan, report := installedFixture225(t, w, "claude-code-cli")
	operation = op
	items, _ := installationItems(report, []string{op.AdapterID})
	if err := w.Journal.saveInstallationObservation(context.Background(), scan, report, items); err != nil {
		t.Fatal(err)
	}
	native, err := w.Journal.preparationNative(context.Background(), scan.ScopeSHA256, op)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := installedPreparationIntent(op, native)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := toolPreparationCheckpoint{ScopeSHA256: scan.ScopeSHA256, WorkerID: w.ID, ClaimID: op.ClaimID, Responded: true, TenantID: op.TenantID, Operation: &op, Started: true, InstalledIntent: intent}
	if err = validateToolPreparationCheckpoint(checkpoint); err != nil {
		t.Fatal(err)
	}
	mixed := checkpoint
	mixed.Intent = &ACPToolPreparationInput{AdapterID: "qoder-acp"}
	if validateToolPreparationCheckpoint(mixed) == nil {
		t.Fatal("ACP and generic typed intents were both accepted")
	}
	dir, err := w.Journal.maintenanceDirectory("tool-command-generations")
	if err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(dir, op.ID+"-commands.json")
	if err = writeInstalledToolPreparation(context.Background(), artifact, *intent); err != nil {
		t.Fatal(err)
	}
	artifactRaw, _ := os.ReadFile(artifact)
	profile := staticInstalledProfile225(t, op, intent.Command)
	probeDir, source, err := w.Journal.preparationProbeDirectory(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	collector, err := newToolProbeCollectorAt(probeDir, source)
	if err != nil {
		t.Fatal(err)
	}
	if err = collector.closeReturned(); err != nil {
		t.Fatal(err)
	}
	proof, err := w.Journal.preparationStopProof(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	generation := toolCommandGeneration{Protocol: tp.Protocol, ScopeSHA256: scan.ScopeSHA256, TenantID: op.TenantID, ProjectID: op.ProjectID, WorkerID: op.WorkerID, BaseSHA256: w.managedBaseSHA, ParentSHA256: w.currentGenerationSHA(), Preparation: preparationReference(op), SourceSHA256: op.SourceSHA256, Commands: map[string]CommandSpec{profile.CommandID: intent.Command}, Profiles: map[string]tc.Profile{profile.CommandID: profile}}
	raw, _ := json.Marshal(generation)
	sha := digest(raw)
	head := &toolGenerationHead{GenerationSHA256: sha, Generation: generation, Receipt: tp.GenerationReceipt{ParentGenerationSHA256: generation.ParentSHA256, GenerationSHA256: sha, CommandID: profile.CommandID, ArtifactSHA256: digest(artifactRaw), Profile: profile, StopProof: &proof}, Inventory: InventoryReport{WorkerID: w.ID, OperationID: operationID(), ScopeSHA256: scan.ScopeSHA256, Items: []tc.Observation{{Profile: profile, Available: false, Reason: "local execution disabled"}}}}
	if err = w.persistManagedGeneration(checkpoint, head); err != nil {
		t.Fatal(err)
	}
	operation.State = "awaiting_inventory"
	operation.ResultCode = "prepared"
	operation.Receipt = &head.Receipt
	next := &Worker{Client: w.Client, Journal: w.Journal, ID: w.ID, AllowInstalledToolDiscovery: true, AllowToolPreparation: true}
	if err = next.enableToolMaintenance(); err != nil {
		t.Fatal(err)
	}
	if err = next.restoreToolGeneration(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(artifact)
	if len(nonces) != 1 || nonces[0] != op.ClaimID || !reflect.DeepEqual(after, artifactRaw) || len(next.currentExecSnapshot().Commands) != 1 || next.Exec.AllowExec {
		t.Fatal("generic restart invented a claim/effect or widened execution")
	}
	checkpoint.Result = w.preparationResult(checkpoint, "unknown", "worker_restarted", nil, nil)
	key := checkpoint.Result.IdempotencyKey
	if err = w.Journal.savePreparation(checkpoint); err != nil {
		t.Fatal(err)
	}
	restored, err := w.Journal.preparationCheckpoint()
	if err != nil || restored.InstalledIntent == nil || restored.Result.IdempotencyKey != key || restored.Intent != nil {
		t.Fatal("unknown generic source/body was lost on persistence", err)
	}
	if err = os.WriteFile(intent.Command.Executable, []byte("MZ\x00\x00changed-owned-installed225"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = next.validateGenerationAssets(context.Background(), head); err == nil {
		t.Fatal("restored generic native drift was accepted")
	}
}
