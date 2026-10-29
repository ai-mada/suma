package localization

// Only product-owned diagnostics are translated. Shell output, saved discussion
// and reviewed engineering goals remain original user/provider content.
var interactiveDevelopmentCatalog = func() []Definition {
	rows := [][3]string{
		{"invalid", "Invalid workspace terminal request", "工作区终端请求无效"},
		{"consent", "Confirm real shell execution in the isolated attempt workspace", "请确认在独立工作区中运行真实命令解释器"},
		{"input_queue", "Terminal input queue is full; wait for original delivery receipts", "终端输入队列已满，请等待原输入送达回执"},
		{"changed", "The terminal basis or control version changed; refresh the original session", "终端依据或控制版本已变化，请刷新原会话"},
		{"profile", "Select a current enabled interactive terminal profile", "请选择当前已启用的交互终端配置"},
		{"active", "This task already has an active or uncertain terminal session", "此任务已有活动或结果待核对的终端会话"},
		{"authority", "The terminal creator no longer has current workspace execution authority", "终端创建者当前已无工作区执行权限"},
		{"input_unknown", "Terminal input delivery is unknown; inspect the original session without replaying input", "终端输入是否送达尚待核对，请检查原会话，不要重发输入"},
		{"tree_unknown", "The original terminal process tree has not been confirmed stopped", "原终端进程树尚未确认停止"},
		{"controls", "Interactive terminal sessions must use their dedicated controls", "请使用交互终端专用控制操作"},
		{"required", "Interactive shells require a protected terminal session", "交互式命令解释器需要受保护的终端会话"},
		{"capability", "Terminal capability requires an isolated administrator shell profile", "终端能力需要独立的管理员命令解释器配置"},
		{"configured", "Interactive terminal requires a dedicated shell without inherited secrets or automation capabilities", "交互终端需要专用命令解释器，禁止继承密钥或自动化能力"},
		{"different_capability", "Terminal capability differs from local configuration", "终端能力与本机配置不一致"},
		{"binding", "Terminal execution requires its original non-retryable assignment and control client", "终端执行需要原不可重试派发及控制面连接"},
		{"shell", "Terminal execution requires an isolated administrator shell profile", "终端执行需要独立的管理员命令解释器配置"},
		{"secrets", "Terminal profile cannot forward execution credentials", "终端配置不能转发执行凭据"},
		{"no_pty", "Terminal profile does not have a current real PTY capability", "终端配置当前没有实际可用的伪终端能力"},
		{"stopped_before_start", "Terminal execution was stopped before starting", "终端执行已在启动前停止"},
		{"changed_shell", "Terminal shell profile changed before process launch", "进程启动前终端配置发生了变化"},
		{"no_model", "Terminal sessions cannot inherit a model gateway binding", "终端会话不能继承模型网关绑定"},
		{"exchange", "Terminal exchange identity is invalid", "终端交换身份无效"},
		{"response", "Terminal exchange response is invalid", "终端交换回执无效"},
		{"epoch", "Terminal control epoch moved backwards", "终端控制代次发生倒退"},
		{"input_changed", "Terminal input differs from its original accepted bytes", "终端输入与原已接受字节不一致"},
		{"ack", "Terminal output acknowledgement differs from original sent bytes", "终端输出确认与原已发送字节不一致"},
		{"gap", "Terminal output acknowledgement fell behind retained history", "终端输出确认已落后于保留范围"},
		{"frame_limit", "Terminal frame exceeds its byte limit", "终端帧超过字节上限"},
		{"frame_bytes", "Terminal frame bytes are invalid", "终端帧字节无效"},
		{"runtime", "terminal runtime configuration is invalid", "终端运行配置无效"},
		{"runtime_unknown", "original terminal process ownership must be reconciled", "需要核对原终端进程的归属"},
		{"descendants", "terminal descendant containment could not be confirmed", "无法确认终端后代进程已受控"},
		{"journal", "terminal journal already exists; do not restart the original process", "终端日志已存在，请勿重新启动原进程"},
		{"unknown_delivery", "terminal input delivery is unknown; do not replay its bytes", "终端输入送达状态未知，请勿重放字节"},
		{"input_conflict", "terminal input identity or sequence conflicts with the original receipt", "终端输入身份或序号与原回执冲突"},
		{"input_limit", "terminal input receipt limit reached", "终端输入回执已达到上限"},
		{"not_accepting", "terminal process is not accepting input", "终端进程当前不接受输入"},
		{"handoff_invalid", "Invalid discussion development handoff request", "讨论开发交接请求无效"},
		{"handoff_changed", "The discussion development basis changed; review the current task and reply", "讨论开发依据已变化，请审阅当前任务和回复"},
		{"handoff_source", "Select a verified completed assistant reply for the current task", "请选择当前任务已核验完成的智能体回复"},
		{"handoff_consent", "Confirm the reviewed plan, selected model budget and real workspace execution", "请确认审阅后的计划、模型预算及实际工作区执行"},
		{"handoff_profile", "Select an enabled writable native workspace development profile", "请选择已启用且支持原生工作区写入的开发配置"},
		{"handoff_limit", "The reviewed development context exceeds 32000 UTF-8 bytes; reduce its scope", "已审阅的开发上下文超过32000个UTF-8字节，请缩小范围"},
	}
	out := make([]Definition, 0, len(rows))
	for _, row := range rows {
		out = append(out, Definition{Key: "interactive_development." + row[0], SourceTemplate: row[1], Texts: map[Locale]string{ZhCN: row[2], EnUS: row[1]}})
	}
	return out
}()
