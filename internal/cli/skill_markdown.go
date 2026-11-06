package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

const (
	knowledgeSkillInvalid    = "choose a UTF-8 SKILL.md with valid YAML, a skill name, a description and Markdown instructions"
	knowledgeSkillTooLarge   = "the skill file, description or instructions exceed the project draft limits"
	knowledgeSkillReadFailed = "the local skill file could not be read"
	maxSkillMarkdownBytes    = 32 * 1024
)

type skillMarkdown struct {
	Title, Summary, Content string
}

var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func readSkillMarkdown(path string) (skillMarkdown, error) {
	base := filepath.Base(path)
	if len(base) != len("SKILL.md") || !strings.EqualFold(base, "SKILL.md") || strings.Contains(path, "://") ||
		strings.HasPrefix(strings.ReplaceAll(path, `\`, "/"), "//") {
		return skillMarkdown{}, usage(knowledgeSkillInvalid)
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return skillMarkdown{}, usage(knowledgeSkillReadFailed)
	}
	if before.Size() > maxSkillMarkdownBytes {
		return skillMarkdown{}, usage(knowledgeSkillTooLarge)
	}
	file, err := os.Open(path)
	if err != nil {
		return skillMarkdown{}, usage(knowledgeSkillReadFailed)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return skillMarkdown{}, usage(knowledgeSkillReadFailed)
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxSkillMarkdownBytes+1))
	if err != nil {
		return skillMarkdown{}, usage(knowledgeSkillReadFailed)
	}
	if len(raw) > maxSkillMarkdownBytes {
		return skillMarkdown{}, usage(knowledgeSkillTooLarge)
	}
	return parseSkillMarkdown(raw)
}

func parseSkillMarkdown(raw []byte) (skillMarkdown, error) {
	invalid := usage(knowledgeSkillInvalid)
	if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		return skillMarkdown{}, invalid
	}
	text := strings.TrimPrefix(string(raw), "\uFEFF")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	boundary := func(line string) bool { return strings.TrimRight(line, " \t") == "---" }
	if !boundary(lines[0]) {
		return skillMarkdown{}, invalid
	}
	end := 1
	for end < len(lines) && !boundary(lines[end]) {
		end++
	}
	if end < 2 || end == len(lines) {
		return skillMarkdown{}, invalid
	}
	var document, extra yaml.Node
	decoder := yaml.NewDecoder(strings.NewReader(strings.Join(lines[1:end], "\n")))
	if decoder.Decode(&document) != nil || decoder.Decode(&extra) != io.EOF || len(document.Content) != 1 {
		return skillMarkdown{}, invalid
	}
	mapping := document.Content[0]
	if mapping.Kind != yaml.MappingNode || !validSkillMetadata(mapping) {
		return skillMarkdown{}, invalid
	}

	draft := skillMarkdown{Content: strings.Join(lines[end+1:], "\n")}
	for i := 0; i < len(mapping.Content); i += 2 {
		switch mapping.Content[i].Value {
		case "name":
			draft.Title, _ = skillYAMLString(mapping.Content[i+1])
		case "description":
			draft.Summary, _ = skillYAMLString(mapping.Content[i+1])
		}
	}
	if len(draft.Title) > 64 || !skillNamePattern.MatchString(draft.Title) ||
		!utf8.ValidString(draft.Summary) || strings.ContainsRune(draft.Summary, 0) ||
		strings.TrimSpace(draft.Summary) == "" || strings.TrimSpace(draft.Content) == "" {
		return skillMarkdown{}, invalid
	}
	if len(draft.Summary) > 1000 || len(draft.Content) > 16000 {
		return skillMarkdown{}, usage(knowledgeSkillTooLarge)
	}
	return draft, nil
}

// Validate the AST directly. No alias is expanded and only the skill's two
// metadata fields are copied into the resulting project draft.
func validSkillMetadata(node *yaml.Node) bool {
	if node.Kind == yaml.AliasNode {
		return false
	}
	if node.Kind == yaml.MappingNode {
		keys := make(map[string]bool)
		for i := 0; i < len(node.Content); i += 2 {
			key, ok := skillYAMLString(node.Content[i])
			if !ok || keys[key] {
				return false
			}
			keys[key] = true
		}
	}
	for _, child := range node.Content {
		if !validSkillMetadata(child) {
			return false
		}
	}
	return true
}

var skillYAMLCoreNonString = regexp.MustCompile(`^(?:~|[Nn]ull|NULL|[Tt]rue|TRUE|[Ff]alse|FALSE|0o[0-7]+|0x[0-9a-fA-F]+|[-+]?(\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][-+]?[0-9]+)?|[-+]?\.(inf|Inf|INF)|\.(nan|NaN|NAN))$`)

func skillYAMLString(node *yaml.Node) (string, bool) {
	if node.Kind != yaml.ScalarNode {
		return "", false
	}
	if node.Style != 0 {
		if node.ShortTag() != "!!str" {
			return "", false
		}
		return node.Value, true
	}
	// Match YAML 1.2 core for implicit scalars. The Go resolver also accepts
	// timestamps and legacy numeric syntax that the workbench treats as text.
	if node.Value == "" || skillYAMLCoreNonString.MatchString(node.Value) {
		return "", false
	}
	return node.Value, true
}
