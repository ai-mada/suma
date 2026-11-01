# 开发工具交接与远端接入架构

T-DEVELOPER-TOOL-HANDOFF-182，suma 1.0.0。依据[冻结合同](../../contracts/developer-tool-handoff.md)；证据与限制见[专项验证](../quality/developer-tool-handoff-verification.md)。本阶段扩展两个已有入口，不建立新身份源、公共 API、调度器或兼容别名。

## A：讨论来源进入原任务派发

`TaskDiscussionDevelopmentCreateRequest` 接受可选的 `model_execution_selection` 与 `codex_execution_selection`，同时提供时拒绝。它不强制二选一：原派发器根据准确 Profile 决定需要原生模型选择、Codex 目录选择，还是沿用固定配置。请求不开放任意 `knowledge`。

构建顺序保持为：

1. 原当前权限事务、稳定操作键与回执恢复；已确认的原操作优先恢复。
2. 核验准确 reply／message、任务和需求版本、封存来源摘要及消息携带的有序知识依赖。
3. 将 `knowledgeReferences(reply.Source.Knowledge)` 传入 `buildTaskDispatchSnapshotTx`。原 `taskContextForDispatchTx`／`resolvePublishedKnowledgeTx` 复核当前发布版本、摘要、来源资格与可访问性。
4. 原工作流构建器判断结构化可写协议及独立 `candidate_from` 测试节点，固定任务上下文；再由原模型、Codex 目录和原生技能构建逻辑固定绑定。审阅目标仅作为工程目标追加，不授予工具、路径或权限。
5. 预览、来源关系、活动与回执同事务保存；明确 Start 沿用原 CAS、路由、预算、运行唯一性与任务绑定。

知识原本就在构建前进入该路径，本阶段保留此顺序，并新增包含来源技能的 Codex 回归证据；没有后置补技能包或额外来源解析器。Codex 的 `Context.Knowledge`、原生技能包和领取配置必须一致。技能正文通过原生附件传递，普通记忆保留在原目标上下文。

普通派发器目前支持 suma、codex、harness、codebuddy、固定 Qoder ACP、Claude、OpenCode SDK、Octop。支持判断由原协议与不可变 Profile 完成；前端可采用原 dispatch-options 的资格结果，不能靠名称授予权限。Codex App Server 沿用目录／配置摘要、模型和推理绑定，用量为 observe-only，enforce 策略继续拒绝。固定模型配置不产生另一套选择或虚假预算。

后续继续使用原 Runner、独立工作树、候选封存与传输、同提交独立测试、正式 Review、人工验收及基线交接。原创建／Start 正文和操作键恢复相同计划／运行；不把聊天回复、进程退出或测试成功升级为人工验收。

## B：配置准备与一次库存操作

`--prepare-tools PATH [--local-tool ID]` 复用八项本机探测目录，只接受同时满足 Installed、LaunchSupported 且存在 Command 的结果，输出原 `CommandSpec`／`DecodeCommands` 格式。当前可生成项为 Codex JSONL、CodeBuddy、dsh 与 Claude；其他配置型／仅检测项保留原因。

准备模式不读取 Runner 凭据、不打开日志、不连接控制面。目标必须是新文件，父目录及祖先须已存在且无链接；原父目录锚定和独占创建防止覆盖或链接穿越。零条命令不创建文件。输出 `suma-runner-tools-preparation-v1` 的 prepared、file、command_count、remaining，仅记录非秘密配置事实。

`--report-tools` 复用原项目／执行节点凭据、日志锁与完整 `DescribeTools`。新清单先持久化原 operation，再上报并记录 ACK。存在 pending 时先恢复同 operation／body；恢复不依赖后来变化或缺失的命令文件，且该次调用停在原 ACK，不追加新探测／新清单。未知结果和其他任务日志保持原样。

本地 inventory 的 `scope_sha256` 绑定经原规则校验的控制 URL、项目与执行节点；完整 URL 路径保留，仅规范末尾斜杠，凭据不进入摘要。持久日志、单次／持续上报、HTTP 客户端及只读检查器拒绝缺失或不匹配的摘要，不回填旧状态、不跨控制面／项目／执行节点重放。续期凭据本身不改变该作用域。

单次模式只做 inventory，不进入 `Worker.Run`，不 Claim、不发送任务 Heartbeat。`--allow-exec` 仍只授权原固定程序／协议／工作区探测；可写配置仍需管理员仓库、完整不可变基线及可用工作区。无该标志时保留真实禁用观察。确认结果使用 `suma-runner-tool-report-v1`，不创造健康或启用权限。

## AI SOP 与验证边界

AI 只提交明确审阅后的工程目标；始终保留原来源 pins、权限、版本、原操作和候选质量链。遇到未确认写入先恢复，不换键；遇到作用域不符、来源失效或预算能力不符时停止依赖动作，展示原事实。配置准备、库存、连接、启用、执行、独立测试与人工验收分别记录。

定向证据包含生产 HTTP／SQLite／派发器的受控协议检查，以及实际本机文件、日志锁、Windows junction 与受控 HTTP。未变候选独测全链沿用原限定证据。物理远端、多主机、真实厂商模型／账单、其他数据库、跨平台安装和生产规模仍需独立验收；原 31 项／P0–P7 范围不缩减。
