package runner

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type preparationReader191 struct{ reads int }

func (r *preparationReader191) Read([]byte) (int, error) { r.reads++; return 0, io.EOF }
func preparationInput191(t *testing.T) (map[string]string, CommandSpec) {
	t.Helper()
	spec := cursorConfigFixture(t)
	return map[string]string{"adapter_id": "cursor-cli", "executable": spec.Executable, "reviewed_version": spec.ACP.ReviewedVersion, "model": spec.ACP.Model, "credential_env_ref": spec.ACP.CredentialEnvRef}, spec
}
func TestPrepareACPCommandsWritesOneFixedConfigurationWithoutExecution(t *testing.T) {
	input, want := preparationInput191(t)
	raw, _ := json.Marshal(input)
	t.Setenv("PATH", "")
	t.Setenv(want.ACP.CredentialEnvRef, "")
	t.Setenv("SUMA_RUNNER_TOKEN", "never-copy-platform-token191")
	file := filepath.Join(t.TempDir(), "cursor-commands.json")
	result, err := PrepareACPToolCommands(context.Background(), file, strings.NewReader(string(raw)))
	if err != nil || !result.Prepared || result.Protocol != ToolPreparationProtocol || result.CommandCount != 1 || result.File != file || len(result.Remaining) != 0 {
		t.Fatal("configuration facts were not prepared", result, err)
	}
	bytes, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := DecodeCommands(bytes)
	if err != nil || len(commands) != 1 || !samePermissionValue(commands["local-cursor-cli"], want) {
		t.Fatal("preparation changed the fixed Cursor command", err)
	}
	if strings.Contains(string(bytes), "never-copy-platform-token191") || strings.Contains(string(bytes), "--force") || strings.Contains(string(bytes), "--yolo") {
		t.Fatal("secrets or arbitrary execution policy entered prepared file")
	}
	if commands["local-cursor-cli"].ACP.CursorPackage.SHA256 == "" || ACPConfigurationDigest(*commands["local-cursor-cli"].ACP) == "" {
		t.Fatal("closure/configuration was not bound")
	}
}

func TestPrepareACPDestinationIsRejectedBeforeReadingInput(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "existing.json")
	if err := os.WriteFile(file, []byte("original191"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, root, filepath.Join(root, "missing", "commands.json")} {
		reader := &preparationReader191{}
		if _, err := PrepareACPToolCommands(context.Background(), path, reader); err == nil || reader.reads != 0 {
			t.Fatal("unsafe destination consumed input or was accepted")
		}
	}
	bytes, _ := os.ReadFile(file)
	if string(bytes) != "original191" {
		t.Fatal("existing original changed")
	}
	if runtime.GOOS == "windows" {
		for _, alias := range []string{root + ".", root + " "} {
			reader := &preparationReader191{}
			if _, err := PrepareACPToolCommands(context.Background(), filepath.Join(alias, "alias.json"), reader); err == nil || reader.reads != 0 {
				t.Fatal("Windows destination alias consumed input before rejection")
			}
		}
	}
	input, spec := preparationInput191(t)
	raw, _ := json.Marshal(input)
	inside := filepath.Join(spec.ACP.CursorPackage.PackageRoot, "commands.json")
	if _, err := PrepareACPToolCommands(context.Background(), inside, strings.NewReader(string(raw))); err == nil {
		t.Fatal("configuration file was placed inside its own package closure")
	}
	if _, err := os.Lstat(inside); !os.IsNotExist(err) {
		t.Fatal("package closure was modified")
	}
	if _, err := validateCursorPackage(*spec.ACP.CursorPackage); err != nil {
		t.Fatal("preparation changed the reviewed package", err)
	}
}

func TestPrepareACPRejectsClosedInputAndWrongNodeWithoutFile(t *testing.T) {
	input, _ := preparationInput191(t)
	raw, _ := json.Marshal(input)
	bad := []string{"", `null`, string(raw) + `{}`, strings.TrimSuffix(string(raw), "}") + `,"args":["--force"]}`, strings.TrimSuffix(string(raw), "}") + `,"model":"duplicate"}`, strings.Replace(string(raw), `"adapter_id":"cursor-cli"`, `"adapter_id":"unregistered-acp"`, 1), strings.Replace(string(raw), `"model":"fixed-model189"`, `"model":null`, 1), strings.Repeat("x", (16<<10)+1)}
	for _, field := range []string{"reviewed_version", "model", "credential_env_ref"} {
		copy := map[string]string{}
		for key, value := range input {
			copy[key] = value
		}
		delete(copy, field)
		value, _ := json.Marshal(copy)
		bad = append(bad, string(value))
	}
	for _, edit := range []func(map[string]string){func(v map[string]string) {
		v["executable"] = filepath.Join(filepath.Dir(v["executable"]), "cursor.cmd")
	}, func(v map[string]string) { v["credential_env_ref"] = "SUMA_RUNNER_TOKEN" }} {
		copy := map[string]string{}
		for key, value := range input {
			copy[key] = value
		}
		edit(copy)
		value, _ := json.Marshal(copy)
		bad = append(bad, string(value))
	}
	for i, value := range bad {
		file := filepath.Join(t.TempDir(), "commands.json")
		if _, err := PrepareACPToolCommands(context.Background(), file, strings.NewReader(value)); err == nil {
			t.Fatal("invalid private input accepted", i)
		}
		if _, err := os.Lstat(file); !os.IsNotExist(err) {
			t.Fatal("invalid input created a file", i)
		}
	}
}
