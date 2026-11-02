package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type poolDiagnosticBrokenInput211 struct{}

func (poolDiagnosticBrokenInput211) Read(p []byte) (int, error) {
	return copy(p, []byte(`{"private-read-error211":`)), errors.New("private-reader-cause211")
}

func TestPoolPreparationDiagnosticFactsBindOnlyCompleteInputAndSafeLocations(t *testing.T) {
	input := poolPreparationFixture209(t)
	directory := filepath.Dir(input.Scopes[0].Workspace)
	second := input.Scopes[0]
	second.ProjectID = "second211"
	second.Workspace = filepath.Join(directory, "workspace-two")
	if err := os.Mkdir(second.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	input.Scopes = append(input.Scopes, second)
	validRaw, _ := json.Marshal(input)
	assertFailure := func(ctx context.Context, selected PoolConfigPreparationInput, reader io.Reader, destination, code, field string, row int, inputSHA string) {
		t.Helper()
		if reader == nil {
			raw, _ := json.Marshal(selected)
			reader = bytes.NewReader(raw)
			if inputSHA == "bound" {
				inputSHA = digest(raw)
			}
		}
		_, err := PreparePoolLocalConfig(ctx, destination, reader)
		var failure *PoolConfigPreparationError
		if !errors.As(err, &failure) || failure.Code != code || failure.Field != field || failure.ScopeIndex != row || failure.InputSHA256 != inputSHA || failure.OutputSHA256 != digest([]byte(destination)) {
			t.Fatalf("unexpected safe failure facts: %#v err=%v", failure, err)
		}
		if strings.Contains(err.Error(), directory) || strings.Contains(err.Error(), "private") {
			t.Fatal("error string included private cause")
		}
	}
	assertFailure(context.Background(), input, nil, "relative211.json", "output_invalid", "output", 0, "")
	existing := filepath.Join(directory, "existing.json")
	if err := os.WriteFile(existing, []byte("private-original211"), 0600); err != nil {
		t.Fatal(err)
	}
	reader := &preparationReader191{}
	assertFailure(context.Background(), input, reader, existing, "output_exists", "output", 0, "")
	if reader.reads != 0 {
		t.Fatal("existing output consumed private input")
	}
	assertFailure(context.Background(), input, nil, filepath.Join(directory, "missing", "output.json"), "output_unavailable", "output", 0, "")
	for _, field := range []string{"workspace", "repository", "worktrees"} {
		selected := input
		selected.Scopes = append([]PoolConfigPreparationScope{}, input.Scopes...)
		switch field {
		case "workspace":
			selected.Scopes[1].Workspace = filepath.Join(directory, "absent-workspace")
		case "repository":
			selected.Scopes[1].Repository = filepath.Join(directory, "absent-repository")
			selected.Scopes[1].BaseCommit = strings.Repeat("a", 40)
		case "worktrees":
			selected.Scopes[1].Worktrees = filepath.Join(directory, "absent-worktrees")
		}
		assertFailure(context.Background(), selected, nil, filepath.Join(directory, field+".json"), "directory_unavailable", field, 2, "bound")
	}
	selected := input
	selected.Scopes = append([]PoolConfigPreparationScope{}, input.Scopes...)
	selected.Scopes[1].Workspace = input.Scopes[0].Workspace
	assertFailure(context.Background(), selected, nil, filepath.Join(directory, "conflict.json"), "workspace_conflict", "workspace", 2, "bound")
	for _, field := range []string{"commands_file", "model_deployments_file", "external_config_file"} {
		for _, available := range []bool{false, true} {
			selected := input
			selected.Scopes = append([]PoolConfigPreparationScope{}, input.Scopes...)
			path := filepath.Join(directory, field+"-private-source211.json")
			expected := "source_unavailable"
			if available {
				expected = "source_invalid"
				if err := os.WriteFile(path, []byte(`{"private-source-content211":`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch field {
			case "commands_file":
				selected.Scopes[1].CommandsFile = path
			case "model_deployments_file":
				selected.Scopes[1].ModelDeploymentsFile = path
			case "external_config_file":
				selected.Scopes[1].ExternalConfigFile = path
			}
			assertFailure(context.Background(), selected, nil, filepath.Join(directory, expected+field+".json"), expected, field, 2, "bound")
		}
	}
	// Whitespace is excluded only after a complete bounded read; incomplete and
	// oversize streams never claim the private draft's digest.
	assertFailure(context.Background(), input, bytes.NewReader([]byte(" \r\n{}\n")), filepath.Join(directory, "invalid-input.json"), "input_invalid", "input", 0, digest([]byte(`{}`)))
	assertFailure(context.Background(), input, bytes.NewReader(bytes.Repeat([]byte("x"), poolPreparationInputLimit+1)), filepath.Join(directory, "too-large.json"), "input_invalid", "input", 0, "")
	assertFailure(context.Background(), input, poolDiagnosticBrokenInput211{}, filepath.Join(directory, "read-error.json"), "input_invalid", "input", 0, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assertFailure(ctx, input, bytes.NewReader(validRaw), filepath.Join(directory, "cancelled.json"), "cancelled", "input", 0, "")
	var standalone *PoolConfigPreparationError
	_, err := DecodePoolConfigPreparationInput([]byte(`{}`))
	if !errors.As(err, &standalone) || standalone.Code != "input_invalid" || standalone.InputSHA256 != digest([]byte(`{}`)) || standalone.OutputSHA256 != "" {
		t.Fatal("standalone decoder lost typed input facts")
	}
}
