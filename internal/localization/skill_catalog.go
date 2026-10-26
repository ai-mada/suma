package localization

// Display messages only. Shared skill text and immutable provenance are user
// data and remain unchanged when the interface language changes.
var skillCatalog = []Definition{
	{Key: "skill_catalog.invalid", SourceTemplate: "Invalid shared skill catalog request", Texts: map[Locale]string{ZhCN: "共享技能目录请求无效", EnUS: "Invalid shared skill catalog request"}},
	{Key: "skill_catalog.version", SourceTemplate: "The shared skill has changed; reload its reviewed version", Texts: map[Locale]string{ZhCN: "共享技能已变化，请重新读取审核版本", EnUS: "The shared skill has changed; reload its reviewed version"}},
	{Key: "skill_catalog.source", SourceTemplate: "The source skill is no longer the current eligible publication", Texts: map[Locale]string{ZhCN: "源技能已不是当前可采用的发布版本", EnUS: "The source skill is no longer the current eligible publication"}},
	{Key: "skill_catalog.sharing", SourceTemplate: "Confirm that this reviewed skill text will be shared with the organization", Texts: map[Locale]string{ZhCN: "请确认将审核后的技能正文共享给当前组织", EnUS: "Confirm that this reviewed skill text will be shared with the organization"}},
	{Key: "skill_catalog.adopting", SourceTemplate: "Confirm adoption as a project draft requiring a separate publication review", Texts: map[Locale]string{ZhCN: "请确认采用为项目草稿，之后仍需单独审核发布", EnUS: "Confirm adoption as a project draft requiring a separate publication review"}},
	{Key: "skill_catalog.retired", SourceTemplate: "This shared skill is retired and cannot be adopted", Texts: map[Locale]string{ZhCN: "此共享技能已退役，不能新采用", EnUS: "This shared skill is retired and cannot be adopted"}},
	{Key: "skill_catalog.origin", SourceTemplate: "A shared skill cannot change its original project or knowledge identity", Texts: map[Locale]string{ZhCN: "共享技能不能更换原项目或知识记录", EnUS: "A shared skill cannot change its original project or knowledge identity"}},
	{Key: "application_tool_project_skill_catalog_list", Texts: map[Locale]string{ZhCN: "检索当前授权组织的共享技能目录，不安装或启用技能", EnUS: "Search the authorized organization's shared skills without installing or enabling them"}},
	{Key: "application_tool_project_skill_catalog_get", Texts: map[Locale]string{ZhCN: "读取共享技能的明确版本和摘要，原来源原件与权限不随文本共享", EnUS: "Read an exact shared skill version and digest without sharing original source files or permissions"}},
	{Key: "skill_catalog.tool_version", Texts: map[Locale]string{ZhCN: "共享技能版本已变化，请重新检索并核对", EnUS: "The shared skill version has changed. Search again and review the version."}},
}
