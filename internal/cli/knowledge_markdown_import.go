package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxKnowledgeMarkdownBytes = 32 * 1024

func knowledgeImportFile(c *client, args []string, command, usageMessage string) (string, error) {
	var file string
	fs := commandFlags("knowledge " + command)
	fs.Func("file", "one local Markdown file", func(value string) error {
		if file != "" || value == "" || value == "-" {
			return usage(usageMessage)
		}
		file = value
		return nil
	})
	if fs.Parse(args) != nil || fs.NArg() != 0 || file == "" ||
		(c.project != "" && !compositionCLIIdentifier(c.project, 64)) {
		return "", usage(usageMessage)
	}
	return file, nil
}

func readKnowledgeMarkdown(path, filename, invalid, tooLarge, readFailed string) ([]byte, error) {
	base := filepath.Base(path)
	if len(base) != len(filename) || !strings.EqualFold(base, filename) || strings.Contains(path, "://") ||
		strings.HasPrefix(strings.ReplaceAll(path, `\`, "/"), "//") {
		return nil, usage(invalid)
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return nil, usage(readFailed)
	}
	if before.Size() > maxKnowledgeMarkdownBytes {
		return nil, usage(tooLarge)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, usage(readFailed)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return nil, usage(readFailed)
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxKnowledgeMarkdownBytes+1))
	if err != nil {
		return nil, usage(readFailed)
	}
	if len(raw) > maxKnowledgeMarkdownBytes {
		return nil, usage(tooLarge)
	}
	return raw, nil
}

func writeKnowledgeDraftPreview(c *client, out io.Writer, kind, title, summary, content, credentialMessage string) error {
	// Reject credentials rather than silently redacting a create request.
	// Short placeholder tokens only match entire fields.
	if c.token != "" {
		for _, value := range []string{title, summary, content, c.project} {
			if strings.TrimSpace(value) == c.token || (len(c.token) >= 8 && strings.Contains(value, c.token)) {
				return usage(credentialMessage)
			}
		}
	}
	body := map[string]any{
		"kind": kind, "title": title, "summary": summary, "content": content,
		"sources": []any{}, "expected_version": 0,
	}
	if c.project != "" {
		body["project_id"] = c.project
	}
	return writeJSON(out, body, "")
}
