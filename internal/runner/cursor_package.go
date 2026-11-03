package runner

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
)

// CursorPackageSpec pins the complete local distribution, including its native
// host and every dependency. Inspection never runs that host or writes a file.
type CursorPackageSpec struct {
	PackageRoot string `json:"package_root"`
	SHA256      string `json:"sha256"`
}

const cursorPackageMaxFiles = 10000
const cursorPackageMaxBytes int64 = 1 << 30

type cursorPackageFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type cursorPackageNode struct {
	path string
	info os.FileInfo
}

func cursorPackageReparse(info os.FileInfo) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	// Windows FileInfo.Sys exposes Win32FileAttributeData. Inspect its public
	// attribute instead of assuming all reparse tags appear as ModeSymlink:
	// for example, Go deliberately reports DEDUP reparse files as regular.
	value := reflect.ValueOf(info.Sys())
	if value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return true
	}
	attributes := value.Elem().FieldByName("FileAttributes")
	if !attributes.IsValid() || attributes.Kind() != reflect.Uint32 {
		return true
	}
	return attributes.Uint()&0x400 != 0 // FILE_ATTRIBUTE_REPARSE_POINT
}

func cursorPackageOrdinary(info os.FileInfo) bool {
	return info != nil && !cursorPackageReparse(info) && (info.Mode().IsRegular() || info.Mode().Type() == os.ModeDir)
}

func cursorPackageRoot(root string) (string, []cursorPackageNode, error) {
	if root == "" || !filepath.IsAbs(root) || strings.TrimSpace(root) != root || strings.ContainsAny(root, "\x00\r\n") {
		return "", nil, fmt.Errorf("Cursor package requires an absolute local directory")
	}
	if runtime.GOOS == "windows" && (!cursorDiscoveryWindowsDrive(root) || cursorDiscoveryWindowsNamespace(root)) {
		return "", nil, fmt.Errorf("Cursor package requires a local drive path")
	}
	for _, part := range strings.FieldsFunc(strings.TrimPrefix(root, filepath.VolumeName(root)), func(r rune) bool {
		return r == '/' || runtime.GOOS == "windows" && r == '\\'
	}) {
		if part == ".." || runtime.GOOS == "windows" && part != "." && (strings.TrimRight(part, ". ") != part || strings.ContainsRune(part, ':')) {
			return "", nil, fmt.Errorf("Cursor package path has an unsupported alias")
		}
	}
	root = filepath.Clean(root)
	parents := []cursorPackageNode{}
	for current := root; ; current = filepath.Dir(current) {
		info, err := directoryIdentity(current)
		if err != nil || !cursorPackageOrdinary(info) || !info.IsDir() {
			return "", nil, fmt.Errorf("Cursor package directory traverses an unavailable or linked parent")
		}
		parents = append(parents, cursorPackageNode{current, info})
		if filepath.Dir(current) == current {
			break
		}
	}
	return root, parents, nil
}

func cursorPackageParentsStable(parents []cursorPackageNode) bool {
	// Inspect outer ancestors first, before traversing their descendants.
	for i := len(parents) - 1; i >= 0; i-- {
		parent := parents[i]
		info, err := directoryIdentity(parent.path)
		if err != nil || !cursorPackageOrdinary(info) || !info.IsDir() || !os.SameFile(parent.info, info) {
			return false
		}
	}
	return true
}

func inspectCursorPackage(root string) (CursorPackageSpec, string, error) {
	var node string
	root, files, err := inspectPinnedInstallationTree(context.Background(), root, func(root string, files []cursorPackageFile) error {
		nodeName := "node"
		if runtime.GOOS == "windows" {
			nodeName += ".exe"
		}
		var nodeSHA string
		indexFound := false
		for _, file := range files {
			if file.Path == nodeName {
				nodeSHA = file.SHA256
			}
			indexFound = indexFound || file.Path == "index.js"
		}
		if nodeSHA == "" || !indexFound {
			return fmt.Errorf("Cursor package requires its fixed native Node and index.js")
		}
		current, sha, err := qoderNativeTarget(context.Background(), filepath.Join(root, nodeName))
		if err != nil || !sameACPPath(current, filepath.Join(root, nodeName)) || sha != nodeSHA {
			return fmt.Errorf("Cursor package native host is invalid or changed")
		}
		node = current
		return nil
	})
	if err != nil {
		return CursorPackageSpec{}, "", err
	}
	raw, _ := json.Marshal(struct {
		Protocol string              `json:"protocol"`
		Files    []cursorPackageFile `json:"files"`
	}{"suma-cursor-package-v1", files})
	return CursorPackageSpec{PackageRoot: root, SHA256: digest(raw)}, node, nil
}

func validateCursorPackage(spec CursorPackageSpec) (string, error) {
	if len(spec.SHA256) != 64 || strings.ToLower(spec.SHA256) != spec.SHA256 {
		return "", fmt.Errorf("Cursor package requires its canonical closure digest")
	}
	if _, err := hex.DecodeString(spec.SHA256); err != nil {
		return "", fmt.Errorf("Cursor package closure digest is invalid")
	}
	current, node, err := inspectCursorPackage(spec.PackageRoot)
	if err != nil || current != spec {
		return "", fmt.Errorf("Cursor package differs from its reviewed complete closure")
	}
	return node, nil
}
