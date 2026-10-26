package localization

// Only platform-owned failure-diagnosis diagnostics. Evidence, model output,
// hypotheses and user-edited repair text always keep their original language.
var failureDiagnosisCatalog = []Definition{
	{Key: "failure_diagnosis.invalid", SourceTemplate: "Invalid failure diagnosis request", Texts: map[Locale]string{ZhCN: "失败分诊请求无效", EnUS: "Invalid failure diagnosis request"}},
	{Key: "failure_diagnosis.source", SourceTemplate: "Failure evidence has changed; create a new diagnosis", Texts: map[Locale]string{ZhCN: "失败依据已变化，请重新分析", EnUS: "Failure evidence has changed; create a new diagnosis"}},
	{Key: "failure_diagnosis.uncertain", SourceTemplate: "A current known failed node is required; reconcile the original run first", Texts: map[Locale]string{ZhCN: "需选择当前已知失败的节点；请先核对原运行", EnUS: "A current known failed node is required; reconcile the original run first"}},
	{Key: "failure_diagnosis.issue", SourceTemplate: "The failure issue is not yet available; wait for issue projection", Texts: map[Locale]string{ZhCN: "故障问题尚未生成，请等待持久问题回流", EnUS: "The failure issue is not yet available; wait for issue projection"}},
	{Key: "failure_diagnosis.profile", SourceTemplate: "Select a current enabled read-only Suma failure diagnosis profile", Texts: map[Locale]string{ZhCN: "请选择当前已启用的只读 Suma 失败分诊工具", EnUS: "Select a current enabled read-only Suma failure diagnosis profile"}},
	{Key: "failure_diagnosis.consent", SourceTemplate: "Confirm that the selected failure evidence will be sent to the configured model", Texts: map[Locale]string{ZhCN: "请确认将所选失败证据发送给此配置的模型", EnUS: "Confirm that the selected failure evidence will be sent to the configured model"}},
	{Key: "failure_diagnosis.output", SourceTemplate: "The model output does not satisfy the fixed failure diagnosis contract", Texts: map[Locale]string{ZhCN: "模型输出不符合固定的失败分诊格式", EnUS: "The model output does not satisfy the fixed failure diagnosis contract"}},
	{Key: "failure_diagnosis.version", SourceTemplate: "The failure diagnosis has changed; reload before deciding", Texts: map[Locale]string{ZhCN: "分诊记录已变化，请保留草稿并读取最新版本", EnUS: "The failure diagnosis has changed; reload before deciding"}},
	{Key: "failure_diagnosis.draft", SourceTemplate: "Confirm adoption as a draft repair requirement requiring normal delivery review", Texts: map[Locale]string{ZhCN: "请确认仅创建待正常交付评审的修复需求草稿", EnUS: "Confirm adoption as a draft repair requirement requiring normal delivery review"}},
	{Key: "failure_diagnosis.failed", SourceTemplate: "The failure diagnosis execution did not complete successfully", Texts: map[Locale]string{ZhCN: "失败分诊执行未成功完成", EnUS: "The failure diagnosis execution did not complete successfully"}},
	{Key: "failure_diagnosis.unknown", SourceTemplate: "The failure diagnosis outcome is unknown; inspect the original diagnosis run", Texts: map[Locale]string{ZhCN: "分诊结果未知，请核对原诊断运行", EnUS: "The failure diagnosis outcome is unknown; inspect the original diagnosis run"}},
	{Key: "failure_diagnosis.prompt", SourceTemplate: "Failure diagnosis context exceeds 32000 UTF-8 bytes", Texts: map[Locale]string{ZhCN: "失败分诊上下文超过 32000 个 UTF-8 字节，请缩小证据范围", EnUS: "Failure diagnosis context exceeds 32000 UTF-8 bytes"}},
	{Key: "failure_diagnosis.authority", SourceTemplate: "The diagnosis creator no longer has current execution authority", Texts: map[Locale]string{ZhCN: "分诊发起者当前已无执行权限", EnUS: "The diagnosis creator no longer has current execution authority"}},
	{Key: "failure_diagnosis.existing", SourceTemplate: "This issue already has a repair requirement; open the current requirement", Texts: map[Locale]string{ZhCN: "此问题已有修复需求，请打开现有需求", EnUS: "This issue already has a repair requirement; open the current requirement"}},
}
