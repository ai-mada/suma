package cli

import "io"

const (
	knowledgeActionsUsage     = "knowledge requires list, search, show, history, import-skill, import-helix-spec, create, edit, extract, or command. See knowledge --help"
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

	file, err := knowledgeImportFile(c, args, "import-skill", knowledgeSkillImportUsage)
	if err != nil {
		return err
	}
	skill, err := readSkillMarkdown(file)
	if err != nil {
		return err
	}
	return writeKnowledgeDraftPreview(c, out, "skill", skill.Title, skill.Summary, skill.Content, knowledgeSkillCredential)
}
