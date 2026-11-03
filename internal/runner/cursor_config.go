package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func cursorNodeName() string {
	if runtime.GOOS == "windows" {
		return "node.exe"
	}
	return "node"
}
func cursorArguments(root string) []string {
	return []string{filepath.Join(root, "index.js"), "--disable-auto-update", "--sandbox", "enabled", "acp"}
}

// BuildCursorACPCommand is the local administrator's fixed package entry.
// It hashes the complete dependency closure; no renderer argv is accepted.
func BuildCursorACPCommand(root, version, model, credential string) (CommandSpec, error) {
	pack, node, err := inspectCursorPackage(root)
	if err != nil {
		return CommandSpec{}, err
	}
	spec := CommandSpec{Adapter: "cursor", Protocol: ac.Protocol, Executable: node, Args: cursorArguments(pack.PackageRoot), Input: "acp-stdio", ACP: &ACPExecutionSpec{Runtime: "cursor", ReviewedVersion: version, Model: model, CredentialEnvRef: credential, CursorPackage: &pack}}
	if err = validateACPExecutionSpec(spec); err != nil {
		return CommandSpec{}, err
	}
	return spec, nil
}
func validateCursorCommand(spec CommandSpec) error {
	p := spec.ACP
	if p == nil || p.CursorPackage == nil || !desktopManagedPath(p.CursorPackage.PackageRoot) || !tc.ValidDigest(p.CursorPackage.SHA256) || p.CredentialEnvRef == "" || !sameACPPath(spec.Executable, filepath.Join(p.CursorPackage.PackageRoot, cursorNodeName())) || !slices.Equal(spec.Args, cursorArguments(p.CursorPackage.PackageRoot)) {
		return fmt.Errorf("Cursor requires its fixed reviewed package, sandbox and explicit credential reference")
	}
	return nil
}
func cursorCredential(spec CommandSpec) (string, error) {
	value, ok := os.LookupEnv(spec.ACP.CredentialEnvRef)
	if !ok || value == "" || len(value) > 16384 || strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("Cursor explicit credential reference is unavailable")
	}
	return value, nil
}
func cursorPackageUnchanged(spec CursorPackageSpec) bool {
	_, err := validateCursorPackage(spec)
	return err == nil
}
func cursorEnvironment(spec CommandSpec, home, source string, credential bool) ([]string, []string, error) {
	if err := validateACPExecutionSpec(spec); err != nil || spec.ACP.Runtime != "cursor" {
		return nil, nil, fmt.Errorf("Cursor fixed configuration unavailable")
	}
	if !filepath.IsAbs(home) || !filepath.IsAbs(source) || within(source, home) {
		return nil, nil, fmt.Errorf("Cursor home must be outside the attempt source")
	}
	if err := safeMkdir(home); err != nil {
		return nil, nil, fmt.Errorf("Cursor isolated home unavailable")
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		return nil, nil, fmt.Errorf("Cursor requires a fresh empty isolated home")
	}
	config, temp := filepath.Join(home, "cursor-config"), filepath.Join(home, "tmp")
	for _, directory := range []string{config, temp, filepath.Join(home, "cache")} {
		if err = safeMkdir(directory); err != nil {
			return nil, nil, fmt.Errorf("Cursor isolated configuration unavailable")
		}
	}
	base, _, err := commandEnvironment(CommandSpec{})
	if err != nil {
		return nil, nil, err
	}
	env := []string{}
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if map[string]bool{"PATH": true, "SYSTEMROOT": true, "SYSTEMDRIVE": true, "WINDIR": true, "COMSPEC": true, "PATHEXT": true, "LANG": true}[strings.ToUpper(name)] {
			env = append(env, entry)
		}
	}
	env = append(env, "HOME="+home, "USERPROFILE="+home, "APPDATA="+home, "LOCALAPPDATA="+home, "CURSOR_CONFIG_DIR="+config, "XDG_CONFIG_HOME="+config, "XDG_CACHE_HOME="+filepath.Join(home, "cache"), "TEMP="+temp, "TMP="+temp, "TMPDIR="+temp, "NODE_COMPILE_CACHE="+filepath.Join(home, "cache"), "DO_NOT_TRACK=1")
	secrets := []string{}
	if credential {
		value, e := cursorCredential(spec)
		if e != nil {
			return nil, nil, e
		}
		env = append(env, "CURSOR_API_KEY="+value)
		secrets = append(secrets, value)
	}
	return env, secrets, nil
}

func cursorVersion(parent context.Context, spec CommandSpec, secrets []string) (string, error) {
	value, err := cursorCredential(spec)
	if err != nil {
		return "", err
	}
	secrets = append(append([]string{}, secrets...), value)
	version, err := probeCursorVersion(parent, spec, secrets)
	if err != nil {
		return "", err
	}
	if version != spec.ACP.ReviewedVersion {
		return "", newProbeFailure("version", "version_unsupported", nil)
	}
	return version, nil
}

// Installation discovery supplies no key and never opens an ACP session.
func probeCursorVersion(parent context.Context, spec CommandSpec, secrets []string) (string, error) {
	if _, err := validateCursorPackage(*spec.ACP.CursorPackage); err != nil {
		return "", err
	}
	directory, cleanup, err := newACPProbeDirectory()
	if err != nil {
		return "", err
	}
	parent, closeScope, err := bindPrivateProbeDirectory(parent, directory, cleanup)
	if err != nil {
		return "", err
	}
	defer closeScope()
	source := filepath.Join(directory, "source")
	if err = safeMkdir(source); err != nil {
		return "", err
	}
	if err = validateCursorWorkspace(source); err != nil {
		return "", err
	}
	env, _, err := cursorEnvironment(spec, filepath.Join(directory, "home"), source, false)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	cmd := exec.Command(spec.Executable, filepath.Join(spec.ACP.CursorPackage.PackageRoot, "index.js"), "--disable-auto-update", "--version")
	cmd.Dir, cmd.Env = source, env
	out, diagnostic := newOutput(32<<10, secrets), newOutput(32<<10, secrets)
	cmd.Stdout, cmd.Stderr = out, diagnostic
	waitErr, stopErr := runProbeProcess(ctx, cmd)
	out.mu.Lock()
	raw, truncated := out.b.String(), out.truncated
	out.mu.Unlock()
	diagnostic.mu.Lock()
	truncated = truncated || diagnostic.truncated
	diagnostic.mu.Unlock()
	version := strings.TrimSpace(raw)
	if failure := probeProcessFailure(ctx, "version", waitErr, stopErr, truncated); failure != nil {
		return "", failure
	}
	if !acpLocalText(version, 256) || strings.TrimSpace(out.String()) != version {
		return "", newProbeFailure("version", "probe_failed", nil)
	}
	if _, err = validateCursorPackage(*spec.ACP.CursorPackage); err != nil {
		return "", err
	}
	return version, nil
}
