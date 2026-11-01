package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func preparationDiscoveryFixture(t *testing.T) InstalledToolDiscovery {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	result := InstalledToolDiscovery{Protocol: InstalledToolDiscoveryProtocol, Items: []InstalledToolDiscoveryItem{}}
	for _, entry := range localToolDirectory() {
		item := InstalledToolDiscoveryItem{AdapterID: entry.id, Installed: true, ReasonCode: "configuration_required"}
		if entry.id == "cursor-cli" {
			item.ReasonCode = "cursor_agent_unverified"
		}
		if entry.id != "codex-app-server" && entry.command.Adapter != "" {
			spec := entry.command
			spec.Executable = exe
			item.Command, item.LaunchSupported, item.ReasonCode = &spec, true, "workspace_required"
		}
		result.Items = append(result.Items, item)
	}
	return result
}

func TestPrepareToolCommandsWritesOriginalSpecsAndKeepsConfigurationOnlyFacts(t *testing.T) {
	t.Setenv("SUMA_RUNNER_TOKEN", "private-platform-token-must-not-be-written")
	file := filepath.Join(t.TempDir(), "commands.json")
	report := preparationDiscoveryFixture(t)
	calls := 0
	result, err := prepareToolCommands(context.Background(), file, "", func(_ context.Context, id string) (InstalledToolDiscovery, error) {
		calls++
		if id != "" {
			t.Fatal("changed original discovery scope")
		}
		return report, nil
	})
	if err != nil || calls != 1 || !result.Prepared || result.File != file || result.CommandCount != 4 || len(result.Remaining) != 4 {
		t.Fatalf("incorrect preparation facts: %+v %v", result, err)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := DecodeCommands(raw)
	if err != nil || len(commands) != 4 || strings.Contains(string(raw), "private-platform-token") {
		t.Fatal("configuration did not preserve safe original command contract", err)
	}
	for _, item := range report.Items {
		command, present := commands["local-"+item.AdapterID]
		if present != (item.Command != nil) {
			t.Fatal("installation-only observation generated authority", item.AdapterID)
		}
		if present {
			want, _ := json.Marshal(item.Command)
			got, _ := json.Marshal(command)
			if string(want) != string(got) {
				t.Fatal("fixed scanner policy changed", item.AdapterID)
			}
		}
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(file)
		if info.Mode().Perm() != 0600 {
			t.Fatal("configuration is not private")
		}
	}
}

func TestPrepareToolCommandsNeverOverwritesOrCreatesMissingParents(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "commands.json")
	const original = "unique existing configuration"
	if err := os.WriteFile(file, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	discover := func(context.Context, string) (InstalledToolDiscovery, error) {
		calls++
		return preparationDiscoveryFixture(t), nil
	}
	for _, destination := range []string{file, root, filepath.Join(root, "absent", "commands.json")} {
		if _, err := prepareToolCommands(context.Background(), destination, "", discover); err == nil {
			t.Fatal("unsafe destination was accepted", destination)
		}
	}
	if calls != 0 {
		t.Fatal("unsafe destination launched local discovery")
	}
	if raw, err := os.ReadFile(file); err != nil || string(raw) != original {
		t.Fatal("existing configuration was modified", err)
	}
}

func TestPrepareToolCommandsRejectsLinkedDestinationAndAncestors(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS != "windows" {
			t.Skip("directory symlink creation unavailable on this host:", err)
		}
		// Windows directory junctions exercise the same unknown-target boundary
		// without requesting the machine's symbolic-link privilege.
		if output, junctionErr := exec.Command("cmd.exe", "/c", "mklink", "/J", link, target).CombinedOutput(); junctionErr != nil {
			t.Skip("directory link creation unavailable on this host:", err, junctionErr, string(output))
		}
	}
	for _, destination := range []string{link, filepath.Join(link, "commands.json")} {
		if _, err := prepareToolCommands(context.Background(), destination, "", func(context.Context, string) (InstalledToolDiscovery, error) {
			t.Fatal("linked destination launched discovery")
			return InstalledToolDiscovery{}, nil
		}); err == nil {
			t.Fatal("followed linked destination")
		}
	}
	if entries, err := os.ReadDir(target); err != nil || len(entries) != 0 {
		t.Fatal("link target was modified", err)
	}
}

func TestPrepareToolCommandsRejectsInvalidDiscoveryWithoutWriting(t *testing.T) {
	for _, kind := range []string{"protocol", "duplicate", "configuration"} {
		t.Run(kind, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "commands.json")
			report := preparationDiscoveryFixture(t)
			switch kind {
			case "protocol":
				report.Protocol = "other"
			case "duplicate":
				report.Items[1].AdapterID = report.Items[0].AdapterID
			case "configuration":
				report.Items[0].Command.Protocol = "invalid"
			}
			_, err := prepareToolCommands(context.Background(), file, "", func(context.Context, string) (InstalledToolDiscovery, error) { return report, nil })
			if err == nil {
				t.Fatal("invalid discovery generated a command file")
			}
			if _, statErr := os.Lstat(file); !os.IsNotExist(statErr) {
				t.Fatal("invalid discovery left a command file", statErr)
			}
		})
	}
}
