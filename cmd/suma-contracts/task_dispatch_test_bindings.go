package main

// Explicit tester choices belong to the original task preview. Schemas are
// generated from the same Go types; these constraints document the native
// validation without inventing another API or execution authority.
func enhanceTaskDispatchTestBindingSchemas(schemas map[string]any) {
	for _, name := range []string{"TaskDispatchPreviewRequest", "TaskDispatchPlan"} {
		field := toolContractMap(schemas, name, "properties", "test_bindings")
		field["maxItems"] = 64
		field["description"] = "Explicit overrides for existing static tests whose candidate_from is the selected Agent. Omitted preserves published definitions without proving independence. Preview freezes exact current tool/Worker/command; original Start rechecks current authority, route and observation."
	}
	for _, name := range []string{"TaskDispatchTestSelection", "TaskDispatchTestBinding"} {
		schema := toolContractMap(schemas, name)
		schema["additionalProperties"] = false
		for _, fieldName := range []string{"node_id", "tool_profile_id"} {
			field := toolContractMap(schema, "properties", fieldName)
			field["minLength"] = 1
			field["maxLength"] = 150
			if fieldName == "tool_profile_id" {
				field["maxLength"] = 64
			}
		}
	}
	toolContractMap(schemas, "TaskDispatchTestSelection", "properties", "expected_tool_version")["minimum"] = 1
	toolContractMap(schemas, "TaskDispatchTestBinding", "properties", "tool_profile_version")["minimum"] = 1
	toolContractMap(schemas, "TaskDispatchTestBinding", "properties", "tool_profile_sha256")["pattern"] = "^[0-9a-f]{64}$"
}
