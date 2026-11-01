package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Cursor also imports executable settings from project ancestors and from
// Claude-compatible settings. Check their presence without reading any body;
// ordinary rules and instruction files remain available to the agent.
func validateCursorWorkspace(source string) error {
	return validateCursorWorkspacePaths(source, cursorEnterpriseHookPaths())
}

func cursorEnterpriseHookPaths() []string {
	// These are the pinned loader's fixed paths, independent of HOME and of the
	// ProgramData environment variable. The generated private home cannot hide
	// an enterprise hook, so its presence requires refusing this local launch.
	switch runtime.GOOS {
	case "windows":
		return []string{`C:\ProgramData\Cursor\hooks.json`}
	case "darwin":
		return []string{"/Library/Application Support/Cursor/hooks.json"}
	default:
		return []string{"/etc/cursor/hooks.json"}
	}
}

func validateCursorWorkspacePaths(source string, enterprisePaths []string) error {
	_, parents, err := cursorPackageRoot(source)
	if err != nil {
		return fmt.Errorf("Cursor workspace must be an ordinary local directory")
	}
	for _, parent := range parents {
		if err := cursorWorkspaceScope(parent); err != nil {
			return err
		}
	}
	if !cursorPackageParentsStable(parents) {
		return fmt.Errorf("Cursor workspace ancestor changed during inspection")
	}
	return validateCursorEnterpriseHooks(enterprisePaths)
}

func cursorWorkspaceScope(parent cursorPackageNode) error {
	scope, err := os.OpenRoot(parent.path)
	if err != nil {
		return fmt.Errorf("Cursor workspace ancestor is unavailable")
	}
	defer scope.Close()
	info, err := scope.Stat(".")
	if err != nil || !os.SameFile(parent.info, info) || !cursorPackageOrdinary(info) || !info.IsDir() {
		return fmt.Errorf("Cursor workspace ancestor changed while opening")
	}
	for _, setting := range []struct {
		directory string
		files     []string
	}{
		{".cursor", []string{"cli.json", "mcp.json", "hooks.json"}},
		{".claude", []string{"settings.json", "settings.local.json"}},
	} {
		before, err := scope.Lstat(setting.directory)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !cursorPackageOrdinary(before) || !before.IsDir() {
			return fmt.Errorf("Cursor workspace configuration directory is linked or unavailable")
		}
		configuration, err := scope.OpenRoot(setting.directory)
		if err != nil {
			return fmt.Errorf("Cursor workspace configuration directory could not be inspected")
		}
		opened, err := configuration.Stat(".")
		if err != nil || !os.SameFile(before, opened) || !cursorPackageOrdinary(opened) || !opened.IsDir() {
			_ = configuration.Close()
			return fmt.Errorf("Cursor workspace configuration directory changed")
		}
		for _, name := range setting.files {
			_, err := configuration.Lstat(name)
			if os.IsNotExist(err) {
				continue
			}
			_ = configuration.Close()
			if err != nil {
				return fmt.Errorf("Cursor workspace executable configuration could not be inspected")
			}
			return fmt.Errorf("Cursor workspace or ancestor contains executable configuration %s/%s", setting.directory, name)
		}
		closeErr := configuration.Close()
		after, err := scope.Lstat(setting.directory)
		if closeErr != nil || err != nil || !cursorPackageOrdinary(after) || !after.IsDir() || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
			return fmt.Errorf("Cursor workspace configuration directory changed during inspection")
		}
	}
	return nil
}

func validateCursorEnterpriseHooks(paths []string) error {
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("Cursor enterprise hook path is invalid")
		}
		ancestors := []string{}
		for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
			ancestors = append(ancestors, parent)
			if filepath.Dir(parent) == parent {
				break
			}
		}
		parents := []cursorPackageNode{}
		absent := false
		for i := len(ancestors) - 1; i >= 0; i-- {
			info, err := os.Lstat(ancestors[i])
			if os.IsNotExist(err) {
				absent = true
				break
			}
			if err != nil || !cursorPackageOrdinary(info) || !info.IsDir() {
				return fmt.Errorf("Cursor enterprise hook parent is linked or unavailable")
			}
			parents = append([]cursorPackageNode{{ancestors[i], info}}, parents...)
		}
		if !absent {
			_, err := os.Lstat(path)
			if err == nil {
				return fmt.Errorf("Cursor enterprise hooks must be absent for this isolated execution")
			}
			if !os.IsNotExist(err) {
				return fmt.Errorf("Cursor enterprise hooks could not be inspected")
			}
		}
		if !cursorPackageParentsStable(parents) {
			return fmt.Errorf("Cursor enterprise hook parent changed during inspection")
		}
	}
	return nil
}
