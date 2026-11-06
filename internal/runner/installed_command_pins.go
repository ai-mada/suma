package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type installedCommandPinContext struct{}

func wantsInstalledCommandPins(ctx context.Context) bool {
	value, _ := ctx.Value(installedCommandPinContext{}).(bool)
	return value
}

// These are private source pins, not executable flags or provider credentials.
// Native commands use only HostSHA256. Node commands additionally pin the
// original resolver's entire module root, including nested/hoisted siblings.
type InstalledCommandPins struct {
	AdapterID    string `json:"adapter_id"`
	HostSHA256   string `json:"host_sha256"`
	ResolverRoot string `json:"resolver_root,omitempty" wire:"nonnull"`
	TreeSHA256   string `json:"tree_sha256,omitempty" wire:"nonnull"`
}

func genericInstalledAdapter(id string) bool {
	switch id {
	case "codex-jsonl", "codebuddy-cli", "dsh-multica", "claude-code-cli":
		return true
	}
	return false
}
func installedDirectoryEntry(id string) (localToolDirectoryEntry, bool) {
	for _, e := range localToolDirectory() {
		if e.id == id && genericInstalledAdapter(id) {
			return e, true
		}
	}
	return localToolDirectoryEntry{}, false
}
func validateInstalledCommandSpec(spec CommandSpec) error {
	pins := spec.InstalledCommand
	if pins == nil {
		return nil
	}
	if !evidenceHash.MatchString(pins.HostSHA256) || !desktopManagedPath(spec.Executable) {
		return fmt.Errorf("invalid installed command source pins")
	}
	if pins.AdapterID == managedCodexAdapterID {
		if pins.ResolverRoot != "" || pins.TreeSHA256 != "" || !fixedManagedCodexCommand(spec) {
			return fmt.Errorf("invalid installed Codex app-server source pins")
		}
		return nil
	}
	entry, ok := installedDirectoryEntry(pins.AdapterID)
	if !ok {
		return fmt.Errorf("invalid installed command adapter")
	}
	expected := normalizedCommand(entry.command)
	expected.Executable = spec.Executable
	expected.InstalledCommand = pins
	js, err := teamCLIEntry(spec)
	if err != nil {
		return err
	}
	if js != "" {
		if pins.ResolverRoot == "" || !desktopManagedPath(pins.ResolverRoot) || filepath.Base(pins.ResolverRoot) != "node_modules" || !evidenceHash.MatchString(pins.TreeSHA256) || !within(pins.ResolverRoot, js) {
			return fmt.Errorf("installed JavaScript command requires its original complete module root")
		}
		manifest, err := localToolPackageManifest(pins.ResolverRoot, entry.packageName)
		if err != nil {
			return fmt.Errorf("installed package metadata changed")
		}
		bin := ""
		switch value := manifest["bin"].(type) {
		case string:
			bin = value
		case map[string]any:
			bin, _ = value[entry.binary].(string)
		}
		original, err := localToolPackageFile(filepath.Join(pins.ResolverRoot, filepath.FromSlash(entry.packageName)), bin, 16<<20)
		if err != nil || !sameACPPath(original, js) {
			return fmt.Errorf("installed command no longer names its original package entry")
		}
		expected.Args = append([]string{js}, expected.Args...)
	} else if pins.ResolverRoot != "" || pins.TreeSHA256 != "" {
		return fmt.Errorf("native installed command cannot claim a Node package tree")
	}
	if pins.AdapterID == "claude-code-cli" {
		if len(spec.Args) < 1 {
			return fmt.Errorf("installed Claude command requires an explicit model")
		}
		expected.Args = claudeArguments(spec.Args[len(spec.Args)-1])
	}
	a, _ := json.Marshal(normalizedCommand(spec))
	b, _ := json.Marshal(expected)
	if string(a) != string(b) {
		return fmt.Errorf("installed command differs from its fixed scanned protocol")
	}
	return nil
}

// Node's absolute entry still searches ancestor node_modules directories.
// Reject an existing resolver outside the captured tree; recheck absences at
// every digest, so a later sibling/external install cannot silently take over.
// Workspace, account and user plugin contents are not advertised as this tree.
func installedNodeResolutionClosed(spec CommandSpec, entry, root string) bool {
	candidates := map[string]bool{}
	for dir := filepath.Dir(entry); ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "node_modules")
		if !within(root, candidate) && !sameACPPath(root, candidate) {
			candidates[candidate] = true
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	for _, name := range []string{"HOME", "USERPROFILE"} {
		if home := os.Getenv(name); filepath.IsAbs(home) {
			candidates[filepath.Join(home, ".node_modules")] = true
			candidates[filepath.Join(home, ".node_libraries")] = true
		}
	}
	prefix := filepath.Dir(spec.Executable)
	candidates[filepath.Join(prefix, "lib", "node")] = true
	candidates[filepath.Join(filepath.Dir(prefix), "lib", "node")] = true
	for candidate := range candidates {
		if within(root, candidate) || sameACPPath(root, candidate) {
			continue
		}
		if _, err := os.Lstat(candidate); !os.IsNotExist(err) {
			return false
		}
	}
	return true
}
func installedTreeSHA(ctx context.Context, root string) (string, error) {
	_, files, err := inspectPinnedInstallationTree(ctx, root, nil)
	if err != nil {
		return "", err
	}
	raw, _ := json.Marshal(struct {
		Protocol string              `json:"protocol"`
		Files    []cursorPackageFile `json:"files"`
	}{"suma-installed-loadable-tree-v1", files})
	return digest(raw), nil
}
func pinResolvedInstalledCommand(ctx context.Context, id string, spec CommandSpec, resolverRoot string) (CommandSpec, error) {
	if !genericInstalledAdapter(id) {
		return CommandSpec{}, fmt.Errorf("unsupported installed command factory")
	}
	exe, sha, err := qoderNativeTarget(ctx, spec.Executable)
	if err != nil {
		return CommandSpec{}, err
	}
	spec = normalizedCommand(spec)
	spec.Executable = exe
	pins := &InstalledCommandPins{AdapterID: id, HostSHA256: sha}
	entry, err := teamCLIEntry(spec)
	if err != nil {
		return CommandSpec{}, err
	}
	if entry != "" {
		if resolverRoot == "" || !installedNodeResolutionClosed(spec, entry, resolverRoot) {
			return CommandSpec{}, fmt.Errorf("installed package has an external resolver; use the original manual guide")
		}
		pins.ResolverRoot, _, err = cursorPackageRoot(resolverRoot)
		if err != nil {
			return CommandSpec{}, err
		}
		pins.TreeSHA256, err = installedTreeSHA(ctx, pins.ResolverRoot)
		if err != nil {
			return CommandSpec{}, err
		}
	}
	spec.InstalledCommand = pins
	if err = validateInstalledCommandSpec(spec); err != nil {
		return CommandSpec{}, err
	}
	return spec, validateInstalledCommandPins(ctx, spec)
}
func validateInstalledCommandPins(ctx context.Context, spec CommandSpec) error {
	if err := validateInstalledCommandSpec(spec); err != nil {
		return err
	}
	p := spec.InstalledCommand
	if p == nil {
		return nil
	}
	if !sameACPProbeInstallation(ctx, spec.Executable, p.HostSHA256) {
		return fmt.Errorf("installed native host bytes changed")
	}
	if p.ResolverRoot != "" {
		entry, err := teamCLIEntry(spec)
		if err != nil || entry == "" || !installedNodeResolutionClosed(spec, entry, p.ResolverRoot) {
			return fmt.Errorf("installed module resolver changed")
		}
		sha, err := installedTreeSHA(ctx, p.ResolverRoot)
		if err != nil || sha != p.TreeSHA256 {
			return fmt.Errorf("installed complete dependency tree changed")
		}
	}
	return ctx.Err()
}

// The original rooted Cursor traversal is shared by installation closures.
// Callers choose the fixed install root, never a workspace or browser path.
func inspectPinnedInstallationTree(ctx context.Context, root string, validate func(string, []cursorPackageFile) error) (string, []cursorPackageFile, error) {
	root, parents, err := cursorPackageRoot(root)
	if err != nil {
		return "", nil, err
	}
	owned, err := os.OpenRoot(root)
	if err != nil {
		return "", nil, fmt.Errorf("Cursor package directory is unavailable")
	}
	defer owned.Close()
	anchored, err := owned.Stat(".")
	if err != nil || !os.SameFile(parents[0].info, anchored) || !cursorPackageParentsStable(parents) {
		return "", nil, fmt.Errorf("Cursor package directory changed while opening")
	}
	files := []cursorPackageFile{}
	nodes := []cursorPackageNode{}
	directories := 0
	var total int64
	var scan func(string) error
	scan = func(relative string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		before, err := owned.Lstat(relative)
		if err != nil || !cursorPackageOrdinary(before) || !before.IsDir() {
			return fmt.Errorf("Cursor package contains a linked or unavailable directory")
		}
		directories++
		if directories > cursorPackageMaxFiles {
			return fmt.Errorf("Cursor package contains too many directories")
		}
		directory, err := owned.Open(relative)
		if err != nil {
			return fmt.Errorf("Cursor package directory could not be read")
		}
		opened, statErr := directory.Stat()
		if statErr != nil || !cursorPackageOrdinary(opened) || !opened.IsDir() || !os.SameFile(before, opened) {
			_ = directory.Close()
			return fmt.Errorf("Cursor package directory changed while reading")
		}
		children, readErr := directory.ReadDir(cursorPackageMaxFiles + 1)
		if readErr == io.EOF {
			readErr = nil
		}
		closeErr := directory.Close()
		if readErr != nil || closeErr != nil {
			return fmt.Errorf("Cursor package directory could not be enumerated")
		}
		if len(children) > cursorPackageMaxFiles {
			return fmt.Errorf("installation directory exceeds its entry limit")
		}
		nodes = append(nodes, cursorPackageNode{relative, before})
		for _, child := range children {
			if err := ctx.Err(); err != nil {
				return err
			}
			name := filepath.Join(relative, child.Name())
			info, err := owned.Lstat(name)
			if err != nil || !cursorPackageOrdinary(info) {
				return fmt.Errorf("Cursor package contains a linked or special node")
			}
			if info.IsDir() {
				if err = scan(name); err != nil {
					return err
				}
				continue
			}
			if len(files) >= cursorPackageMaxFiles || info.Size() < 0 || info.Size() > cursorPackageMaxBytes-total {
				return fmt.Errorf("Cursor package exceeds its file or byte limit")
			}
			total += info.Size()
			file, err := owned.Open(name)
			if err != nil {
				return fmt.Errorf("Cursor package file could not be read")
			}
			opened, statErr := file.Stat()
			if statErr != nil || !cursorPackageOrdinary(opened) || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
				_ = file.Close()
				return fmt.Errorf("Cursor package file changed while opening")
			}
			hash := sha256.New()
			count, readErr := io.Copy(hash, io.LimitReader(file, info.Size()+1))
			after, statErr := file.Stat()
			closeErr := file.Close()
			current, currentErr := owned.Lstat(name)
			if readErr != nil || statErr != nil || closeErr != nil || currentErr != nil || count != info.Size() || !cursorPackageOrdinary(current) || !os.SameFile(info, after) || !os.SameFile(info, current) || after.Size() != info.Size() || current.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) || !current.ModTime().Equal(info.ModTime()) {
				return fmt.Errorf("Cursor package file changed while hashing")
			}
			files = append(files, cursorPackageFile{filepath.ToSlash(name), count, hex.EncodeToString(hash.Sum(nil))})
			nodes = append(nodes, cursorPackageNode{name, info})
		}
		return nil
	}
	if err = scan("."); err != nil {
		return "", nil, err
	}
	if validate != nil {
		if err = validate(root, files); err != nil {
			return "", nil, err
		}
	}
	if !cursorPackageParentsStable(parents) {
		return "", nil, fmt.Errorf("Cursor package parent identity changed")
	}
	for _, previous := range nodes {
		current, err := owned.Lstat(previous.path)
		if err != nil || !cursorPackageOrdinary(current) || !os.SameFile(previous.info, current) || current.Size() != previous.info.Size() || !current.ModTime().Equal(previous.info.ModTime()) {
			return "", nil, fmt.Errorf("Cursor package tree changed during inspection")
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return root, files, nil
}
