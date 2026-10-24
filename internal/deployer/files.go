package deployer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ai-mada/suma/internal/releasecontract"
)

func inside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
func safeDirectory(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	missing := []string{}
	for dir := abs; ; dir = filepath.Dir(dir) {
		info, e := os.Lstat(dir)
		if os.IsNotExist(e) {
			missing = append(missing, dir)
		} else if e != nil {
			return e
		} else {
			resolved, e := filepath.EvalSymlinks(dir)
			if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !strings.EqualFold(resolved, dir) {
				return fmt.Errorf("provider directory linked or unavailable")
			}
			break
		}
		if filepath.Dir(dir) == dir {
			return fmt.Errorf("provider root unavailable")
		}
	}
	for i := len(missing) - 1; i >= 0; i-- {
		if err = os.Mkdir(missing[i], 0700); err != nil {
			return err
		}
	}
	return nil
}
func safeFile(root, path string, size int64, hash string) ([]byte, error) {
	if !inside(root, path) || size < 0 || size > releasecontract.MaxFileBytes {
		return nil, fmt.Errorf("unsafe provider file")
	}
	parents := map[string]os.FileInfo{}
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		if !inside(root, dir) {
			return nil, fmt.Errorf("provider parent escaped root")
		}
		info, e := os.Lstat(dir)
		resolved, e2 := filepath.EvalSymlinks(dir)
		if e != nil || e2 != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !strings.EqualFold(resolved, dir) {
			return nil, fmt.Errorf("provider file parent linked")
		}
		parents[dir] = info
		if strings.EqualFold(dir, root) {
			break
		}
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() != size {
		return nil, fmt.Errorf("provider file unavailable or changed")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) || !singleLink(f) {
		_ = f.Close()
		return nil, fmt.Errorf("provider file identity invalid")
	}
	b, readErr := io.ReadAll(io.LimitReader(f, size+1))
	after, e2 := f.Stat()
	_ = f.Close()
	last, e3 := os.Lstat(path)
	if readErr != nil || e2 != nil || e3 != nil || int64(len(b)) != size || hashBytes(b) != hash || !os.SameFile(opened, last) || !after.ModTime().Equal(before.ModTime()) || last.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("provider file changed")
	}
	for dir, info := range parents {
		current, e := os.Lstat(dir)
		resolved, e2 := filepath.EvalSymlinks(dir)
		if e != nil || e2 != nil || !os.SameFile(info, current) || current.Mode()&os.ModeSymlink != 0 || !strings.EqualFold(resolved, dir) {
			return nil, fmt.Errorf("provider directory changed")
		}
	}
	return b, nil
}
func verifyRevision(root string, m releasecontract.ArtifactManifest) error {
	if _, _, err := releasecontract.CanonicalManifest(m); err != nil {
		return err
	}
	expected := map[string]bool{}
	for _, file := range m.Files {
		expected[file.Path] = true
		if _, err := safeFile(root, filepath.Join(root, filepath.FromSlash(file.Path)), file.SizeBytes, file.SHA256); err != nil {
			return err
		}
	}
	count := 0
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		count++
		if count > 4096 || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("provider revision linked or oversized")
		}
		if !entry.IsDir() {
			rel, _ := filepath.Rel(root, path)
			if !expected[filepath.ToSlash(rel)] {
				return fmt.Errorf("provider revision contains extra file")
			}
		}
		return nil
	})
}
