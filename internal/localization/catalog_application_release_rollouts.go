package localization

// Exact rollout diagnostics do not translate user text or grant authority.
var applicationReleaseRolloutCatalog = []Definition{
	{Key: "release_rollout.invalid", SourceTemplate: "Invalid application release rollout request", Texts: map[Locale]string{ZhCN: "应用发布推进请求无效", EnUS: "Invalid application release rollout request"}},
	{Key: "release_rollout.changed", SourceTemplate: "The application release rollout basis has changed", Texts: map[Locale]string{ZhCN: "应用发布推进依据已变化，请重新预览后决定", EnUS: "The application release rollout basis has changed"}},
	{Key: "release_rollout.blocked", SourceTemplate: "The application release rollout is blocked", Texts: map[Locale]string{ZhCN: "应用发布推进仍有阻断项，请核对原计划或操作", EnUS: "The application release rollout is blocked"}},
	{Key: "dag.subworkflow.service_binding_invalid", SourceTemplate: "invalid: invalid service-bound subworkflow", Texts: map[Locale]string{ZhCN: "受保护子执行的固定绑定无效", EnUS: "invalid: invalid service-bound subworkflow"}},
	{Key: "dag.subworkflow.service_binding_unavailable", SourceTemplate: "invalid: service-bound subworkflow handler unavailable", Texts: map[Locale]string{ZhCN: "受保护子执行服务不可用", EnUS: "invalid: service-bound subworkflow handler unavailable"}},
}
