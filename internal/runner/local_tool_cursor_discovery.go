package runner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
)

// Named agent/editor/launcher entries remain passive observations. Only the
// fixed Node+index package is closure-checked and given a read-only version
// probe; neither observation supplies a model or execution configuration.
func discoverLocalCursor(ctx context.Context, paths []string) InstalledToolDiscoveryItem {
	return discoverCursorPackage(ctx, paths, func(ctx context.Context, spec CommandSpec) (string, error) { return probeCursorVersion(ctx, spec, nil) })
}
func discoverCursorPackage(ctx context.Context, paths []string, probe func(context.Context, CommandSpec) (string, error)) InstalledToolDiscoveryItem {
	for _, root := range paths {
		// Only the fixed package pair is active discovery. Editor entries and
		// arbitrary programs named agent retain their original passive facts.
		if _, err := os.Lstat(filepath.Join(root, "index.js")); err != nil {
			continue
		}
		spec, err := BuildCursorACPCommand(root, "discovery-only", "discovery-only", "CURSOR_DISCOVERY_CREDENTIAL")
		if err != nil {
			continue
		}
		version, err := probe(ctx, spec)
		if err != nil || !acpLocalText(version, 256) || !cursorPackageUnchanged(*spec.ACP.CursorPackage) {
			return InstalledToolDiscoveryItem{AdapterID: "cursor-cli", Product: "Cursor", Installed: true, ReasonCode: "probe_failed"}
		}
		_, sha, err := qoderNativeTarget(ctx, spec.Executable)
		if err != nil {
			continue
		}
		return InstalledToolDiscoveryItem{AdapterID: "cursor-cli", Product: "Cursor", Installed: true, Executable: spec.Executable, ExecutableSHA256: sha, Version: version, ReasonCode: "acp_configuration_required"}
	}
	return scanLocalCursor(ctx, paths, runtime.GOOS, os.Lstat)
}

func scanLocalCursor(ctx context.Context, paths []string, platform string, lstat func(string) (os.FileInfo, error)) InstalledToolDiscoveryItem {
	item := InstalledToolDiscoveryItem{AdapterID: "cursor-cli", Product: "Cursor", ReasonCode: "not_installed"}
	nativeNames := []string{"agent", "cursor-agent"}
	if platform == "windows" {
		nativeNames = []string{"agent.exe", "cursor-agent.exe"}
	}
	scriptNames := []string{"agent.cmd", "agent.bat", "agent.ps1", "cursor-agent.cmd", "cursor-agent.bat", "cursor-agent.ps1"}
	if platform != "windows" {
		scriptNames = append(scriptNames, "agent.sh", "cursor-agent.sh")
	}
	editorNames := []string{"cursor", "cursor.cmd"}
	if platform == "windows" {
		editorNames = append(editorNames, "cursor.exe")
	}
	var native, script string
	editor, incomplete := false, false
	for _, directory := range paths {
		if ctx.Err() != nil {
			incomplete = true
			break
		}
		// IsAbs on Windows also accepts UNC and device namespaces. Reject
		// those lexically, before even Lstat-ing their root directory.
		if platform == "windows" && (cursorDiscoveryWindowsNamespace(directory) || runtime.GOOS == "windows" && !cursorDiscoveryWindowsDrive(directory)) {
			incomplete = true
			continue
		}
		if !filepath.IsAbs(directory) || !localToolSafeText(directory, 4096) {
			continue
		}
		directory = filepath.Clean(directory)
		exists, safe := cursorDiscoveryDirectory(ctx, directory, lstat)
		if !safe {
			incomplete = true
			continue
		}
		if !exists {
			continue
		}
		for kind, names := range [][]string{nativeNames, scriptNames, editorNames} {
			for _, name := range names {
				if ctx.Err() != nil {
					incomplete = true
					break
				}
				candidate := filepath.Join(directory, name)
				if !localToolSafeText(candidate, 4096) {
					incomplete = true
					continue
				}
				info, err := lstat(candidate)
				if os.IsNotExist(err) {
					continue
				}
				if err != nil || info == nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
					incomplete = true
					continue
				}
				switch kind {
				case 0:
					if native == "" {
						native = candidate
					}
				case 1:
					if script == "" {
						script = candidate
					}
				case 2:
					editor = true
				}
			}
		}
	}
	switch {
	case incomplete || ctx.Err() != nil:
		item.ReasonCode = "cursor_scan_incomplete"
	case native != "":
		item.Installed, item.Executable, item.ReasonCode = true, native, "cursor_agent_unverified"
	case script != "":
		item.Installed, item.Executable, item.ReasonCode = true, script, "cursor_launcher_unsupported"
	case editor:
		item.ReasonCode = "cursor_editor_only"
	}
	return item
}

func cursorDiscoveryWindowsSeparator(character byte) bool {
	return character == '\\' || character == '/'
}

func cursorDiscoveryWindowsNamespace(directory string) bool {
	if len(directory) >= 2 && cursorDiscoveryWindowsSeparator(directory[0]) && cursorDiscoveryWindowsSeparator(directory[1]) {
		return true
	}
	return len(directory) >= 4 && cursorDiscoveryWindowsSeparator(directory[0]) && directory[1:3] == "??" && cursorDiscoveryWindowsSeparator(directory[3])
}

func cursorDiscoveryWindowsDrive(directory string) bool {
	if len(directory) < 3 {
		return false
	}
	letter := directory[0]
	return (letter >= 'A' && letter <= 'Z' || letter >= 'a' && letter <= 'z') && directory[1] == ':' && cursorDiscoveryWindowsSeparator(directory[2])
}

// Lstat every directory component before looking at its fixed child names.
// Following a PATH-directory link would silently inspect a different location.
func cursorDiscoveryDirectory(ctx context.Context, directory string, lstat func(string) (os.FileInfo, error)) (exists, safe bool) {
	parents := []string{}
	for current := directory; ; current = filepath.Dir(current) {
		parents = append(parents, current)
		if filepath.Dir(current) == current {
			break
		}
	}
	for index := len(parents) - 1; index >= 0; index-- {
		if ctx.Err() != nil {
			return false, false
		}
		info, err := lstat(parents[index])
		if os.IsNotExist(err) {
			return false, true
		}
		if err != nil || info == nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return false, false
		}
	}
	return true, true
}
