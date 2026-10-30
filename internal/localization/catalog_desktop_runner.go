package localization

// The private desktop API emits stable domain diagnostics. Display language
// changes neither signed requests, credential digests nor persisted receipts.
var desktopRunnerCatalog = []Definition{
	{Key: "desktop_runner.invalid", SourceTemplate: "desktop_runner_invalid", Texts: map[Locale]string{ZhCN: "本机连接请求无效，请核对当前项目并重试原操作", EnUS: "The local connection request is invalid. Check the current project and retry the original operation."}},
	{Key: "desktop_runner.unavailable", SourceTemplate: "desktop_runner_unavailable", Texts: map[Locale]string{ZhCN: "当前服务不支持本机托管连接，请使用速码桌面端", EnUS: "This service does not support managed local connections. Use the Suma desktop application."}},
	{Key: "desktop_runner.proof_invalid", SourceTemplate: "desktop_runner_proof_invalid", Texts: map[Locale]string{ZhCN: "本机连接证明无效，请从当前桌面窗口核对连接", EnUS: "The local connection proof is invalid. Reconcile it from the current desktop window."}},
	{Key: "desktop_runner.proof_expired", SourceTemplate: "desktop_runner_proof_expired", Texts: map[Locale]string{ZhCN: "本机连接证明已过期，请核对原连接后重试", EnUS: "The local connection proof expired. Reconcile the original connection and try again."}},
	{Key: "desktop_runner.repository_unavailable", SourceTemplate: "desktop_runner_repository_unavailable", Texts: map[Locale]string{ZhCN: "当前项目目录不可用，请检查已绑定的本机工作区", EnUS: "The project directory is unavailable. Check its bound local workspace."}},
	{Key: "desktop_runner.repository_unsafe", SourceTemplate: "desktop_runner_repository_unsafe", Texts: map[Locale]string{ZhCN: "项目目录未满足安全的 Git 工作区要求，请检查仓库根目录和缓存忽略配置", EnUS: "The directory does not meet the Git workspace requirements. Check its repository root and ignored cache configuration."}},
	{Key: "desktop_runner.head_unavailable", SourceTemplate: "desktop_runner_head_unavailable", Texts: map[Locale]string{ZhCN: "项目仓库没有可固定的完整提交，请先提交代码再连接", EnUS: "The repository has no complete commit to pin. Commit its code before connecting."}},
	{Key: "desktop_runner.repository_changed", SourceTemplate: "desktop_runner_repository_changed", Texts: map[Locale]string{ZhCN: "项目仓库或提交已变化，请重新核对本机连接依据", EnUS: "The repository or commit changed. Recheck the local connection basis."}},
	{Key: "desktop_runner.context_changed", SourceTemplate: "desktop_runner_context_changed", Texts: map[Locale]string{ZhCN: "本机实例或项目工作区已变化，请核对原连接后再操作", EnUS: "The desktop instance or project workspace changed. Reconcile the original connection first."}},
	{Key: "desktop_runner.worker_exists", SourceTemplate: "desktop_runner_worker_exists", Texts: map[Locale]string{ZhCN: "该执行节点已有登记，请核对原操作，不要重复发行凭据", EnUS: "The execution node is already registered. Reconcile the original operation instead of issuing another credential."}},
}
