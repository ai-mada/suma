package localization

// These aliases belong only to exact, host-owned diagnostics. Case messages,
// report XML, titles and human review reasons are original project content.
var testPlanCatalog = []Definition{
	{Key: "test_plan.invalid", SourceTemplate: "Invalid test plan request", Texts: map[Locale]string{ZhCN: "测试计划请求无效", EnUS: "Invalid test plan request"}},
	{Key: "test_plan.version", SourceTemplate: "The test plan or selected task has changed; reload before deciding", Texts: map[Locale]string{ZhCN: "测试计划或所选任务已变化，请读取最新版本后再决定", EnUS: "The test plan or selected task has changed; reload before deciding"}},
	{Key: "test_plan.source", SourceTemplate: "The test plan source has changed; review and save a current draft", Texts: map[Locale]string{ZhCN: "测试计划依据已变化，请核对并保存当前草稿", EnUS: "The test plan source has changed; review and save a current draft"}},
	{Key: "test_plan.incomplete", SourceTemplate: "Complete the test cases and task acceptance coverage before publication", Texts: map[Locale]string{ZhCN: "请补齐测试用例和任务验收标准覆盖后再发布", EnUS: "Complete the test cases and task acceptance coverage before publication"}},
	{Key: "test_plan.gate", SourceTemplate: "The current test plan does not have complete passing evidence for this run", Texts: map[Locale]string{ZhCN: "当前测试计划尚无覆盖本次运行的完整通过证据", EnUS: "The current test plan does not have complete passing evidence for this run"}},
	{Key: "test_plan.binding", SourceTemplate: "Published test plans require fixed report-capable test nodes without dynamic or child workflows", Texts: map[Locale]string{ZhCN: "已发布测试计划需要固定且支持报告的测试节点，暂不支持动态或子流程", EnUS: "Published test plans require fixed report-capable test nodes without dynamic or child workflows"}},
	{Key: "test_plan.defect", SourceTemplate: "Only a current known failed or error test case can create a defect", Texts: map[Locale]string{ZhCN: "只有当前已确认失败或异常的测试用例可以创建缺陷", EnUS: "Only a current known failed or error test case can create a defect"}},
	{Key: "test_plan.dispatch_changed", SourceTemplate: "The published test plan changed; preview dispatch again", Texts: map[Locale]string{ZhCN: "已发布测试计划已变化，请重新预览派发", EnUS: "The published test plan changed; preview dispatch again"}},
	{Key: "test_plan.revision_inconsistent", SourceTemplate: "test plan revision is inconsistent", Texts: map[Locale]string{ZhCN: "测试计划修订数据不一致，请核对存储记录", EnUS: "The test plan revision is inconsistent; inspect the stored record."}},
	{Key: "test_plan.run_inconsistent", SourceTemplate: "test plan run snapshot is inconsistent", Texts: map[Locale]string{ZhCN: "测试计划的运行快照不一致，请核对存储记录", EnUS: "The test plan run snapshot is inconsistent; inspect the stored record."}},
	{Key: "test_plan.report_inconsistent", SourceTemplate: "test plan report is inconsistent", Texts: map[Locale]string{ZhCN: "测试计划报告数据不一致，请核对存储记录", EnUS: "The test plan report is inconsistent; inspect the stored record."}},
	{Key: "test_plan.report.invalid", SourceTemplate: "Invalid or unsupported JUnit test report", Texts: map[Locale]string{ZhCN: "JUnit 测试报告无效或含有不支持的内容", EnUS: "Invalid or unsupported JUnit test report"}},
	{Key: "test_plan.report.digest", SourceTemplate: "JUnit report protocol or original hash is invalid", Texts: map[Locale]string{ZhCN: "JUnit 报告协议或原件摘要无效", EnUS: "JUnit report protocol or original hash is invalid"}},
	{Key: "test_plan.report.capability", SourceTemplate: "invalid isolated test report capability", Texts: map[Locale]string{ZhCN: "隔离测试报告能力声明无效", EnUS: "The isolated test report capability is invalid."}},
	{Key: "test_plan.runner.configuration", SourceTemplate: "test reports require an isolated administrator-designated command test", Texts: map[Locale]string{ZhCN: "测试报告需要管理员指定的隔离命令测试", EnUS: "Test reports require an isolated administrator-designated command test."}},
	{Key: "test_plan.runner.path", SourceTemplate: "test report paths are assigned by the Runner, not command configuration", Texts: map[Locale]string{ZhCN: "测试报告路径由执行节点分配，不能在命令配置中指定", EnUS: "Test report paths are assigned by the Runner, not command configuration."}},
	{Key: "test_plan.runner.fresh_directory", SourceTemplate: "test report directory is not fresh or is unavailable", Texts: map[Locale]string{ZhCN: "测试报告目录已存在或不可用，请核对原执行", EnUS: "The test report directory is not fresh or is unavailable."}},
	{Key: "test_plan.runner.capture", SourceTemplate: "test report is missing, invalid or unsafe", Texts: map[Locale]string{ZhCN: "测试报告缺失、格式无效或文件不安全", EnUS: "The test report is missing, invalid or unsafe."}},
	{Key: "test_plan.runner.secret", SourceTemplate: "test report contains a configured secret and was retained locally", Texts: map[Locale]string{ZhCN: "测试报告含有已配置的敏感值，已保留在本地且未上传", EnUS: "The test report contains a configured secret and was retained locally."}},
	{Key: "test_plan.runner.capability", SourceTemplate: "test report capability does not match local configuration", Texts: map[Locale]string{ZhCN: "测试报告能力与本地配置不符", EnUS: "The test report capability does not match the local configuration."}},
	{Key: "test_plan.runner.placeholder", SourceTemplate: "test report path placeholders require an enabled report profile", Texts: map[Locale]string{ZhCN: "报告路径占位符需要启用测试报告工具配置", EnUS: "Test report path placeholders require an enabled report profile."}},
	{Key: "test_plan.runner.failed_cases", SourceTemplate: "test report includes failed or error cases", Texts: map[Locale]string{ZhCN: "测试报告包含失败或异常用例", EnUS: "The test report includes failed or error cases."}},
}
