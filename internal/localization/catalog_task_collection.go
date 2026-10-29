package localization

var taskCollectionCatalog = []Definition{
	{Key: "task_collection.group_key", SourceTemplate: "A task group has an invalid key", Texts: map[Locale]string{ZhCN: "任务分组的源字段无效，请检查源记录后重新加载", EnUS: "A task group has an invalid key. Check the source record and reload."}},
	{Key: "task_collection.query", SourceTemplate: "Invalid project task collection query or cursor", Texts: map[Locale]string{ZhCN: "项目任务筛选或分页依据无效，请核对条件并重新加载第一页", EnUS: "The project task filters or cursor are invalid. Check the filters and reload the first page."}},
	{Key: "task_collection.timestamp", SourceTemplate: "A task has an invalid UTC timestamp", Texts: map[Locale]string{ZhCN: "任务源数据的UTC时间无效，请检查源记录；系统未改写或隐藏该记录", EnUS: "A task has an invalid UTC timestamp. Check the source record; it has not been rewritten or hidden."}},
}
