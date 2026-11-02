# 目标工具上报反馈合同

T-TOOL-REPORT-FEEDBACK-204；suma 1.0.0；负责人：主集成代理。关联[工具准备合同](tool-preparation-recovery.md)与[后续计划](../docs/delivery/tool-management-next.md)。完整31项／P0–P7及58 limited／34 pending／92项基线保持active。

## 目的和权威

打通原RemoteRunnerSetupGuide的报告步骤到服务端当前库存，再明确回到同一工具／节点的Quick核对。复用原GET /tool-discovery、正式适配目录、观察解析、完整capability匹配、原matches／暂停登记／独立启用，不增加业务API、表、CLI模式、库存事件流或第二派发器。

复制命令不表示执行。ToolPreparation只表示配置文件形成；Runner recovered=true只表示原报告ACK恢复，不能当成新探测或续期。服务端ObservedAt是报告接收时间，不是命令执行时间。新库存可能来自现有Worker，不能归因为刚复制的命令或本次文件执行成功。

## 当前读取与限时等候

- 目标必须有当前项目、明确worker与九项已支持localTool；项目必须等于当前项目上下文，不能猜唯一节点或借手动输入切换授权范围。
- 使用当前调用者读取正式适配目录及原库存，严格复用readAdapterCatalog、parseToolDiscovery及matchingQuickTools，保留完整协议／profile／native与SDK能力校验，不按产品名合并或另造健康规则。
- 提供一次“检查上报”和明确“等待新清单”。复制动作不自动开始或重置等候；当前已有新鲜观察可供核对，不称本次命令已完成。
- 等候先取得并固定服务端baseline.asOf及baseline.observedAt。只有当前同scope的fresh报告且observedAt严格晚于baseline.asOf及已有的baseline.observedAt两个边界，才显示“收到新清单”；比较保留9位小数精度，不用Date.parse毫秒舍入或浏览器墙钟。
- 时间相同、回退及旧ACK不算新报告；它们可能仍是可核对的当前库存，不伪造报告修订号或operation关联。此机制不承诺辨识同一数据库时刻的不同操作。
- 单flight、最多60秒／每3秒一次；页面隐藏暂停读取，总截止不续；取消、卸载、目标／调用者／配置采用代次变化立即中止并拒绝迟到响应。401／403／404撤掉观察并停止，网络失败／超时表达未知，不等同未执行。
- 新鲜观察的按钮资格按响应remainingMs减去请求耗时、monotonic clock和后续经过时间校验。过期必须撤掉继续资格，不能等下一次轮询才隐藏旧结果。

## 准确返回与草稿

前端本地类型ToolReportFeedbackTarget为{project,workerID,localTool}。明确选择一个当前可核对条目时输出ToolReportFeedbackSelection={project,workerID,adapterID,commandID,profileSHA256,observedAt}。这些是受核观察，不授予执行权限。

回调只在当前已核caller／scope／intent下生效；原Workspace重新核当前上下文和写入阻塞，明确开启准确adapterID／workerID／commandID的新Quick草稿。目标改变时清旧名称、命令及replacementSource，不隐式迁移旧换新意图。Quick仍重新读取当前事实，反馈本身不登记或启用。

Remote手动改变目标、路径／壳／身份／控制地址或采用新的configurationRequest时，都必须失效旧观察及等候，即便新的配置恰好仍写同一路径。普通渲染不覆盖人工字段。A→B→A、相同nonce跨目标、迟到权限失败均不得复活原结果；现有高水位规则保留。

## 分工与验证

控制代理拥有反馈纯模型、原时间精度函数的单一导出及定向模型检查。交互代理拥有反馈组件、原Guide／Workspace准确返回接线、双语／样式和受影响组件检查。执行代理拥有隔离真实HTTP／SQLite反馈夹具；主集成代理拥有公共合同、实际浏览器驱动／构建、文档、通用检查及阶段Git。

验证只覆盖本次改变：真实当前API报告、旧ACK不刷新、新库存、权限失效、精度／过期、取消／隐藏、准确返回及双语代表界面。使用自有报告元数据夹具时明确区分厂商CLI与模型，不重复未变化SDK或完整测试链。需要严格operation与文件因果时，另做正式来源版本设计，不能靠现时间字段假称已实现。
