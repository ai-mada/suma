package localization

// Exact application/source messages only. Original user text and immutable
// source evidence never become generic English aliases.
var projectApplicationCatalog = []Definition{
	{Key: "application.invalid", SourceTemplate: "Invalid project application request", Texts: map[Locale]string{ZhCN: "应用请求无效", EnUS: "Invalid project application request"}},
	{Key: "application.changed", SourceTemplate: "The application or reviewed code version has changed; reload before deciding", Texts: map[Locale]string{ZhCN: "应用或已审阅代码版本已变化，请重新读取后决定", EnUS: "The application or reviewed code version has changed; reload before deciding"}},
	{Key: "application.scope", SourceTemplate: "Application code scopes require current governed targets and explicit shared ownership", Texts: map[Locale]string{ZhCN: "代码范围需要当前受控仓库目标，并明确声明共享归属", EnUS: "Application code scopes require current governed targets and explicit shared ownership"}},
	{Key: "application.team", SourceTemplate: "The application owner must be a current project-qualified team", Texts: map[Locale]string{ZhCN: "应用责任团队必须具有当前项目资格", EnUS: "The application owner must be a current project-qualified team"}},
	{Key: "application.source", SourceTemplate: "The application source requires an applied repository receipt and current independent accepted evidence", Texts: map[Locale]string{ZhCN: "应用来源需要真实已应用仓库回执及当前独立测试验收依据", EnUS: "The application source requires an applied repository receipt and current independent accepted evidence"}},
	{Key: "application.inspection", SourceTemplate: "The fixed source inspection is incomplete or does not match the reviewed commit", Texts: map[Locale]string{ZhCN: "固定代码来源检查不完整，或与已审阅提交不符", EnUS: "The fixed source inspection is incomplete or does not match the reviewed commit"}},
	{Key: "application.dependencies", SourceTemplate: "Unresolved or partial dependency observations require explicit acknowledgement", Texts: map[Locale]string{ZhCN: "依赖观察有未解析或不完整范围，需要明确确认", EnUS: "Unresolved or partial dependency observations require explicit acknowledgement"}},
	{Key: "application.source.unavailable", SourceTemplate: "Application source inspection is unavailable", Texts: map[Locale]string{ZhCN: "源码检查不可用，请配置独立读取权限及受信解析器", EnUS: "Application source inspection is unavailable"}},
	{Key: "application.source.program_changed", SourceTemplate: "The configured application source inspector changed", Texts: map[Locale]string{ZhCN: "受信源码解析器已变化，请重新核对部署配置", EnUS: "The configured application source inspector changed"}},
	{Key: "application.source.invalid_report", SourceTemplate: "The application source report failed its fixed contract", Texts: map[Locale]string{ZhCN: "源码报告不符合固定来源合同", EnUS: "The application source report failed its fixed contract"}},
	{Key: "application.source.limit", SourceTemplate: "The application source exceeds the inspection allowance", Texts: map[Locale]string{ZhCN: "源码范围超过本次检查上限，请缩小范围", EnUS: "The application source exceeds the inspection allowance"}},
}
