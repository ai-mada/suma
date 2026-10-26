package runner

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// nativeWorkspacePath is also used by the sealing module. It accepts only one
// portable slash spelling. Filesystem access additionally checks exact case and
// each component through os.Root; lexical validation alone is not a sandbox.
func nativeWorkspacePath(value string) (string, error) {
	if value == "" || len(value) > 1024 || !utf8.ValidString(value) || strings.ContainsAny(value, "\\:\x00") || strings.HasPrefix(value, "/") || !filepath.IsLocal(filepath.FromSlash(value)) {
		return "", nativeError("native_workspace_path")
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.IndexFunc(part, func(r rune) bool { return unicode.IsControl(r) }) >= 0 {
			return "", nativeError("native_workspace_path")
		}
		lower := strings.ToLower(part)
		switch lower {
		case ".git", ".gitattributes", ".gitmodules", ".suma", ".cache", "node_modules", ".aws", ".ssh", ".pnpm-store", ".codex", ".agents", ".kube", ".azure", ".config", ".docker", ".npmrc", ".pypirc", ".netrc", ".git-credentials", "credentials", "credentials.json", "secrets", "secrets.json", "id_rsa", "id_ed25519":
			return "", nativeError("native_workspace_path")
		}
		if strings.HasPrefix(lower, ".env") || strings.HasPrefix(lower, ".suma-save-") || strings.HasPrefix(lower, ".suma-native-") {
			return "", nativeError("native_workspace_path")
		}
		for _, suffix := range []string{".pem", ".key", ".p12", ".pfx"} {
			if strings.HasSuffix(lower, suffix) {
				return "", nativeError("native_workspace_path")
			}
		}
		device := strings.SplitN(lower, ".", 2)[0]
		if device == "con" || device == "prn" || device == "aux" || device == "nul" || device == "conin$" || device == "conout$" {
			return "", nativeError("native_workspace_path")
		}
		if strings.HasPrefix(device, "com") || strings.HasPrefix(device, "lpt") {
			tail := device[3:]
			if len(tail) == 1 && tail[0] >= '1' && tail[0] <= '9' || tail == "¹" || tail == "²" || tail == "³" {
				return "", nativeError("native_workspace_path")
			}
		}
	}
	return value, nil
}

func nativeArguments(name string, raw []byte) (map[string]any, error) {
	if name == "merge_status" || name == "merge_continue" {
		return nativeMergeArguments(name, raw)
	}
	if len(raw) > 2<<20 || !tc.ValidJSONUnicode(raw) {
		return nil, nativeError("native_workspace_arguments")
	}
	args, err := decodeToolObject(raw)
	if err != nil {
		return nil, nativeError("native_workspace_arguments")
	}
	allowed := map[string]string{}
	required := []string{}
	switch name {
	case "list":
		allowed = map[string]string{"directory": "string", "limit": "integer", "cursor": "string"}
	case "read":
		allowed = map[string]string{"path": "path", "start_line": "integer", "max_lines": "integer"}
		required = []string{"path"}
	case "write":
		allowed = map[string]string{"path": "path", "content": "content", "expected_sha256": "sha", "expected_absent": "true"}
		required = []string{"path", "content"}
	case "edit":
		allowed = map[string]string{"path": "path", "expected_sha256": "sha", "edits": "edits"}
		required = []string{"path", "expected_sha256", "edits"}
	case "remove":
		allowed = map[string]string{"path": "path", "expected_sha256": "sha"}
		required = []string{"path", "expected_sha256"}
	case "check":
		allowed = map[string]string{"check_id": "string"}
		required = []string{"check_id"}
	default:
		return nil, nativeError("native_workspace_arguments")
	}
	for key, value := range args {
		kind, ok := allowed[key]
		if !ok || value == nil {
			return nil, nativeError("native_workspace_arguments")
		}
		switch kind {
		case "edits":
			edits, ok := value.([]any)
			if !ok || len(edits) < 1 || len(edits) > 32 {
				return nil, nativeError("native_workspace_arguments")
			}
			for _, item := range edits {
				edit, ok := item.(map[string]any)
				if !ok || len(edit) != 2 {
					return nil, nativeError("native_workspace_arguments")
				}
				old, oldOK := edit["old_text"].(string)
				next, nextOK := edit["new_text"].(string)
				if !oldOK || !nextOK || old == "" || len(old) > nativeWorkspaceFileLimit || len(next) > nativeWorkspaceFileLimit || strings.ContainsRune(old, 0) || strings.ContainsRune(next, 0) {
					return nil, nativeError("native_workspace_arguments")
				}
			}
		case "integer":
			number, ok := value.(json.Number)
			if !ok || !nativeDecimal(string(number)) {
				return nil, nativeError("native_workspace_arguments")
			}
			v, e := strconv.Atoi(string(number))
			if e != nil || v < 1 || key == "limit" && v > 200 || key == "max_lines" && v > 1000 || key == "start_line" && v > nativeWorkspaceFileLimit+1 {
				return nil, nativeError("native_workspace_arguments")
			}
		case "true":
			v, ok := value.(bool)
			if !ok || !v {
				return nil, nativeError("native_workspace_arguments")
			}
		default:
			text, ok := value.(string)
			if !ok || !utf8.ValidString(text) {
				return nil, nativeError("native_workspace_arguments")
			}
			if kind == "content" {
				if len(text) > nativeWorkspaceFileLimit || strings.ContainsRune(text, 0) {
					return nil, nativeError("native_workspace_text")
				}
			} else if text == "" || len(text) > 4096 {
				return nil, nativeError("native_workspace_arguments")
			}
			if kind == "sha" && !evidenceHash.MatchString(text) {
				return nil, nativeError("native_workspace_arguments")
			}
			if kind == "path" {
				if _, e := nativeWorkspacePath(text); e != nil {
					return nil, e
				}
			}
		}
	}
	for _, key := range required {
		if _, ok := args[key]; !ok {
			return nil, nativeError("native_workspace_arguments")
		}
	}
	if name == "write" {
		_, sha := args["expected_sha256"]
		_, absent := args["expected_absent"]
		if sha == absent {
			return nil, nativeError("native_workspace_arguments")
		}
	}
	if directory := nativeString(args, "directory"); directory != "" {
		if _, err := nativeWorkspacePath(directory); err != nil {
			return nil, err
		}
	}
	return args, nil
}
func nativeDecimal(value string) bool {
	if value == "" || value[0] < '1' || value[0] > '9' {
		return false
	}
	for _, b := range []byte(value) {
		if b < '0' || b > '9' {
			return false
		}
	}
	return true
}

func nativeCaseKey(value string) string {
	return strings.Map(func(r rune) rune {
		smallest := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < smallest {
				smallest = next
			}
		}
		return smallest
	}, value)
}

// validatePath checks aliases and links through an already anchored root. The
// actual open/rename/remove also uses that root, so a concurrent directory
// replacement cannot redirect the operation outside the attempt.
func nativeValidatePath(root *os.Root, name string, missing bool) error {
	if _, err := nativeWorkspacePath(name); err != nil {
		return err
	}
	parent := "."
	parts := strings.Split(name, "/")
	for index, part := range parts {
		dir, err := root.Open(parent)
		if err != nil {
			return nativeError("native_workspace_missing")
		}
		entries, err := dir.ReadDir(-1)
		_ = dir.Close()
		if err != nil {
			return nativeError("native_workspace_unavailable")
		}
		found := false
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), part) {
				if entry.Name() != part {
					return nativeError("native_workspace_path")
				}
				found = true
			}
		}
		if !found {
			if missing {
				return nil
			}
			return nativeError("native_workspace_missing")
		}
		current := path.Join(parent, part)
		info, err := root.Lstat(current)
		if err != nil {
			return nativeError("native_workspace_missing")
		}
		if info.Mode()&os.ModeSymlink != 0 || index < len(parts)-1 && !info.IsDir() || index == len(parts)-1 && !info.IsDir() && !info.Mode().IsRegular() {
			return nativeError("native_workspace_path")
		}
		parent = current
	}
	return nil
}

func nativeReadBytes(root *os.Root, name string) ([]byte, bool, error) {
	if err := nativeValidatePath(root, name, true); err != nil {
		return nil, false, err
	}
	f, err := root.Open(name)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, nativeError("native_workspace_unavailable")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return nil, false, nativeError("native_workspace_path")
	}
	if st.Size() > nativeWorkspaceFileLimit {
		return nil, false, nativeError("native_workspace_text")
	}
	b, err := io.ReadAll(io.LimitReader(f, nativeWorkspaceFileLimit+1))
	if err != nil {
		return nil, false, nativeError("native_workspace_unavailable")
	}
	if len(b) > nativeWorkspaceFileLimit || !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
		return nil, false, nativeError("native_workspace_text")
	}
	return b, true, nil
}

type nativeReadResult struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	SHA256    string `json:"sha256"`
	Bytes     int    `json:"bytes"`
	StartLine int    `json:"start_line"`
	NextLine  int    `json:"next_line,omitempty"`
	Complete  bool   `json:"complete"`
}

func (n *nativeWorkspace) readLocked(args map[string]any) (any, error) {
	name := nativeString(args, "path")
	data, exists, err := nativeReadBytes(n.root, name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nativeError("native_workspace_missing")
	}
	start, max := nativeWorkspaceInteger(args, "start_line", 1), nativeWorkspaceInteger(args, "max_lines", 200)
	lines := bytes.SplitAfter(data, []byte{'\n'})
	if len(data) == 0 {
		lines = nil
	} else if len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	if start > len(lines)+1 {
		return nil, nativeError("native_workspace_arguments")
	}
	end := min(len(lines), start-1+max)
	selected := []byte{}
	for _, line := range lines[min(start-1, len(lines)):end] {
		if len(line) > 48<<10 {
			return nil, nativeError("native_workspace_output_limit")
		}
		if len(selected)+len(line) > 48<<10 {
			break
		}
		selected = append(selected, line...)
	}
	count := 0
	if len(selected) > 0 {
		count = bytes.Count(selected, []byte{'\n'})
		if selected[len(selected)-1] != '\n' {
			count++
		}
	}
	next := start + count
	complete := next > len(lines)
	if complete {
		next = 0
	}
	result := nativeReadResult{Path: name, Content: string(selected), SHA256: digest(data), Bytes: len(data), StartLine: start, NextLine: next, Complete: complete}
	b, _ := json.Marshal(result)
	if len(b) > nativeWorkspaceResultLimit {
		return nil, nativeError("native_workspace_output_limit")
	}
	return result, nil
}

type nativeListEntry struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}
type nativeListCursor struct {
	Scope      string `json:"scope"`
	Directory  string `json:"directory"`
	Snapshot   string `json:"snapshot"`
	Offset     int    `json:"offset"`
	Generation int    `json:"generation"`
}

func (n *nativeWorkspace) listLocked(args map[string]any) (any, error) {
	directory := nativeString(args, "directory")
	name := directory
	if name == "" {
		name = "."
	} else if err := nativeValidatePath(n.root, name, false); err != nil {
		return nil, err
	}
	f, err := n.root.Open(name)
	if err != nil {
		return nil, nativeError("native_workspace_missing")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.IsDir() {
		return nil, nativeError("native_workspace_path")
	}
	all, err := f.ReadDir(10001)
	if err != nil && err != io.EOF {
		return nil, nativeError("native_workspace_unavailable")
	}
	if len(all) > 10000 {
		return nil, nativeError("native_workspace_output_limit")
	}
	entries := []nativeListEntry{}
	for _, entry := range all {
		item := entry.Name()
		if directory != "" {
			item = directory + "/" + item
		}
		if _, err := nativeWorkspacePath(item); err != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, nativeError("native_workspace_unavailable")
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular() {
			return nil, nativeError("native_workspace_path")
		}
		kind := "file"
		if info.IsDir() {
			kind = "directory"
		}
		entries = append(entries, nativeListEntry{Path: item, Kind: kind})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	seenNames := map[string]bool{}
	for _, entry := range entries {
		fold := nativeCaseKey(entry.Path)
		if seenNames[fold] {
			return nil, nativeError("native_workspace_path")
		}
		seenNames[fold] = true
	}
	b, _ := json.Marshal(entries)
	snapshot := digest(b)
	offset := 0
	if cursor := nativeString(args, "cursor"); cursor != "" {
		raw, e := base64.RawURLEncoding.Strict().DecodeString(cursor)
		var c nativeListCursor
		parseErr := json.Unmarshal(raw, &c)
		canonical, _ := json.Marshal(c)
		if e != nil || len(raw) > 4096 || parseErr != nil || !bytes.Equal(raw, canonical) || c.Scope != digest([]byte(key(n.assignment)+":"+n.policySHA)) || c.Directory != directory || c.Snapshot != snapshot || c.Generation != n.ledger.Generation || c.Offset < 1 || c.Offset >= len(entries) {
			return nil, nativeError("native_workspace_cursor")
		}
		offset = c.Offset
	}
	end := min(len(entries), offset+nativeWorkspaceInteger(args, "limit", 100))
	next := ""
	if end < len(entries) {
		b, _ := json.Marshal(nativeListCursor{Scope: digest([]byte(key(n.assignment) + ":" + n.policySHA)), Directory: directory, Snapshot: snapshot, Offset: end, Generation: n.ledger.Generation})
		next = base64.RawURLEncoding.EncodeToString(b)
	}
	return map[string]any{"entries": entries[offset:end], "next_cursor": next, "complete": next == ""}, nil
}

func (n *nativeWorkspace) snapshotLocked() (map[string]string, string, error) {
	root := n.root
	if root == nil {
		var err error
		root, err = os.OpenRoot(n.workspace.Source)
		if err != nil {
			return nil, "", nativeError("native_workspace_scope")
		}
		defer root.Close()
	}
	st, err := os.Stat(n.workspace.Source)
	if err != nil || !os.SameFile(st, n.identity) {
		return nil, "", nativeError("native_workspace_scope")
	}
	files := map[string]string{}
	total := int64(0)
	var walk func(string) error
	walk = func(dir string) error {
		f, err := root.Open(dir)
		if err != nil {
			return nativeError("native_workspace_unavailable")
		}
		entries, err := f.ReadDir(10001)
		_ = f.Close()
		if err != nil && err != io.EOF {
			return nativeError("native_workspace_unavailable")
		}
		if len(entries) > 10000 {
			return nativeError("native_workspace_output_limit")
		}
		seen := map[string]bool{}
		for _, entry := range entries {
			name := entry.Name()
			if dir != "." {
				name = dir + "/" + name
			}
			if _, e := nativeWorkspacePath(name); e != nil {
				continue
			}
			lower := nativeCaseKey(entry.Name())
			if seen[lower] {
				return nativeError("native_workspace_path")
			}
			seen[lower] = true
			info, e := root.Lstat(name)
			if e != nil {
				return nativeError("native_workspace_unavailable")
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nativeError("native_workspace_path")
			}
			if info.IsDir() {
				if e = walk(name); e != nil {
					return e
				}
				continue
			}
			if !info.Mode().IsRegular() || info.Size() > 16<<20 {
				return nativeError("native_workspace_path")
			}
			total += info.Size()
			if total > 64<<20 || len(files) >= 10000 {
				return nativeError("native_workspace_output_limit")
			}
			f, e := root.Open(name)
			if e != nil {
				return nativeError("native_workspace_unavailable")
			}
			b, e := io.ReadAll(io.LimitReader(f, 16<<20+1))
			_ = f.Close()
			if e != nil || len(b) > 16<<20 {
				return nativeError("native_workspace_unavailable")
			}
			files[name] = digest(b)
		}
		return nil
	}
	if err = walk("."); err != nil {
		return nil, "", err
	}
	raw, _ := json.Marshal(files)
	return files, digest(raw), nil
}

func (n *nativeWorkspace) verifySourceLocked() (map[string]string, string, error) {
	files, sha, err := n.snapshotLocked()
	if err != nil {
		return nil, "", err
	}
	expected := map[string]string{}
	for p, h := range n.ledger.Initial {
		expected[p] = h
	}
	for p, e := range n.ledger.Files {
		if e.Removed {
			delete(expected, p)
		} else {
			expected[p] = e.SHA256
		}
	}
	a, _ := json.Marshal(files)
	b, _ := json.Marshal(expected)
	if !bytes.Equal(a, b) {
		return nil, "", nativeError("native_workspace_unknown")
	}
	return files, sha, nil
}
