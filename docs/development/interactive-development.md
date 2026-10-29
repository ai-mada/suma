# 交互开发：终端、讨论交接与AI SOP

状态：T130限定实现；产品1.0.0，2026-10-29。负责人：工作台、控制面、DAG和Runner领域。关联需求9、12、17、22、23及完整P0–P7／1–26；58项有限实现、34项待办、92项总账保持不变。

[架构](../architecture/interactive-terminal.md)、[终端界面](../design/interactive-terminal.md)、[恢复与交付交互](../design/terminal-recovery.md)、[T130合同](../../contracts/terminal-recovery-candidates.md)、[当前验证报告](../quality/terminal-recovery-candidates-verification.md)和[摘要](../quality/terminal-recovery-candidates-summary.json)分别说明实现、操作及证据。T129原终端和讨论开发记录保留在[历史报告](../quality/interactive-development-verification.md)／[摘要](../quality/interactive-development-summary.json)；组件检查不代替实际故障恢复、浏览器或跨平台验收。

## 两条明确入口

交互终端用于当前任务的人工受控Shell操作。它由专用`terminal_session`节点、原Go DAG和实际Runner执行，使用独立attempt Git工作树，不使用IDE绑定的共享目录。自然退出或停止不自动产生候选、测试证明或正式验收；停止修改须按下述独立授权流程交付。

“从讨论转为开发”以已完成助手回复作为受保护出处，人工审阅后生成原生开发计划。本次重新选择模型、参数和预算；明确启动后沿原候选、独立测试及人工Review链路交付。助手文本、创建预览和实际编码分别表达。

执行节点管理员可参考[Windows终端配置](../../deploy/runner/interactive-terminal.example.json)登记专用Shell；仍需明确启用执行、固定仓库／代码基线和仓库内专用缓存工作树，并在真实库存上启用档案。该示例不适用于Unix。原生开发使用[平台模型网关配置的命令示例](../../deploy/runner/native-coding.example.json)，提供方密钥由Go侧管理，不向智能体子进程或终端转发。示例中的本机自检不替代独立执行节点的正式测试。

| 术语 | 准确含义 |
| --- | --- |
| 创建终端／Create terminal | 明确授权后保存原会话和受保护DAG运行，不等于Shell已启动。 |
| 输入已接收／Input accepted | 原输入等待投递；序号递增不证明PTY已收到。 |
| 已送达／Delivered | 字节交给原PTY，不证明命令执行成功。 |
| 交付未知／Delivery unknown | 不重发原字节，不自动新建Shell，核对原会话。 |
| 断开查看／Disconnect view | 仅断开视图，不确认进程已停止。 |
| 请求停止／Request stop | 等待真实进程树结果，不提前标记stopped。 |
| 创建开发计划／Create development plan | 固定原任务、reply出处、流程、工具和新模型预算，形成原派发预览。 |
| 启动已审阅开发／Start reviewed development | 单独启动原保留run，不跳过独立测试或人工验收。 |

## 使用终端

1. 在项目IDE上方选定准确任务，展开“交互终端”。未选任务时不读取其他任务会话或自动创建空Shell。
2. 当前列表显示最近最多20个会话；已有会话可直接选择。`profiles_complete=false`表示配置目录覆盖不完整，空结果不证明不存在其他配置。
3. 新建时选择当前可用的专用终端Profile、60–3600秒时限，明确勾选Shell授权。尺寸由工作台适配；请求不能提供任意目录、参数或脚本。
4. 等待原Runner领取及真实running状态。连接状态不代表进程状态，只读查看者不获得输入权。
5. 在xterm输入，单次最多8KiB；原输入不进入浏览器持久恢复区。未知／被拒输入会提示并暂停新输入，不自动重放。核对确认不能扩大服务端权限。
6. 折叠或断开查看后，重连同一会话和输出游标。出现gap时，明确知道历史不完整。
7. 结束时勾选停止确认并请求停止，等待stopped、exited或reconciling实际事实。Ctrl-C、关闭页面、Shell退出和后代停止不能互相替代。

当前输入、resize及消费ACK由创建者身份控制，epoch固定为1；没有多人接管或跨标签独占控制功能。项目有写权限人员可请求停止。任务／需求、机器、工具、路由或权限变化后，不继续使用失效依据。

活动终端支持原出站broker的只读实时文件检查；该入口不能远程保存或验收。停止后的候选封存与基线交接使用独立受保护命令。Shell的独立工作树不是OS安全沙箱，不据此宣称不可信多租户隔离完成。

## 中断核对与终端修改交付

1. 在原会话展开“核对中断的执行”，读取原运行、尝试、栅栏、派发与真实进程树停止证明。没有停止证明只能等待原执行节点回报，不能人工声明进程已停。
2. 当前审批人逐项核对全部交付未知输入，填写说明并明确按失败关闭原执行。确认只接受未知事实，不把输入改成已送达、不重放，也不把原运行改成成功。网络未知时核对同一原请求；当前恢复范围和未覆盖故障以专项报告为准。
3. 在“将终端修改交给质量流程”读取已有交付计划；刷新和选择只查看原事实，不重复执行。选择新预览时，须已有停止快照，选择已发布的固定检查／候选测试流程及原Worker上的匹配检查配置，再明确授权创建预览。
4. 核对固定任务、原基线、停止快照和检查流程，再单独授权启动。宿主从原缓存验证并复制修改到新工作树，固定检查后封存；另一Worker导入同一SHA独立测试。候选、JUnit、正式Review和问题回流继续沿现有质量链路，提前验收仍拒绝。
5. 原任务已正式验收且仍绑定该精确运行后，可展开“交接已验收基线”，选择另一未启动任务，核对当前基线、填写理由并明确确认。此处只绑定候选基线，不启动后续执行；后续仍须单独派发、测试和验收。

终端启动授权不包含候选封存授权，候选预览不包含执行授权，执行成功不包含人工验收。当前切片使用固定命令而非新模型调用；实际本机完整链路、恢复证据与当前界面范围由[T130报告](../quality/terminal-recovery-candidates-verification.md)及[摘要](../quality/terminal-recovery-candidates-summary.json)统一记录。

## 从回复交接开发

1. 完成原任务讨论及真实助手回复，确认任务和讨论依据仍有效。
2. 展开“从讨论转为开发”，查看原回复，编辑已审阅说明，最多8,000个UTF-8字节。
3. 补充本计划检查项，每行一项、最多32项、每项最多1,000字节；可明确采用原任务标准作为参考。改变正式范围仍走原任务／需求编辑，不静默覆盖。
4. 选择已发布流程、编码步骤和当前派发选项中的可写原生智能体。只有一个编码步骤时可自动选定该步骤，工具仍明确选择。
5. 复用现有模型面板，重新固定模型、参数及Token／费用预算。通用原生上限128步／131072总输出；只读回复专门限制仍为1步／8192，不混用用途。
6. 明确审阅后创建计划。查看原TaskDispatchPlan、任务上下文、工作流和模型预算；这一步不运行代码。
7. 再次审阅并明确启动原保留run。依据变化时刷新并重新审阅；HTTP结果未知时核对原操作，不创建另一计划冒充恢复。
8. 沿原运行查看真实进度、独立工作树及封存候选，同SHA独立测试和人工Review通过后，按原任务验收规则交付。

本阶段执行器限定原生suma／suma-jsonl-v1、可写attempt工作区及统一模型协议，不从Octop或其他CLI名称推断交接能力。API行为、真实原生开发和浏览器旅程分别记录，不互相替代。

## 开发与排障SOP

修改前先读取当前任务、需求、reply出处及原会话。入口为`workspace_terminal*`／`task_discussion_development*`领域、`terminalcontract`、DAG受保护节点、Runner执行器和`terminalruntime`；界面为TerminalPanel、TerminalViewport及DiscussionDevelopmentPanel。

保持一个调度器和一个模型预算权威。不得由HTTP直接启动Shell，不得把普通command伪装为PTY，不得从用户ID重造凭据，不得把模型文本自动喂给终端。恢复依赖原输入身份、投递回执及实际停止结果。

排障先查当前授权／库存，再查原DAG和准确attempt，之后核对journal、输入回执与输出序号；网络失联、输入未知、进程树未知分别处理。通用日志不复制输入、环境或终端正文，只保留必要身份、操作、序号与摘要。Shell使用独立HOME和临时目录，不通过EnvFrom转授秘密。

功能变更只运行受影响的合同、权限／回执、真实PTY或界面检查。实际源改变后重建对应产物；不重复无关模型调用、SDK安装或完整旧旅程。源码、协议夹具、Windows底层实际、浏览器、跨平台安装和生产验收分开归档。

## 当前交付范围

Windows ConPTY／Job底层的中文、Ctrl-C、resize及前后台停止已有实际证据。Unix没有完整后代containment证明，Probe不宣称可用。SaaS隔离、多人接管、WebSocket、LSP／DAP、三系统实际安装及完整工作台保留待办；当前完整平台并未交付完成。

*English: Reconnect only to the original terminal session; ambiguous input is not replayed. T130 recovery requires actual persisted tree-stop proof, individual unknown-input acknowledgement and explicit failed-only closure. Stopped changes use separate candidate-preview and start consent, fixed checks, a new worktree and same-SHA tests on another Worker. Only current formal acceptance of that run permits an explicit baseline handoff to an unstarted task, without starting it. Reviewed discussion development still chooses a fresh native model budget through the original dispatch plan. Recovery scenarios, Windows/browser evidence, Unix containment, cross-platform installation and production remain separately scoped.*
