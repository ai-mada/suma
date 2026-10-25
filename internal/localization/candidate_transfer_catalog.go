package localization

// Only exact owned candidate diagnostics are translated; bundle bytes and
// immutable Git/Runner evidence retain their original content.
var candidateTransferCatalog = []Definition{
	{Key: "candidate_transfer.bytes_mismatch", SourceTemplate: "Candidate bundle bytes do not match the manifest", Texts: map[Locale]string{ZhCN: "候选代码包的字节与封存清单不一致", EnUS: "Candidate bundle bytes do not match the manifest"}},
	{Key: "candidate_transfer.blob_missing", SourceTemplate: "Candidate bundle is missing or changed", Texts: map[Locale]string{ZhCN: "候选代码包缺失或已变化", EnUS: "Candidate bundle is missing or changed"}},
	{Key: "candidate_transfer.storage_invalid", SourceTemplate: "Candidate bundle storage reference is invalid", Texts: map[Locale]string{ZhCN: "候选代码包存储引用无效", EnUS: "Candidate bundle storage reference is invalid"}},
	{Key: "candidate_transfer.consumer_invalid", SourceTemplate: "Candidate consumer must be an immutable command, agent or test", Texts: map[Locale]string{ZhCN: "候选消费节点必须是不可变构建命令、智能体或测试", EnUS: "Candidate consumer must be an immutable command, agent or test"}},
	{Key: "candidate_transfer.source_exists", SourceTemplate: "Candidate source already has a transfer; recover the original operation", Texts: map[Locale]string{ZhCN: "候选来源已有传递记录，请恢复原操作", EnUS: "Candidate source already has a transfer; recover the original operation"}},
	{Key: "candidate_transfer.source_invalid", SourceTemplate: "Candidate source must be a code command or agent", Texts: map[Locale]string{ZhCN: "候选来源必须是代码命令或智能体", EnUS: "Candidate source must be a code command or agent"}},
	{Key: "candidate_transfer.source_proof_changed", SourceTemplate: "Candidate source result does not match sealed evidence", Texts: map[Locale]string{ZhCN: "候选来源结果与封存依据不一致", EnUS: "Candidate source result does not match sealed evidence"}},
	{Key: "candidate_transfer.source_attempt_changed", SourceTemplate: "Candidate source successful attempt changed", Texts: map[Locale]string{ZhCN: "候选来源的成功尝试已变化", EnUS: "Candidate source successful attempt changed"}},
	{Key: "candidate_transfer.already_sealed", SourceTemplate: "Candidate transfer already sealed; recover the original operation", Texts: map[Locale]string{ZhCN: "候选传递已封存，请恢复原操作", EnUS: "Candidate transfer already sealed; recover the original operation"}},
	{Key: "candidate_transfer.not_accepting", SourceTemplate: "Candidate transfer is not accepting bytes", Texts: map[Locale]string{ZhCN: "候选传递当前不接受代码包写入", EnUS: "Candidate transfer is not accepting bytes"}},
	{Key: "candidate_transfer.not_sealed", SourceTemplate: "Candidate transfer is not sealed", Texts: map[Locale]string{ZhCN: "候选传递尚未封存", EnUS: "Candidate transfer is not sealed"}},
	{Key: "candidate_transfer.lease_changed", SourceTemplate: "Candidate transfer lease is no longer current", Texts: map[Locale]string{ZhCN: "候选传递的执行租约已失效", EnUS: "Candidate transfer lease is no longer current"}},
	{Key: "candidate_transfer.metadata_changed", SourceTemplate: "Candidate transfer metadata changed", Texts: map[Locale]string{ZhCN: "候选传递元数据已变化", EnUS: "Candidate transfer metadata changed"}},
	{Key: "candidate_transfer.pin_missing", SourceTemplate: "Consumer has no matching frozen candidate", Texts: map[Locale]string{ZhCN: "消费节点没有匹配的固定候选依据", EnUS: "Consumer has no matching frozen candidate"}},
	{Key: "candidate_transfer.size_invalid", SourceTemplate: "Invalid candidate bundle size or operation ID", Texts: map[Locale]string{ZhCN: "候选代码包大小或操作标识无效", EnUS: "Invalid candidate bundle size or operation ID"}},
	{Key: "candidate_transfer.assignment_invalid", SourceTemplate: "Invalid candidate transfer assignment", Texts: map[Locale]string{ZhCN: "候选传递执行身份无效", EnUS: "Invalid candidate transfer assignment"}},
	{Key: "candidate_transfer.manifest_invalid", SourceTemplate: "Invalid candidate transfer manifest or assignment", Texts: map[Locale]string{ZhCN: "候选传递清单或执行身份无效", EnUS: "Invalid candidate transfer manifest or assignment"}},
	{Key: "candidate_transfer.operation_invalid", SourceTemplate: "Invalid candidate transfer operation ID", Texts: map[Locale]string{ZhCN: "候选传递操作标识无效", EnUS: "Invalid candidate transfer operation ID"}},
	{Key: "candidate_transfer.request_invalid", SourceTemplate: "Invalid candidate transfer request", Texts: map[Locale]string{ZhCN: "候选传递请求无效", EnUS: "Invalid candidate transfer request"}},
	{Key: "candidate_transfer.operation_conflict", SourceTemplate: "Candidate transfer operation content changed", Texts: map[Locale]string{ZhCN: "候选传递操作标识对应的内容已变化", EnUS: "Candidate transfer operation content changed"}},
}
