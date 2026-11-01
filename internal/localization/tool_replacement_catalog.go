package localization

var toolReplacementCatalog = []Definition{
	{Key: "execution.tool_profile.replacement_snapshot_limit", Texts: map[Locale]string{ZhCN: "配置快照超过2MiB限制，未提交换新", EnUS: "The configuration snapshots exceed 2 MiB. No replacement was committed."}},
	{Key: "validation.tool_profile_replacement", Texts: map[Locale]string{ZhCN: "工具配置换新请求或分页参数无效", EnUS: "The tool configuration replacement request or pagination parameters are invalid."}},
	{Key: "execution.tool_profile.replacement_changed", Texts: map[Locale]string{ZhCN: "工具配置换新依据已变化，请重新核对当前配置", EnUS: "The tool configuration replacement basis has changed. Review the current configurations."}},
	{Key: "execution.tool_profile.replacement_inventory", Texts: map[Locale]string{ZhCN: "目标节点尚未报告新鲜且完整可用的匹配配置", EnUS: "The target worker has not reported a fresh, complete and available matching configuration."}},
}
