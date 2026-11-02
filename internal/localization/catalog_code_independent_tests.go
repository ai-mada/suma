package localization

// The two diagnostics describe actual current execution evidence. A selected
// test profile or two slots on the same Worker never supplies these facts.
var codeIndependentTestCatalog = []Definition{
	{Key: "execution.evidence.current_code_test_identity_required", Texts: map[Locale]string{ZhCN: "代码和测试必须有明确成功的当前执行节点与尝试证据", EnUS: "Code and tests require successful current worker and attempt evidence."}},
	{Key: "execution.evidence.independent_current_candidate_required", Texts: map[Locale]string{ZhCN: "每个候选代码必须由另一执行节点按当前来源尝试独立测试", EnUS: "Each code candidate must be tested by another worker against its exact current source attempt."}},
}
