package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func compositionSource(t *testing.T, directory, name, body string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func compositionInput(t *testing.T, files ...string) []byte {
	t.Helper()
	raw, err := json.Marshal(struct {
		Files []string `json:"files"`
	}{files})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func compositionFailure(t *testing.T, err error, code string, source int) {
	t.Helper()
	var failure *CommandCompositionError
	if !errors.As(err, &failure) || failure.Code != code || failure.SourceIndex != source {
		t.Fatalf("incorrect safe failure: %#v (%v), want %s/%d", failure, err, code, source)
	}
	if strings.Contains(err.Error(), "private-") || strings.Contains(err.Error(), "never-execute") {
		t.Fatal("private source content or path leaked in error")
	}
}

func TestCommandCompositionCombinesCrossSourceChecksWithoutChangingOriginals(t *testing.T) {
	directory := t.TempDir()
	developer := map[string]CommandSpec{"develop": {
		Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: "never-execute-developer", Args: []string{filepath.Join(directory, "owned.bundle.mjs")}, Input: "agent-jsonl", ModelExecutionProtocol: "suma-model-execution-v1",
		NativeWorkspace: &NativeWorkspacePolicy{Checks: []string{"check"}, RuntimeManifest: filepath.Join(directory, "owned.manifest.json")},
	}}
	rawDeveloper, err := json.Marshal(developer)
	if err != nil {
		t.Fatal(err)
	}
	// A source fragment may intentionally lack a referenced check. Only the
	// complete composed allowlist is executable configuration authority.
	if _, err = DecodeCommands(rawDeveloper); err == nil {
		t.Fatal("standalone incomplete developer source unexpectedly passed")
	}
	first := compositionSource(t, directory, "private-developer.json", string(rawDeveloper))
	secondBody := `{"check":{"adapter":"command","protocol":"text-v1","executable":"never-execute-check","args":["--check"]},"independent":{"adapter":"command","protocol":"text-v1","executable":"never-execute-test","args":["--junitxml={{test_report_path}}"],"test_evidence":true,"test_report":true}}`
	second := compositionSource(t, directory, "private-checks.json", secondBody)
	output := filepath.Join(directory, "composed.json")
	report, err := PrepareCommandComposition(context.Background(), output, bytes.NewReader(compositionInput(t, first, second)))
	if err != nil || report.Protocol != "suma-command-composition-v1" || report.SourceCount != 2 || report.CommandCount != 3 {
		t.Fatalf("composition failed: %+v %v", report, err)
	}
	composed, err := os.ReadFile(output)
	if err != nil || len(composed) == 0 || composed[len(composed)-1] != '\n' || report.ConfigurationSHA256 != digest(composed) {
		t.Fatal("receipt does not identify the exact persisted bytes", err)
	}
	commands, err := DecodeCommands(composed)
	if err != nil || len(commands) != 3 || commands["develop"].NativeWorkspace.Checks[0] != "check" || !commands["independent"].TestEvidence || !commands["independent"].TestReport || commands["check"].TestEvidence {
		t.Fatal("composition changed original checks or independent-test facts", err)
	}
	for path, want := range map[string][]byte{first: rawDeveloper, second: []byte(secondBody)} {
		got, readErr := os.ReadFile(path)
		if readErr != nil || !bytes.Equal(got, want) {
			t.Fatal("original source changed", readErr)
		}
	}
	safe, _ := json.Marshal(report)
	var fields map[string]any
	_ = json.Unmarshal(safe, &fields)
	if len(fields) != 4 || bytes.Contains(safe, []byte(directory)) || bytes.Contains(safe, []byte("never-execute")) {
		t.Fatal("composition receipt leaked paths or command configuration")
	}
	reader := &preparationReader191{}
	_, err = PrepareCommandComposition(context.Background(), output, reader)
	compositionFailure(t, err, "output_exists", 0)
	if reader.reads != 0 {
		t.Fatal("existing output consumed private input")
	}
	after, _ := os.ReadFile(output)
	if !bytes.Equal(composed, after) {
		t.Fatal("existing output was overwritten")
	}
}

func TestCommandCompositionRejectsInvalidSourcesAndAmbiguousConfiguration(t *testing.T) {
	directory := t.TempDir()
	first := compositionSource(t, directory, "private-first.json", `{"first":{"executable":"never-execute-first"}}`)
	second := compositionSource(t, directory, "private-second.json", `{"second":{"executable":"never-execute-second"}}`)
	for name, raw := range map[string][]byte{
		"null": []byte(`null`), "unknown": []byte(`{"files":[],"command":"private-secret"}`), "missing": []byte(`{}`), "one": compositionInput(t, first), "too-many": compositionInput(t, append(make([]string, 16), second)...),
		"duplicate-key": []byte(`{"files":[],"files":[]}`), "null-path": []byte(`{"files":[null,null]}`), "trailing": append(compositionInput(t, first, second), []byte(`{}`)...), "oversize": bytes.Repeat([]byte(" "), (64<<10)+1),
	} {
		t.Run(name, func(t *testing.T) {
			output := filepath.Join(directory, name+"-output.json")
			_, err := PrepareCommandComposition(context.Background(), output, bytes.NewReader(raw))
			compositionFailure(t, err, "input_invalid", 0)
			if _, err = os.Lstat(output); !os.IsNotExist(err) {
				t.Fatal("invalid input created an output")
			}
		})
	}
	for name, body := range map[string]string{
		"empty": `{}`, "null-object": `null`, "null-command": `{"other":null}`, "scalar-command": `{"other":true}`, "unknown-field": `{"other":{"executable":"never-execute","secret":"private-secret"}}`,
		"duplicate-command": `{"other":{"executable":"one"},"other":{"executable":"two"}}`, "duplicate-field": `{"other":{"executable":"one","executable":"two"}}`, "surrogate": `{"other":{"executable":"private-\ud800"}}`, "trailing-source": `{} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			bad := compositionSource(t, directory, name+".json", body)
			_, err := PrepareCommandComposition(context.Background(), filepath.Join(directory, name+"-output.json"), bytes.NewReader(compositionInput(t, first, bad)))
			compositionFailure(t, err, "source_invalid", 2)
		})
	}
	duplicate := compositionSource(t, directory, "duplicate-id.json", `{"first":{"executable":"different-private-command"}}`)
	_, err := PrepareCommandComposition(context.Background(), filepath.Join(directory, "duplicate-output.json"), bytes.NewReader(compositionInput(t, first, duplicate)))
	compositionFailure(t, err, "duplicate_command", 2)
	_, err = PrepareCommandComposition(context.Background(), filepath.Join(directory, "same-source-output.json"), bytes.NewReader(compositionInput(t, first, first)))
	compositionFailure(t, err, "source_invalid", 2)
	hardlink := filepath.Join(directory, "same-source-hardlink.json")
	if err = os.Link(first, hardlink); err != nil {
		t.Fatal("local hardlink fixture unavailable", err)
	}
	_, err = PrepareCommandComposition(context.Background(), filepath.Join(directory, "hardlink-output.json"), bytes.NewReader(compositionInput(t, first, hardlink)))
	compositionFailure(t, err, "source_invalid", 2)
	_, err = PrepareCommandComposition(context.Background(), filepath.Join(directory, "missing-output.json"), bytes.NewReader(compositionInput(t, first, filepath.Join(directory, "missing.json"))))
	compositionFailure(t, err, "source_unavailable", 2)
	// Structurally valid source fragments still pass only the original complete
	// configuration validator. A dangling workspace check cannot be promoted.
	badPolicy, _ := json.Marshal(map[string]CommandSpec{"develop": {Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: "never-execute", Input: "agent-jsonl", ModelExecutionProtocol: "suma-model-execution-v1", NativeWorkspace: &NativeWorkspacePolicy{Checks: []string{"missing"}, RuntimeManifest: filepath.Join(directory, "manifest.json")}}})
	bad := compositionSource(t, directory, "dangling-policy.json", string(badPolicy))
	_, err = PrepareCommandComposition(context.Background(), filepath.Join(directory, "dangling-output.json"), bytes.NewReader(compositionInput(t, first, bad)))
	compositionFailure(t, err, "configuration_invalid", 0)
}

func TestCommandCompositionGuardsBoundsCancellationAndSourceStability(t *testing.T) {
	directory := t.TempDir()
	reader := &preparationReader191{}
	_, err := PrepareCommandComposition(context.Background(), "relative-output.json", reader)
	compositionFailure(t, err, "output_invalid", 0)
	if reader.reads != 0 {
		t.Fatal("relative output consumed private input")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = PrepareCommandComposition(ctx, filepath.Join(directory, "cancelled.json"), reader)
	compositionFailure(t, err, "cancelled", 0)
	if reader.reads != 0 {
		t.Fatal("cancelled composition consumed private input")
	}
	first := compositionSource(t, directory, "private-first.json", `{"first":{"executable":"never-execute"}}`)
	snapshot, err := inspectPoolPreparationFile(first, poolPreparationFileLimit)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(first, []byte(`{"changed":{"executable":"never-execute"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	compositionFailure(t, validateCommandCompositionSources(context.Background(), []poolPreparationFile{snapshot}), "inputs_changed", 1)
	second := compositionSource(t, directory, "private-second.json", `{"second":{"executable":"never-execute"}}`)
	oversize := compositionSource(t, directory, "oversize.json", strings.Repeat(" ", (2<<20)+1))
	_, err = PrepareCommandComposition(context.Background(), filepath.Join(directory, "oversize-output.json"), bytes.NewReader(compositionInput(t, first, oversize)))
	compositionFailure(t, err, "source_unavailable", 2)
	// Whitespace counts as actual source bytes too; compact decoded objects
	// must not let sixteen near-limit sources evade the cumulative read bound.
	large := compositionSource(t, directory, "large.json", `{"large":{"executable":"never-execute"}}`+strings.Repeat(" ", 1<<20))
	largeOther := compositionSource(t, directory, "large-other.json", `{"large-other":{"executable":"never-execute"}}`+strings.Repeat(" ", 1<<20))
	_, err = PrepareCommandComposition(context.Background(), filepath.Join(directory, "cumulative-output.json"), bytes.NewReader(compositionInput(t, large, largeOther)))
	compositionFailure(t, err, "source_unavailable", 2)
	_, err = PrepareCommandComposition(context.Background(), filepath.Join(directory, "bad-path-output.json"), bytes.NewReader(compositionInput(t, second, "relative-source.json")))
	compositionFailure(t, err, "source_unavailable", 2)
	for file, count := range []int{64, 65} {
		commands := map[string]CommandSpec{}
		for index := 0; index < count; index++ {
			commands["command-"+strconv.Itoa(file)+"-"+strconv.Itoa(index)] = CommandSpec{Executable: "never-execute"}
		}
		raw, marshalErr := json.Marshal(commands)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		compositionSource(t, directory, "command-limit-"+strconv.Itoa(file)+".json", string(raw))
	}
	_, err = PrepareCommandComposition(context.Background(), filepath.Join(directory, "command-limit-output.json"), bytes.NewReader(compositionInput(t, filepath.Join(directory, "command-limit-0.json"), filepath.Join(directory, "command-limit-1.json"))))
	compositionFailure(t, err, "configuration_invalid", 0)
	arguments := []string{strings.Repeat("x", (2<<20)-1200)}
	for index := 0; index < 96; index++ {
		arguments = append(arguments, "short")
	}
	compactCommands := map[string]CommandSpec{"expanded": {Executable: "never-execute", Args: arguments}}
	compact, _ := json.Marshal(compactCommands)
	complete := map[string]CommandSpec{"expanded": compactCommands["expanded"], "second": {Executable: "never-execute"}}
	pretty, _ := json.MarshalIndent(complete, "", "  ")
	secondBytes, _ := os.ReadFile(second)
	if len(compact)+len(secondBytes) > 2<<20 || len(pretty)+1 <= 2<<20 {
		t.Fatal("output expansion fixture does not exercise separate source and final bounds")
	}
	expanded := compositionSource(t, directory, "expanded.json", string(compact))
	_, err = PrepareCommandComposition(context.Background(), filepath.Join(directory, "expanded-output.json"), bytes.NewReader(compositionInput(t, expanded, second)))
	compositionFailure(t, err, "configuration_invalid", 0)
}
