package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// teamCLIEntry accepts an explicitly configured Node entry, never an npm shell
// shim or an entry discovered from the task's working directory.
func teamCLIEntry(spec CommandSpec) (string, error) {
	if spec.Adapter != "harness" && spec.Adapter != "codebuddy" {
		return "", nil
	}
	node := strings.ToLower(filepath.Base(spec.Executable))
	javascript := func(name string) bool {
		switch strings.ToLower(filepath.Ext(name)) {
		case ".js", ".mjs", ".cjs":
			return true
		}
		return false
	}
	if node != "node" && node != "node.exe" {
		if len(spec.Args) > 0 && javascript(spec.Args[0]) {
			return "", fmt.Errorf("team CLI JavaScript entries require the Node executable")
		}
		return "", nil
	}
	if len(spec.Args) == 0 || !filepath.IsAbs(spec.Args[0]) || !javascript(spec.Args[0]) || strings.ContainsRune(spec.Args[0], 0) {
		return "", fmt.Errorf("team CLI Node mode requires one absolute JavaScript entry before the fixed CLI arguments")
	}
	entry := spec.Args[0]
	st, err := os.Lstat(entry)
	if err != nil || !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || st.Size() > 16<<20 {
		return "", fmt.Errorf("team CLI Node entry must be an available regular file of at most 16 MiB without a symbolic link")
	}
	return entry, nil
}

// Team CLI adapters own their wire arguments. The workflow supplies a goal,
// never executable flags, resume IDs, a working directory or permission replies.
// Local administrators still own the CLI's authentication and permission policy.
func validateTeamCLICommand(spec CommandSpec) error {
	if spec.Adapter != "harness" && spec.Adapter != "codebuddy" {
		return nil
	}
	if spec.ReadOnly || spec.TestEvidence || spec.NativeWorkspace != nil {
		return fmt.Errorf("team CLI requires an isolated development profile; formal tests remain independent")
	}
	entry, err := teamCLIEntry(spec)
	if err != nil {
		return err
	}
	args := spec.Args
	if entry != "" {
		args = args[1:]
	}
	if spec.Adapter == "harness" {
		if spec.Input != "dsh-jsonl" || len(args) != 3 || args[0] != "--profile" || args[1] != "multica" || args[2] != "--stdio" {
			return fmt.Errorf("Harness requires the fixed multica stdio profile and dsh-jsonl input")
		}
		return nil
	}
	if spec.Input != "goal" {
		return fmt.Errorf("CodeBuddy requires fixed stream-json output and goal input")
	}
	seen := map[string]bool{}
	for i := 0; i < len(args); i++ {
		name := args[i]
		if seen[name] {
			return fmt.Errorf("CodeBuddy command contains duplicate or unsupported protocol arguments")
		}
		seen[name] = true
		switch name {
		case "-p", "--verbose":
		case "--input-format", "--output-format", "--permission-mode", "--model", "--allowedTools":
			i++
			if i == len(args) {
				return fmt.Errorf("CodeBuddy command requires a fixed argument value")
			}
			value := args[i]
			if value == "" || len(value) > 1000 || strings.HasPrefix(value, "-") || strings.ContainsAny(value, "\x00\r\n") {
				return fmt.Errorf("CodeBuddy command requires a fixed argument value")
			}
			if name == "--input-format" && value != "text" || name == "--output-format" && value != "stream-json" || name == "--permission-mode" && value != "default" && value != "acceptEdits" {
				return fmt.Errorf("CodeBuddy command contains duplicate or unsupported protocol arguments")
			}
		default:
			return fmt.Errorf("CodeBuddy command contains duplicate or unsupported protocol arguments")
		}
	}
	if !seen["-p"] || !seen["--verbose"] || !seen["--input-format"] || !seen["--output-format"] {
		return fmt.Errorf("CodeBuddy requires fixed stream-json output and goal input")
	}
	return nil
}

func teamCLIVersion(ctx context.Context, exe string, spec CommandSpec, secrets []string) (string, error) {
	if err := validateTeamCLICommand(spec); err != nil {
		return "", err
	}
	entry, err := teamCLIEntry(spec)
	if err != nil {
		return "", err
	}
	probeArgs := func(args ...string) []string {
		if entry != "" {
			return append([]string{entry}, args...)
		}
		return args
	}
	version, err := probeCommand(ctx, exe, probeArgs("--version"), spec, secrets)
	if err != nil {
		return "", err
	}
	version = strings.TrimSpace(strings.SplitN(version, "\n", 2)[0])
	if version == "" || len(version) > 180 {
		return "", fmt.Errorf("CLI version is missing or oversized")
	}
	if spec.Adapter == "harness" {
		// The launcher's generic help cannot prove that this profile is present.
		// This explicit upstream probe exits without execute/model/installation.
		raw, err := probeCommandStreams(ctx, exe, probeArgs("--profile", "multica", "--probe"), spec, secrets, true)
		if err != nil {
			return "", fmt.Errorf("Harness multica runtime profile is unavailable or incompatible")
		}
		frame, err := decodeToolObject([]byte(strings.TrimSpace(raw)))
		if err != nil || textAt(frame, "type") != "probe" || textAt(frame, "runtime") != "dsh" {
			return "", fmt.Errorf("Harness multica runtime profile is unavailable or incompatible")
		}
		v, vok := nativeInteger(frame, "v")
		protocol, pok := nativeInteger(frame, "protocol_version")
		plugin := textAt(frame, "plugin_version")
		if !vok || !pok || v != 1 || protocol != 1 || plugin == "" || len(plugin) > 64 || strings.ContainsAny(plugin, "\x00\r\n") {
			return "", fmt.Errorf("Harness multica runtime profile is unavailable or incompatible")
		}
		return version + " / multica " + plugin, nil
	}
	help, err := probeCommand(ctx, exe, probeArgs("--help"), spec, secrets)
	if err != nil || !strings.Contains(help, "--input-format") || !strings.Contains(help, "--output-format") || !strings.Contains(help, "--verbose") {
		return "", fmt.Errorf("installed CodeBuddy help does not match the fixed stream-json protocol")
	}
	return version, nil
}
