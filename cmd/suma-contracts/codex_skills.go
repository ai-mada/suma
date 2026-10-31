package main

func enhanceCodexSkillSchemas(schemas map[string]any) {
	for _, name := range []string{"ExecutionSkillDocument", "ExecutionSkillBundle", "ExecutionSkillReceipt", "ExecutionSkillEvidence"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	for _, name := range []string{"ExecutionSkillDocument", "ExecutionSkillReceipt"} {
		properties := toolContractMap(schemas, name, "properties")
		properties["revision"].(map[string]any)["minimum"] = 1
		properties["revision"].(map[string]any)["maximum"] = 9007199254740991
		properties["name"].(map[string]any)["pattern"] = "^suma-[a-f0-9]{24}$"
		for _, field := range []string{"sha256", "document_sha256"} {
			properties[field].(map[string]any)["pattern"] = "^[a-f0-9]{64}$"
		}
	}
	document := toolContractMap(schemas, "ExecutionSkillDocument", "properties")
	document["content_sha256"].(map[string]any)["pattern"] = "^[a-f0-9]{64}$"
	for field, maximum := range map[string]int{"title": 200, "description": 1000, "content": 16000} {
		document[field].(map[string]any)["minLength"] = 1
		document[field].(map[string]any)["maxLength"] = maximum
		document[field].(map[string]any)["description"] = "The service enforces this bound in UTF-8 bytes; no truncation."
	}
	for _, name := range []string{"ExecutionSkillBundle", "ExecutionSkillEvidence"} {
		properties := toolContractMap(schemas, name, "properties")
		properties["bundle_sha256"].(map[string]any)["pattern"] = "^[a-f0-9]{64}$"
		properties["documents"].(map[string]any)["minItems"] = 1
		properties["documents"].(map[string]any)["maxItems"] = 16
	}
	toolContractMap(schemas, "ExecutionSkillBundle", "properties", "protocol")["enum"] = []string{"suma-codex-skills-v1"}
	toolContractMap(schemas, "ExecutionSkillEvidence")["description"] = "Independent host materialization, CLI catalogue, invocation request, turn ACK and final byte verification. None proves model reading, skill effectiveness, independent tests or human acceptance."
}
