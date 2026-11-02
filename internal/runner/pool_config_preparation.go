package runner

import (
	"bytes"
	"context"
	"encoding/json"
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
	if strictjson.Decode(raw, &value, poolPreparationInputLimit) != nil || !validPoolLocalID(value.PoolID) || !validPoolLocalID(value.RunnerID) || value.Slots < 1 || value.Slots > 128 || len(value.Scopes) < 1 || len(value.Scopes) > 256 {
		return value, poolPreparationInvalid()
	}
	seen := map[string]bool{}
	for _, scope := range value.Scopes {
		key := scope.TenantID + "/" + scope.ProjectID
		if !validPoolLocalID(scope.TenantID) || !validPoolLocalID(scope.ProjectID) || seen[key] || !poolPreparationPath(scope.Workspace) || (scope.Repository == "") != (scope.BaseCommit == "") || scope.BaseCommit != "" && !commitID.MatchString(scope.BaseCommit) {
			return value, poolPreparationInvalid()
		}
		seen[key] = true
		for _, path := range []string{scope.CommandsFile, scope.Repository, scope.Worktrees, scope.ModelDeploymentsFile, scope.ExternalConfigFile} {
			if path != "" && !poolPreparationPath(path) {
				return value, poolPreparationInvalid()
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
func PreparePoolLocalConfig(ctx context.Context, destination string, input io.Reader) (PoolConfigPreparation, error) {
	var report PoolConfigPreparation
	if !poolPreparationPath(destination) {
		return report, poolPreparationInvalid()
	}
	file, parent, err := openToolCommandDestination(destination)
	if err != nil {
		return report, poolPreparationInvalid()
	}
	defer parent.Close()
	_, outputParents, err := cursorPackageRoot(filepath.Dir(file))
	if err != nil {
		return report, poolPreparationInvalid()
	}
	anchored, err := parent.Stat(".")
	if err != nil || !os.SameFile(anchored, outputParents[0].info) || ctx.Err() != nil || input == nil {
		return report, poolPreparationInvalid()
	}
	raw, err := io.ReadAll(io.LimitReader(input, poolPreparationInputLimit+1))
	if err != nil {
		return report, poolPreparationInvalid()
	}
	selected, err := DecodePoolConfigPreparationInput(raw)
	if err != nil {
		return report, err
	}
	sources := poolPreparationSources{files: map[string]poolPreparationFile{}}
	config := PoolLocalConfig{SchemaVersion: 1, PoolID: selected.PoolID, RunnerID: selected.RunnerID, Slots: selected.Slots, Scopes: []PoolLocalScope{}}
	directories := [][]cursorPackageNode{outputParents}
	workspaces := []os.FileInfo{}
	encodedScopeBytes := 0
	for _, scope := range selected.Scopes {
		if ctx.Err() != nil {
			return report, poolPreparationInvalid()
		}
		local := PoolLocalScope{TenantID: scope.TenantID, ProjectID: scope.ProjectID, BaseCommit: scope.BaseCommit, Commands: map[string]CommandSpec{}}
		for _, item := range []struct {
			source    string
			target    *string
			workspace bool
		}{{scope.Workspace, &local.Workspace, true}, {scope.Repository, &local.Repository, false}, {scope.Worktrees, &local.Worktrees, false}} {
			if item.source == "" {
				continue
			}
			path, nodes, e := cursorPackageRoot(item.source)
			if e != nil {
				return report, poolPreparationInvalid()
			}
			*item.target = path
			directories = append(directories, nodes)
			if item.workspace {
				for _, other := range workspaces {
					if os.SameFile(other, nodes[0].info) {
						return report, poolPreparationInvalid()
					}
				}
				workspaces = append(workspaces, nodes[0].info)
			}
		}
		if scope.CommandsFile != "" {
			body, e := sources.read(ctx, scope.CommandsFile)
			if e != nil || decodePoolPreparationSource(body, &local.Commands) != nil {
				return report, poolPreparationInvalid()
			}
			local.Commands, e = DecodeCommands(body)
			if e != nil {
				return report, poolPreparationInvalid()
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
			if e != nil || decodePoolPreparationSource(body, &local.ModelDeployments) != nil {
				return report, poolPreparationInvalid()
			}
			local.ModelDeployments, e = DecodeModelDeployments(body)
			if e != nil {
				return report, poolPreparationInvalid()
			}
		}
		if scope.ExternalConfigFile != "" {
			body, e := sources.read(ctx, scope.ExternalConfigFile)
			if e != nil || decodePoolPreparationSource(body, &local.External) != nil {
				return report, poolPreparationInvalid()
			}
		}
		// Bound repeated source expansion before retaining another decoded scope.
		// One small allowlist reused by 256 projects must not allocate an
		// unbounded final JSON before the output-size guard gets to reject it.
		scopeBytes, e := json.MarshalIndent(local, "    ", "  ")
		if e != nil || len(scopeBytes) > poolPreparationFileLimit-encodedScopeBytes {
			return report, poolPreparationInvalid()
		}
		encodedScopeBytes += len(scopeBytes)
		config.Scopes = append(config.Scopes, local)
	}
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil || len(encoded)+1 > poolPreparationFileLimit {
		return report, poolPreparationInvalid()
	}
	encoded = append(encoded, '\n')
	if _, err = DecodePoolConfig(encoded); err != nil {
		return report, poolPreparationInvalid()
	}
	var final PoolLocalConfig
	if decodePoolPreparationSource(encoded, &final) != nil {
		return report, poolPreparationInvalid()
	}
	if !sources.stable(ctx) {
		return report, poolPreparationInvalid()
	}
	for _, nodes := range directories {
		if !cursorPackageParentsStable(nodes) {
			return report, poolPreparationInvalid()
		}
	}
	if ctx.Err() != nil {
		return report, poolPreparationInvalid()
	}
	output, err := parent.OpenFile(filepath.Base(file), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return report, poolPreparationInvalid()
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
		return report, poolPreparationInvalid()
	}
	complete = true
	return PoolConfigPreparation{Protocol: PoolConfigPreparationProtocol, PoolID: config.PoolID, RunnerID: config.RunnerID, ScopeCount: len(config.Scopes), Slots: config.Slots, ConfigurationSHA256: digest(encoded)}, nil
}
