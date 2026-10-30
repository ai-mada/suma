package localization

var taskReviewRequestsCatalog = []Definition{
	{Key: "task_review_requests.invalid", SourceTemplate: "Invalid task review request or cursor", Texts: map[Locale]string{ZhCN: "任务审核请求或分页依据无效，请核对当前任务、版本及原操作标识", EnUS: "The task review request or cursor is invalid. Check the task, versions and original operation ID."}},
	{Key: "task_review_requests.changed", SourceTemplate: "The task review request has changed; reload before deciding", Texts: map[Locale]string{ZhCN: "审核请求已变化，请重新读取后明确操作", EnUS: "The review request changed. Reload it before deciding."}},
	{Key: "task_review_requests.binding", SourceTemplate: "The task or execution binding has changed; reload the current delivery", Texts: map[Locale]string{ZhCN: "任务或执行绑定已变化，请重新读取当前交付", EnUS: "The task or execution binding changed. Reload the current delivery."}},
	{Key: "task_review_requests.recipient", SourceTemplate: "Select a current active human with project review authority", Texts: map[Locale]string{ZhCN: "请选择当前活跃且有项目审核权限的规范人员", EnUS: "Select a current active person with project review authority."}},
	{Key: "task_review_requests.pending", SourceTemplate: "This task already has a pending review request; cancel it explicitly before requesting again", Texts: map[Locale]string{ZhCN: "该任务已有待处理审核请求，请明确取消后再重新请求", EnUS: "This task already has a pending review request. Cancel it explicitly before requesting again."}},
	{Key: "task_review_requests.permission", SourceTemplate: "Only the original requester or a current project administrator may cancel this request", Texts: map[Locale]string{ZhCN: "仅原发起人或当前项目管理员可在有写权时取消该请求", EnUS: "Only the original requester or a current project administrator with write access may cancel this request."}},
	{Key: "task_review_requests.source", SourceTemplate: "The task review request source is invalid", Texts: map[Locale]string{ZhCN: "审核请求来源数据无效，请检查准确任务、运行和正式验收记录", EnUS: "The review request source is invalid. Check the exact task, run and formal review records."}},
}
