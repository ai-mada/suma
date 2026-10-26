package localization

// Host-owned setup diagnostics; provider prose and immutable event originals stay raw.
var teamCLICatalog = []Definition{
	{Key: "team_cli.node_executable", Texts: map[Locale]string{ZhCN: "团队 CLI 的 JavaScript 入口需要使用 Node 可执行文件", EnUS: "team CLI JavaScript entries require the Node executable"}},
	{Key: "team_cli.node_entry", Texts: map[Locale]string{ZhCN: "团队 CLI 的 Node 模式需要在固定参数前指定一个 JavaScript 入口绝对路径", EnUS: "team CLI Node mode requires one absolute JavaScript entry before the fixed CLI arguments"}},
	{Key: "team_cli.node_file", Texts: map[Locale]string{ZhCN: "团队 CLI 的 Node 入口必须为不超过 16 MiB 的现有普通文件，不能使用符号链接", EnUS: "team CLI Node entry must be an available regular file of at most 16 MiB without a symbolic link"}},
	{Key: "team_cli.entry_unavailable", Texts: map[Locale]string{ZhCN: "运行时 JavaScript 入口不可用", EnUS: "runtime JavaScript entry is unavailable"}},
	{Key: "team_cli.entry_bound", Texts: map[Locale]string{ZhCN: "运行时 JavaScript 入口不是大小合规的普通文件", EnUS: "runtime JavaScript entry is not a bounded regular file"}},
	{Key: "team_cli.entry_digest", Texts: map[Locale]string{ZhCN: "无法获取运行时 JavaScript 入口内容摘要", EnUS: "runtime JavaScript entry digest unavailable"}},
	{Key: "team_cli.profile", Texts: map[Locale]string{ZhCN: "团队 CLI 需要隔离的开发配置；正式测试必须独立执行", EnUS: "team CLI requires an isolated development profile; formal tests remain independent"}},
	{Key: "team_cli.harness_args", Texts: map[Locale]string{ZhCN: "Harness 需要固定 multica stdio 配置和 dsh-jsonl 输入", EnUS: "Harness requires the fixed multica stdio profile and dsh-jsonl input"}},
	{Key: "team_cli.buddy_args", Texts: map[Locale]string{ZhCN: "CodeBuddy 需要固定 stream-json 输出和目标输入", EnUS: "CodeBuddy requires fixed stream-json output and goal input"}},
	{Key: "team_cli.buddy_override", Texts: map[Locale]string{ZhCN: "CodeBuddy 命令包含重复或不支持的协议参数", EnUS: "CodeBuddy command contains duplicate or unsupported protocol arguments"}},
	{Key: "team_cli.buddy_value", Texts: map[Locale]string{ZhCN: "CodeBuddy 命令需要固定参数值", EnUS: "CodeBuddy command requires a fixed argument value"}},
	{Key: "team_cli.harness_probe", Texts: map[Locale]string{ZhCN: "Harness 的 multica 运行配置不可用或不兼容", EnUS: "Harness multica runtime profile is unavailable or incompatible"}},
	{Key: "team_cli.buddy_probe", Texts: map[Locale]string{ZhCN: "已安装 CodeBuddy 的帮助信息与固定 stream-json 协议不匹配", EnUS: "installed CodeBuddy help does not match the fixed stream-json protocol"}},
	{Key: "team_cli.agent_node", Texts: map[Locale]string{ZhCN: "团队 CLI 配置需要使用智能体节点", EnUS: "team CLI profiles require an agent node"}},
	{Key: "team_cli.dsh_input", Texts: map[Locale]string{ZhCN: "dsh-jsonl 输入需要使用 Harness 适配器", EnUS: "dsh-jsonl input requires the Harness adapter"}},
	{Key: "team_cli.dsh_prepare", Texts: map[Locale]string{ZhCN: "无法准备 Harness 输入", EnUS: "Harness input could not be prepared"}},
	{Key: "team_cli.dsh_pipe", Texts: map[Locale]string{ZhCN: "无法打开 Harness 标准输入管道", EnUS: "Harness stdin pipe could not be opened"}},
	{Key: "team_cli.input_format", Texts: map[Locale]string{ZhCN: "智能体命令配置需要使用受支持的固定标准输入格式", EnUS: "agent command specification requires a supported fixed stdin format"}},
}
