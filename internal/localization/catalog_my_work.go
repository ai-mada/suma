package localization

var myWorkCatalog = []Definition{
	{Key: "my_work.query", SourceTemplate: "Invalid personal work query or cursor", Texts: map[Locale]string{ZhCN: "个人任务查询或分页依据无效，请核对筛选并重新加载第一页", EnUS: "The personal task query or cursor is invalid. Check the filters and reload the first page."}},
	{Key: "my_work.method", SourceTemplate: "Method not allowed", Texts: map[Locale]string{ZhCN: "当前个人任务接口不支持此请求方式", EnUS: "This request method is not supported by the personal task endpoint."}},
}
