package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const ToolPreparationProtocol = "suma-runner-tools-preparation-v1"

type UnpreparedTool struct {
	AdapterID  string `json:"adapter_id"`
	ReasonCode string `json:"reason_code"`
}

// ToolPreparation contains configuration facts only. Preparing a command does
// not attest a fresh inventory, model login, dispatch permission or task result.
type ToolPreparation struct {
	Protocol     string           `json:"protocol"`
	Prepared     bool             `json:"prepared"`
	File         string           `json:"file"`
	CommandCount int              `json:"command_count"`
	Remaining    []UnpreparedTool `json:"remaining"`
}

// PrepareToolCommands uses the existing fixed local discovery policies. It
// neither reads Runner credentials nor opens a journal or control connection.
func PrepareToolCommands(ctx context.Context, path, adapterID string) (ToolPreparation, error) {
	return prepareToolCommands(ctx, path, adapterID, DiscoverInstalledTools)
}

func prepareToolCommands(ctx context.Context, path, adapterID string, discover func(context.Context, string) (InstalledToolDiscovery, error)) (ToolPreparation, error) {
	result := ToolPreparation{Protocol: ToolPreparationProtocol, Remaining: []UnpreparedTool{}}
	file, parent, err := openToolCommandDestination(path)
	result.File = file
	if err != nil {
		return result, err
	}
	defer parent.Close()
	report, err := discover(ctx, adapterID)
	if err != nil {
		return result, err
	}
	if report.Protocol != InstalledToolDiscoveryProtocol || len(report.Items) == 0 || len(report.Items) > len(localToolDirectory()) {
		return result, fmt.Errorf("local tool discovery did not return bounded current facts")
	}
	commands := map[string]CommandSpec{}
	seen := map[string]bool{}
	for _, item := range report.Items {
		if seen[item.AdapterID] || !localToolSafeText(item.AdapterID, 100) || !localToolSafeText(item.ReasonCode, 100) {
			return result, fmt.Errorf("local tool discovery returned invalid identities")
		}
		seen[item.AdapterID] = true
		if item.Installed && item.LaunchSupported && item.Command != nil {
			commands["local-"+item.AdapterID] = *item.Command
		} else {
			result.Remaining = append(result.Remaining, UnpreparedTool{AdapterID: item.AdapterID, ReasonCode: item.ReasonCode})
		}
	}
	if len(commands) == 0 {
		return result, nil
	}
	if err = writePreparedToolCommands(ctx, parent, filepath.Base(file), commands); err != nil {
		return result, err
	}
	result.Prepared, result.CommandCount = true, len(commands)
	return result, nil
}

// Both preparation modes reject an existing or linked destination before
// consuming private input, inspecting a package, or discovering any tool.
func openToolCommandDestination(path string) (string, *os.Root, error) {
	if !localToolSafeText(path, 4096) {
		return "", nil, fmt.Errorf("command destination must be a bounded local path")
	}
	// Reject aliases before Abs/Clean or any filesystem/stdin access. In
	// particular, Windows trailing dots/spaces and ADS cannot name a new file.
	for _, part := range strings.FieldsFunc(strings.TrimPrefix(path, filepath.VolumeName(path)), func(r rune) bool { return r == '/' || runtime.GOOS == "windows" && r == '\\' }) {
		if part == ".." || runtime.GOOS == "windows" && part != "." && (strings.TrimRight(part, ". ") != part || strings.ContainsRune(part, ':')) {
			return "", nil, fmt.Errorf("command destination has an unsupported alias")
		}
	}
	file, err := filepath.Abs(path)
	if err != nil {
		return "", nil, fmt.Errorf("command destination is unavailable")
	}
	if runtime.GOOS == "windows" && (!cursorDiscoveryWindowsDrive(file) || cursorDiscoveryWindowsNamespace(file)) {
		return file, nil, fmt.Errorf("command destination must be local")
	}
	// Never create parents or follow an existing destination or directory link.
	// OpenRoot below anchors creation in the already checked parent directory.
	if _, err = os.Lstat(file); !os.IsNotExist(err) {
		return file, nil, fmt.Errorf("command destination must be a new file")
	}
	for directory := filepath.Dir(file); ; directory = filepath.Dir(directory) {
		info, statErr := os.Lstat(directory)
		if statErr != nil || !info.IsDir() || !cursorPackageOrdinary(info) {
			return file, nil, fmt.Errorf("command destination needs existing directories without links")
		}
		if filepath.Dir(directory) == directory {
			break
		}
	}
	parent, err := os.OpenRoot(filepath.Dir(file))
	if err != nil {
		return file, nil, fmt.Errorf("command destination directory is unavailable")
	}
	return file, parent, nil
}

func writePreparedToolCommands(ctx context.Context, parent *os.Root, leaf string, commands map[string]CommandSpec) error {
	if len(commands) == 0 {
		return fmt.Errorf("tool configuration must not be empty")
	}
	raw, err := json.MarshalIndent(commands, "", "  ")
	if err != nil {
		return fmt.Errorf("local tool configuration could not be encoded")
	}
	if _, err = DecodeCommands(raw); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	output, err := parent.OpenFile(leaf, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("command destination must be a new writable file")
	}
	created := false
	defer func() {
		_ = output.Close()
		if !created {
			_ = parent.Remove(leaf)
		}
	}()
	if _, err = output.Write(append(raw, '\n')); err == nil {
		err = output.Sync()
	}
	if closeErr := output.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("local tool configuration could not be persisted")
	}
	created = true
	return nil
}
