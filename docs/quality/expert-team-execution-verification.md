# 专家团队执行跟进验证

阶段：T-TEAM-EXECUTION-141；产品1.0.0；2026-10-30。最初增量检查基线为 `86fb65e`；联合验证时139与143已提交至 `f5fedde`，140／141／142共同联调。不将其他阶段的既有检查重复计数。完整平台目标继续在原实施计划中推进。

## 当前实现

原专家团队计划现在按准确运行的完整节点状态区分排队、等待执行、正在执行和待核对；历史上存在attempt不再表示当前仍在执行。最多投影24个节点，优先未知与异常／活动事实，明确完整节点数及截断。节点数量与状态分类来自全量读取，不能用截断列表计算完成度。

执行明细只在当前授权读取时生成；不保存到计划及历史修订，不因读取时间变化增加计划版本。原控制器仍按真实业务状态变化更新派生状态。计划、运行、独立测试及正式验收继续分开；可容忍的失败／取消分支不覆盖有效的整体运行与人工验收。

前端沿原团队计划展示当前步骤、原始原因、读取时间、范围和准确run／node／attempt链接；只做导航，不加入取消、重试或批准动作。详细设计见[执行跟进](../design/expert-team-execution.md)，领域规则见[原合同](../../contracts/expert-team-orchestration.md)。

## 已取得证据

- 后端8个顶层用例通过，4.303秒，实际命令为 `go test ./internal/platform -run 'TestExpertTeam(ExecutionProjection|OrchestrationReconcilingNodeProjection|OrchestrationBoundedFairDueAndNoopVersion|DependentDispatchWaitsForFormalReview|PlanStartReceiptKeepsOriginalRunsAfterHeadChange)' -count=1 -v`，日志 `.cache/team-execution-141/backend-tests.log`。包含8个状态子场景、24项截断与全量分类、字段白名单、历史明细清理、撤权／跨项目、版本稳定、原操作回执及前置任务正式验收。子场景不另外累加。部分状态通过数据库夹具注入，证明投影逻辑，不冒充真实厂商运行或多机验收。
- `go test ./cmd/suma-contracts -count=1` 全包通过，1.349秒；生成及 `-check` 通过：736模型、340路径。生成前后语义比较仅新增2个执行事实schema并扩展ExpertTeamPlanItem，已有路径不变；139／140注册保留。日志与语义比较位于 `.cache/team-execution-141/contracts/`。
- 复审纠正团队条目的 `executing` 与节点 `running` 枚举混淆；纠正失败／取消的可容忍分支被误计阻塞；执行明细与读取时间从持久CAS比较中排除；未知节点优先呈现。

- 前端4文件24项通过（9.402秒）：`ExpertTeamExecutionFactsModel.test.ts`、`ExpertTeamExecutionFacts.test.tsx`、`TeamTaskDispatchDialog.test.tsx`、`TeamExecutionWorkspace.test.tsx`。使用Vitest `--maxWorkers=3 --minWorkers=1`，JSON收据 `.cache/team-execution-141/ui-tests.json`。覆盖准确节点链接、服务端顺序、截断校验、未知原因、暂停计划继续刷新、当前权限与原计划上下文。
- `node node_modules/typescript/bin/tsc --noEmit` 通过；`node scripts/check-i18n.mjs` 通过，7228对语言键，生产硬编码0。键数不代表全平台翻译完成率。
- `node node_modules/vite/bin/vite.js build --outDir ../../.cache/team-execution-141/web` 通过，44.17秒，保留既有大chunk提示。未启动服务或覆盖活动预览。App、navigation、Inbox、生成TS与本阶段UI文件在构建前后摘要一致；这是指定文件检查，不声称捕获所有编译器输入。入口SHA256为 `c54353045b23b1723d9f1e428f0e33f9f830e4f9d80b41795997d535625ba8da`；紧凑命令收据 `.cache/team-execution-141/ui-summary.json`。

环境为Windows、仓库内Go1.26.10及Node22.16.0，模块、构建和临时缓存使用仓库 `.cache/`，未改变全局环境。复用未变化的139／140证据，不重复全平台套件；本阶段机器摘要见[同阶段摘要](expert-team-execution-summary.json)。

## 联合验证与实际修复

用户明确恢复43959访问授权后，使用同一来源上的匹配控制面与Web构建完成代表性实际旅程。旧139实例与原始数据已保留证据；本轮使用SQLite在线备份副本和新合成项目。生产输入与二进制／前端资产摘要见 `.cache/e2e/collaboration-140-142/build/`，不将旧139界面验收重复算作本阶段。

- 通过正式API、当前身份和本地受控DSH协议准备三份专家计划：真实审批等待、真实协议缺失整体结束消息后的reconciling、尚未领取的ready排队。没有直接写数据库、商业模型调用或制造accepted；原Runner已核实停止。对象与准确节点／attempt记录见 `.cache/team-execution-141/live/{ready.json,readback.json,stop-proof.json}`。
- 实际GUI确认waiting与queued计入等待，executing为0；协议中断显示效果待核对、保留attempt1，不自动重试；attempt0深链不伪造attempt1，原run／node／attempt1与浏览器返回计划均准确。中文桌面浅色、英文390px深色、长原因／Runner换行和键盘焦点已检查。暂停waiting计划后原运行继续等待，读取时间从07:58:44更新至07:59:54，未手动刷新。原始截图与逐项记录见 `dist/quality/collaboration-140-142/`。
- 实际暂停成功后出现错误的草稿离开提示。回归先复现失败，修复为同一计划的回执只原位重新读取，不重复触发导航；一般离开保护、未知操作恢复不放宽。`TeamTaskDispatchDialog.test.tsx` 10项通过，8.56秒（单文件受影响复验，非新增10项）。
- 审查发现历史修订仍把queued计为Executing、waiting_execution计为Blocked。新增 `TestExpertTeamExecutionHistoryCountsAfterPendingRevision` 先复现失败，修复后history／projection相关9个顶层用例通过，5.146秒。命令：`go test ./internal/platform -run 'TestExpertTeam(ExecutionHistoryCountsAfterPendingRevision|ExecutionProjection|CurrentModelExcludesOriginalOnlyHistoryAndUsesRevisionTime|PlanCommandsReviseOnlyPendingStopAndReplay|OrchestrationReconcilingNodeProjection|OrchestrationBoundedFairDueAndNoopVersion)' -count=1 -v`。日志 `.cache/team-execution-141/{history-red.log,history-tests.log}`。验证历史明细与列表统计一致、不保存动态观察、不重写旧修订、不改变运行和attempt；合成节点状态仅证明分类规则。

## 尚未验证

上述暂停提示与历史统计修复等待统一新构建后的受影响GUI复验。实际浏览器未覆盖30节点截断、真实撤权与故障注入，继续沿用有明确范围的Go／DOM证据。140／142联调中发现的测试计划角色和新请求运行读取问题由原范围修复后复验；整个联合阶段尚未据此提交或推送。

未新增WorkBuddy云任务执行协议、厂商模型调用、物理多机或生产验收。WorkBuddy官方协议核查只补入[接入矩阵](../integrations/development-tool-matrix.md)，不把研究或目录标识视为可执行集成。完整团队自治、工具接入、离线同步和平台范围继续保留。
