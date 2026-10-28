# 团队执行工作台合同

状态：T-TEAM-110／1.0.0，当前实施合同。负责人：主集成、工作台代理。关联需求：团队化开发、任务进度管理及完整平台需求1、9、12、13、23。业务语义沿用[专家团队派发](expert-team-dispatch.md)、[依赖编排](expert-team-orchestration.md)和[共享执行池](execution-pools.md)；本合同只扩展已有执行记录入口，不增加调度器、聚合接口或派发权限。

## 入口与读取

`ExecutionWorkspace`提供执行记录、调度队列、团队执行三种视图。团队执行无需先选择任务，使用已有接口：

- `GET /api/v1/expert-team-plans?project_id=…&limit=10`读取初始页，`before=…`使用原响应的`next_cursor`继续读取；最多每次10个计划，按计划ID合并已加载项。
- `GET /api/v1/expert-team-plans/{id}?project_id=…`读取选中计划，复用`TeamTaskDispatchDialog`及原计划详情／修订／命令合同。

读取只展示当前可见的项目事实，不创建计划、运行、预留或模型调用。服务端继续校验当前账号、组织、项目及凭证权限；客户端身份、团队称谓、资源名称和URL都不是授权。

## 信息范围与状态

每个计划显示真实团队名称、状态、源修订，以及原`ExpertTeamPlanProgress`计数和阻塞原因；待验收、已验收、等待、执行中、阻塞、停止分别保留，不合并成一个虚构完成率。预览不表示已派发，运行成功不表示正式验收，停止未开始派发不表示运行进程已经停止。

`total`是服务端最近一次成功响应中的当前可见计划总数；已加载数只表示客户端持有的不同计划数量。逐计划进度不是全项目聚合指标，不对已加载页求和后声称项目／组织总进度。列表不从注册、`last_seen_at`、工具安装或当前状态推断机器健康、空闲容量或物理多机运行。

只有原响应提供下一游标时才允许加载更多。刷新重新读取列表；它不派发任务，也不保证完整分页或滚动位置持久恢复。列表没有浏览器派发循环或额外全项目定时轮询；打开详情的既有限界刷新规则继续有效。

## URL与隔离

采用现有路由字段：`view=development&sub=runs&layout=team`，选中详情使用`panel=expert-team:<planID>`，并保留当前`project`。刷新或分享链接后，先通过当前权限读取，再恢复对应视图／计划；计划不存在或不可读时显示准确错误，不自动定位其他计划。关闭详情清除当前面板字段，返回同一列表，不更改项目或派发新任务。

账号、组织、项目构成独立读取范围。切换范围立即隐藏旧列表与详情，丢弃旧请求的迟到结果，不让原范围的记录、错误或游标进入新范围。401／403清除可见旧数据并停止后续分页；瞬时读取失败可保留已加载快照，但必须明确状态尚未确认并提供读取重试，不能把失败展示为空项目或健康状态。

## 命令与恢复

列表打开、刷新、翻页和查看详情均为读取。启动、暂停、恢复、停止及未开始项修订继续通过原详情明确操作，使用原版本与幂等标识并检查当前权限。未知写入结果沿原请求恢复；读取重试不等于重新启动，暂停／恢复不等于创建替代运行，失败任务的重试与核对仍在原运行／attempt中处理。历史修订不提供当前操作权限。

## 参考及验收边界

只读研究Multica固定提交`a47a1ced0c71bdb113a43facce4675442acb9001`中的[`server/internal/dispatch/reason.go`](https://github.com/multica-ai/multica/blob/a47a1ced0c71bdb113a43facce4675442acb9001/server/internal/dispatch/reason.go)和[`server/pkg/db/queries/runtime.sql`](https://github.com/multica-ai/multica/blob/a47a1ced0c71bdb113a43facce4675442acb9001/server/pkg/db/queries/runtime.sql)：借鉴来源明确的派发原因、可见范围与有界查询。本轮不宣称该提交是最新版本，不修改参考工作树，不复制其源码，也不引入与速码原调度器并行的控制面。

对应实现入口为`apps/web/src/ExecutionWorkspace.tsx`与现有团队计划详情；用户说明见[中文](../docs/development/team-execution.md)／[English](../docs/en/team-execution.md)。验收应覆盖分页／总数／范围切换／迟到响应／错误重试／深链和中英文呈现，复用原调度及命令证据。仅有页面和同机协议验证不能证明物理多机、外部Multica服务互操作、厂商工具任务、完整IDE、吞吐或生产验收。
