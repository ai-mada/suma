# 工作区宿主操作合同

状态：当前1.0.0实现合同；负责人：执行运行时／控制面／工作台。关联：需求13、23、P2／P4、T-WORKSPACE-120。实现：`internal/workspaceoperationcontract`、`internal/runner/native_workspace_trace.go`、`internal/dag/workspace_operation_progress.go`；证据：[阶段报告](../docs/quality/workspace-operations-verification.md)。

## 来源与身份

`suma-workspace-operation-v1`是Go宿主事实，与SDK的`ToolEvent`分开。仅复用当前`/runner/progress`、`/pool-runner/progress`、`tool_progress`事件、SSE和确切attempt历史分页，不新增派发器或工具执行入口。

私有请求是恰四字段`{project_id,assignment,operation_id,host_operation}`；Pool请求另外有`runner_id,tenant_id`。不得混入`message`、`tool_event`、未知或null字段。公共Schema从实际Go DTO生成，两个请求分支互斥。SDK报文不能通过自报`source`升级为宿主事实。

`WorkspaceOperationEvent`保存宿主调用ID、修订、局部序号、动作、阶段、世代、UTC时间、请求／策略／父工具摘要，及可选安全相对路径、检查ID、效果ID、结果摘要、退出码、停止标志和受控错误码。禁止文件正文、输出、命令参数、环境、模型内容、端点和凭据。原始不合法路径不回显；根目录不伪造为文件定位。

`event_id`绑定租户、项目、运行、节点、Worker、attempt、fence、调用ID、修订和序号；HTTP的`operation_id`等于该ID。平台每次先核验当前凭证／授权、固定档案和确切持久attempt，再恢复回执。相同原操作重复回传只恢复原回执；换正文或挪用身份冲突。

## 调用与效果

动作是`list/read/write/edit/remove/check/merge_status/merge_continue/seal`。一次真实调用有`revision=1,phase=started`和`revision=2`终态；终态为`completed/failed/unknown`。参数与CAS拒绝也记录。开始记录先持久化，再准入效果；结束记录持久化后才投影。

调用ID与效果幂等ID分别保存。再次读取原write／check回执，是新的宿主调用，带`replayed`并引用原效果，不重复执行。封存由Go父进程完成，独立记入同一条宿主时间线。进程退出、检查完成、封存成功、独立测试和人工验收仍是不同事实。

`check`需要结合真实`exit_code`、`stopped`及未知状态判断。`completed`阶段本身不等于测试通过。检查详情还必须绑定策略、效果ID、世代、检查ID及结果；父工具摘要与检查命令摘要含义不同，不能互比。详情快照已被后续检查替换时显示不可用，不能按相同checkID打开另一轮输出。

## 有界持久化与恢复

每attempt最多2048次调用／4096条记录，与SDK日志上限分别计数。每次调用在效果前预留两阶段容量；达到上限明确停止新操作，保留部分覆盖状态，禁止静默截断成功。`operation_trace`只描述本地采集，缺失表示未提供。它不证明平台确认或浏览器已载入全部记录。

开始MCP前，父Journal绑定原工作区私有账本及完整assignment／策略／档案摘要。重启只导入匹配原账本中的安全元数据；开始而未闭合的调用标记未知，不执行写入、检查、模型或Git。原效果的CAS与核对合同继续有效。网络或回执失败不触发效果重做。

旧attempt可在当前权限仍成立时补传原元数据；平台只追加到该原attempt，不续租、不替换新结果、不推进下游。授权已失效时保留本地待确认记录，不能绕过撤权。平台历史仍受固定水位、扫描／字节预算和当前权限限制。

## 展示规则

工作台默认收起操作区，按真实局部序号展示；平台外层事件序号只表示持久接收顺序。按调用ID归并合法修订，不由日志或动画推断进度。矛盾／不支持的记录保留原文且不宣称成功。当前窗口、确切历史页与本地采集覆盖分别标明；筛选、切换attempt、撤权与重试不得混用其他执行的缓存。

该合同不增加OS隔离证明、任意命令权限、重试授权、业务验收或发布授权。物理多主机、全平台审计和生产性能仍需独立验收。
