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

func poolPreparationFixture209(t *testing.T) PoolConfigPreparationInput {
	t.Helper()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	return PoolConfigPreparationInput{PoolID: "owned-pool209", RunnerID: "owned-runner209", Slots: 2, Scopes: []PoolConfigPreparationScope{{TenantID: "tenant209", ProjectID: "project209", Workspace: workspace}}}
}

func TestPreparePoolConfigComposesOriginalLocalContractWithoutExecution(t *testing.T) {
	input := poolPreparationFixture209(t)
	directory := filepath.Dir(input.Scopes[0].Workspace)
	commands := filepath.Join(directory, "commands.json")
	// This intentionally uninstalled executable is a static allowlist entry,
	// not a version probe. Preparation must not resolve or invoke it.
	commandBytes := []byte(`{"owned-check209":{"executable":"never-execute-pool209","args":[],"read_only":true,"env_from":["OWNED_REFERENCE209"]}}`)
	if err := os.WriteFile(commands, commandBytes, 0600); err != nil {
		t.Fatal(err)
	}
	models := filepath.Join(directory, "models.json")
	if err := os.WriteFile(models, []byte(`[]`), 0600); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(directory, "external.json")
	externalBytes := []byte(`{"owned-provider209":{"url":"https://provider.example.test/repository","token_env":"OWNED_REFERENCE209","protocol":"suma.repository-integration.v1"}}`)
	if err := os.WriteFile(external, externalBytes, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OWNED_REFERENCE209", "private-value-not-read209")
	input.Scopes[0].CommandsFile, input.Scopes[0].ModelDeploymentsFile, input.Scopes[0].ExternalConfigFile = commands, models, external
	second := input.Scopes[0]
	second.ProjectID, second.Workspace = "project210", filepath.Join(directory, "workspace-two")
	second.CommandsFile, second.ModelDeploymentsFile, second.ExternalConfigFile = "", "", ""
	if err := os.Mkdir(second.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	input.Scopes = append(input.Scopes, second)
	raw, _ := json.Marshal(input)
	file := filepath.Join(directory, "pool.json")
	report, err := PreparePoolLocalConfig(context.Background(), file, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	config, err := DecodePoolConfig(saved)
	if err != nil || config.SchemaVersion != 1 || config.PoolID != input.PoolID || config.RunnerID != input.RunnerID || config.Slots != 2 || len(config.Scopes) != 2 || config.PollIntervalMS != 0 || config.HeartbeatIntervalMS != 0 {
		t.Fatal("prepared file differs from original local Pool contract", err)
	}
	if config.Scopes[0].Commands["owned-check209"].Executable != "never-execute-pool209" || config.Scopes[0].External["owned-provider209"].TokenEnv != "OWNED_REFERENCE209" || config.Scopes[1].Commands == nil || len(config.Scopes[1].Commands) != 0 || strings.Contains(string(saved), "private-value-not-read209") || bytes.Contains(saved, []byte(": null")) {
		t.Fatal("local source references or explicit empty commands changed")
	}
	encoded, _ := json.Marshal(report)
	var safe map[string]any
	_ = json.Unmarshal(encoded, &safe)
	if len(safe) != 6 || report.Protocol != PoolConfigPreparationProtocol || report.PoolID != input.PoolID || report.RunnerID != input.RunnerID || report.ScopeCount != 2 || report.Slots != 2 || report.ConfigurationSHA256 != digest(saved) || saved[len(saved)-1] != '\n' || strings.Contains(string(encoded), directory) || strings.Contains(string(encoded), "OWNED_REFERENCE209") {
		t.Fatal("unsafe or inaccurate preparation receipt")
	}
	reader := &preparationReader191{}
	if _, err = PreparePoolLocalConfig(context.Background(), file, reader); err == nil || reader.reads != 0 {
		t.Fatal("existing output was read or replaced")
	}
	after, _ := os.ReadFile(file)
	original, _ := os.ReadFile(commands)
	if !bytes.Equal(after, saved) || !bytes.Equal(original, commandBytes) {
		t.Fatal("original input or output was modified")
	}
}

func TestPoolConfigPreparationRejectsAmbiguousUnsafeOrUnboundedSources(t *testing.T) {
	input := poolPreparationFixture209(t)
	raw, _ := json.Marshal(input)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	for _, change := range []func(map[string]any){
		func(v map[string]any) { delete(v, "slots") },
		func(v map[string]any) { v["schema_version"] = 1 },
		func(v map[string]any) { v["scopes"] = nil },
		func(v map[string]any) { v["slots"] = 129 },
		func(v map[string]any) { s := v["scopes"].([]any); delete(s[0].(map[string]any), "commands_file") },
		func(v map[string]any) { s := v["scopes"].([]any); s[0].(map[string]any)["workspace"] = "relative" },
		func(v map[string]any) {
			s := v["scopes"].([]any)
			s[0].(map[string]any)["repository"] = input.Scopes[0].Workspace
		},
		func(v map[string]any) { s := v["scopes"].([]any); v["scopes"] = append(s, s[0]) },
	} {
		var candidate map[string]any
		_ = json.Unmarshal(raw, &candidate)
		change(candidate)
		candidateRaw, _ := json.Marshal(candidate)
		if _, err := DecodePoolConfigPreparationInput(candidateRaw); err == nil {
			t.Fatal("invalid private input accepted")
		}
	}
	for _, invalid := range [][]byte{append(append([]byte{}, raw...), []byte(` {}`)...), bytes.Replace(raw, []byte(`"slots":2`), []byte(`"slots":2,"slots":2`), 1), bytes.Replace(raw, []byte(`"pool_id":"owned-pool209"`), []byte(`"pool_id":"\ud800"`), 1), append(bytes.Repeat([]byte(" "), poolPreparationInputLimit), raw...)} {
		if _, err := DecodePoolConfigPreparationInput(invalid); err == nil {
			t.Fatal("ambiguous or oversized JSON accepted")
		}
	}
	directory := filepath.Dir(input.Scopes[0].Workspace)
	for _, destination := range []string{"relative-pool209.json", filepath.Join(directory, "missing", "pool.json")} {
		reader := &preparationReader191{}
		if _, err := PreparePoolLocalConfig(context.Background(), destination, reader); err == nil || reader.reads != 0 {
			t.Fatal("unsafe destination consumed stdin")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := &preparationReader191{}
	if _, err := PreparePoolLocalConfig(ctx, filepath.Join(directory, "cancelled.json"), reader); err == nil || reader.reads != 0 {
		t.Fatal("cancelled preparation consumed stdin")
	}
	assertRejected := func(name string, value PoolConfigPreparationInput) {
		t.Helper()
		candidate, _ := json.Marshal(value)
		file := filepath.Join(directory, name+".json")
		if _, err := PreparePoolLocalConfig(context.Background(), file, bytes.NewReader(candidate)); err == nil {
			t.Fatal("unsafe source accepted", name)
		}
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatal("failed preparation created output", name)
		}
	}
	for i, source := range []string{`null`, `{"bad":{"executable":"x","args":null}}`, `{"bad":{},"bad":{}}`, `{"bad":{"executable":"x","args":[],"unexpected":true}}`} {
		path := filepath.Join(directory, "invalid-source.json")
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		selected := input
		selected.Scopes = append([]PoolConfigPreparationScope{}, input.Scopes...)
		selected.Scopes[0].CommandsFile = path
		assertRejected(string(rune('a'+i)), selected)
	}
	// Same cached source is counted once, but final expansion across scopes is
	// bounded separately. Distinct whitespace-padded sources consume raw budget.
	first, second := filepath.Join(directory, "large-a.json"), filepath.Join(directory, "large-b.json")
	padding := append([]byte(`{}`), bytes.Repeat([]byte(" "), (1<<20)+32)...)
	if err := os.WriteFile(first, padding, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, padding, 0600); err != nil {
		t.Fatal(err)
	}
	selected := input
	selected.Scopes = append([]PoolConfigPreparationScope{}, input.Scopes...)
	selected.Scopes[0].CommandsFile, selected.Scopes[0].ExternalConfigFile = first, second
	assertRejected("source-budget", selected)
	large, _ := json.Marshal(map[string]CommandSpec{"owned-large": {Executable: "never-execute-pool209", Args: []string{strings.Repeat("x", 1<<20)}, ReadOnly: true}})
	if err := os.WriteFile(first, large, 0600); err != nil {
		t.Fatal(err)
	}
	selected.Scopes[0].ExternalConfigFile = ""
	copyScope := selected.Scopes[0]
	copyScope.ProjectID = "other209"
	copyScope.Workspace = filepath.Join(directory, "other-workspace")
	if err := os.Mkdir(copyScope.Workspace, 0700); err != nil {
		t.Fatal(err)
	}
	selected.Scopes = append(selected.Scopes, copyScope)
	assertRejected("expansion-budget", selected)
	sources := poolPreparationSources{files: map[string]poolPreparationFile{}}
	if _, err := sources.read(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if sources.stable(context.Background()) {
		t.Fatal("changed source passed final pin check")
	}
	link := filepath.Join(directory, "commands-link.json")
	if os.Symlink(first, link) == nil {
		selected.Scopes = selected.Scopes[:1]
		selected.Scopes[0].CommandsFile = link
		assertRejected("source-link", selected)
	}
}
