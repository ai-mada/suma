package cli

import (
	"io"
	"strings"
)

const (
	knowledgeActionsUsage     = "knowledge requires list, search, show, history, import-skill, create, edit, extract, or command. See knowledge --help"
	knowledgeSkillImportUsage = "knowledge import-skill requires one --file SKILL.md and accepts an optional global --project"
	knowledgeSkillCredential  = "the skill draft input contains the current credential. Remove the credential before generating a preview"
)

func knowledgeSkillImportCommand(c *client, args []string, out io.Writer) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		_, err := io.WriteString(out, c.text(
			"  suma [--project ID] knowledge import-skill --file SKILL.md\n\n离线读取一个本地技能文件并输出草稿 JSON。指定 --project 可固定目标项目。\n复核 JSON 后，使用 knowledge create --file draft.json 保存草稿。\n",
			"  suma [--project ID] knowledge import-skill --file SKILL.md\n\nRead one local skill file offline and print draft JSON. Set --project to pin the target project.\nReview the JSON and save it with knowledge create --file draft.json.\n",
		))
		return err
	}

	var file string
	fs := commandFlags("knowledge import-skill")
	fs.Func("file", "one local SKILL.md", func(value string) error {
		if file != "" || value == "" || value == "-" {
			return usage(knowledgeSkillImportUsage)
		}
		file = value
		return nil
	})
	if fs.Parse(args) != nil || fs.NArg() != 0 || file == "" ||
		(c.project != "" && !compositionCLIIdentifier(c.project, 64)) {
		return usage(knowledgeSkillImportUsage)
	}

	skill, err := readSkillMarkdown(file)
	if err != nil {
		return err
	}
	// The output is a create request. Reject credentials rather than silently
	// redacting the draft. Short placeholder tokens only match entire fields.
	if c.token != "" {
		for _, value := range []string{skill.Title, skill.Summary, skill.Content, c.project} {
			if strings.TrimSpace(value) == c.token || (len(c.token) >= 8 && strings.Contains(value, c.token)) {
				return usage(knowledgeSkillCredential)
			}
		}
	}
	body := map[string]any{
		"kind": "skill", "title": skill.Title, "summary": skill.Summary, "content": skill.Content,
		"sources": []any{}, "expected_version": 0,
	}
	if c.project != "" {
		body["project_id"] = c.project
	}
	return writeJSON(out, body, "")
}
