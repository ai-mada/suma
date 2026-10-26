package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestTeamCLINodeEntryConfiguration(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "local-cli.cjs")
	if err := os.WriteFile(entry, []byte("// local entry fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	dsh := CommandSpec{Adapter: "harness", Protocol: "dsh-multica-jsonl-v1", Executable: "node.exe", Args: []string{entry, "--profile", "multica", "--stdio"}, Input: "dsh-jsonl"}
	buddy := CommandSpec{Adapter: "codebuddy", Protocol: "codebuddy-jsonl-v1", Executable: "node", Args: []string{entry, "-p", "--input-format", "text", "--output-format", "stream-json", "--verbose"}, Input: "goal"}
	for _, spec := range []CommandSpec{dsh, buddy} {
		got, err := teamCLIEntry(spec)
		if err != nil || got != entry || validateTeamCLICommand(spec) != nil {
			t.Fatalf("explicit Node entry rejected: %q %v", got, err)
		}
	}
	for name, mutate := range map[string]func(*CommandSpec){
		"relative": func(s *CommandSpec) { s.Args[0] = "local-cli.cjs" },
		"missing":  func(s *CommandSpec) { s.Args[0] = filepath.Join(filepath.Dir(entry), "missing.js") },
		"directory": func(s *CommandSpec) {
			s.Args[0] = filepath.Join(t.TempDir(), "cli.js")
			if err := os.Mkdir(s.Args[0], 0700); err != nil {
				t.Fatal(err)
			}
		},
		"node_eval":         func(s *CommandSpec) { s.Args = append([]string{"-e", "arbitrary code"}, s.Args...) },
		"wrong_executable":  func(s *CommandSpec) { s.Executable = "python.exe" },
		"wrong_extension":   func(s *CommandSpec) { s.Args[0] = filepath.Join(filepath.Dir(entry), "cli.cmd") },
		"permission_bypass": func(s *CommandSpec) { s.Args = append(s.Args, "--permission-mode", "bypassPermissions") },
		"resume":            func(s *CommandSpec) { s.Args = append(s.Args, "--resume", "old-session") },
	} {
		t.Run(name, func(t *testing.T) {
			spec := buddy
			spec.Args = append([]string{}, buddy.Args...)
			mutate(&spec)
			if validateTeamCLICommand(spec) == nil {
				t.Fatal("uncontrolled Node entry or protocol override accepted")
			}
		})
	}
	oversized := filepath.Join(filepath.Dir(entry), "oversized.js")
	f, err := os.Create(oversized)
	if err != nil {
		t.Fatal(err)
	}
	err = f.Truncate((16 << 20) + 1)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	spec := dsh
	spec.Args = append([]string{}, dsh.Args...)
	spec.Args[0] = oversized
	if _, err = teamCLIEntry(spec); err == nil {
		t.Fatal("oversized entry accepted")
	}
	link := filepath.Join(filepath.Dir(entry), "linked.js")
	if err = os.Symlink(entry, link); err == nil {
		spec.Args[0] = link
		if _, err = teamCLIEntry(spec); err == nil {
			t.Fatal("symbolic entry accepted")
		}
	}
	if got, err := teamCLIEntry(CommandSpec{Adapter: "command", Executable: "node", Args: []string{"relative.js"}}); err != nil || got != "" {
		t.Fatal("non-team command changed")
	}
}

func TestTeamCLINodeEntryProbesAndContentBinding(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable for local CLI entry protocol fixture")
	}
	entry := filepath.Join(t.TempDir(), "team-entry.mjs")
	script := `const a=process.argv.slice(2);if(a.length===1&&a[0]==='--version'){console.log('local-node-cli protocol-fixture')}else if(a.length===1&&a[0]==='--help'){console.log('--input-format --output-format --verbose')}else if(a.join(' ')==='--profile multica --probe'){console.error('local entry probe diagnostic');console.log(JSON.stringify({v:1,type:'probe',runtime:'dsh',plugin_version:'node-entry-fixture',protocol_version:1}))}else{process.exit(29)}`
	if err = os.WriteFile(entry, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	dsh := CommandSpec{Adapter: "harness", Protocol: "dsh-multica-jsonl-v1", Executable: node, Args: []string{entry, "--profile", "multica", "--stdio"}, Input: "dsh-jsonl"}
	buddy := CommandSpec{Adapter: "codebuddy", Protocol: "codebuddy-jsonl-v1", Executable: node, Args: []string{entry, "-p", "--input-format", "text", "--output-format", "stream-json", "--verbose"}, Input: "goal"}
	for _, spec := range []CommandSpec{dsh, buddy} {
		version, err := teamCLIVersion(context.Background(), node, spec, nil)
		if err != nil || !strings.Contains(version, "local-node-cli protocol-fixture") {
			t.Fatalf("entry was not used for the fixed probes: %q %v", version, err)
		}
	}
	_, executableSHA, err := commandExecutable(dsh)
	if err != nil {
		t.Fatal(err)
	}
	before, err := commandProfileDigest(dsh, executableSHA)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(entry, []byte(script+"\n// changed administrator entry bytes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := commandProfileDigest(dsh, executableSHA)
	if err != nil || before == after {
		t.Fatalf("Node entry bytes were not bound to the command profile: before=%s after=%s err=%v", before, after, err)
	}
}

func TestTeamCLIConfigurationRejectsProtocolAndPermissionOverrides(t *testing.T) {
	dsh := CommandSpec{Adapter: "harness", Protocol: "dsh-multica-jsonl-v1", Executable: "dsh", Args: []string{"--profile", "multica", "--stdio"}, Input: "dsh-jsonl"}
	buddy := CommandSpec{Adapter: "codebuddy", Protocol: "codebuddy-jsonl-v1", Executable: "codebuddy", Args: []string{"-p", "--input-format", "text", "--output-format", "stream-json", "--verbose", "--permission-mode", "acceptEdits"}, Input: "goal"}
	for _, spec := range []CommandSpec{dsh, buddy} {
		if err := ValidateToolConfiguration(ExecConfig{Commands: map[string]CommandSpec{"develop": spec}}); err != nil {
			t.Fatal(err)
		}
	}
	for name, change := range map[string]func(*CommandSpec){
		"permission_bypass": func(s *CommandSpec) { s.Args[len(s.Args)-1] = "bypassPermissions" },
		"resume":            func(s *CommandSpec) { s.Args = append(s.Args, "--resume", "old-session") },
		"inline_goal":       func(s *CommandSpec) { s.Args = append(s.Args, "untrusted prompt") },
		"readonly_claim":    func(s *CommandSpec) { s.ReadOnly = true },
		"formal_test_claim": func(s *CommandSpec) { s.TestEvidence = true },
		"cwd_override":      func(s *CommandSpec) { s.Args = append(s.Args, "--cwd", "other-repository") },
		"protocol_override": func(s *CommandSpec) { s.Args = append(s.Args, "--output-format", "text") },
	} {
		t.Run(name, func(t *testing.T) {
			spec := buddy
			spec.Args = append([]string{}, buddy.Args...)
			change(&spec)
			if err := ValidateToolConfiguration(ExecConfig{Commands: map[string]CommandSpec{"develop": spec}}); err == nil {
				t.Fatal("unsafe local profile was accepted")
			}
		})
	}
	dsh.Args = []string{"--profile", "web", "--stdio"}
	if validateTeamCLICommand(dsh) == nil {
		t.Fatal("another DSH profile was accepted as the Multica protocol")
	}
	if tc.ValidateProfile(tc.Profile{CommandID: "develop", Adapter: "harness", Protocol: "codebuddy-jsonl-v1"}, false) == nil {
		t.Fatal("cross-adapter protocol was accepted")
	}
}

func TestTeamCLIDoesNotReceiveRunnerCredential(t *testing.T) {
	t.Setenv("SUMA_RUNNER_TOKEN", "runner-credential-must-not-reach-vendor")
	for _, adapter := range []string{"harness", "codebuddy"} {
		environment, _, err := commandEnvironment(CommandSpec{Adapter: adapter, Env: map[string]string{"SUMA_RUNNER_TOKEN": "configured-runner-credential"}, EnvFrom: []string{"SUMA_RUNNER_TOKEN"}})
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range environment {
			if strings.HasPrefix(strings.ToUpper(value), "SUMA_RUNNER_TOKEN=") {
				t.Fatal("vendor process received a platform Runner credential")
			}
		}
	}
	// An omitted adapter is the ordinary local-command default. Its explicit
	// administrator environment is distinct from a model-facing CLI process.
	for _, adapter := range []string{"", "command"} {
		environment, _, err := commandEnvironment(CommandSpec{Adapter: adapter, Env: map[string]string{"SUMA_RUNNER_TOKEN": "explicit-local-command"}})
		if err != nil || !strings.Contains(strings.Join(environment, "\n"), "SUMA_RUNNER_TOKEN=explicit-local-command") {
			t.Fatal("default local-command environment changed")
		}
	}
}

func TestCodebuddyNormalizedPolicyIsHashedWithoutMutatingCaller(t *testing.T) {
	source := CommandSpec{Adapter: "codebuddy", Env: map[string]string{"CODEBUDDY_CODE_DISABLE_BACKGROUND_TASKS": "0", "PROJECT_SETTING": "keep"}}
	normalized := normalizedCommand(source)
	if source.Env["CODEBUDDY_CODE_DISABLE_BACKGROUND_TASKS"] != "0" || normalized.Env["CODEBUDDY_CODE_DISABLE_BACKGROUND_TASKS"] != "1" || normalized.Env["PROJECT_SETTING"] != "keep" {
		t.Fatal("headless policy was not canonicalized independently")
	}
	normalized.Env["PROJECT_SETTING"] = "changed"
	if source.Env["PROJECT_SETTING"] != "keep" {
		t.Fatal("normalization mutated the administrator configuration")
	}
}
