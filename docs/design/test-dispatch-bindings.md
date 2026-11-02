# 明确测试派发绑定设计

速码1.0.0 · T-TEST-DISPATCH-BINDINGS-216。单一当前合同：[test-dispatch-bindings.md](../../contracts/test-dispatch-bindings.md)。本设计扩展原计划，不引入另一套执行或质量权威。

## 输入和冻结事实

原`TaskDispatchPreviewRequest.test_bindings`为可选数组，最多64项；显式`null`、缺失项字段、未知字段、大小写替代键和重复节点不作为有效选择。每项仅包含`node_id`、`tool_profile_id`、`expected_tool_version`。省略数组保留原发布定义；可以只明确覆盖其中一部分关联测试。

原`TaskDispatchPlan.test_bindings`只保存本次明确覆盖的六项事实：`node_id`、`tool_profile_id`、`tool_profile_version`、`tool_profile_sha256`、`worker_id`、`command_id`。版本、SHA、Worker和命令由服务端从当前准确工具解析。读取历史不会补读新元数据冒充原快照。

接口保持原`POST /task-dispatch-plans`、`GET /task-dispatch-plans/{id}`和`POST /task-dispatch-plans/{id}/start`；项目参数、当前权限、操作回执及版本语义不变。没有新增表、后端路径、外部SDK或调度器。

## 服务模块及事务

`task_dispatch_test_bindings.go`在原事务内完成选择解析、流程副本覆盖和冻结事实校验。目标必须是原发布图中的静态`test`，`candidate_from`准确指向所选开发节点，并具有一个固定命令；不得兼任`artifact_from`或借用其他产出节点。

只修改该节点的命令、工具ID及SHA。原`command:<旧命令>`这一准确命令约束可随明确选择替换为新命令约束；其他能力约束不得删除、放宽或静默忽略。节点身份、类型、依赖、条件、输入、超时和重试保持原定义，最终仍通过原`dag.Compile`。

每项复用原工具准入：同租户及项目、当前权限、准确版本和SHA、启用状态、新鲜匹配库存、当前执行身份及项目路由。工具必须是可写的`command/text-v1`，其`TestReportProtocol`为`suma-junit-report-v1`。测试Worker必须非空且不同于开发Worker。

预览后，原计划读取、原子Start前置门及最终运行路由钩子重复核对当前依据。冻结元数据还必须与真正执行的流程节点一致，不能只验证展示DTO。失败沿原事务回滚，不能留下额外运行或部分业务绑定；相同操作key恢复原计划或运行，改变正文仍冲突。

## 工作台与推荐

明确选择受当前账户、组织、API调用者、项目、任务版本、流程版本、开发节点和开发工具版本/SHA共同约束。目录分页不决定已选项是否有效；准确采用读取后才能提交。切源和ABA通过单调来源标识使旧异步结果失效，原pending优先冻结正文与key。

测试覆盖存在或仍未确定时，推荐的`creationDisabled`只阻止新创建及新采用，不覆盖原待确认操作的恢复入口。正式预览是本阶段承载明确覆盖的入口，推荐不得静默丢弃这些输入。

## 原质量门闭环

纯Run验收门`validateCodeTestEvidence`按实际producer逐一匹配测试，不再仅按提交SHA归组。双方节点ID及Worker须非空，当前attempt/fence为正数，测试成功且Worker不同；测试来源四项必须与该producer一致，测试`base=head=producer head`。双方attempt都为1合法，数值不同不是独立性的证据。

小纯来源匹配谓词由纯Run门与原严格数据库配对复用。纯Run投影没有完整节点配置及数据库候选固定记录，不能取代原`dag_candidate_revisions`、DAG完成核对、完整JUnit报告或Review事务。此门修复的是同Worker证据被当作独立测试的问题，不新增人工审批流程。

## AI开发与排障SOP

先核对原合同和已发布节点，再检查显式请求、六字段快照、派生节点及当前工具是否一致。准入失败先定位权限、版本、库存、Worker或路由依据，不通过替换SHA、复制报告、改变producer或发布临时图绕过。写结果未知时只恢复原操作。

验证集中于新增选择、快照一致性、当前资格、幂等恢复及准确来源配对；已通过且输入未变的原链路证据复用。实际运行、界面与正式验收结果由最终质量记录给出，本文不提前填写汇总数量。完整31项／P0–P7及跨机器、其他数据库、厂商模型、安装、规模和生产验证仍在总体目标内。

阶段结果已记录于[限定验证](../quality/test-dispatch-bindings-verification.md)：真实Git／另一Worker同提交JUnit及夹具Review通过；UI只提交预览，启动与Review由Go宿主完成。SQLite取消恢复采用官方驱动修复，完整服务包／厂商模型及正式客户签署仍有各自验收边界。
