package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	cc "github.com/ai-mada/suma/internal/codexcontract"
)

func codexExecutionSkillsRequest(a Assignment, binding *cc.Binding) (*cc.ExecutionSkillBundle, error) {
	configured, present := a.Config["execution_skills"]
	if a.ExecutionSkills == nil {
		if present {
			return nil, fmt.Errorf("Codex skills require a protected assignment bundle")
		}
		return nil, nil
	}
	if binding == nil || a.Type != "agent" || !present || configured == nil {
		return nil, fmt.Errorf("Codex skills require the fixed Codex agent assignment")
	}
	// Decode both sides into independent strict values. Workflow text and a
	// caller-owned mutable slice cannot become the authority for mounted bytes.
	protected, err := cc.DecodeExecutionSkillBundle(a.ExecutionSkills)
	if err != nil {
		return nil, fmt.Errorf("Codex assignment skill bundle is invalid")
	}
	fromConfig, err := cc.DecodeExecutionSkillBundle(configured)
	if err != nil || fromConfig == nil {
		return nil, fmt.Errorf("Codex skill configuration is invalid")
	}
	want, _ := json.Marshal(protected)
	got, _ := json.Marshal(fromConfig)
	if !bytes.Equal(want, got) {
		return nil, fmt.Errorf("Codex skills differ from the protected assignment")
	}
	return protected, nil
}

func codexSkillMountError() error {
	return fmt.Errorf("Codex isolated skill files are unavailable or changed")
}

// The already-created home belongs to this attempt, outside its Git source.
// Refuse linked ancestors, then bind the directory handle to the same entry.
// Operations below use os.Root handles rather than untrusted concatenated paths.
func openCodexSkillHome(home string) (*os.Root, error) {
	if !filepath.IsAbs(home) || filepath.Clean(home) != home {
		return nil, codexSkillMountError()
	}
	var expected os.FileInfo
	for p := home; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, codexSkillMountError()
		}
		if p == home {
			expected = info
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return nil, codexSkillMountError()
	}
	opened, err := root.Stat(".")
	after, afterErr := os.Lstat(home)
	if err != nil || afterErr != nil || after.Mode()&os.ModeSymlink != 0 || !after.IsDir() || !os.SameFile(expected, opened) || !os.SameFile(expected, after) {
		root.Close()
		return nil, codexSkillMountError()
	}
	return root, nil
}

func openCodexSkillDirectory(parent *os.Root, name string, create bool) (*os.Root, error) {
	before, err := parent.Lstat(name)
	if errors.Is(err, os.ErrNotExist) && create {
		if err = parent.Mkdir(name, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, codexSkillMountError()
		}
		before, err = parent.Lstat(name)
	}
	if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
		return nil, codexSkillMountError()
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, codexSkillMountError()
	}
	opened, err := child.Stat(".")
	after, afterErr := parent.Lstat(name)
	if err != nil || afterErr != nil || after.Mode()&os.ModeSymlink != 0 || !after.IsDir() || !os.SameFile(before, opened) || !os.SameFile(before, after) {
		child.Close()
		return nil, codexSkillMountError()
	}
	return child, nil
}

func readExactCodexSkill(directory *os.Root, want []byte) error {
	before, err := directory.Lstat("SKILL.md")
	if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() || before.Size() != int64(len(want)) {
		return codexSkillMountError()
	}
	file, err := directory.Open("SKILL.md")
	if err != nil {
		return codexSkillMountError()
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return codexSkillMountError()
	}
	raw, err := io.ReadAll(io.LimitReader(file, int64(len(want))+1))
	after, afterErr := directory.Lstat("SKILL.md")
	if err != nil || afterErr != nil || after.Mode()&os.ModeSymlink != 0 || !after.Mode().IsRegular() || after.Size() != int64(len(want)) || !os.SameFile(before, after) || !bytes.Equal(raw, want) {
		return codexSkillMountError()
	}
	return nil
}

func createExactCodexSkill(directory *os.Root, want []byte) error {
	file, err := directory.OpenFile("SKILL.md", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0400)
	if errors.Is(err, os.ErrExist) {
		return readExactCodexSkill(directory, want)
	}
	if err != nil {
		return codexSkillMountError()
	}
	// Never replace a mismatched file. A failed partial creation remains an
	// inspectable failure, and cannot be adopted by a later call as complete.
	written, writeErr := file.Write(want)
	if writeErr == nil && written != len(want) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return codexSkillMountError()
	}
	return readExactCodexSkill(directory, want)
}

func codexSkillFiles(home string, bundle *cc.ExecutionSkillBundle, create bool) error {
	if bundle == nil {
		return nil
	}
	if cc.ValidateExecutionSkillBundle(*bundle) != nil {
		return codexSkillMountError()
	}
	root, err := openCodexSkillHome(home)
	if err != nil {
		return err
	}
	defer root.Close()
	skills, err := openCodexSkillDirectory(root, "skills", create)
	if err != nil {
		return err
	}
	defer skills.Close()
	for _, doc := range bundle.Documents {
		directory, err := openCodexSkillDirectory(skills, doc.Name, create)
		if err != nil {
			return err
		}
		want, err := cc.RenderExecutionSkill(doc)
		if err == nil {
			if create {
				err = createExactCodexSkill(directory, want)
			} else {
				err = readExactCodexSkill(directory, want)
			}
		}
		directory.Close()
		if err != nil {
			return codexSkillMountError()
		}
	}
	return nil
}

func mountCodexSkills(home string, bundle *cc.ExecutionSkillBundle) (*cc.ExecutionSkillEvidence, error) {
	if bundle == nil {
		return nil, nil
	}
	if cc.ValidateExecutionSkillBundle(*bundle) != nil {
		return nil, codexSkillMountError()
	}
	evidence := &cc.ExecutionSkillEvidence{BundleSHA256: bundle.BundleSHA256, Documents: cc.ExecutionSkillReceipts(*bundle)}
	if err := codexSkillFiles(home, bundle, true); err != nil {
		evidence.FailureCode = "materialization_failed"
		return evidence, err
	}
	evidence.Materialized = true
	return evidence, nil
}

// This verifies bytes at an observed boundary, not OS-enforced immutability or
// model use. Only normal process-tree completion supplies the final receipt flag.
func verifyCodexSkillFiles(home string, bundle *cc.ExecutionSkillBundle) error {
	return codexSkillFiles(home, bundle, false)
}
