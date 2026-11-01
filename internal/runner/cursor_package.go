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
	"reflect"
	"runtime"
	"sort"
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
		info, err := os.Lstat(current)
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
		info, err := os.Lstat(parent.path)
		if err != nil || !cursorPackageOrdinary(info) || !info.IsDir() || !os.SameFile(parent.info, info) {
			return false
		}
	}
	return true
}

func inspectCursorPackage(root string) (CursorPackageSpec, string, error) {
	root, parents, err := cursorPackageRoot(root)
	if err != nil {
		return CursorPackageSpec{}, "", err
	}
	owned, err := os.OpenRoot(root)
	if err != nil {
		return CursorPackageSpec{}, "", fmt.Errorf("Cursor package directory is unavailable")
	}
	defer owned.Close()
	anchored, err := owned.Stat(".")
	if err != nil || !os.SameFile(parents[0].info, anchored) || !cursorPackageParentsStable(parents) {
		return CursorPackageSpec{}, "", fmt.Errorf("Cursor package directory changed while opening")
	}
	files := []cursorPackageFile{}
	nodes := []cursorPackageNode{}
	directories := 0
	var total int64
	var scan func(string) error
	scan = func(relative string) error {
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
		children, readErr := directory.ReadDir(-1)
		closeErr := directory.Close()
		if readErr != nil || closeErr != nil {
			return fmt.Errorf("Cursor package directory could not be enumerated")
		}
		nodes = append(nodes, cursorPackageNode{relative, before})
		for _, child := range children {
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
		return CursorPackageSpec{}, "", err
	}
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
		return CursorPackageSpec{}, "", fmt.Errorf("Cursor package requires its fixed native Node and index.js")
	}
	node, nativeSHA, err := qoderNativeTarget(context.Background(), filepath.Join(root, nodeName))
	if err != nil || !sameACPPath(node, filepath.Join(root, nodeName)) || nativeSHA != nodeSHA {
		return CursorPackageSpec{}, "", fmt.Errorf("Cursor package native host is invalid or changed")
	}
	if !cursorPackageParentsStable(parents) {
		return CursorPackageSpec{}, "", fmt.Errorf("Cursor package parent identity changed")
	}
	for _, previous := range nodes {
		current, err := owned.Lstat(previous.path)
		if err != nil || !cursorPackageOrdinary(current) || !os.SameFile(previous.info, current) || current.Size() != previous.info.Size() || !current.ModTime().Equal(previous.info.ModTime()) {
			return CursorPackageSpec{}, "", fmt.Errorf("Cursor package tree changed during inspection")
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
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
