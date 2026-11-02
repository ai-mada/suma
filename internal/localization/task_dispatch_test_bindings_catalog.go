package localization

var taskDispatchTestBindingsCatalog = []Definition{
	{Key: "validation.task_dispatch_test_binding", SourceTemplate: "请选择所选开发节点关联的静态测试节点及准确工具版本，不得改变额外能力约束", Texts: map[Locale]string{ZhCN: "请选择所选开发节点关联的静态测试节点及准确工具版本，不得改变额外能力约束", EnUS: "Select a static test linked to the chosen development node and an exact tool version. Additional capability constraints must remain unchanged."}},
	{Key: "validation.task_dispatch_test_profile", SourceTemplate: "独立测试需要另一执行节点上可写的固定命令工具及完整JUnit报告协议", Texts: map[Locale]string{ZhCN: "独立测试需要另一执行节点上可写的固定命令工具及完整JUnit报告协议", EnUS: "Independent tests require a writable fixed command with the complete JUnit report protocol on a different worker."}},
	{Key: "execution.task_dispatch.test_binding_changed", SourceTemplate: "独立测试工具或计划固定依据已变化，请重新预览派发", Texts: map[Locale]string{ZhCN: "独立测试工具或计划固定依据已变化，请重新预览派发", EnUS: "The independent test tool or frozen plan basis has changed. Preview dispatch again."}},
}
