package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSkillMarkdown(t *testing.T) {
	const base = "---\nname: release-checks\ndescription: Review a release candidate.\n---\n# Steps\n\n- Inspect the candidate.\n"
	for _, tc := range []struct {
		name, data, wantError string
	}{
		{"duplicate metadata key", "---\nname: release-checks\ndescription: A skill\nmetadata: {version: one, version: two}\n---\nBody", knowledgeSkillInvalid},
		{"alias in unused metadata", "---\nname: release-checks\ndescription: &description A skill\nmetadata: {tag: *description}\n---\nBody", knowledgeSkillInvalid},
		{"duplicate root key", strings.Replace(base, "name: release-checks", "name: release-checks\n'name': other", 1), knowledgeSkillInvalid},
		{"malformed YAML", strings.Replace(base, "name: release-checks", "name: [release-checks", 1), knowledgeSkillInvalid},
		{"numeric title", strings.Replace(base, "release-checks", "2026", 1), knowledgeSkillInvalid},
		{"invalid slug", strings.Replace(base, "release-checks", "release--checks", 1), knowledgeSkillInvalid},
		{"typed summary", strings.Replace(base, "Review a release candidate.", "!!int 12", 1), knowledgeSkillInvalid},
		{"missing summary", strings.Replace(base, "description: Review a release candidate.\n", "", 1), knowledgeSkillInvalid},
		{"empty body", strings.Split(base, "# Steps")[0] + " \n", knowledgeSkillInvalid},
		{"malformed UTF-8", base + "\xff", knowledgeSkillInvalid},
		{"NUL", base + "\x00", knowledgeSkillInvalid},
		{"NUL from scalar escape", strings.Replace(base, "Review a release candidate.", `"hidden\0detail"`, 1), knowledgeSkillInvalid},
		{"file limit", strings.Repeat("a", maxKnowledgeMarkdownBytes+1), knowledgeSkillTooLarge},
		{"summary UTF-8 limit", strings.Replace(base, "Review a release candidate.", strings.Repeat("审", 334), 1), knowledgeSkillTooLarge},
		{"body UTF-8 limit", strings.Split(base, "# Steps")[0] + strings.Repeat("审", 5334), knowledgeSkillTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readSkillMarkdown(writeSkillMarkdownFixture(t, tc.data))
			if err == nil || err.Error() != tc.wantError {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
		})
	}
	t.Run("preserve portable markdown and multiline summary", func(t *testing.T) {
		const content = "# Steps\n\n- Inspect the candidate.\n\n---\n"
		const input = "---\nname: release-checks\ndescription: >-\n  Review a release candidate.\n  发布前检查。\nmetadata: {version: 1}\nallowed-tools: Bash\n---\n" + content
		got, err := readSkillMarkdown(writeSkillMarkdownFixture(t, "\uFEFF"+strings.ReplaceAll(input, "\n", "\r\n")))
		want := skillMarkdown{Title: "release-checks", Summary: "Review a release candidate. 发布前检查。", Content: content}
		if err != nil || got != want {
			t.Fatalf("draft = %#v, error = %v, want %#v", got, err, want)
		}
	})
	t.Run("YAML 1.2 plain scalars", func(t *testing.T) {
		const input = "---\nname: 0b101\ndescription: 2026-11-06\n---\nSteps"
		got, err := readSkillMarkdown(writeSkillMarkdownFixture(t, input))
		want := skillMarkdown{Title: "0b101", Summary: "2026-11-06", Content: "Steps"}
		if err != nil || got != want {
			t.Fatalf("draft = %#v, error = %v, want %#v", got, err, want)
		}
	})
}

func TestReadSkillMarkdownRejectsUnsupportedPath(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, want string }{
		{directory, knowledgeSkillReadFailed},
		{filepath.Join(t.TempDir(), "SKILL.md"), knowledgeSkillReadFailed},
		{"instructions.md", knowledgeSkillInvalid},
		{"https://example.invalid/SKILL.md", knowledgeSkillInvalid},
		{`\\example.invalid\share\SKILL.md`, knowledgeSkillInvalid},
		{`/\example.invalid/share/SKILL.md`, knowledgeSkillInvalid},
	} {
		_, err := readSkillMarkdown(tc.path)
		if err == nil || err.Error() != tc.want {
			t.Errorf("error = %v, want %q", err, tc.want)
		}
	}
	t.Run("symlink", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "SKILL.md")
		if err := os.Symlink(writeSkillMarkdownFixture(t, "---\nname: fixture\ndescription: Valid skill\n---\nBody"), link); err != nil {
			t.Skipf("test environment cannot create symlinks: %v", err)
		}
		if _, err := readSkillMarkdown(link); err == nil || err.Error() != knowledgeSkillReadFailed {
			t.Fatalf("symlink error = %v, want %q", err, knowledgeSkillReadFailed)
		}
	})
}

func writeSkillMarkdownFixture(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Skill.MD")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
