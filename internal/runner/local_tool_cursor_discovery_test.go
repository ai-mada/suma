package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func cursorDiscoveryItem(t *testing.T, ctx context.Context, paths []string) (InstalledToolDiscoveryItem, error) {
	t.Helper()
	result, err := discoverInstalledTools(ctx, "cursor-cli", paths, func(context.Context, CommandSpec) (string, error) {
		t.Fatal("a passive Cursor candidate reached the executable probe")
		return "", nil
	})
	if err != nil {
		return InstalledToolDiscoveryItem{}, err
	}
	if result.Protocol != InstalledToolDiscoveryProtocol || len(result.Items) != 1 {
		t.Fatalf("incorrect passive envelope: %+v", result)
	}
	return result.Items[0], nil
}

func TestLocalCursorDiscoveryPassiveWireAndNativePriority(t *testing.T) {
	native := "agent"
	editor := "cursor"
	if runtime.GOOS == "windows" {
		native += ".exe"
		editor += ".exe"
	}
	for _, scenario := range []struct {
		name, reason string
		files        []string
		want         string
	}{
		{"missing", "not_installed", nil, ""},
		{"editor-only", "cursor_editor_only", []string{editor}, ""},
		{"editor-command", "cursor_editor_only", []string{"cursor.cmd"}, ""},
		{"generic-agent-is-unverified", "cursor_agent_unverified", []string{native}, native},
		{"script-is-not-launchable", "cursor_launcher_unsupported", []string{"cursor-agent.ps1"}, "cursor-agent.ps1"},
		{"native-over-script", "cursor_agent_unverified", []string{"cursor-agent.cmd", native}, native},
		{"non-fixed-name-is-ignored", "not_installed", []string{"agent-helper.exe", "cursor-agent.js"}, ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range scenario.files {
				localDiscoveryFile(t, filepath.Join(root, name), "arbitrary candidate; must never execute or inspect contents")
			}
			item, err := cursorDiscoveryItem(t, context.Background(), []string{root})
			if err != nil || item.AdapterID != "cursor-cli" || item.Product != "Cursor" || item.ReasonCode != scenario.reason || item.LaunchSupported || item.Command != nil || item.Version != "" || item.Installed != (scenario.want != "") {
				t.Fatalf("incorrect passive facts: %+v %v", item, err)
			}
			want := ""
			if scenario.want != "" {
				want = filepath.Join(root, scenario.want)
			}
			if item.Executable != want {
				t.Fatalf("candidate disclosure=%q, want %q", item.Executable, want)
			}
			raw, _ := json.Marshal(item)
			var wire map[string]any
			_ = json.Unmarshal(raw, &wire)
			wantFields := 5
			if item.Installed {
				wantFields++
			}
			if len(wire) != wantFields || wire["launch_supported"] != false || wire["version"] != nil || wire["command"] != nil || !item.Installed && wire["executable"] != nil {
				t.Fatalf("passive wire granted authority or disclosed an unconfirmed path: %s", raw)
			}
		})
	}
	first, second := t.TempDir(), t.TempDir()
	localDiscoveryFile(t, filepath.Join(first, "agent.cmd"), "never run")
	want := localDiscoveryFile(t, filepath.Join(second, strings.Replace(native, "agent", "cursor-agent", 1)), "also never run")
	item, err := cursorDiscoveryItem(t, context.Background(), []string{first, second})
	if err != nil || item.ReasonCode != "cursor_agent_unverified" || item.Executable != want {
		t.Fatalf("native in later PATH directory did not outrank script: %+v %v", item, err)
	}
}

func TestLocalCursorDiscoveryAccessErrorsAndPlatformNames(t *testing.T) {
	root := t.TempDir()
	denied := filepath.Join(root, "agent.exe")
	localDiscoveryFile(t, filepath.Join(root, "cursor-agent.exe"), "unverified later candidate")
	item := scanLocalCursor(context.Background(), []string{root}, "windows", func(name string) (os.FileInfo, error) {
		if name == denied {
			return nil, os.ErrPermission
		}
		return os.Lstat(name)
	})
	if item.ReasonCode != "cursor_scan_incomplete" || item.Installed || item.Executable != "" || item.Command != nil || item.Version != "" {
		t.Fatalf("access failure was hidden by missing or later evidence: %+v", item)
	}
	for _, scenario := range []struct{ platform, native, script string }{
		{"windows", "cursor-agent.exe", "cursor-agent.cmd"},
		{"linux", "cursor-agent", "cursor-agent.sh"},
	} {
		t.Run(scenario.platform, func(t *testing.T) {
			root := t.TempDir()
			wantScript := localDiscoveryFile(t, filepath.Join(root, scenario.script), "never run launcher")
			item := scanLocalCursor(context.Background(), []string{root}, scenario.platform, os.Lstat)
			if item.ReasonCode != "cursor_launcher_unsupported" || item.Executable != wantScript || item.LaunchSupported {
				t.Fatalf("script filename not passive: %+v", item)
			}
			wantNative := localDiscoveryFile(t, filepath.Join(root, scenario.native), "filename does not prove file format")
			item = scanLocalCursor(context.Background(), []string{root}, scenario.platform, os.Lstat)
			if item.ReasonCode != "cursor_agent_unverified" || item.Executable != wantNative || item.LaunchSupported {
				t.Fatalf("native filename not passive: %+v", item)
			}
		})
	}
}

func TestLocalCursorDiscoveryUnsafeFilesystemIsIncomplete(t *testing.T) {
	native := "agent"
	if runtime.GOOS == "windows" {
		native += ".exe"
	}
	t.Run("candidate-directory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, native), 0700); err != nil {
			t.Fatal(err)
		}
		item, err := cursorDiscoveryItem(t, context.Background(), []string{root})
		if err != nil || item.ReasonCode != "cursor_scan_incomplete" || item.Installed || item.Executable != "" {
			t.Fatalf("non-regular candidate became missing or launchable: %+v %v", item, err)
		}
	})
	t.Run("path-entry-is-file", func(t *testing.T) {
		root := t.TempDir()
		entry := localDiscoveryFile(t, filepath.Join(root, "not-a-directory"), "owned file")
		item, err := cursorDiscoveryItem(t, context.Background(), []string{entry})
		if err != nil || item.ReasonCode != "cursor_scan_incomplete" || item.Installed || item.Executable != "" {
			t.Fatalf("invalid PATH access became not installed: %+v %v", item, err)
		}
	})
	t.Run("candidate-symlink", func(t *testing.T) {
		root := t.TempDir()
		target := localDiscoveryFile(t, filepath.Join(t.TempDir(), "outside"), "never follow")
		if err := os.Symlink(target, filepath.Join(root, native)); err != nil {
			t.Skip("file symlinks unavailable")
		}
		item, err := cursorDiscoveryItem(t, context.Background(), []string{root})
		if err != nil || item.ReasonCode != "cursor_scan_incomplete" || item.Installed || item.Executable != "" {
			t.Fatalf("symlink candidate escaped passive scan: %+v %v", item, err)
		}
	})
	t.Run("path-parent-symlink", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		localDiscoveryFile(t, filepath.Join(outside, "bin", native), "never follow directory link")
		link := filepath.Join(root, "link")
		if err := os.Symlink(outside, link); err != nil {
			t.Skip("directory symlinks unavailable")
		}
		item, err := cursorDiscoveryItem(t, context.Background(), []string{filepath.Join(link, "bin")})
		if err != nil || item.ReasonCode != "cursor_scan_incomplete" || item.Installed || item.Executable != "" {
			t.Fatalf("intermediate directory link escaped scan: %+v %v", item, err)
		}
	})
}

func TestLocalCursorDiscoveryCancellationRemainsError(t *testing.T) {
	for _, expired := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if expired {
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		} else {
			cancel()
		}
		_, err := cursorDiscoveryItem(t, ctx, []string{t.TempDir()})
		cancel()
		if err == nil || !strings.Contains(err.Error(), "cancelled or timed out") {
			t.Fatalf("cancelled scan was replaced by an installation observation: %v", err)
		}
	}
}

func TestLocalCursorDiscoveryWindowsLocalPathsOnly(t *testing.T) {
	for _, directory := range []string{
		`\\cursor-test.invalid\share\bin`,
		`//cursor-test.invalid/share/bin`,
		`\\?\C:\tools`,
		`\\.\C:\tools`,
		`\\.\pipe\cursor`,
		`\\?\UNC\cursor-test.invalid\share`,
		`/\cursor-test.invalid\share`,
		`\??\C:\tools`,
	} {
		calls := 0
		item := scanLocalCursor(context.Background(), []string{directory}, "windows", func(string) (os.FileInfo, error) {
			calls++
			return nil, os.ErrNotExist
		})
		if calls != 0 || item.ReasonCode != "cursor_scan_incomplete" || item.Installed || item.Executable != "" || item.LaunchSupported || item.Command != nil || item.Version != "" {
			t.Fatalf("remote/device path reached filesystem or yielded facts: calls=%d item=%+v", calls, item)
		}
	}
	// The guard must still allow the current OS's ordinary local absolute PATH.
	calls := 0
	item := scanLocalCursor(context.Background(), []string{t.TempDir()}, runtime.GOOS, func(string) (os.FileInfo, error) {
		calls++
		return nil, os.ErrNotExist
	})
	if calls == 0 || item.ReasonCode != "not_installed" {
		t.Fatalf("ordinary local path was blocked: calls=%d item=%+v", calls, item)
	}
}
