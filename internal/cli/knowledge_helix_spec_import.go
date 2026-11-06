package cli

import (
	"bytes"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	knowledgeHelixSpecImportUsage = "knowledge import-helix-spec requires one --file spec.md and accepts an optional global --project"
	knowledgeHelixSpecInvalid     = "choose a nonempty UTF-8 spec.md"
	knowledgeHelixSpecTooLarge    = "the specification file is limited to 32 KiB and the draft content to 16,000 bytes"
	knowledgeHelixSpecReadFailed  = "the local specification file could not be read"
	knowledgeHelixSpecCredential  = "the specification draft input contains the current credential. Remove the credential before generating a preview"
)

func knowledgeHelixSpecImportCommand(c *client, args []string, out io.Writer) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		_, err := io.WriteString(out, c.text(
			"  suma [--project ID] knowledge import-helix-spec --file spec.md\n\n离线读取一个本地 HelixHarness 规范并输出草稿 JSON。指定 --project 可固定目标项目。\n复核 JSON 后，使用 knowledge create --file draft.json 保存草稿。\n",
			"  suma [--project ID] knowledge import-helix-spec --file spec.md\n\nRead one local HelixHarness specification offline and print draft JSON. Set --project to pin the target project.\nReview the JSON and save it with knowledge create --file draft.json.\n",
		))
		return err
	}
	file, err := knowledgeImportFile(c, args, "import-helix-spec", knowledgeHelixSpecImportUsage)
	if err != nil {
		return err
	}
	raw, err := readKnowledgeMarkdown(file, "spec.md", knowledgeHelixSpecInvalid, knowledgeHelixSpecTooLarge, knowledgeHelixSpecReadFailed)
	if err != nil {
		return err
	}
	if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		return usage(knowledgeHelixSpecInvalid)
	}
	content := strings.ReplaceAll(strings.TrimPrefix(string(raw), "\uFEFF"), "\r\n", "\n")
	if strings.TrimSpace(content) == "" {
		return usage(knowledgeHelixSpecInvalid)
	}
	if len(content) > 16000 {
		return usage(knowledgeHelixSpecTooLarge)
	}
	return writeKnowledgeDraftPreview(c, out, "spec", helixSpecTitle(content), "", content, knowledgeHelixSpecCredential)
}

func helixSpecTitle(content string) string {
	for line := range strings.SplitSeq(content, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "#\t") {
			title := strings.TrimSpace(line[2:])
			if title != "" && len(title) <= 200 {
				return title
			}
		}
		break
	}
	return "HelixHarness spec"
}
