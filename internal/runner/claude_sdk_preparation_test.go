package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type sdkPreparationUnread struct{ reads int }

func (r *sdkPreparationUnread) Read([]byte) (int, error) { r.reads++; return 0, os.ErrInvalid }

func TestSDKPreparationRejectsOverwriteAndPrivateOverrides(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "commands.json")
	original := []byte("user-owned configuration\n")
	if err := os.WriteFile(existing, original, 0600); err != nil {
		t.Fatal(err)
	}
	input := &sdkPreparationUnread{}
	result, err := PrepareSDKToolCommands(context.Background(), existing, input)
	after, _ := os.ReadFile(existing)
	if err == nil || result.Prepared || input.reads != 0 || string(after) != string(original) {
		t.Fatal("existing configuration reached private read or overwrite")
	}
	fields := map[string]string{"adapter_id": "claude-code-sdk", "executable": filepath.Join(dir, "node.exe"), "host": filepath.Join(dir, "suma-claude.bundle.mjs"), "native_cli_executable": filepath.Join(dir, "claude.exe"), "model": "claude-sonnet-5", "credential_env_ref": "ANTHROPIC_API_KEY", "reviewed_cli_version": "2.1.314"}
	raw, _ := json.Marshal(fields)
	if _, err = DecodeSDKToolPreparationInput(raw); err != nil {
		t.Fatal(err)
	}
	fields["args"] = "--dangerously-skip-permissions"
	raw, _ = json.Marshal(fields)
	if _, err = DecodeSDKToolPreparationInput(raw); err == nil {
		t.Fatal("raw process arguments entered SDK preparation")
	}
	delete(fields, "args")
	fields["reviewed_cli_version"] = "latest"
	raw, _ = json.Marshal(fields)
	if _, err = DecodeSDKToolPreparationInput(raw); err == nil {
		t.Fatal("unreviewed native version admitted")
	}
	if _, err = DecodeSDKToolPreparationInput([]byte(strings.Repeat(" ", ACPToolPreparationInputLimit+1))); err == nil {
		t.Fatal("unbounded preparation input admitted")
	}
}
