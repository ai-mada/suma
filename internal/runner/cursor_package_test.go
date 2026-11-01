package runner

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func cursorPackageFixture189(t *testing.T) (string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "package")
	if err := os.MkdirAll(filepath.Join(root, "dependencies"), 0700); err != nil {
		t.Fatal(err)
	}
	nodeName := "node"
	if runtime.GOOS == "windows" {
		nodeName += ".exe"
	}
	node := filepath.Join(root, nodeName)
	current, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(current)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	target, err := os.OpenFile(node, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(target, source)
	if closeErr := target.Close(); copyErr != nil || closeErr != nil {
		t.Fatal("owned native fixture copy unavailable", copyErr, closeErr)
	}
	for name, body := range map[string]string{
		"index.js":                "throw new Error('package inspection must never execute this file');\n",
		"dependencies/runtime.js": "exports.owned = 'dependency-one';\n",
		"cursor-agent.cmd":        "@echo launcher must never run\r\n",
	} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root, node
}

func TestCursorPackageClosureIsStableAndPinsEveryDependency(t *testing.T) {
	root, node := cursorPackageFixture189(t)
	spec, selected, err := inspectCursorPackage(root + string(filepath.Separator) + ".")
	if err != nil || spec.PackageRoot != root || len(spec.SHA256) != 64 || selected != node {
		t.Fatal("complete owned package could not be inspected without execution", err)
	}
	again, sameNode, err := inspectCursorPackage(root)
	if err != nil || again != spec || sameNode != node {
		t.Fatal("unchanged package produced a different closure", err)
	}
	if verified, err := validateCursorPackage(spec); err != nil || verified != node {
		t.Fatal("current exact package closure was rejected", err)
	}
	dependency := filepath.Join(root, "dependencies", "runtime.js")
	if err := os.WriteFile(dependency, []byte("exports.owned = 'dependency-two';\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateCursorPackage(spec); err == nil {
		t.Fatal("dependency change retained the original execution identity")
	}
	changed, _, err := inspectCursorPackage(root)
	if err != nil || changed.SHA256 == spec.SHA256 {
		t.Fatal("dependency bytes were omitted from the closure", err)
	}
	if err := os.WriteFile(dependency, []byte("exports.owned = 'dependency-one';\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if verified, err := validateCursorPackage(spec); err != nil || verified != node {
		t.Fatal("same original bytes did not restore the deterministic closure", err)
	}
	if err := os.WriteFile(filepath.Join(root, "new-dependency.js"), []byte("new dependency\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateCursorPackage(spec); err == nil {
		t.Fatal("additional dependency was not bound into the package identity")
	}
	badHash := spec
	badHash.SHA256 = strings.ToUpper(spec.SHA256)
	if _, err := validateCursorPackage(badHash); err == nil {
		t.Fatal("noncanonical closure digest was accepted")
	}
}

func TestCursorPackageRejectsLinkedTreeAndAncestors(t *testing.T) {
	root, _ := cursorPackageFixture189(t)
	outside := t.TempDir()
	link := filepath.Join(root, "linked-dependency")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS != "windows" {
			t.Skip("directory link creation unavailable:", err)
		}
		if output, junctionErr := exec.Command("cmd.exe", "/c", "mklink", "/J", link, outside).CombinedOutput(); junctionErr != nil {
			t.Skip("directory link creation unavailable:", err, junctionErr, string(output))
		}
	}
	if _, _, err := inspectCursorPackage(root); err == nil {
		t.Fatal("package closure followed an unknown linked dependency")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	ancestor := filepath.Join(t.TempDir(), "linked-parent")
	if err := os.Symlink(filepath.Dir(root), ancestor); err != nil {
		if runtime.GOOS != "windows" {
			t.Skip("ancestor link creation unavailable:", err)
		}
		if output, junctionErr := exec.Command("cmd.exe", "/c", "mklink", "/J", ancestor, filepath.Dir(root)).CombinedOutput(); junctionErr != nil {
			t.Skip("ancestor link creation unavailable:", err, junctionErr, string(output))
		}
	}
	if _, _, err := inspectCursorPackage(filepath.Join(ancestor, filepath.Base(root))); err == nil {
		t.Fatal("package root traversed a linked ancestor")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatal("inspecting a rejected link modified its target", err)
	}
}

func TestCursorPackageRejectsMissingEntryAndUnsafeRoot(t *testing.T) {
	root, node := cursorPackageFixture189(t)
	for _, bad := range []string{"", ".", filepath.Join(root, "missing"), filepath.Join(root, "index.js")} {
		if _, _, err := inspectCursorPackage(bad); err == nil {
			t.Fatal("missing, relative or file package root was accepted")
		}
	}
	if runtime.GOOS == "windows" {
		for _, bad := range []string{`\\server\share\package`, `\\?\` + root, root + ".", root + " "} {
			if _, _, err := inspectCursorPackage(bad); err == nil {
				t.Fatal("nonlocal namespace or Windows alias was accepted")
			}
		}
	}
	index := filepath.Join(root, "index.js")
	if err := os.Rename(index, index+".saved"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inspectCursorPackage(root); err == nil {
		t.Fatal("package without the fixed JavaScript entry was accepted")
	}
	if err := os.Mkdir(index, 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inspectCursorPackage(root); err == nil {
		t.Fatal("directory masquerading as index.js was accepted")
	}
	if err := os.Remove(index); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(index+".saved", index); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(node, node+".saved"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inspectCursorPackage(root); err == nil {
		t.Fatal("package without its fixed native host was accepted")
	}
	if err := os.WriteFile(node, []byte("#!/bin/sh\necho launcher\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inspectCursorPackage(root); err == nil {
		t.Fatal("launcher content masquerading as the native host was accepted")
	}
}
