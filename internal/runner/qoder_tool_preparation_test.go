package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func qoderPreparationInput221(t *testing.T) ACPToolPreparationInput {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return ACPToolPreparationInput{AdapterID: "qoder-acp", Executable: executable, ReviewedVersion: "administrator-reviewed-fixture221", Model: "explicit-model221", CredentialEnvRef: ""}
}

func TestQoderPreparationClosedFiveFieldsAndOptionalCredentialReference(t *testing.T) {
	input := qoderPreparationInput221(t)
	raw, _ := json.Marshal(input)
	decoded, err := DecodeACPToolPreparationInput(raw)
	if err != nil || decoded != input {
		t.Fatal("explicit native Qoder input with an empty credential name was rejected", err)
	}
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	for _, edit := range []func(map[string]any){
		func(v map[string]any) { delete(v, "credential_env_ref") },
		func(v map[string]any) { v["credential_env_ref"] = nil },
		func(v map[string]any) { v["credential_env_ref"] = "SUMA_RUNNER_TOKEN" },
		func(v map[string]any) { v["credential_env_ref"] = "HOME" },
		func(v map[string]any) { v["adapter_id"] = "unregistered-acp" },
		func(v map[string]any) { v["model"] = "" },
		func(v map[string]any) { v["reviewed_version"] = strings.Repeat("x", 257) },
		func(v map[string]any) { v["executable"] = "relative.exe" },
		func(v map[string]any) { v["args"] = []string{"--yolo"} },
	} {
		copy := map[string]any{}
		for key, value := range fields {
			copy[key] = value
		}
		edit(copy)
		candidate, _ := json.Marshal(copy)
		if _, err := DecodeACPToolPreparationInput(candidate); err == nil {
			t.Fatal("unsafe or incomplete private input was accepted")
		}
	}
	for index, candidate := range [][]byte{nil, []byte("null"), append(append([]byte{}, raw...), []byte(" {}")...), append([]byte(`{"model":"duplicate",`), raw[1:]...), bytes.Replace(raw, []byte(`"model":"explicit-model221"`), []byte(`"model":"\ud800"`), 1), bytes.Repeat([]byte(" "), ACPToolPreparationInputLimit+1)} {
		if _, err := DecodeACPToolPreparationInput(candidate); err == nil {
			t.Fatal("nonclosed private JSON was accepted", index)
		}
	}
	input.CredentialEnvRef = "LOCAL_QODER_CREDENTIAL"
	raw, _ = json.Marshal(input)
	if _, err := DecodeACPToolPreparationInput(raw); err != nil {
		t.Fatal("explicit credential name was rejected", err)
	}
	cursor := ACPToolPreparationInput{AdapterID: "cursor-cli", Executable: filepath.Join(t.TempDir(), cursorNodeName()), ReviewedVersion: "reviewed-cursor", Model: "fixed-model", CredentialEnvRef: ""}
	raw, _ = json.Marshal(cursor)
	if _, err := DecodeACPToolPreparationInput(raw); err == nil {
		t.Fatal("Qoder optional credential weakened Cursor's nonempty credential contract")
	}
}

func TestPrepareQoderCommandsWritesOnlyFixedNativeConfigurationWithoutExecution(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("SUMA_RUNNER_TOKEN", "private-platform-value221")
	t.Setenv("LOCAL_QODER_CREDENTIAL", "private-provider-value221")
	for _, credential := range []string{"", "LOCAL_QODER_CREDENTIAL"} {
		input := qoderPreparationInput221(t)
		input.CredentialEnvRef = credential
		raw, _ := json.Marshal(input)
		file := filepath.Join(t.TempDir(), "qoder-commands.json")
		result, err := PrepareACPToolCommands(context.Background(), file, bytes.NewReader(raw))
		if err != nil || !result.Prepared || result.Protocol != ToolPreparationProtocol || result.CommandCount != 1 || len(result.Remaining) != 0 || result.File != file {
			t.Fatal("static native configuration was not prepared", result, err)
		}
		saved, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		commands, err := DecodeCommands(saved)
		want, errSpec := acpCommandForAdapter(input.AdapterID, input.Executable, input.ReviewedVersion, input.Model, credential)
		if err != nil || errSpec != nil || len(commands) != 1 || !samePermissionValue(commands[QoderToolPreparationCommandID], want) {
			t.Fatal("Qoder preparation changed the original fixed ACP protocol", err, errSpec)
		}
		if bytes.Contains(saved, []byte("private-platform-value221")) || bytes.Contains(saved, []byte("private-provider-value221")) || bytes.Contains(saved, []byte("--yolo")) {
			t.Fatal("credential values or unreviewed policy entered configuration")
		}
		reader := &preparationReader191{}
		if _, err := PrepareACPToolCommands(context.Background(), file, reader); err == nil || reader.reads != 0 {
			t.Fatal("existing output was not rejected before private input")
		}
		after, _ := os.ReadFile(file)
		if !bytes.Equal(saved, after) {
			t.Fatal("original new file was overwritten")
		}
	}
}

func TestPrepareQoderRejectsChangedNativeInvalidTargetAndCancellation(t *testing.T) {
	input := qoderPreparationInput221(t)
	root := t.TempDir()
	native := filepath.Join(root, "owned-native.exe")
	// This is an explicitly inert format fixture for static pin tests, never a
	// real Qoder binary or an executable-process acceptance test.
	if err := os.WriteFile(native, []byte("MZowned static format221"), 0700); err != nil {
		t.Fatal(err)
	}
	input.Executable = native
	pinned, err := snapshotQoderToolPreparation(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	file, parent, err := openToolCommandDestination(filepath.Join(t.TempDir(), "changed.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if err = os.WriteFile(native, []byte("MZchanged static format221"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = pinned.write(context.Background(), file, parent); err == nil {
		t.Fatal("changed installation reused an old native pin")
	}
	if _, err = os.Lstat(file); !os.IsNotExist(err) {
		t.Fatal("changed source created output")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := &preparationReader191{}
	if _, err = PrepareACPToolCommands(ctx, filepath.Join(t.TempDir(), "cancelled.json"), reader); err == nil || reader.reads != 0 {
		t.Fatal("cancelled preparation read private input")
	}
	if err = os.WriteFile(native, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(input)
	invalidFile := filepath.Join(t.TempDir(), "invalid.json")
	if _, err = PrepareACPToolCommands(context.Background(), invalidFile, bytes.NewReader(raw)); err == nil {
		t.Fatal("script or invalid native format accepted")
	}
	if _, err = os.Lstat(invalidFile); !os.IsNotExist(err) {
		t.Fatal("invalid native target created output")
	}
}
