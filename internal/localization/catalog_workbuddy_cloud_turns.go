package localization

var workBuddyCloudTurnsCatalog = []Definition{
	{Key: "workbuddy_cloud_turn.invalid", SourceTemplate: "Invalid WorkBuddy cloud turn request", Texts: map[Locale]string{ZhCN: "云会话请求无效，请核对原任务、版本和操作", EnUS: "The cloud turn request is invalid. Check the original task, versions and operation."}},
	{Key: "workbuddy_cloud_turn.changed", SourceTemplate: "The WorkBuddy cloud turn changed; reload before deciding", Texts: map[Locale]string{ZhCN: "原云回合依据已变化，请重新读取后明确操作", EnUS: "The original cloud turn changed. Reload before deciding."}},
	{Key: "workbuddy_cloud_turn.unavailable", SourceTemplate: "WorkBuddy cloud turn is unavailable", Texts: map[Locale]string{ZhCN: "当前无法建立原任务的云会话，请核对账号与连接状态", EnUS: "The original cloud task conversation is unavailable. Check the account and connection."}},
	{Key: "workbuddy_cloud_turn_unknown", SourceTemplate: "workbuddy_cloud_turn_unknown", Texts: map[Locale]string{ZhCN: "原回合结果尚未核实，请核对原操作；不会自动再次发送", EnUS: "The original turn outcome is unverified. Reconcile the original operation; it will not be sent again automatically."}},
	{Key: "workbuddy_cloud_turn_interrupted", SourceTemplate: "workbuddy_cloud_turn_interrupted", Texts: map[Locale]string{ZhCN: "原会话连接中断，已保存的回复仍保留，远端执行状态待核对", EnUS: "The original conversation connection was interrupted. Saved replies remain available; remote execution needs reconciliation."}},
	{Key: "workbuddy_cloud_turn_limited", SourceTemplate: "workbuddy_cloud_turn_limited", Texts: map[Locale]string{ZhCN: "本次会话达到采集上限，仅保留已保存内容，执行结果尚未核实", EnUS: "This turn reached its collection limit. Saved content is retained; execution remains unverified."}},
	{Key: "workbuddy_cloud_turn_authorization_changed", SourceTemplate: "workbuddy_cloud_turn_authorization_changed", Texts: map[Locale]string{ZhCN: "原会话授权或执行依据已变化，后续发送已停止，原执行事实待核对", EnUS: "The original conversation authority or execution basis changed. Further sending stopped; reconcile the original execution facts."}},
	{Key: "workbuddy_cloud_turn_rejected", SourceTemplate: "workbuddy_cloud_turn_rejected", Texts: map[Locale]string{ZhCN: "本次回合请求未取得执行资格，请检查原任务与授权", EnUS: "This turn request was not admitted. Check the original task and authorization."}},
	{Key: "workbuddy_cloud_turn_unavailable", SourceTemplate: "workbuddy_cloud_turn_unavailable", Texts: map[Locale]string{ZhCN: "原会话目前不可用，请保留并核对本次操作", EnUS: "The original conversation is currently unavailable. Preserve and reconcile this operation."}},
}
