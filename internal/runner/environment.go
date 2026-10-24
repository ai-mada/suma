package runner

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
)

// commandEnvironment carries ordinary operating-system context, never the
// complete parent environment. All other names require the local allowlist.
func commandEnvironment(spec CommandSpec) ([]string, []string, error) {
	names := []string{"PATH", "SystemRoot", "WINDIR", "HOME", "USERPROFILE", "TEMP", "TMP", "LANG"}
	if runtime.GOOS == "windows" {
		names = append(names, "SystemDrive", "ComSpec", "PATHEXT", "APPDATA", "LOCALAPPDATA", "ProgramData", "ALLUSERSPROFILE", "ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "CommonProgramFiles", "CommonProgramFiles(x86)", "CommonProgramW6432", "HOMEDRIVE", "HOMEPATH", "PUBLIC", "OS")
	}
	canonical := func(name string) string {
		if runtime.GOOS == "windows" {
			return strings.ToUpper(name)
		}
		return name
	}
	values := map[string]string{}
	for _, name := range names {
		if value := os.Getenv(name); value != "" {
			values[canonical(name)] = value
		}
	}
	secrets := []string{}
	set := func(name, value string) error {
		if name == "" || strings.ContainsAny(name, "=\x00") || strings.ContainsRune(value, 0) {
			return fmt.Errorf("invalid command environment")
		}
		values[canonical(name)] = value
		if value != "" && credential.MatchString(name+"="+value) {
			secrets = append(secrets, value)
		}
		return nil
	}
	keys := make([]string, 0, len(spec.Env))
	for name := range spec.Env {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	seen := map[string]bool{}
	for _, name := range keys {
		if (spec.Adapter == "suma" || spec.Adapter == "codex") && strings.EqualFold(name, "SUMA_RUNNER_TOKEN") {
			continue
		}
		key := canonical(name)
		if seen[key] {
			return nil, nil, fmt.Errorf("duplicate command environment name")
		}
		seen[key] = true
		if err := set(name, spec.Env[name]); err != nil {
			return nil, nil, err
		}
	}
	for _, name := range spec.EnvFrom {
		if (spec.Adapter == "suma" || spec.Adapter == "codex") && strings.EqualFold(name, "SUMA_RUNNER_TOKEN") {
			continue
		}
		if name == "" || strings.ContainsAny(name, "=\x00") {
			return nil, nil, fmt.Errorf("invalid forwarded environment name")
		}
		if value, ok := os.LookupEnv(name); ok {
			if err := set(name, value); err != nil {
				return nil, nil, err
			}
		}
	}
	keys = keys[:0]
	for name := range values {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, name := range keys {
		environment = append(environment, name+"="+values[name])
	}
	return environment, secrets, nil
}
