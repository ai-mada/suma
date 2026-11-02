package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ai-mada/suma/internal/strictjson"
)

const PoolConfigPreparationProtocol = "suma-pool-config-preparation-v1"
const poolPreparationInputLimit = 64 << 10
const poolPreparationFileLimit = 2 << 20

// Every private scope key is explicit. Empty references mean no corresponding
// local configuration, never an environment lookup or automatic discovery.
type PoolConfigPreparationScope struct {
	TenantID             string `json:"tenant_id"`
	ProjectID            string `json:"project_id"`
	Workspace            string `json:"workspace"`
	CommandsFile         string `json:"commands_file"`
	Repository           string `json:"repository"`
	BaseCommit           string `json:"base_commit"`
	Worktrees            string `json:"worktrees"`
	ModelDeploymentsFile string `json:"model_deployments_file"`
	ExternalConfigFile   string `json:"external_config_file"`
}
type PoolConfigPreparationInput struct {
	PoolID   string                       `json:"pool_id"`
	RunnerID string                       `json:"runner_id"`
	Slots    int                          `json:"slots"`
	Scopes   []PoolConfigPreparationScope `json:"scopes"`
}

// This safe receipt deliberately contains no path, source content or provider
// credentials. Its digest identifies exactly the bytes persisted, including LF.
type PoolConfigPreparation struct {
	Protocol            string `json:"protocol"`
	PoolID              string `json:"pool_id"`
	RunnerID            string `json:"runner_id"`
	ScopeCount          int    `json:"scope_count"`
	Slots               int    `json:"slots"`
	ConfigurationSHA256 string `json:"configuration_sha256"`
}

func poolPreparationInvalid() error {
	return fmt.Errorf("local Pool configuration preparation failed; preserve original files")
}
func poolPreparationPath(path string) bool {
	if !desktopManagedPath(path) || strings.TrimSpace(path) != path {
		return false
	}
	if runtime.GOOS == "windows" && (!cursorDiscoveryWindowsDrive(path) || cursorDiscoveryWindowsNamespace(path)) {
		return false
	}
	for _, part := range strings.FieldsFunc(strings.TrimPrefix(path, filepath.VolumeName(path)), func(r rune) bool { return r == '/' || runtime.GOOS == "windows" && r == '\\' }) {
		if part == ".." || runtime.GOOS == "windows" && part != "." && (strings.TrimRight(part, ". ") != part || strings.ContainsRune(part, ':')) {
			return false
		}
	}
	return true
}
func DecodePoolConfigPreparationInput(raw []byte) (PoolConfigPreparationInput, error) {
	var value PoolConfigPreparationInput
	invalid := func() error {
		failure := poolPreparationFailure("input_invalid", 0, "input")
		failure.InputSHA256 = poolPreparationInputDigest(raw)
		return failure
	}
	if strictjson.Decode(raw, &value, poolPreparationInputLimit) != nil || !validPoolLocalID(value.PoolID) || !validPoolLocalID(value.RunnerID) || value.Slots < 1 || value.Slots > 128 || len(value.Scopes) < 1 || len(value.Scopes) > 256 {
		return value, invalid()
	}
	seen := map[string]bool{}
	for _, scope := range value.Scopes {
		key := scope.TenantID + "/" + scope.ProjectID
		if !validPoolLocalID(scope.TenantID) || !validPoolLocalID(scope.ProjectID) || seen[key] || !poolPreparationPath(scope.Workspace) || (scope.Repository == "") != (scope.BaseCommit == "") || scope.BaseCommit != "" && !commitID.MatchString(scope.BaseCommit) {
			return value, invalid()
		}
		seen[key] = true
		for _, path := range []string{scope.CommandsFile, scope.Repository, scope.Worktrees, scope.ModelDeploymentsFile, scope.ExternalConfigFile} {
			if path != "" && !poolPreparationPath(path) {
				return value, invalid()
			}
		}
	}
	return value, nil
}

// Keep existing domain decoders authoritative while rejecting null values and
// ambiguous raw JSON before ordinary decoding can erase that evidence.
func decodePoolPreparationSource(raw []byte, out any) error {
	var value any
	if strictjson.Decode(raw, &value, poolPreparationFileLimit) != nil {
		return poolPreparationInvalid()
	}
	var nonnull func(any) bool
	nonnull = func(v any) bool {
		switch row := v.(type) {
		case nil:
			return false
		case map[string]any:
			for _, item := range row {
				if !nonnull(item) {
					return false
				}
			}
		case []any:
			for _, item := range row {
				if !nonnull(item) {
					return false
				}
			}
		}
		return true
	}
	if !nonnull(value) {
		return poolPreparationInvalid()
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return poolPreparationInvalid()
	}
	return nil
}

type poolPreparationFile struct {
	path    string
	raw     []byte
	info    os.FileInfo
	parents []cursorPackageNode
}
type poolPreparationSources struct {
	files map[string]poolPreparationFile
	total int
}

func inspectPoolPreparationFile(path string, remaining int) (poolPreparationFile, error) {
	var result poolPreparationFile
	if !poolPreparationPath(path) || remaining < 0 {
		return result, poolPreparationInvalid()
	}
	path = filepath.Clean(path)
	_, parents, err := cursorPackageRoot(filepath.Dir(path))
	if err != nil {
		return result, poolPreparationInvalid()
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || !cursorPackageOrdinary(before) || before.Size() > int64(remaining) || before.Size() > poolPreparationFileLimit {
		return result, poolPreparationInvalid()
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return result, poolPreparationInvalid()
	}
	defer root.Close()
	parent, err := root.Stat(".")
	if err != nil || !os.SameFile(parent, parents[0].info) {
		return result, poolPreparationInvalid()
	}
	f, err := root.Open(filepath.Base(path))
	if err != nil {
		return result, poolPreparationInvalid()
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(opened, before) {
		return result, poolPreparationInvalid()
	}
	raw, err := io.ReadAll(io.LimitReader(f, int64(remaining)+1))
	if err != nil || len(raw) > remaining || len(raw) > poolPreparationFileLimit {
		return result, poolPreparationInvalid()
	}
	after, err := os.Lstat(path)
	if err != nil || !cursorPackageOrdinary(after) || !os.SameFile(opened, after) || after.Size() != int64(len(raw)) || !opened.ModTime().Equal(after.ModTime()) || !cursorPackageParentsStable(parents) {
		return result, poolPreparationInvalid()
	}
	return poolPreparationFile{path: path, raw: raw, info: after, parents: parents}, nil
}
func (s *poolPreparationSources) read(ctx context.Context, path string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, poolPreparationInvalid()
	}
	key := filepath.Clean(path)
	if saved, ok := s.files[key]; ok {
		return saved.raw, nil
	}
	value, err := inspectPoolPreparationFile(path, poolPreparationFileLimit-s.total)
	if err != nil {
		return nil, err
	}
	s.total += len(value.raw)
	s.files[key] = value
	return value.raw, nil
}
func (s *poolPreparationSources) stable(ctx context.Context) bool {
	for _, value := range s.files {
		if ctx.Err() != nil || !cursorPackageParentsStable(value.parents) {
			return false
		}
		current, err := inspectPoolPreparationFile(value.path, poolPreparationFileLimit)
		if err != nil || !os.SameFile(value.info, current.info) || !bytes.Equal(value.raw, current.raw) {
			return false
		}
	}
	return true
}

// PreparePoolLocalConfig only composes existing local allowlists. It performs
// no Git, executable, environment, keyring, HTTP or authorization operation.
func PreparePoolLocalConfig(ctx context.Context, destination string, input io.Reader) (report PoolConfigPreparation, resultErr error) {
	inputSHA := ""
	defer func() {
		if resultErr == nil {
			return
		}
		failure := poolPreparationFailure("configuration_invalid", 0, "input")
		var classified *PoolConfigPreparationError
		if errors.As(resultErr, &classified) {
			copy := *classified
			failure = &copy
		}
		failure.InputSHA256 = inputSHA
		failure.OutputSHA256 = digest([]byte(destination))
		resultErr = failure
	}()
	if !poolPreparationPath(destination) {
		return report, poolPreparationFailure("output_invalid", 0, "output")
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		if err == nil {
			return report, poolPreparationFailure("output_exists", 0, "output")
		}
		return report, poolPreparationFailure("output_unavailable", 0, "output")
	}
	file, parent, err := openToolCommandDestination(destination)
	if err != nil {
		// Recheck only to distinguish an output introduced during the original
		// protected destination guard. Never retry creation or overwrite it.
		if _, existErr := os.Lstat(destination); existErr == nil {
			return report, poolPreparationFailure("output_exists", 0, "output")
		}
		return report, poolPreparationFailure("output_unavailable", 0, "output")
	}
	defer parent.Close()
	_, outputParents, err := cursorPackageRoot(filepath.Dir(file))
	if err != nil {
		return report, poolPreparationFailure("output_unavailable", 0, "output")
	}
	anchored, err := parent.Stat(".")
	if err != nil || !os.SameFile(anchored, outputParents[0].info) {
		return report, poolPreparationFailure("output_unavailable", 0, "output")
	}
	if ctx.Err() != nil {
		return report, poolPreparationFailure("cancelled", 0, "input")
	}
	if input == nil {
		return report, poolPreparationFailure("input_invalid", 0, "input")
	}
	raw, err := io.ReadAll(io.LimitReader(input, poolPreparationInputLimit+1))
	if err != nil {
		if ctx.Err() != nil {
			return report, poolPreparationFailure("cancelled", 0, "input")
		}
		return report, poolPreparationFailure("input_invalid", 0, "input")
	}
	inputSHA = poolPreparationInputDigest(raw)
	selected, err := DecodePoolConfigPreparationInput(raw)
	if err != nil {
		return report, err
	}
	sources := poolPreparationSources{files: map[string]poolPreparationFile{}}
	config := PoolLocalConfig{SchemaVersion: 1, PoolID: selected.PoolID, RunnerID: selected.RunnerID, Slots: selected.Slots, Scopes: []PoolLocalScope{}}
	directories := [][]cursorPackageNode{outputParents}
	workspaces := []os.FileInfo{}
	encodedScopeBytes := 0
	for index, scope := range selected.Scopes {
		row := index + 1
		if ctx.Err() != nil {
			return report, poolPreparationFailure("cancelled", 0, "input")
		}
		local := PoolLocalScope{TenantID: scope.TenantID, ProjectID: scope.ProjectID, BaseCommit: scope.BaseCommit, Commands: map[string]CommandSpec{}}
		for _, item := range []struct {
			source    string
			field     string
			target    *string
			workspace bool
		}{{scope.Workspace, "workspace", &local.Workspace, true}, {scope.Repository, "repository", &local.Repository, false}, {scope.Worktrees, "worktrees", &local.Worktrees, false}} {
			if item.source == "" {
				continue
			}
			path, nodes, e := cursorPackageRoot(item.source)
			if e != nil {
				return report, poolPreparationFailure("directory_unavailable", row, item.field)
			}
			*item.target = path
			directories = append(directories, nodes)
			if item.workspace {
				for _, other := range workspaces {
					if os.SameFile(other, nodes[0].info) {
						return report, poolPreparationFailure("workspace_conflict", row, "workspace")
					}
				}
				workspaces = append(workspaces, nodes[0].info)
			}
		}
		if scope.CommandsFile != "" {
			body, e := sources.read(ctx, scope.CommandsFile)
			if e != nil {
				if ctx.Err() != nil {
					return report, poolPreparationFailure("cancelled", 0, "input")
				}
				return report, poolPreparationFailure("source_unavailable", row, "commands_file")
			}
			if decodePoolPreparationSource(body, &local.Commands) != nil {
				return report, poolPreparationFailure("source_invalid", row, "commands_file")
			}
			local.Commands, e = DecodeCommands(body)
			if e != nil {
				return report, poolPreparationFailure("source_invalid", row, "commands_file")
			}
			for id, spec := range local.Commands {
				if spec.Args == nil {
					spec.Args = []string{}
				}
				if spec.NativeWorkspace != nil && spec.NativeWorkspace.Checks == nil {
					spec.NativeWorkspace.Checks = []string{}
				}
				local.Commands[id] = spec
			}
		}
		if scope.ModelDeploymentsFile != "" {
			body, e := sources.read(ctx, scope.ModelDeploymentsFile)
			if e != nil {
				if ctx.Err() != nil {
					return report, poolPreparationFailure("cancelled", 0, "input")
				}
				return report, poolPreparationFailure("source_unavailable", row, "model_deployments_file")
			}
			if decodePoolPreparationSource(body, &local.ModelDeployments) != nil {
				return report, poolPreparationFailure("source_invalid", row, "model_deployments_file")
			}
			local.ModelDeployments, e = DecodeModelDeployments(body)
			if e != nil {
				return report, poolPreparationFailure("source_invalid", row, "model_deployments_file")
			}
		}
		if scope.ExternalConfigFile != "" {
			body, e := sources.read(ctx, scope.ExternalConfigFile)
			if e != nil {
				if ctx.Err() != nil {
					return report, poolPreparationFailure("cancelled", 0, "input")
				}
				return report, poolPreparationFailure("source_unavailable", row, "external_config_file")
			}
			if decodePoolPreparationSource(body, &local.External) != nil {
				return report, poolPreparationFailure("source_invalid", row, "external_config_file")
			}
		}
		// Bound repeated source expansion before retaining another decoded scope.
		// One small allowlist reused by 256 projects must not allocate an
		// unbounded final JSON before the output-size guard gets to reject it.
		scopeBytes, e := json.MarshalIndent(local, "    ", "  ")
		if e != nil || len(scopeBytes) > poolPreparationFileLimit-encodedScopeBytes {
			return report, poolPreparationFailure("configuration_invalid", 0, "input")
		}
		encodedScopeBytes += len(scopeBytes)
		config.Scopes = append(config.Scopes, local)
	}
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil || len(encoded)+1 > poolPreparationFileLimit {
		return report, poolPreparationFailure("configuration_invalid", 0, "input")
	}
	encoded = append(encoded, '\n')
	if _, err = DecodePoolConfig(encoded); err != nil {
		return report, poolPreparationFailure("configuration_invalid", 0, "input")
	}
	var final PoolLocalConfig
	if decodePoolPreparationSource(encoded, &final) != nil {
		return report, poolPreparationFailure("configuration_invalid", 0, "input")
	}
	if !sources.stable(ctx) {
		if ctx.Err() != nil {
			return report, poolPreparationFailure("cancelled", 0, "input")
		}
		return report, poolPreparationFailure("inputs_changed", 0, "input")
	}
	for _, nodes := range directories {
		if !cursorPackageParentsStable(nodes) {
			return report, poolPreparationFailure("inputs_changed", 0, "input")
		}
	}
	if ctx.Err() != nil {
		return report, poolPreparationFailure("cancelled", 0, "input")
	}
	output, err := parent.OpenFile(filepath.Base(file), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if os.IsExist(err) {
			return report, poolPreparationFailure("output_exists", 0, "output")
		}
		return report, poolPreparationFailure("output_unavailable", 0, "output")
	}
	complete := false
	defer func() {
		_ = output.Close()
		if !complete {
			_ = parent.Remove(filepath.Base(file))
		}
	}()
	count, err := output.Write(encoded)
	if err == nil && count != len(encoded) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = output.Sync()
	}
	if closeErr := output.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return report, poolPreparationFailure("output_unconfirmed", 0, "output")
	}
	complete = true
	return PoolConfigPreparation{Protocol: PoolConfigPreparationProtocol, PoolID: config.PoolID, RunnerID: config.RunnerID, ScopeCount: len(config.Scopes), Slots: config.Slots, ConfigurationSHA256: digest(encoded)}, nil
}
