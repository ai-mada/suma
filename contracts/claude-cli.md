# Claude Code 前台CLI执行合同

T-CLAUDE-CLI-158，速码1.0.0。状态：固定CLI实现及159受控正式Worker／HTTP／GUI链路已限定验证；本机未发现真实Claude安装，未登录或调用真实厂商。协议夹具、正式平台联调与厂商实测分别记录。保留完整58／34／92范围。

## 身份与固定宿主策略

入口固定为`claude-code-cli`，adapter=`claude`，protocol=`claude-jsonl-v1`。这与`claude-code-client`的MCP配置导出、仍planned的Claude SDK执行入口分开。复用原Worker、DAG／Pool领取、当前授权、Profile、Journal、工作树、候选、独立测试和人工验收；无第二派发器。

首版仅原生可执行文件，不运行Node、npm、shell或`.cmd/.bat/.ps1`包装器。只支持已审阅的官方2.1.314基线，实际版本必须等于`2.1.314`或`2.1.314 (Claude Code)`，并由同一可执行文件的5秒有界`--help`确认全部固定安全参数。新版本未经审阅不可自动扩大协议支持。实际可执行SHA、版本、规范化参数及强制环境共同进入原Profile摘要；只探测安装与协议，不证明登录或模型可用。

固定参数为`-p --input-format text --output-format stream-json --verbose --no-session-persistence --safe-mode --restricted`，加以下固定值：

- `--settings`：`{"disableAllHooks":true,"permissions":{"blockReadsOutsideWorkingDirectories":true,"blockWritesOutsideWorkingDirectories":true}}`。
- `--strict-mcp-config --mcp-config {"mcpServers":{}}`。
- `--permission-mode dontAsk --permission-prompts none`。
- `--tools Read,Edit,Write,Glob,Grep,Bash`；自动许可仅Read／Edit／Write／Glob／Grep及`Bash(git status *)`、`Bash(git diff *)`、`Bash(git add *)`、`Bash(git commit *)`。这是管理员明确审阅的有限Git记录权限，不是通用Bash许可。
- `--model`为管理员固定的完整`claude-*`模型ID，模板采用官方文档当前示例`claude-sonnet-5`。不接受sonnet／opus等可漂移别名。该配置不是平台模型选择或供应方身份保证。

目标只由stdin一次写入后关闭。任务／模型不能提供argv、cwd、会话、resume、plugin、MCP、权限回复或凭据。`EnvFrom`及非宿主固定环境拒绝；宿主规范化并强制`CLAUDE_CODE_DISABLE_BACKGROUND_TASKS`、`CLAUDE_CODE_DISABLE_AGENT_VIEW`、`CLAUDE_CODE_DISABLE_CRON`、`CLAUDE_CODE_DISABLE_AUTO_MEMORY`、`CLAUDE_CODE_DISABLE_CLAUDE_MDS`、`CLAUDE_CODE_DISABLE_ARTIFACT`、`CLAUDE_CODE_DISABLE_BUNDLED_SKILLS`为1。此次进程同时固定`DISABLE_UPDATES=1`，不自动更新或安装已固定版本，不修改用户配置。不得使用dangerously-skip-permissions或隐式批准control_request。

保留工具自身现有登录与系统认证上下文，速码不读取或复制登录文件，不传Runner凭据。不使用`--bare`，因为官方明确该模式不读取OAuth／系统钥匙串。safe-mode关闭项目自动定制与MCP发现；managed policy仍优先，可能执行管理员hooks等，本合同不绕过或冒称禁用这些策略。restricted和目录读写限制约束内建文件工具，不是Bash的OS隔离。宿主及本地管理员仍在原信任边界内。

## 配置、模型与当前派发

`Profile.ClaudeExecution`为公开固定事实：`model`、`model_source=administrator_cli_argument`、`configuration_sha256`、`budget_mode=observe_only`、`identity_evidence=configuration_ack_only`、`auth_strategy=existing_cli_login`。available=true必须有完整cap及实际版本；不可用报告可缺cap，不可伪造已探测。无WorkspaceTools／ModelGateway／TestReport／ConflictRepair或专用purpose能力。

派发计划及原agent节点的`claude_execution`必须与当前已启用Profile完全相同，`MaxRetries=0`。任何model/env/permission/MCP/resume等节点覆盖，包含null，都拒绝。预览、明确Start、private／Pool claim、回执持续核对当前成员／owner、Profile／库存、Pool授权、attempt与fence。连接或配置不自动授权。

`ModelExecutionOptions.fixed_claude`只显示原配置，supported／codex_supported为false，无平台可选deployment。组织与项目任一enforce策略在预览、启动、claim及accountingCheck／Reserve均拒绝；CLI的max-budget-usd不是平台硬预算，不映射宿主gateway。未定价、未上报与部分token事实保留，不填0，不把客户端USD估算当真实账单。

## 流与终态

复用64KiB单行、JSON／UTF-8／深度／重复键边界、脱敏与持久事件，不保存thinking、原工具输入／结果或认证日志。init必须唯一，session稳定，model精确确认固定完整ID；发现外部MCP、plugin、后台或交互协议即停止并保留unknown。主会话与子代理不得混记；此固定tools集不开放Agent，非空parent_tool_use_id不进入主会话终态。

assistant文本只作进度。tool_use／tool_result必须同一ID闭合，不能用子工具成功代替整体结果。control_request不自动回复，权限拒绝作为原事实保留。

唯一整体`result`要求原session、合法is_error与官方已知subtype。success还需无permission_denials、无未完成工具、无deferred_tool_use或排队turn；可选terminal_reason保持空或completed，其他reason不能推成完成。缺失／冲突／晚到／非法结果或未知EOF保留unknown。退出0、模型成功文本或供应方subtype均不能替代全部条件。

用量以合法最终modelUsage优先于主循环usage，不相加重复总额；缺总额时按唯一message.id累计已见主循环input／cache，output占位不当完成用量。崩溃零化的结果不得覆盖此前已见部分用量。所有CLI用量最多partial，缺失为unreported；不声明供应方账单或完整辅助调用覆盖。

## 工作树、候选与停止

每次执行沿原固定仓库与管理员完整base创建独立工作树，不进入用户脏目录。CLI使用已明确许可的Git命令记录真实提交；文本或未提交修改不算候选。只有clean、新HEAD、完整来源证据及原传输渠道才能沿原candidate outbox上传。后续独立command／JUnit测试准确候选，人工验收仍由原权威负责，Claude不自认证测试或验收。

取消复用宿主进程控制与真实Wait／Finish。Windows Job Object可提供本机进程树停止证据；其他系统沿既有限制，不能用信号发送或协议ACK冒充所有后代已停。重启恢复原journal／outbox，不自动重发模型任务或任意resume未知作用。

## 官方主源与验收边界

- [2.1.314正式发布](https://github.com/anthropics/claude-code/releases/tag/v2.1.314)：写目录保护基线。
- [CLI参考](https://code.claude.com/docs/en/cli-reference)：safe-mode／restricted／完整模型与参数。
- [程序化运行](https://code.claude.com/docs/en/headless)：print／stream-json、认证与终止。
- [环境变量](https://code.claude.com/docs/en/env-vars)：后台／agent view／cron等明确门禁。
- [Hooks参考](https://code.claude.com/docs/en/hooks#disable-or-remove-hooks)：disableAllHooks及managed优先级。
- [官方结果类型](https://code.claude.com/docs/en/agent-sdk/typescript#sdkresultmessage)与[用量说明](https://code.claude.com/docs/en/agent-sdk/cost-tracking)：可选reason、deferred／queued与部分用量。

定向回归分别验证参数／权限边界、版本与help、会话／模型／终态、权限拒绝与部分用量、真实本机协议子进程、工作树候选与独立测试、原操作及两级预算拒绝。受控协议夹具不代表真实Claude已安装或厂商任务通过。最终一次相关集成构建及中英／窄屏实际界面由root协调；不改T156／157在途入口，不提前宣称通过。

## T-CLAUDE-FORMAL-159：正式运行回执

原计划保留发布来源流程，实际受托管运行使用准确保留的run_id及`managed-<run_id>`工作流、版本1。直接任务、讨论开发、终端候选和候选交接共用准确运行身份核对，各入口仍独立验证当前项目、原计划／来源、状态、节点和模型资格。读取原计划失败时保留原操作，不用新Start替代恢复。

受控协议夹具已沿正式双Worker、HTTP领取／结果、真实候选、同提交Node／JUnit及独立测试账号Review退回验证；原Run保持成功，任务阻塞并关联Issue。跨窗口操作持久性未证明：新窗口没有原pending，测试宿主仅重载原请求后恢复；它不构成产品自动持久恢复。实际范围及失败见[正式链路报告](../docs/quality/claude-formal-verification.md)。真实厂商、物理多机、其他系统与生产继续待验。
